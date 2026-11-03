// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
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
	rep, err := analyzeModule("testdata/fixture", 40, "", false)
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
	rep, err := analyzeModule("testdata/fixture", 40, "", false)
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

// TestKitFlagValuesTiedOnlyWhenReviewed: the reviewed credential kit's
// FlagValues(os.Args, "--config") is the config flag's value (reported as a
// flag, which the MCP check then proves refused); a computed flag name and a
// connector's own argv scan stay os.Args reaching a file. With the kit not
// proven reviewed, its call is analysed as code and its read is os.Args too.
func TestKitFlagValuesTiedOnlyWhenReviewed(t *testing.T) {
	lines := map[string]string{}
	src, err := os.ReadFile("testdata/kitargv/cmd/app/main.go")
	if err != nil {
		t.Fatal(err)
	}
	for i, l := range strings.Split(string(src), "\n") {
		for _, tag := range []string{"TIED", "COMPUTED", "OWN", "BUILT"} {
			if strings.Contains(l, "// "+tag) {
				lines[tag] = "cmd/app/main.go:" + strconv.Itoa(i+1)
			}
		}
	}
	for _, reviewed := range []bool{true, false} {
		rep, err := analyzeModule("testdata/kitargv", 40, "", reviewed)
		if err != nil {
			t.Fatal(err)
		}
		if len(rep.Errors) > 0 || !rep.Converged {
			t.Fatalf("kitargv did not analyse cleanly: converged=%v errors=%v", rep.Converged, rep.Errors)
		}
		argv := map[string]bool{}
		for _, f := range rep.Other {
			if f.Kind == "os-args" {
				for _, s := range f.Sinks {
					argv[s.Pos] = true
				}
			}
		}
		flagSinks := map[string]bool{}
		for _, f := range rep.Flags {
			if f.Name == "config" {
				for _, s := range f.Sinks {
					flagSinks[s.Pos] = true
				}
			}
		}
		notesSinks := map[string]bool{}
		for _, f := range rep.Flags {
			if f.Name == "notes" {
				for _, s := range f.Sinks {
					notesSinks[s.Pos] = true
				}
			}
		}
		if !notesSinks[lines["BUILT"]] {
			t.Errorf("reviewed=%v: --notes inside an argv the program built for FlagValues was not reported: %v", reviewed, notesSinks)
		}
		if !argv[lines["COMPUTED"]] || !argv[lines["OWN"]] {
			t.Errorf("reviewed=%v: os.Args reaching a file outside a constant FlagValues was not reported: %v", reviewed, argv)
		}
		if reviewed && (argv[lines["TIED"]] || !flagSinks[lines["TIED"]]) {
			t.Errorf("reviewed kit: FlagValues(os.Args, \"--config\") was not tied to --config: os.Args sinks %v, config sinks %v", argv, flagSinks)
		}
		if !reviewed && (!argv[lines["TIED"]] || flagSinks[lines["TIED"]]) {
			t.Errorf("kit not proven reviewed: its FlagValues read was tied to a flag anyway: os.Args sinks %v, config sinks %v", argv, flagSinks)
		}
	}
}

// The reviewed kit's d.Files(args) call is resolved to the function the
// module stores in Declarations.Files, so a Files that reads its --config
// through FlagValues does not carry os.Args into the kit's read. A Files that
// scans argv itself still does, and so does a Files the analyzer cannot name
// (a variable), and any Files when the kit is not proven reviewed.
func TestKitFilesResolvedOnlyToStoredFunctions(t *testing.T) {
	for _, c := range []struct {
		mod      string
		reviewed bool
		argv     bool
	}{
		{"kitargv", true, false},
		{"kitargv", false, true},
		{"kitfilesown", true, true},
		{"kitfilesvar", true, true},
	} {
		src, err := os.ReadFile("testdata/" + c.mod + "/internal/credkit/credkit.go")
		if err != nil {
			t.Fatal(err)
		}
		at := ""
		for i, l := range strings.Split(string(src), "\n") {
			if strings.Contains(l, "// KITREAD") {
				at = "internal/credkit/credkit.go:" + strconv.Itoa(i+1)
			}
		}
		rep, err := analyzeModule("testdata/"+c.mod, 40, "", c.reviewed)
		if err != nil {
			t.Fatal(err)
		}
		if len(rep.Errors) > 0 || !rep.Converged {
			t.Fatalf("%s did not analyse cleanly: converged=%v errors=%v", c.mod, rep.Converged, rep.Errors)
		}
		got := false
		for _, f := range rep.Other {
			for _, s := range f.Sinks {
				got = got || f.Kind == "os-args" && s.Pos == at
			}
		}
		if got != c.argv {
			t.Errorf("%s reviewed=%v: os.Args reaches the kit's read = %v, want %v", c.mod, c.reviewed, got, c.argv)
		}
	}
}

// A program that writes into os.Args (argv[0] = "--config=" + notes) makes
// FlagValues return what it wrote: the kit's scan is then not tied to the
// flag, and the read stays reported (as os.Args, or as the written flag).
func TestKitFlagValuesNotTiedOverWrittenArgv(t *testing.T) {
	src, err := os.ReadFile("testdata/kitargvmut/cmd/app/main.go")
	if err != nil {
		t.Fatal(err)
	}
	at := ""
	for i, l := range strings.Split(string(src), "\n") {
		if strings.Contains(l, "// MUTATED") {
			at = "cmd/app/main.go:" + strconv.Itoa(i+1)
		}
	}
	rep, err := analyzeModule("testdata/kitargvmut", 40, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Errors) > 0 || !rep.Converged {
		t.Fatalf("kitargvmut did not analyse cleanly: converged=%v errors=%v", rep.Converged, rep.Errors)
	}
	reported, tied := false, false
	for _, f := range append(rep.Flags, rep.Other...) {
		for _, s := range f.Sinks {
			if s.Pos == at {
				reported = reported || f.Kind == "os-args" || f.Name == "notes"
				tied = tied || f.Name == "config"
			}
		}
	}
	if !reported || tied {
		t.Errorf("a FlagValues scan of a written argv: reported=%v (want true), tied to --config=%v (want false) at %s", reported, tied, at)
	}
}

// Any write into argv anywhere in the module - a constant included, which
// carries no taint - turns the FlagValues tie off: the kit's scan of argv
// then returns what the program put there, so os.Args stays reported.
func TestKitFlagValuesTieOffForEveryArgvWrite(t *testing.T) {
	kit, err := os.ReadFile("testdata/kitargv/internal/credkit/credkit.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, imp, decl, before string
		tied                    bool
	}{
		{"no write", "", "", "", true},
		{"read-only uses", "", "", "_ = len(os.Args); _ = os.Args[0]; for range os.Args {}", true},
		{"constant store", "", "", `os.Args[1] = "--config"`, false},
		{"store through a slice", "", "", `a := os.Args[1:]; a[0] = "--config"`, false},
		{"append in place", "", "", `_ = append(os.Args[:1], "--config")`, false},
		{"copy into argv", "", "", `copy(os.Args[1:], []string{"--config"})`, false},
		{"global reassigned", "", "", `os.Args = []string{"app", "--config", "x"}`, false},
		{"external call", "sort", "", `sort.Strings(os.Args)`, false},
		{"helper writes its parameter", "", "func w(a []string) { a[0] = \"--config\" }", `w(os.Args[1:])`, false},
		{"escapes into an interface", "", "var sink any", `sink = os.Args`, false},
		{"argv returned through a function value", "", "func get() []string { return os.Args }\n\nvar f = get",
			`f()[1] = "--config"`, false},
		{"argv returned by a method", "", "type g struct{}\n\nfunc (g) get() []string { return os.Args }\n\ntype getter interface{ get() []string }\n\nvar i getter = g{}",
			`_ = g{}.get(); i.get()[1] = "--config"`, false},
		{"a parser that keeps argv (stdlib flag)", "flag", "",
			`fs := flag.NewFlagSet("app", flag.ContinueOnError); _ = fs.Parse(os.Args[1:]); fs.Args()[0] = "--config"`, false},
	} {
		dir := t.TempDir()
		files := map[string]string{
			"go.mod":                      "module kitw\n\ngo 1.25\n",
			"internal/credkit/credkit.go": strings.Replace(string(kit), "kitargv/", "kitw/", -1),
			"cmd/app/main.go": "package main\n\nimport (\n\t\"os\"\n\n\t\"kitw/internal/credkit\"\n)\n\n" +
				c.decl + "\n\nfunc main() {\n\t" + c.before + "\n\tfor _, p := range credkit.FlagValues(os.Args[1:], \"--config\") {\n\t\t_, _ = os.ReadFile(p) // READ\n\t}\n}\n",
		}
		if c.imp != "" {
			files["cmd/app/main.go"] = strings.Replace(files["cmd/app/main.go"], "import (\n", "import (\n\t\""+c.imp+"\"\n", 1)
		}
		for name, body := range files {
			p := filepath.Join(dir, name)
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		rep, err := analyzeModule(dir, 40, "", true)
		if err != nil {
			t.Fatal(err)
		}
		if len(rep.Errors) > 0 || !rep.Converged {
			t.Fatalf("%s did not analyse cleanly: converged=%v errors=%v", c.name, rep.Converged, rep.Errors)
		}
		argv := false
		for _, f := range rep.Other {
			for _, s := range f.Sinks {
				argv = argv || f.Kind == "os-args" && strings.HasPrefix(s.Pos, "cmd/app/main.go")
			}
		}
		if argv == c.tied {
			t.Errorf("%s: os.Args reported at the read = %v, want %v", c.name, argv, !c.tied)
		}
	}
}
