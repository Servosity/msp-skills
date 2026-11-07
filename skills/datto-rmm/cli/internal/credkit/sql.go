// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// Contracts 4 and 5 for SQLite: one statement, its text and its bound
// values, is one unit. SQLDriver wraps the SQLite driver so every statement
// is prepared through the wrapper (it offers no direct Exec or Query of its
// own for database/sql to go around it) and checked when it runs. A
// withheld statement does not run; its error is the notice.
//
//	sql.Register("credkit-sqlite", credkit.SQLDriver(&sqlite.Driver{}))
//	db, err := sql.Open("credkit-sqlite", path)

import (
	"context"
	"database/sql/driver"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// SQLDriver wraps inner so every statement is checked before it runs.
func SQLDriver(inner driver.Driver) driver.Driver { return sqlDriver{inner} }

type sqlDriver struct{ inner driver.Driver }

// openStatements is every statement a SQLite driver may build from the
// connection name's query (after the first '?', at index 1 or later, as
// modernc.org/sqlite reads it). Blunt rule, the class's second round: each
// _pragma value as "pragma <value>"; each other _ key that is not a plain
// setting, in every spelling a driver writes it ("pragma k = v",
// "pragma k=v", "pragma k(v)", "pragma k v"); the begin a _txlock gives.
// ok is false for a query the kit cannot parse: the name is withheld.
func openStatements(name string) (out []string, ok bool) {
	i := strings.IndexByte(name, '?')
	if i < 1 {
		return nil, true
	}
	q, err := url.ParseQuery(name[i+1:])
	if err != nil {
		return nil, false
	}
	for k, vs := range q {
		for _, v := range vs {
			switch k {
			case "_pragma":
				out = append(out, "pragma "+v)
			case "_txlock":
				out = append(out, "begin "+v)
			case "_time_format", "_inttotime", "_texttotime", "_timezone":
			default:
				if p, ok := strings.CutPrefix(k, "_"); ok {
					out = append(out, "pragma "+p+" = "+v, "pragma "+p+"="+v, "pragma "+p+"("+v+")", "pragma "+p+" "+v)
				}
			}
		}
	}
	return out, true
}

func (d sqlDriver) Open(name string) (driver.Conn, error) {
	defer emitEnd(emitBegin()) // the barrier, to the end of the call (held.go heldMu)
	// The name is a unit too, and so is every statement a driver builds
	// from it before any statement of the program's (openStatements).
	stmts, ok := openStatements(name)
	for _, u := range append([]string{name}, stmts...) {
		if !ok || withheld([]byte(u)) {
			return nil, withheldError{SinkSQLite}
		}
	}
	c, err := d.inner.Open(name)
	if err != nil {
		return nil, err
	}
	return &sqlConn{c}, nil
}

// sqlConn embeds the driver.Conn interface, so only Prepare, Close and Begin
// are promoted: the inner connection's ExecerContext and QueryerContext are
// hidden, and database/sql prepares every statement through Prepare.
type sqlConn struct{ driver.Conn }

func (c *sqlConn) Prepare(query string) (driver.Stmt, error) {
	s, err := c.Conn.Prepare(query)
	if err != nil {
		return nil, err
	}
	return &sqlStmt{s, query}, nil
}

func (c *sqlConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	pc, ok := c.Conn.(driver.ConnPrepareContext)
	if !ok {
		return c.Prepare(query)
	}
	s, err := pc.PrepareContext(ctx, query)
	if err != nil {
		return nil, err
	}
	return &sqlStmt{s, query}, nil
}

// checkTx checks the statements a driver runs for a transaction itself,
// each a unit (kit15rv: they ran unchecked): modernc.org/sqlite's begin
// (with the lock mode a DSN's _txlock names), commit and rollback.
func checkTx(units ...string) error {
	for _, u := range units {
		if withheld([]byte(u)) {
			return withheldError{SinkSQLite}
		}
	}
	return nil
}

func (c *sqlConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	defer emitEnd(emitBegin()) // the barrier, to the end of the call (held.go heldMu)
	if err := checkTx("begin", "begin deferred", "begin immediate", "begin exclusive"); err != nil {
		return nil, err
	}
	var tx driver.Tx
	var err error
	if bt, ok := c.Conn.(driver.ConnBeginTx); ok {
		tx, err = bt.BeginTx(ctx, opts)
	} else {
		tx, err = c.Conn.Begin() //nolint:staticcheck // the fallback database/sql itself uses
	}
	if err != nil {
		return nil, err
	}
	return sqlTx{tx}, nil
}

// Begin hides the inner connection's: every transaction is a sqlTx.
func (c *sqlConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

// sqlTx checks the statement each end of a transaction runs before the
// driver runs it; a withheld one is not run (fail closed).
type sqlTx struct{ driver.Tx }

// Commit checks rollback too: a driver may run it itself when the commit
// fails with the transaction still open (modernc tx.go, kit15rv round 2).
func (t sqlTx) Commit() error {
	defer emitEnd(emitBegin()) // the barrier, to the end of the call (held.go heldMu)
	if err := checkTx("commit", "rollback"); err != nil {
		return err
	}
	return t.Tx.Commit()
}

func (t sqlTx) Rollback() error {
	defer emitEnd(emitBegin()) // the barrier, to the end of the call (held.go heldMu)
	if err := checkTx("rollback"); err != nil {
		return err
	}
	return t.Tx.Rollback()
}

func (c *sqlConn) ResetSession(ctx context.Context) error {
	if r, ok := c.Conn.(driver.SessionResetter); ok {
		return r.ResetSession(ctx)
	}
	return nil
}

func (c *sqlConn) IsValid() bool {
	if v, ok := c.Conn.(driver.Validator); ok {
		return v.IsValid()
	}
	return true
}

func (c *sqlConn) CheckNamedValue(nv *driver.NamedValue) error {
	if n, ok := c.Conn.(driver.NamedValueChecker); ok {
		return n.CheckNamedValue(nv)
	}
	return driver.ErrSkip
}

type sqlStmt struct {
	driver.Stmt
	query string
}

// sqlUnit is one statement as a unit: its text, then each bound value on a
// line of its own.
func sqlUnit(query string, args []driver.NamedValue) []byte {
	var b strings.Builder
	b.WriteString(query)
	for _, a := range args {
		b.WriteString("\n")
		if a.Name != "" {
			b.WriteString(a.Name + "=")
		}
		switch v := a.Value.(type) {
		case nil:
		case string:
			b.WriteString(v)
		case []byte:
			b.Write(v)
		case int64:
			b.WriteString(strconv.FormatInt(v, 10))
		case float64:
			b.WriteString(strconv.FormatFloat(v, 'f', -1, 64))
		case time.Time:
			// Every spelling a driver stores: RFC 3339, and the default
			// String form with the zone's name (modernc SQLite's).
			b.WriteString(v.Format(time.RFC3339Nano) + "\n" + v.String())
		default:
			fmt.Fprint(&b, v)
		}
	}
	return []byte(b.String())
}

func named(args []driver.Value) []driver.NamedValue {
	out := make([]driver.NamedValue, len(args))
	for i, v := range args {
		out[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
	}
	return out
}

func values(args []driver.NamedValue) []driver.Value {
	out := make([]driver.Value, len(args))
	for i, a := range args {
		out[i] = a.Value
	}
	return out
}

func (s *sqlStmt) check(args []driver.NamedValue) error {
	if withheld(sqlUnit(s.query, args)) {
		return withheldError{SinkSQLite}
	}
	return nil
}

func (s *sqlStmt) Exec(args []driver.Value) (driver.Result, error) {
	defer emitEnd(emitBegin()) // the barrier, to the end of the call (held.go heldMu)
	if err := s.check(named(args)); err != nil {
		return nil, err
	}
	return s.Stmt.Exec(args) //nolint:staticcheck // the driver.Stmt interface method
}

func (s *sqlStmt) Query(args []driver.Value) (driver.Rows, error) {
	defer emitEnd(emitBegin()) // the barrier, to the end of the call (held.go heldMu)
	if err := s.check(named(args)); err != nil {
		return nil, err
	}
	return s.Stmt.Query(args) //nolint:staticcheck // the driver.Stmt interface method
}

func (s *sqlStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	defer emitEnd(emitBegin()) // the barrier, to the end of the call (held.go heldMu)
	if err := s.check(args); err != nil {
		return nil, err
	}
	if e, ok := s.Stmt.(driver.StmtExecContext); ok {
		return e.ExecContext(ctx, args)
	}
	return s.Stmt.Exec(values(args)) //nolint:staticcheck // the fallback database/sql itself uses
}

func (s *sqlStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	defer emitEnd(emitBegin()) // the barrier, to the end of the call (held.go heldMu)
	if err := s.check(args); err != nil {
		return nil, err
	}
	if q, ok := s.Stmt.(driver.StmtQueryContext); ok {
		return q.QueryContext(ctx, args)
	}
	return s.Stmt.Query(values(args)) //nolint:staticcheck // the fallback database/sql itself uses
}

func (s *sqlStmt) CheckNamedValue(nv *driver.NamedValue) error {
	if n, ok := s.Stmt.(driver.NamedValueChecker); ok {
		return n.CheckNamedValue(nv)
	}
	return driver.ErrSkip
}
