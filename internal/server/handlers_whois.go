package server

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/likexian/whois"
)

// handleWhois consulta el registro WHOIS de un dominio o IP desde el
// servidor (puerto 43, con resolución del servidor correcto vía IANA).
func (s *Server) handleWhois(w http.ResponseWriter, r *http.Request) {
	domain := strings.TrimSpace(r.URL.Query().Get("domain"))
	if domain == "" || len(domain) > 253 {
		writeErr(w, http.StatusBadRequest, "indica un dominio o IP válidos")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	// la librería no acepta contexto: se ejecuta en una goroutine y
	// se abandona si supera el timeout
	type res struct {
		text string
		err  error
	}
	ch := make(chan res, 1)
	go func() {
		text, err := whois.Whois(domain)
		ch <- res{text, err}
	}()

	var out res
	select {
	case <-ctx.Done():
		writeErr(w, http.StatusGatewayTimeout, "la consulta WHOIS tardó demasiado")
		return
	case out = <-ch:
	}

	if out.err != nil {
		writeErr(w, http.StatusBadGateway, "consulta WHOIS falló: "+out.err.Error())
		return
	}
	if strings.TrimSpace(out.text) == "" {
		writeErr(w, http.StatusNotFound, "el registro WHOIS está vacío (el dominio puede no tener whois)")
		return
	}
	writeOK(w, map[string]any{"domain": domain, "text": out.text})
}
