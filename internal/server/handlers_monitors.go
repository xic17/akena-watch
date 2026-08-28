package server

import (
	"context"
	"errors"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"akena-watch/internal/monitor"
	"akena-watch/internal/store"
)

type monitorInput struct {
	Name           string  `json:"name"`
	Group          string  `json:"group"`
	Type           string  `json:"type"`
	URL            string  `json:"url"`
	Method         string  `json:"method"`
	ExpectedStatus int     `json:"expected_status"`
	Keyword        string  `json:"keyword"`
	Body           string  `json:"body"`
	InvertKeyword  bool    `json:"invert_keyword"`
	TimeoutS       int     `json:"timeout_s"`
	IntervalS      int     `json:"interval_s"`
	Active         *bool   `json:"active"`
	Public         *bool   `json:"public"`
	Notify         *bool   `json:"notify"`
	NotifyOwner    *bool   `json:"notify_owner"`
	MaxRetries     int     `json:"max_retries"`
	NotifierIDs    []int64 `json:"notifier_ids"`
	// 0 = desactivado; si la latencia supera el umbral durante slow_retries
	// checks consecutivos (con estado up), se alerta como "lento".
	LatencyThresholdMS int `json:"latency_threshold_ms"`
	SlowRetries        int `json:"slow_retries"`
	// 0 = desactivado; avisa cuando el certificado TLS expire en ≤ N días.
	CertAlertDays int `json:"cert_alert_days"`
}

// toMonitor valida la entrada y aplica valores por defecto.
func (in monitorInput) toMonitor() (store.Monitor, error) {
	m := store.Monitor{
		Name:           strings.TrimSpace(in.Name),
		Group:          strings.TrimSpace(in.Group),
		Type:           in.Type,
		URL:            strings.TrimSpace(in.URL),
		Method:         strings.ToUpper(strings.TrimSpace(in.Method)),
		ExpectedStatus: in.ExpectedStatus,
		Keyword:        in.Keyword,
		Body:           in.Body,
		InvertKeyword:  in.InvertKeyword,
		TimeoutS:       in.TimeoutS,
		IntervalS:      in.IntervalS,
		MaxRetries:     in.MaxRetries,
		LatencyThresholdMS: in.LatencyThresholdMS,
		SlowRetries:        in.SlowRetries,
		CertAlertDays:      in.CertAlertDays,
	}
	// Por defecto un monitor nuevo está activo y con alertas habilitadas.
	m.Active = in.Active == nil || *in.Active
	m.Notify = in.Notify == nil || *in.Notify
	if in.Public != nil {
		m.Public = *in.Public
	}
	if in.NotifyOwner != nil {
		m.NotifyOwner = *in.NotifyOwner
	}
	if m.Method == "" {
		m.Method = http.MethodGet
	}
	if m.ExpectedStatus == 0 {
		m.ExpectedStatus = 200
	}
	if m.TimeoutS == 0 {
		m.TimeoutS = 10
	}
	if m.IntervalS == 0 {
		m.IntervalS = 60
	}
	if m.MaxRetries == 0 {
		m.MaxRetries = 1
	}

	if len(m.Name) == 0 || len(m.Name) > 64 {
		return m, errors.New("el nombre debe tener entre 1 y 64 caracteres")
	}
	if len(m.Group) > 64 {
		return m, errors.New("el grupo no puede superar 64 caracteres")
	}
	switch m.Type {
	case store.TypeHTTP:
		if !strings.HasPrefix(m.URL, "http://") && !strings.HasPrefix(m.URL, "https://") {
			return m, errors.New("la URL HTTP debe comenzar con http:// o https://")
		}
		switch m.Method {
		case http.MethodGet, http.MethodPost, http.MethodHead, http.MethodPut,
			http.MethodPatch, http.MethodDelete, http.MethodOptions:
		default:
			return m, errors.New("método HTTP no válido")
		}
		if m.ExpectedStatus < 100 || m.ExpectedStatus > 599 {
			return m, errors.New("el estado HTTP esperado debe estar entre 100 y 599")
		}
	case store.TypeTCP, store.TypeDNS:
		if m.URL == "" {
			return m, errors.New("el destino no puede estar vacío")
		}
		if m.Body != "" {
			return m, errors.New("el cuerpo JSON solo aplica a monitores HTTP")
		}
	default:
		return m, errors.New("tipo de monitor no válido (http, tcp o dns)")
	}
	if m.TimeoutS < 1 || m.TimeoutS > 120 {
		return m, errors.New("el timeout debe estar entre 1 y 120 segundos")
	}
	if m.IntervalS < 10 || m.IntervalS > 86400 {
		return m, errors.New("el intervalo debe estar entre 10 segundos y 24 horas")
	}
	if m.MaxRetries < 1 || m.MaxRetries > 10 {
		return m, errors.New("los reintentos deben estar entre 1 y 10")
	}
	if m.LatencyThresholdMS < 0 || m.LatencyThresholdMS > 60000 {
		return m, errors.New("el umbral de lentitud debe estar entre 0 y 60000 ms")
	}
	if m.LatencyThresholdMS > 0 && (m.SlowRetries < 1 || m.SlowRetries > 10) {
		return m, errors.New("los checks de lentitud deben estar entre 1 y 10")
	}
	if m.CertAlertDays < 0 || m.CertAlertDays > 365 {
		return m, errors.New("el aviso de certificado debe estar entre 0 y 365 días")
	}
	return m, nil
}

func parseID(r *http.Request, key string) (int64, error) {
	return strconv.ParseInt(r.PathValue(key), 10, 64)
}

func round2(x float64) float64 { return math.Round(x*100) / 100 }

// monitorPayload arma el objeto JSON completo de un monitor para la UI.
func (s *Server) monitorPayload(m store.MonitorWithOwner) (map[string]any, error) {
	p := map[string]any{
		"id": m.ID, "owner_id": m.OwnerID, "owner": m.OwnerName, "name": m.Name,
		"group": m.Group, "type": m.Type, "url": m.URL, "method": m.Method,
		"expected_status": m.ExpectedStatus, "keyword": m.Keyword, "body": m.Body,
		"invert_keyword": m.InvertKeyword,
		"timeout_s":      m.TimeoutS, "interval_s": m.IntervalS,
		"active": m.Active, "public": m.Public, "notify": m.Notify, "notify_owner": m.NotifyOwner,
		"max_retries": m.MaxRetries,
		"latency_threshold_ms": m.LatencyThresholdMS,
		"slow_retries":        m.SlowRetries,
		"cert_alert_days":     m.CertAlertDays,
	}

	now := time.Now()
	if up, total, err := s.st.Uptime(m.ID, now.Add(-24*time.Hour)); err == nil && total > 0 {
		p["uptime_24h"] = round2(float64(up) / float64(total) * 100)
	}
	if up, total, err := s.st.Uptime(m.ID, now.Add(-7*24*time.Hour)); err == nil && total > 0 {
		p["uptime_7d"] = round2(float64(up) / float64(total) * 100)
	}
	if hb, err := s.st.LatestHeartbeat(m.ID); err == nil && hb != nil {
		p["last_heartbeat"] = map[string]any{
			"status": hb.Status, "code": hb.Code, "latency_ms": hb.LatencyMS,
			"error": hb.Error, "checked_at": hb.CheckedAt.Format(time.RFC3339),
		}
		// estado "lento" derivado: latencia actual por encima del umbral
		p["slow"] = m.LatencyThresholdMS > 0 && hb.Status == store.StatusUp &&
			hb.LatencyMS >= m.LatencyThresholdMS
	}
	if shares, err := s.st.ListShares(m.ID); err == nil {
		list := make([]map[string]any, 0, len(shares))
		for _, sh := range shares {
			list = append(list, map[string]any{
				"user_id": sh.UserID, "username": sh.Username, "can_edit": sh.CanEdit,
			})
		}
		p["shares"] = list
	}
	if notifs, err := s.st.ListNotificationsForMonitor(m.ID); err == nil {
		ids := make([]int64, 0, len(notifs))
		for _, n := range notifs {
			ids = append(ids, n.ID)
		}
		p["notifier_ids"] = ids
	}
	return p, nil
}

func (s *Server) monitorWithOwner(m store.Monitor) (store.MonitorWithOwner, error) {
	u, err := s.st.GetUserByID(m.OwnerID)
	if err != nil {
		return store.MonitorWithOwner{}, err
	}
	return store.MonitorWithOwner{Monitor: m, OwnerName: u.Username}, nil
}

// --- handlers ---

func (s *Server) handleListMonitors(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	mons, err := s.st.ListMonitorsForUser(u.ID, u.IsAdmin())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "error interno")
		return
	}
	out := make([]map[string]any, 0, len(mons))
	for _, m := range mons {
		if p, err := s.monitorPayload(m); err == nil {
			p["access"] = s.monitorAccess(u, m)
			out = append(out, p)
		}
	}
	writeOK(w, map[string]any{"monitors": out})
}

// monitorAccess indica cómo ve el usuario el monitor: "owner", "shared",
// "group" (por su acceso a grupos) o "admin".
func (s *Server) monitorAccess(u *store.User, m store.MonitorWithOwner) string {
	if u.IsAdmin() {
		return "admin"
	}
	if m.OwnerID == u.ID {
		return "owner"
	}
	shares, err := s.st.ListShares(m.ID)
	if err == nil {
		for _, sh := range shares {
			if sh.UserID == u.ID {
				return "shared"
			}
		}
	}
	return "group"
}

func (s *Server) handleCreateMonitor(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	var in monitorInput
	if err := readJSON(w, r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "solicitud inválida")
		return
	}
	m, err := in.toMonitor()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	m.OwnerID = u.ID
	created, err := s.st.CreateMonitor(m)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "no se pudo crear el monitor")
		return
	}
	if err := s.st.SetMonitorNotifiers(created.ID, in.NotifierIDs); err != nil {
		writeErr(w, http.StatusInternalServerError, "no se pudieron asociar los canales")
		return
	}
	p, _ := s.monitorPayload(store.MonitorWithOwner{Monitor: created, OwnerName: u.Username})
	writeOK(w, map[string]any{"monitor": p})
}

func (s *Server) handleGetMonitor(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	id, err := parseID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "id inválido")
		return
	}
	ok, err := s.st.CanViewMonitor(u.ID, id, u.IsAdmin())
	if err != nil || !ok {
		writeErr(w, http.StatusNotFound, "monitor no encontrado")
		return
	}
	m, err := s.st.GetMonitor(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "monitor no encontrado")
		return
	}
	mo, err := s.monitorWithOwner(m)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "error interno")
		return
	}
	p, _ := s.monitorPayload(mo)
	writeOK(w, map[string]any{"monitor": p})
}

func (s *Server) handleUpdateMonitor(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	id, err := parseID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "id inválido")
		return
	}
	canEdit, err := s.st.CanEditMonitor(u.ID, id, u.IsAdmin())
	if err != nil || !canEdit {
		writeErr(w, http.StatusForbidden, "no tienes permiso para editar este monitor")
		return
	}
	cur, err := s.st.GetMonitor(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "monitor no encontrado")
		return
	}

	var in monitorInput
	if err := readJSON(w, r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "solicitud inválida")
		return
	}
	nm, err := in.toMonitor()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	// conserva identidad y propiedad
	cur.Name, cur.Type, cur.URL = nm.Name, nm.Type, nm.URL
	cur.Group = nm.Group
	cur.Method, cur.ExpectedStatus, cur.Keyword = nm.Method, nm.ExpectedStatus, nm.Keyword
	cur.Body, cur.InvertKeyword = nm.Body, nm.InvertKeyword
	cur.TimeoutS, cur.IntervalS = nm.TimeoutS, nm.IntervalS
	cur.Active, cur.Public, cur.Notify, cur.MaxRetries = nm.Active, nm.Public, nm.Notify, nm.MaxRetries
	cur.NotifyOwner = nm.NotifyOwner
	cur.LatencyThresholdMS = nm.LatencyThresholdMS
	cur.SlowRetries = nm.SlowRetries
	cur.CertAlertDays = nm.CertAlertDays

	if err := s.st.UpdateMonitor(cur); err != nil {
		writeErr(w, http.StatusInternalServerError, "no se pudo actualizar el monitor")
		return
	}
	if err := s.st.SetMonitorNotifiers(id, in.NotifierIDs); err != nil {
		writeErr(w, http.StatusInternalServerError, "no se pudieron asociar los canales")
		return
	}
	mo, err := s.monitorWithOwner(cur) // el propietario real, no el editor
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "error interno")
		return
	}
	p, _ := s.monitorPayload(mo)
	writeOK(w, map[string]any{"monitor": p})
}

// handleSetMonitorActive pausa (active=false) o reanuda (active=true) un
// monitor desde el dashboard: el scheduler lo deja de comprobar al instante.
func (s *Server) handleSetMonitorActive(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	id, err := parseID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "id inválido")
		return
	}
	canEdit, err := s.st.CanEditMonitor(u.ID, id, u.IsAdmin())
	if err != nil || !canEdit {
		writeErr(w, http.StatusForbidden, "no tienes permiso para modificar este monitor")
		return
	}
	var in struct {
		Active bool `json:"active"`
	}
	if err := readJSON(w, r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "solicitud inválida")
		return
	}
	if err := s.st.SetMonitorActive(id, in.Active); err != nil {
		writeErr(w, http.StatusInternalServerError, "no se pudo actualizar el monitor")
		return
	}
	m, err := s.st.GetMonitor(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "monitor no encontrado")
		return
	}
	mo, err := s.monitorWithOwner(m)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "error interno")
		return
	}
	p, _ := s.monitorPayload(mo)
	writeOK(w, map[string]any{"monitor": p})
}

func (s *Server) handleDeleteMonitor(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	id, err := parseID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "id inválido")
		return
	}
	m, err := s.st.GetMonitor(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "monitor no encontrado")
		return
	}
	// Solo el propietario (o un admin) puede eliminar: ni las comparticiones
	// con edición ni los accesos por grupo/manual habilitan el borrado.
	if !u.IsAdmin() && m.OwnerID != u.ID {
		writeErr(w, http.StatusForbidden, "solo el propietario puede eliminar un monitor")
		return
	}
	if err := s.st.DeleteMonitor(id); err != nil {
		writeErr(w, http.StatusInternalServerError, "no se pudo eliminar el monitor")
		return
	}
	writeOK(w, map[string]any{})
}

// handleTestMonitor ejecuta un check manual sin guardar resultados.
func (s *Server) handleTestMonitor(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	id, err := parseID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "id inválido")
		return
	}
	ok, err := s.st.CanViewMonitor(u.ID, id, u.IsAdmin())
	if err != nil || !ok {
		writeErr(w, http.StatusNotFound, "monitor no encontrado")
		return
	}
	m, err := s.st.GetMonitor(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "monitor no encontrado")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(m.TimeoutS+5)*time.Second)
	defer cancel()
	res := monitor.Check(ctx, m)
	writeOK(w, map[string]any{
		"status": res.Status, "code": res.Code, "latency_ms": res.LatencyMS, "error": res.Error,
	})
}

func (s *Server) handleHeartbeats(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	id, err := parseID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "id inválido")
		return
	}
	ok, err := s.st.CanViewMonitor(u.ID, id, u.IsAdmin())
	if err != nil || !ok {
		writeErr(w, http.StatusNotFound, "monitor no encontrado")
		return
	}
	hours := int64(24)
	if h := r.URL.Query().Get("hours"); h != "" {
		if v, err := strconv.ParseInt(h, 10, 64); err == nil && v > 0 && v <= 24*90 {
			hours = v
		}
	}
	hb, err := s.st.ListHeartbeats(id, time.Now().Add(-time.Duration(hours)*time.Hour), 1000)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "error interno")
		return
	}
	out := make([]map[string]any, 0, len(hb))
	for _, h := range hb {
		out = append(out, map[string]any{
			"status": h.Status, "code": h.Code, "latency_ms": h.LatencyMS,
			"error": h.Error, "checked_at": h.CheckedAt.Format(time.RFC3339),
		})
	}
	writeOK(w, map[string]any{"heartbeats": out})
}

// handleMonitorStats devuelve estadísticas detalladas de un monitor:
// uptime en varias ventanas, percentiles de latencia y últimos eventos.
func (s *Server) handleMonitorStats(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	id, err := parseID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "id inválido")
		return
	}
	ok, err := s.st.CanViewMonitor(u.ID, id, u.IsAdmin())
	if err != nil || !ok {
		writeErr(w, http.StatusNotFound, "monitor no encontrado")
		return
	}

	now := time.Now()
	uptime := map[string]any{}
	for _, win := range []struct {
		key string
		d   time.Duration
	}{
		{"24h", 24 * time.Hour},
		{"7d", 7 * 24 * time.Hour},
		{"30d", 30 * 24 * time.Hour},
	} {
		if up, total, err := s.st.Uptime(id, now.Add(-win.d)); err == nil && total > 0 {
			uptime[win.key] = round2(float64(up) / float64(total) * 100)
		}
	}

	// latencias y conteos sobre la ventana de 7 días
	hb, err := s.st.ListHeartbeats(id, now.Add(-7*24*time.Hour), 20000)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "error interno")
		return
	}
	var lats []int
	up, down := 0, 0
	for _, h := range hb {
		if h.LatencyMS > 0 {
			lats = append(lats, h.LatencyMS)
		}
		if h.Status == store.StatusUp {
			up++
		} else {
			down++
		}
	}

	latency := map[string]any{"min": 0, "max": 0, "avg": 0, "p95": 0}
	if len(lats) > 0 {
		sort.Ints(lats)
		sum := 0
		for _, v := range lats {
			sum += v
		}
		latency["min"] = lats[0]
		latency["max"] = lats[len(lats)-1]
		latency["avg"] = round2(float64(sum) / float64(len(lats)))
		latency["p95"] = lats[int(float64(len(lats)-1)*0.95)]
	}

	events := make([]map[string]any, 0, 20)
	for _, h := range hb {
		if len(events) >= 20 {
			break
		}
		events = append(events, map[string]any{
			"status": h.Status, "code": h.Code, "latency_ms": h.LatencyMS,
			"error": h.Error, "checked_at": h.CheckedAt.Format(time.RFC3339),
		})
	}

	writeOK(w, map[string]any{
		"uptime":  uptime,
		"latency": latency,
		"checks":  map[string]any{"total": up + down, "up": up, "down": down},
		"events":  events,
	})
}

// handleAllHeartbeats devuelve los heartbeats recientes de todos los
// monitores visibles para el usuario (para dibujar gráficas de una vez).
func (s *Server) handleAllHeartbeats(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	hours := int64(24)
	if h := r.URL.Query().Get("hours"); h != "" {
		if v, err := strconv.ParseInt(h, 10, 64); err == nil && v > 0 && v <= 24*30 {
			hours = v
		}
	}

	mons, err := s.st.ListMonitorsForUser(u.ID, u.IsAdmin())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "error interno")
		return
	}
	visible := make(map[int64]bool, len(mons))
	for _, m := range mons {
		visible[m.ID] = true
	}

	hb, err := s.st.ListRecentForMonitors(time.Now().Add(-time.Duration(hours)*time.Hour), 500)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "error interno")
		return
	}
	out := make([]map[string]any, 0, len(hb))
	for _, h := range hb {
		if !visible[h.MonitorID] {
			continue
		}
		out = append(out, map[string]any{
			"monitor_id": h.MonitorID, "status": h.Status, "code": h.Code,
			"latency_ms": h.LatencyMS, "error": h.Error,
			"checked_at": h.CheckedAt.Format(time.RFC3339),
		})
	}
	writeOK(w, map[string]any{"heartbeats": out})
}

// --- comparticiones ---

func (s *Server) canManageShares(u *store.User, m store.Monitor) bool {
	return u.IsAdmin() || u.ID == m.OwnerID
}

func (s *Server) handleSetShare(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	mid, err := parseID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "id inválido")
		return
	}
	uid, err := parseID(r, "uid")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "usuario inválido")
		return
	}
	m, err := s.st.GetMonitor(mid)
	if err != nil {
		writeErr(w, http.StatusNotFound, "monitor no encontrado")
		return
	}
	if !s.canManageShares(u, m) {
		writeErr(w, http.StatusForbidden, "solo el propietario puede compartir el monitor")
		return
	}
	if uid == m.OwnerID {
		writeErr(w, http.StatusBadRequest, "el monitor ya pertenece a ese usuario")
		return
	}
	if _, err := s.st.GetUserByID(uid); err != nil {
		writeErr(w, http.StatusNotFound, "usuario no encontrado")
		return
	}
	var in struct {
		CanEdit bool `json:"can_edit"`
	}
	_ = readJSON(w, r, &in)
	if err := s.st.SetShare(mid, uid, in.CanEdit); err != nil {
		writeErr(w, http.StatusInternalServerError, "no se pudo compartir")
		return
	}
	writeOK(w, map[string]any{})
}

func (s *Server) handleDeleteShare(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	mid, err := parseID(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "id inválido")
		return
	}
	uid, err := parseID(r, "uid")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "usuario inválido")
		return
	}
	m, err := s.st.GetMonitor(mid)
	if err != nil {
		writeErr(w, http.StatusNotFound, "monitor no encontrado")
		return
	}
	if !s.canManageShares(u, m) {
		writeErr(w, http.StatusForbidden, "solo el propietario puede dejar de compartir")
		return
	}
	if err := s.st.DeleteShare(mid, uid); err != nil {
		writeErr(w, http.StatusInternalServerError, "no se pudo eliminar la compartición")
		return
	}
	writeOK(w, map[string]any{})
}
