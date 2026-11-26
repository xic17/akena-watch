// Akena Watch — Siempre en Guardia.
//
// Monitor de disponibilidad portable: un solo binario, SQLite embebida,
// sin dependencias de runtime. Corre en cualquier Linux, detrás de un
// panel como CloudPanel 2, o dentro de Cloudflare Containers.
//
// Configuración por variables de entorno:
//
//	AKENA_DATA_DIR  directorio donde vive akena.db (default: ./data)
//	AKENA_BIND      interfaz de escucha (default: 0.0.0.0)
//	AKENA_PORT      puerto HTTP (default: $PORT o 8080)
//	AKENA_LOGIN_INTENTOS     fallos de acceso antes de bloquear (default: 5)
//	AKENA_LOGIN_BLOQUEO_SEG  segundos de bloqueo inicial (default: 30)
//	AKENA_LOGIN_MEMORIA_MIN  minutos que se recuerda un fallo (default: 15)
//	AKENA_SOLO_PUBLICO       1 = no permitir destinos internos (loopback, rangos
//	                         privados ni metadatos del cloud) desde monitores ni
//	                         herramientas
//	AKENA_DEMO_MIN           reinicia los datos cada N minutos (0 = desactivado);
//	                         implica AKENA_SOLO_PUBLICO y silencia las alertas
//	AKENA_DEMO_USER / AKENA_DEMO_PASS  administrador de la demo
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"akena-watch/internal/demo"
	"akena-watch/internal/monitor"
	"akena-watch/internal/netguard"
	"akena-watch/internal/notifier"
	"akena-watch/internal/server"
	"akena-watch/internal/store"
)

// version se inyecta en tiempo de compilación con
// -ldflags "-X main.version=<versión>" (ver Makefile).
var version = "dev"

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[akena-watch] ")

	dataDir := envOr("AKENA_DATA_DIR", "./data")
	bind := envOr("AKENA_BIND", "0.0.0.0")
	port := envOr("AKENA_PORT", envOr("PORT", "8080"))
	demoMin := envInt("AKENA_DEMO_MIN", 0)
	soloPublico := demoMin > 0 || envOr("AKENA_SOLO_PUBLICO", "0") == "1"

	// La política de red se fija antes de arrancar el planificador: los primeros
	// checks ya deben cumplirla.
	netguard.Configurar(soloPublico)
	if soloPublico {
		log.Println("política de red estricta: solo se permiten destinos públicos")
	}
	if demoMin > 0 {
		log.Printf("modo demostración: los datos se reinician cada %d minutos", demoMin)
	}

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		log.Fatalf("no se pudo crear el directorio de datos %q: %v", dataDir, err)
	}

	dbPath := filepath.Join(dataDir, "akena.db")
	st, err := store.Open(dbPath)
	if err != nil {
		log.Fatalf("no se pudo abrir la base de datos: %v", err)
	}
	defer st.Close()

	hub := server.NewHub()
	notify := notifier.NewManager(st)
	if demoMin > 0 {
		// La demo no puede enviar alertas: nadie usa la infraestructura del
		// anfitrión para mandar mensajes a terceros.
		notify.Silenciar(true)
	}
	sched := monitor.NewScheduler(st, hub, notify)
	sched.Start()
	defer sched.Stop()

	srv := server.New(st, hub, notify, version)

	if demoMin > 0 {
		srv.Demo(demoMin)
		pararDemo, err := demo.Iniciar(st, sched, demo.Config{
			Cada:    time.Duration(demoMin) * time.Minute,
			Usuario: envOr("AKENA_DEMO_USER", "akena"),
			Clave:   envOr("AKENA_DEMO_PASS", "Demo2026!Secure"),
		})
		if err != nil {
			log.Fatalf("modo demo: %v", err)
		}
		defer pararDemo()
	}

	httpSrv := &http.Server{
		Addr:              bind + ":" + port,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("Akena Watch — Siempre en Guardia (versión %s)", version)
		log.Printf("escuchando en http://%s:%s (datos en %s)", bind, port, dbPath)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("servidor HTTP: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("deteniendo...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// envInt lee un entero no negativo; cualquier valor inválido deja el default.
func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return n
		}
	}
	return def
}
