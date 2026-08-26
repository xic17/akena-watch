package server

import (
	"encoding/json"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"time"

	"akena-watch/internal/notifier"
	"akena-watch/internal/store"
	"akena-watch/internal/web"
)

// Server agrupa las dependencias de la capa HTTP.
type Server struct {
	st      *store.Store
	hub     *Hub
	notify  *notifier.Manager
	version string
	pages   map[string]*template.Template
}

// New construye el servidor. Cada página vive en un clon del template
// base con su propio namespace, para que los bloques "content"/"title"
// de cada archivo no colisionen entre sí.
func New(st *store.Store, hub *Hub, notify *notifier.Manager, version string) *Server {
	base := template.Must(template.ParseFS(web.FS, "templates/base.html"))
	pages := make(map[string]*template.Template, 6)
	for _, name := range []string{
		"setup.html", "login.html", "dashboard.html", "tools.html", "users.html", "about.html", "status.html",
	} {
		pages[name] = template.Must(base.Clone())
		template.Must(pages[name].ParseFS(web.FS, "templates/"+name))
	}
	return &Server{st: st, hub: hub, notify: notify, version: version, pages: pages}
}

// Handler construye el mux con todas las rutas y la cadena de middlewares.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Páginas
	mux.HandleFunc("GET /", s.handleRoot)
	mux.HandleFunc("GET /setup", s.handleSetupPage)
	mux.HandleFunc("GET /login", s.handleLoginPage)
	mux.HandleFunc("GET /dashboard", s.authPage(s.handleDashboard))
	mux.HandleFunc("GET /tools", s.authPage(s.handleToolsPage))
	mux.HandleFunc("GET /users", s.authPage(s.adminPage(s.handleUsersPage)))
	mux.HandleFunc("GET /about", s.handleAboutPage)
	mux.HandleFunc("GET /status/{slug}", s.handleStatusPage)

	// API pública
	mux.HandleFunc("POST /api/setup", s.handleSetup)
	mux.HandleFunc("POST /api/login", s.handleLogin)
	mux.HandleFunc("POST /api/logout", s.handleLogout)
	mux.HandleFunc("GET /ping", s.handlePing)
	mux.HandleFunc("GET /api/version", s.handleVersion)
	mux.HandleFunc("GET /ws", s.handleWS)
	mux.HandleFunc("GET /ws/ping", s.handlePingWS)

	// API autenticada
	mux.HandleFunc("GET /api/me", s.authJSON(s.handleMe))
	mux.HandleFunc("GET /api/monitors", s.authJSON(s.handleListMonitors))
	mux.HandleFunc("POST /api/monitors", s.authJSON(s.handleCreateMonitor))
	mux.HandleFunc("GET /api/monitors/{id}", s.authJSON(s.handleGetMonitor))
	mux.HandleFunc("PUT /api/monitors/{id}", s.authJSON(s.handleUpdateMonitor))
	mux.HandleFunc("DELETE /api/monitors/{id}", s.authJSON(s.handleDeleteMonitor))
	mux.HandleFunc("POST /api/monitors/{id}/test", s.authJSON(s.handleTestMonitor))
	mux.HandleFunc("GET /api/monitors/{id}/heartbeats", s.authJSON(s.handleHeartbeats))
	mux.HandleFunc("GET /api/monitors/{id}/stats", s.authJSON(s.handleMonitorStats))
	mux.HandleFunc("GET /api/heartbeats", s.authJSON(s.handleAllHeartbeats))
	mux.HandleFunc("PUT /api/monitors/{id}/share/{uid}", s.authJSON(s.handleSetShare))
	mux.HandleFunc("DELETE /api/monitors/{id}/share/{uid}", s.authJSON(s.handleDeleteShare))
	mux.HandleFunc("GET /api/notifications", s.authJSON(s.handleListNotifications))
	mux.HandleFunc("POST /api/notifications", s.authJSON(s.handleCreateNotification))
	mux.HandleFunc("PUT /api/notifications/{id}", s.authJSON(s.handleUpdateNotification))
	mux.HandleFunc("DELETE /api/notifications/{id}", s.authJSON(s.handleDeleteNotification))
	mux.HandleFunc("POST /api/notifications/{id}/test", s.authJSON(s.handleTestNotification))
	mux.HandleFunc("GET /api/users", s.authJSON(s.handleListUsers))
	mux.HandleFunc("POST /api/users", s.authJSON(s.adminJSON(s.handleCreateUser)))
	mux.HandleFunc("PUT /api/users/{id}", s.authJSON(s.adminJSON(s.handleUpdateUser)))
	mux.HandleFunc("DELETE /api/users/{id}", s.authJSON(s.adminJSON(s.handleDeleteUser)))
	mux.HandleFunc("GET /api/statuspage", s.authJSON(s.handleGetStatusPage))
	mux.HandleFunc("PUT /api/statuspage", s.authJSON(s.handleUpdateStatusPage))
	mux.HandleFunc("GET /api/whois", s.authJSON(s.handleWhois))
	mux.HandleFunc("GET /api/dns", s.authJSON(s.handleDNSLookup))

	staticFS, err := fs.Sub(web.FS, "static")
	if err != nil {
		panic(err)
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	return s.csrf(s.logRequests(mux))
}

// --- Middlewares ---

// csrf rechaza peticiones de estado con un Origin de otro host.
func (s *Server) csrf(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			if origin := r.Header.Get("Origin"); origin != "" {
				u, err := url.Parse(origin)
				if err != nil || u.Host != r.Host {
					writeErr(w, http.StatusForbidden, "origen no permitido")
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ping" || len(r.URL.Path) >= 7 && r.URL.Path[:7] == "/static" {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s (%s)", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

// --- helpers JSON ---

func readJSON(w http.ResponseWriter, r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeOK(w http.ResponseWriter, data map[string]any) {
	data["ok"] = true
	writeJSON(w, http.StatusOK, data)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"ok": false, "error": msg})
}
