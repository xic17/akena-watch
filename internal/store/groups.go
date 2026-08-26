package store

// GroupCount es un grupo de monitores con su cantidad de monitores.
type GroupCount struct {
	Name  string
	Count int
}

// GetUserGroups devuelve los grupos a los que un usuario tiene acceso.
func (s *Store) GetUserGroups(userID int64) ([]string, error) {
	rows, err := s.db.Query(
		`SELECT group_name FROM user_groups WHERE user_id = ? ORDER BY group_name COLLATE NOCASE`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var g string
		if err := rows.Scan(&g); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// SetUserGroups reemplaza los grupos a los que un usuario tiene acceso.
func (s *Store) SetUserGroups(userID int64, groups []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM user_groups WHERE user_id = ?", userID); err != nil {
		return err
	}
	for _, g := range groups {
		if _, err := tx.Exec(
			"INSERT OR IGNORE INTO user_groups (user_id, group_name) VALUES (?, ?)",
			userID, g); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListGroups devuelve los grupos existentes (de todos los monitores) con
// su cantidad de monitores.
func (s *Store) ListGroups() ([]GroupCount, error) {
	rows, err := s.db.Query(
		`SELECT group_name, COUNT(*) FROM monitors
		 WHERE group_name <> '' GROUP BY group_name ORDER BY group_name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []GroupCount
	for rows.Next() {
		var g GroupCount
		if err := rows.Scan(&g.Name, &g.Count); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// ListUserManualAccess devuelve los monitores asignados a mano a un
// usuario (comparticiones de solo lectura can_edit = 0).
func (s *Store) ListUserManualAccess(userID int64) ([]int64, error) {
	rows, err := s.db.Query(
		`SELECT monitor_id FROM monitor_shares WHERE user_id = ? AND can_edit = 0 ORDER BY monitor_id`,
		userID)
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

// SetUserManualAccess reemplaza los monitores asignados a mano a un
// usuario. No toca las comparticiones con edición (can_edit = 1).
func (s *Store) SetUserManualAccess(userID int64, monitorIDs []int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM monitor_shares WHERE user_id = ? AND can_edit = 0", userID); err != nil {
		return err
	}
	for _, id := range monitorIDs {
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO monitor_shares (monitor_id, user_id, can_edit) VALUES (?, ?, 0)`,
			id, userID); err != nil {
			return err
		}
	}
	return tx.Commit()
}
