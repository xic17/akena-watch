// Package store es la capa de persistencia. SQLite embebida (pure Go,
// sin cgo) para máxima portabilidad: el mismo archivo de base de datos
// funciona en cualquier Linux, en un contenedor o montado sobre R2.
package store

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const timeFmt = time.RFC3339Nano

// Store envuelve la conexión SQLite.
type Store struct {
	db *sql.DB
}

// Open abre (o crea) la base de datos en path y aplica el esquema.
func Open(path string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)",
		filepath.ToSlash(path))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// Una sola conexión: serializa el acceso y evita "database is locked".
	db.SetMaxOpenConns(1)

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

const schema = `
CREATE TABLE IF NOT EXISTS users (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	username      TEXT NOT NULL UNIQUE COLLATE NOCASE,
	email         TEXT NOT NULL DEFAULT '',
	telegram_id   TEXT NOT NULL DEFAULT '',
	password_hash TEXT NOT NULL,
	role          TEXT NOT NULL DEFAULT 'collaborator',
	status_title  TEXT NOT NULL DEFAULT 'Estado de los servicios',
	status_desc   TEXT NOT NULL DEFAULT '',
	created_at    TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
	token      TEXT PRIMARY KEY,
	user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	created_at TEXT NOT NULL,
	expires_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS monitors (
	id               INTEGER PRIMARY KEY AUTOINCREMENT,
	owner_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	name             TEXT NOT NULL,
	group_name       TEXT NOT NULL DEFAULT '',
	type             TEXT NOT NULL,
	url              TEXT NOT NULL,
	method           TEXT NOT NULL DEFAULT 'GET',
	expected_status  INTEGER NOT NULL DEFAULT 200,
	keyword          TEXT NOT NULL DEFAULT '',
	body             TEXT NOT NULL DEFAULT '',
	invert_keyword   INTEGER NOT NULL DEFAULT 0,
	timeout_s        INTEGER NOT NULL DEFAULT 10,
	interval_s       INTEGER NOT NULL DEFAULT 60,
	active           INTEGER NOT NULL DEFAULT 1,
	public           INTEGER NOT NULL DEFAULT 0,
	notify           INTEGER NOT NULL DEFAULT 1,
	notify_owner     INTEGER NOT NULL DEFAULT 0,
	max_retries      INTEGER NOT NULL DEFAULT 1,
	latency_threshold_ms INTEGER NOT NULL DEFAULT 0,
	slow_retries     INTEGER NOT NULL DEFAULT 0,
	cert_alert_days  INTEGER NOT NULL DEFAULT 0,
	created_at       TEXT NOT NULL,
	updated_at       TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS monitor_shares (
	monitor_id INTEGER NOT NULL REFERENCES monitors(id) ON DELETE CASCADE,
	user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	can_edit   INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (monitor_id, user_id)
);

CREATE TABLE IF NOT EXISTS heartbeats (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	monitor_id INTEGER NOT NULL REFERENCES monitors(id) ON DELETE CASCADE,
	status     TEXT NOT NULL,
	code       INTEGER NOT NULL DEFAULT 0,
	latency_ms INTEGER NOT NULL DEFAULT 0,
	error      TEXT NOT NULL DEFAULT '',
	checked_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_heartbeats_monitor ON heartbeats(monitor_id, id DESC);

CREATE TABLE IF NOT EXISTS notifications (
	id       INTEGER PRIMARY KEY AUTOINCREMENT,
	owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	name     TEXT NOT NULL,
	type     TEXT NOT NULL,
	config   TEXT NOT NULL,
	active   INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS monitor_notifiers (
	monitor_id      INTEGER NOT NULL REFERENCES monitors(id) ON DELETE CASCADE,
	notification_id INTEGER NOT NULL REFERENCES notifications(id) ON DELETE CASCADE,
	PRIMARY KEY (monitor_id, notification_id)
);

CREATE TABLE IF NOT EXISTS user_groups (
	user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	group_name TEXT NOT NULL,
	can_edit   INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (user_id, group_name)
);
`

func (s *Store) migrate() error {
	if _, err := s.db.Exec(schema); err != nil {
		return err
	}
	if err := s.migrateMonitorsBody(); err != nil {
		return err
	}
	if err := s.migrateUsersEmail(); err != nil {
		return err
	}
	if err := s.migrateUsersTelegram(); err != nil {
		return err
	}
	if err := s.migrateMonitorsNotifyOwner(); err != nil {
		return err
	}
	if err := s.migrateMonitorsLatency(); err != nil {
		return err
	}
	if err := s.migrateMonitorsCert(); err != nil {
		return err
	}
	return s.migrateMonitorsGroup()
}

// migrateMonitorsGroup añade la columna group_name (categoría) a los
// monitores de bases de datos creadas con esquemas anteriores.
func (s *Store) migrateMonitorsGroup() error {
	return s.migraColumna("monitors", "group_name")
}

// migraColumna añade una columna TEXT con default ” si no existe.
func (s *Store) migraColumna(table, column string) error {
	rows, err := s.db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt any
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			rows.Close()
			return err
		}
		if name == column {
			found = true
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if !found {
		_, err = s.db.Exec("ALTER TABLE " + table + " ADD COLUMN " + column + " TEXT NOT NULL DEFAULT ''")
	}
	return err
}

// migrateUsersEmail añade la columna email y crea el índice único
// (correos sin duplicados; los vacíos no cuentan).
func (s *Store) migrateUsersEmail() error {
	if err := s.migraColumna("users", "email"); err != nil {
		return err
	}
	_, err := s.db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email ON users(email) WHERE email <> ''")
	return err
}

func (s *Store) migrateUsersTelegram() error {
	return s.migraColumna("users", "telegram_id")
}

func (s *Store) migrateMonitorsNotifyOwner() error {
	rows, err := s.db.Query("PRAGMA table_info(monitors)")
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt any
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			rows.Close()
			return err
		}
		if name == "notify_owner" {
			found = true
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if !found {
		_, err = s.db.Exec("ALTER TABLE monitors ADD COLUMN notify_owner INTEGER NOT NULL DEFAULT 0")
	}
	return err
}

// migrateMonitorsBody añade la columna body (cuerpo JSON de los checks
// HTTP) a bases de datos creadas con esquemas anteriores.
func (s *Store) migrateMonitorsBody() error {
	rows, err := s.db.Query("PRAGMA table_info(monitors)")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt any
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return err
		}
		if name == "body" {
			return nil // ya migrada
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = s.db.Exec("ALTER TABLE monitors ADD COLUMN body TEXT NOT NULL DEFAULT ''")
	return err
}

// migrateMonitorsLatency añade el umbral de lentitud y los reintentos de
// lentitud (0 = desactivado) a bases de datos creadas antes de esta función.
func (s *Store) migrateMonitorsLatency() error {
	for _, col := range []struct {
		name string
		def  string
	}{
		{"latency_threshold_ms", "0"},
		{"slow_retries", "0"},
	} {
		rows, err := s.db.Query("PRAGMA table_info(monitors)")
		if err != nil {
			return err
		}
		found := false
		for rows.Next() {
			var cid, notnull, pk int
			var name, ctype string
			var dflt any
			if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
				rows.Close()
				return err
			}
			if name == col.name {
				found = true
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if !found {
			if _, err := s.db.Exec("ALTER TABLE monitors ADD COLUMN " + col.name + " INTEGER NOT NULL DEFAULT " + col.def); err != nil {
				return err
			}
		}
	}
	return nil
}

// migrateMonitorsCert añade la columna cert_alert_days (aviso de expiración
// del certificado TLS, 0 = desactivado) a bases de datos previas.
func (s *Store) migrateMonitorsCert() error {
	rows, err := s.db.Query("PRAGMA table_info(monitors)")
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt any
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			rows.Close()
			return err
		}
		if name == "cert_alert_days" {
			found = true
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if !found {
		_, err = s.db.Exec("ALTER TABLE monitors ADD COLUMN cert_alert_days INTEGER NOT NULL DEFAULT 0")
	}
	return err
}

func nowStr() string { return time.Now().UTC().Format(timeFmt) }

func parseTime(s string) time.Time {
	t, err := time.Parse(timeFmt, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

func isUniqueErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE")
}
