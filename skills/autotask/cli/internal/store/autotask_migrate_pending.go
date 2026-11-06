// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"context"
	"database/sql"
	"regexp"
	"strings"
)

// Hand-fix (handfixes.json store-open-runs-only-missing-objects). Every
// statement in migrate's list is CREATE ... IF NOT EXISTS, which SQLite runs as
// a no-op when an object of that type and name already exists. The list holds
// about 480 statements and every store open ran all of them; the credential
// kit's SQL driver checks the text of each one, so with a credential held one
// open cost seconds of CPU (CI #565: the black-box suite passed go test's
// 10 minute limit). pendingMigrations drops only the statements whose object
// already exists, so an open of a current store runs none. A statement it
// cannot name runs as before. execMigrations then runs what is left as one
// statement text, so the kit checks one unit instead of one per statement: a
// fresh store's first open cost 5 CPU-s and costs 1.7 (Windows CI #565 still
// passed the limit after the skip, every test home opening a fresh store).

// createIfNotExists names the object a CREATE ... IF NOT EXISTS statement
// creates: its sqlite_master type, then its bare or double-quoted name. A
// quoted name holding a quote, or a schema-qualified one, does not match.
var createIfNotExists = regexp.MustCompile(`^\s*CREATE\s+(UNIQUE\s+INDEX|INDEX|VIRTUAL\s+TABLE|TABLE|TRIGGER)\s+IF\s+NOT\s+EXISTS\s+(?:"([^".]+)"|([A-Za-z_][A-Za-z0-9_]*))[\s(]`)

// createdObject returns the sqlite_master key (type, NUL, lower-case name) of
// the object stmt creates, or false when stmt is not one this file can name.
func createdObject(stmt string) (string, bool) {
	m := createIfNotExists.FindStringSubmatch(stmt)
	if m == nil {
		return "", false
	}
	kind := "table"
	switch {
	case strings.HasSuffix(m[1], "INDEX"):
		kind = "index"
	case m[1] == "TRIGGER":
		kind = "trigger"
	}
	name := m[2]
	if name == "" {
		name = m[3]
	}
	return kind + "\x00" + strings.ToLower(name), true
}

// pendingMigrations returns the statements of stmts whose object does not
// exist yet, in their order. It reads sqlite_master on conn, inside the
// caller's migration transaction.
func pendingMigrations(ctx context.Context, conn *sql.Conn, stmts []string) ([]string, error) {
	rows, err := conn.QueryContext(ctx, `SELECT type, name FROM sqlite_master`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	have := map[string]bool{}
	for rows.Next() {
		var kind, name string
		if err := rows.Scan(&kind, &name); err != nil {
			return nil, err
		}
		have[kind+"\x00"+strings.ToLower(name)] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var out []string
	for _, m := range stmts {
		if key, ok := createdObject(m); ok && have[key] {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

// execMigrations runs stmts in order as one multi-statement Exec on conn, in
// the caller's migration transaction: the first statement that fails returns
// its error and the caller rolls the transaction back, as with one Exec each.
// The separator starts on a new line, so a statement ending in a -- comment
// cannot swallow it.
func execMigrations(ctx context.Context, conn *sql.Conn, stmts []string) error {
	if len(stmts) == 0 {
		return nil
	}
	_, err := conn.ExecContext(ctx, strings.Join(stmts, "\n;\n"))
	return err
}
