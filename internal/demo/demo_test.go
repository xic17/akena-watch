package demo

import (
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"akena-watch/internal/store"
)

// TestReiniciar comprueba que la demo vuelve a un estado limpio y sembrado:
// un solo administrador, los monitores de ejemplo y ninguna sesión antigua.
func TestReiniciar(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "demo.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	// Estado previo: un colaborador con monitor, canal y sesión.
	viejo, err := st.CreateUser("viejo", "hash", store.RoleCollaborator, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateMonitor(store.Monitor{
		OwnerID: viejo.ID, Name: "antes", Type: store.TypeHTTP, URL: "https://ejemplo.com",
		Method: "GET", ExpectedStatus: 200, TimeoutS: 5, IntervalS: 60,
		Active: true, Notify: true, MaxRetries: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateSession(viejo.ID, "sesion-vieja", time.Hour); err != nil {
		t.Fatal(err)
	}

	cfg := Config{Cada: time.Minute, Usuario: "akena", Clave: "Demo2026!Secure"}
	if err := Reiniciar(st, nil, cfg); err != nil {
		t.Fatalf("Reiniciar: %v", err)
	}

	if n, _ := st.CountUsers(); n != 1 {
		t.Fatalf("usuarios = %d, esperado 1", n)
	}
	u, err := st.GetUserByUsername("akena")
	if err != nil {
		t.Fatalf("no se creó el administrador de la demo: %v", err)
	}
	if u.Role != store.RoleAdmin {
		t.Fatalf("rol = %q, esperado admin", u.Role)
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(cfg.Clave)) != nil {
		t.Fatal("la contraseña de la demo no coincide")
	}

	mons, err := st.ListMonitorsForUser(u.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(mons) != 3 {
		t.Fatalf("monitores = %d, esperado 3", len(mons))
	}
	for _, m := range mons {
		if m.Notify {
			t.Fatalf("el monitor %q no debe enviar alertas en la demo", m.Name)
		}
		if m.OwnerID != u.ID {
			t.Fatalf("monitor %q con propietario %d, esperado %d", m.Name, m.OwnerID, u.ID)
		}
	}

	// La sesión anterior no puede sobrevivir al reinicio.
	if _, err := st.UserForSession("sesion-vieja"); err == nil {
		t.Fatal("la sesión antigua debería haber sido borrada")
	}

	// Un segundo reinicio no duplica nada.
	if err := Reiniciar(st, nil, cfg); err != nil {
		t.Fatalf("segundo Reiniciar: %v", err)
	}
	if n, _ := st.CountUsers(); n != 1 {
		t.Fatalf("tras el segundo reinicio, usuarios = %d, esperado 1", n)
	}
}
