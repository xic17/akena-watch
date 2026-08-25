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
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"akena-watch/internal/monitor"
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
	sched := monitor.NewScheduler(st, hub, notify)
	sched.Start()
	defer sched.Stop()

	srv := server.New(st, hub, notify, version)
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
