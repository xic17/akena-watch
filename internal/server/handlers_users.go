package server

import (
	"net/http"
	"strings"

	"akena-watch/internal/store"
	"golang.org/x/crypto/bcrypt"
)

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.st.ListUsers()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "error interno")
		return
	}
	out := make([]map[string]any, 0, len(users))
	for _, u := range users {
		n, err := s.st.CountMonitors(u.ID)
		if err != nil {
			n = 0
		}
		out = append(out, map[string]any{
			"id": u.ID, "username": u.Username, "role": u.Role,
			"monitors": n, "created_at": u.CreatedAt.Format("2006-01-02"),
		})
	}
	writeOK(w, map[string]any{"users": out})
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := readJSON(w, r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "solicitud inválida")
		return
	}
	in.Username = strings.TrimSpace(in.Username)
	if err := validateCredentials(in.Username, in.Password); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if in.Role != store.RoleAdmin && in.Role != store.RoleCollaborator {
		in.Role = store.RoleCollaborator
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "error interno")
		return
	}
	u, err := s.st.CreateUser(in.Username, string(hash), in.Role)
	if err == store.ErrUsernameTaken {
		writeErr(w, http.StatusBadRequest, "ese nombre de usuario ya existe")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "no se pudo crear el usuario")
		return
	}
	writeOK(w, map[string]any{
		"user": map[string]any{"id": u.ID, "username": u.Username, "role": u.Role},
	})
}

func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	id, err := parseID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "id inválido")
		return
	}
	var in struct {
		Role string `json:"role"`
	}
	if err := readJSON(w, r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "solicitud inválida")
		return
	}
	if in.Role != store.RoleAdmin && in.Role != store.RoleCollaborator {
		writeErr(w, http.StatusBadRequest, "rol no válido")
		return
	}
	target, err := s.st.GetUserByID(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "usuario no encontrado")
		return
	}
	if target.ID == me.ID && target.Role == store.RoleAdmin && in.Role != store.RoleAdmin {
		writeErr(w, http.StatusBadRequest, "no puedes quitarte el rol de administrador")
		return
	}
	if target.Role == store.RoleAdmin && in.Role != store.RoleAdmin {
		admins, err := s.st.CountAdmins()
		if err == nil && admins <= 1 {
			writeErr(w, http.StatusBadRequest, "debe existir al menos un administrador")
			return
		}
	}
	if err := s.st.UpdateUserRole(id, in.Role); err != nil {
		writeErr(w, http.StatusInternalServerError, "no se pudo actualizar el rol")
		return
	}
	writeOK(w, map[string]any{})
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	id, err := parseID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "id inválido")
		return
	}
	if id == me.ID {
		writeErr(w, http.StatusBadRequest, "no puedes eliminarte a ti mismo")
		return
	}
	target, err := s.st.GetUserByID(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "usuario no encontrado")
		return
	}
	if target.Role == store.RoleAdmin {
		admins, err := s.st.CountAdmins()
		if err == nil && admins <= 1 {
			writeErr(w, http.StatusBadRequest, "debe existir al menos un administrador")
			return
		}
	}
	if err := s.st.DeleteUser(id); err != nil {
		writeErr(w, http.StatusInternalServerError, "no se pudo eliminar el usuario")
		return
	}
	writeOK(w, map[string]any{})
}
