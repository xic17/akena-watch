package store

import (
	"database/sql"
	"errors"
	"time"
)

// Tipos de monitor soportados.
const (
	TypeHTTP = "http"
	TypeTCP  = "tcp"
	TypeDNS  = "dns"
)

// Monitor es la configuración de un check periódico.
type Monitor struct {
	ID             int64
	OwnerID        int64
	Name           string
	Type           string
	URL            string
	Method         string
	ExpectedStatus int
	Keyword        string
	InvertKeyword  bool
	TimeoutS       int
	IntervalS      int
	Active         bool
	Public         bool
	Notify         bool
	MaxRetries     int
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// MonitorWithOwner agrega el nombre del propietario para listados.
type MonitorWithOwner struct {
	Monitor
	OwnerName string
}

// Share es una compartición de monitor con otro usuario.
type Share struct {
	MonitorID int64
	UserID    int64
	CanEdit   bool
	Username  string
}

// CreateMonitor inserta un monitor y devuelve la fila completa.
func (s *Store) CreateMonitor(m Monitor) (Monitor, error) {
	now := nowStr()
	res, err := s.db.Exec(
		`INSERT INTO monitors (owner_id, name, type, url, method, expected_status, keyword,
		 invert_keyword, timeout_s, interval_s, active, public, notify, max_retries, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.OwnerID, m.Name, m.Type, m.URL, m.Method, m.ExpectedStatus, m.Keyword,
		boolInt(m.InvertKeyword), m.TimeoutS, m.IntervalS, boolInt(m.Active), boolInt(m.Public),
		boolInt(m.Notify), m.MaxRetries, now, now)
	if err != nil {
		return Monitor{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Monitor{}, err
	}
	return s.GetMonitor(id)
}

func (s *Store) UpdateMonitor(m Monitor) error {
	_, err := s.db.Exec(
		`UPDATE monitors SET name=?, type=?, url=?, method=?, expected_status=?, keyword=?,
		 invert_keyword=?, timeout_s=?, interval_s=?, active=?, public=?, notify=?, max_retries=?, updated_at=?
		 WHERE id=?`,
		m.Name, m.Type, m.URL, m.Method, m.ExpectedStatus, m.Keyword,
		boolInt(m.InvertKeyword), m.TimeoutS, m.IntervalS, boolInt(m.Active), boolInt(m.Public),
		boolInt(m.Notify), m.MaxRetries, nowStr(), m.ID)
	return err
}

func (s *Store) GetMonitor(id int64) (Monitor, error) {
	row := s.db.QueryRow("SELECT "+monitorCols+" FROM monitors m WHERE m.id = ?", id)
	return scanMonitor(row)
}

// ListMonitorsForUser devuelve los monitores visibles para un usuario:
// los propios, los compartidos con él y (si es admin) todos los del sistema.
func (s *Store) ListMonitorsForUser(userID int64, isAdmin bool) ([]MonitorWithOwner, error) {
	var rows *sql.Rows
	var err error
	if isAdmin {
		rows, err = s.db.Query(
			"SELECT " + monitorCols + ", u.username FROM monitors m JOIN users u ON u.id = m.owner_id ORDER BY m.name COLLATE NOCASE")
	} else {
		rows, err = s.db.Query(
			`SELECT `+monitorCols+`, u.username FROM monitors m
			 JOIN users u ON u.id = m.owner_id
			 LEFT JOIN monitor_shares ms ON ms.monitor_id = m.id
			 WHERE m.owner_id = ? OR ms.user_id = ?
			 ORDER BY m.name COLLATE NOCASE`, userID, userID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []MonitorWithOwner
	for rows.Next() {
		m, err := scanMonitorWithOwner(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ListActiveMonitors devuelve todos los monitores activos (para el scheduler).
func (s *Store) ListActiveMonitors() ([]Monitor, error) {
	rows, err := s.db.Query("SELECT " + monitorCols + " FROM monitors m WHERE m.active = 1")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Monitor
	for rows.Next() {
		m, err := scanMonitor(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// CountMonitors devuelve cuántos monitores posee un usuario.
func (s *Store) CountMonitors(userID int64) (int, error) {
	var n int
	err := s.db.QueryRow("SELECT COUNT(*) FROM monitors WHERE owner_id = ?", userID).Scan(&n)
	return n, err
}

// ListPublicMonitors devuelve los monitores marcados como públicos
// de un usuario (para la página de estado).
func (s *Store) ListPublicMonitors(userID int64) ([]Monitor, error) {
	rows, err := s.db.Query("SELECT "+monitorCols+" FROM monitors m WHERE m.owner_id = ? AND m.public = 1 ORDER BY m.name COLLATE NOCASE", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Monitor
	for rows.Next() {
		m, err := scanMonitor(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// DeleteMonitor elimina un monitor y su historial.
func (s *Store) DeleteMonitor(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, q := range []string{
		"DELETE FROM monitor_shares WHERE monitor_id = ?",
		"DELETE FROM monitor_notifiers WHERE monitor_id = ?",
		"DELETE FROM heartbeats WHERE monitor_id = ?",
		"DELETE FROM monitors WHERE id = ?",
	} {
		if _, err := tx.Exec(q, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// --- Comparticiones ---

func (s *Store) SetShare(monitorID, userID int64, canEdit bool) error {
	_, err := s.db.Exec(
		`INSERT INTO monitor_shares (monitor_id, user_id, can_edit) VALUES (?, ?, ?)
		 ON CONFLICT(monitor_id, user_id) DO UPDATE SET can_edit = excluded.can_edit`,
		monitorID, userID, boolInt(canEdit))
	return err
}

func (s *Store) DeleteShare(monitorID, userID int64) error {
	_, err := s.db.Exec("DELETE FROM monitor_shares WHERE monitor_id = ? AND user_id = ?", monitorID, userID)
	return err
}

func (s *Store) ListShares(monitorID int64) ([]Share, error) {
	rows, err := s.db.Query(
		`SELECT ms.monitor_id, ms.user_id, ms.can_edit, u.username
		 FROM monitor_shares ms JOIN users u ON u.id = ms.user_id
		 WHERE ms.monitor_id = ? ORDER BY u.username COLLATE NOCASE`, monitorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Share
	for rows.Next() {
		var sh Share
		var canEdit int
		if err := rows.Scan(&sh.MonitorID, &sh.UserID, &canEdit, &sh.Username); err != nil {
			return nil, err
		}
		sh.CanEdit = canEdit == 1
		out = append(out, sh)
	}
	return out, rows.Err()
}

// CanViewMonitor indica si un usuario puede ver un monitor
// (propietario, compartido o admin).
func (s *Store) CanViewMonitor(userID, monitorID int64, isAdmin bool) (bool, error) {
	if isAdmin {
		return true, nil
	}
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM monitors m
		 LEFT JOIN monitor_shares ms ON ms.monitor_id = m.id AND ms.user_id = ?
		 WHERE m.id = ? AND (m.owner_id = ? OR ms.user_id IS NOT NULL)`,
		userID, monitorID, userID).Scan(&n)
	return n > 0, err
}

// CanEditMonitor indica si un usuario puede modificar un monitor
// (propietario, compartido con edición o admin).
func (s *Store) CanEditMonitor(userID, monitorID int64, isAdmin bool) (bool, error) {
	if isAdmin {
		return true, nil
	}
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM monitors m
		 LEFT JOIN monitor_shares ms ON ms.monitor_id = m.id AND ms.user_id = ? AND ms.can_edit = 1
		 WHERE m.id = ? AND (m.owner_id = ? OR ms.user_id IS NOT NULL)`,
		userID, monitorID, userID).Scan(&n)
	return n > 0, err
}

// ListMonitorViewerIDs devuelve los IDs de todos los usuarios que deben
// recibir eventos en tiempo real de un monitor: propietario, compartidos
// y todos los administradores.
func (s *Store) ListMonitorViewerIDs(monitorID int64) ([]int64, error) {
	rows, err := s.db.Query(
		`SELECT DISTINCT u.id FROM users u
		 WHERE u.id IN (
			SELECT owner_id FROM monitors WHERE id = ?
			UNION
			SELECT user_id FROM monitor_shares WHERE monitor_id = ?
			UNION
			SELECT id FROM users WHERE role = 'admin'
		 )`, monitorID, monitorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// --- helpers de columnas ---

const monitorCols = `m.id, m.owner_id, m.name, m.type, m.url, m.method, m.expected_status,
	m.keyword, m.invert_keyword, m.timeout_s, m.interval_s, m.active, m.public, m.notify,
	m.max_retries, m.created_at, m.updated_at`

func scanMonitor(row scanner) (Monitor, error) {
	var m Monitor
	var inv, act, pub, not int
	var createdAt, updatedAt string
	err := row.Scan(&m.ID, &m.OwnerID, &m.Name, &m.Type, &m.URL, &m.Method, &m.ExpectedStatus,
		&m.Keyword, &inv, &m.TimeoutS, &m.IntervalS, &act, &pub, &not, &m.MaxRetries,
		&createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Monitor{}, ErrNotFound
	}
	if err != nil {
		return Monitor{}, err
	}
	m.InvertKeyword = inv == 1
	m.Active = act == 1
	m.Public = pub == 1
	m.Notify = not == 1
	m.CreatedAt = parseTime(createdAt)
	m.UpdatedAt = parseTime(updatedAt)
	return m, nil
}

type monitorRowScanner interface {
	scanner
	Columns() ([]string, error)
}

func scanMonitorWithOwner(row monitorRowScanner) (MonitorWithOwner, error) {
	var m Monitor
	var inv, act, pub, not int
	var createdAt, updatedAt string
	var owner string
	err := row.Scan(&m.ID, &m.OwnerID, &m.Name, &m.Type, &m.URL, &m.Method, &m.ExpectedStatus,
		&m.Keyword, &inv, &m.TimeoutS, &m.IntervalS, &act, &pub, &not, &m.MaxRetries,
		&createdAt, &updatedAt, &owner)
	if errors.Is(err, sql.ErrNoRows) {
		return MonitorWithOwner{}, ErrNotFound
	}
	if err != nil {
		return MonitorWithOwner{}, err
	}
	m.InvertKeyword = inv == 1
	m.Active = act == 1
	m.Public = pub == 1
	m.Notify = not == 1
	m.CreatedAt = parseTime(createdAt)
	m.UpdatedAt = parseTime(updatedAt)
	return MonitorWithOwner{Monitor: m, OwnerName: owner}, nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
