// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-sqlite-every-handle):
// every SQLite handle the program opens checks its statements through the
// shared credential kit, reads included (contract sections 2 and 5 cover SQLite
// statement text and bound values). Class credential-split-across-sql-bindings
// (review r2 P1): a held value split across two bound values was stored in halves.

package store

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"zoom-phone-pp-cli/internal/credkit"
)

// sqlSinkHeld is held by this test process only. It shares no 8-character
// piece with any test name, path or statement elsewhere in this package.
const sqlSinkHeld = "sqlsinkheld7Qz4kW"

// Every store opener, read-write and read-only, refuses a statement whose
// text or bound values carry a held credential, before SQLite runs it.
func TestEveryStoreOpenChecksStatementsThroughTheKit(t *testing.T) {
	if inFreshProcess(t) {
		return
	}
	credkit.Hold("config:auth_header", sqlSinkHeld)
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "data.db")
	rw, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := rw.Close(); err != nil {
		t.Fatal(err)
	}
	openers := map[string]func() (*Store, error){
		"Open":                func() (*Store, error) { return Open(path) },
		"OpenWithContext":     func() (*Store, error) { return OpenWithContext(ctx, path) },
		"OpenReadOnly":        func() (*Store, error) { return OpenReadOnly(path) },
		"OpenReadOnlyContext": func() (*Store, error) { return OpenReadOnlyContext(ctx, path) },
		"OpenReadOnlyValueCapped": func() (*Store, error) {
			return OpenReadOnlyValueCapped(path, 1<<20)
		},
	}
	for name, open := range openers {
		s, err := open()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var got string
		err = s.DB().QueryRowContext(ctx, "SELECT '"+sqlSinkHeld+"' AS sample").Scan(&got)
		if !errors.Is(err, credkit.ErrWithheld) {
			t.Errorf("%s ran a statement whose text carries a held credential (err %v, row %q)", name, err, got)
		}
		got = ""
		err = s.DB().QueryRowContext(ctx, "SELECT ? AS sample", sqlSinkHeld).Scan(&got)
		if !errors.Is(err, credkit.ErrWithheld) {
			t.Errorf("%s ran a statement whose bound value is a held credential (err %v, row %q)", name, err, got)
		}
		var one int
		if err := s.DB().QueryRowContext(ctx, "SELECT 1").Scan(&one); err != nil || one != 1 {
			t.Errorf("%s refused an ordinary statement: %v", name, err)
		}
		_ = s.Close()
	}
}

// Class credential-split-across-sql-bindings: a held credential split across two
// bound values of one statement is refused and stored nowhere, on a read-write
// handle and a read-only one.
func TestCredentialSplitAcrossBindingsIsRefused(t *testing.T) {
	if inFreshProcess(t) {
		return
	}
	const held = "splitBoundValueHeld9Xq7Lm2Pv4Tz"
	credkit.Hold("config:auth_header", held)
	a, b := held[:len(held)/2], held[len(held)/2:]
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "data.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.DB().ExecContext(ctx, `CREATE TABLE split_probe(a TEXT, b TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().ExecContext(ctx, `INSERT INTO split_probe(a,b) VALUES (?,?)`, a, b); !errors.Is(err, credkit.ErrWithheld) {
		t.Errorf("the write ran with a held value split across its bound values (err %v)", err)
	}
	var n int
	if err := s.DB().QueryRowContext(ctx, `SELECT count(*) FROM split_probe`).Scan(&n); err != nil || n != 0 {
		t.Errorf("%d row(s) stored (err %v)", n, err)
	}
	if _, err := s.DB().ExecContext(ctx, `INSERT INTO split_probe(a,b) VALUES (?,?)`, "ordinary", "values"); err != nil {
		t.Errorf("an ordinary write was refused: %v", err)
	}
	ro, err := OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	var got string
	err = ro.DB().QueryRowContext(ctx, `SELECT ? || ?`, a, b).Scan(&got)
	if !errors.Is(err, credkit.ErrWithheld) {
		t.Errorf("a read-only handle ran a statement with a held value split across its bound values (err %v, row %q)", err, got)
	}
}

// Class store-write-bypasses-chokepoint (review r1 P1): every table takes a
// statement through the kit, so a note repeating a held credential never reaches
// the learnings an MCP search reads, and an ordinary one does.
func TestLearningRepeatingAHeldValueIsNotStored(t *testing.T) {
	if inFreshProcess(t) {
		return
	}
	const held = "learnHeldCredential7Kq2Wz9"
	credkit.Hold("env:ZOOM_PHONE_API_SECRET", held)
	s, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if _, err := s.DB().ExecContext(ctx, `CREATE VIRTUAL TABLE IF NOT EXISTS probe_learnings USING fts5(notes)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().ExecContext(ctx, `INSERT INTO probe_learnings(notes) VALUES (?)`, "rotated "+held); !errors.Is(err, credkit.ErrWithheld) {
		t.Errorf("a note repeating a held credential was written (err %v)", err)
	}
	if _, err := s.DB().ExecContext(ctx, `INSERT INTO probe_learnings(notes) VALUES (?)`, "rotated on schedule"); err != nil {
		t.Errorf("an ordinary note was refused: %v", err)
	}
}

// No non-test file opens a SQLite handle the kit does not wrap: every
// sql.Open names the kit-wrapped driver constant, every sql.Register wraps
// its driver in credkit.SQLDriver, and no code reaches modernc's driver or
// its connection directly (sqlite.Driver, sqlite.Limit) outside the one
// package that registers the wrapped driver.
func TestNoSQLiteHandleBypassesTheKit(t *testing.T) {
	root := filepath.Join("..", "..")
	fset := token.NewFileSet()
	var bad []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == "." {
				return nil
			}
			if rel == "internal/credkit" || strings.HasPrefix(d.Name(), ".") || d.Name() == "testdata" || d.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, p, nil, 0)
		if err != nil {
			return err
		}
		sqlName, sqliteName := "", ""
		for _, im := range f.Imports {
			path, _ := strconv.Unquote(im.Path.Value)
			name := ""
			if im.Name != nil {
				name = im.Name.Name
			}
			switch path {
			case "database/sql":
				sqlName = "sql"
				if name != "" {
					sqlName = name
				}
			case "modernc.org/sqlite":
				sqliteName = "sqlite"
				if name != "" {
					sqliteName = name
				}
			}
		}
		owner := strings.HasPrefix(rel, "internal/credsql/")
		ast.Inspect(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.CallExpr:
				sel, ok := x.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				pkg, ok := sel.X.(*ast.Ident)
				if !ok || sqlName == "" || pkg.Name != sqlName {
					return true
				}
				where := fset.Position(x.Pos()).String()
				switch sel.Sel.Name {
				case "Open":
					if len(x.Args) == 0 || !kitDriverName(x.Args[0], owner) {
						bad = append(bad, where+": sql.Open with a driver that is not the kit-wrapped credsql.Driver")
					}
				case "OpenDB":
					if !owner {
						bad = append(bad, where+": sql.OpenDB from a connector the kit does not wrap")
					}
				case "Register":
					if len(x.Args) != 2 || !isKitSQLDriverCall(x.Args[1]) {
						bad = append(bad, where+": sql.Register without credkit.SQLDriver")
					}
				}
			case *ast.SelectorExpr:
				pkg, ok := x.X.(*ast.Ident)
				if ok && sqliteName != "" && pkg.Name == sqliteName && !owner {
					switch x.Sel.Name {
					case "Driver", "Limit", "RegisterConnectionHook", "NewBackup", "NewRestore":
						bad = append(bad, fset.Position(x.Pos()).String()+": modernc sqlite."+x.Sel.Name+" outside internal/credsql")
					}
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range bad {
		t.Error(b)
	}
}

func kitDriverName(e ast.Expr, owner bool) bool {
	if sel, ok := e.(*ast.SelectorExpr); ok {
		pkg, ok := sel.X.(*ast.Ident)
		return ok && pkg.Name == "credsql" && sel.Sel.Name == "Driver"
	}
	id, ok := e.(*ast.Ident)
	return ok && owner && id.Name == "Driver"
}

func isKitSQLDriverCall(e ast.Expr) bool {
	c, ok := e.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := c.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "credkit" && sel.Sel.Name == "SQLDriver"
}
