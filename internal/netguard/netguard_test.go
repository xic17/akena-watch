package netguard

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPermitido(t *testing.T) {
	casos := []struct {
		ip       string
		estricto bool
		esperado bool
	}{
		// Nunca permitidas, en ningún modo.
		{"169.254.169.254", false, false}, // metadatos del cloud
		{"169.254.1.1", false, false},     // link-local
		{"fe80::1", false, false},
		{"0.0.0.0", false, false},
		{"224.0.0.1", false, false},
		{"::", false, false},
		// Privadas y loopback: permitidas salvo en modo estricto.
		{"127.0.0.1", false, true},
		{"10.1.2.3", false, true},
		{"192.168.1.10", false, true},
		{"172.16.0.1", false, true},
		{"::1", false, true},
		{"fd00::1", false, true},
		{"127.0.0.1", true, false},
		{"10.1.2.3", true, false},
		{"192.168.1.10", true, false},
		{"172.16.0.1", true, false},
		{"100.64.0.1", true, false}, // CGNAT
		{"::1", true, false},
		{"fd00::1", true, false},
		// Públicas: siempre.
		{"1.1.1.1", true, true},
		{"93.184.216.34", true, true},
		{"2606:4700:4700::1111", true, true},
	}
	for _, c := range casos {
		t.Run(c.ip, func(t *testing.T) {
			Configurar(c.estricto)
			t.Cleanup(func() { Configurar(false) })
			if got := Permitido(net.ParseIP(c.ip)); got != c.esperado {
				t.Fatalf("Permitido(%s) en estricto=%v = %v, esperado %v", c.ip, c.estricto, got, c.esperado)
			}
		})
	}
}

func TestComprobarRechazaInternos(t *testing.T) {
	Configurar(true)
	defer Configurar(false)

	for _, host := range []string{"127.0.0.1", "localhost:8080", "192.168.1.1:443", "169.254.169.254"} {
		if err := Comprobar(host); !errors.Is(err, ErrBloqueado) {
			t.Fatalf("Comprobar(%q) = %v, esperado ErrBloqueado", host, err)
		}
	}
	// Un nombre que no resuelve no se rechaza aquí: fallará al conectar.
	if err := Comprobar("no-existe.invalid"); err != nil {
		t.Fatalf("Comprobar(no-existe.invalid) = %v, esperado nil", err)
	}
}

// TestClienteBloqueaEnEstricto comprueba la defensa real: aunque la URL sea
// loopback, el transporte no llega a conectar en modo estricto.
func TestClienteBloqueaEnEstricto(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	Configurar(false)
	res, err := Cliente(2*time.Second, nil).Get(ts.URL)
	if err != nil {
		t.Fatalf("sin modo estricto debería conectar: %v", err)
	}
	res.Body.Close()

	Configurar(true)
	defer Configurar(false)
	if _, err := Cliente(2*time.Second, nil).Get(ts.URL); !errors.Is(err, ErrBloqueado) {
		t.Fatalf("en modo estricto debería bloquearse, got %v", err)
	}
}

func TestDialerBloqueaEnEstricto(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	Configurar(true)
	defer Configurar(false)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := Dialer(time.Second).DialContext(ctx, "tcp", ln.Addr().String()); !errors.Is(err, ErrBloqueado) {
		t.Fatalf("el dialer debería bloquear loopback, got %v", err)
	}
}
