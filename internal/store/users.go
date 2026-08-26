package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Roles disponibles.
const (
	RoleAdmin        = "admin"
	RoleCollaborator = "collaborator"
)

var (
	ErrNotFound      = errors.New("no encontrado")
	ErrUsernameTaken = errors.New("el nombre de usuario ya existe")
	ErrEmailTaken    = errors.New("ese correo ya está registrado")
)

// User es un usuario de la aplicación. Cada usuario posee sus propios
// monitores y canales de notificación.
type User struct {
	ID           int64
	Username     string
	Email        string
	TelegramID   string
	PasswordHash string
	Role         string
	StatusTitle  string
	StatusDesc   string
	CreatedAt    time.Time
}

func (u User) IsAdmin() bool { return u.Role == RoleAdmin }

// CountUsers devuelve el número total de usuarios. Con cero usuarios la
// aplicación entra en modo de instalación (primer arranque).
func (s *Store) CountUsers() (int, error) {
	var n int
	err := s.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&n)
	return n, err
}

// CreateUser crea un usuario y devuelve la fila completa.
func (s *Store) CreateUser(username, passwordHash, role, email, telegramID string) (User, error) {
	res, err := s.db.Exec(
		`INSERT INTO users (username, email, telegram_id, password_hash, role, status_title, status_desc, created_at)
		 VALUES (?, ?, ?, ?, ?, 'Estado de los servicios', '', ?)`,
		username, email, telegramID, passwordHash, role, nowStr())
	if err != nil {
		if isUniqueErr(err) {
			if strings.Contains(err.Error(), "users.username") {
				return User{}, ErrUsernameTaken
			}
			return User{}, ErrEmailTaken
		}
		return User{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return User{}, err
	}
	return s.GetUserByID(id)
}

func (s *Store) GetUserByID(id int64) (User, error) {
	row := s.db.QueryRow(
		`SELECT id, username, email, telegram_id, password_hash, role, status_title, status_desc, created_at
		 FROM users WHERE id = ?`, id)
	return scanUser(row)
}

func (s *Store) GetUserByUsername(username string) (User, error) {
	row := s.db.QueryRow(
		`SELECT id, username, email, telegram_id, password_hash, role, status_title, status_desc, created_at
		 FROM users WHERE username = ?`, username)
	return scanUser(row)
}

// ListUsers devuelve todos los usuarios ordenados por nombre.
func (s *Store) ListUsers() ([]User, error) {
	rows, err := s.db.Query(
		`SELECT id, username, email, telegram_id, password_hash, role, status_title, status_desc, created_at
		 FROM users ORDER BY username COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// UpdateUser actualiza el rol, el correo y el ID de Telegram de un usuario.
func (s *Store) UpdateUser(id int64, role, email, telegramID string) error {
	_, err := s.db.Exec("UPDATE users SET role = ?, email = ?, telegram_id = ? WHERE id = ?",
		role, email, telegramID, id)
	if err != nil && isUniqueErr(err) {
		return ErrEmailTaken
	}
	return err
}

// UpdateProfile actualiza el correo y el ID de Telegram de un usuario
// (auto-servicio: no toca el rol).
func (s *Store) UpdateProfile(id int64, email, telegramID string) error {
	_, err := s.db.Exec("UPDATE users SET email = ?, telegram_id = ? WHERE id = ?", email, telegramID, id)
	if err != nil && isUniqueErr(err) {
		return ErrEmailTaken
	}
	return err
}

// UpdateStatusPage actualiza el título y la descripción de la página
// de estado pública del usuario (el slug es su nombre de usuario).
func (s *Store) UpdateStatusPage(id int64, title, desc string) error {
	_, err := s.db.Exec("UPDATE users SET status_title = ?, status_desc = ? WHERE id = ?", title, desc, id)
	return err
}

// CountAdmins devuelve cuántos administradores hay (para proteger
// al último admin de ser degradado o eliminado).
func (s *Store) CountAdmins() (int, error) {
	var n int
	err := s.db.QueryRow("SELECT COUNT(*) FROM users WHERE role = ?", RoleAdmin).Scan(&n)
	return n, err
}

// DeleteUser elimina un usuario y todo lo que le pertenece
// (monitores, heartbeats, notificaciones, sesiones, comparticiones).
func (s *Store) DeleteUser(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmts := []string{
		"DELETE FROM sessions WHERE user_id = ?",
		"DELETE FROM user_groups WHERE user_id = ?",
		"DELETE FROM monitor_shares WHERE user_id = ?",
		"DELETE FROM monitor_shares WHERE monitor_id IN (SELECT id FROM monitors WHERE owner_id = ?)",
		"DELETE FROM monitor_notifiers WHERE monitor_id IN (SELECT id FROM monitors WHERE owner_id = ?)",
		"DELETE FROM heartbeats WHERE monitor_id IN (SELECT id FROM monitors WHERE owner_id = ?)",
		"DELETE FROM monitors WHERE owner_id = ?",
		"DELETE FROM notifications WHERE owner_id = ?",
		"DELETE FROM users WHERE id = ?",
	}
	for _, q := range stmts {
		if _, err := tx.Exec(q, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// --- Sesiones ---

// CreateSession registra un token de sesión con su expiración.
func (s *Store) CreateSession(userID int64, token string, ttl time.Duration) error {
	now := time.Now().UTC()
	_, err := s.db.Exec(
		`INSERT INTO sessions (token, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)`,
		token, userID, now.Format(timeFmt), now.Add(ttl).Format(timeFmt))
	return err
}

// UserForSession devuelve el usuario dueño de un token válido.
// Las sesiones expiradas se eliminan al detectarse.
func (s *Store) UserForSession(token string) (User, error) {
	row := s.db.QueryRow(
		`SELECT u.id, u.username, u.email, u.telegram_id, u.password_hash, u.role, u.status_title, u.status_desc, u.created_at
		 FROM sessions s JOIN users u ON u.id = s.user_id
		 WHERE s.token = ? AND s.expires_at > ?`,
		token, nowStr())
	u, err := scanUser(row)
	if err != nil {
		return User{}, ErrNotFound
	}
	// limpieza perezosa de sesiones expiradas
	_, _ = s.db.Exec("DELETE FROM sessions WHERE expires_at <= ?", nowStr())
	return u, nil
}

func (s *Store) DeleteSession(token string) error {
	_, err := s.db.Exec("DELETE FROM sessions WHERE token = ?", token)
	return err
}

type scanner interface {
	Scan(dest ...any) error
}

func scanUser(row scanner) (User, error) {
	var u User
	var createdAt string
	err := row.Scan(&u.ID, &u.Username, &u.Email, &u.TelegramID, &u.PasswordHash, &u.Role, &u.StatusTitle, &u.StatusDesc, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	u.CreatedAt = parseTime(createdAt)
	return u, nil
}
