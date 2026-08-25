// Package monitor implementa los checks de disponibilidad (HTTP, TCP, DNS)
// y el scheduler que los ejecuta periódicamente.
package monitor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"akena-watch/internal/store"
)

// Result es el resultado de un check.
type Result struct {
	Status    string // "up" | "down"
	Code      int    // código HTTP / 0
	LatencyMS int
	Error     string
}

// Check ejecuta el check apropiado según el tipo de monitor.
func Check(ctx context.Context, m store.Monitor) Result {
	switch m.Type {
	case store.TypeHTTP:
		return checkHTTP(ctx, m)
	case store.TypeTCP:
		return checkTCP(ctx, m)
	case store.TypeDNS:
		return checkDNS(ctx, m)
	default:
		return Result{Status: store.StatusDown, Error: "tipo de monitor desconocido: " + m.Type}
	}
}

func checkHTTP(ctx context.Context, m store.Monitor) Result {
	req, err := http.NewRequestWithContext(ctx, m.Method, m.URL, nil)
	if err != nil {
		return Result{Status: store.StatusDown, Error: "URL inválida: " + err.Error()}
	}
	req.Header.Set("User-Agent", "akena-watch/1.0 (Siempre en Guardia)")

	client := &http.Client{
		Timeout: time.Duration(m.TimeoutS) * time.Second,
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("demasiadas redirecciones")
			}
			return nil
		},
	}

	start := time.Now()
	resp, err := client.Do(req)
	latency := int(time.Since(start).Milliseconds())
	if err != nil {
		return Result{Status: store.StatusDown, LatencyMS: latency, Error: err.Error()}
	}
	defer resp.Body.Close()

	code := resp.StatusCode
	if m.Keyword != "" {
		body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
		if err != nil {
			return Result{Status: store.StatusDown, Code: code, LatencyMS: latency,
				Error: "error leyendo respuesta: " + err.Error()}
		}
		found := strings.Contains(string(body), m.Keyword)
		if m.InvertKeyword && found {
			return Result{Status: store.StatusDown, Code: code, LatencyMS: latency,
				Error: "se encontró la palabra clave (check invertido)"}
		}
		if !m.InvertKeyword && !found {
			return Result{Status: store.StatusDown, Code: code, LatencyMS: latency,
				Error: fmt.Sprintf("no se encontró la palabra clave %q", m.Keyword)}
		}
	}

	if code != m.ExpectedStatus {
		return Result{Status: store.StatusDown, Code: code, LatencyMS: latency,
			Error: fmt.Sprintf("estado HTTP %d, se esperaba %d", code, m.ExpectedStatus)}
	}
	return Result{Status: store.StatusUp, Code: code, LatencyMS: latency}
}

func checkTCP(ctx context.Context, m store.Monitor) Result {
	host := m.URL
	if u, err := url.Parse(m.URL); err == nil && u.Host != "" {
		host = u.Host
	}
	if _, _, err := net.SplitHostPort(host); err != nil {
		host = net.JoinHostPort(host, "80")
	}

	d := net.Dialer{Timeout: time.Duration(m.TimeoutS) * time.Second}
	start := time.Now()
	conn, err := d.DialContext(ctx, "tcp", host)
	latency := int(time.Since(start).Milliseconds())
	if err != nil {
		return Result{Status: store.StatusDown, LatencyMS: latency, Error: err.Error()}
	}
	conn.Close()
	return Result{Status: store.StatusUp, LatencyMS: latency}
}

func checkDNS(ctx context.Context, m store.Monitor) Result {
	host := m.URL
	if u, err := url.Parse(m.URL); err == nil && u.Host != "" {
		host = u.Host
	}
	host = strings.Trim(host, "/")

	resolver := &net.Resolver{}
	start := time.Now()
	addrs, err := resolver.LookupHost(ctx, host)
	latency := int(time.Since(start).Milliseconds())
	if err != nil {
		return Result{Status: store.StatusDown, LatencyMS: latency, Error: err.Error()}
	}
	if len(addrs) == 0 {
		return Result{Status: store.StatusDown, LatencyMS: latency, Error: "sin registros A/AAAA"}
	}
	return Result{Status: store.StatusUp, LatencyMS: latency}
}
