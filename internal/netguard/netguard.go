// Package netguard decide a qué direcciones puede conectarse Akena Watch.
//
// El binario hace peticiones salientes por diseño: monitores, herramientas
// (inspección HTTP, TLS, puertos, ping) y canales de alerta. Eso abre la puerta
// al SSRF: alguien con sesión podría apuntar a servicios internos del propio
// servidor o a los metadatos del proveedor cloud.
//
// Hay dos niveles de política:
//
//   - Siempre se bloquean las direcciones que nunca son un destino legítimo:
//     link-local (incluido 169.254.169.254, los metadatos del cloud),
//     no especificadas y multicast.
//   - En modo estricto (lo activa el modo demo o AKENA_SOLO_PUBLICO) además se
//     bloquean loopback, rangos privados y CGNAT: solo se permiten destinos
//     públicos.
//
// La comprobación se hace en el momento de conectar, con la IP ya resuelta
// (Dialer.Control), así que una redirección o un DNS que devuelva una IP
// interna tampoco se cuelan.
package netguard

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"syscall"
	"time"
)

// ErrBloqueado identifica un destino rechazado por la política de red.
var ErrBloqueado = errors.New("destino interno bloqueado por la política de red")

var soloPublico atomic.Bool

// Configurar fija el modo estricto. Se llama una vez al arrancar.
func Configurar(estricto bool) { soloPublico.Store(estricto) }

// Activo indica si está vigente el modo estricto (solo destinos públicos).
func Activo() bool { return soloPublico.Load() }

// Permitido aplica la política a una IP ya resuelta.
func Permitido(ip net.IP) bool {
	if ip == nil {
		return false
	}
	// Nunca legítimas, en ningún modo.
	if ip.IsUnspecified() || ip.IsMulticast() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	if !soloPublico.Load() {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() {
		return false
	}
	// 100.64.0.0/10 (CGNAT), que IsPrivate no cubre.
	if v4 := ip.To4(); v4 != nil && v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
		return false
	}
	return true
}

// Comprobar valida un host ("ejemplo.com", "ejemplo.com:443" o una IP) antes de
// usarlo, para poder dar un error claro. Si el nombre no resuelve no se rechaza:
// que falle la conexión con su propio error.
func Comprobar(host string) error {
	nombre := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		nombre = h
	}
	if ip := net.ParseIP(nombre); ip != nil {
		if !Permitido(ip) {
			return fmt.Errorf("%w: %s", ErrBloqueado, ip)
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, nombre)
	if err != nil || len(addrs) == 0 {
		return nil
	}
	for _, a := range addrs {
		if Permitido(a.IP) {
			return nil // basta con que una dirección sea válida
		}
	}
	return fmt.Errorf("%w: %s", ErrBloqueado, nombre)
}

// Dialer devuelve un dialer que aplica la política a cada conexión, con la IP
// ya resuelta. Es la pieza que usan TCP, TLS y el escaneo de puertos.
func Dialer(timeout time.Duration) *net.Dialer {
	d := &net.Dialer{Timeout: timeout}
	d.Control = func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			host = address
		}
		ip := net.ParseIP(host)
		if ip == nil || !Permitido(ip) {
			return fmt.Errorf("%w: %s", ErrBloqueado, host)
		}
		return nil
	}
	return d
}

// Cliente devuelve un cliente HTTP cuyo transporte pasa por Dialer, de modo que
// también las redirecciones y los DNS cambiantes quedan sujetos a la política.
func Cliente(timeout time.Duration, redirigir func(*http.Request, []*http.Request) error) *http.Client {
	d := Dialer(30 * time.Second)
	tr := &http.Transport{
		DialContext:           d.DialContext,
		MaxIdleConns:          10,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
	}
	// Un proxy intermedio se saltaría la comprobación: en modo estricto no se usa.
	if !soloPublico.Load() {
		tr.Proxy = http.ProxyFromEnvironment
	}
	return &http.Client{Timeout: timeout, Transport: tr, CheckRedirect: redirigir}
}
