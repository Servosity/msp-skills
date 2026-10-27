// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated). The local store never keeps a credential:
//   - mergeIncomingResourceData is the one function every write path calls
//     (generic Upsert, UpsertBatch / UpsertBatchDetailed, every typed
//     Upsert<Type>, sync, the read write-through and mutation write-back) to
//     produce the row it stores in resources, resources_fts and the typed
//     table. The generated body is renamed mergeIncomingResourceDataUnscrubbed
//     in store.go; a reprint that restores the generated name collides with
//     this method and fails go build instead of silently dropping the guard.
//   - scrubStoredSecrets (called from migrateExtras, inside the migration
//     transaction) removes what an older build already wrote, from every
//     table sqlite_master lists, once per old store: when the store is below
//     schema v14 (secretScrubSchemaVersion) or carries no scrub stamp. The
//     generated migrate stamps v14 only after migrateExtras returns, in the
//     same transaction, so a scrub that fails or is interrupted leaves the
//     store below v14 and the next open scrubs it again; an older binary
//     refuses a v14 store, so it cannot write a credential back afterwards.

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"

	"sentinelone-pp-cli/internal/cliutil"
)

// mergeIncomingResourceData returns the row to store: the generated
// keep-richer merge with every secret-named field (cliutil.RedactSecretFields)
// dropped from the incoming record and from the merged result, so a secret an
// older build left in the existing row is not carried forward either. A
// never-stored resource (cliutil.IsNeverStoredResource) keeps no content.
func (s *Store) mergeIncomingResourceData(tx *sql.Tx, resourceType, id string, incoming json.RawMessage) json.RawMessage {
	if cliutil.IsNeverStoredResource(resourceType) {
		return json.RawMessage(`{}`)
	}
	scrubbed, _ := cliutil.RedactSecretFields(incoming)
	merged, _ := cliutil.RedactSecretFields(s.mergeIncomingResourceDataUnscrubbed(tx, resourceType, id, scrubbed))
	return merged
}

// secretScrubSchemaVersion is the store schema version that marks a store as
// scrubbed under the every-table walk below. A store below it was written by
// an older build and is scrubbed on its next read-write open. Growing the
// field rule or the never-stored list later means bumping this together
// with StoreSchemaVersion, so older binaries are locked out of the store
// they could otherwise refill.
const secretScrubSchemaVersion = 14

// A reprint that regenerates store.go with an older StoreSchemaVersion fails
// to compile here, instead of shipping a binary that stamps scrubbed stores
// with a version an older binary still opens.
const _ = uint(StoreSchemaVersion - secretScrubSchemaVersion)

// secretScrubRuleVersion is written to secret_scrub_state after a scrub. The
// schema version is the gate; a missing or older stamp is a second trigger
// (a store whose stamp row was lost is scrubbed again). 1 = the v13 scrub of
// a hand-picked table list; 2 = the every-table walk.
const secretScrubRuleVersion = 2

const secretScrubBatch = 500

// scrubStoredSecrets runs inside the migration transaction, before the
// version stamp. When the store is due (secretScrubDue) it deletes the
// never-stored partitions, clears every secret-named column, drops every
// secret-named field from every JSON value in every table, rebuilds
// resources_fts from the scrubbed resources rows, and stamps
// secret_scrub_state. secure_delete zeroes the freed bytes. Any error aborts
// the whole migration (fail closed): nothing is committed and the store
// stays below v14.
func (s *Store) scrubStoredSecrets(ctx context.Context, conn *sql.Conn) error {
	due, err := secretScrubDue(ctx, conn)
	if err != nil || !due {
		return err
	}
	if _, err := conn.ExecContext(ctx, `PRAGMA secure_delete = ON`); err != nil {
		return fmt.Errorf("secret scrub: secure_delete: %w", err)
	}
	walk, _, err := secretScrubPlan(ctx, conn)
	if err != nil {
		return err
	}
	if err := deleteNeverStoredPartitions(ctx, conn, walk); err != nil {
		return err
	}
	for _, t := range walk {
		if err := scrubSecretColumns(ctx, conn, t); err != nil {
			return err
		}
		if err := scrubSecretValues(ctx, conn, t); err != nil {
			return err
		}
	}
	// resources_fts is derived from resources. Rebuilding it from the
	// scrubbed rows drops every indexed term a credential left, including
	// stale entries no resources row explains.
	if err := s.migrateResourcesFTSContent(ctx, conn); err != nil {
		return fmt.Errorf("secret scrub: rebuilding resources_fts: %w", err)
	}
	if _, err := conn.ExecContext(ctx,
		`INSERT INTO secret_scrub_state (id, rule_version) VALUES (1, ?)
		 ON CONFLICT(id) DO UPDATE SET rule_version = excluded.rule_version`,
		secretScrubRuleVersion,
	); err != nil {
		return fmt.Errorf("secret scrub: stamping state: %w", err)
	}
	return nil
}

// secretScrubDue reports whether this open must scrub: the store's
// user_version is below secretScrubSchemaVersion, or its scrub stamp is
// missing or older than secretScrubRuleVersion. user_version is read here,
// inside the migration transaction, before the generated migrate stamps the
// new version (it does so only after migrateExtras returns), so it is the
// version the store had when this open began.
func secretScrubDue(ctx context.Context, conn *sql.Conn) (bool, error) {
	var version int
	if err := conn.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		return false, fmt.Errorf("secret scrub: reading schema version: %w", err)
	}
	if _, err := conn.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS secret_scrub_state (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		rule_version INTEGER NOT NULL
	)`); err != nil {
		return false, fmt.Errorf("secret scrub: %w", err)
	}
	var stamped int
	err := conn.QueryRowContext(ctx, `SELECT rule_version FROM secret_scrub_state WHERE id = 1`).Scan(&stamped)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("secret scrub: reading state: %w", err)
	}
	return version < secretScrubSchemaVersion || stamped < secretScrubRuleVersion, nil
}

type secretScrubColumn struct {
	name    string
	notNull bool
	pk      int // 1-based position in the primary key, 0 when not in it
}

type secretScrubTable struct {
	name         string
	withoutRowid bool
	columns      []secretScrubColumn // every stored column (generated ones are recomputed by SQLite)
}

// secretScrubPlan discovers, at run time, every table the scrub walks: each
// ordinary table sqlite_master lists. It skips SQLite's own tables
// (sqlite_*) and virtual tables with their shadow tables (resources_fts,
// rebuilt from the scrubbed resources rows instead), and returns their names
// as skipped. A table pragma_table_list cannot classify, or a name that is
// not a plain identifier, fails the scrub (fail closed).
func secretScrubPlan(ctx context.Context, conn *sql.Conn) (walk []secretScrubTable, skipped []string, err error) {
	rows, err := conn.QueryContext(ctx, `SELECT m.name, coalesce(l.type, ''), coalesce(l.wr, 0)
		FROM sqlite_master AS m
		LEFT JOIN pragma_table_list AS l ON l.schema = 'main' AND l.name = m.name
		WHERE m.type = 'table'
		ORDER BY m.name`)
	if err != nil {
		return nil, nil, fmt.Errorf("secret scrub: listing tables: %w", err)
	}
	for rows.Next() {
		var name, kind string
		var wr int
		if err := rows.Scan(&name, &kind, &wr); err != nil {
			_ = rows.Close()
			return nil, nil, fmt.Errorf("secret scrub: listing tables: %w", err)
		}
		switch {
		case strings.HasPrefix(strings.ToLower(name), "sqlite_"), kind == "virtual", kind == "shadow":
			skipped = append(skipped, name)
		case kind == "table":
			walk = append(walk, secretScrubTable{name: name, withoutRowid: wr != 0})
		default:
			_ = rows.Close()
			return nil, nil, fmt.Errorf("secret scrub: cannot classify table %q (type %q)", name, kind)
		}
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("secret scrub: listing tables: %w", err)
	}
	for i := range walk {
		if !secretScrubIdentRE.MatchString(walk[i].name) {
			return nil, nil, fmt.Errorf("secret scrub: table name %q is not a plain SQL identifier", walk[i].name)
		}
		cols, err := secretScrubColumns(ctx, conn, walk[i].name)
		if err != nil {
			return nil, nil, err
		}
		walk[i].columns = cols
	}
	return walk, skipped, nil
}

func secretScrubColumns(ctx context.Context, conn *sql.Conn, table string) ([]secretScrubColumn, error) {
	rows, err := conn.QueryContext(ctx, `SELECT name, "notnull", pk, hidden FROM pragma_table_xinfo(?)`, table)
	if err != nil {
		return nil, fmt.Errorf("secret scrub: inspecting %s: %w", table, err)
	}
	defer rows.Close()
	var cols []secretScrubColumn
	for rows.Next() {
		var c secretScrubColumn
		var notNull, hidden int
		if err := rows.Scan(&c.name, &notNull, &c.pk, &hidden); err != nil {
			return nil, fmt.Errorf("secret scrub: inspecting %s: %w", table, err)
		}
		if hidden != 0 {
			continue // a generated column: SQLite recomputes it from the scrubbed columns
		}
		if !secretScrubIdentRE.MatchString(c.name) {
			return nil, fmt.Errorf("secret scrub: column %s.%q is not a plain SQL identifier", table, c.name)
		}
		c.notNull = notNull != 0
		cols = append(cols, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("secret scrub: inspecting %s: %w", table, err)
	}
	return cols, nil
}

// secretScrubIdentRE is the only identifier shape secretScrubSQL splices
// into a statement.
var secretScrubIdentRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// secretScrubSQL is the one place the scrub builds SQL from a name it did
// not write itself. tmpl is a constant at every call site; each {} in it is
// replaced by the next identifier, double-quoted, and an identifier that is
// not a plain SQL name (letters, digits, underscore: nothing that can close
// the quote or carry SQL) is refused. Table and column names come from
// sqlite_master and pragma_table_xinfo; every value is a ? parameter.
func secretScrubSQL(tmpl string, idents ...string) (string, error) {
	var b strings.Builder
	for _, id := range idents {
		i := strings.Index(tmpl, "{}")
		if i < 0 {
			return "", fmt.Errorf("secret scrub: statement has fewer slots than identifiers")
		}
		if !secretScrubIdentRE.MatchString(id) {
			return "", fmt.Errorf("secret scrub: %q is not a plain SQL identifier", id)
		}
		b.WriteString(tmpl[:i])
		b.WriteByte('"')
		b.WriteString(id)
		b.WriteByte('"')
		tmpl = tmpl[i+2:]
	}
	if strings.Contains(tmpl, "{}") {
		return "", fmt.Errorf("secret scrub: statement has more slots than identifiers")
	}
	b.WriteString(tmpl)
	return b.String(), nil
}

func secretScrubExec(ctx context.Context, conn *sql.Conn, tmpl string, idents []string, args ...any) (int64, error) {
	q, err := secretScrubSQL(tmpl, idents...)
	if err != nil {
		return 0, err
	}
	res, err := conn.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, fmt.Errorf("secret scrub: %s: %w", strings.Join(idents, "."), err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// deleteNeverStoredPartitions removes the rows whose content is a
// credential: every resources and sync_state row of a never-stored resource
// type (cliutil.IsNeverStoredResource, any case or _/- spelling), and every
// row of a table that belongs to such a partition (named after it, or its
// typed table). The tables themselves stay.
func deleteNeverStoredPartitions(ctx context.Context, conn *sql.Conn, walk []secretScrubTable) error {
	for _, t := range walk {
		if !cliutil.IsNeverStoredResource(t.name) && !neverStoredTypedTable(t.name) {
			continue
		}
		if _, err := secretScrubExec(ctx, conn, `DELETE FROM {}`, []string{t.name}); err != nil {
			return err
		}
	}
	for _, t := range walk {
		if t.name != "resources" && t.name != "sync_state" {
			continue
		}
		if !t.hasColumn("resource_type") {
			continue // a store from before rows were keyed by resource_type
		}
		q, err := secretScrubSQL(`SELECT DISTINCT {} FROM {}`, "resource_type", t.name)
		if err != nil {
			return err
		}
		rows, err := conn.QueryContext(ctx, q)
		if err != nil {
			return fmt.Errorf("secret scrub: listing %s partitions: %w", t.name, err)
		}
		var doomed []string
		for rows.Next() {
			var rt sql.NullString
			if err := rows.Scan(&rt); err != nil {
				_ = rows.Close()
				return fmt.Errorf("secret scrub: listing %s partitions: %w", t.name, err)
			}
			if rt.Valid && cliutil.IsNeverStoredResource(rt.String) {
				doomed = append(doomed, rt.String)
			}
		}
		_ = rows.Close()
		if err := rows.Err(); err != nil {
			return fmt.Errorf("secret scrub: listing %s partitions: %w", t.name, err)
		}
		for _, rt := range doomed {
			if _, err := secretScrubExec(ctx, conn, `DELETE FROM {} WHERE {} = ?`, []string{t.name, "resource_type"}, rt); err != nil {
				return err
			}
		}
	}
	return nil
}

func neverStoredTypedTable(table string) bool {
	for resourceType, typed := range typedListTableByResource {
		if typed == table && cliutil.IsNeverStoredResource(resourceType) {
			return true
		}
	}
	return false
}

func (t secretScrubTable) hasColumn(name string) bool {
	for _, c := range t.columns {
		if c.name == name {
			return true
		}
	}
	return false
}

// scrubSecretColumns clears every column whose name is secret-named under
// the shared field rule (cliutil.IsSecretFieldName): NULL, or ” where the
// column is NOT NULL.
func scrubSecretColumns(ctx context.Context, conn *sql.Conn, t secretScrubTable) error {
	for _, c := range t.columns {
		if !cliutil.IsSecretFieldName(c.name) {
			continue
		}
		tmpl := `UPDATE {} SET {} = NULL WHERE {} IS NOT NULL`
		if c.notNull {
			tmpl = `UPDATE {} SET {} = '' WHERE {} <> ''`
		}
		if _, err := secretScrubExec(ctx, conn, tmpl, []string{t.name, c.name, c.name}); err != nil {
			return err
		}
	}
	return nil
}

// scrubSecretValue passes one stored value through the shared field rule: a
// TEXT or BLOB value that parses as one JSON document loses every
// secret-named key at any depth (and the value of a secret-named name/value
// pair). Anything else is returned unchanged. The storage class is kept.
func scrubSecretValue(v any) (any, bool) {
	switch x := v.(type) {
	case string:
		if out, removed := cliutil.RedactSecretFields(json.RawMessage(x)); removed {
			return string(out), true
		}
	case []byte:
		if out, removed := cliutil.RedactSecretFields(json.RawMessage(x)); removed {
			return []byte(out), true
		}
	}
	return v, false
}

// scrubSecretValues rewrites every TEXT/BLOB value of t that the field rule
// changes. A rowid table is read in rowid batches; a WITHOUT ROWID table is
// read once, keeping only the changed values, and updated by primary key.
// Columns cleared by scrubSecretColumns are skipped.
func scrubSecretValues(ctx context.Context, conn *sql.Conn, t secretScrubTable) error {
	var cols, keys []string
	for _, c := range t.columns {
		if !cliutil.IsSecretFieldName(c.name) {
			cols = append(cols, c.name)
		}
	}
	if len(cols) == 0 {
		return nil
	}
	if t.withoutRowid {
		keys = t.primaryKey()
		if len(keys) == 0 {
			return fmt.Errorf("secret scrub: WITHOUT ROWID table %s has no primary key", t.name)
		}
		return scrubSecretValuesByKey(ctx, conn, t.name, cols, keys)
	}
	return scrubSecretValuesByRowid(ctx, conn, t.name, cols)
}

func (t secretScrubTable) primaryKey() []string {
	var keys []string
	for pos := 1; ; pos++ {
		found := false
		for _, c := range t.columns {
			if c.pk == pos {
				keys = append(keys, c.name)
				found = true
			}
		}
		if !found {
			return keys
		}
	}
}

// scrubSecretRow passes every value of one row through scrubSecretValue and
// writes the changed ones back in one UPDATE, keyed by rowid (keys nil) or
// by the primary key columns (their values as read, so a changed key column
// still finds its row).
func scrubSecretRow(ctx context.Context, conn *sql.Conn, table string, cols []string, vals []any, keys []string, keyVals []any) error {
	var set []string
	var args []any
	for i, v := range vals {
		if scrubbed, changed := scrubSecretValue(v); changed {
			set = append(set, cols[i])
			args = append(args, scrubbed)
		}
	}
	if len(set) == 0 {
		return nil
	}
	tmpl := `UPDATE {} SET ` + strings.TrimPrefix(strings.Repeat(`, {} = ?`, len(set)), `, `)
	idents := append([]string{table}, set...)
	if keys == nil {
		tmpl += ` WHERE rowid = ?`
	} else {
		tmpl += ` WHERE ` + strings.TrimPrefix(strings.Repeat(` AND {} = ?`, len(keys)), ` AND `)
		idents = append(idents, keys...)
	}
	_, err := secretScrubExec(ctx, conn, tmpl, idents, append(args, keyVals...)...)
	return err
}

func scrubSecretValuesByRowid(ctx context.Context, conn *sql.Conn, table string, cols []string) error {
	sel, err := secretScrubSQL(`SELECT rowid`+strings.Repeat(`, {}`, len(cols))+` FROM {} WHERE rowid >= ? ORDER BY rowid LIMIT ?`,
		append(append([]string(nil), cols...), table)...)
	if err != nil {
		return err
	}
	type row struct {
		rowid int64
		vals  []any
	}
	from := int64(math.MinInt64)
	for {
		rows, err := conn.QueryContext(ctx, sel, from, secretScrubBatch)
		if err != nil {
			return fmt.Errorf("secret scrub: reading %s: %w", table, err)
		}
		var batch []row
		for rows.Next() {
			r := row{vals: make([]any, len(cols))}
			dest := []any{&r.rowid}
			for i := range r.vals {
				dest = append(dest, &r.vals[i])
			}
			if err := rows.Scan(dest...); err != nil {
				_ = rows.Close()
				return fmt.Errorf("secret scrub: reading %s: %w", table, err)
			}
			batch = append(batch, r)
		}
		_ = rows.Close()
		if err := rows.Err(); err != nil {
			return fmt.Errorf("secret scrub: reading %s: %w", table, err)
		}
		for _, r := range batch {
			if err := scrubSecretRow(ctx, conn, table, cols, r.vals, nil, []any{r.rowid}); err != nil {
				return err
			}
		}
		if len(batch) < secretScrubBatch {
			return nil
		}
		last := batch[len(batch)-1].rowid
		if last == math.MaxInt64 {
			return nil
		}
		from = last + 1
	}
}

// scrubSecretValuesByKey reads a WITHOUT ROWID table once, keeping only the
// rows the field rule changes, then rewrites them by primary key.
func scrubSecretValuesByKey(ctx context.Context, conn *sql.Conn, table string, cols, keys []string) error {
	idents := append(append(append([]string(nil), keys...), cols...), table)
	sel, err := secretScrubSQL(`SELECT `+strings.TrimPrefix(strings.Repeat(`, {}`, len(keys)+len(cols)), `, `)+` FROM {}`, idents...)
	if err != nil {
		return err
	}
	type row struct{ keyVals, vals []any }
	var changed []row
	rows, err := conn.QueryContext(ctx, sel)
	if err != nil {
		return fmt.Errorf("secret scrub: reading %s: %w", table, err)
	}
	for rows.Next() {
		all := make([]any, len(keys)+len(cols))
		dest := make([]any, len(all))
		for i := range all {
			dest[i] = &all[i]
		}
		if err := rows.Scan(dest...); err != nil {
			_ = rows.Close()
			return fmt.Errorf("secret scrub: reading %s: %w", table, err)
		}
		for _, v := range all[len(keys):] {
			if _, c := scrubSecretValue(v); c {
				changed = append(changed, row{keyVals: all[:len(keys)], vals: all[len(keys):]})
				break
			}
		}
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("secret scrub: reading %s: %w", table, err)
	}
	for _, r := range changed {
		if err := scrubSecretRow(ctx, conn, table, cols, r.vals, keys, r.keyVals); err != nil {
			return err
		}
	}
	return nil
}
