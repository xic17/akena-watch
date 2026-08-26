package store

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestMonitorCRUD(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	u, err := s.CreateUser("akena", "hash", RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}

	m := Monitor{
		OwnerID: u.ID, Name: "Test", Type: TypeHTTP, URL: "https://example.com",
		Method: "GET", ExpectedStatus: 200, TimeoutS: 5, IntervalS: 60,
		Active: true, Notify: true, MaxRetries: 1,
	}
	created, err := s.CreateMonitor(m)
	if err != nil {
		t.Fatalf("CreateMonitor: %v", err)
	}

	got, err := s.GetMonitor(created.ID)
	if err != nil {
		t.Fatalf("GetMonitor: %v", err)
	}
	if got.Name != "Test" || !got.Active || !got.Notify {
		t.Fatalf("monitor inesperado: %+v", got)
	}

	if err := s.InsertHeartbeat(Heartbeat{
		MonitorID: created.ID, Status: StatusUp, LatencyMS: 42, CheckedAt: time.Now(),
	}); err != nil {
		t.Fatalf("InsertHeartbeat: %v", err)
	}
	up, total, err := s.Uptime(created.ID, time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("Uptime: %v", err)
	}
	if up != 1 || total != 1 {
		t.Fatalf("uptime = %d/%d, esperado 1/1", up, total)
	}

	viewers, err := s.ListMonitorViewerIDs(created.ID)
	if err != nil {
		t.Fatalf("ListMonitorViewerIDs: %v", err)
	}
	if len(viewers) != 1 || viewers[0] != u.ID {
		t.Fatalf("viewers = %v, esperado [%d]", viewers, u.ID)
	}
}

// TestMigrateBodyColumn verifica que una base de datos creada con el
// esquema anterior (sin la columna body) se migra correctamente al abrirla.
func TestMigrateBodyColumn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	// esquema viejo: tabla monitors sin la columna body
	if _, err := db.Exec(`CREATE TABLE monitors (
		id INTEGER PRIMARY KEY AUTOINCREMENT, owner_id INTEGER NOT NULL, name TEXT NOT NULL,
		type TEXT NOT NULL, url TEXT NOT NULL, method TEXT NOT NULL DEFAULT 'GET',
		expected_status INTEGER NOT NULL DEFAULT 200, keyword TEXT NOT NULL DEFAULT '',
		invert_keyword INTEGER NOT NULL DEFAULT 0, timeout_s INTEGER NOT NULL DEFAULT 10,
		interval_s INTEGER NOT NULL DEFAULT 60, active INTEGER NOT NULL DEFAULT 1,
		public INTEGER NOT NULL DEFAULT 0, notify INTEGER NOT NULL DEFAULT 1,
		max_retries INTEGER NOT NULL DEFAULT 1, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := Open(path) // dispara la migración
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	u, err := s.CreateUser("akena", "hash", RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	m, err := s.CreateMonitor(Monitor{
		OwnerID: u.ID, Name: "API", Type: TypeHTTP, URL: "https://api.ejemplo.com",
		Method: "POST", Body: `{"query":"status"}`, TimeoutS: 5, IntervalS: 60,
		Active: true, Notify: true, MaxRetries: 1,
	})
	if err != nil {
		t.Fatalf("CreateMonitor tras migración: %v", err)
	}
	got, err := s.GetMonitor(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Body != `{"query":"status"}` {
		t.Fatalf("body = %q, esperado {\"query\":\"status\"}", got.Body)
	}
}
