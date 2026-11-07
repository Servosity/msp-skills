// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package credsql registers the one SQLite driver this program opens every
// database with (handfixes.json credkit-wiring): modernc.org/sqlite behind the
// credential kit's credkit.SQLDriver, so every statement, its text and its
// bound values, is checked before SQLite runs it (credential leak contract
// sections 2 and 5, reads included). Hand-written; a reprint never regenerates
// it.
//
// It is its own package so internal/store and internal/platform can both
// import it without importing each other.
package credsql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"fortinet-pp-cli/internal/credkit"
)

// Driver is the driver name every sql.Open in this program uses.
const Driver = "credkit-sqlite"

// LengthLimitParam is a DSN query parameter: a connection opened with
// _length_limit=N gets SQLITE_LIMIT_LENGTH N (no string or blob longer than
// N bytes) before the kit hands it to database/sql. The MCP sql tool caps
// its values this way; sqlite.Limit on a connection the program holds would
// need modernc's own connection, which the kit wraps.
const LengthLimitParam = "_length_limit"

// maxLength is SQLite's compiled-in SQLITE_MAX_LENGTH in modernc.org/sqlite.
const maxLength = 1_000_000_000

// The registration is init's first statement, so the kit's wiring checker can
// see it runs before any database is opened.
func init() {
	sql.Register(Driver, credkit.SQLDriver(cappedSQLite()))
}

// cappedSQLite is modernc's driver with the _length_limit connection hook.
func cappedSQLite() *sqlite.Driver {
	d := &sqlite.Driver{}
	d.RegisterConnectionHook(applyLengthLimit)
	return d
}

// lengthLimit is the _length_limit a DSN names. A malformed or repeated
// value is an error, so the open fails rather than running uncapped.
func lengthLimit(dsn string) (n int, ok bool, err error) {
	i := strings.IndexByte(dsn, '?')
	if i < 1 {
		return 0, false, nil
	}
	q, err := url.ParseQuery(dsn[i+1:])
	if err != nil {
		return 0, false, fmt.Errorf("credsql: reading the DSN's query: %w", err)
	}
	vs, ok := q[LengthLimitParam]
	if !ok {
		return 0, false, nil
	}
	if len(vs) != 1 {
		return 0, false, fmt.Errorf("credsql: %s given %d times", LengthLimitParam, len(vs))
	}
	n, err = strconv.Atoi(vs[0])
	if err != nil || n < 1 || n > maxLength {
		return 0, false, fmt.Errorf("credsql: %s=%q is not a length from 1 to %d", LengthLimitParam, vs[0], maxLength)
	}
	return n, true, nil
}

// applyLengthLimit is the inner driver's connection hook: it runs on
// modernc's own connection, after the DSN's pragmas and before the kit wraps
// it. sqlite.Limit reaches a connection only through a *sql.Conn, so the
// connection is lent to a one-connection sql.DB for that call and nothing
// else: Limit is the C call sqlite3_limit, not a statement, and no statement
// runs through the lent handle. The DB is closed with the connection still
// checked out, which database/sql never closes, so the connection goes on to
// the kit unchanged but for its limit.
func applyLengthLimit(c sqlite.ExecQuerierContext, dsn string) error {
	n, ok, err := lengthLimit(dsn)
	if err != nil || !ok {
		return err
	}
	conn, isConn := c.(driver.Conn)
	if !isConn {
		return fmt.Errorf("credsql: %T is not a driver connection", c)
	}
	db := sql.OpenDB(lent{conn})
	defer db.Close()
	sc, err := db.Conn(context.Background())
	if err != nil {
		return err
	}
	// sc is never closed: closing it would close the lent connection.
	if _, err := sqlite.Limit(sc, sqlite3.SQLITE_LIMIT_LENGTH, n); err != nil {
		return err
	}
	got, err := sqlite.Limit(sc, sqlite3.SQLITE_LIMIT_LENGTH, -1)
	if err != nil {
		return err
	}
	if got != n {
		return fmt.Errorf("credsql: SQLITE_LIMIT_LENGTH is %d after setting %d", got, n)
	}
	return nil
}

// lent hands database/sql one connection that already exists.
type lent struct{ c driver.Conn }

func (l lent) Connect(context.Context) (driver.Conn, error) { return l.c, nil }
func (l lent) Driver() driver.Driver                        { return opensNothing{} }

type opensNothing struct{}

func (opensNothing) Open(string) (driver.Conn, error) {
	return nil, errors.New("credsql: the lent connection's driver opens nothing")
}
