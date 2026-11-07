// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Command constgen writes a connector's credential-kit constants table
// (consttable.go): public text and every SQL statement its Go source
// carries as a string literal. vendor_credkit.py copies it into the
// connector, with the kit's import path, and runs it there, so the table
// is built with the connector's own Go and golang.org/x/text.
//
//	go run ./internal/credkitconstgen -root . -pkg credsql -kit example.com/x/internal/credkit
package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"credkit"
)

// sqlWords is every first word a SQLite statement can start with.
var sqlWords = map[string]bool{
	"ALTER": true, "ANALYZE": true, "ATTACH": true, "BEGIN": true, "COMMIT": true, "CREATE": true,
	"DELETE": true, "DETACH": true, "DROP": true, "END": true, "EXPLAIN": true, "INSERT": true,
	"PRAGMA": true, "REINDEX": true, "RELEASE": true, "REPLACE": true, "ROLLBACK": true,
	"SAVEPOINT": true, "SELECT": true, "UPDATE": true, "VACUUM": true, "VALUES": true, "WITH": true,
}

func isSQL(s string) bool {
	s = strings.TrimLeft(s, " \t\r\n(")
	i := strings.IndexFunc(s, func(r rune) bool { return !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z') })
	if i < 0 {
		i = len(s)
	}
	return sqlWords[strings.ToUpper(s[:i])]
}

func main() {
	root := flag.String("root", ".", "the connector's module directory")
	pkg := flag.String("pkg", "", "the generated file's package name")
	kit := flag.String("kit", "", "the kit's import path")
	flag.Parse()
	var units []string
	err := filepath.WalkDir(*root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if n := d.Name(); n == "credkit" || n == "testdata" || n == "vendor" || strings.HasPrefix(n, ".") && p != *root {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), p, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if l, ok := n.(*ast.BasicLit); ok && l.Kind == token.STRING {
				// A statement with a placeholder runs with bound values, so
				// it is never a unit of its text alone.
				if s, err := strconv.Unquote(l.Value); err == nil && isSQL(s) && !strings.Contains(s, "?") {
					units = append(units, s)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "constgen:", err)
		os.Exit(1)
	}
	slices.Sort(units)
	units = slices.Compact(units)
	table, err := credkit.ConstantTable(units, true)
	if err != nil {
		fmt.Fprintln(os.Stderr, "constgen:", err)
		os.Exit(1)
	}
	fmt.Printf(`// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package %s

// The credential kit's constants table for this connector (credkit
// consttable.go): its public text and the %d SQL statements its source
// carries, so a process reads their verdict instead of decoding them again.
// Written by tools/maintainer/vendor_credkit.py; it holds nothing derived
// from a credential, and the kit ignores it once the kit, Go or
// golang.org/x/text changes.

import "%s"

func init() { credkit.RegisterConstants(constantsTable) }

const constantsTable = %q
`, *pkg, len(units), *kit, table)
}
