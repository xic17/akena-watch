package server

import (
	"log"
	"net/http"

	"akena-watch/internal/store"
)

// pageData es el contexto mínimo que reciben las plantillas.
type pageData struct {
	User        *store.User
	Setup       bool
	IsAdmin     bool
	Username    string
	StatusTitle string
	StatusDesc  string
	Version     string
}

// render ejecuta la plantilla base (que incluye los bloques "title" y
// "content" de la página indicada) con los datos proporcionados.
func (s *Server) render(w http.ResponseWriter, name string, data pageData) {
	data.Version = s.version // la versión se inyecta en todas las páginas
	page, ok := s.pages[name]
	if !ok {
		http.NotFound(w, nil)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := page.ExecuteTemplate(w, "base.html", data); err != nil {
		log.Printf("render %s: %v", name, err)
	}
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	n, _ := s.st.CountUsers()
	if n == 0 {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}
	if _, err := s.currentUser(r); err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (s *Server) handleSetupPage(w http.ResponseWriter, r *http.Request) {
	n, _ := s.st.CountUsers()
	if n > 0 {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	s.render(w, "setup.html", pageData{Setup: true})
}

func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	n, _ := s.st.CountUsers()
	if n == 0 {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}
	if _, err := s.currentUser(r); err == nil {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}
	s.render(w, "login.html", pageData{})
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	s.render(w, "dashboard.html", pageData{User: u, IsAdmin: u.IsAdmin(), Username: u.Username})
}

func (s *Server) handleUsersPage(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	s.render(w, "users.html", pageData{User: u, IsAdmin: true, Username: u.Username})
}

// handleAboutPage es pública: es la página de homenaje.
func (s *Server) handleAboutPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, "about.html", pageData{})
}
