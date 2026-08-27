package server

import (
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// maxScanPorts limita cuántos puertos se pueden escanear por petición.
const maxScanPorts = 1024

// commonPorts es la lista por defecto y el perfil "comunes" del frontend.
var commonPorts = []int{21, 22, 25, 53, 80, 110, 143, 443, 465, 587, 993, 995,
	3000, 3306, 3389, 5432, 5900, 6379, 8080, 8443, 9200, 11211, 27017}

// portServices asocia nombres a puertos conocidos para el resultado.
var portServices = map[int]string{
	21: "ftp", 22: "ssh", 23: "telnet", 25: "smtp", 53: "dns", 80: "http",
	110: "pop3", 143: "imap", 443: "https", 465: "smtps", 587: "submission",
	993: "imaps", 995: "pop3s", 3000: "web dev", 3306: "mysql", 3389: "rdp",
	5432: "postgresql", 5900: "vnc", 6379: "redis", 8080: "http-alt",
	8443: "https-alt", 9200: "elasticsearch", 11211: "memcached", 27017: "mongodb",
}

// handlePortScan comprueba qué puertos TCP de un host aceptan conexión.
// Escaneo por conexión (no SYN): sin privilegios, seguro para la red ajena.
func (s *Server) handlePortScan(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Host       string `json:"host"`
		Ports      []int  `json:"ports"`
		RangeStart int    `json:"range_start"`
		RangeEnd   int    `json:"range_end"`
	}
	if err := readJSON(w, r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "solicitud inválida")
		return
	}
	host := strings.TrimSpace(in.Host)
	if host == "" || len(host) > 253 {
		writeErr(w, http.StatusBadRequest, "indica un host válido (nombre o IP)")
		return
	}

	// construye la lista de puertos: rango, lista explícita o por defecto
	seen := map[int]bool{}
	var ports []int
	add := func(p int) {
		if p >= 1 && p <= 65535 && !seen[p] {
			seen[p] = true
			ports = append(ports, p)
		}
	}
	if in.RangeEnd > 0 {
		start, end := in.RangeStart, in.RangeEnd
		if start < 1 {
			start = 1
		}
		if end > 65535 {
			end = 65535
		}
		if end < start {
			writeErr(w, http.StatusBadRequest, "el rango no es válido (hasta menor que desde)")
			return
		}
		for p := start; p <= end; p++ {
			add(p)
		}
	}
	for _, p := range in.Ports {
		add(p)
	}
	if len(ports) == 0 {
		for _, p := range commonPorts {
			add(p)
		}
	}
	if len(ports) > maxScanPorts {
		writeErr(w, http.StatusBadRequest, "demasiados puertos: máximo "+strconv.Itoa(maxScanPorts)+" por escaneo")
		return
	}

	// resuelve una sola vez (prefiere IPv4) para no consultar DNS por puerto
	ip := net.ParseIP(host)
	if ip == nil {
		ips, err := net.LookupIP(host)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "resolución DNS: "+err.Error())
			return
		}
		for _, cand := range ips {
			if cand.To4() != nil {
				ip = cand.To4()
				break
			}
		}
		if ip == nil {
			ip = ips[0]
		}
	}

	var (
		mu      sync.Mutex
		open    = make([]map[string]any, 0) // nunca nil: JSON [] en vez de null
		startAt = time.Now()
		sem     = make(chan struct{}, 50)
		wg      sync.WaitGroup
	)
	for _, p := range ports {
		wg.Add(1)
		go func(port int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			addr := net.JoinHostPort(ip.String(), strconv.Itoa(port))
			t0 := time.Now()
			conn, err := net.DialTimeout("tcp", addr, 700*time.Millisecond)
			lat := int(time.Since(t0).Milliseconds())
			if err != nil {
				return
			}
			conn.Close()
			mu.Lock()
			open = append(open, map[string]any{
				"port":       port,
				"service":    portServices[port],
				"latency_ms": lat,
			})
			mu.Unlock()
		}(p)
	}
	wg.Wait()
	sort.Slice(open, func(i, j int) bool {
		return open[i]["port"].(int) < open[j]["port"].(int)
	})

	writeOK(w, map[string]any{
		"host":       host,
		"total":      len(ports),
		"open":       open,
		"elapsed_ms": int(time.Since(startAt).Milliseconds()),
	})
}
