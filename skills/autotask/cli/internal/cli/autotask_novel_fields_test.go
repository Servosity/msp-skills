// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Codex r1 class wrong-field-read-from-a-documented-response: account-brief
// read an opportunity's lastActivityDate, which OpportunityModel does not
// carry (its field is lastActivity), so every recently changed opportunity
// was left out. Every field name a command reads from a synced record (each
// key passed to strAt, intAt, numAt, boolAt or timeAt) must be a property of
// that record's spec.json model (the model its POST /<Entity>/query returns),
// or a case variant of one read in the same call. A record is traced from
// listEntity(db, "<resource>") through range loops and into the helpers it is
// passed to; a read the trace cannot place must be documented by some model.
func TestRecordFieldsReadAreDocumented(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "spec.json"))
	if os.IsNotExist(err) {
		// The spec source is not public (press/publication.json), so the
		// monorepo carries no spec.json; the press tree that has it runs this.
		t.Skip("spec.json is not vendored into this tree")
	}
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Paths map[string]map[string]struct {
			Responses map[string]struct {
				Schema struct {
					Ref string `json:"$ref"`
				} `json:"schema"`
			} `json:"responses"`
		} `json:"paths"`
		Definitions map[string]struct {
			Properties map[string]json.RawMessage `json:"properties"`
		} `json:"definitions"`
	}
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	anyModel := map[string]bool{}
	for _, d := range spec.Definitions {
		for p := range d.Properties {
			anyModel[p] = true
		}
	}
	modelRef := regexp.MustCompile(`\[(\w+),`)
	modelOf := func(resource string) map[string]bool {
		path, err := syncResourcePath(resource)
		if err != nil {
			t.Fatalf("listEntity reads %q, which sync does not walk: %v", resource, err)
		}
		m := modelRef.FindStringSubmatch(spec.Paths[path]["post"].Responses["200"].Schema.Ref)
		if m == nil {
			t.Fatalf("%s (%s) returns no documented model", resource, path)
		}
		props := map[string]bool{}
		for p := range spec.Definitions[m[1]].Properties {
			props[p] = true
		}
		return props
	}

	fset := token.NewFileSet()
	var files []*ast.File
	names, _ := filepath.Glob("*.go")
	for _, f := range names {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, file)
	}
	accessors := map[string]bool{"strAt": true, "intAt": true, "numAt": true, "boolAt": true, "timeAt": true}
	// helperOf: function name -> resource of the record its first parameter receives.
	helperOf := map[string]string{}
	// trace walks one function body; read is called for every accessor call on a record it can place.
	trace := func(fn *ast.FuncDecl, read func(call *ast.CallExpr, resource string)) {
		env := map[string]string{} // ident -> resource, for both record slices and records
		if r := helperOf[fn.Name.Name]; r != "" && len(fn.Type.Params.List) > 0 && len(fn.Type.Params.List[0].Names) > 0 {
			env[fn.Type.Params.List[0].Names[0].Name] = r
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.AssignStmt:
				if call, ok := n.Rhs[0].(*ast.CallExpr); ok && len(n.Rhs) == 1 {
					if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "listEntity" && len(call.Args) == 2 {
						if lit, ok := call.Args[1].(*ast.BasicLit); ok {
							r, _ := strconv.Unquote(lit.Value)
							env[n.Lhs[0].(*ast.Ident).Name] = r
						}
					}
				}
			case *ast.RangeStmt:
				if x, ok := n.X.(*ast.Ident); ok && env[x.Name] != "" {
					if v, ok := n.Value.(*ast.Ident); ok {
						env[v.Name] = env[x.Name]
					}
				}
			case *ast.CallExpr:
				id, ok := n.Fun.(*ast.Ident)
				if !ok || len(n.Args) == 0 {
					return true
				}
				arg, ok := n.Args[0].(*ast.Ident)
				if !ok || env[arg.Name] == "" {
					return true
				}
				if accessors[id.Name] {
					read(n, env[arg.Name])
				} else if id.Name != "listEntity" {
					helperOf[id.Name] = env[arg.Name]
				}
			}
			return true
		})
	}
	var funcs []*ast.FuncDecl
	for _, f := range files {
		for _, d := range f.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok && fn.Body != nil {
				funcs = append(funcs, fn)
			}
		}
	}
	for pass := 0; pass < 3; pass++ { // helpers calling helpers
		for _, fn := range funcs {
			trace(fn, func(*ast.CallExpr, string) {})
		}
	}

	keysOf := func(call *ast.CallExpr) []string {
		var keys []string
		for _, a := range call.Args[1:] {
			if lit, ok := a.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				k, _ := strconv.Unquote(lit.Value)
				keys = append(keys, k)
			}
		}
		return keys
	}
	var bad []string
	check := func(call *ast.CallExpr, documented map[string]bool, model string) {
		keys := keysOf(call)
		for _, k := range keys {
			variant := documented[k]
			for _, o := range keys {
				variant = variant || (o != k && documented[o] && strings.EqualFold(o, k))
			}
			if !variant {
				bad = append(bad, fset.Position(call.Pos()).String()+" "+model+" "+strconv.Quote(k))
			}
		}
	}
	placed := map[*ast.CallExpr]bool{}
	for _, fn := range funcs {
		trace(fn, func(call *ast.CallExpr, resource string) {
			placed[call] = true
			check(call, modelOf(resource), resource)
		})
	}
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || placed[call] || len(call.Args) < 2 {
				return true
			}
			if id, ok := call.Fun.(*ast.Ident); ok && accessors[id.Name] {
				check(call, anyModel, "(any model)")
			}
			return true
		})
	}
	sort.Strings(bad)
	if len(bad) > 0 {
		t.Errorf("fields read that the record's spec.json model does not document:\n  %s", strings.Join(bad, "\n  "))
	}
}
