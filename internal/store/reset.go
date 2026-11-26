package store

// ReiniciarTodo borra todos los datos de la aplicación: usuarios, monitores,
// canales, historial y sesiones. Lo usa el modo demo para devolver la instancia
// a un estado limpio.
//
// Las claves foráneas van en cascada (monitores, canales, sesiones, historial y
// comparticiones cuelgan de users), así que borrar los usuarios ya arrastra el
// resto; el bucle es una red de seguridad para datos huérfanos.
func (s *Store) ReiniciarTodo() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, q := range []string{
		"DELETE FROM users",
		"DELETE FROM monitors",
		"DELETE FROM notifications",
		"DELETE FROM heartbeats",
		"DELETE FROM sessions",
		"DELETE FROM monitor_shares",
		"DELETE FROM monitor_notifiers",
		"DELETE FROM user_groups",
	} {
		if _, err := tx.Exec(q); err != nil {
			return err
		}
	}
	// Que los identificadores vuelvan a empezar. La tabla solo existe si alguna
	// vez se insertó algo; si no está, el error no importa.
	_, _ = tx.Exec("DELETE FROM sqlite_sequence")

	return tx.Commit()
}
