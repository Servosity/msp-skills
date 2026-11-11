// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// SQLite (owner GO 2026-11-09, synccpu): the store is not checked like the
// other sinks. The full check cost about 1 CPU-s per stored row and was 99.8%
// of a sync's CPU, so SQLDriver passes every statement through EXCEPT one
// rule: while a value from a declared vendor response field (ResponseFields,
// source "response:*") is held, a statement whose string or []byte argument
// carries that whole value, raw or JSON-escaped, is refused. A vendor secret
// then never persists to print unheld in a later run. No decoders, no pieces;
// one atomic load while no response value is held. Accepted risk: a
// credential from any other source can sit in the local store unchecked.
//
//	sql.Register("credkit-sqlite", credkit.SQLDriver(&sqlite.Driver{}))
//	db, err := sql.Open("credkit-sqlite", path)

import (
	"bytes"
	"context"
	"database/sql/driver"
	"encoding/json"
	"slices"
	"strings"
	"sync/atomic"
)

// storeArmed is set once a response value is held or the held set becomes
// unknown; until then every statement runs unread.
var storeArmed atomic.Bool

// storeRefuses reports whether args must not be stored: one carries a held
// response value whole, raw or as encoding/json writes it, or the held set
// is unknown (fail closed, as before).
func storeRefuses(args []driver.NamedValue) bool {
	if !storeArmed.Load() {
		return false
	}
	needles, unknown := storeNeedles()
	if unknown {
		return true
	}
	for _, a := range args {
		switch v := a.Value.(type) {
		case string:
			for _, n := range needles {
				if strings.Contains(v, n) {
					return true
				}
			}
		case []byte:
			for _, n := range needles {
				if bytes.Contains(v, []byte(n)) {
					return true
				}
			}
		}
	}
	return false
}

// storeNeedles is every held response value, raw and JSON-escaped (with and
// without encoding/json's HTML escapes), cached per held-set generation.
// (The JSON words true, false and null are public text: never held from a
// response, so never needles.) ponytail: a very short held value refuses every
// row carrying it; add a minimum length if a connector ever declares one.

// noHTMLEscape turns encoding/json's default output into its
// SetEscapeHTML(false) form: it undoes the \u003c, \u003e and \u0026
// escapes and nothing else, walking escape pairs so an escaped backslash
// followed by "u003c" (a literal in the value) stays as it is.
func noHTMLEscape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		if r := map[string]byte{"u003c": '<', "u003e": '>', "u0026": '&'}[s[i+1:min(i+6, len(s))]]; r != 0 {
			b.WriteByte(r)
			i += 5
			continue
		}
		b.WriteString(s[i : i+2])
		i++
	}
	return b.String()
}

func storeNeedles() ([]string, bool) {
	reg.Lock()
	defer reg.Unlock()
	if reg.unknown {
		return nil, true
	}
	if reg.sgen != reg.gen {
		reg.store = reg.store[:0]
		add := func(n string) {
			if n != "" && !slices.Contains(reg.store, n) {
				reg.store = append(reg.store, n)
			}
		}
		for v, e := range reg.vals {
			if !e.resp {
				continue
			}
			add(v)
			if b, err := json.Marshal(v); err == nil {
				add(string(b[1 : len(b)-1]))               // encoding/json's default
				add(noHTMLEscape(string(b[1 : len(b)-1]))) // SetEscapeHTML(false)
			}
		}
		reg.sgen = reg.gen
	}
	return reg.store, false
}

// SQLDriver wraps inner so each statement's arguments meet the one store rule.
func SQLDriver(inner driver.Driver) driver.Driver { return sqlDriver{inner} }

type sqlDriver struct{ inner driver.Driver }

func (d sqlDriver) Open(name string) (driver.Conn, error) {
	c, err := d.inner.Open(name)
	if err != nil {
		return nil, err
	}
	return &sqlConn{c}, nil
}

// sqlConn passes everything through; the statements it prepares and the
// ones it runs directly meet the store rule.
type sqlConn struct{ driver.Conn }

func (c *sqlConn) Prepare(query string) (driver.Stmt, error) {
	s, err := c.Conn.Prepare(query)
	if err != nil {
		return nil, err
	}
	return &sqlStmt{s}, nil
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
	return &sqlStmt{s}, nil
}

func (c *sqlConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	e, ok := c.Conn.(driver.ExecerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	if storeRefuses(args) {
		return nil, withheldError{SinkSQLite}
	}
	return e.ExecContext(ctx, query, args)
}

func (c *sqlConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	q, ok := c.Conn.(driver.QueryerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	if storeRefuses(args) {
		return nil, withheldError{SinkSQLite}
	}
	return q.QueryContext(ctx, query, args)
}

func (c *sqlConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if bt, ok := c.Conn.(driver.ConnBeginTx); ok {
		return bt.BeginTx(ctx, opts)
	}
	return c.Conn.Begin() //nolint:staticcheck // the fallback database/sql itself uses
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

type sqlStmt struct{ driver.Stmt }

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

func (s *sqlStmt) Exec(args []driver.Value) (driver.Result, error) {
	if storeRefuses(named(args)) {
		return nil, withheldError{SinkSQLite}
	}
	return s.Stmt.Exec(args) //nolint:staticcheck // the driver.Stmt interface method
}

func (s *sqlStmt) Query(args []driver.Value) (driver.Rows, error) {
	if storeRefuses(named(args)) {
		return nil, withheldError{SinkSQLite}
	}
	return s.Stmt.Query(args) //nolint:staticcheck // the driver.Stmt interface method
}

func (s *sqlStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	if storeRefuses(args) {
		return nil, withheldError{SinkSQLite}
	}
	if e, ok := s.Stmt.(driver.StmtExecContext); ok {
		return e.ExecContext(ctx, args)
	}
	return s.Stmt.Exec(values(args)) //nolint:staticcheck // the fallback database/sql itself uses
}

func (s *sqlStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	if storeRefuses(args) {
		return nil, withheldError{SinkSQLite}
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
