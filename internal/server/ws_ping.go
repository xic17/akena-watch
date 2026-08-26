package server

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"akena-watch/internal/ping"
)

// handlePingWS sirve la herramienta de ping en tiempo real.
//
// El cliente envía {"type":"start", ...} y {"type":"stop"}, y recibe:
//   - {"type":"result","seq":N,"ok":bool,"latency_ms":N,"error":"..."} por paquete
//   - {"type":"done","stats":{...}} al terminar (count agotado o stop)
func (s *Server) handlePingWS(w http.ResponseWriter, r *http.Request) {
	u, err := s.currentUser(r)
	if err != nil {
		if tok := r.URL.Query().Get("token"); tok != "" {
			u, err = s.st.UserForSession(tok)
		}
	}
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "sesión inválida")
		return
	}
	_ = u // cualquier sesión válida puede usar la herramienta

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	var wmu sync.Mutex
	send := func(v any) {
		wmu.Lock()
		defer wmu.Unlock()
		_ = conn.WriteJSON(v)
	}

	var (
		runMu  sync.Mutex
		cancel context.CancelFunc
		sessID int
	)

	start := func(cfg ping.Config) {
		runMu.Lock()
		if cancel != nil {
			cancel()
		}
		ctx, c := context.WithCancel(context.Background())
		cancel = c
		sessID++
		myID := sessID
		runMu.Unlock()

		go func() {
			sent, recv, minLat, maxLat, sum := 0, 0, 0, 0, 0
			cfg.Run(ctx, func(res ping.Result) {
				sent++
				if res.OK {
					recv++
					sum += res.LatencyMS
					if minLat == 0 || res.LatencyMS < minLat {
						minLat = res.LatencyMS
					}
					if res.LatencyMS > maxLat {
						maxLat = res.LatencyMS
					}
				}
				send(map[string]any{
					"type": "result", "seq": res.Seq, "ok": res.OK,
					"latency_ms": res.LatencyMS, "error": res.Error,
				})
			})

			// solo la sesión más reciente envía el resumen final
			runMu.Lock()
			current := myID == sessID
			runMu.Unlock()
			if !current {
				return
			}

			loss := 0
			if sent > 0 {
				loss = (sent - recv) * 100 / sent
			}
			stats := map[string]any{
				"sent": sent, "received": recv, "lost": sent - recv, "loss_pct": loss,
			}
			if recv > 0 {
				stats["min"] = minLat
				stats["avg"] = sum / recv
				stats["max"] = maxLat
			}
			send(map[string]any{"type": "done", "stats": stats})
		}()
	}

	stop := func() {
		runMu.Lock()
		defer runMu.Unlock()
		if cancel != nil {
			cancel()
			cancel = nil
		}
	}

	for {
		var msg map[string]any
		if err := conn.ReadJSON(&msg); err != nil {
			return
		}
		switch msg["type"] {
		case "start":
			cfg, err := parsePingConfig(msg)
			if err != nil {
				send(map[string]any{"type": "error", "error": err.Error()})
				continue
			}
			start(cfg)
		case "stop":
			stop()
		}
	}
}

func numVal(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	}
	return 0, false
}

func parsePingConfig(msg map[string]any) (ping.Config, error) {
	host, _ := msg["host"].(string)
	host = strings.TrimSpace(host)
	// tolerar URLs pegadas: quedarse con el host
	if u, err := url.Parse(host); err == nil && u.Host != "" && (u.Scheme == "http" || u.Scheme == "https") {
		host = u.Host
	}
	if host == "" || len(host) > 253 {
		return ping.Config{}, errors.New("indica un host válido (nombre o IP)")
	}

	proto, _ := msg["proto"].(string)
	if proto != ping.ProtoICMP {
		proto = ping.ProtoTCP
	}
	port := 443
	if proto == ping.ProtoTCP {
		if p, ok := numVal(msg["port"]); ok && p >= 1 && p <= 65535 {
			port = p
		}
	}
	interval := time.Second
	if ms, ok := numVal(msg["interval_ms"]); ok && ms >= 200 && ms <= 10000 {
		interval = time.Duration(ms) * time.Millisecond
	}
	count := 0
	if c, ok := numVal(msg["count"]); ok && c >= 0 && c <= 10000 {
		count = c
	}
	return ping.Config{Host: host, Proto: proto, Port: port, Interval: interval, Count: count}, nil
}
