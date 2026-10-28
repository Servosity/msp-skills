// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

// kaseya-vsa handfix (store-never-keeps-secrets, review r1 P1-A): a record's
// ID was extracted before withoutStoredSecrets ran, so a record whose ID (or
// parent ID) repeated the token secret was stored with it in resources.id,
// resources_fts.id and the typed table; sync cursors and scope keys, and the
// learn tables, had no check at all.
//
// The one place every store write passes is the database driver: the
// read-write handle (OpenWithContext) opens through withholdingSQLiteDriver,
// which wraps modernc.org/sqlite and checks every bound value of every
// statement that can persist one. A statement whose value repeats a held
// credential, in any form cliutil.HeldEchoes recognizes, is not run: it
// reports zero rows changed, and the process warns once, with no value in
// the warning. Every statement of one record carries its storage ID, so the
// record is skipped whole and the rest of the batch is stored. Reads and
// deletes persist no value and run unchanged. Statement text is never
// checked: the store builds it from schema constants only, never from a
// response.

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"kaseya-vsa-pp-cli/internal/cliutil"
)

const withholdingSQLiteDriver = "kaseya-vsa-sqlite-withholding"

func init() {
	sql.Register(withholdingSQLiteDriver, withholdingDriver{})
}

var (
	innerSQLiteOnce   sync.Once
	innerSQLiteDriver driver.Driver
	heldWriteWarned   sync.Once
)

// innerSQLite is the driver modernc.org/sqlite registered as "sqlite".
func innerSQLite() driver.Driver {
	innerSQLiteOnce.Do(func() {
		if db, err := sql.Open("sqlite", ""); err == nil {
			innerSQLiteDriver = db.Driver()
			_ = db.Close()
		}
	})
	return innerSQLiteDriver
}

type withholdingDriver struct{}

func (withholdingDriver) Open(name string) (driver.Conn, error) {
	inner := innerSQLite()
	if inner == nil {
		return nil, errors.New("sqlite driver is not registered")
	}
	c, err := inner.Open(name)
	if err != nil {
		return nil, err
	}
	return &withholdingConn{inner: c}, nil
}

// errHeldValueRefused is returned only where a statement that writes is run
// as a query, which the store never does; Exec skips silently instead.
var errHeldValueRefused = errors.New("store write refused: a value repeated a credential this program holds")

// persistsValues reports whether a statement can write a bound value: every
// statement except a read or a delete.
func persistsValues(query string) bool {
	q := strings.TrimLeft(query, " \t\r\n(")
	for strings.HasPrefix(q, "--") || strings.HasPrefix(q, "/*") {
		if strings.HasPrefix(q, "--") {
			if i := strings.IndexByte(q, '\n'); i >= 0 {
				q = strings.TrimLeft(q[i+1:], " \t\r\n(")
				continue
			}
			return false
		}
		if i := strings.Index(q, "*/"); i >= 0 {
			q = strings.TrimLeft(q[i+2:], " \t\r\n(")
			continue
		}
		return false
	}
	word := q
	if i := strings.IndexAny(q, " \t\r\n("); i >= 0 {
		word = q[:i]
	}
	switch strings.ToUpper(word) {
	case "SELECT", "DELETE":
		return false
	}
	return true
}

// heldInArgs reports whether a statement that writes would persist a value
// that repeats a held credential.
func heldInArgs(query string, args []driver.NamedValue) bool {
	if len(args) == 0 || !persistsValues(query) {
		return false
	}
	for _, a := range args {
		var text string
		switch v := a.Value.(type) {
		case string:
			text = v
		case []byte:
			text = string(v)
		default:
			continue
		}
		if cliutil.HeldEchoes(text) {
			heldWriteWarned.Do(func() {
				fmt.Fprintln(os.Stderr, "warning: a record was not stored locally: a value in it repeated a credential this program holds")
			})
			return true
		}
	}
	return false
}

type skippedWrite struct{}

func (skippedWrite) LastInsertId() (int64, error) { return 0, nil }
func (skippedWrite) RowsAffected() (int64, error) { return 0, nil }

type withholdingConn struct{ inner driver.Conn }

func (c *withholdingConn) Prepare(query string) (driver.Stmt, error) {
	return c.PrepareContext(context.Background(), query)
}

func (c *withholdingConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	var s driver.Stmt
	var err error
	if p, ok := c.inner.(driver.ConnPrepareContext); ok {
		s, err = p.PrepareContext(ctx, query)
	} else {
		s, err = c.inner.Prepare(query)
	}
	if err != nil {
		return nil, err
	}
	return &withholdingStmt{inner: s, query: query}, nil
}

func (c *withholdingConn) Close() error { return c.inner.Close() }

func (c *withholdingConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

func (c *withholdingConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if b, ok := c.inner.(driver.ConnBeginTx); ok {
		return b.BeginTx(ctx, opts)
	}
	return c.inner.Begin()
}

func (c *withholdingConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if heldInArgs(query, args) {
		return skippedWrite{}, nil
	}
	if e, ok := c.inner.(driver.ExecerContext); ok {
		return e.ExecContext(ctx, query, args)
	}
	return nil, driver.ErrSkip
}

func (c *withholdingConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if heldInArgs(query, args) {
		return nil, errHeldValueRefused
	}
	if q, ok := c.inner.(driver.QueryerContext); ok {
		return q.QueryContext(ctx, query, args)
	}
	return nil, driver.ErrSkip
}

func (c *withholdingConn) Ping(ctx context.Context) error {
	if p, ok := c.inner.(driver.Pinger); ok {
		return p.Ping(ctx)
	}
	return nil
}

func (c *withholdingConn) ResetSession(ctx context.Context) error {
	if r, ok := c.inner.(driver.SessionResetter); ok {
		return r.ResetSession(ctx)
	}
	return nil
}

func (c *withholdingConn) IsValid() bool {
	if v, ok := c.inner.(driver.Validator); ok {
		return v.IsValid()
	}
	return true
}

type withholdingStmt struct {
	inner driver.Stmt
	query string
}

func (s *withholdingStmt) Close() error  { return s.inner.Close() }
func (s *withholdingStmt) NumInput() int { return s.inner.NumInput() }

func (s *withholdingStmt) Exec(args []driver.Value) (driver.Result, error) {
	return s.ExecContext(context.Background(), namedValues(args))
}

func (s *withholdingStmt) Query(args []driver.Value) (driver.Rows, error) {
	return s.QueryContext(context.Background(), namedValues(args))
}

func (s *withholdingStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	if heldInArgs(s.query, args) {
		return skippedWrite{}, nil
	}
	if e, ok := s.inner.(driver.StmtExecContext); ok {
		return e.ExecContext(ctx, args)
	}
	return s.inner.Exec(plainValues(args))
}

func (s *withholdingStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	if heldInArgs(s.query, args) {
		return nil, errHeldValueRefused
	}
	if q, ok := s.inner.(driver.StmtQueryContext); ok {
		return q.QueryContext(ctx, args)
	}
	return s.inner.Query(plainValues(args))
}

func namedValues(args []driver.Value) []driver.NamedValue {
	out := make([]driver.NamedValue, len(args))
	for i, v := range args {
		out[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
	}
	return out
}

func plainValues(args []driver.NamedValue) []driver.Value {
	out := make([]driver.Value, len(args))
	for i, a := range args {
		out[i] = a.Value
	}
	return out
}
