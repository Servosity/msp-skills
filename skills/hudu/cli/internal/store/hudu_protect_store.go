// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-fix (handfixes.json vault-secrets-withheld): the store side of the
// Hudu secret rule in internal/huduprotect. Every database write of a resource
// row goes through mergeIncomingResourceData (the generic Upsert, every typed
// Upsert<X> and UpsertBatchDetailed call it before the generic and typed
// writers), and that funnel stores scrubProtectedResourceData of the MERGED
// row. Scrubbing the merge result, not just the incoming row, matters: the
// keep-richer merge keeps keys the incoming row lacks and keeps an old value
// where the incoming one is empty, so a password written by an older binary
// would otherwise survive every later sync.
//
// Rows already on disk are scrubbed once, when a store below
// protectedRowsScrubSchemaVersion is opened read-write (migrate), with
// secure_delete on so freed pages are zeroed and the full-text index rebuilt
// for every rewritten row. A read-only open of such a store upgrades it first
// (readOnlyStoreNeedsScrub, upgradeUnscrubbedStoreBeforeRead).

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"hudu-pp-cli/internal/huduprotect"
)

// protectedRowsScrubSchemaVersion pins the one-time scrub of stored vault and
// asset rows. It stays at the bump that introduced the scrub so later
// additive migrations do not rewrite those rows again.
const protectedRowsScrubSchemaVersion = 14

// rowQuery runs a single-row query on whichever handle the caller holds (the
// funnel's transaction or the migration connection).
type rowQuery func(query string, args ...any) *sql.Row

// scrubProtectedResourceData applies the Hudu secret rule to one stored row.
// A resource type that names neither the vault nor assets passes unchanged,
// byte for byte.
func scrubProtectedResourceData(layouts huduprotect.LayoutLookup, resourceType string, data json.RawMessage) json.RawMessage {
	switch huduprotect.ClassifyStoreResourceType(resourceType) {
	case huduprotect.Vault:
		return huduprotect.ScrubVaultDocument(data)
	case huduprotect.Asset:
		return huduprotect.ScrubAssetDocument(data, layouts)
	default:
		return data
	}
}

// scrubLegacyResourceData is the one-time upgrade's rule for a row written by
// an older binary: a protected type gets the protected rule; any other type
// gets huduprotect.ScrubUnknownProvenanceDocument, because an older binary
// recorded no answered URL and may have stored a redirect's vault or asset
// answer under that type.
func scrubLegacyResourceData(layouts huduprotect.LayoutLookup, resourceType string, data json.RawMessage) json.RawMessage {
	if huduprotect.ClassifyStoreResourceType(resourceType) != huduprotect.None {
		return scrubProtectedResourceData(layouts, resourceType, data)
	}
	return huduprotect.ScrubUnknownProvenanceDocument(data, layouts)
}

// storeLayoutLookup reads an asset layout's field types from the same store:
// the generic row (resource type asset-layouts, or its legacy snake_case
// name) or the typed asset_layouts table. A layout that is not stored, or does
// not parse, reports ok=false and the asset rule then treats the field as
// secret; the next sync refreshes the row through the client rule.
func storeLayoutLookup(query rowQuery) huduprotect.LayoutLookup {
	type result struct {
		types map[string]string
		ok    bool
	}
	memo := map[string]result{}
	return func(layoutID string) (map[string]string, bool) {
		if query == nil || layoutID == "" {
			return nil, false
		}
		if r, seen := memo[layoutID]; seen {
			return r.types, r.ok
		}
		var r result
		for _, q := range []struct {
			sql  string
			args []any
		}{
			{`SELECT data FROM resources WHERE resource_type = ? AND id = ?`, []any{"asset-layouts", layoutID}},
			{`SELECT data FROM resources WHERE resource_type = ? AND id = ?`, []any{"asset_layouts", layoutID}},
			{`SELECT data FROM "asset_layouts" WHERE id = ?`, []any{layoutID}},
		} {
			var data string
			if err := query(q.sql, q.args...).Scan(&data); err != nil {
				continue
			}
			if types, ok := huduprotect.ParseLayoutFieldTypes(json.RawMessage(data)); ok {
				r = result{types: types, ok: true}
				break
			}
		}
		memo[layoutID] = r
		return r.types, r.ok
	}
}

// migrateScrubProtectedRows rewrites every stored vault and asset row through
// the same scrub as the write funnel, and every other stored row through
// huduprotect.ScrubUnknownProvenanceDocument (scrubLegacyResourceData): every
// generic resources row and the data column of every typed table. Rows are
// visited by SQLite rowid, so a row whose id or type is NULL (a legacy typed
// table allows it) is scrubbed too. The scrub re-encodes each row from its
// scrubbed tree; a row of an unprotected type whose stored bytes equal that
// encoding is not rewritten, and every other row is rewritten with its search
// index entry rebuilt. An index entry no stored row owns is dropped. It runs
// inside migrate's transaction and reports whether it rewrote anything, so
// migrate can checkpoint the WAL afterwards.
func (s *Store) migrateScrubProtectedRows(ctx context.Context, conn *sql.Conn) (bool, error) {
	if _, err := conn.ExecContext(ctx, `PRAGMA secure_delete = ON`); err != nil {
		return false, fmt.Errorf("enabling secure_delete: %w", err)
	}
	layouts := storeLayoutLookup(func(q string, args ...any) *sql.Row { return conn.QueryRowContext(ctx, q, args...) })

	type storedRow struct {
		rowid        int64
		resourceType sql.NullString
		id           sql.NullString
		data         sql.NullString
	}
	// readRows reads every row of a table by rowid; typeExpr yields each
	// row's resource type.
	readRows := func(quoted, typeExpr string) ([]storedRow, error) {
		rows, err := conn.QueryContext(ctx, fmt.Sprintf(`SELECT rowid, %s, id, data FROM %s`, typeExpr, quoted))
		if err != nil {
			return nil, fmt.Errorf("reading %s rows: %w", quoted, err)
		}
		defer rows.Close()
		var out []storedRow
		for rows.Next() {
			var r storedRow
			if err := rows.Scan(&r.rowid, &r.resourceType, &r.id, &r.data); err != nil {
				return nil, fmt.Errorf("reading %s rows: %w", quoted, err)
			}
			out = append(out, r)
		}
		return out, rows.Err()
	}
	// scrubRow returns the row's scrubbed data and whether it must be
	// rewritten. A NULL data column holds nothing to scrub.
	scrubRow := func(r storedRow) (string, bool) {
		if !r.data.Valid {
			return "", false
		}
		scrubbed := string(scrubLegacyResourceData(layouts, r.resourceType.String, json.RawMessage(r.data.String)))
		protected := huduprotect.ClassifyStoreResourceType(r.resourceType.String) != huduprotect.None
		return scrubbed, protected || scrubbed != r.data.String
	}

	rewrote := false
	resourceRows, err := readRows(`resources`, `resource_type`)
	if err != nil {
		return false, err
	}
	indexed := map[int64]bool{}
	for _, r := range resourceRows {
		keyed := r.resourceType.Valid && r.id.Valid
		ftsRow := ftsRowID(r.resourceType.String, r.id.String)
		if keyed {
			indexed[ftsRow] = true
		}
		scrubbed, rewrite := scrubRow(r)
		if !rewrite {
			continue
		}
		if _, err := conn.ExecContext(ctx, `UPDATE resources SET data = ? WHERE rowid = ?`, scrubbed, r.rowid); err != nil {
			return false, fmt.Errorf("rewriting resources row %d: %w", r.rowid, err)
		}
		rewrote = true
		if !keyed {
			continue
		}
		if _, err := conn.ExecContext(ctx, `DELETE FROM resources_fts WHERE rowid = ?`, ftsRow); err != nil {
			return false, fmt.Errorf("clearing search index for %s/%s: %w", r.resourceType.String, r.id.String, err)
		}
		if _, err := conn.ExecContext(ctx,
			`INSERT INTO resources_fts (rowid, id, resource_type, content) VALUES (?, ?, ?, ?)`,
			ftsRow, r.id.String, r.resourceType.String, searchableResourceContent(json.RawMessage(scrubbed)),
		); err != nil {
			return false, fmt.Errorf("re-indexing %s/%s: %w", r.resourceType.String, r.id.String, err)
		}
	}

	// An index entry that no stored row owns any more (a renamed partition, a
	// re-keyed id) still holds the text it was built from, and an older binary
	// may have indexed a redirect's vault or asset answer under any type, so
	// every such entry is dropped. Every index write keys its entry
	// ftsRowID(resource_type, id) of a stored row, and the rowid migrations
	// above have already run, so a row's own entry is never taken for one.
	ftsRows, err := conn.QueryContext(ctx, `SELECT rowid FROM resources_fts`)
	if err != nil {
		return false, fmt.Errorf("reading search index: %w", err)
	}
	var orphans []int64
	for ftsRows.Next() {
		var rowid int64
		if err := ftsRows.Scan(&rowid); err != nil {
			_ = ftsRows.Close()
			return false, fmt.Errorf("reading search index: %w", err)
		}
		if !indexed[rowid] {
			orphans = append(orphans, rowid)
		}
	}
	if err := ftsRows.Err(); err != nil {
		_ = ftsRows.Close()
		return false, fmt.Errorf("reading search index: %w", err)
	}
	if err := ftsRows.Close(); err != nil {
		return false, fmt.Errorf("reading search index: %w", err)
	}
	for _, rowid := range orphans {
		if _, err := conn.ExecContext(ctx, `DELETE FROM resources_fts WHERE rowid = ?`, rowid); err != nil {
			return false, fmt.Errorf("dropping orphan search entry: %w", err)
		}
		rewrote = true
	}

	typedTables, err := typedDataTables(ctx, conn)
	if err != nil {
		return false, err
	}
	for _, table := range typedTables {
		// A typed table is named for its resource type and has no type column.
		quoted := `"` + strings.ReplaceAll(table, `"`, `""`) + `"`
		rows, err := readRows(quoted, `'`+strings.ReplaceAll(table, `'`, `''`)+`'`)
		if err != nil {
			return false, err
		}
		for _, r := range rows {
			scrubbed, rewrite := scrubRow(r)
			if !rewrite {
				continue
			}
			if _, err := conn.ExecContext(ctx, fmt.Sprintf(`UPDATE %s SET data = ? WHERE rowid = ?`, quoted), scrubbed, r.rowid); err != nil {
				return false, fmt.Errorf("rewriting %s row %d: %w", table, r.rowid, err)
			}
			rewrote = true
		}
	}

	if rewrote {
		if _, err := conn.ExecContext(ctx, `INSERT INTO resources_fts(resources_fts) VALUES('optimize')`); err != nil {
			return false, fmt.Errorf("optimizing search index: %w", err)
		}
	}
	return rewrote, nil
}

// typedDataTables lists every typed table that mirrors resource rows: a table
// with an id and a data column, other than the generic resources table.
func typedDataTables(ctx context.Context, conn *sql.Conn) ([]string, error) {
	rows, err := conn.QueryContext(ctx, `SELECT m.name FROM sqlite_master m
		WHERE m.type = 'table' AND m.name <> 'resources'
		AND EXISTS (SELECT 1 FROM pragma_table_info(m.name) WHERE name = 'id')
		AND EXISTS (SELECT 1 FROM pragma_table_info(m.name) WHERE name = 'data')`)
	if err != nil {
		return nil, fmt.Errorf("listing typed tables: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

// checkpointAfterScrub truncates the WAL after the scrub committed, so the
// pre-scrub page images it held are gone. Best-effort: a reader holding an
// old snapshot can keep the checkpoint from completing.
func checkpointAfterScrub(ctx context.Context, conn *sql.Conn) {
	_, _ = conn.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`)
}

// readOnlyStoreNeedsScrub reports whether a store opened read-only may still
// hold vault secrets or password-type asset field values written by an older
// binary; a read-only handle can never rewrite them, so MCP `sql` and `search`
// would read them. It fails closed: an error reading the stamp (SQLITE_BUSY
// from a concurrent writer included) is returned to the caller, never read as
// "already scrubbed". A store stamped below protectedRowsScrubSchemaVersion
// needs the scrub, and so does an unstamped one (user_version 0) holding the
// search index or any table with a data column (where every stored body
// lives; typed columns hold only metadata), since older binaries left
// existing stores unstamped.
func readOnlyStoreNeedsScrub(ctx context.Context, db *sql.DB) (bool, error) {
	var current int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&current); err != nil {
		return false, fmt.Errorf("could not confirm the local store had the password-vault and asset-field scrub, so it was not read: %w", err)
	}
	if current >= protectedRowsScrubSchemaVersion {
		return false, nil
	}
	if current >= 1 {
		return true, nil
	}
	var tables int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master m WHERE m.type = 'table'
		AND (m.name = 'resources_fts' OR EXISTS (SELECT 1 FROM pragma_table_info(m.name) WHERE name = 'data'))`).Scan(&tables); err != nil {
		return false, fmt.Errorf("could not confirm the local store had the password-vault and asset-field scrub, so it was not read: %w", err)
	}
	return tables > 0, nil
}

// readOnlyScrubUpgraded marks the one read-only reopen after an upgrade, so a
// store that still reads as unscrubbed is refused instead of upgraded again.
type readOnlyScrubUpgraded struct{}

// upgradeUnscrubbedStoreBeforeRead opens the store read-write once, which
// migrates and scrubs it, and confirms the stamp. If that fails the read fails
// closed rather than serving the unscrubbed rows.
func upgradeUnscrubbedStoreBeforeRead(ctx context.Context, dbPath string) error {
	s, err := OpenWithContext(ctx, dbPath)
	if err != nil {
		return fmt.Errorf("the local store predates the password-vault and asset-field scrub and could not be upgraded (run any hudu-cli command that writes, such as sync, with write access to it): %w", err)
	}
	defer s.Close()
	var current int
	if err := s.db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&current); err != nil {
		return fmt.Errorf("confirming the password-vault and asset-field scrub: %w", err)
	}
	if current < protectedRowsScrubSchemaVersion {
		return fmt.Errorf("the local store is still at schema version %d after the password-vault and asset-field scrub", current)
	}
	return nil
}
