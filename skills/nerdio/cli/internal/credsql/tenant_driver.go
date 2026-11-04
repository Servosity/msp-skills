// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (handfixes.json tenant-secrets-shown-never-stored): inside the
// credential kit's driver, every statement runs through this wrapper, which
// refuses a write whose bound value repeats a tenant secret this process showed
// (cliutil.TenantSecretEchoes: a LAPS password, a BitLocker key, a secure
// variable's value), so no store write, by any command or table, keeps one. The
// kit judges held credentials; tenant secrets are customer data the kit does
// not hold, because the operator's own output shows them. A read passes: it
// stores nothing, and a lookup must match what the operator typed.

package credsql

import (
	"context"
	"database/sql/driver"
	"errors"
	"regexp"

	"nerdio-pp-cli/internal/cliutil"
)

// errStoreWithheld names the refusal without repeating the value.
var errStoreWithheld = errors.New("store write withheld: a value repeats a tenant secret this program showed")

// writeStatement matches SQL text that can write, whichever call carries it
// (INSERT ... RETURNING runs through a query).
var writeStatement = regexp.MustCompile(`(?i)\b(insert|update|delete|replace)\b`)

type guardDriver struct{ inner driver.Driver }

func (d guardDriver) Open(name string) (driver.Conn, error) {
	c, err := d.inner.Open(name)
	if err != nil {
		return nil, err
	}
	return &guardConn{Conn: c}, nil
}

func guardStatement(query string, args []driver.NamedValue) error {
	if !writeStatement.MatchString(query) {
		return nil
	}
	if cliutil.TenantSecretEchoes(query) {
		return errStoreWithheld
	}
	for _, a := range args {
		switch v := a.Value.(type) {
		case string:
			if cliutil.TenantSecretEchoes(v) {
				return errStoreWithheld
			}
		case []byte:
			if cliutil.TenantSecretEchoes(string(v)) {
				return errStoreWithheld
			}
		}
	}
	return nil
}

type guardConn struct{ driver.Conn }

func (c *guardConn) Prepare(query string) (driver.Stmt, error) {
	return c.PrepareContext(context.Background(), query)
}

func (c *guardConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	if err := guardStatement(query, nil); err != nil {
		return nil, err
	}
	var s driver.Stmt
	var err error
	if pc, ok := c.Conn.(driver.ConnPrepareContext); ok {
		s, err = pc.PrepareContext(ctx, query)
	} else {
		s, err = c.Conn.Prepare(query)
	}
	if err != nil {
		return nil, err
	}
	return &guardStmt{Stmt: s, query: query}, nil
}

func (c *guardConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	ec, ok := c.Conn.(driver.ExecerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	if err := guardStatement(query, args); err != nil {
		return nil, err
	}
	return ec.ExecContext(ctx, query, args)
}

func (c *guardConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	qc, ok := c.Conn.(driver.QueryerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	if err := guardStatement(query, args); err != nil {
		return nil, err
	}
	return qc.QueryContext(ctx, query, args)
}

func (c *guardConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if bc, ok := c.Conn.(driver.ConnBeginTx); ok {
		return bc.BeginTx(ctx, opts)
	}
	return c.Conn.Begin() //nolint:staticcheck // fallback for a driver without BeginTx
}

func (c *guardConn) Ping(ctx context.Context) error {
	if p, ok := c.Conn.(driver.Pinger); ok {
		return p.Ping(ctx)
	}
	return nil
}

func (c *guardConn) ResetSession(ctx context.Context) error {
	if r, ok := c.Conn.(driver.SessionResetter); ok {
		return r.ResetSession(ctx)
	}
	return nil
}

func (c *guardConn) IsValid() bool {
	if v, ok := c.Conn.(driver.Validator); ok {
		return v.IsValid()
	}
	return true
}

type guardStmt struct {
	driver.Stmt
	query string
}

func (s *guardStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	if err := guardStatement(s.query, args); err != nil {
		return nil, err
	}
	if ec, ok := s.Stmt.(driver.StmtExecContext); ok {
		return ec.ExecContext(ctx, args)
	}
	return s.Stmt.Exec(namedValues(args)) //nolint:staticcheck // fallback for a driver without StmtExecContext
}

func (s *guardStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	if err := guardStatement(s.query, args); err != nil {
		return nil, err
	}
	if qc, ok := s.Stmt.(driver.StmtQueryContext); ok {
		return qc.QueryContext(ctx, args)
	}
	return s.Stmt.Query(namedValues(args)) //nolint:staticcheck // fallback for a driver without StmtQueryContext
}

func namedValues(args []driver.NamedValue) []driver.Value {
	out := make([]driver.Value, len(args))
	for i, a := range args {
		out[i] = a.Value
	}
	return out
}
