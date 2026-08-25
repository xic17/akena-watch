package server

import (
	"net/http"
	"strings"
	"time"
)

// handleGetStatusPage devuelve la configuración de la página de estado
// del usuario autenticado.
func (s *Server) handleGetStatusPage(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	writeOK(w, map[string]any{
		"title":        u.StatusTitle,
		"desc":         u.StatusDesc,
		"slug":         u.Username,
		"url":          "/status/" + u.Username,
		"public_count": s.countPublicMonitors(u.ID),
	})
}

func (s *Server) handleUpdateStatusPage(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	var in struct {
		Title string `json:"title"`
		Desc  string `json:"desc"`
	}
	if err := readJSON(w, r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "solicitud inválida")
		return
	}
	in.Title = strings.TrimSpace(in.Title)
	in.Desc = strings.TrimSpace(in.Desc)
	if len(in.Title) > 100 {
		writeErr(w, http.StatusBadRequest, "el título no puede superar 100 caracteres")
		return
	}
	if len(in.Desc) > 500 {
		writeErr(w, http.StatusBadRequest, "la descripción no puede superar 500 caracteres")
		return
	}
	if in.Title == "" {
		in.Title = "Estado de los servicios"
	}
	if err := s.st.UpdateStatusPage(u.ID, in.Title, in.Desc); err != nil {
		writeErr(w, http.StatusInternalServerError, "no se pudo actualizar la página de estado")
		return
	}
	writeOK(w, map[string]any{"title": in.Title, "desc": in.Desc})
}

func (s *Server) countPublicMonitors(userID int64) int {
	mons, err := s.st.ListPublicMonitors(userID)
	if err != nil {
		return 0
	}
	return len(mons)
}

// handleStatusPage es la página de estado pública (sin autenticación).
// Con ?json=1 devuelve datos para que el navegador refresque solo.
func (s *Server) handleStatusPage(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	u, err := s.st.GetUserByUsername(slug)
	if err != nil {
		if r.URL.Query().Get("json") == "1" {
			writeErr(w, http.StatusNotFound, "página de estado no encontrada")
			return
		}
		http.NotFound(w, r)
		return
	}
	mons, err := s.st.ListPublicMonitors(u.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "error interno")
		return
	}

	if r.URL.Query().Get("json") == "1" {
		out := make([]map[string]any, 0, len(mons))
		for _, m := range mons {
			item := map[string]any{"name": m.Name, "type": m.Type}
			if hb, err := s.st.LatestHeartbeat(m.ID); err == nil && hb != nil {
				item["status"] = hb.Status
				item["latency_ms"] = hb.LatencyMS
				item["error"] = hb.Error
				item["last_checked"] = hb.CheckedAt.Format("2006-01-02T15:04:05Z07:00")
			}
			if up, total, err := s.st.Uptime(m.ID, nowMinus(30*24)); err == nil && total > 0 {
				item["uptime_30d"] = round2(float64(up) / float64(total) * 100)
			}
			out = append(out, item)
		}
		writeOK(w, map[string]any{"title": u.StatusTitle, "desc": u.StatusDesc, "monitors": out})
		return
	}

	s.render(w, "status.html", pageData{
		Username:    u.Username,
		StatusTitle: u.StatusTitle,
		StatusDesc:  u.StatusDesc,
	})
}

func nowMinus(d time.Duration) time.Time { return time.Now().Add(-d) }
