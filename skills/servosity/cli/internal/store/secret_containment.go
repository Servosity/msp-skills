// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/servosity/handfixes.json under
// the "secret-family-never-persisted" entry. See docs/reprint-survival.md.

package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"servosity-msp-pp-cli/internal/cliutil"
)

// sensitiveFamilyResourceTypes are sync resources whose whole payload is a
// credential: backup-account MFA codes, the user's MFA backup codes and the
// MFA-enrollment secret. The local store never keeps them (stored credentials
// are kept with their passwords removed, see cliutil.IsSecretFieldName);
// current-user-api-token is handled by sensitiveMirroredResource.
var sensitiveFamilyResourceTypes = map[string]bool{
	"backups-mfa-codes":             true,
	"current-user-mfa-backup-codes": true,
	"current-user-start-mfa":        true,
}

// Secret fields are recognised by cliutil.IsSecretFieldName on the
// normalized key (lower case, separators removed), so the API's snake_case
// fields (password, postmark_server_api_token, agent_provision_token_id,
// product_key) and its PascalCase backup-set fields (LanPassword,
// ApplicationSettings.EncryptionPassword) match alike. They are dropped from
// every row before it is written, so neither the resources table, a typed
// table's columns, nor any FTS index carries them.

// sensitiveFamilyPurgeSchemaVersion: every store a released build wrote is
// below this version (0.6.10 stamped 10), and the read-only opens behind MCP
// search/sql and --data-source local refuse a store below StoreSchemaVersion,
// so each one is purged on its first read-write open before any reader sees it.
const sensitiveFamilyPurgeSchemaVersion = 13

// scrubPersistedSecrets returns data without any secret-named key.
// Data that is not JSON, or carries no such key, is returned unchanged.
func scrubPersistedSecrets(data json.RawMessage) json.RawMessage {
	if !cliutil.MayCarrySecretField(data) {
		return data
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if dec.Decode(&v) != nil {
		return data
	}
	if !dropSecretFields(v) {
		return data
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if enc.Encode(v) != nil {
		return data
	}
	return json.RawMessage(bytes.TrimRight(buf.Bytes(), "\n"))
}

func dropSecretFields(v any) bool {
	changed := false
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			if cliutil.IsSecretFieldName(k) {
				delete(t, k)
				changed = true
				continue
			}
			if dropSecretFields(child) {
				changed = true
			}
		}
	case []any:
		for _, child := range t {
			if dropSecretFields(child) {
				changed = true
			}
		}
	}
	return changed
}

// migratePurgeSecretFamily removes what older builds mirrored: every row of a
// sensitiveFamilyResourceTypes type, every secret-named column value,
// and the secret keys inside every stored data document. When it removed
// anything it rebuilds resources_fts so no index row keeps a removed value.
func migratePurgeSecretFamily(ctx context.Context, conn *sql.Conn) error {
	// Zero the pages the purge frees, so a removed value does not stay in the
	// file's free pages; the connection's own setting is restored after.
	var prevSecureDelete int
	if err := conn.QueryRowContext(ctx, `PRAGMA secure_delete`).Scan(&prevSecureDelete); err == nil {
		if _, err := conn.ExecContext(ctx, `PRAGMA secure_delete = ON`); err == nil {
			defer func() {
				_, _ = conn.ExecContext(context.WithoutCancel(ctx), fmt.Sprintf(`PRAGMA secure_delete = %d`, prevSecureDelete))
			}()
		}
	}
	changed := false
	for t := range sensitiveFamilyResourceTypes {
		res, err := conn.ExecContext(ctx, `DELETE FROM resources WHERE resource_type = ?`, t)
		if err != nil {
			return fmt.Errorf("deleting sensitive resource rows: %w", err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			changed = true
		}
	}
	if ok, err := tableHasColumn(ctx, conn, "sync_state", "resource_type"); err != nil {
		return err
	} else if ok {
		for t := range sensitiveFamilyResourceTypes {
			if _, err := conn.ExecContext(ctx, `DELETE FROM sync_state WHERE resource_type = ?`, t); err != nil {
				return fmt.Errorf("deleting sensitive sync state: %w", err)
			}
		}
	}
	tables, err := secretScrubTables(ctx, conn)
	if err != nil {
		return err
	}
	for _, table := range tables {
		scrubbed, err := scrubSecretTable(ctx, conn, table)
		if err != nil {
			return err
		}
		changed = changed || scrubbed
	}
	if !changed {
		return nil
	}
	if _, err := conn.ExecContext(ctx, `DROP TABLE IF EXISTS resources_fts`); err != nil {
		return fmt.Errorf("dropping resources_fts: %w", err)
	}
	if _, err := conn.ExecContext(ctx, resourcesFTSCreateSQL); err != nil {
		return fmt.Errorf("creating resources_fts: %w", err)
	}
	if err := rebuildResourcesFTS(ctx, conn); err != nil {
		return fmt.Errorf("rebuilding resources_fts: %w", err)
	}
	return nil
}

// secretScrubTables lists the ordinary tables (not FTS shadow or virtual
// tables) that hold API data: every table with a data column or a column named
// like a secret field.
func secretScrubTables(ctx context.Context, conn *sql.Conn) ([]string, error) {
	rows, err := conn.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' AND sql NOT LIKE 'CREATE VIRTUAL TABLE%'`)
	if err != nil {
		return nil, fmt.Errorf("listing tables: %w", err)
	}
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			_ = rows.Close()
			return nil, err
		}
		names = append(names, name)
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var out []string
	for _, name := range names {
		if strings.Contains(name, "_fts_") || strings.HasSuffix(name, "_fts") {
			continue
		}
		cols, err := tableColumns(ctx, conn, name)
		if err != nil {
			return nil, err
		}
		for _, c := range cols {
			if c == "data" || cliutil.IsSecretFieldName(c) {
				out = append(out, name)
				break
			}
		}
	}
	return out, nil
}

func tableColumns(ctx context.Context, conn *sql.Conn, table string) ([]string, error) {
	rows, err := conn.QueryContext(ctx, fmt.Sprintf(`PRAGMA table_info("%s")`, table))
	if err != nil {
		return nil, fmt.Errorf("table_info %s: %w", table, err)
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		cols = append(cols, name)
	}
	return cols, rows.Err()
}

// scrubSecretTable reports whether it changed any row.
func scrubSecretTable(ctx context.Context, conn *sql.Conn, table string) (bool, error) {
	cols, err := tableColumns(ctx, conn, table)
	if err != nil {
		return false, err
	}
	changed := false
	hasData := false
	for _, c := range cols {
		if c == "data" {
			hasData = true
			continue
		}
		if cliutil.IsSecretFieldName(c) {
			res, err := conn.ExecContext(ctx, fmt.Sprintf(`UPDATE "%s" SET "%s" = NULL WHERE "%s" IS NOT NULL`, table, c, c))
			if err != nil {
				return false, fmt.Errorf("clearing %s.%s: %w", table, c, err)
			}
			if n, _ := res.RowsAffected(); n > 0 {
				changed = true
			}
		}
	}
	if !hasData {
		return changed, nil
	}
	rows, err := conn.QueryContext(ctx, fmt.Sprintf(`SELECT rowid, data FROM "%s" WHERE data IS NOT NULL`, table))
	if err != nil {
		return false, fmt.Errorf("reading %s: %w", table, err)
	}
	type update struct {
		rowid int64
		data  string
	}
	var updates []update
	for rows.Next() {
		var rowid int64
		var data string
		if err := rows.Scan(&rowid, &data); err != nil {
			_ = rows.Close()
			return false, err
		}
		if scrubbed := scrubPersistedSecrets(json.RawMessage(data)); string(scrubbed) != data {
			updates = append(updates, update{rowid, string(scrubbed)})
		}
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return false, err
	}
	for _, u := range updates {
		if _, err := conn.ExecContext(ctx, fmt.Sprintf(`UPDATE "%s" SET data = ? WHERE rowid = ?`, table), u.data, u.rowid); err != nil {
			return false, fmt.Errorf("scrubbing %s row %d: %w", table, u.rowid, err)
		}
	}
	return changed || len(updates) > 0, nil
}
