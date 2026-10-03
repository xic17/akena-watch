// Akena Watch — limitador de intentos de acceso (anti fuerza bruta).
package server

import (
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// LoginLimit configura el límite de intentos de acceso.
type LoginLimit struct {
	Intentos int           // fallos permitidos antes de bloquear
	Bloqueo  time.Duration // bloqueo inicial (se dobla con cada fallo extra)
	Memoria  time.Duration // cuánto se recuerda un fallo sin más actividad
}

// Valores por defecto. Se pueden cambiar con las variables de entorno
// AKENA_LOGIN_INTENTOS, AKENA_LOGIN_BLOQUEO_SEG y AKENA_LOGIN_MEMORIA_MIN.
var loginLimitPorDefecto = LoginLimit{
	Intentos: 5,
	Bloqueo:  30 * time.Second,
	Memoria:  15 * time.Minute,
}

const (
	bloqueoMaximo     = 10 * time.Minute // tope del bloqueo creciente
	maxRegistros      = 10000            // tope de claves por mapa (evita agotar memoria)
	intervaloLimpieza = time.Minute      // cada cuánto se olvidan los fallos viejos
)

// loginLimitFromEnv lee la configuración del entorno; cualquier valor ausente
// o inválido deja el valor por defecto.
func loginLimitFromEnv() LoginLimit {
	c := loginLimitPorDefecto
	if v, err := strconv.Atoi(os.Getenv("AKENA_LOGIN_INTENTOS")); err == nil && v > 0 {
		c.Intentos = v
	}
	if v, err := strconv.Atoi(os.Getenv("AKENA_LOGIN_BLOQUEO_SEG")); err == nil && v > 0 {
		c.Bloqueo = time.Duration(v) * time.Second
	}
	if v, err := strconv.Atoi(os.Getenv("AKENA_LOGIN_MEMORIA_MIN")); err == nil && v > 0 {
		c.Memoria = time.Duration(v) * time.Minute
	}
	return c
}

// intentos acumula los fallos de una clave (un usuario o una IP).
type intentos struct {
	fallos         int
	bloqueadoHasta time.Time
	visto          time.Time
}

// loginLimiter cuenta los intentos de acceso fallidos.
//
// Guarda los fallos por nombre de usuario y por IP. La clave por usuario es la
// que de verdad protege: el nombre lo elige quien ataca, así que no puede
// esquivarla como esquivaría una IP falsificando cabeceras —atacar la misma
// cuenta siempre cae en el mismo contador—. La clave por IP, además, frena el
// rocío de contraseñas sobre muchas cuentas distintas.
//
// Agotados los intentos, cada fallo nuevo dobla el bloqueo hasta un tope. Un
// acceso correcto borra el historial de esa cuenta y de esa IP.
//
// Todo vive en memoria: reiniciar Akena Watch perdona los bloqueos, que es lo
// que quiere el administrador legítimo y algo que quien ataca no puede forzar
// desde fuera.
type loginLimiter struct {
	cfg            LoginLimit
	mu             sync.Mutex
	porUsuario     map[string]*intentos
	porIP          map[string]*intentos
	ultimaLimpieza time.Time
}

func newLoginLimiter(cfg LoginLimit) *loginLimiter {
	if cfg.Intentos <= 0 {
		cfg.Intentos = loginLimitPorDefecto.Intentos
	}
	if cfg.Bloqueo <= 0 {
		cfg.Bloqueo = loginLimitPorDefecto.Bloqueo
	}
	if cfg.Memoria <= 0 {
		cfg.Memoria = loginLimitPorDefecto.Memoria
	}
	return &loginLimiter{
		cfg:            cfg,
		porUsuario:     map[string]*intentos{},
		porIP:          map[string]*intentos{},
		ultimaLimpieza: time.Now(),
	}
}

// espera indica cuánto falta para poder volver a intentar el acceso. Cero
// significa que el intento puede seguir adelante.
func (l *loginLimiter) espera(usuario, ip string) time.Duration {
	ahora := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.limpiarSiToca(ahora)
	d := restante(l.porUsuario[claveUsuario(usuario)], ahora)
	if dIP := restante(l.porIP[ip], ahora); dIP > d {
		d = dIP
	}
	return d
}

// fallo registra un intento fallido y devuelve el bloqueo aplicado (cero si
// todavía quedan intentos antes de bloquear).
func (l *loginLimiter) fallo(usuario, ip string) time.Duration {
	ahora := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.limpiarSiToca(ahora)
	d := l.anotar(l.porUsuario, claveUsuario(usuario), ahora)
	if dIP := l.anotar(l.porIP, ip, ahora); dIP > d {
		d = dIP
	}
	return d
}

// exito limpia el historial: quien acierta la contraseña empieza de cero.
func (l *loginLimiter) exito(usuario, ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.porUsuario, claveUsuario(usuario))
	delete(l.porIP, ip)
}

// anotar suma un fallo a una clave y programa el bloqueo correspondiente.
// Los valores del llamador deben venir ya normalizados (claveUsuario).
func (l *loginLimiter) anotar(m map[string]*intentos, clave string, ahora time.Time) time.Duration {
	r := m[clave]
	if r == nil {
		// Tope de memoria: si el mapa está lleno, no se registra la clave
		// nueva. La otra clave del limitador (usuario o IP) sigue contando.
		if len(m) >= maxRegistros {
			return 0
		}
		r = &intentos{}
		m[clave] = r
	}
	r.fallos++
	r.visto = ahora
	if r.fallos < l.cfg.Intentos {
		return 0
	}
	d := l.cfg.Bloqueo
	for i := l.cfg.Intentos; i < r.fallos && d < bloqueoMaximo; i++ {
		d *= 2
	}
	if d > bloqueoMaximo {
		d = bloqueoMaximo
	}
	r.bloqueadoHasta = ahora.Add(d)
	return d
}

func restante(r *intentos, ahora time.Time) time.Duration {
	if r == nil {
		return 0
	}
	if d := r.bloqueadoHasta.Sub(ahora); d > 0 {
		return d
	}
	return 0
}

// limpiarSiToca olvida las claves sin actividad para que los mapas no crezcan
// sin fin. Se llama desde las propias consultas, como mucho una vez por minuto.
func (l *loginLimiter) limpiarSiToca(ahora time.Time) {
	if ahora.Sub(l.ultimaLimpieza) < intervaloLimpieza {
		return
	}
	l.ultimaLimpieza = ahora
	for _, m := range []map[string]*intentos{l.porUsuario, l.porIP} {
		for k, r := range m {
			if r.bloqueadoHasta.After(ahora) {
				continue // sigue bloqueado: se conserva hasta que expire
			}
			if ahora.Sub(r.visto) > l.cfg.Memoria {
				delete(m, k)
			}
		}
	}
}

// claveUsuario normaliza el nombre para contar los intentos. Los usuarios no
// distinguen mayúsculas (UNIQUE COLLATE NOCASE), así que "Akena" y "akena" son
// la misma cuenta y deben caer en el mismo contador. También se recorta la
// longitud: el nombre viene del cliente y no puede inflar la memoria.
func claveUsuario(usuario string) string {
	u := strings.ToLower(strings.TrimSpace(usuario))
	if len(u) > 64 {
		u = u[:64]
	}
	return u
}

// clientIP devuelve la IP con la que se cuenta un intento.
//
// Detrás de un proxy inverso en la misma máquina (el nginx de CloudPanel)
// todas las peticiones llegan desde loopback, así que si el cliente no se
// identifica todas caerían en el mismo contador. En ese caso —y solo en ese,
// porque solo el proxy local puede hablar por loopback— se acepta la cabecera
// que pone el proxy. Desde cualquier otra dirección se usa la IP del socket,
// que no se puede falsificar.
//
// Esta IP solo alimenta el límite por IP: la clave por usuario no depende de
// nada que el cliente pueda inventar.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return host
	}
	if v := ipDeProxy(r); v != "" {
		return v
	}
	return host
}

// ipDeProxy lee la IP del cliente de las cabeceras habituales del proxy.
// Se prefiere X-Real-IP (nginx la fija con $remote_addr) y, si falta, el
// último salto de X-Forwarded-For, que es el que vio el proxy inmediato: el
// primer salto lo controla quien envía la petición. Se descartan valores que
// no sean una IP, y también loopback, que no puede ser un cliente real de un
// proxy local.
func ipDeProxy(r *http.Request) string {
	if v := ipValida(r.Header.Get("X-Real-IP")); v != "" {
		return v
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		saltos := strings.Split(xff, ",")
		return ipValida(saltos[len(saltos)-1])
	}
	return ""
}

func ipValida(s string) string {
	ip := net.ParseIP(strings.TrimSpace(s))
	if ip == nil || ip.IsLoopback() {
		return ""
	}
	return ip.String()
}
