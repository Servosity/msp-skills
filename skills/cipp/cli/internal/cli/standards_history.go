// Copyright 2026 damienstevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"fmt"

	"cipp-pp-cli/internal/store"
)

// Standards drift needs an append-only history. The generic resources table
// upserts on (resource_type, id), so a second `fanout --endpoint /ListStandards
// --save` OVERWRITES each standard's row in place instead of adding a second
// snapshot — collapsing the history `standards drift` diffs. This table keeps
// one row per (snapshot_ts, tenant, standard) so two saves at different times
// produce two comparable snapshots.

const standardsSnapshotsSchema = `
CREATE TABLE IF NOT EXISTS standards_snapshots (
	snapshot_ts TEXT NOT NULL,
	tenant      TEXT NOT NULL,
	standard    TEXT NOT NULL,
	state       TEXT NOT NULL DEFAULT '',
	data        TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_standards_snapshots_tenant_ts
	ON standards_snapshots(tenant, snapshot_ts);
`

// ensureStandardsSnapshotsTable lazily creates the history table. Invoked from
// the novel commands that need it, per the hand-edit durability pattern (the
// generated store migrations are not edited).
func ensureStandardsSnapshotsTable(ctx context.Context, db *store.Store) error {
	_, err := db.DB().ExecContext(ctx, standardsSnapshotsSchema)
	return err
}

// appendStandardsSnapshot records one fanout run's standards rows for a tenant
// under a shared snapshot timestamp. Rows with no resolvable standard name are
// skipped, matching loadStandardsSnapshots semantics.
func appendStandardsSnapshot(ctx context.Context, db *store.Store, snapshotTS, tenant string, items []json.RawMessage) error {
	if len(items) == 0 {
		return nil
	}
	if err := ensureStandardsSnapshotsTable(ctx, db); err != nil {
		return fmt.Errorf("creating standards_snapshots table: %w", err)
	}
	tx, err := db.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO standards_snapshots (snapshot_ts, tenant, standard, state, data) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, raw := range items {
		var obj map[string]any
		if json.Unmarshal(raw, &obj) != nil {
			continue
		}
		name := standardName(obj)
		if name == "" {
			continue
		}
		if _, err := stmt.ExecContext(ctx, snapshotTS, tenant, name, standardState(obj), string(raw)); err != nil {
			return err
		}
	}
	return tx.Commit()
}
