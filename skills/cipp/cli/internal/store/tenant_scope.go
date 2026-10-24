// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"encoding/json"
	"fmt"
	"sync"
)

// TenantKeyField is the field fanout --save stamps on every row with the
// tenant it came from.
const TenantKeyField = "_tenant"

var tenantScopeMu sync.Mutex

// TenantScopeResource keys resourceType's stored rows on (id, _tenant)
// instead of id alone. CIPP is tenant-scoped end to end, and different tenants
// return rows that share an id (a license SKU, a role template, a standard
// template), so a plain (resource_type, id) key lets the last tenant saved
// overwrite every other tenant's row. Registering _tenant as the parent key
// reuses the store's own composite-id path (resourceStorageID), so readers and
// the migration on open behave exactly as for a generated dependent resource.
func TenantScopeResource(resourceType string) {
	tenantScopeMu.Lock()
	defer tenantScopeMu.Unlock()
	for _, k := range resourceParentKeyColumns[resourceType] {
		if k == TenantKeyField {
			return
		}
	}
	resourceParentKeyColumns[resourceType] = append(resourceParentKeyColumns[resourceType], TenantKeyField)
}

// ReplaceTenantRows makes items the complete stored set of resourceType for
// tenant, in ONE transaction: the tenant's previous rows (and their FTS
// entries) are deleted and items inserted with UpsertBatch's id resolution.
// A failed insert rolls the delete back, so a write error never destroys the
// data that was there. fanout calls it with each tenant's fresh response, so
// a resource deleted upstream stops counting in fleet reports instead of
// lingering in an upsert-only table. Returns the number of rows stored.
func (s *Store) ReplaceTenantRows(resourceType, tenant string, items []json.RawMessage) (int, error) {
	// A complete set is rows, every one of them: an item that is not a JSON
	// object (a CIPP failure string such as ["Failed to retrieve policies"])
	// means the response is not the tenant's data, so nothing is replaced.
	for _, item := range items {
		if _, err := DecodeJSONObject(item); err != nil {
			return 0, fmt.Errorf("refusing to replace %s rows for %s: the response holds a non-row item %s", resourceType, tenant, truncateItem(item))
		}
	}
	s.lockForWrite()
	defer s.unlockAfterWrite()
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT id FROM resources WHERE resource_type = ? AND json_extract(data, '$._tenant') = ?`, resourceType, tenant)
	if err != nil {
		return 0, err
	}
	var old []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return 0, err
		}
		old = append(old, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, err
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	for _, id := range old {
		if _, err := tx.Exec(`DELETE FROM resources WHERE resource_type = ? AND id = ?`, resourceType, id); err != nil {
			return 0, err
		}
		// FTS cleanup is best-effort, matching upsertGenericResourceTx.
		_, _ = tx.Exec(`DELETE FROM resources_fts WHERE rowid = ?`, ftsRowID(resourceType, id))
	}
	stored := 0
	for _, item := range items {
		obj, err := DecodeJSONObject(item)
		if err != nil {
			continue
		}
		id := ExtractResourceID(resourceType, obj)
		if id == "" {
			if keyObj, rowObj, rowItem, ok := unwrapIDBearingEnvelopeItem(resourceType, item, obj); ok {
				id = ExtractResourceID(resourceType, keyObj)
				obj = rowObj
				item = rowItem
			}
		}
		if id == "" {
			id = ResolveStorageID(resourceType, obj)
		}
		if id == "" {
			continue
		}
		if err := s.upsertGenericResourceTx(tx, resourceType, resourceStorageID(resourceType, id, obj), item); err != nil {
			return 0, err
		}
		stored++
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return stored, nil
}

// truncateItem shows at most 200 bytes of a refused item.
func truncateItem(item json.RawMessage) string {
	if len(item) > 200 {
		return string(item[:200]) + "..."
	}
	return string(item)
}
