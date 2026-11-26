// Package demo mantiene limpia una instancia de demostración.
//
// Cada cierto tiempo borra los datos, vuelve a crear el administrador y los
// monitores de ejemplo, y olvida el estado del planificador. Así una demo
// pública (con la cuenta en modo administrador, para que se vea la herramienta
// completa) no acumula basura ni deja monitores ajenos corriendo.
package demo

import (
	"log"
	"time"

	"golang.org/x/crypto/bcrypt"

	"akena-watch/internal/monitor"
	"akena-watch/internal/store"
)

// Config define la demo.
type Config struct {
	Cada    time.Duration // cada cuánto se reinicia
	Usuario string        // administrador de la demo
	Clave   string        // contraseña de la demo
}

// Iniciar limpia la demo al arrancar y programa el siguiente reinicio. Devuelve
// la función que detiene el bucle (se llama al apagar).
func Iniciar(st *store.Store, sched *monitor.Scheduler, cfg Config) (func(), error) {
	if cfg.Cada <= 0 {
		cfg.Cada = 15 * time.Minute
	}
	if cfg.Usuario == "" {
		cfg.Usuario = "akena"
	}
	if err := Reiniciar(st, sched, cfg); err != nil {
		return nil, err
	}

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		t := time.NewTicker(cfg.Cada)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				if err := Reiniciar(st, sched, cfg); err != nil {
					log.Printf("demo: no se pudo reiniciar: %v", err)
				}
			}
		}
	}()
	return func() { close(stop); <-done }, nil
}

// Reiniciar deja la instancia como recién instalada y vuelve a sembrar la demo.
func Reiniciar(st *store.Store, sched *monitor.Scheduler, cfg Config) error {
	if err := st.ReiniciarTodo(); err != nil {
		return err
	}
	if sched != nil {
		// Los checks en vuelo pueden terminar, pero el estado viejo se descarta.
		sched.Reset()
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(cfg.Clave), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u, err := st.CreateUser(cfg.Usuario, string(hash), store.RoleAdmin, "", "")
	if err != nil {
		return err
	}
	monitores := semilla(u.ID)
	for _, m := range monitores {
		if _, err := st.CreateMonitor(m); err != nil {
			return err
		}
	}
	log.Printf("demo: datos reiniciados (usuario %q, %d monitores)", cfg.Usuario, len(monitores))
	return nil
}

// semilla son los monitores de ejemplo: destinos públicos y sin alertas, para
// que la demo no genere tráfico hacia terceros ni notificaciones.
func semilla(ownerID int64) []store.Monitor {
	return []store.Monitor{
		{
			OwnerID: ownerID, Name: "Sitio web", Group: "web",
			Type: store.TypeHTTP, URL: "https://akenawatch.com", Method: "GET",
			ExpectedStatus: 200, TimeoutS: 10, IntervalS: 60, MaxRetries: 1,
			Active: true, Public: true, Notify: false,
			DomainAlertDays: 365, // enseña el vencimiento del dominio en la ficha
		},
		{
			OwnerID: ownerID, Name: "DNS del dominio", Group: "infra",
			Type: store.TypeDNS, URL: "akenawatch.com",
			TimeoutS: 10, IntervalS: 120, MaxRetries: 1,
			Active: true, Public: true, Notify: false,
		},
		{
			OwnerID: ownerID, Name: "Página de ejemplo", Group: "web",
			Type: store.TypeHTTP, URL: "https://example.com", Method: "GET",
			ExpectedStatus: 200, TimeoutS: 10, IntervalS: 120, MaxRetries: 1,
			Active: true, Public: true, Notify: false,
		},
	}
}
