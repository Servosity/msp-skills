// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
package store

// ResetAcronisMirror removes the previous snapshot before an unfiltered full
// enumeration. The CLI marks the mirror incomplete first; a failed refresh
// cannot expose the remaining subset as a complete fleet.
func (s *Store) ResetAcronisMirror() error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Fixed connector-owned tables only; no caller-controlled SQL identifiers.
	for _, statement := range []string{
		`DELETE FROM users`, `DELETE FROM usages`, `DELETE FROM offering_items`,
		`DELETE FROM agent_manager`, `DELETE FROM task_manager`, `DELETE FROM tenants`,
		`DELETE FROM clients`,
	} {
		if _, err := tx.Exec(statement); err != nil {
			return err
		}
	}
	types := []string{"users", "usages", "offering_items", "agent-manager", "task-manager", "task-manager-v2-activities", "tenants", "clients"}
	for _, resource := range types {
		rows, err := tx.Query(`SELECT id FROM resources WHERE resource_type=?`, resource)
		if err != nil {
			return err
		}
		var ids []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				_ = rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		err = rows.Err()
		closeErr := rows.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			return err
		}
		for _, id := range ids {
			if _, err := tx.Exec(`DELETE FROM resources_fts WHERE rowid=?`, ftsRowID(resource, id)); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(`DELETE FROM resources WHERE resource_type=?`, resource); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM sync_state WHERE resource_type=?`, resource); err != nil {
			return err
		}
	}
	return tx.Commit()
}
