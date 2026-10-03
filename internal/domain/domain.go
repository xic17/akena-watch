// Package domain averigua cuándo vence el registro de un dominio.
//
// Se pregunta primero por RDAP (JSON sobre HTTPS, estructurado) y, si el
// registro no publica la fecha por esa vía, se recurre a WHOIS (puerto 43) y se
// extrae la fecha del texto. Hay registros que sencillamente no publican la
// fecha de vencimiento —.de, por ejemplo—: en ese caso se devuelve ErrSinFecha.
package domain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/likexian/whois"
	"golang.org/x/net/publicsuffix"
)

// ErrSinFecha indica que el registro existe pero no publica (o no se ha sabido
// leer) la fecha de vencimiento.
var ErrSinFecha = errors.New("el registro no publica la fecha de vencimiento")

const (
	rdapBase    = "https://rdap.org/domain/" // redirector al RDAP del registro
	httpTimeout = 12 * time.Second
	whoisTTL    = 20 * time.Second
	maxCuerpo   = 1 << 20 // 1 MiB: las respuestas RDAP son pequeñas
)

// Info es lo que se sabe del vencimiento de un dominio.
type Info struct {
	Host      string    `json:"host"`       // lo que pidió el usuario
	Domain    string    `json:"domain"`     // dominio registrable (eTLD+1)
	ExpiresAt time.Time `json:"expires_at"` // fecha de vencimiento
	DaysLeft  int       `json:"days_left"`  // días que faltan (negativo = vencido)
	Registrar string    `json:"registrar,omitempty"`
	Source    string    `json:"source"` // "rdap" o "whois"
}

// Lookup consulta el vencimiento del dominio al que pertenece la entrada, que
// puede ser una URL, un host con puerto o un dominio.
func Lookup(ctx context.Context, entrada string) (Info, error) {
	host := Normalizar(entrada)
	if host == "" {
		return Info{}, errors.New("indica un dominio válido")
	}
	dom := Registrable(host)
	if dom == "" {
		return Info{}, fmt.Errorf("%q no parece un dominio con registro público", entrada)
	}
	info := Info{Host: host, Domain: dom}

	if vence, registrador, err := viaRDAP(ctx, dom); err == nil {
		return completar(info, vence, registrador, "rdap"), nil
	}
	if vence, err := viaWHOIS(ctx, dom); err == nil {
		return completar(info, vence, "", "whois"), nil
	}
	return info, ErrSinFecha
}

// Normalizar acepta lo que suele pegarse en un formulario —una URL, un
// host:puerto o un dominio— y devuelve solo el nombre de host en minúsculas.
func Normalizar(entrada string) string {
	s := strings.TrimSpace(entrada)
	if s == "" {
		return ""
	}
	if strings.Contains(s, "://") {
		if u, err := url.Parse(s); err == nil && u.Host != "" {
			s = u.Host
		}
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	if host, _, err := net.SplitHostPort(s); err == nil {
		s = host
	} else if i := strings.LastIndex(s, ":"); i >= 0 && soloDigitos(s[i+1:]) {
		s = s[:i] // host:puerto que SplitHostPort rechaza
	}
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(s)), ".")
}

// Registrable devuelve el dominio registrable (eTLD+1): de
// "panel.api.ejemplo.com" devuelve "ejemplo.com". Devuelve "" para una IP o un
// nombre sin sufijo público conocido (por ejemplo un host de la red local).
func Registrable(host string) string {
	if host == "" || net.ParseIP(host) != nil {
		return ""
	}
	dom, err := publicsuffix.EffectiveTLDPlusOne(host)
	if err != nil {
		return ""
	}
	return dom
}

func completar(info Info, vence time.Time, registrar, fuente string) Info {
	info.ExpiresAt = vence.UTC()
	// Redondeo hacia arriba, igual que el aviso de certificado: el día del
	// vencimiento cuenta como 0 días restantes.
	info.DaysLeft = int(math.Ceil(time.Until(vence).Hours() / 24))
	info.Registrar = registrar
	info.Source = fuente
	return info
}

func soloDigitos(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// --- RDAP ---

type rdapEntity struct {
	Roles      []string `json:"roles"`
	VCardArray []any    `json:"vcardArray"`
}

func viaRDAP(ctx context.Context, dom string) (time.Time, string, error) {
	ctx, cancel := context.WithTimeout(ctx, httpTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rdapBase+url.PathEscape(dom), nil)
	if err != nil {
		return time.Time{}, "", err
	}
	req.Header.Set("Accept", "application/rdap+json, application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return time.Time{}, "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return time.Time{}, "", fmt.Errorf("rdap respondió %d", res.StatusCode)
	}

	var datos struct {
		Events []struct {
			Action string `json:"eventAction"`
			Date   string `json:"eventDate"`
		} `json:"events"`
		Entities []rdapEntity `json:"entities"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, maxCuerpo)).Decode(&datos); err != nil {
		return time.Time{}, "", err
	}
	for _, ev := range datos.Events {
		if !strings.EqualFold(ev.Action, "expiration") && !strings.EqualFold(ev.Action, "expiry") {
			continue
		}
		if t, err := time.Parse(time.RFC3339, ev.Date); err == nil {
			return t, registradorDe(datos.Entities), nil
		}
	}
	return time.Time{}, "", ErrSinFecha
}

// registradorDe busca el nombre del registrador en las entidades RDAP.
func registradorDe(entidades []rdapEntity) string {
	for _, e := range entidades {
		if !tieneRol(e.Roles, "registrar") || len(e.VCardArray) < 2 {
			continue
		}
		props, _ := e.VCardArray[1].([]any)
		for _, p := range props {
			kv, _ := p.([]any)
			if len(kv) < 4 {
				continue
			}
			if nombre, _ := kv[0].(string); strings.EqualFold(nombre, "fn") {
				if v, _ := kv[3].(string); strings.TrimSpace(v) != "" {
					return strings.TrimSpace(v)
				}
			}
		}
	}
	return ""
}

func tieneRol(roles []string, rol string) bool {
	for _, r := range roles {
		if strings.EqualFold(r, rol) {
			return true
		}
	}
	return false
}

// --- WHOIS ---

func viaWHOIS(ctx context.Context, dom string) (time.Time, error) {
	ctx, cancel := context.WithTimeout(ctx, whoisTTL)
	defer cancel()

	// La librería no acepta contexto: se ejecuta en una goroutine y se
	// abandona si la consulta se pasa de tiempo.
	type res struct {
		texto string
		err   error
	}
	ch := make(chan res, 1)
	go func() {
		texto, err := whois.Whois(dom)
		ch <- res{texto, err}
	}()

	select {
	case <-ctx.Done():
		return time.Time{}, ctx.Err()
	case r := <-ch:
		if r.err != nil {
			return time.Time{}, r.err
		}
		return FechaEnTexto(r.texto)
	}
}

// FechaEnTexto busca la fecha de vencimiento en un registro WHOIS.
//
// Reconoce las etiquetas que usan los registros más habituales (gTLD, .uk,
// .mx, .cl, .br, .es…) y varios formatos de fecha. Solo mira la etiqueta que va
// al principio de la línea, de modo que las líneas de aviso —que mencionan la
// "expiration date" sin dar una fecha propia— no cuelan.
func FechaEnTexto(texto string) (time.Time, error) {
	for _, linea := range strings.Split(texto, "\n") {
		linea = strings.TrimSpace(linea)
		i := strings.Index(linea, ":")
		if i <= 0 {
			continue
		}
		if !etiquetaDeVencimiento(linea[:i]) {
			continue
		}
		if t, err := fechaDe(linea[i+1:]); err == nil {
			return t, nil
		}
	}
	return time.Time{}, ErrSinFecha
}

// etiquetasDeVencimiento son las etiquetas exactas que se aceptan (el texto ya
// llega en minúsculas y con los espacios colapsados).
var etiquetasDeVencimiento = []string{
	"registry expiry date",
	"registrar registration expiration date",
	"domain expiration date",
	"expiry date", "expiration date", "expiration time", "expiration",
	"expiry", "expires", "expire", "paid-till", "renewal date",
	"valid until", "validity",
	"fecha de expiracion", "fecha de expiración",
	"fecha de vencimiento", "expiracion", "expiración", "vencimiento", "vence",
}

func etiquetaDeVencimiento(s string) bool {
	s = strings.ToLower(strings.TrimSpace(strings.TrimLeft(s, "%*")))
	s = strings.Join(strings.Fields(s), " ")
	for _, e := range etiquetasDeVencimiento {
		if s == e {
			return true
		}
	}
	// Variantes con prefijo: "registrar expiry date", "domain expiry", …
	return strings.HasSuffix(s, " expiry") || strings.HasSuffix(s, " expiry date") ||
		strings.HasSuffix(s, " expiration") || strings.HasSuffix(s, " expiration date")
}

var (
	reISO      = regexp.MustCompile(`(\d{4})-(\d{2})-(\d{2})`)
	reDMonY    = regexp.MustCompile(`(\d{1,2})-([A-Za-z]{3})-(\d{4})`)
	reYMDpunto = regexp.MustCompile(`(\d{4})\.(\d{2})\.(\d{2})`)
	reCompacta = regexp.MustCompile(`\b(\d{4})(\d{2})(\d{2})\b`)
	reDMYpunto = regexp.MustCompile(`(\d{1,2})\.(\d{1,2})\.(\d{4})`)
	reMesLargo = regexp.MustCompile(`([A-Za-z]{3,9})\.?\s+(\d{1,2}),?\s+(\d{4})`)
)

// fechaDe interpreta la fecha que sigue a la etiqueta. Los formatos se prueban
// de más fiable a menos: el ISO no admite ambigüedad y es el más común.
func fechaDe(valor string) (time.Time, error) {
	valor = strings.TrimSpace(valor)
	if valor == "" || valor == "-" {
		return time.Time{}, ErrSinFecha
	}
	if m := reISO.FindStringSubmatch(valor); m != nil {
		return fechaValida(time.Parse("2006-01-02", m[1]+"-"+m[2]+"-"+m[3]))
	}
	if m := reDMonY.FindStringSubmatch(valor); m != nil {
		return fechaValida(time.Parse("2-Jan-2006", m[1]+"-"+m[2]+"-"+m[3]))
	}
	if m := reMesLargo.FindStringSubmatch(valor); m != nil {
		return fechaValida(time.Parse("January 2 2006", m[1]+" "+m[2]+" "+m[3]))
	}
	if m := reYMDpunto.FindStringSubmatch(valor); m != nil {
		return fechaValida(time.Parse("2006.01.02", m[1]+"."+m[2]+"."+m[3]))
	}
	if m := reCompacta.FindStringSubmatch(valor); m != nil {
		return fechaValida(time.Parse("20060102", m[1]+m[2]+m[3]))
	}
	if m := reDMYpunto.FindStringSubmatch(valor); m != nil {
		// día.mes.año (formato europeo); si el día no encaja, el parseo falla
		return fechaValida(time.Parse("2.1.2006", m[1]+"."+m[2]+"."+m[3]))
	}
	return time.Time{}, ErrSinFecha
}

// fechaValida descarta fechas imposibles, para no confundir con una fecha
// cualquier número que aparezca en la línea.
func fechaValida(t time.Time, err error) (time.Time, error) {
	if err != nil {
		return time.Time{}, err
	}
	if t.Year() < 1990 || t.Year() > 2200 {
		return time.Time{}, ErrSinFecha
	}
	return t, nil
}
