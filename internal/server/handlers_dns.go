package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type dnsRecord struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// handleDNSLookup consulta los registros DNS de un host desde el servidor
// (resolver del sistema). Tipos: A, AAAA, CNAME, MX, NS, TXT y PTR.
func (s *Server) handleDNSLookup(w http.ResponseWriter, r *http.Request) {
	host := strings.TrimSpace(r.URL.Query().Get("host"))
	typ := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("type")))
	if typ == "" {
		typ = "A"
	}
	host = hostFromURL(host)
	if host == "" || len(host) > 253 {
		writeErr(w, http.StatusBadRequest, "indica un host válido")
		return
	}
	if typ == "PTR" && net.ParseIP(host) == nil {
		writeErr(w, http.StatusBadRequest, "para PTR (inverso) indica una IP")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	start := time.Now()
	var records []dnsRecord
	var err error
	res := net.DefaultResolver

	switch typ {
	case "A", "AAAA":
		var ips []net.IP
		ips, err = res.LookupIP(ctx, "ip", host)
		if err == nil {
			for _, ip := range ips {
				if (typ == "A") == (ip.To4() != nil) {
					records = append(records, dnsRecord{Type: typ, Value: ip.String()})
				}
			}
		}
	case "CNAME":
		var cname string
		cname, err = res.LookupCNAME(ctx, host)
		if err == nil {
			records = append(records, dnsRecord{Type: "CNAME", Value: cname})
		}
	case "MX":
		var mxs []*net.MX
		mxs, err = res.LookupMX(ctx, host)
		if err == nil {
			for _, mx := range mxs {
				records = append(records, dnsRecord{Type: "MX", Value: fmt.Sprintf("%d %s", mx.Pref, mx.Host)})
			}
		}
	case "NS":
		var nss []*net.NS
		nss, err = res.LookupNS(ctx, host)
		if err == nil {
			for _, ns := range nss {
				records = append(records, dnsRecord{Type: "NS", Value: ns.Host})
			}
		}
	case "TXT":
		var txts []string
		txts, err = res.LookupTXT(ctx, host)
		if err == nil {
			for _, txt := range txts {
				records = append(records, dnsRecord{Type: "TXT", Value: txt})
			}
		}
	case "PTR":
		var names []string
		names, err = res.LookupAddr(ctx, host)
		if err == nil {
			for _, name := range names {
				records = append(records, dnsRecord{Type: "PTR", Value: name})
			}
		}
	default:
		writeErr(w, http.StatusBadRequest, "tipo no soportado (A, AAAA, CNAME, MX, NS, TXT, PTR)")
		return
	}

	elapsed := int(time.Since(start).Milliseconds())
	if err != nil {
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
			writeErr(w, http.StatusNotFound, "el host no se resolvió: "+dnsErr.Name)
			return
		}
		writeErr(w, http.StatusBadGateway, "falló la consulta DNS: "+err.Error())
		return
	}
	writeOK(w, map[string]any{"host": host, "type": typ, "records": records, "elapsed_ms": elapsed})
}

// hostFromURL tolera URLs pegadas (https://ejemplo.com/x → ejemplo.com).
func hostFromURL(s string) string {
	if !strings.Contains(s, "://") {
		return s
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return s
	}
	return u.Host
}
