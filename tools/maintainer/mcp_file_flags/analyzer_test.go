// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestFixtureBothDirections runs the analyzer on testdata/fixture, where every
// flag named want-* names a filesystem path through a different data-flow
// shape, every flag named comp-* only ever becomes a component inside a
// directory the program chose, and every flag named not-* reaches no path (it
// is printed, hashed, parsed as a number, sent to an API, used only to select a
// fixed path, or filtered to a safe alphabet). A check that only ever reports,
// or never reports, fails one half of this test.
func TestFixtureBothDirections(t *testing.T) {
	rep, err := analyzeModule("testdata/fixture", 40, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Errors) > 0 || !rep.Converged {
		t.Fatalf("fixture did not analyse cleanly: converged=%v errors=%v", rep.Converged, rep.Errors)
	}
	found := map[string]bool{}
	for _, f := range rep.Flags {
		found[f.Name] = true
	}
	component := map[string]bool{}
	for _, f := range rep.FlagComponents {
		component[f.Name] = true
	}
	declared := fixtureFlagNames(t)
	var wants, comps, nots int
	for _, name := range declared {
		switch {
		case strings.HasPrefix(name, "want-"):
			wants++
			if !found[name] {
				t.Errorf("--%s names a filesystem path in the fixture but was not reported", name)
			}
		case strings.HasPrefix(name, "comp-"):
			comps++
			if found[name] {
				t.Errorf("--%s is only a path component but was reported as naming a path", name)
			}
			if !component[name] {
				t.Errorf("--%s is a path component but was not reported as one", name)
			}
		case strings.HasPrefix(name, "not-"):
			nots++
			if found[name] || component[name] {
				t.Errorf("--%s never reaches a filesystem path but was reported", name)
			}
		}
	}
	if wants < 18 || comps < 3 || nots < 6 {
		t.Fatalf("fixture lost cases: %d want-, %d comp-, %d not- flags", wants, comps, nots)
	}
	var names []string
	for n := range found {
		names = append(names, n)
	}
	sort.Strings(names)
	t.Logf("reported: %v", names)
}

// TestMCPHandlerRootConfinement runs the analyzer on the in-process MCP
// handlers in testdata/fixture/mcpsrv, which read files through an os.Root.
// A handler named handleWant* lets the MCP caller name a host path (the root
// is opened at a value the caller sent, or a plain os.Open sits beside a
// confined root) and must be reported as a path; a handler named handleComp*
// only names an entry inside a root the operator chose (the environment, a
// constant) and must be reported as a component, never as a path.
func TestMCPHandlerRootConfinement(t *testing.T) {
	rep, err := analyzeModule("testdata/fixture", 40, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Errors) > 0 || !rep.Converged {
		t.Fatalf("fixture did not analyse cleanly: converged=%v errors=%v", rep.Converged, rep.Errors)
	}
	path := map[string]bool{}
	for _, f := range rep.MCP {
		path[f.Func] = true
	}
	component := map[string]bool{}
	for _, f := range rep.MCPComponents {
		component[f.Func] = true
	}
	var wants, comps int
	for _, name := range fixtureHandlerNames(t) {
		fn := "mcpsrv." + name
		switch {
		case strings.HasPrefix(name, "handleWant"):
			wants++
			if !path[fn] {
				t.Errorf("%s lets the MCP caller name a host path but was not reported", name)
			}
		case strings.HasPrefix(name, "handleComp"):
			comps++
			if path[fn] {
				t.Errorf("%s only names an entry inside the operator's root but was reported as naming a path", name)
			}
			if !component[fn] {
				t.Errorf("%s names an entry inside a root but was not reported as a component", name)
			}
		}
	}
	if wants < 3 || comps < 2 {
		t.Fatalf("MCP fixture lost cases: %d handleWant, %d handleComp", wants, comps)
	}
}

// fixtureHandlerNames lists the handleWant*/handleComp* functions in the MCP
// fixture, so a case added there is checked without editing this test.
func fixtureHandlerNames(t *testing.T) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "testdata/fixture/mcpsrv/mcpsrv.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if ok && fd.Recv == nil && (strings.HasPrefix(fd.Name.Name, "handleWant") || strings.HasPrefix(fd.Name.Name, "handleComp")) {
			out = append(out, fd.Name.Name)
		}
	}
	return out
}

// fixtureFlagNames reads every string literal starting want-, comp- or not- from the
// fixture source, so a case added there is checked without editing this test.
func fixtureFlagNames(t *testing.T) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "testdata/fixture/cli/cli.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		s, err := strconv.Unquote(lit.Value)
		if err == nil && (strings.HasPrefix(s, "want-") || strings.HasPrefix(s, "comp-") || strings.HasPrefix(s, "not-")) && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
		return true
	})
	return out
}
