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
		groups, _ := s.st.GetUserGroups(u.ID)
		access, _ := s.st.ListUserManualAccess(u.ID)
		out = append(out, map[string]any{
			"id": u.ID, "username": u.Username, "email": u.Email, "telegram_id": u.TelegramID,
			"role": u.Role, "monitors": n, "groups": groups, "access": access,
			"created_at": u.CreatedAt.Format("2006-01-02"),
		})
	}
	writeOK(w, map[string]any{"users": out})
}

// handleListGroups devuelve los grupos existentes con su número de monitores.
func (s *Server) handleListGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := s.st.ListGroups()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "error interno")
		return
	}
	out := make([]map[string]any, 0, len(groups))
	for _, g := range groups {
		out = append(out, map[string]any{"name": g.Name, "count": g.Count})
	}
	writeOK(w, map[string]any{"groups": out})
}

// handleSetUserGroups asigna los grupos que un colaborador puede ver.
func (s *Server) handleSetUserGroups(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "id inválido")
		return
	}
	if _, err := s.st.GetUserByID(id); err != nil {
		writeErr(w, http.StatusNotFound, "usuario no encontrado")
		return
	}
	var in struct {
		Groups []string `json:"groups"`
	}
	if err := readJSON(w, r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "solicitud inválida")
		return
	}
	seen := make(map[string]bool, len(in.Groups))
	groups := make([]string, 0, len(in.Groups))
	for _, g := range in.Groups {
		g = strings.TrimSpace(g)
		if g == "" || len(g) > 64 || seen[g] {
			continue
		}
		seen[g] = true
		groups = append(groups, g)
	}
	if err := s.st.SetUserGroups(id, groups); err != nil {
		writeErr(w, http.StatusInternalServerError, "no se pudieron guardar los grupos")
		return
	}
	writeOK(w, map[string]any{"groups": groups})
}

// handleSetUserAccess asigna manualmente los monitores que un colaborador
// puede ver (además de los de sus grupos).
func (s *Server) handleSetUserAccess(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "id inválido")
		return
	}
	if _, err := s.st.GetUserByID(id); err != nil {
		writeErr(w, http.StatusNotFound, "usuario no encontrado")
		return
	}
	var in struct {
		MonitorIDs []int64 `json:"monitor_ids"`
	}
	if err := readJSON(w, r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "solicitud inválida")
		return
	}
	if err := s.st.SetUserManualAccess(id, in.MonitorIDs); err != nil {
		writeErr(w, http.StatusInternalServerError, "no se pudo guardar el acceso")
		return
	}
	writeOK(w, map[string]any{})
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username   string `json:"username"`
		Password   string `json:"password"`
		Role       string `json:"role"`
		Email      string `json:"email"`
		TelegramID string `json:"telegram_id"`
	}
	if err := readJSON(w, r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "solicitud inválida")
		return
	}
	in.Username = strings.TrimSpace(in.Username)
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.TelegramID = strings.TrimSpace(in.TelegramID)
	if err := validateCredentials(in.Username, in.Password); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateEmail(in.Email); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateTelegramID(in.TelegramID); err != nil {
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
	u, err := s.st.CreateUser(in.Username, string(hash), in.Role, in.Email, in.TelegramID)
	if err == store.ErrUsernameTaken {
		writeErr(w, http.StatusBadRequest, "ese nombre de usuario ya existe")
		return
	}
	if err == store.ErrEmailTaken {
		writeErr(w, http.StatusBadRequest, "ese correo ya está registrado")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "no se pudo crear el usuario")
		return
	}
	writeOK(w, map[string]any{
		"user": map[string]any{"id": u.ID, "username": u.Username, "email": u.Email, "telegram_id": u.TelegramID, "role": u.Role},
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
		Role       string `json:"role"`
		Email      string `json:"email"`
		TelegramID string `json:"telegram_id"`
	}
	if err := readJSON(w, r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "solicitud inválida")
		return
	}
	if in.Role != store.RoleAdmin && in.Role != store.RoleCollaborator {
		writeErr(w, http.StatusBadRequest, "rol no válido")
		return
	}
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.TelegramID = strings.TrimSpace(in.TelegramID)
	if err := validateEmail(in.Email); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateTelegramID(in.TelegramID); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
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
	if err := s.st.UpdateUser(id, in.Role, in.Email, in.TelegramID); err == store.ErrEmailTaken {
		writeErr(w, http.StatusBadRequest, "ese correo ya está registrado")
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, "no se pudo actualizar el usuario")
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
