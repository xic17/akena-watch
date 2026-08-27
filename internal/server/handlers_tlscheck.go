package server

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/tls"
	"fmt"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// handleTLSCheck inspecciona el certificado TLS de un servidor: validez,
// emisor, SANs, serie, algoritmos y días restantes. Se conecta con
// InsecureSkipVerify a propósito: queremos ver el certificado aunque esté
// caducado o no coincida con el host (es una herramienta de diagnóstico).
func (s *Server) handleTLSCheck(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	}
	if err := readJSON(w, r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "solicitud inválida")
		return
	}
	host := strings.TrimSpace(in.Host)
	// tolerar URLs pegadas: quedarse con el host
	if u := strings.SplitN(host, "://", 2); len(u) == 2 {
		host = strings.TrimSuffix(strings.SplitN(u[1], "/", 2)[0], "/")
	}
	if host == "" || len(host) > 253 {
		writeErr(w, http.StatusBadRequest, "indica un host válido (nombre o IP)")
		return
	}
	if in.Port == 0 {
		in.Port = 443
	}
	if in.Port < 1 || in.Port > 65535 {
		writeErr(w, http.StatusBadRequest, "el puerto debe estar entre 1 y 65535")
		return
	}

	// SNI solo para nombres (no para IPs, como hace un cliente real).
	serverName := ""
	if net.ParseIP(host) == nil {
		serverName = host
	}

	start := time.Now()
	addr := net.JoinHostPort(host, strconv.Itoa(in.Port))
	raw, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		writeOK(w, map[string]any{
			"host": host, "port": in.Port, "success": false,
			"error": err.Error(), "handshake_ms": int(time.Since(start).Milliseconds()),
		})
		return
	}
	conn := tls.Client(raw, &tls.Config{InsecureSkipVerify: true, ServerName: serverName})
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second)) // acota conexión + handshake
	if err := conn.Handshake(); err != nil {
		raw.Close()
		writeOK(w, map[string]any{
			"host": host, "port": in.Port, "success": false,
			"error": err.Error(), "handshake_ms": int(time.Since(start).Milliseconds()),
		})
		return
	}
	defer conn.Close()

	state := conn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		writeOK(w, map[string]any{
			"host": host, "port": in.Port, "success": false,
			"error": "el servidor no presentó certificado", "handshake_ms": int(time.Since(start).Milliseconds()),
		})
		return
	}
	leaf := state.PeerCertificates[0]
	now := time.Now()
	days := int(math.Ceil(time.Until(leaf.NotAfter).Hours() / 24))

	writeOK(w, map[string]any{
		"host": host, "port": in.Port, "success": true,
		"handshake_ms": int(time.Since(start).Milliseconds()),
		"tls_proto":    tls.VersionName(state.Version),
		"cipher":       tls.CipherSuiteName(state.CipherSuite),
		"chain_len":    len(state.PeerCertificates),
		"cert": map[string]any{
			"subject":       leaf.Subject.CommonName,
			"sans":          leaf.DNSNames,
			"issuer":        leaf.Issuer.CommonName,
			"serial":        leaf.SerialNumber.Text(16),
			"not_before":    leaf.NotBefore.Format("2006-01-02"),
			"not_after":     leaf.NotAfter.Format("2006-01-02"),
			"days_left":     days,
			"expired":       now.After(leaf.NotAfter),
			"not_yet_valid": now.Before(leaf.NotBefore),
			"sig_alg":       leaf.SignatureAlgorithm.String(),
			"key":           keyDescription(leaf.PublicKey),
		},
	})
}

// keyDescription describe la clave pública del certificado.
func keyDescription(pub any) string {
	switch k := pub.(type) {
	case *rsa.PublicKey:
		return fmt.Sprintf("RSA %d bits", k.N.BitLen())
	case *ecdsa.PublicKey:
		return "ECDSA " + k.Curve.Params().Name
	case ed25519.PublicKey:
		return "Ed25519"
	default:
		return "desconocida"
	}
}
