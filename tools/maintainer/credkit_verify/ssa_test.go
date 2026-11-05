// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package main

// The SSA rules (loop ruling 13): one strict case per modeled instruction
// kind, so removing a rule fails a test, plus the coverage test over the
// pinned x/tools' instruction and value kinds.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

func init() { cases = append(cases, ssaCases...) }

func strict(c tcase) tcase { c.strict = true; return c }

var ssaCases = []tcase{
	// Alloc: a client or command made by value.
	strict(tcase{name: "ssa fail: a zero client by value in a connector struct",
		files: file("\t\"net/http\"", "type holder struct{ c [1]http.Client }\n\nfunc g() { var h holder; _, _ = h.c[0].Get(\"https://x\") }\n"),
		want:  []string{"rule 2|case.go|net/http.Client|held by value", "rule 2|case.go|sink:net/http.Client|made here by value"}}),
	// MakeInterface: a sink value only becomes an interface as credkit.Transport's argument.
	strict(tcase{name: "ssa pass: a command compared with an interface",
		files: file("\t\"os/exec\"", "func g(c *exec.Cmd, x any) bool { return x == c }\n")}),
	strict(tcase{name: "ssa fail: a client's carrier as an interface in a struct field",
		files: file("\t\"net/http\"", "type api struct{ c *http.Client }\n\ntype box struct{ V any }\n\nfunc g(a *api) box { return box{V: a} }\n"),
		want:  []string{"rule 2|case.go|carrier|is stored into memory the checker does not follow"}}),
	// An unexported field of a connector struct: every load of it is followed.
	strict(tcase{name: "ssa fail: a carrier in an unexported field, read and handed to another package",
		files: file("\t\"context\"\n\t\"net/http\"", "type api struct{ c *http.Client }\n\ntype box struct{ v any }\n\nfunc g(a *api) box { return box{v: a} }\n\nfunc h(b box) context.Context { return context.WithValue(context.Background(), 0, b.v) }\n"),
		want:  []string{"rule 2|case.go|carrier|handed to context.WithValue"}}),
	strict(tcase{name: "ssa fail: a carrier in an unexported field whose address is taken",
		files: file("\t\"net/http\"", "type api struct{ c *http.Client }\n\ntype box struct{ v any }\n\nfunc g(a *api) box { return box{v: a} }\n\nfunc h(b *box) *any { return &b.v }\n"),
		want:  []string{"rule 2|case.go|carrier|a field whose address"}}),
	strict(tcase{name: "ssa pass: a carrier in an unexported field, read and called through",
		files: file("\t\"net/http\"", "type api struct{ c *http.Client }\n\nfunc (a *api) Get(p string) string { return p }\n\ntype getter interface{ Get(string) string }\n\ntype box struct{ v getter }\n\nfunc g(a *api) box { return box{v: a} }\n\nfunc h(b box) string { return b.v.Get(\"x\") }\n")}),
	// A generic instantiated with a carrier inside another generic body: one
	// finding, at the call that instantiates it (ruling 12a), none inside it.
	strict(tcase{name: "ssa fail: a carrier generic called from a generic body",
		files: file("\t\"net/http\"", "type api struct{ c *http.Client }\n\nfunc run[S, U any](xs []S, f func(S) string, _ U) {\n\tgo func() {\n\t\tfor _, x := range xs {\n\t\t\t_ = func() string { return f(x) }()\n\t\t}\n\t}()\n}\n\nfunc outer[T any](xs []api, t T) { run(xs, func(a api) string { return \"\" }, t) }\n\nfunc g(xs []api) { outer(xs, 1) }\n"),
		want:  []string{"rule 2|case.go:17:|generic:carrier", "rule 2|case.go:17:|generic:net/http.Client"}}),
	// A copy of a client loaded from memory keeps the transport it was built
	// with; a store into the copy's Transport is checked.
	strict(tcase{name: "ssa pass: a copy of a client with a field changed",
		files: file("\t\"net/http\"", "func g(base *http.Client) *http.Client {\n\tclone := *base\n\tclone.Timeout = 0\n\treturn &clone\n}\n")}),
	strict(tcase{name: "ssa fail: a copy of a client given another transport",
		files: file("\t\"net/http\"", "func g(base *http.Client) *http.Client {\n\tclone := *base\n\tif t, ok := base.Transport.(*http.Transport); ok {\n\t\tclone.Transport = t.Clone()\n\t}\n\treturn &clone\n}\n"),
		want:  []string{"rule 2|case.go|net/http.Client.Transport|set to something other than credkit.Transport"}}),
	// A hook list of a function type naming a connector type, that nothing
	// fills: only connector code could make such a function, and none does.
	strict(tcase{name: "ssa pass: an empty hook list of a connector function type",
		files: file("\t\"net/http\"", "type api struct{ c *http.Client }\n\nvar hooks []func(*api) error\n\nfunc g(a *api) error {\n\tfor _, h := range hooks {\n\t\tif err := h(a); err != nil {\n\t\t\treturn err\n\t\t}\n\t}\n\treturn nil\n}\n")}),
	strict(tcase{name: "ssa fail: an empty hook list of a function type another package can make",
		files: file("\t\"net/http\"", "var hooks []func(*http.Client) error\n\nfunc g(c *http.Client) error {\n\tfor _, h := range hooks {\n\t\tif err := h(c); err != nil {\n\t\t\treturn err\n\t\t}\n\t}\n\treturn nil\n}\n"),
		want:  []string{"rule 2|case.go|call:dynamic|cannot resolve"}}),
	// Called through an interface, the followed value runs its own type's
	// method: none when it lacks the method, another package's when promoted.
	strict(tcase{name: "ssa pass: a carrier asserted to an interface its type lacks",
		files: file("\t\"net/http\"", "type api struct{ c *http.Client }\n\nfunc vars(c any) map[string]string {\n\tif v, ok := c.(interface{ Vars() map[string]string }); ok {\n\t\treturn v.Vars()\n\t}\n\treturn nil\n}\n\nfunc g(a *api) map[string]string { return vars(a) }\n")}),
	strict(tcase{name: "ssa fail: a carrier called through an interface on a promoted method",
		files: file("\t\"net/http\"", "type api struct{ *http.Client }\n\ntype doer interface {\n\tDo(*http.Request) (*http.Response, error)\n}\n\nfunc send(d doer, q *http.Request) { _, _ = d.Do(q) }\n\nfunc g(a *api, q *http.Request) { send(a, q) }\n"),
		want:  []string{"rule 2|case.go|carrier|method Do called where the checker cannot see", "rule 2|case.go|doer.Do|through an interface"}}),
	// An interface method call gets no unreached pass: another package's
	// value can arrive as a connector interface.
	strict(tcase{name: "ssa fail: a connector interface from another package's value, called in init",
		files: with(dep("any.go", "var _ http.Header\n\nfunc Get() any { return nil }\n"),
			caseFile, file("\t\"net/http\"\n\n\t\"example.com/httpx\"", "type api struct{ c *http.Client }\n\ntype binder interface{ Bind(*api) error }\n\nfunc init() {\n\tif b, ok := httpx.Get().(binder); ok {\n\t\t_ = b.Bind(nil)\n\t}\n}\n")[caseFile]),
		want: []string{"rule 2|case.go|call:unresolved|before credkit.Start", "rule 2|case.go|binder.Bind|through an interface", "rule 2|case.go:14:|carrier|type assertion"}}),
	strict(tcase{name: "ssa fail: a carrier called through an interface and handed to its own method",
		files: file("\t\"context\"\n\t\"net/http\"", "type api struct{ c *http.Client }\n\nfunc (a *api) Hand(x hander) context.Context { return context.WithValue(context.Background(), 0, any(x)) }\n\ntype hander interface{ Hand(hander) context.Context }\n\nfunc send(h hander) context.Context { return h.Hand(h) }\n\nfunc g(a *api) context.Context { return send(a) }\n"),
		want:  []string{"rule 2|case.go|carrier|handed to context.WithValue"}}),
	// A followed interface value handed to a call VTA resolves to nothing.
	strict(tcase{name: "ssa fail: a carrier handed to a function value another package returns",
		files: with(dep("hook.go", "var _ http.Header\n\nfunc Hook() func(any) { return func(any) {} }\n"),
			caseFile, file("\t\"net/http\"\n\n\t\"example.com/httpx\"", "type api struct{ c *http.Client }\n\nfunc g(a *api) { httpx.Hook()(a) }\n")[caseFile]),
		want: []string{"rule 2|case.go|carrier|a call the checker cannot resolve", "rule 2|case.go|carrier:net/http.Client|example.com/httpx.Hook"}}),
	// A function field nothing ever sets is nil: a call through it reaches no function.
	strict(tcase{name: "ssa pass: a client handed to a function field nothing sets",
		files: file("\t\"net/http\"", "type hooks struct{ bind func(*http.Client) error }\n\nvar reg *hooks\n\nfunc g(c *http.Client) error {\n\tif reg == nil || reg.bind == nil {\n\t\treturn nil\n\t}\n\treturn reg.bind(c)\n}\n")}),
	strict(tcase{name: "ssa fail: a client handed to a function field another package's function is stored into",
		files: with(dep("bind.go", "func Bind(c *http.Client) error { return nil }\n"),
			caseFile, file("\t\"net/http\"\n\n\t\"example.com/httpx\"", "type hooks struct{ bind func(*http.Client) error }\n\nvar reg = &hooks{bind: httpx.Bind}\n\nfunc g(c *http.Client) error { return reg.bind(c) }\n")[caseFile]),
		want: []string{"rule 2|case.go|call:dynamic|cannot resolve", "rule 2|case.go|example.com/httpx.Bind|a function of another package"}}),
	// The fleet's generated shape: the API client handed on as a
	// connector-local interface and asserted to another one.
	strict(tcase{name: "ssa pass: the API client through connector interfaces",
		files: file("\t\"net/http\"\n\n"+credkitImport, "type api struct{ c *http.Client }\n\nfunc (a *api) Get(p string) string { return p }\n\nfunc (a *api) Vars() map[string]string { return nil }\n\nfunc vars(c any) map[string]string {\n\tsrc, ok := c.(interface{ Vars() map[string]string })\n\tif !ok || src == nil {\n\t\treturn nil\n\t}\n\treturn src.Vars()\n}\n\nfunc tail(c interface{ Get(string) string }) string { return c.Get(\"x\") }\n\nfunc g() {\n\ta := &api{c: &http.Client{Transport: credkit.Transport(nil)}}\n\t_ = vars(a)\n\t_ = tail(a)\n}\n")}),
	strict(tcase{name: "ssa pass: a connector transport wrapped by the kit",
		files: file("\t\"net/http\"\n\n"+credkitImport, "type rt struct{ c *http.Client }\n\nfunc (r *rt) RoundTrip(q *http.Request) (*http.Response, error) { return r.c.Do(q) }\n\nfunc g() *http.Client {\n\tinner := &http.Client{Transport: credkit.Transport(nil)}\n\treturn &http.Client{Transport: credkit.Transport(&rt{c: inner})}\n}\n")}),
	// Store: a client's Transport only from credkit.Transport.
	strict(tcase{name: "ssa fail: a transport stored through a pointer parameter",
		files: file("\t\"net/http\"\n\n"+credkitImport, "func set(p *http.RoundTripper, v http.RoundTripper) { *p = v }\n\nfunc g() *http.Client {\n\tc := &http.Client{Transport: credkit.Transport(nil)}\n\tset(&c.Transport, nil)\n\treturn c\n}\n"),
		want:  []string{"rule 2|case.go|net/http.Client.Transport|address of an http.Client's transport", "rule 2|case.go|sink:net/http.RoundTripper|a nil http.RoundTripper"}}),
	// Call: a value holding a sink handed to another package.
	strict(tcase{name: "ssa fail: a struct holding a client handed to another package",
		files: file("\t\"encoding/json\"\n\t\"net/http\"", "type api struct{ C *http.Client }\n\nfunc g(a *api) { _, _ = json.Marshal(a) }\n"),
		want:  []string{"rule 2|case.go|carrier|becomes the interface any", "rule 2|case.go|carrier:net/http.Client|encoding/json.Marshal"}}),
	// Call: through an interface method.
	strict(tcase{name: "ssa fail: a client handed to a method through an interface",
		files: file("\t\"net/http\"", "type user interface{ Use(*http.Client) }\n\nfunc g(u user, c *http.Client) { u.Use(c) }\n"),
		want:  []string{"rule 2|case.go|user.Use|through an interface", "rule 2|case.go|sink:net/http.Client|method called through an interface"}}),
	// Call: a dynamic call VTA resolves to connector functions only is followed.
	strict(tcase{name: "ssa pass: a client from a function variable VTA resolves",
		files: file("\t\"net/http\"\n\n"+credkitImport, "var mk = func() *http.Client { return &http.Client{Transport: credkit.Transport(nil)} }\n\nfunc g() { c := mk(); _, _ = c.Get(\"https://x\") }\n")}),
	// TypeAssert: a sink recovered from an interface.
	strict(tcase{name: "ssa fail: a client recovered by a type assertion",
		files: file("\t\"net/http\"", "func g(x any) { c := x.(*http.Client); _, _ = c.Get(\"https://x\") }\n"),
		want:  []string{"rule 2|case.go|sink:net/http.Client|recovers this"}}),
	// ChangeType / Convert: a carrier converted to another type.
	strict(tcase{name: "ssa fail: a slice of clients converted to a named slice",
		files: file("\t\"net/http\"", "type pool []*http.Client\n\nfunc g(cs []*http.Client) pool { return pool(cs) }\n"),
		want:  []string{"rule 2|case.go|sink:net/http.Client|a conversion makes", "rule 2|case.go|carrier-convert:net/http.Client", "rule 2|case.go|carrier|which holds a sink, converted to", "rule 2|case.go|sink:net/http.Client|converted to acme"}}),
	// Field: a sink in another package's field.
	strict(tcase{name: "ssa fail: a client read from another package's struct field",
		files: with(dep("hold.go", "type Hold struct{ C *http.Client }\n"),
			caseFile, file("\t\"example.com/httpx\"", "func g(h *httpx.Hold) { _, _ = h.C.Get(\"https://x\") }\n")[caseFile]),
		want: []string{"rule 2|case.go|example.com/httpx.C|field of another package's type (example.com/httpx.C)"}}),
	// UnOp: another package's variable.
	strict(tcase{name: "ssa pass: http.DefaultTransport cloned for the kit",
		files: file("\t\"net/http\"\n\n"+credkitImport, "func g() *http.Client {\n\tt := http.DefaultTransport.(*http.Transport).Clone()\n\treturn &http.Client{Transport: credkit.Transport(t)}\n}\n")}),
	// MakeSlice / MakeMap / Lookup: zero clients by value.
	strict(tcase{name: "ssa fail: a map of clients by value",
		files: file("\t\"net/http\"", "func g(m map[string]http.Client) bool {\n\t_, ok := m[\"x\"]\n\treturn ok\n}\n"),
		want:  []string{"rule 2|case.go|net/http.Client|held by value", "rule 2|case.go|sink:net/http.Client|map lookup"}}),
	strict(tcase{name: "ssa fail: a slice of clients made by value",
		files: file("\t\"net/http\"", "func g(n int) []http.Client {\n\treturn make([]http.Client, n)\n}\n"),
		want:  []string{"rule 2|case.go|net/http.Client|held by value", "rule 2|case.go|sink:net/http.Client|a container of"}}),
	// MakeClosure: a method value of a sink.
	strict(tcase{name: "ssa fail: a command's Output as a method value",
		files: file("\t\"os/exec\"", "func g() { f := exec.Command(\"x\").Output; _, _ = f() }\n"),
		want:  []string{"rule 2|case.go|os/exec.Cmd.Output|not a method the kit allows", "rule 2|case.go|sink:os/exec.Cmd|bound into"}}),
	// Call of a generic instantiated with a sink-holding type argument.
	strict(tcase{name: "ssa fail: a generic instantiated with a client",
		files: file("\t\"net/http\"", "func id[T any](v T) T { return v }\n\nfunc g(c *http.Client) *http.Client { return id(c) }\n"),
		want:  []string{"rule 2|case.go|generic:net/http.Client", "rule 2|case.go|generic:sink:net/http.Client", "rule 2|case.go|sink:net/http.Client|type parameter of id", "rule 2|case.go|sink:net/http.Client|becomes a T"}}),
	// The init region: a dynamic call whose callees VTA cannot resolve.
	strict(tcase{name: "ssa fail: an unresolved dynamic call in init",
		files: file("", "var hook func()\n\nfunc init() {\n\tif hook != nil {\n\t\thook()\n\t}\n}\n"),
		want:  []string{"rule 2|case.go|call:unresolved|before credkit.Start"}}),
	strict(tcase{name: "ssa pass: a dynamic call in init VTA resolves to quiet code",
		files: file("", "var hook = func() int { return 1 }\n\nvar n int\n\nfunc init() { n = hook() }\n")}),
	// The init region on the call graph: output a method reached through an interface makes.
	strict(tcase{name: "ssa fail: output before Start through an interface VTA resolves",
		files: file("\t\"fmt\"", "type sayer interface{ say() }\n\ntype loud struct{}\n\nfunc (loud) say() { fmt.Println(\"x\") }\n\nfunc pick() sayer { return loud{} }\n\nvar s = pick()\n\nfunc init() { s.say() }\n"),
		want:  []string{"rule 2|case.go|fmt.Println|before credkit.Start"}}),
	// Shapes the AST rules let through: the SSA kind alone reports them.
	strict(tcase{name: "ssa fail: another package's carrier recovered by a type assertion",
		files: with(dep("hold.go", "type Hold struct{ C *http.Client }\n"),
			caseFile, file("\t\"example.com/httpx\"", "func g(x any) *httpx.Hold { return x.(*httpx.Hold) }\n")[caseFile]),
		want: []string{"rule 2|case.go|carrier|recovers a value holding a sink"}}),
	strict(tcase{name: "ssa fail: a carrier read from another package's field",
		files: with(dep("hold.go", "type Inner struct{ C *http.Client }\n\ntype Hold struct{ In *Inner }\n"),
			caseFile, file("\t\"example.com/httpx\"", "func g(h *httpx.Hold) *httpx.Inner { return h.In }\n")[caseFile]),
		want: []string{"rule 2|case.go|example.com/httpx.In|a value holding a sink in a field"}}),
	strict(tcase{name: "ssa fail: a named carrier assigned to an unnamed struct",
		files: file("\t\"net/http\"", "type a struct{ C *http.Client }\n\nfunc g(x a) struct{ C *http.Client } { return x }\n"),
		want:  []string{"rule 2|case.go|carrier|which holds a sink, converted to"}}),

	// Sources (ruling 13f): the decoded type is the destination seen through
	// every conversion, beside the converted type the AST reads.
	{name: "ssa fail: a source decoded through a conversion",
		edits: [][3]string{{"internal/config/config.go", "func Path() string", "type extra struct {\n\tSecret string `json:\"secret\"`\n}\n\ntype extraWire struct {\n\tSecret string `json:\"profile\"`\n}\n\nfunc Path() string"},
			{"internal/config/config.go", "\t\terr = json.Unmarshal(b, &c)\n", "\t\terr = json.Unmarshal(b, &c)\n\t\tvar x extra\n\t\t_ = json.Unmarshal(b, (*extraWire)(&x))\n"}},
		want: []string{"rule 3|internal/config/config.go|config:secret|not classified"}},
	{name: "ssa pass: a source decoded through a conversion, classified",
		edits: [][3]string{{"internal/config/config.go", "func Path() string", "type extra struct {\n\tSecret string `json:\"token\"`\n}\n\ntype extraWire struct {\n\tSecret string `json:\"profile\"`\n}\n\nfunc Path() string"},
			{"internal/config/config.go", "\t\terr = json.Unmarshal(b, &c)\n", "\t\terr = json.Unmarshal(b, &c)\n\t\tvar x extra\n\t\t_ = json.Unmarshal(b, (*extraWire)(&x))\n"}}},
	// Ruling 13g: cgo, go:linkname and assembly fail, in any file of a connector package.
	strict(tcase{name: "ssa fail: cgo in a file the release build leaves out",
		files: map[string]string{"internal/cli/c.go": "//go:build cgo\n\npackage cli\n\n// int one() { return 1; }\nimport \"C\"\n\nfunc one() int { return int(C.one()) }\n"},
		want:  []string{"rule 2|internal/cli/c.go|cgo|imports \"C\""}}),
	strict(tcase{name: "ssa fail: a go:linkname directive",
		files: map[string]string{"internal/cli/ln.go": "package cli\n\nimport _ \"unsafe\"\n\n//go:linkname nanotime runtime.nanotime\nfunc nanotime() int64\n"},
		want:  []string{"rule 2|internal/cli/ln.go|go:linkname"}}),
	strict(tcase{name: "ssa fail: an assembly file",
		files: map[string]string{"internal/cli/x_amd64.s": "// nothing\n"},
		want:  []string{"rule 2|internal/cli/x_amd64.s|assembly"}}),
}

// TestSSAKindsCovered: every concrete type of the pinned
// golang.org/x/tools/go/ssa that implements ssa.Instruction or ssa.Value has
// a case in instr or operand (ssa.go), and each switch fails by default.
func TestSSAKindsCovered(t *testing.T) {
	pkgs, err := packages.Load(&packages.Config{Mode: packages.NeedTypes | packages.NeedName}, "golang.org/x/tools/go/ssa")
	if err != nil || len(pkgs) != 1 || len(pkgs[0].Errors) > 0 {
		t.Fatalf("load go/ssa: %v %v", err, pkgs)
	}
	sc := pkgs[0].Types.Scope()
	iface := func(n string) *types.Interface { return sc.Lookup(n).Type().Underlying().(*types.Interface) }
	instr, value := iface("Instruction"), iface("Value")
	var want []string
	for _, n := range sc.Names() {
		tn, ok := sc.Lookup(n).(*types.TypeName)
		if !ok || types.IsInterface(tn.Type()) {
			continue
		}
		pt := types.NewPointer(tn.Type())
		if types.Implements(pt, instr) || types.Implements(pt, value) {
			want = append(want, n)
		}
	}
	sort.Strings(want)
	if len(want) < 40 {
		t.Fatalf("only %d kinds found in go/ssa: %v", len(want), want)
	}
	f, err := parser.ParseFile(token.NewFileSet(), "ssa.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	defaults := 0
	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || (fd.Name.Name != "instr" && fd.Name.Name != "operand") {
			continue
		}
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSwitchStmt)
			if !ok {
				return true
			}
			for _, st := range ts.Body.List {
				cc := st.(*ast.CaseClause)
				if cc.List == nil {
					defaults++
					src := ""
					ast.Inspect(cc, func(m ast.Node) bool {
						if c, ok := m.(*ast.CallExpr); ok {
							if s, ok := c.Fun.(*ast.SelectorExpr); ok {
								src += s.Sel.Name + " "
							}
						}
						return true
					})
					if !strings.Contains(src, "fail ") {
						t.Errorf("the default case of %s does not fail", fd.Name.Name)
					}
				}
				for _, e := range cc.List {
					if st, ok := e.(*ast.StarExpr); ok {
						if s, ok := st.X.(*ast.SelectorExpr); ok {
							have[s.Sel.Name] = true
						}
					}
				}
			}
			return false
		})
	}
	if defaults != 2 {
		t.Errorf("want a failing default in both instr and operand, found %d defaults", defaults)
	}
	for _, k := range want {
		if !have[k] {
			t.Errorf("ssa.%s has no case in instr or operand: a kind the pinned x/tools adds must be modeled or fail", k)
		}
	}
}

// TestSSAAlone: each SSA rule on its own, before the merge with the AST
// rules' findings (which hides an SSA finding the AST reports at the same
// place), so removing any SSA rule fails here even where the AST also sees it.
func TestSSAAlone(t *testing.T) {
	base, err := filepath.Abs("testdata/base")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range ssaAlone {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			copyTree(t, base, dir)
			cli := filepath.Join(dir, "cli")
			for p, body := range c.files {
				must(t, os.MkdirAll(filepath.Dir(filepath.Join(cli, p)), 0o755))
				must(t, os.WriteFile(filepath.Join(cli, p), []byte(body), 0o644))
			}
			l, err := loadLedger(filepath.Join(dir, ledgerName))
			must(t, err)
			p, err := analyzePass(cli, runtime.GOOS, runtime.GOARCH, l)
			must(t, err)
			var lines []string
			for _, f := range p.ssaOut {
				lines = append(lines, f.String())
			}
			for _, w := range c.want {
				if !anyWanted(lines, w) {
					t.Errorf("want an SSA finding with %q; got:\n%s", w, strings.Join(lines, "\n"))
				}
			}
			for _, l := range lines {
				if !wanted(l, c.want) {
					t.Errorf("unexpected SSA finding:\n%s", l)
				}
			}
		})
	}
}

func anyWanted(lines []string, w string) bool {
	for _, l := range lines {
		if wanted(l, []string{w}) {
			return true
		}
	}
	return false
}

var ssaAlone = []tcase{
	one("alone: a copy of a client given another transport", "func g(base *http.Client) *http.Client {\n\tclone := *base\n\tif t, ok := base.Transport.(*http.Transport); ok {\n\t\tclone.Transport = t.Clone()\n\t}\n\treturn &clone\n}\n",
		"case.go|net/http.Client.Transport|something other than credkit.Transport"),
	one("alone: a client literal without the kit's transport", "var c = &http.Client{}\n", "case.go|net/http.Client{}|made here by value"),
	one("alone: a client's transport set from elsewhere", "func g(c *http.Client, t http.RoundTripper) { c.Transport = t }\n", "case.go|net/http.Client.Transport|something other than credkit.Transport"),
	one("alone: the address of a client's transport", "func g(c *http.Client) *http.RoundTripper { return &c.Transport }\n", "case.go|net/http.Client.Transport|address of an http.Client's transport"),
	one("alone: a client handed to another package", "func g(c *http.Client) context.Context { return context.WithValue(context.Background(), 0, c) }\n",
		"case.go|sink:net/http.Client|becomes the interface any|handed to context.WithValue"),
	{name: "alone: a client handed to another package's function by its own type",
		files: with(dep("takec.go", "func TakeClient(c *http.Client) {}\n"),
			caseFile, file("\t\"net/http\"\n\n\t\"example.com/httpx\"", "func g(c *http.Client) { httpx.TakeClient(c) }\n")[caseFile]),
		want: []string{"case.go|sink:net/http.Client|handed to example.com/httpx.TakeClient|cannot see into"}},
	one("alone: a client another package builds", "var c = oauth2.NewClient(context.Background(), nil)\n", "case.go|golang.org/x/oauth2.NewClient|another package builds"),
	one("alone: a struct holding a client handed to another package", "type api struct{ c *http.Client }\n\nfunc use(a *api) { fmt.Sprint(a) }\n", "case.go|carrier|becomes the interface any|handed to fmt.Sprint"),
	one("alone: a command method the kit does not allow", "func g() { _ = exec.Command(\"x\").Run() }\n", "case.go|os/exec.Cmd.Run|not a method the kit allows"),
	one("alone: http.DefaultClient", "func g() { _, _ = http.DefaultClient.Get(\"https://x\") }\n", "case.go|net/http.DefaultClient|another package holds"),
	one("alone: a sink handed to a function the checker cannot resolve", "func g(f func(*http.Client), c *http.Client) { f(c) }\n", "case.go|call:dynamic|cannot resolve"),
	one("alone: output in init", "func init() { fmt.Println(\"x\") }\n", "case.go|fmt.Println|before credkit.Start"),
	one("alone: a writer handed on in init", "func init() { fmt.Fprint(os.Stderr, \"x\") }\n", "case.go|fmt.Fprint|before credkit.Start"),
	one("alone: a callback another package calls in init", "func init() { _ = strings.Map(func(r rune) rune { fmt.Println(\"x\"); return r }, \"x\") }\n", "case.go|fmt.Println|before credkit.Start"),
	one("alone: a closure only stored in init is not called there", "var hooks []func()\n\nfunc init() { hooks = append(hooks, func() { fmt.Println(\"x\") }) }\n"),
}
