package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"akena-watch/internal/store"
)

type notifInput struct {
	Name   string         `json:"name"`
	Type   string         `json:"type"`
	Config map[string]any `json:"config"`
	Active bool           `json:"active"`
}

func validateNotifInput(in notifInput) error {
	if in.Name == "" || len(in.Name) > 64 {
		return errors.New("el nombre debe tener entre 1 y 64 caracteres")
	}
	require := func(keys ...string) error {
		for _, k := range keys {
			v, ok := in.Config[k]
			if !ok || v == nil || v == "" {
				return fmt.Errorf("falta el campo %q", k)
			}
		}
		return nil
	}
	switch in.Type {
	case store.NotifWebhook:
		return require("url")
	case store.NotifTelegram:
		return require("bot_token", "chat_id")
	case store.NotifSMTP:
		if err := require("host", "from", "to"); err != nil {
			return err
		}
		p, _ := in.Config["port"].(float64)
		if p < 1 || p > 65535 {
			return errors.New("el puerto SMTP debe estar entre 1 y 65535")
		}
		return nil
	}
	return errors.New("tipo de canal no válido (webhook, telegram o smtp)")
}

func notifPayload(n store.Notification) map[string]any {
	var cfg map[string]any
	_ = json.Unmarshal([]byte(n.Config), &cfg)
	return map[string]any{
		"id": n.ID, "name": n.Name, "type": n.Type, "config": cfg, "active": n.Active,
	}
}

func (s *Server) handleListNotifications(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	notifs, err := s.st.ListNotifications(u.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "error interno")
		return
	}
	out := make([]map[string]any, 0, len(notifs))
	for _, n := range notifs {
		out = append(out, notifPayload(n))
	}
	writeOK(w, map[string]any{"notifications": out})
}

func (s *Server) handleCreateNotification(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	var in notifInput
	if err := readJSON(w, r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "solicitud inválida")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if err := validateNotifInput(in); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	cfg, err := json.Marshal(in.Config)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "configuración inválida")
		return
	}
	n, err := s.st.CreateNotification(store.Notification{
		OwnerID: u.ID, Name: in.Name, Type: in.Type, Config: string(cfg), Active: in.Active,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "no se pudo crear el canal")
		return
	}
	writeOK(w, map[string]any{"notification": notifPayload(n)})
}

func (s *Server) handleUpdateNotification(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	id, err := parseID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "id inválido")
		return
	}
	cur, err := s.st.GetNotification(id)
	if err != nil || cur.OwnerID != u.ID {
		writeErr(w, http.StatusNotFound, "canal no encontrado")
		return
	}
	var in notifInput
	if err := readJSON(w, r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "solicitud inválida")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if err := validateNotifInput(in); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	cfg, err := json.Marshal(in.Config)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "configuración inválida")
		return
	}
	cur.Name, cur.Type, cur.Config, cur.Active = in.Name, in.Type, string(cfg), in.Active
	if err := s.st.UpdateNotification(cur); err != nil {
		writeErr(w, http.StatusInternalServerError, "no se pudo actualizar el canal")
		return
	}
	writeOK(w, map[string]any{"notification": notifPayload(cur)})
}

func (s *Server) handleDeleteNotification(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	id, err := parseID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "id inválido")
		return
	}
	cur, err := s.st.GetNotification(id)
	if err != nil || cur.OwnerID != u.ID {
		writeErr(w, http.StatusNotFound, "canal no encontrado")
		return
	}
	if err := s.st.DeleteNotification(id); err != nil {
		writeErr(w, http.StatusInternalServerError, "no se pudo eliminar el canal")
		return
	}
	writeOK(w, map[string]any{})
}
