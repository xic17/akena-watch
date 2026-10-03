package server

import (
	"errors"
	"net/http"
	"strings"

	"akena-watch/internal/domain"
)

// handleDomainCheck consulta cuándo vence el registro del dominio indicado
// (o del destino de un monitor). Es la misma consulta que hace el aviso de
// vencimiento, pero a petición: primero RDAP y, si el registro no lo publica
// por esa vía, WHOIS.
func (s *Server) handleDomainCheck(w http.ResponseWriter, r *http.Request) {
	entrada := strings.TrimSpace(r.URL.Query().Get("domain"))
	if entrada == "" || len(entrada) > 512 {
		writeErr(w, http.StatusBadRequest, "indica un dominio o una URL")
		return
	}

	info, err := domain.Lookup(r.Context(), entrada)
	switch {
	case errors.Is(err, domain.ErrSinFecha):
		writeErr(w, http.StatusNotFound,
			"no se pudo averiguar el vencimiento: el dominio puede no existir o su registro no publica la fecha")
		return
	case err != nil:
		writeErr(w, http.StatusBadGateway, "la consulta falló: "+err.Error())
		return
	}

	writeOK(w, map[string]any{
		"domain":     info.Domain,
		"host":       info.Host,
		"expires_at": info.ExpiresAt.Format("2006-01-02"),
		"days_left":  info.DaysLeft,
		"registrar":  info.Registrar,
		"source":     info.Source,
	})
}
