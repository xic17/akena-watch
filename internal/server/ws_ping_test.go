package server

import (
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"akena-watch/internal/notifier"
	"akena-watch/internal/store"
)

// TestPingToolWS conecta un cliente WebSocket real, lanza una sesión de
// ping TCP contra un listener local y verifica resultados + resumen.
func TestPingToolWS(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	u, err := st.CreateUser("akena", "hash", store.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	token := "test-token"
	if err := st.CreateSession(u.ID, token, time.Hour); err != nil {
		t.Fatal(err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	srv := New(st, NewHub(), notifier.NewManager(st), "test")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws/ping"
	c, resp, err := websocket.DefaultDialer.Dial(wsURL, http.Header{"Cookie": []string{"akena_session=" + token}})
	if err != nil {
		t.Fatalf("dial: %v (http %d)", err, resp.StatusCode)
	}
	defer c.Close()

	if err := c.WriteJSON(map[string]any{
		"type": "start", "host": "127.0.0.1", "proto": "tcp",
		"port": port, "interval_ms": 200, "count": 2,
	}); err != nil {
		t.Fatal(err)
	}

	results, done := 0, 0
	deadline := time.After(8 * time.Second)
	for done == 0 {
		select {
		case <-deadline:
			t.Fatalf("timeout: results=%d done=%d", results, done)
		default:
		}
		var msg map[string]any
		if err := c.ReadJSON(&msg); err != nil {
			t.Fatalf("lectura WS: %v", err)
		}
		switch msg["type"] {
		case "result":
			results++
			if msg["ok"] != true {
				t.Fatalf("resultado esperado ok=true, got %v", msg)
			}
		case "done":
			done++
			stats, _ := msg["stats"].(map[string]any)
			if stats["received"] != float64(2) || stats["lost"] != float64(0) {
				t.Fatalf("stats = %v, esperado 2 recibidos y 0 perdidos", stats)
			}
		default:
			t.Fatalf("mensaje inesperado: %v", msg)
		}
	}
	if results != 2 {
		t.Fatalf("results = %d, esperado 2", results)
	}
}

// TestPingToolWSRejectsBadHost verifica que una configuración inválida
// devuelve un error por el socket en vez de romper la conexión.
func TestPingToolWSRejectsBadHost(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	u, err := st.CreateUser("akena", "hash", store.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CreateSession(u.ID, "test-token-2", time.Hour); err != nil {
		t.Fatal(err)
	}

	srv := New(st, NewHub(), notifier.NewManager(st), "test")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws/ping"
	c, _, err := websocket.DefaultDialer.Dial(wsURL, http.Header{"Cookie": []string{"akena_session=test-token-2"}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	if err := c.WriteJSON(map[string]any{"type": "start", "host": "   "}); err != nil {
		t.Fatal(err)
	}
	var msg map[string]any
	if err := c.ReadJSON(&msg); err != nil {
		t.Fatal(err)
	}
	if msg["type"] != "error" || msg["error"] == "" {
		t.Fatalf("se esperaba un error claro, got %v", msg)
	}
}
