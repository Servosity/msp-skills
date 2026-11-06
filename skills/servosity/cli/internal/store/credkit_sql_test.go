// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-sqlite-every-handle):
// every SQLite handle the program opens checks its statements through the
// shared credential kit. Codex publish pass 2 round 2: OpenReadOnly used the
// bare "sqlite" driver, so MCP sql {"query":"SELECT '<held credential>' AS
// sample"} ran a credential-bearing statement unchecked (contract sections 2
// and 5 cover SQLite statement text and bound values, reads included).

package store

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"servosity-msp-pp-cli/internal/credkit"
)

// sqlSinkHeld is held by a child of the test binary only: a held value in this
// process would make every later store test pay the kit's check of the 1,000-plus
// migration statements. It shares no 8-character piece with any test name, path
// or statement elsewhere in this package.
const sqlSinkHeld = "sqlsinkheld7Qz4kW"

// Every store opener, read-write and read-only, refuses a statement whose
// text or bound values carry a held credential, before SQLite runs it.
func TestEveryStoreOpenChecksStatementsThroughTheKit(t *testing.T) {
	const childEnv = "STORE_SQL_SINK_HELD_PASS"
	if os.Getenv(childEnv) == "" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestEveryStoreOpenChecksStatementsThroughTheKit$", "-test.v")
		cmd.Env = append(os.Environ(), childEnv+"=1")
		out, err := cmd.CombinedOutput()
		if err != nil || !strings.Contains(string(out), "--- PASS: TestEveryStoreOpenChecksStatementsThroughTheKit") {
			t.Fatalf("the held pass failed: %v\n%s", err, out)
		}
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
