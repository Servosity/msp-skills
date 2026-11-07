// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

// microsoft-graph handfix (pull-replaces-snapshot): a completed `pull` is the
// tenant's state for a resource, so it replaces what the store held. The
// typed upserts merge keep-richer (an incoming empty array keeps the stored
// one) and never delete, which kept a removed license, an emptied group, a
// revoked role membership and a deleted user as current (Codex r1 classes
// stale-snapshot-kept). ReplaceResources writes exactly the rows Graph
// returned, in one transaction, so a failed write leaves the previous snapshot
// whole. Every statement passes the credential kit's SQL driver
// (internal/credsql), as every other store write does.

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

type typedUpsertTx func(*Store, *sql.Tx, string, map[string]any, json.RawMessage) error

var replaceableResources = map[string]struct {
	clear  string // a literal per table: no SQL is built from a value
	upsert typedUpsertTx
}{
	"users":           {`DELETE FROM "users"`, (*Store).upsertUsersTx},
	"groups":          {`DELETE FROM "groups"`, (*Store).upsertGroupsTx},
	"directory-roles": {`DELETE FROM "directory_roles"`, (*Store).upsertDirectoryRolesTx},
	"security":        {`DELETE FROM "security"`, (*Store).upsertSecurityTx},
	"licenses":        {`DELETE FROM "licenses"`, (*Store).upsertLicensesTx},
	"devices":         {`DELETE FROM "devices"`, (*Store).upsertDevicesTx},
	"managed-devices": {`DELETE FROM "managed_devices"`, (*Store).upsertManagedDevicesTx},
	"phone-numbers":   {`DELETE FROM "phone_numbers"`, (*Store).upsertPhoneNumbersTx},
	"teams-users":     {`DELETE FROM "teams_users"`, (*Store).upsertTeamsUsersTx},
}

// ReplaceResources makes items the whole stored set of resourceType.
func (s *Store) ReplaceResources(resourceType string, items []json.RawMessage) error {
	typed, ok := replaceableResources[resourceType]
	if !ok {
		return fmt.Errorf("no typed table for %s", resourceType)
	}
	s.lockForWrite()
	defer s.unlockAfterWrite()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	rows, err := tx.Query(`SELECT id FROM resources WHERE resource_type = ?`, resourceType)
	if err != nil {
		return err
	}
	var old []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		old = append(old, id)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range old {
		if _, err := tx.Exec(`DELETE FROM resources_fts WHERE rowid = ?`, ftsRowID(resourceType, id)); err != nil {
			return fmt.Errorf("clearing the %s search index: %w", resourceType, err)
		}
	}
	if _, err := tx.Exec(`DELETE FROM resources WHERE resource_type = ?`, resourceType); err != nil {
		return err
	}
	if _, err := tx.Exec(typed.clear); err != nil {
		return err
	}

	for _, raw := range items {
		obj, err := DecodeJSONObject(raw)
		if err != nil {
			return fmt.Errorf("unmarshaling %s: %w", resourceType, err)
		}
		id := ResolveStorageID(resourceType, obj)
		if id == "" {
			return fmt.Errorf("missing id for %s", resourceType)
		}
		storageID := resourceStorageID(resourceType, id, obj)
		data := raw
		if err := s.upsertGenericResourceTx(tx, resourceType, storageID, data); err != nil {
			return err
		}
		if err := typed.upsert(s, tx, storageID, obj, data); err != nil {
			return err
		}
	}
	return tx.Commit()
}
