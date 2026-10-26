// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored (not generated). The store half of the credential tier
// (internal/cliutil/tl_credential_tier.go). From this release the CLI never
// writes a credential-tier response to the store (internal/cli
// writeThroughCacheUnlessCredentialTier). A store an earlier binary wrote to
// can already hold one: `organizations auth-key` merged its authKey into the
// organization's row in organizations.data, organizations.auth_key,
// resources.data and the resources_fts leaf index, where the MCP `sql` and
// `search` tools read it. scrubCredentialTierRows removes every copy on each
// writable open, keeping the rows themselves, and drops the typed key columns
// (schema v14 no longer declares them). upgradeLegacyCredentialStore runs that
// writable open before any read-only open of a store below v14, so the MCP
// sql, search and export tools never read a store an earlier binary left. It is idempotent and cheap when
// there is nothing to remove: a row is parsed only when its raw text holds the
// key name or a \u escape. That prefilter is sound: in JSON text a key's
// letters are either literal (LIKE folds ASCII case) or \u-escaped, and SQLite
// decodes both, so json_extract(data, '$.authKey') would read either spelling.
//
// Every statement is a literal: no SQL is assembled at runtime. A tier member
// whose store resource or column has no statements here fails the open (and
// TestLegacyCredentialTierValuesAreScrubbedOnOpen), so a new member cannot be
// added to the tier without its scrub.
//
// Called from migrateExtras (extras.go) inside the migration transaction
// (handfixes.json: credential-tier-never-cached-never-served-to-mcp).

package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"threatlocker-pp-cli/internal/cliutil"
)

type credentialTierTypedSQL struct {
	selectByKey string            // id, data of rows whose text may hold the key (? = LIKE pattern)
	removePath  string            // json_remove one path from one row (? = path, id)
	clearColumn map[string]string // column -> statement nulling it
	dropColumn  map[string]string // column -> statement dropping it from the schema
}

var credentialTierTypedStatements = map[string]credentialTierTypedSQL{
	"organizations": {
		selectByKey: `SELECT id, data FROM "organizations" WHERE json_valid(data) AND (data LIKE ? OR instr(data, char(92) || 'u') > 0)`,
		removePath:  `UPDATE "organizations" SET data = json_remove(data, ?) WHERE id = ?`,
		clearColumn: map[string]string{
			"auth_key": `UPDATE "organizations" SET "auth_key" = NULL WHERE "auth_key" IS NOT NULL`,
		},
		dropColumn: map[string]string{
			"auth_key": `ALTER TABLE "organizations" DROP COLUMN "auth_key"`,
		},
	},
	"computers": {
		selectByKey: `SELECT id, data FROM "computers" WHERE json_valid(data) AND (data LIKE ? OR instr(data, char(92) || 'u') > 0)`,
		removePath:  `UPDATE "computers" SET data = json_remove(data, ?) WHERE id = ?`,
		clearColumn: map[string]string{
			"install_key":  `UPDATE "computers" SET "install_key" = NULL WHERE "install_key" IS NOT NULL`,
			"download_url": `UPDATE "computers" SET "download_url" = NULL WHERE "download_url" IS NOT NULL`,
		},
		dropColumn: map[string]string{
			"install_key":  `ALTER TABLE "computers" DROP COLUMN "install_key"`,
			"download_url": `ALTER TABLE "computers" DROP COLUMN "download_url"`,
		},
	},
}

const (
	tierResourcesSelectSQL = `SELECT id, data FROM resources WHERE resource_type = ? AND json_valid(data) AND (data LIKE ? OR instr(data, char(92) || 'u') > 0)`
	tierResourcesRemoveSQL = `UPDATE resources SET data = json_remove(data, ?) WHERE resource_type = ? AND id = ?`
)

func (s *Store) scrubCredentialTierRows(ctx context.Context, conn *sql.Conn) error {
	for _, ep := range cliutil.CredentialTier() {
		if ep.StoreResource == "" || len(ep.StoreFields) == 0 {
			continue
		}
		stmts, ok := credentialTierTypedStatements[ep.StoreResource]
		if !ok {
			return fmt.Errorf("credential-tier scrub: no statements for store resource %q (%s)", ep.StoreResource, ep.ID)
		}
		table := strings.ReplaceAll(ep.StoreResource, "-", "_")
		typedExists, err := tableExists(ctx, conn, table)
		if err != nil {
			return err
		}
		touched := map[string]bool{}
		for _, f := range ep.StoreFields {
			if typedExists {
				if f.Column != "" {
					clear, ok := stmts.clearColumn[f.Column]
					drop, dropOK := stmts.dropColumn[f.Column]
					if !ok || !dropOK {
						return fmt.Errorf("credential-tier scrub: no statements clearing and dropping %s.%s (%s)", table, f.Column, ep.ID)
					}
					if has, err := credentialTierColumnExists(ctx, conn, table, f.Column); err != nil {
						return err
					} else if has {
						if _, err := conn.ExecContext(ctx, clear); err != nil {
							return fmt.Errorf("credential-tier scrub: clearing %s.%s: %w", table, f.Column, err)
						}
						if _, err := conn.ExecContext(ctx, drop); err != nil {
							return fmt.Errorf("credential-tier scrub: dropping %s.%s: %w", table, f.Column, err)
						}
					}
				}
				if _, err := scrubCredentialTierKey(ctx, conn, stmts.selectByKey, stmts.removePath, "", f.JSONKey); err != nil {
					return err
				}
			}
			ids, err := scrubCredentialTierKey(ctx, conn, tierResourcesSelectSQL, tierResourcesRemoveSQL, ep.StoreResource, f.JSONKey)
			if err != nil {
				return err
			}
			for _, id := range ids {
				touched[id] = true
			}
		}
		if len(touched) == 0 {
			continue
		}
		if ok, err := tableExists(ctx, conn, "resources_fts"); err != nil {
			return err
		} else if !ok {
			continue // a later FTS rebuild indexes the already-scrubbed rows
		}
		for id := range touched {
			var data string
			if err := conn.QueryRowContext(ctx, `SELECT data FROM resources WHERE resource_type = ? AND id = ?`, ep.StoreResource, id).Scan(&data); err != nil {
				return fmt.Errorf("credential-tier scrub: reading %s/%s: %w", ep.StoreResource, id, err)
			}
			rowid := ftsRowID(ep.StoreResource, id)
			if _, err := conn.ExecContext(ctx, `DELETE FROM resources_fts WHERE rowid = ?`, rowid); err != nil {
				return fmt.Errorf("credential-tier scrub: unindexing %s/%s: %w", ep.StoreResource, id, err)
			}
			if _, err := conn.ExecContext(ctx, `INSERT INTO resources_fts (rowid, id, resource_type, content) VALUES (?, ?, ?, ?)`,
				rowid, id, ep.StoreResource, searchableResourceContent([]byte(data))); err != nil {
				return fmt.Errorf("credential-tier scrub: reindexing %s/%s: %w", ep.StoreResource, id, err)
			}
		}
	}
	return nil
}

func credentialTierColumnExists(ctx context.Context, conn *sql.Conn, table, column string) (bool, error) {
	var n int
	if err := conn.QueryRowContext(ctx, `SELECT count(*) FROM pragma_table_info(?) WHERE name = ?`, table, column).Scan(&n); err != nil {
		return false, fmt.Errorf("credential-tier scrub: reading %s columns: %w", table, err)
	}
	return n > 0, nil
}

// scrubCredentialTierKey removes key, at any depth, from every row selectSQL
// returns, one json_remove per path, and returns the ids it rewrote. With a
// resourceType the statements take it as their resource_type argument.
func scrubCredentialTierKey(ctx context.Context, conn *sql.Conn, selectSQL, removeSQL, resourceType, key string) ([]string, error) {
	selectArgs := []any{`%"` + key + `"%`}
	if resourceType != "" {
		selectArgs = append([]any{resourceType}, selectArgs...)
	}
	rows, err := conn.QueryContext(ctx, selectSQL, selectArgs...)
	if err != nil {
		return nil, fmt.Errorf("credential-tier scrub: scanning for %s: %w", key, err)
	}
	type candidate struct{ id, data string }
	var found []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.id, &c.data); err != nil {
			_ = rows.Close()
			return nil, err
		}
		found = append(found, c)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	var rewritten []string
	for _, c := range found {
		paths, err := credentialTierJSONPaths(ctx, conn, c.data, key)
		if err != nil {
			return nil, err
		}
		for _, p := range paths {
			args := []any{p, c.id}
			if resourceType != "" {
				args = []any{p, resourceType, c.id}
			}
			if _, err := conn.ExecContext(ctx, removeSQL, args...); err != nil {
				return nil, fmt.Errorf("credential-tier scrub: removing %s from %s: %w", p, c.id, err)
			}
		}
		if len(paths) > 0 {
			rewritten = append(rewritten, c.id)
		}
	}
	return rewritten, nil
}

// credentialTierJSONPaths lists the JSON paths of every object member named
// key (case-insensitively), deepest first so removing one never invalidates
// the path of another.
func credentialTierJSONPaths(ctx context.Context, conn *sql.Conn, data, key string) ([]string, error) {
	rows, err := conn.QueryContext(ctx, `SELECT key, fullkey FROM json_tree(?) ORDER BY length(fullkey) DESC`, data)
	if err != nil {
		return nil, fmt.Errorf("credential-tier scrub: walking JSON: %w", err)
	}
	defer rows.Close()
	var paths []string
	for rows.Next() {
		var member sql.NullString
		var full string
		if err := rows.Scan(&member, &full); err != nil {
			return nil, err
		}
		if member.Valid && strings.EqualFold(member.String, key) {
			paths = append(paths, full)
		}
	}
	return paths, rows.Err()
}

// legacyUpgradeOpenHook is the writable open upgradeLegacyCredentialStore
// runs; a test seam for the fail-closed path.
var legacyUpgradeOpenHook = OpenWithContext

// upgradeLegacyCredentialStore runs on every read-only open, after the
// generated read-only open succeeded (so read-only directories and blocked WAL
// files keep working through its immutable fallback). A store stamped below
// StoreSchemaVersion was last written by an earlier binary (v0.3.12 is schema
// 9) and may still hold an agent installation key in a typed column, a row's
// JSON or the search index; an unstamped database is treated the same way when
// it shows any trace of one. Such a store is closed, opened for writing once,
// which runs the migrations (the credential-tier scrub, the column drop, the
// index rebuild) and stamps the current version, and then reopened read-only.
// Only then may a read-only caller (the MCP sql, search and export tools,
// local readers) see it. When the writable open fails, the read-only open
// fails too: answering from a store that may hold a key is the leak this
// exists to stop. A store at the current version is never reopened for
// writing, and an earlier binary refuses a store stamped newer than it
// understands, so it cannot add a key back.
// Hand-wired (handfixes.json: credential-tier-never-cached-never-served-to-mcp).
func upgradeLegacyCredentialStore(ctx context.Context, ro *Store, reopen func(context.Context, string) (*Store, error)) (*Store, error) {
	needs, current, err := legacyCredentialStoreNeedsUpgrade(ctx, ro.db)
	if err != nil {
		_ = ro.Close()
		return nil, fmt.Errorf("checking the local store at %s for keys an earlier version stored: %w", ro.path, err)
	}
	if !needs {
		return ro, nil
	}
	path := ro.path
	_ = ro.Close()
	w, err := legacyUpgradeOpenHook(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("the local store at %s was written by an earlier version (schema %d) and may still hold an agent installation key, so it is upgraded before it is read; opening it for that upgrade failed: %w. Run a threatlocker-cli command that writes the store (for example threatlocker-cli sync) with write access to it, then retry", path, current, err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("upgrading the local store at %s: %w", path, err)
	}
	again, err := reopen(ctx, path)
	if err != nil {
		return nil, err
	}
	if needs, current, err = legacyCredentialStoreNeedsUpgrade(ctx, again.db); err != nil || needs {
		_ = again.Close()
		return nil, fmt.Errorf("the local store at %s is still at schema %d after its upgrade and may hold an agent installation key: %v", path, current, err)
	}
	return again, nil
}

// legacyCredentialStoreNeedsUpgrade reports whether a read-only handle may be
// looking at keys an earlier binary stored: any stamped version below the
// current one, or an unstamped database that still declares a key column or
// has a row whose JSON may hold a key (the scrub's own prefilter: the key name,
// or a \u escape).
func legacyCredentialStoreNeedsUpgrade(ctx context.Context, db *sql.DB) (bool, int, error) {
	var current int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&current); err != nil {
		return false, 0, err
	}
	if current >= StoreSchemaVersion {
		return false, current, nil
	}
	if current > 0 {
		return true, current, nil
	}
	hasColumn := func(table, column string) (bool, error) {
		var n int
		err := db.QueryRowContext(ctx, `SELECT count(*) FROM pragma_table_info(?) WHERE name = ?`, table, column).Scan(&n)
		return n > 0, err
	}
	mayHoldKey := func(query string, args ...any) (bool, error) {
		var n int
		err := db.QueryRowContext(ctx, query, args...).Scan(&n)
		return n > 0, err
	}
	resourcesTable, err := hasColumn("resources", "resource_type")
	if err != nil {
		return false, current, err
	}
	for _, ep := range cliutil.CredentialTier() {
		table := strings.ReplaceAll(ep.StoreResource, "-", "_")
		typedTable, err := hasColumn(table, "data")
		if err != nil {
			return false, current, err
		}
		for _, f := range ep.StoreFields {
			pattern := `%"` + f.JSONKey + `"%`
			checks := []func() (bool, error){
				func() (bool, error) { return hasColumn(table, f.Column) },
			}
			if typedTable {
				checks = append(checks, func() (bool, error) {
					return mayHoldKey(`SELECT count(*) FROM "`+table+`" WHERE data LIKE ? OR instr(data, char(92) || 'u') > 0`, pattern)
				})
			}
			if resourcesTable {
				checks = append(checks, func() (bool, error) {
					return mayHoldKey(`SELECT count(*) FROM resources WHERE resource_type = ? AND (data LIKE ? OR instr(data, char(92) || 'u') > 0)`, ep.StoreResource, pattern)
				})
			}
			for _, check := range checks {
				if hit, err := check(); err != nil {
					return false, current, err
				} else if hit {
					return true, current, nil
				}
			}
		}
	}
	return false, current, nil
}
