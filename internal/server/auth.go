package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"akena-watch/internal/store"
	"golang.org/x/crypto/bcrypt"
)

const (
	sessionCookie = "akena_session"
	sessionTTL    = 30 * 24 * time.Hour
)

type ctxKey int

const ctxUserKey ctxKey = iota

// --- sesiones ---

func newSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *Server) currentUser(r *http.Request) (store.User, error) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return store.User{}, store.ErrNotFound
	}
	return s.st.UserForSession(c.Value)
}

func userFrom(r *http.Request) *store.User {
	u, _ := r.Context().Value(ctxUserKey).(*store.User)
	return u
}

func (s *Server) startSession(w http.ResponseWriter, u store.User) error {
	token, err := newSessionToken()
	if err != nil {
		return err
	}
	if err := s.st.CreateSession(u.ID, token, sessionTTL); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})
	return nil
}

func (s *Server) clearSession(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		_ = s.st.DeleteSession(c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
}

// --- middlewares de autenticación ---

func (s *Server) authPage(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, err := s.currentUser(r)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), ctxUserKey, &u)))
	}
}

func (s *Server) adminPage(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := userFrom(r)
		if u == nil || !u.IsAdmin() {
			http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

func (s *Server) authJSON(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, err := s.currentUser(r)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "sesión inválida o expirada")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), ctxUserKey, &u)))
	}
}

func (s *Server) adminJSON(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := userFrom(r)
		if u == nil || !u.IsAdmin() {
			writeErr(w, http.StatusForbidden, "se requiere rol de administrador")
			return
		}
		next(w, r)
	}
}

// --- instalación (primer arranque) y login ---

var usernameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
var emailRe = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

func validateCredentials(username, password string) error {
	if len(username) < 3 || len(username) > 32 {
		return errors.New("el usuario debe tener entre 3 y 32 caracteres")
	}
	if !usernameRe.MatchString(username) {
		return errors.New("el usuario solo puede contener letras, números, guiones y guiones bajos")
	}
	if len(password) < 8 {
		return errors.New("la contraseña debe tener al menos 8 caracteres")
	}
	return nil
}

// validateEmail valida un correo opcional: vacío es válido.
func validateEmail(email string) error {
	if email == "" {
		return nil
	}
	if len(email) > 254 || !emailRe.MatchString(email) {
		return errors.New("el correo no es válido")
	}
	return nil
}

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	n, err := s.st.CountUsers()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "error interno")
		return
	}
	if n > 0 {
		writeErr(w, http.StatusBadRequest, "la instalación ya fue completada")
		return
	}

	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Email    string `json:"email"`
	}
	if err := readJSON(w, r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "solicitud inválida")
		return
	}
	in.Username = strings.TrimSpace(in.Username)
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	if err := validateCredentials(in.Username, in.Password); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateEmail(in.Email); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "error interno")
		return
	}
	u, err := s.st.CreateUser(in.Username, string(hash), store.RoleAdmin, in.Email)
	if err == store.ErrEmailTaken {
		writeErr(w, http.StatusBadRequest, "ese correo ya está registrado")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "no se pudo crear el administrador")
		return
	}
	if err := s.startSession(w, u); err != nil {
		writeErr(w, http.StatusInternalServerError, "no se pudo iniciar la sesión")
		return
	}
	writeOK(w, map[string]any{"redirect": "/dashboard"})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(w, r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "solicitud inválida")
		return
	}

	u, err := s.st.GetUserByUsername(strings.TrimSpace(in.Username))
	if err != nil || bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(in.Password)) != nil {
		writeErr(w, http.StatusUnauthorized, "usuario o contraseña incorrectos")
		return
	}
	if err := s.startSession(w, u); err != nil {
		writeErr(w, http.StatusInternalServerError, "no se pudo iniciar la sesión")
		return
	}
	writeOK(w, map[string]any{"redirect": "/dashboard"})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.clearSession(w, r)
	writeOK(w, map[string]any{"redirect": "/login"})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	writeOK(w, map[string]any{
		"id":       u.ID,
		"username": u.Username,
		"email":    u.Email,
		"role":     u.Role,
	})
}

func (s *Server) handlePing(w http.ResponseWriter, _ *http.Request) {
	// Health check usado por el keep-alive de Cloudflare Containers.
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleVersion(w http.ResponseWriter, _ *http.Request) {
	writeOK(w, map[string]any{
		"name":    "akena-watch",
		"version": s.version,
	})
}
