package store

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestMonitorDomainExpiry(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "domain.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	u, err := s.CreateUser("akena", "hash", RoleAdmin, "", "")
	if err != nil {
		t.Fatal(err)
	}
	created, err := s.CreateMonitor(Monitor{
		OwnerID: u.ID, Name: "Web", Type: TypeHTTP, URL: "https://ejemplo.com",
		Method: "GET", ExpectedStatus: 200, TimeoutS: 5, IntervalS: 60,
		Active: true, Notify: true, MaxRetries: 1, DomainAlertDays: 30,
	})
	if err != nil {
		t.Fatalf("CreateMonitor: %v", err)
	}
	if created.DomainAlertDays != 30 {
		t.Fatalf("domain_alert_days = %d, esperado 30", created.DomainAlertDays)
	}

	// La última fecha conocida la escribe el planificador, no el formulario.
	vence := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	if err := s.SetDomainExpiry(created.ID, vence, "Ejemplo Registrar"); err != nil {
		t.Fatalf("SetDomainExpiry: %v", err)
	}
	got, err := s.GetMonitor(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.DomainExpiresAt.Equal(vence) || got.DomainRegistrar != "Ejemplo Registrar" {
		t.Fatalf("vencimiento = %v / %q, esperado %v / %q",
			got.DomainExpiresAt, got.DomainRegistrar, vence, "Ejemplo Registrar")
	}

	// Editar el monitor no puede perder el dato ya consultado.
	got.Name = "Web 2"
	got.DomainAlertDays = 15
	if err := s.UpdateMonitor(got); err != nil {
		t.Fatalf("UpdateMonitor: %v", err)
	}
	got2, err := s.GetMonitor(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got2.DomainExpiresAt.Equal(vence) || got2.DomainRegistrar != "Ejemplo Registrar" {
		t.Fatalf("editar perdió el vencimiento: %v / %q", got2.DomainExpiresAt, got2.DomainRegistrar)
	}
	if got2.DomainAlertDays != 15 || got2.Name != "Web 2" {
		t.Fatalf("la edición no se guardó: %+v", got2)
	}
}

func TestMonitorCRUD(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	u, err := s.CreateUser("akena", "hash", RoleAdmin, "", "")
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

	u, err := s.CreateUser("akena", "hash", RoleAdmin, "", "")
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

// TestMigrateUserEmail verifica que una base con el esquema anterior
// (usuarios sin correo) se migra y que el correo es único.
func TestMigrateUserEmail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	// esquema viejo: users sin la columna email
	if _, err := db.Exec(`CREATE TABLE users (
		id INTEGER PRIMARY KEY AUTOINCREMENT, username TEXT NOT NULL UNIQUE COLLATE NOCASE,
		password_hash TEXT NOT NULL, role TEXT NOT NULL DEFAULT 'collaborator',
		status_title TEXT NOT NULL DEFAULT 'Estado de los servicios',
		status_desc TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL)`); err != nil {
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

	u, err := s.CreateUser("akena", "hash", RoleAdmin, "akena@ejemplo.com", "123456789")
	if err != nil {
		t.Fatalf("crear usuario con correo tras migración: %v", err)
	}
	got, err := s.GetUserByID(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Email != "akena@ejemplo.com" || got.TelegramID != "123456789" {
		t.Fatalf("email=%q telegram=%q, esperado akena@ejemplo.com / 123456789", got.Email, got.TelegramID)
	}

	// correo duplicado → ErrEmailTaken
	if _, err := s.CreateUser("otro", "hash", RoleCollaborator, "akena@ejemplo.com", ""); err != ErrEmailTaken {
		t.Fatalf("esperaba ErrEmailTaken, got %v", err)
	}
	// correos vacíos no colisionan
	if _, err := s.CreateUser("sin-correo", "hash", RoleCollaborator, "", ""); err != nil {
		t.Fatalf("usuario sin correo: %v", err)
	}

	// perfil auto-servicio
	if err := s.UpdateProfile(u.ID, "akena.nueva@ejemplo.com", "-1001234567890"); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	got2, _ := s.GetUserByID(u.ID)
	if got2.Email != "akena.nueva@ejemplo.com" || got2.TelegramID != "-1001234567890" {
		t.Fatalf("perfil tras UpdateProfile: %+v", got2)
	}
}

// TestGroupsVisibility verifica el acceso de un colaborador por grupos
// (solo vista) y por monitores asignados manualmente.
func TestGroupsVisibility(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	owner, err := s.CreateUser("owner", "hash", RoleCollaborator, "", "")
	if err != nil {
		t.Fatal(err)
	}
	viewer, err := s.CreateUser("viewer", "hash", RoleCollaborator, "", "")
	if err != nil {
		t.Fatal(err)
	}

	mk := func(name, group string) Monitor {
		m, err := s.CreateMonitor(Monitor{
			OwnerID: owner.ID, Name: name, Group: group, Type: TypeHTTP, URL: "https://x.ejemplo.com",
			Active: true, Notify: true, MaxRetries: 1, TimeoutS: 5, IntervalS: 60,
		})
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	m1 := mk("Web", "web")
	m2 := mk("DB", "db")

	// sin permisos no ve nada
	if can, _ := s.CanViewMonitor(viewer.ID, m1.ID, false); can {
		t.Fatal("no debería ver m1 sin permisos")
	}
	if list, _ := s.ListMonitorsForUser(viewer.ID, false); len(list) != 0 {
		t.Fatalf("lista inicial = %d, esperado 0", len(list))
	}

	// acceso por grupo: solo vista
	if err := s.SetUserGroups(viewer.ID, []string{"web"}); err != nil {
		t.Fatal(err)
	}
	if can, _ := s.CanViewMonitor(viewer.ID, m1.ID, false); !can {
		t.Fatal("debería ver m1 por grupo")
	}
	if can, _ := s.CanViewMonitor(viewer.ID, m2.ID, false); can {
		t.Fatal("no debería ver m2")
	}
	if canEdit, _ := s.CanEditMonitor(viewer.ID, m1.ID, false); canEdit {
		t.Fatal("el acceso por grupo solo da vista")
	}

	// acceso manual a m2
	if err := s.SetUserManualAccess(viewer.ID, []int64{m2.ID}); err != nil {
		t.Fatal(err)
	}
	if can, _ := s.CanViewMonitor(viewer.ID, m2.ID, false); !can {
		t.Fatal("debería ver m2 por acceso manual")
	}
	if list, _ := s.ListMonitorsForUser(viewer.ID, false); len(list) != 2 {
		t.Fatalf("lista = %d, esperado 2", len(list))
	}

	// el colaborador con grupo recibe eventos en tiempo real
	ids, err := s.ListMonitorViewerIDs(m1.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, id := range ids {
		if id == viewer.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("viewer debería aparecer en ListMonitorViewerIDs")
	}

	// reemplazar grupos quita el acceso
	if err := s.SetUserGroups(viewer.ID, []string{}); err != nil {
		t.Fatal(err)
	}
	if can, _ := s.CanViewMonitor(viewer.ID, m1.ID, false); can {
		t.Fatal("tras quitar el grupo no debería ver m1")
	}
}
