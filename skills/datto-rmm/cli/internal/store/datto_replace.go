// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import "encoding/json"

// ReplaceResource writes data as the whole record (handfix
// refreshed-collection-replaces-inventory, Codex review r2 class refreshed
// collection keeps stale inventory). Upsert's keep-richer merge is right when a
// list row meets a richer detail row of one object; for a live per-device
// collection it is wrong: an empty or shorter list is the current truth, and the
// merge kept what the device no longer has. Credential fields stay out as on
// every other write.
func (s *Store) ReplaceResource(resourceType, id string, data json.RawMessage) error {
	s.lockForWrite()
	defer s.unlockAfterWrite()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := s.upsertGenericResourceTx(tx, resourceType, id, withoutStoredSecrets(data)); err != nil {
		return err
	}
	return tx.Commit()
}
