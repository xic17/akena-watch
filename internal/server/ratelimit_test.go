package server

import (
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"akena-watch/internal/notifier"
	"akena-watch/internal/store"
)

// --- unidad del limitador ---

func TestLoginLimiterBloqueaYReinicia(t *testing.T) {
	l := newLoginLimiter(LoginLimit{Intentos: 3, Bloqueo: time.Minute, Memoria: time.Minute})

	if d := l.espera("akena", "1.2.3.4"); d != 0 {
		t.Fatalf("sin fallos no debe haber espera, got %v", d)
	}
	for i := 0; i < 2; i++ {
		if d := l.fallo("akena", "1.2.3.4"); d != 0 {
			t.Fatalf("fallo %d no debería bloquear todavía, got %v", i+1, d)
		}
	}
	if d := l.fallo("akena", "1.2.3.4"); d == 0 {
		t.Fatal("el tercer fallo debe bloquear")
	}
	if d := l.espera("akena", "1.2.3.4"); d <= 0 {
		t.Fatal("tras agotar los intentos debe haber espera")
	}

	l.exito("akena", "1.2.3.4")
	if d := l.espera("akena", "1.2.3.4"); d != 0 {
		t.Fatalf("un acierto debe borrar el historial, got %v", d)
	}
}

func TestLoginLimiterCreceConReintentos(t *testing.T) {
	l := newLoginLimiter(LoginLimit{Intentos: 1, Bloqueo: 100 * time.Millisecond, Memoria: time.Minute})

	primero := l.fallo("akena", "1.2.3.4")
	segundo := l.fallo("akena", "1.2.3.4")
	if segundo <= primero {
		t.Fatalf("los reintentos deben alargar el bloqueo: %v -> %v", primero, segundo)
	}
	if segundo > bloqueoMaximo {
		t.Fatalf("el bloqueo no debe pasar del tope: %v", segundo)
	}
}

func TestLoginLimiterNoDistingueMayusculas(t *testing.T) {
	l := newLoginLimiter(LoginLimit{Intentos: 2, Bloqueo: time.Minute, Memoria: time.Minute})

	l.fallo("Akena", "1.2.3.4")
	l.fallo("akena", "1.2.3.4")
	if d := l.espera("AKENA", "1.2.3.4"); d <= 0 {
		t.Fatal("\"Akena\", \"akena\" y \"AKENA\" son la misma cuenta y comparten contador")
	}
}

func TestLoginLimiterCuentaPorIP(t *testing.T) {
	l := newLoginLimiter(LoginLimit{Intentos: 2, Bloqueo: time.Minute, Memoria: time.Minute})

	l.fallo("uno", "1.2.3.4")
	l.fallo("dos", "1.2.3.4")
	if d := l.espera("tres", "1.2.3.4"); d <= 0 {
		t.Fatal("el rocío de contraseñas desde una misma IP debe frenarse")
	}
	if d := l.espera("tres", "5.6.7.8"); d != 0 {
		t.Fatal("otra IP no debe verse afectada")
	}
}

func TestClientIP(t *testing.T) {
	casos := []struct {
		nombre     string
		remoteAddr string
		headers    map[string]string
		esperado   string
	}{
		{"socket directo", "203.0.113.9:5555", nil, "203.0.113.9"},
		{"proxy local usa X-Real-IP", "127.0.0.1:5555", map[string]string{"X-Real-IP": "203.0.113.9"}, "203.0.113.9"},
		{"proxy local usa el último salto de XFF", "127.0.0.1:5555", map[string]string{"X-Forwarded-For": "1.1.1.1, 203.0.113.9"}, "203.0.113.9"},
		{"sin cabeceras detrás del proxy", "127.0.0.1:5555", nil, "127.0.0.1"},
		{"cabecera falsificada no se usa fuera de loopback", "203.0.113.9:5555", map[string]string{"X-Real-IP": "10.0.0.1"}, "203.0.113.9"},
		{"cabecera inválida se ignora", "127.0.0.1:5555", map[string]string{"X-Real-IP": "no-es-una-ip"}, "127.0.0.1"},
		{"loopback en la cabecera se ignora", "127.0.0.1:5555", map[string]string{"X-Real-IP": "127.0.0.1"}, "127.0.0.1"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/api/login", nil)
			r.RemoteAddr = c.remoteAddr
			for k, v := range c.headers {
				r.Header.Set(k, v)
			}
			if got := clientIP(r); got != c.esperado {
				t.Fatalf("clientIP = %q, esperado %q", got, c.esperado)
			}
		})
	}
}

// --- integración con el endpoint de acceso ---

func servidorDePrueba(t *testing.T, cfg LoginLimit) (*httptest.Server, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "login.db"))
	if err != nil {
		t.Fatal(err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("clave-correcta"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateUser("akena", string(hash), store.RoleAdmin, "", ""); err != nil {
		t.Fatal(err)
	}
	srv := New(st, NewHub(), notifier.NewManager(st), "test")
	srv.limite = newLoginLimiter(cfg)
	ts := httptest.NewServer(srv.Handler())
	return ts, st
}

func acceso(t *testing.T, ts *httptest.Server, usuario, clave string) *http.Response {
	t.Helper()
	cuerpo, _ := json.Marshal(map[string]string{"username": usuario, "password": clave})
	res, err := http.Post(ts.URL+"/api/login", "application/json", strings.NewReader(string(cuerpo)))
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestLoginBloqueaYSeRecupera(t *testing.T) {
	ts, st := servidorDePrueba(t, LoginLimit{Intentos: 3, Bloqueo: 200 * time.Millisecond, Memoria: time.Minute})
	defer ts.Close()
	defer st.Close()

	for i := 0; i < 3; i++ {
		res := acceso(t, ts, "akena", "clave-mala")
		res.Body.Close()
		if res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("fallo %d: código = %d, esperado 401", i+1, res.StatusCode)
		}
	}

	// Con los intentos agotados ni siquiera la contraseña correcta pasa.
	res := acceso(t, ts, "akena", "clave-correcta")
	defer res.Body.Close()
	if res.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("código = %d, esperado 429", res.StatusCode)
	}
	if res.Header.Get("Retry-After") == "" {
		t.Fatal("una respuesta 429 debe traer Retry-After")
	}
	var cuerpo map[string]any
	_ = json.NewDecoder(res.Body).Decode(&cuerpo)
	if cuerpo["error"] == nil || cuerpo["error"] == "" {
		t.Fatalf("se esperaba un mensaje claro, got %v", cuerpo)
	}

	// Pasado el bloqueo, el acceso correcto funciona.
	time.Sleep(250 * time.Millisecond)
	res2 := acceso(t, ts, "akena", "clave-correcta")
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusOK {
		t.Fatalf("tras el bloqueo el acceso correcto debe entrar, código = %d", res2.StatusCode)
	}
}

func TestLoginExitoReiniciaContador(t *testing.T) {
	ts, st := servidorDePrueba(t, LoginLimit{Intentos: 3, Bloqueo: time.Minute, Memoria: time.Minute})
	defer ts.Close()
	defer st.Close()

	// Un fallo se perdona tras un acceso correcto: el siguiente fallo no
	// arrastra el anterior.
	for vez := 0; vez < 2; vez++ {
		res := acceso(t, ts, "akena", "clave-mala")
		res.Body.Close()
		if res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("vuelta %d: fallo = %d, esperado 401", vez+1, res.StatusCode)
		}
		ok := acceso(t, ts, "akena", "clave-correcta")
		ok.Body.Close()
		if ok.StatusCode != http.StatusOK {
			t.Fatalf("vuelta %d: acceso correcto = %d, esperado 200", vez+1, ok.StatusCode)
		}
	}
}

func TestLoginFrenaRocioDeUsuarios(t *testing.T) {
	ts, st := servidorDePrueba(t, LoginLimit{Intentos: 2, Bloqueo: time.Minute, Memoria: time.Minute})
	defer ts.Close()
	defer st.Close()

	// Usuarios distintos desde la misma IP (httptest usa loopback) siguen
	// cayendo en el contador por IP.
	for _, usuario := range []string{"uno", "dos"} {
		res := acceso(t, ts, usuario, "clave-mala")
		res.Body.Close()
		if res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("usuario %q: código = %d, esperado 401", usuario, res.StatusCode)
		}
	}
	res := acceso(t, ts, "tres", "clave-mala")
	defer res.Body.Close()
	if res.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("código = %d, esperado 429", res.StatusCode)
	}
}

// TestLoginRegistraElFalloSinLaContrasena comprueba que el registro de un
// acceso fallido no deja la contraseña probada en el log.
func TestLoginRegistraElFalloSinLaContrasena(t *testing.T) {
	ts, st := servidorDePrueba(t, LoginLimit{Intentos: 5, Bloqueo: time.Minute, Memoria: time.Minute})
	defer ts.Close()
	defer st.Close()

	var buf strings.Builder
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	res := acceso(t, ts, "akena", "clave-secreta-que-no-debe-aparecer")
	res.Body.Close()

	lineas := buf.String()
	if !strings.Contains(lineas, "acceso fallido") {
		t.Fatalf("se esperaba una línea de fallo, got %q", lineas)
	}
	if strings.Contains(lineas, "clave-secreta-que-no-debe-aparecer") {
		t.Fatalf("la contraseña probada no debe acabar en el log: %q", lineas)
	}
}
