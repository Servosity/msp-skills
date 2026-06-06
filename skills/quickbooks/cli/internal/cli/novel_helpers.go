// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"fmt"

	"quickbooks-pp-cli/internal/store"
)

// openLocalStore opens the synced SQLite store the transcendence commands read.
// OpenWithContext creates+migrates an empty store if none exists, so analytics on
// an unsynced store return honest-empty results rather than an error.
func openLocalStore(ctx context.Context, dbPath string) (*store.Store, error) {
	if dbPath == "" {
		dbPath = defaultDBPath("quickbooks-cli")
	}
	db, err := store.OpenWithContext(ctx, dbPath)
	if err != nil {
		return nil, configErr(fmt.Errorf("opening local store at %s: %w — run `quickbooks-cli sync` first", dbPath, err))
	}
	return db, nil
}

// loadResources returns every synced row for a resource_type, decoded into
// json.Number-safe maps (store.DecodeJSONObject uses UseNumber). It queries the
// resources table directly rather than store.List, which caps at 200 rows —
// analytics must see the full set.
func loadResources(ctx context.Context, db *store.Store, resourceType string) ([]map[string]any, error) {
	rows, err := db.DB().QueryContext(ctx, `SELECT data FROM resources WHERE resource_type = ?`, resourceType)
	if err != nil {
		return nil, apiErr(fmt.Errorf("querying %s from local store: %w", resourceType, err))
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		obj, derr := store.DecodeJSONObject(json.RawMessage(data))
		if derr != nil {
			continue
		}
		out = append(out, obj)
	}
	return out, rows.Err()
}
