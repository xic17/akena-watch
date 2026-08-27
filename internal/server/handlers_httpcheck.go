package server

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptrace"
	"strings"
	"time"
)

// handleHTTPCheck inspecciona una URL estilo curl desde el servidor:
// tiempos desglosados (DNS, conexión, TLS, TTFB, total), cadena de
// redirecciones, headers de respuesta y una vista previa del cuerpo.
func (s *Server) handleHTTPCheck(w http.ResponseWriter, r *http.Request) {
	var in struct {
		URL     string            `json:"url"`
		Method  string            `json:"method"`
		Headers map[string]string `json:"headers"`
		Body    string            `json:"body"`
	}
	if err := readJSON(w, r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "solicitud inválida")
		return
	}
	urlStr := strings.TrimSpace(in.URL)
	if !strings.HasPrefix(urlStr, "http://") && !strings.HasPrefix(urlStr, "https://") {
		writeErr(w, http.StatusBadRequest, "la URL debe comenzar con http:// o https://")
		return
	}
	if len(urlStr) > 2048 {
		writeErr(w, http.StatusBadRequest, "la URL es demasiado larga")
		return
	}
	method := strings.ToUpper(strings.TrimSpace(in.Method))
	if method == "" {
		method = http.MethodGet
	}
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch,
		http.MethodDelete, http.MethodHead, http.MethodOptions:
	default:
		writeErr(w, http.StatusBadRequest, "método HTTP no válido")
		return
	}

	var (
		dnsMs, connMs, tlsMs, ttfbMs float64
		dnsStart, connStart, tlsStart time.Time
		tlsProto                     string
		certSubject, certIssuer      string
		certExpires                  string
		certDays                     int
		redirects                    []map[string]any
		start                        = time.Now()
	)

	trace := &httptrace.ClientTrace{
		DNSStart:          func(httptrace.DNSStartInfo) { dnsStart = time.Now() },
		DNSDone:           func(httptrace.DNSDoneInfo) { dnsMs = float64(time.Since(dnsStart).Microseconds()) / 1000 },
		ConnectStart:      func(string, string) { connStart = time.Now() },
		ConnectDone:       func(string, string, error) { connMs = float64(time.Since(connStart).Microseconds()) / 1000 },
		TLSHandshakeStart: func() { tlsStart = time.Now() },
		TLSHandshakeDone: func(cs tls.ConnectionState, err error) {
			tlsMs = float64(time.Since(tlsStart).Microseconds()) / 1000
			if err != nil || len(cs.PeerCertificates) == 0 {
				return
			}
			tlsProto = tls.VersionName(cs.Version)
			c := cs.PeerCertificates[0]
			certSubject = c.Subject.CommonName
			certIssuer = c.Issuer.CommonName
			certExpires = c.NotAfter.Format("2006-01-02")
			certDays = int(time.Until(c.NotAfter).Hours() / 24)
		},
		GotFirstResponseByte: func() { ttfbMs = float64(time.Since(start).Microseconds()) / 1000 },
	}

	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return http.ErrUseLastResponse // cortamos: la cadena ya es larga
			}
			// La respuesta que provocó la redirección viaja en req.Response
			// (los requests de via no la traen): el status es el 3xx anterior.
			status := 0
			if req.Response != nil {
				status = req.Response.StatusCode
			}
			redirects = append(redirects, map[string]any{
				"status": status,
				"url":    req.URL.String(),
			})
			return nil
		},
	}

	req, err := http.NewRequest(method, urlStr, strings.NewReader(in.Body))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "URL no válida: "+err.Error())
		return
	}
	hasContentType := false
	for k, v := range in.Headers {
		req.Header.Set(k, v)
		if strings.EqualFold(k, "Content-Type") {
			hasContentType = true
		}
	}
	if in.Body != "" && !hasContentType {
		req.Header.Set("Content-Type", "application/json")
	}
	if method == http.MethodGet || method == http.MethodHead {
		req.Body = nil
	}
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))

	resp, err := client.Do(req)
	totalMs := float64(time.Since(start).Microseconds()) / 1000
	if err != nil {
		// fallo de red: devolvemos lo que haya (cadena de redirecciones, tiempos)
		writeOK(w, map[string]any{
			"error":     err.Error(),
			"url":       urlStr,
			"method":    method,
			"total_ms":  round2(totalMs),
			"dns_ms":    round2(dnsMs),
			"connect_ms": round2(connMs),
			"tls_ms":    round2(tlsMs),
			"ttfb_ms":   round2(ttfbMs),
			"redirects": redirects,
		})
		return
	}
	defer resp.Body.Close()

	// cuerpo: leemos hasta 64 KB y mostramos una vista previa de 4 KB
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	preview := string(raw)
	if len(preview) > 4000 {
		preview = preview[:4000]
	}

	headers := make(map[string]string, len(resp.Header))
	for k, v := range resp.Header {
		headers[k] = strings.Join(v, ", ")
	}

	writeOK(w, map[string]any{
		"url":         urlStr,
		"final_url":   resp.Request.URL.String(),
		"method":      method,
		"status":      resp.StatusCode,
		"status_text": resp.Status,
		"total_ms":    round2(totalMs),
		"dns_ms":      round2(dnsMs),
		"connect_ms":  round2(connMs),
		"tls_ms":      round2(tlsMs),
		"ttfb_ms":     round2(ttfbMs),
		"redirects":   redirects,
		"headers":     headers,
		"body_preview": preview,
		"body_len":    len(raw),
		"tls_proto":   tlsProto,
		"cert": map[string]any{
			"subject": certSubject, "issuer": certIssuer,
			"expires": certExpires, "days_left": certDays,
		},
	})
}
