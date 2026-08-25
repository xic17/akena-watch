package store

import (
	"database/sql"
	"errors"
	"time"
)

// Estado de un heartbeat.
const (
	StatusUp   = "up"
	StatusDown = "down"
)

// Heartbeat es el resultado de un check en un instante dado.
type Heartbeat struct {
	ID        int64
	MonitorID int64
	Status    string
	Code      int
	LatencyMS int
	Error     string
	CheckedAt time.Time
}

func (s *Store) InsertHeartbeat(h Heartbeat) error {
	_, err := s.db.Exec(
		`INSERT INTO heartbeats (monitor_id, status, code, latency_ms, error, checked_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		h.MonitorID, h.Status, h.Code, h.LatencyMS, h.Error, h.CheckedAt.UTC().Format(timeFmt))
	return err
}

// LatestHeartbeat devuelve el último heartbeat de un monitor, o nil.
func (s *Store) LatestHeartbeat(monitorID int64) (*Heartbeat, error) {
	row := s.db.QueryRow(
		`SELECT id, monitor_id, status, code, latency_ms, error, checked_at
		 FROM heartbeats WHERE monitor_id = ? ORDER BY id DESC LIMIT 1`, monitorID)
	h, err := scanHeartbeat(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &h, nil
}

// ListHeartbeats devuelve heartbeats posteriores a since, más recientes primero.
func (s *Store) ListHeartbeats(monitorID int64, since time.Time, limit int) ([]Heartbeat, error) {
	rows, err := s.db.Query(
		`SELECT id, monitor_id, status, code, latency_ms, error, checked_at
		 FROM heartbeats WHERE monitor_id = ? AND checked_at >= ?
		 ORDER BY id DESC LIMIT ?`, monitorID, since.UTC().Format(timeFmt), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Heartbeat
	for rows.Next() {
		h, err := scanHeartbeat(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// Uptime cuenta heartbeats "up" y totales desde since.
func (s *Store) Uptime(monitorID int64, since time.Time) (up, total int, err error) {
	err = s.db.QueryRow(
		`SELECT
		   COALESCE(SUM(CASE WHEN status = 'up' THEN 1 ELSE 0 END), 0),
		   COUNT(*)
		 FROM heartbeats WHERE monitor_id = ? AND checked_at >= ?`,
		monitorID, since.UTC().Format(timeFmt)).Scan(&up, &total)
	return up, total, err
}

// PruneHeartbeats conserva solo los `keep` heartbeats más recientes
// por monitor y descarta el resto.
func (s *Store) PruneHeartbeats(keep int) error {
	_, err := s.db.Exec(
		`DELETE FROM heartbeats WHERE id IN (
			SELECT id FROM (
				SELECT id, ROW_NUMBER() OVER (PARTITION BY monitor_id ORDER BY id DESC) AS rn
				FROM heartbeats
			) WHERE rn > ?)`, keep)
	return err
}

func scanHeartbeat(row scanner) (Heartbeat, error) {
	var h Heartbeat
	var checkedAt string
	err := row.Scan(&h.ID, &h.MonitorID, &h.Status, &h.Code, &h.LatencyMS, &h.Error, &checkedAt)
	if err != nil {
		return Heartbeat{}, err
	}
	h.CheckedAt = parseTime(checkedAt)
	return h, nil
}
