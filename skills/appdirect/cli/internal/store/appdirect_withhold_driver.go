// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package store

// appdirect handfix (store-never-keeps-secrets, every SQL write): the
// read-write store opens SQLite through WithholdingSQLiteDriver (one line in
// OpenWithContext), so every statement it executes passes the output
// chokepoint before SQLite sees it, on every table and from every writer:
// resource rows and typed tables, learnings, playbooks, learn events,
// candidates, sync state, and any writer added later. Each string argument
// passes cliutil.WithholdText and each byte argument cliutil.WithholdBytes.
// Statement text is built from identifiers only (values travel as
// arguments); a statement whose text still repeats a held credential of
// funnel length is refused, never rewritten. A query that writes (its text
// names INSERT, UPDATE, DELETE or REPLACE, as INSERT ... RETURNING through
// QueryRow would) is treated as a write. Every other query passes unchanged:
// a lookup must match what the operator typed, and a read writes nothing.

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"regexp"

	"appdirect-pp-cli/internal/cliutil"
)

// WithholdingSQLiteDriver is the database/sql driver name of the wrapper.
const WithholdingSQLiteDriver = "sqlite-umbrella-withheld"

// errHeldInStatement refuses a statement whose SQL text repeats a credential.
var errHeldInStatement = errors.New("store write refused: the SQL statement repeats a credential this process holds")

func init() {
	db, err := sql.Open("sqlite", "")
	if err != nil {
		panic("store: sqlite driver not registered: " + err.Error())
	}
	inner := db.Driver()
	_ = db.Close()
	sql.Register(WithholdingSQLiteDriver, withholdingDriver{inner: inner})
}

type withholdingDriver struct{ inner driver.Driver }

func (d withholdingDriver) Open(name string) (driver.Conn, error) {
	c, err := d.inner.Open(name)
	if err != nil {
		return nil, err
	}
	return &withholdingConn{inner: c}, nil
}

// writeStatement matches SQL text that can write, whichever call carries it.
var writeStatement = regexp.MustCompile(`(?i)\b(insert|update|delete|replace)\b`)

// withheldStatement checks a statement's text before it executes.
func withheldStatement(query string) error {
	if cliutil.FunnelHeld(query) != query {
		return errHeldInStatement
	}
	return nil
}

// withheldValue passes one argument through the chokepoint.
func withheldValue(v driver.Value) driver.Value {
	switch t := v.(type) {
	case string:
		return cliutil.WithholdText(t)
	case []byte:
		return cliutil.WithholdBytes(t)
	}
	return v
}

func withheldNamed(args []driver.NamedValue) []driver.NamedValue {
	out := make([]driver.NamedValue, len(args))
	for i, a := range args {
		a.Value = withheldValue(a.Value)
		out[i] = a
	}
	return out
}

func withheldValues(args []driver.Value) []driver.Value {
	out := make([]driver.Value, len(args))
	for i, a := range args {
		out[i] = withheldValue(a)
	}
	return out
}

type withholdingConn struct{ inner driver.Conn }

func (c *withholdingConn) Prepare(query string) (driver.Stmt, error) {
	s, err := c.inner.Prepare(query)
	if err != nil {
		return nil, err
	}
	return &withholdingStmt{inner: s, query: query}, nil
}

func (c *withholdingConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	pc, ok := c.inner.(driver.ConnPrepareContext)
	if !ok {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return c.Prepare(query)
	}
	s, err := pc.PrepareContext(ctx, query)
	if err != nil {
		return nil, err
	}
	return &withholdingStmt{inner: s, query: query}, nil
}

func (c *withholdingConn) Close() error { return c.inner.Close() }

//nolint:staticcheck // driver.Conn requires Begin; BeginTx is preferred below.
func (c *withholdingConn) Begin() (driver.Tx, error) { return c.inner.Begin() } //nolint:staticcheck

func (c *withholdingConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if bt, ok := c.inner.(driver.ConnBeginTx); ok {
		return bt.BeginTx(ctx, opts)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return c.inner.Begin() //nolint:staticcheck
}

// ExecContext is every write that does not prepare first. Without the inner
// fast path, ErrSkip sends database/sql to Prepare, whose statement withholds.
func (c *withholdingConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	ec, ok := c.inner.(driver.ExecerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	if err := withheldStatement(query); err != nil {
		return nil, err
	}
	return ec.ExecContext(ctx, query, withheldNamed(args))
}

func (c *withholdingConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	qc, ok := c.inner.(driver.QueryerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	if writeStatement.MatchString(query) {
		if err := withheldStatement(query); err != nil {
			return nil, err
		}
		args = withheldNamed(args)
	}
	return qc.QueryContext(ctx, query, args)
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

func (c *withholdingConn) CheckNamedValue(nv *driver.NamedValue) error {
	if nc, ok := c.inner.(driver.NamedValueChecker); ok {
		return nc.CheckNamedValue(nv)
	}
	return driver.ErrSkip
}

type withholdingStmt struct {
	inner driver.Stmt
	query string
}

func (s *withholdingStmt) Close() error  { return s.inner.Close() }
func (s *withholdingStmt) NumInput() int { return s.inner.NumInput() }

//nolint:staticcheck // driver.Stmt requires Exec; ExecContext is preferred below.
func (s *withholdingStmt) Exec(args []driver.Value) (driver.Result, error) {
	if err := withheldStatement(s.query); err != nil {
		return nil, err
	}
	return s.inner.Exec(withheldValues(args)) //nolint:staticcheck
}

//nolint:staticcheck // driver.Stmt requires Query; QueryContext is preferred below.
func (s *withholdingStmt) Query(args []driver.Value) (driver.Rows, error) {
	if writeStatement.MatchString(s.query) {
		if err := withheldStatement(s.query); err != nil {
			return nil, err
		}
		args = withheldValues(args)
	}
	return s.inner.Query(args) //nolint:staticcheck
}

func (s *withholdingStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	if err := withheldStatement(s.query); err != nil {
		return nil, err
	}
	if ec, ok := s.inner.(driver.StmtExecContext); ok {
		return ec.ExecContext(ctx, withheldNamed(args))
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.inner.Exec(withheldValues(namedToValues(args))) //nolint:staticcheck
}

func (s *withholdingStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	if writeStatement.MatchString(s.query) {
		if err := withheldStatement(s.query); err != nil {
			return nil, err
		}
		args = withheldNamed(args)
	}
	if qc, ok := s.inner.(driver.StmtQueryContext); ok {
		return qc.QueryContext(ctx, args)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.inner.Query(namedToValues(args)) //nolint:staticcheck
}

func (s *withholdingStmt) CheckNamedValue(nv *driver.NamedValue) error {
	if nc, ok := s.inner.(driver.NamedValueChecker); ok {
		return nc.CheckNamedValue(nv)
	}
	return driver.ErrSkip
}

func namedToValues(args []driver.NamedValue) []driver.Value {
	out := make([]driver.Value, len(args))
	for i, a := range args {
		out[i] = a.Value
	}
	return out
}
