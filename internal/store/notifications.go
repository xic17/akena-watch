package store

import (
	"database/sql"
	"errors"
)

// Tipos de canal de notificación.
const (
	NotifWebhook  = "webhook"
	NotifTelegram = "telegram"
	NotifSMTP     = "smtp"
)

// Notification es un canal de alerta configurado por un usuario.
// Config guarda la configuración específica del tipo como JSON.
type Notification struct {
	ID      int64
	OwnerID int64
	Name    string
	Type    string
	Config  string
	Active  bool
}

func (s *Store) CreateNotification(n Notification) (Notification, error) {
	res, err := s.db.Exec(
		`INSERT INTO notifications (owner_id, name, type, config, active) VALUES (?, ?, ?, ?, ?)`,
		n.OwnerID, n.Name, n.Type, n.Config, boolInt(n.Active))
	if err != nil {
		return Notification{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Notification{}, err
	}
	return s.GetNotification(id)
}

func (s *Store) UpdateNotification(n Notification) error {
	_, err := s.db.Exec(
		`UPDATE notifications SET name = ?, type = ?, config = ?, active = ? WHERE id = ? AND owner_id = ?`,
		n.Name, n.Type, n.Config, boolInt(n.Active), n.ID, n.OwnerID)
	return err
}

func (s *Store) GetNotification(id int64) (Notification, error) {
	row := s.db.QueryRow(
		`SELECT id, owner_id, name, type, config, active FROM notifications WHERE id = ?`, id)
	return scanNotification(row)
}

// ListNotifications devuelve los canales de un usuario.
func (s *Store) ListNotifications(userID int64) ([]Notification, error) {
	rows, err := s.db.Query(
		`SELECT id, owner_id, name, type, config, active FROM notifications
		 WHERE owner_id = ? ORDER BY name COLLATE NOCASE`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Notification
	for rows.Next() {
		n, err := scanNotification(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// ListNotificationsForMonitor devuelve los canales activos asociados a un monitor.
func (s *Store) ListNotificationsForMonitor(monitorID int64) ([]Notification, error) {
	rows, err := s.db.Query(
		`SELECT n.id, n.owner_id, n.name, n.type, n.config, n.active
		 FROM notifications n
		 JOIN monitor_notifiers mn ON mn.notification_id = n.id
		 WHERE mn.monitor_id = ? AND n.active = 1
		 ORDER BY n.name COLLATE NOCASE`, monitorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Notification
	for rows.Next() {
		n, err := scanNotification(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) DeleteNotification(id int64) error {
	_, err := s.db.Exec("DELETE FROM notifications WHERE id = ?", id)
	return err
}

// SetMonitorNotifiers reemplaza los canales asociados a un monitor.
func (s *Store) SetMonitorNotifiers(monitorID int64, ids []int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM monitor_notifiers WHERE monitor_id = ?", monitorID); err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := tx.Exec(
			"INSERT OR IGNORE INTO monitor_notifiers (monitor_id, notification_id) VALUES (?, ?)",
			monitorID, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func scanNotification(row scanner) (Notification, error) {
	var n Notification
	var active int
	err := row.Scan(&n.ID, &n.OwnerID, &n.Name, &n.Type, &n.Config, &active)
	if errors.Is(err, sql.ErrNoRows) {
		return Notification{}, ErrNotFound
	}
	if err != nil {
		return Notification{}, err
	}
	n.Active = active == 1
	return n, nil
}
