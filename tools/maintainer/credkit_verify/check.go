// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package main

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/tools/go/packages"
)

// Finding is one site the checker could not see routed or classified.
type Finding struct {
	Rule      int
	File      string // relative to the connector's cli directory
	Line, Col int
	Func      string // the enclosing top-level function, as the ledger names it
	What      string // the primitive or construct, as the ledger names it
	Msg       string // what it is
	Fix       string // what would satisfy the checker
	Site      bool   // a code site the ledger may list
	Read      bool   // a file read (rule 4): the ledger may call it a source
	Env       bool   // an environment read with names the checker cannot resolve
	SSA       bool   // reported by the SSA rules (ssa.go)
}

func (f Finding) String() string {
	loc := f.File
	if f.Line > 0 {
		loc = fmt.Sprintf("%s:%d:%d", f.File, f.Line, f.Col)
	}
	s := fmt.Sprintf("rule %d  %s  %s: %s. Fix: %s", f.Rule, loc, f.What, f.Msg, f.Fix)
	if f.Site {
		s += fmt.Sprintf(` (or list it in %s: {"file": %q, "func": %q, "what": %q, ...})`, ledgerName, f.File, f.Func, f.What)
	}
	return s
}

func (f Finding) less(g Finding) bool {
	if f.File != g.File {
		return f.File < g.File
	}
	if f.Line != g.Line {
		return f.Line < g.Line
	}
	if f.Col != g.Col {
		return f.Col < g.Col
	}
	if f.Rule != g.Rule {
		return f.Rule < g.Rule
	}
	return f.What+f.Msg < g.What+g.Msg
}

// verify loads the connector once per target ("os/arch"), checks every rule,
// unions the findings and applies the site ledger.
func verify(cli string, targets []string) ([]Finding, error) {
	l, err := loadLedger(filepath.Join(cli, "..", ledgerName))
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var all []Finding
	for _, t := range targets {
		goos, goarch, ok := strings.Cut(strings.TrimSpace(t), "/")
		if !ok || goos == "" || goarch == "" {
			return nil, fmt.Errorf("target %q is not os/arch", t)
		}
		fs, err := analyze(cli, goos, goarch, l)
		if err != nil {
			return nil, fmt.Errorf("%s: %v", t, err)
		}
		for _, f := range fs {
			k := fmt.Sprintf("%d|%s|%d|%d|%s|%s", f.Rule, f.File, f.Line, f.Col, f.What, f.Msg)
			if !seen[k] {
				seen[k] = true
				all = append(all, f)
			}
		}
	}
	return apply(all, l), nil
}

const mcpServerPkg = "github.com/mark3labs/mcp-go/server"

func set(xs ...string) map[string]bool {
	m := map[string]bool{}
	for _, x := range xs {
		m[x] = true
	}
	return m
}

var (
	// File writes: every way the standard library opens a file for writing.
	fileWrite = set("os.WriteFile", "os.Create", "os.CreateTemp", "os.NewFile", "os.CopyFS",
		"io/ioutil.WriteFile", "io/ioutil.TempFile", "os.Root.Create", "os.Root.WriteFile")
	// fileMove puts another file's content at its destination (class file-replace).
	fileMove = set("os.Rename", "os.Link", "os.Symlink", "os.Root.Rename", "os.Root.Link", "os.Root.Symlink")
	// fileOpen writes or reads by its flag argument (index 1).
	fileOpen = set("os.OpenFile", "os.Root.OpenFile")
	// File reads (rule 4).
	fileRead = set("os.ReadFile", "os.Open", "io/ioutil.ReadFile", "os.OpenInRoot", "os.DirFS",
		"os.Root.Open", "os.Root.ReadFile", "os.Root.FS")
	envGet   = set("os.Getenv", "os.LookupEnv")
	envOther = set("os.Environ", "os.ExpandEnv", "os.Expand")
	// HTTP that never passes a connector-built client.
	httpDefault = set("net/http.DefaultClient", "net/http.Get", "net/http.Head", "net/http.Post", "net/http.PostForm")
	httpNewReq  = set("net/http.NewRequest", "net/http.NewRequestWithContext")
	// Starting a child process.
	execRun = set("os/exec.Cmd.Start", "os/exec.Cmd.Run", "os/exec.Cmd.Output", "os/exec.Cmd.CombinedOutput", "os.StartProcess")
	// Serving MCP outside the kit's wrappers.
	mcpServe = set(mcpServerPkg+".StreamableHTTPServer.Start", mcpServerPkg+".StreamableHTTPServer.ServeHTTP",
		mcpServerPkg+".SSEServer.Start", mcpServerPkg+".SSEServer.ServeHTTP")
	// Output calls: unchecked when they run before credkit.Start.
	writeCalls = set("fmt.Print", "fmt.Printf", "fmt.Println", "fmt.Fprint", "fmt.Fprintf", "fmt.Fprintln",
		"log.Print", "log.Printf", "log.Println", "log.Fatal", "log.Fatalf", "log.Fatalln", "log.Panic", "log.Panicf", "log.Panicln", "log.Output",
		"log.Logger.Print", "log.Logger.Printf", "log.Logger.Println", "log.Logger.Fatal", "log.Logger.Fatalf", "log.Logger.Fatalln",
		"log.Logger.Panic", "log.Logger.Panicf", "log.Logger.Panicln", "log.Logger.Output",
		"builtin.print", "builtin.println", "os.File.Write", "os.File.WriteString", "os.File.WriteAt", "os.File.ReadFrom",
		"io.WriteString", "io.Copy", "io.CopyN", "io.CopyBuffer")
	slogOut = set("Debug", "DebugContext", "Info", "InfoContext", "Warn", "WarnContext", "Error", "ErrorContext", "Log", "LogAttrs")
	// Any method by these names may write (an io.Writer, a cobra command): before
	// Start the checker counts each one as output, whatever its receiver.
	writeMethods = set("Write", "WriteString", "WriteByte", "WriteRune", "WriteTo", "ReadFrom",
		"Print", "Printf", "Println", "PrintErr", "PrintErrf", "PrintErrln")
	// Raw network connections: bytes the outbound guard never sees.
	rawDial = set("net.Dial", "net.DialTimeout", "net.DialTCP", "net.DialUDP", "net.DialIP", "net.DialUnix",
		"net.Dialer.Dial", "net.Dialer.DialContext", "crypto/tls.Dial", "crypto/tls.DialWithDialer",
		"crypto/tls.Dialer.Dial", "crypto/tls.Dialer.DialContext")
	// Method names a type assertion to an interface may reach a sink through.
	sinkNames = set("Start", "Run", "Output", "CombinedOutput", "RoundTrip", "Listen", "ServeHTTP")
)

// objKey names an object the way findings and the ledger do: "os.WriteFile",
// "net/http.DefaultTransport", "os/exec.Cmd.Run" (a method, pointer or not),
// "builtin.print".
func objKey(obj types.Object) string {
	switch o := obj.(type) {
	case *types.Builtin:
		return "builtin." + o.Name()
	case *types.Func:
		o = o.Origin()
		if r := o.Signature().Recv(); r != nil {
			t := types.Unalias(r.Type())
			if p, ok := t.(*types.Pointer); ok {
				t = types.Unalias(p.Elem())
			}
			if n, ok := t.(*types.Named); ok && n.Obj().Pkg() != nil {
				return n.Obj().Pkg().Path() + "." + n.Obj().Name() + "." + o.Name()
			}
			return "interface." + o.Name()
		}
	}
	if obj.Pkg() == nil {
		return obj.Name()
	}
	return obj.Pkg().Path() + "." + obj.Name()
}

type ref struct {
	call   *ast.CallExpr // nil when the function is used as a value
	f      Finding       // the site
	encl   *types.Func   // the enclosing top-level function
	info   *types.Info
	method bool       // a method expression (T.m): the arguments are shifted
	stack  []ast.Node // a read's ancestors, for following what it returns
}

type pending struct {
	ctx ast.Node
	f   Finding
}

type envSite struct {
	name string
	f    Finding
}

type pass struct {
	cli, mod, kit string
	fset          *token.FileSet
	pkgs          []*packages.Package
	local         map[string]bool
	ledger        *ledger
	out           []Finding

	clientStruct types.Type                  // net/http.Client's struct, when the program imports net/http
	writer       *types.Interface            // io.Writer
	driverIfaces map[string]*types.Interface // database/sql/driver's Driver, DriverContext, Connector

	funcDecl map[*types.Func]ast.Node
	refs     map[*types.Func][]ref
	edges    map[ast.Node][]*types.Func
	roots    []ast.Node
	writes   []pending
	wrappers [][2]any // {*types.Func, int}
	envNames []envSite
	starts   []ref
	declLits []declLit
	declRefs []ref          // every use of a variable of the kit's Declarations type
	sqlRegs  []sqlUse       // registrations through credkit.SQLDriver that run in an init function
	sqlOpens []sqlUse       // name "" when the driver name is not a constant
	cur      *types.Package // the package being walked
	reported map[ast.Expr]bool
	reads    []ref                // every file read, for the source proof
	exposed  map[*types.Func]bool // connector methods callable through an interface

	slogHandler *types.Interface               // log/slog.Handler
	infos       map[*types.Package]*types.Info // each connector package's type information
	implicit    map[types.Object]bool          // type switch case variables
	decoded     []decoded                      // the types source reads decode into
	setenvs     []Finding                      // environment writes
	osArgs      []ref                          // uses of os.Args that may change it
	chdirs      []Finding                      // working-directory changes
	stdState    []Finding                      // uses of path packages' variables other than as their functions' arguments
	std         map[string]bool                // standard-library package paths
	formats     map[types.Type]map[string]bool // decoded types: the formats they are decoded as
	sinkNamed   map[string]types.Type          // the sink types, by sinkKinds key (nil: not loaded)
	sites       map[string]bool                // reported site positions, so two rules report one site once
	ssaOut      []Finding                      // the SSA rules' findings, merged by mergeSSA
	ssa         *ssaPass
}

type sqlUse struct {
	name string
	pkg  *types.Package
	f    Finding
}

type declLit struct {
	lit  *ast.CompositeLit
	info *types.Info
	v    types.Object // the package-level variable it initializes, if any
	f    Finding
}

func analyze(cli, goos, goarch string, l *ledger) ([]Finding, error) {
	p, err := analyzePass(cli, goos, goarch, l)
	if err != nil {
		return nil, err
	}
	return p.out, nil
}

// analyzePass runs every rule and returns the pass, the SSA rules' own
// findings (ssaOut) kept apart for the tests.
func analyzePass(cli, goos, goarch string, l *ledger) (*pass, error) {
	abs, err := filepath.Abs(cli)
	if err != nil {
		return nil, err
	}
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes |
			packages.NeedTypesInfo | packages.NeedImports | packages.NeedModule,
		Dir: abs,
		// ponytail: cgo off, as the release builds are; a cgo-only file is not checked.
		Env: append(os.Environ(), "GOOS="+goos, "GOARCH="+goarch, "CGO_ENABLED=0", "GOFLAGS=-mod=readonly"),
	}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return nil, err
	}
	var errs []string
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		for _, e := range p.Errors {
			errs = append(errs, e.Error())
		}
	})
	if len(errs) > 0 {
		return nil, fmt.Errorf("the connector does not load: %s", strings.Join(errs, "; "))
	}
	if len(pkgs) == 0 || pkgs[0].Module == nil {
		return nil, fmt.Errorf("no module packages under %s", abs)
	}
	p := &pass{cli: abs, mod: pkgs[0].Module.Path, fset: pkgs[0].Fset, ledger: l, local: map[string]bool{},
		funcDecl: map[*types.Func]ast.Node{}, refs: map[*types.Func][]ref{}, edges: map[ast.Node][]*types.Func{},
		reported: map[ast.Expr]bool{}, exposed: map[*types.Func]bool{}, infos: map[*types.Package]*types.Info{},
		implicit: map[types.Object]bool{}, sites: map[string]bool{}, formats: map[types.Type]map[string]bool{}}
	p.kit = p.mod + "/internal/credkit"
	p.std = map[string]bool{}
	packages.Visit(pkgs, nil, func(pk *packages.Package) {
		if pk.Module == nil {
			p.std[pk.PkgPath] = true // the standard library: no module
		}
	})
	for _, pk := range pkgs {
		if pk.PkgPath != p.kit {
			p.pkgs = append(p.pkgs, pk)
			p.local[pk.PkgPath] = true
			p.infos[pk.Types] = pk.TypesInfo
		}
	}
	if t := lookupType(pkgs, "net/http", "Client"); t != nil {
		p.clientStruct = t.Underlying()
	}
	p.sinkNamed = map[string]types.Type{}
	for _, k := range sinkKinds {
		path, name, _ := cutLast(k, ".")
		p.sinkNamed[k] = lookupType(pkgs, path, name)
	}
	if t := lookupType(pkgs, "io", "Writer"); t != nil {
		p.writer, _ = t.Underlying().(*types.Interface)
	}
	if t := lookupType(pkgs, "log/slog", "Handler"); t != nil {
		p.slogHandler, _ = t.Underlying().(*types.Interface)
	}
	p.driverIfaces = map[string]*types.Interface{}
	for _, n := range []string{"Driver", "DriverContext", "Connector"} {
		if t := lookupType(pkgs, "database/sql/driver", n); t != nil {
			p.driverIfaces[n], _ = t.Underlying().(*types.Interface)
		}
	}
	for _, pk := range p.pkgs {
		for _, f := range pk.Syntax {
			p.index(pk, f)
		}
	}
	for _, pk := range p.pkgs {
		p.cur = pk.Types
		for _, f := range pk.Syntax {
			ast.PreorderStack(f, nil, func(n ast.Node, stack []ast.Node) bool {
				p.visit(pk.TypesInfo, n, stack)
				return true
			})
		}
	}
	p.lowLevel(pkgs)
	p.ssaCheck(pkgs)
	p.prologue()
	d := p.declarations()
	p.envs(d)
	p.sources(d)
	p.configTypes(d)
	p.origins(d)
	p.sqlOpen()
	p.mergeSSA()
	return p, nil
}

// lookupType finds a package-level type among the loaded packages' imports.
func lookupType(pkgs []*packages.Package, path, name string) types.Type {
	var t types.Type
	seen := map[*types.Package]bool{}
	var walk func(*types.Package)
	walk = func(tp *types.Package) {
		if tp == nil || seen[tp] || t != nil {
			return
		}
		seen[tp] = true
		if tp.Path() == path {
			if o, ok := tp.Scope().Lookup(name).(*types.TypeName); ok {
				t = o.Type()
			}
			return
		}
		for _, i := range tp.Imports() {
			walk(i)
		}
	}
	for _, pk := range pkgs {
		walk(pk.Types)
	}
	return t
}

// index maps each top-level function to its declaration and collects the
// roots that run before main: init functions and package-level initializers.
func (p *pass) index(pk *packages.Package, f *ast.File) {
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			if fn, ok := pk.TypesInfo.Defs[d.Name].(*types.Func); ok {
				p.funcDecl[fn] = d
			}
			if d.Name.Name == "init" && d.Recv == nil {
				p.roots = append(p.roots, d)
			}
		case *ast.GenDecl:
			if d.Tok == token.VAR {
				for _, s := range d.Specs {
					if vs := s.(*ast.ValueSpec); len(vs.Values) > 0 {
						p.roots = append(p.roots, vs)
					}
				}
			}
		}
	}
}

// at is a finding positioned at n, with the enclosing function from stack.
func (p *pass) at(n ast.Node, stack []ast.Node, rule int, what, msg, fix string) Finding {
	pos := p.fset.Position(n.Pos())
	rel, err := filepath.Rel(p.cli, pos.Filename)
	if err != nil {
		rel = pos.Filename
	}
	return Finding{Rule: rule, File: filepath.ToSlash(rel), Line: pos.Line, Col: pos.Column, Func: enclosing(stack), What: what, Msg: msg, Fix: fix}
}

func (p *pass) site(n ast.Node, stack []ast.Node, rule int, what, msg, fix string) {
	f := p.at(n, stack, rule, what, msg, fix)
	k := fmt.Sprintf("%s:%d:%d:%s", f.File, f.Line, f.Col, f.What)
	if p.sites[k] {
		return // another rule reported this site already
	}
	p.sites[k] = true
	f.Site = true
	p.out = append(p.out, f)
}

// enclosing names the top-level declaration a node sits in: "Func",
// "Type.Method", or the declaration's keyword ("var", "type", "const").
func enclosing(stack []ast.Node) string {
	if len(stack) < 2 {
		return "file"
	}
	switch d := stack[1].(type) {
	case *ast.FuncDecl:
		if d.Recv != nil && len(d.Recv.List) > 0 {
			return recvName(d.Recv.List[0].Type) + "." + d.Name.Name
		}
		return d.Name.Name
	case *ast.GenDecl:
		return d.Tok.String()
	}
	return "file"
}

func recvName(e ast.Expr) string {
	for {
		switch t := e.(type) {
		case *ast.StarExpr:
			e = t.X
		case *ast.ParenExpr:
			e = t.X
		case *ast.IndexExpr:
			e = t.X
		case *ast.IndexListExpr:
			e = t.X
		case *ast.Ident:
			return t.Name
		default:
			return "?"
		}
	}
}

// context is the top-level node a node runs in for the before-main closure:
// its function declaration, or the package-level value spec it initializes.
func context(stack []ast.Node) ast.Node {
	if len(stack) < 2 {
		return nil
	}
	if g, ok := stack[1].(*ast.GenDecl); ok {
		if len(stack) > 2 {
			return stack[2]
		}
		return g
	}
	return stack[1]
}

func encl(info *types.Info, stack []ast.Node) *types.Func {
	if len(stack) > 1 {
		if d, ok := stack[1].(*ast.FuncDecl); ok {
			fn, _ := info.Defs[d.Name].(*types.Func)
			return fn
		}
	}
	return nil
}

// exprOf is the expression an identifier stands for: the selector when it is
// a selector's name.
func exprOf(id *ast.Ident, stack []ast.Node) (ast.Expr, []ast.Node) {
	if len(stack) > 0 {
		if s, ok := stack[len(stack)-1].(*ast.SelectorExpr); ok && s.Sel == id {
			return s, stack[:len(stack)-1]
		}
	}
	return id, stack
}

// callOf is the call whose function is e (through parentheses), or nil.
func callOf(e ast.Expr, stack []ast.Node) *ast.CallExpr {
	var child ast.Node = e
	for i := len(stack) - 1; i >= 0; i-- {
		switch par := stack[i].(type) {
		case *ast.ParenExpr:
			child = par
			continue
		case *ast.CallExpr:
			if par.Fun == child {
				return par
			}
		}
		return nil
	}
	return nil
}

func unparen(e ast.Expr) ast.Expr { return ast.Unparen(e) }

func calleeObj(info *types.Info, fun ast.Expr) types.Object {
	switch f := unparen(fun).(type) {
	case *ast.Ident:
		return info.Uses[f]
	case *ast.SelectorExpr:
		return info.Uses[f.Sel]
	case *ast.IndexExpr: // an instantiated generic function
		return calleeObj(info, f.X)
	case *ast.IndexListExpr:
		return calleeObj(info, f.X)
	}
	return nil
}

// isKit reports whether obj is one of the kit's package-level functions.
func (p *pass) isKit(obj types.Object, names ...string) bool {
	fn, ok := obj.(*types.Func)
	if !ok || fn.Pkg() == nil || fn.Pkg().Path() != p.kit || fn.Signature().Recv() != nil {
		return false
	}
	for _, n := range names {
		if fn.Name() == n {
			return true
		}
	}
	return false
}

func (p *pass) isKitCall(info *types.Info, e ast.Expr, names ...string) bool {
	c, ok := unparen(e).(*ast.CallExpr)
	return ok && p.isKit(calleeObj(info, c.Fun), names...)
}

// underKit reports whether e is an argument of a call to one of the named kit
// functions, directly or inside parentheses, an address-of, or the elements
// of composite literals that argument builds.
func (p *pass) underKit(info *types.Info, stack []ast.Node, e ast.Node, names ...string) bool {
	child := e
	for i := len(stack) - 1; i >= 0; i-- {
		switch par := stack[i].(type) {
		case *ast.ParenExpr, *ast.CompositeLit:
		case *ast.UnaryExpr:
			if par.Op != token.AND {
				return false
			}
		case *ast.KeyValueExpr:
			if par.Value != child {
				return false
			}
		case *ast.CallExpr:
			if !p.isKit(calleeObj(info, par.Fun), names...) {
				return false
			}
			for _, a := range par.Args {
				if a == child {
					return true
				}
			}
			return false
		default:
			return false
		}
		child = stack[i]
	}
	return false
}

func constString(info *types.Info, e ast.Expr) (string, bool) {
	if tv, ok := info.Types[e]; ok && tv.Value != nil && tv.Value.Kind() == constant.String {
		return constant.StringVal(tv.Value), true
	}
	return "", false
}

func (p *pass) visit(info *types.Info, n ast.Node, stack []ast.Node) {
	if e, ok := n.(ast.Expr); ok {
		p.sinkExpr(info, e, stack)
	}
	switch n := n.(type) {
	case *ast.Ident:
		p.ident(info, n, stack)
	case *ast.CompositeLit:
		p.compositeLit(info, n, stack)
		p.conversions(info, n, stack)
	case *ast.CallExpr:
		p.conversions(info, n, stack)
		p.writerArgs(info, n, stack)
		p.reflectArgs(info, n, stack)
		p.foreignCarrier(info, n, stack)
	case *ast.AssignStmt, *ast.ReturnStmt, *ast.SendStmt, *ast.RangeStmt, *ast.IndexExpr:
		p.conversions(info, n, stack)
	case *ast.ValueSpec:
		p.conversions(info, n, stack)
	case *ast.TypeAssertExpr:
		if n.Type != nil {
			p.assertion(info, n.Type, stack)
			p.assertWriter(info, n.X, n.Type, stack)
		}
	case *ast.CaseClause:
		if o := info.Implicits[n]; o != nil {
			p.implicit[o] = true
		}
		if len(stack) > 1 {
			if ts, ok := stack[len(stack)-2].(*ast.TypeSwitchStmt); ok {
				var x ast.Expr
				switch a := ts.Assign.(type) {
				case *ast.ExprStmt:
					x = unparen(a.X).(*ast.TypeAssertExpr).X
				case *ast.AssignStmt:
					x = unparen(a.Rhs[0]).(*ast.TypeAssertExpr).X
				}
				for _, e := range n.List {
					p.assertion(info, e, stack)
					if x != nil {
						p.assertWriter(info, x, e, stack)
					}
				}
			}
		}
	case *ast.StructType:
		p.envTags(info, n, stack)
	}
}

func (p *pass) isClientOrPtr(t types.Type) bool {
	if pt, ok := types.Unalias(t).(*types.Pointer); ok {
		t = pt.Elem()
	}
	return p.isClient(t)
}

// driverMethod reports whether obj is Open, OpenConnector or Connect on a
// SQL driver, a driver context or a connector (a type or the interface), so a
// connection made without database/sql.
func (p *pass) driverMethod(obj types.Object) bool {
	fn, ok := obj.(*types.Func)
	if !ok || fn.Signature().Recv() == nil || fn.Pkg() == nil || fn.Pkg().Path() == p.kit {
		return false
	}
	iface := map[string]string{"Open": "Driver", "OpenConnector": "DriverContext", "Connect": "Connector"}[fn.Name()]
	if iface == "" {
		return false
	}
	it := p.driverIfaces[iface]
	if it == nil {
		return false
	}
	r := fn.Signature().Recv().Type()
	return types.Implements(r, it) || types.Implements(types.NewPointer(r), it)
}

// holdsClient reports whether a named type is net/http.Client, or a type of
// another package that holds one by value (in a field or an array).
func (p *pass) holdsClient(tn *types.TypeName) bool {
	if p.isClient(tn.Type()) {
		return true
	}
	if tn.Pkg() == nil || p.local[tn.Pkg().Path()] || tn.Pkg().Path() == p.kit {
		return false // a connector type: its own declaration names the client
	}
	var holds func(t types.Type, seen map[types.Type]bool) bool
	holds = func(t types.Type, seen map[types.Type]bool) bool {
		if seen[t] {
			return false
		}
		seen[t] = true
		if p.isClient(t) {
			return true
		}
		switch u := types.Unalias(t).Underlying().(type) {
		case *types.Struct:
			for i := 0; i < u.NumFields(); i++ {
				if holds(u.Field(i).Type(), seen) {
					return true
				}
			}
		case *types.Array:
			return holds(u.Elem(), seen)
		}
		return false
	}
	return holds(tn.Type(), map[types.Type]bool{})
}

// clientTypeOK: a reference to a client type is safe only as a pointer
// (*http.Client), or, for http.Client itself (exact), as the type of a
// composite literal, which compositeLit checks.
func (p *pass) clientTypeOK(e ast.Expr, estack []ast.Node, exact bool) bool {
	var child ast.Node = e
	for i := len(estack) - 1; i >= 0; i-- {
		switch par := estack[i].(type) {
		case *ast.ParenExpr:
			child = par
			continue
		case *ast.StarExpr:
			return par.X == child
		case *ast.CompositeLit:
			return par.Type == child && exact
		}
		return false
	}
	return false
}

func (p *pass) isClient(t types.Type) bool {
	return t != nil && p.clientStruct != nil && types.Identical(types.Unalias(t).Underlying(), p.clientStruct)
}

func (p *pass) assertion(info *types.Info, te ast.Expr, stack []ast.Node) {
	t := info.TypeOf(te)
	if t == nil {
		return
	}
	// A carrier (a struct, function or container) holding such an interface
	// recovers it just the same (class sending-interfaces).
	if k := p.ifaceSink(t, map[types.Type]bool{}, true); k != "" {
		p.site(te, stack, 2, "assert:"+k, "a type assertion to a type that is or carries an interface a "+k+" satisfies with a method that sends or runs: the value recovered may be one the checker never saw built (from reflect, or another package)", "assert to the concrete type, or route the value through the kit")
	}
	if !types.IsInterface(t) {
		return
	}
	it := t.Underlying().(*types.Interface)
	for i := 0; i < it.NumMethods(); i++ {
		if m := it.Method(i).Name(); sinkNames[m] {
			p.site(te, stack, 2, "assert:"+m, "a type assertion to an interface with method "+m+", through which a child process, a request or an MCP server could start out of the checker's sight", "assert to the concrete type, or route the value through the kit")
			return
		}
	}
}

// ident classifies one use of a name.
func (p *pass) ident(info *types.Info, id *ast.Ident, stack []ast.Node) {
	obj := info.Uses[id]
	if obj == nil {
		return
	}
	key := objKey(obj)
	p.instance(info, id, stack)
	e, estack := exprOf(id, stack)
	if tn, ok := obj.(*types.TypeName); ok && p.holdsClient(tn) && !p.clientTypeOK(e, estack, p.isClient(tn.Type())) {
		p.site(e, stack, 2, "net/http.Client", "an http.Client held by value ("+types.TypeString(tn.Type(), nil)+"): its zero value, or one made by var, new, make or an array, sends with http.DefaultTransport", "hold a *http.Client built as &http.Client{Transport: credkit.Transport(...)}")
	}
	if tn, ok := obj.(*types.TypeName); ok && objKey(tn) == "os/exec.Cmd" && !p.clientTypeOK(e, estack, false) {
		p.site(e, stack, 2, "os/exec.Cmd", "an exec.Cmd held by value: its zero value, or one made by var, new, make or an array, is a command exec.Command did not build", "hold a *exec.Cmd from exec.Command or exec.CommandContext")
	}
	call := callOf(e, estack)
	ctx := context(stack)

	if fn, ok := obj.(*types.Func); ok && fn.Pkg() != nil && p.local[fn.Pkg().Path()] {
		r := ref{call: call, f: p.at(id, stack, 3, key, "", ""), encl: encl(info, stack), info: info}
		if s, ok := e.(*ast.SelectorExpr); ok {
			if sel := info.Selections[s]; sel != nil && sel.Kind() == types.MethodExpr {
				r.method = true
			}
		}
		p.refs[fn.Origin()] = append(p.refs[fn.Origin()], r)
		if ctx != nil {
			p.edges[ctx] = append(p.edges[ctx], fn.Origin())
		}
	}
	if p.isKit(obj, "Start") {
		p.starts = append(p.starts, ref{call: call, f: p.at(id, stack, 1, "credkit.Start", "", ""), info: info})
	}
	if v, ok := obj.(*types.Var); ok && !v.IsField() && p.isDecl(v.Type()) {
		p.declRefs = append(p.declRefs, ref{call: call, f: p.at(id, stack, 1, "credkit.Declarations", "", ""), info: info})
	}

	if pk := lowLevelPkg(obj, id, info); pk != "" {
		p.site(e, stack, 2, pk, "a use of package "+pk+", which reaches inside values (a command, client or transport among them) where the checker cannot see", "do without "+pk+", or list the site with its reason")
	}
	if reflectWrites[key] || strings.HasPrefix(key, "reflect.Value.Set") {
		p.site(e, stack, 2, key, "a reflect write or call, which can change or use a command, client or transport inside any value without the checker seeing it", "do not write or call through reflect")
	}
	if v, ok := obj.(*types.Var); ok && !v.IsField() && v.Pkg() != nil && v.Parent() == v.Pkg().Scope() && pathPkgs[v.Pkg().Path()] &&
		!types.IsInterface(v.Type()) && !p.pathArg(info, e, estack) {
		p.stdState = append(p.stdState, p.at(e, stack, 4, key, "", ""))
	}
	switch key {
	case "os.Chdir", "syscall.Chdir", "syscall.Fchdir", "golang.org/x/sys/unix.Chdir", "golang.org/x/sys/unix.Fchdir", "os.File.Chdir",
		"syscall.SetCurrentDirectory", "golang.org/x/sys/windows.SetCurrentDirectory":
		p.chdirs = append(p.chdirs, p.at(e, stack, 4, key, "", ""))
	case "os.Setenv", "os.Unsetenv", "os.Clearenv", "syscall.Setenv", "syscall.Unsetenv", "syscall.Clearenv",
		"golang.org/x/sys/unix.Setenv", "golang.org/x/sys/unix.Unsetenv", "golang.org/x/sys/unix.Clearenv",
		"golang.org/x/sys/windows.Setenv", "golang.org/x/sys/windows.Unsetenv", "golang.org/x/sys/windows.Clearenv":
		p.setenvs = append(p.setenvs, p.at(e, stack, 4, key, "", ""))
	case "os.Args":
		if ok, via := p.osArgsReadOnly(info, e, estack); !ok {
			p.osArgs = append(p.osArgs, ref{f: p.at(e, stack, 4, key, "", ""), encl: via})
		}
	}
	if ctx != nil && (key == "database/sql.Open" || key == "database/sql.OpenDB" || preStartSink[key]) {
		f := p.at(e, stack, 2, key, "a sink used before credkit.Start (this runs in an init function or a package-level initializer, or in a function one of those calls)", "use it after Start, out of init and package-level initializers")
		f.Site = true
		p.writes = append(p.writes, pending{ctx, f})
	}
	switch {
	case writeCalls[key] || key == "builtin.panic" || (obj.Pkg() != nil && obj.Pkg().Path() == "log/slog" && slogOut[obj.Name()]) ||
		(isFunc(obj) && !isPkgFunc(obj) && writeMethods[obj.Name()] && !inMemory(obj)) || p.writingReceiver(info, obj, e):
		// Called or used as a value: a function value taken before main can run there too.
		if ctx != nil {
			f := p.at(e, stack, 2, key, "output written before credkit.Start installs the output layer (this runs in an init function or a package-level initializer, or in a function one of those calls)", "move the output after Start, out of init and package-level initializers")
			f.Site = true
			p.writes = append(p.writes, pending{ctx, f})
		}
	case key == "os.Stdin":
		if !p.stdinReadOnly(info, e, estack) {
			p.site(e, stack, 2, key, "os.Stdin used where it can be written: it is the one open file the kit neither redirects nor opened, and a shell can open it read-write", "only read it: call a read method on it, or pass it where an io.Reader (an interface with no write method) or the kit takes it")
		}
	case fileWrite[key]:
		p.site(e, stack, 2, key, "a file write that does not pass the kit", "use credkit.WriteFile, credkit.Create, credkit.OpenFile or credkit.AppendFile")
	case fileMove[key]:
		p.site(e, stack, 2, key, "a file moved or linked into place, which writes its destination without the kit: a file Start held can be replaced", "write the destination with credkit.WriteFile, or list the site with why its destination is never a credential file")
	case fileOpen[key]:
		p.openFile(info, e, stack, call, key)
	case fileRead[key] || (isPkgFunc(obj) && obj.Name() == "DecodeFile" && obj.Pkg() != nil && !p.local[obj.Pkg().Path()] && obj.Pkg().Path() != p.kit):
		p.read(info, e, stack, call, key)
	case envGet[key]:
		p.envRead(info, e, stack, call, key)
	case envOther[key]:
		if key == "os.ExpandEnv" && call != nil && len(call.Args) == 1 {
			if s, ok := constString(info, call.Args[0]); ok {
				os.Expand(s, func(name string) string {
					p.envNames = append(p.envNames, envSite{name, p.at(call, stack, 3, key, "", "")})
					return ""
				})
				return
			}
		}
		p.envUnresolved(e, stack, key)
	case httpDefault[key]:
		p.site(e, stack, 2, key, "an HTTP request through http.DefaultClient, which does not pass the kit", "send it with an http.Client built with credkit.Transport")
	case httpNewReq[key]:
		p.site(e, stack, 2, key, "an outbound request is built here; the checker cannot see which client sends it", "send it only with a client built with credkit.Transport and list the site with that reason")
	case key == "net/http.DefaultTransport":
		if p.underKit(info, estack, e, "Transport") {
			return
		}
		if len(estack) > 0 {
			if ta, ok := estack[len(estack)-1].(*ast.TypeAssertExpr); ok && ta.X == e {
				return
			}
		}
		p.site(e, stack, 2, key, "http.DefaultTransport used outside credkit.Transport", "pass it straight to credkit.Transport(...)")
	case key == "net/http.Client.Transport" || (isField(obj) && obj.Pkg() != nil && obj.Pkg().Path() == "net/http" && obj.Name() == "Transport"):
		p.clientTransportField(info, id, e, estack, stack)
	case execRun[key]:
		p.site(e, stack, 2, key, "a child process started outside the kit", "start it with credkit.StartCommand or credkit.RunCommand")
	case mcpServe[key]:
		p.site(e, stack, 2, key, "an MCP server served outside the kit", "serve credkit.MCPHTTP(server) instead")
	case key == mcpServerPkg+".StdioServer.Listen":
		if call == nil || len(call.Args) != 3 || !p.isKitCall(info, call.Args[1], "MCPIn") || !p.isKitCall(info, call.Args[2], "MCPOut") {
			p.site(e, stack, 2, key, "the MCP stdio server listens on streams the kit does not check", "call Listen(ctx, credkit.MCPIn(os.Stdin), credkit.MCPOut())")
		}
	case key == mcpServerPkg+".NewMCPServer":
		if call == nil || !p.hasToolMiddleware(info, call) {
			p.site(e, stack, 2, key, "an MCP server without the kit's tool middleware", "pass server.WithToolHandlerMiddleware(credkit.MCPTools) to NewMCPServer")
		}
	case obj.Pkg() != nil && obj.Pkg().Path() == mcpServerPkg && strings.HasPrefix(obj.Name(), "Serve") && isPkgFunc(obj):
		p.site(e, stack, 2, key, "an MCP server served outside the kit", "use NewStdioServer(s).Listen(ctx, credkit.MCPIn(os.Stdin), credkit.MCPOut()) or credkit.MCPHTTP")
	case key == "database/sql.Register":
		if call == nil || len(call.Args) != 2 || !p.isKitCall(info, call.Args[1], "SQLDriver") {
			p.site(e, stack, 2, key, "a SQL driver registered without the kit's wrapper", "register credkit.SQLDriver(driver)")
		} else if name, ok := constString(info, call.Args[0]); ok {
			if p.leadingRegister(info, call, stack) {
				p.sqlRegs = append(p.sqlRegs, sqlUse{name: name, pkg: p.cur})
			} else {
				p.site(e, stack, 2, key, "a SQL driver registration that is not among the leading statements of an init function (only other kit registrations may come before it), so it may not have run before a database is opened", "make it the first statement of an init function")
			}
		} else {
			p.site(e, stack, 2, key, "a SQL driver registered under a name that is not a constant", "register it under a constant name")
		}
	case key == "database/sql.Open":
		name := ""
		if call != nil && len(call.Args) == 2 {
			name, _ = constString(info, call.Args[0])
		}
		f := p.at(e, stack, 2, key, "a database opened with a driver the kit does not wrap", "open it with a constant driver name registered as sql.Register(name, credkit.SQLDriver(driver))")
		f.Site = true
		p.sqlOpens = append(p.sqlOpens, sqlUse{name, p.cur, f})
	case key == "database/sql.OpenDB":
		p.site(e, stack, 2, key, "a database opened from a connector the kit does not wrap", "use sql.Open with a driver registered through credkit.SQLDriver")
	case rawDial[key]:
		p.site(e, stack, 2, key, "a raw network connection, whose bytes the kit's outbound guard never sees", "send requests through an http.Client built with credkit.Transport")
	case p.driverMethod(obj):
		p.site(e, stack, 2, key, "a SQL driver opened directly, around database/sql and the kit's wrapper", "open the database with sql.Open and a driver registered through credkit.SQLDriver")
	case isFunc(obj) && obj.Name() == "RoundTrip" && !isPkgFunc(obj) && obj.Pkg() != nil && obj.Pkg().Path() != p.kit:
		if s, ok := e.(*ast.SelectorExpr); ok && p.isKitCall(info, s.X, "Transport") {
			return
		}
		p.site(e, stack, 2, key, "a request sent by a RoundTrip the kit does not wrap", "send through an http.Client whose Transport is credkit.Transport(...)")
	case isFunc(obj) && obj.Pkg() != nil && (obj.Pkg().Path() == "syscall" || strings.HasPrefix(obj.Pkg().Path(), "golang.org/x/sys/")):
		p.site(e, stack, 2, key, "a system call the checker cannot classify (it may write, read the environment or start a process)", "use the os package through the kit, or list the site with its reason")
	case isFunc(obj) && obj.Pkg() != nil && strings.HasPrefix(obj.Pkg().Path(), "golang.org/x/oauth2"):
		p.site(e, stack, 2, key, "golang.org/x/oauth2 sends its own requests with a client the kit does not wrap unless its context carries one", "give it the kit's client through oauth2.HTTPClient in the context and list the site with that reason")
	case isFunc(obj) && obj.Pkg() != nil && !p.local[obj.Pkg().Path()] && obj.Pkg().Path() != p.kit && obj.Pkg().Path() != "net/http" && p.returnsClient(obj.(*types.Func)):
		p.site(e, stack, 2, key, "an HTTP client or transport built by another package, so its transport is not the kit's", "build the client in the connector with credkit.Transport")
	}
}

// lowLevelPkg: obj is package reflect's or unsafe's ("reflect", "unsafe"),
// unsafe's builtins (Sizeof, Add...) included, or "" (loop ruling 13g).
func lowLevelPkg(obj types.Object, id *ast.Ident, info *types.Info) string {
	if obj.Pkg() != nil {
		switch obj.Pkg().Path() {
		case "reflect", "unsafe":
			return obj.Pkg().Path()
		}
	}
	if b, ok := obj.(*types.Builtin); ok && types.Unsafe.Scope().Lookup(b.Name()) == obj {
		return "unsafe"
	}
	return ""
}

// inMemory reports whether a method is one of strings.Builder's or
// bytes.Buffer's, which write only into memory.
func inMemory(o types.Object) bool {
	k := objKey(o)
	return strings.HasPrefix(k, "strings.Builder.") || strings.HasPrefix(k, "bytes.Buffer.")
}

// preStartSink: sink uses that may not run before credkit.Start.
var preStartSink = set("net/http.Client.Do", "net/http.Client.Get", "net/http.Client.Head", "net/http.Client.Post", "net/http.Client.PostForm",
	"database/sql.DB.Exec", "database/sql.DB.ExecContext", "database/sql.DB.Query", "database/sql.DB.QueryContext",
	"database/sql.DB.QueryRow", "database/sql.DB.QueryRowContext", "database/sql.DB.Begin", "database/sql.DB.BeginTx", "database/sql.DB.Conn")

// writingReceiver: a method called on a value that writes (any io.Writer
// other than an exact in-memory buffer, or a slog handler) may write,
// whatever the method's name: before Start, that is output.
func (p *pass) writingReceiver(info *types.Info, obj types.Object, e ast.Expr) bool {
	s, ok := e.(*ast.SelectorExpr)
	if !ok || !isFunc(obj) || isPkgFunc(obj) {
		return false
	}
	sel := info.Selections[s]
	if sel == nil || sel.Kind() == types.FieldVal {
		return false // a method value, or a method expression whose receiver is its first argument
	}
	t := info.TypeOf(s.X)
	if t == nil || inMemoryType(t) {
		return false
	}
	impl := func(it *types.Interface) bool {
		return it != nil && (types.Implements(t, it) || (!types.IsInterface(t) && types.Implements(types.NewPointer(t), it)))
	}
	return impl(p.writer) || impl(p.slogHandler)
}

// osArgsReadOnly: os.Args only read (len, an index) or handed to a path
// function as os.Args[1:].
// osArgsReadOnly: os.Args only read (len, an index), or (the function
// returned) handed as os.Args[1:] to a path-shaped function, which only a
// declared via may be: sources proves via pure (class argv-writes).
func (p *pass) osArgsReadOnly(info *types.Info, e ast.Expr, estack []ast.Node) (bool, *types.Func) {
	n, estack := outerParen(e, estack)
	if len(estack) == 0 {
		return false, nil
	}
	switch x := estack[len(estack)-1].(type) {
	case *ast.CallExpr:
		return builtinName(info, x.Fun) == "len", nil
	case *ast.IndexExpr:
		return x.X == n && indexRead(x, estack[:len(estack)-1]), nil
	case *ast.SliceExpr:
		if !isOSArgs1(info, x) {
			return false, nil
		}
		s, st := outerParen(x, estack[:len(estack)-1])
		if len(st) == 0 {
			return false, nil
		}
		c, ok := st[len(st)-1].(*ast.CallExpr)
		if !ok || len(c.Args) != 1 || c.Args[0] != s {
			return false, nil
		}
		fn, ok := calleeObj(info, c.Fun).(*types.Func)
		if !ok || !p.localFunc(fn) {
			return false, nil
		}
		if argsForm, ok := viaShape(fn); ok && argsForm {
			return false, fn.Origin()
		}
	}
	return false, nil
}

// writerArgs: a call that is handed a writer (a stream, a file, any
// io.Writer that is not memory) may write to it, and the checker cannot see
// whether it does, so before Start every such call is output. The two
// exceptions only capture the writer for output methods the checker already
// tracks: log.New (the Logger's Print methods) and slog's handlers (slog's
// output functions).
func (p *pass) writerArgs(info *types.Info, call *ast.CallExpr, stack []ast.Node) {
	ctx := context(stack)
	if ctx == nil || p.writer == nil {
		return
	}
	k := objKey(orNil(calleeObj(info, call.Fun)))
	if writeCalls[k] {
		return // the call itself is output already
	}
	switch k {
	case "log.New", "log.Logger.SetOutput", "log.SetOutput", "log/slog.NewTextHandler", "log/slog.NewJSONHandler":
		return
	}
	for _, a := range call.Args {
		for _, t := range components(info.TypeOf(a)) { // f(g()) hands on each of g's results (class tuple-writers)
			if t == nil || !types.Implements(t, p.writer) || inMemoryType(t) {
				continue
			}
			f := p.at(a, stack, 2, "writer:"+types.TypeString(t, nil), "a writer handed to a call before credkit.Start installs the output layer (this runs in an init function or a package-level initializer, or in a function one of those calls), which may write to it", "move the call after Start, out of init and package-level initializers")
			f.Site = true
			p.writes = append(p.writes, pending{ctx, f})
		}
	}
}

// writeNames are the methods that write into their receiver.
var writeNames = set("Write", "WriteString", "WriteAt", "ReadFrom", "SyscallConn")

// writable reports whether t has a method that writes into it.
func writable(t types.Type) bool {
	ms := types.NewMethodSet(t)
	for i := 0; i < ms.Len(); i++ {
		if writeNames[ms.At(i).Obj().Name()] {
			return true
		}
	}
	if !types.IsInterface(t) {
		ms = types.NewMethodSet(types.NewPointer(t))
		for i := 0; i < ms.Len(); i++ {
			if writeNames[ms.At(i).Obj().Name()] {
				return true
			}
		}
	}
	return false
}

// stdinReadOnly reports whether this use of os.Stdin can only read it: the
// receiver of a method that does not write, an argument of a kit call, an
// argument or assigned value whose type is an interface with no write method,
// or the variable assigned (os.Stdin = r).
func (p *pass) stdinReadOnly(info *types.Info, e ast.Expr, estack []ast.Node) bool {
	child := ast.Node(e)
	i := len(estack) - 1
	for ; i >= 0; i-- {
		if _, ok := estack[i].(*ast.ParenExpr); !ok {
			break
		}
		child = estack[i]
	}
	if i < 0 {
		return false
	}
	readOnly := func(t types.Type) bool { return t != nil && types.IsInterface(t) && !writable(t) }
	switch par := estack[i].(type) {
	case *ast.SelectorExpr:
		return par.X == child && !writeNames[par.Sel.Name]
	case *ast.CallExpr:
		if p.isKit(calleeObj(info, par.Fun), "MCPIn", "Terminal") {
			return true
		}
		sig, ok := types.Unalias(info.TypeOf(par.Fun)).(*types.Signature)
		if !ok {
			return false
		}
		for j, a := range par.Args {
			if a != child {
				continue
			}
			ps := sig.Params()
			switch {
			case sig.Variadic() && j >= ps.Len()-1 && !par.Ellipsis.IsValid():
				sl, _ := ps.At(ps.Len() - 1).Type().Underlying().(*types.Slice)
				return sl != nil && readOnly(sl.Elem())
			case j < ps.Len():
				return readOnly(ps.At(j).Type())
			}
		}
	case *ast.AssignStmt:
		for j, l := range par.Lhs {
			if l == child {
				return true
			}
			if j < len(par.Rhs) && par.Rhs[j] == child && len(par.Lhs) == len(par.Rhs) {
				return readOnly(info.TypeOf(l))
			}
		}
	case *ast.ValueSpec:
		return par.Type != nil && readOnly(info.TypeOf(par.Type))
	}
	return false
}

// assertWriter: asserting a value whose static interface has no write method
// to a type that has one recovers a writer the checker let pass as a reader
// (os.Stdin passed as an io.Reader, asserted back to *os.File).
func (p *pass) assertWriter(info *types.Info, x, te ast.Expr, stack []ast.Node) {
	xt, t := info.TypeOf(x), info.TypeOf(te)
	if xt == nil || t == nil || !types.IsInterface(xt) || writable(xt) || !writable(t) {
		return
	}
	p.site(te, stack, 2, "assert:writer", "a type assertion from an interface with no write method to "+types.TypeString(t, nil)+", which can write: a reader (os.Stdin) could become a writer here", "keep the value a reader, or assert from a writer type")
}

// inMemoryType reports whether t (or what it points to) is exactly
// strings.Builder or bytes.Buffer, the only writers that never reach a sink.
func inMemoryType(t types.Type) bool {
	if pt, ok := types.Unalias(t).(*types.Pointer); ok {
		t = pt.Elem()
	}
	n, ok := types.Unalias(t).(*types.Named)
	if !ok || n.Obj().Pkg() == nil {
		return false
	}
	k := n.Obj().Pkg().Path() + "." + n.Obj().Name()
	return k == "strings.Builder" || k == "bytes.Buffer"
}

func isFunc(o types.Object) bool { _, ok := o.(*types.Func); return ok }

func isPkgFunc(o types.Object) bool {
	f, ok := o.(*types.Func)
	return ok && f.Signature().Recv() == nil
}

func isField(o types.Object) bool { v, ok := o.(*types.Var); return ok && v.IsField() }

func (p *pass) isDecl(t types.Type) bool {
	n, ok := types.Unalias(t).(*types.Named)
	return ok && n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == p.kit && n.Obj().Name() == "Declarations"
}

// returnsClient reports whether fn returns an http.Client, a pointer to one or
// an http.RoundTripper.
func (p *pass) returnsClient(fn *types.Func) bool {
	res := fn.Signature().Results()
	for i := 0; i < res.Len(); i++ {
		t := types.Unalias(res.At(i).Type())
		if pt, ok := t.(*types.Pointer); ok {
			t = pt.Elem()
		}
		if p.isClient(t) {
			return true
		}
		if n, ok := types.Unalias(t).(*types.Named); ok && n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == "net/http" && n.Obj().Name() == "RoundTripper" {
			return true
		}
	}
	return false
}

func (p *pass) hasToolMiddleware(info *types.Info, call *ast.CallExpr) bool {
	for _, a := range call.Args {
		c, ok := unparen(a).(*ast.CallExpr)
		if !ok || objKey(orNil(calleeObj(info, c.Fun))) != mcpServerPkg+".WithToolHandlerMiddleware" || len(c.Args) != 1 {
			continue
		}
		if p.isKit(calleeObj(info, c.Args[0]), "MCPTools") {
			return true
		}
	}
	return false
}

func orNil(o types.Object) types.Object {
	if o == nil {
		return types.NewLabel(token.NoPos, nil, "")
	}
	return o
}

// openFile: os.OpenFile writes unless its flag is a constant that opens the
// file read-only.
func (p *pass) openFile(info *types.Info, e ast.Expr, stack []ast.Node, call *ast.CallExpr, key string) {
	if call != nil && len(call.Args) == 3 {
		if tv, ok := info.Types[call.Args[1]]; ok && tv.Value != nil {
			if fl, ok := constant.Int64Val(tv.Value); ok && fl&p.writeBits() == 0 {
				p.read(info, e, stack, call, key)
				return
			}
		}
	}
	p.site(e, stack, 2, key, "a file opened for writing (or with flags that are not a constant) that does not pass the kit", "use credkit.OpenFile")
}

// writeBits is O_WRONLY|O_RDWR|O_APPEND|O_CREATE|O_TRUNC|O_EXCL for the GOOS loaded.
func (p *pass) writeBits() int64 {
	var bits int64
	for _, pk := range p.pkgs {
		for _, imp := range pk.Types.Imports() {
			if imp.Path() != "os" {
				continue
			}
			for _, n := range []string{"O_WRONLY", "O_RDWR", "O_APPEND", "O_CREATE", "O_TRUNC", "O_EXCL"} {
				if c, ok := imp.Scope().Lookup(n).(*types.Const); ok {
					v, _ := constant.Int64Val(c.Val())
					bits |= v
				}
			}
			return bits
		}
	}
	return ^int64(0) // unknown: every flag writes
}

func (p *pass) read(info *types.Info, e ast.Expr, stack []ast.Node, call *ast.CallExpr, key string) {
	f := p.at(e, stack, 4, key, "a file read: the checker cannot tell whether it reads a config file, a credential store or a token cache", `list it with kind "source" (a credential file Declarations.Files names, so Start holds it first, with "via": the function both the read's path and Files come from) or "exempt" (not a credential file), with the reason`)
	f.Site, f.Read = true, true
	p.out = append(p.out, f)
	p.reads = append(p.reads, ref{call: call, f: f, encl: encl(info, stack), info: info, stack: append([]ast.Node(nil), stack...)})
}

// clientTransportField: a write to http.Client.Transport must be credkit.Transport(...).
func (p *pass) clientTransportField(info *types.Info, id *ast.Ident, e ast.Expr, estack, stack []ast.Node) {
	if len(stack) > 0 {
		if kv, ok := stack[len(stack)-1].(*ast.KeyValueExpr); ok && kv.Key == id {
			return // a composite literal key: compositeLit checks it
		}
	}
	var child ast.Node = e
	i := len(estack) - 1
	for ; i >= 0; i-- { // through parentheses: (c.Transport) is c.Transport
		if _, ok := estack[i].(*ast.ParenExpr); !ok {
			break
		}
		child = estack[i]
	}
	if i < 0 {
		return
	}
	switch par := estack[i].(type) {
	case *ast.AssignStmt:
		for i, l := range par.Lhs {
			if l == child {
				if len(par.Lhs) == len(par.Rhs) && p.isKitCall(info, par.Rhs[i], "Transport") {
					return
				}
				p.site(e, stack, 2, "net/http.Client.Transport", "an http.Client's transport set to something other than credkit.Transport(...)", "assign credkit.Transport(base)")
				return
			}
		}
	case *ast.RangeStmt:
		if par.Key == child || par.Value == child {
			p.site(e, stack, 2, "net/http.Client.Transport", "an http.Client's transport set by a range clause", "assign credkit.Transport(base)")
		}
	case *ast.UnaryExpr:
		if par.Op == token.AND {
			p.site(e, stack, 2, "net/http.Client.Transport", "the address of an http.Client's transport is taken, so it can be replaced out of the checker's sight", "assign credkit.Transport(base) directly")
		}
	}
}

// compositeLit checks http.Client literals and records Declarations literals.
func (p *pass) compositeLit(info *types.Info, lit *ast.CompositeLit, stack []ast.Node) {
	t := info.TypeOf(lit)
	if t == nil {
		return
	}
	if p.isClient(t) {
		var tr ast.Expr
		for i, el := range lit.Elts {
			if kv, ok := el.(*ast.KeyValueExpr); ok {
				if k, ok := kv.Key.(*ast.Ident); ok && k.Name == "Transport" {
					tr = kv.Value
				}
			} else if i == 0 {
				tr = el
			}
		}
		if tr == nil || !p.isKitCall(info, tr, "Transport") {
			if tr != nil {
				p.reported[tr] = true
			}
			p.site(lit, stack, 2, "net/http.Client{}", "an http.Client whose Transport is not credkit.Transport(...)", "set Transport: credkit.Transport(base)")
		}
	}
	if p.isDecl(t) {
		dl := declLit{lit: lit, info: info, f: p.at(lit, stack, 1, "credkit.Declarations", "", "")}
		if len(stack) > 0 {
			if vs, ok := stack[len(stack)-1].(*ast.ValueSpec); ok && len(stack) == 3 {
				for i, v := range vs.Values {
					if v == lit {
						dl.v = info.Defs[vs.Names[i]]
					}
				}
			}
		}
		p.declLits = append(p.declLits, dl)
	}
}

// conversions checks every place a value is implicitly converted to an
// interface: a value whose method set reaches a sink method (a child
// process's Run, a transport's RoundTrip, an MCP server's ServeHTTP) may only
// become an interface where the kit takes it.
func (p *pass) conversions(info *types.Info, n ast.Node, stack []ast.Node) {
	anc := append(stack[:len(stack):len(stack)], n)
	check := func(e ast.Expr, t types.Type) {
		if e != nil && t != nil {
			p.conv(info, anc, e, t)
		}
	}
	// One element of a multi-value expression (r, _ = f(); g(f()); return f()).
	tupleCheck := func(e ast.Expr, src, t types.Type) {
		if e != nil && t != nil && types.IsInterface(t) {
			p.convType(info, anc, e, src, t, false)
		}
	}
	switch n := n.(type) {
	case *ast.CallExpr:
		tv, ok := info.Types[n.Fun]
		if !ok {
			return
		}
		if tv.IsType() {
			if len(n.Args) == 1 {
				check(n.Args[0], tv.Type)
				p.carrierConvert(info, anc, n.Args[0], tv.Type)
			}
			return
		}
		sig, ok := types.Unalias(tv.Type).Underlying().(*types.Signature)
		if !ok {
			return
		}
		ps := sig.Params()
		param := func(i int) types.Type {
			switch {
			case sig.Variadic() && i >= ps.Len()-1 && !n.Ellipsis.IsValid():
				if sl, ok := ps.At(ps.Len() - 1).Type().Underlying().(*types.Slice); ok {
					return sl.Elem()
				}
			case i < ps.Len():
				return ps.At(i).Type()
			}
			return nil
		}
		if tu, ok := info.TypeOf(firstOr(n.Args)).(*types.Tuple); ok && len(n.Args) == 1 {
			for i := 0; i < tu.Len(); i++ {
				tupleCheck(n.Args[0], tu.At(i).Type(), param(i))
			}
			return
		}
		for i, a := range n.Args {
			check(a, param(i))
		}
	case *ast.AssignStmt:
		if n.Tok == token.ASSIGN && len(n.Lhs) == len(n.Rhs) {
			for i := range n.Lhs {
				check(n.Rhs[i], info.TypeOf(n.Lhs[i]))
			}
		} else if n.Tok == token.ASSIGN && len(n.Rhs) == 1 {
			if tu, ok := info.TypeOf(n.Rhs[0]).(*types.Tuple); ok && tu.Len() == len(n.Lhs) {
				for i, l := range n.Lhs {
					if !blank(l) {
						tupleCheck(n.Rhs[0], tu.At(i).Type(), info.TypeOf(l))
					}
				}
			}
		}
	case *ast.ValueSpec:
		if n.Type != nil && len(n.Values) == len(n.Names) {
			for _, v := range n.Values {
				check(v, info.TypeOf(n.Type))
			}
		} else if n.Type != nil && len(n.Values) == 1 {
			if tu, ok := info.TypeOf(n.Values[0]).(*types.Tuple); ok {
				for i := 0; i < tu.Len(); i++ {
					tupleCheck(n.Values[0], tu.At(i).Type(), info.TypeOf(n.Type))
				}
			}
		}
	case *ast.ReturnStmt:
		sig := enclosingSig(info, stack)
		if sig != nil && sig.Results().Len() == len(n.Results) {
			for i, r := range n.Results {
				check(r, sig.Results().At(i).Type())
			}
		} else if sig != nil && len(n.Results) == 1 {
			if tu, ok := info.TypeOf(n.Results[0]).(*types.Tuple); ok && tu.Len() == sig.Results().Len() {
				for i := 0; i < tu.Len(); i++ {
					tupleCheck(n.Results[0], tu.At(i).Type(), sig.Results().At(i).Type())
				}
			}
		}
	case *ast.SendStmt:
		if ch, ok := types.Unalias(info.TypeOf(n.Chan)).Underlying().(*types.Chan); ok {
			check(n.Value, ch.Elem())
		}
	case *ast.RangeStmt: // for k, v = range x: each iteration value stored in k and v
		if n.Tok != token.ASSIGN {
			return
		}
		for i, l := range []ast.Expr{n.Key, n.Value} {
			if src := rangeElem(info.TypeOf(n.X), i); l != nil && !blank(l) && src != nil {
				tupleCheck(n.X, src, info.TypeOf(l))
			}
		}
	case *ast.IndexExpr: // m[k]: the key stored (or looked up) as the map's key type
		if m, ok := types.Unalias(info.TypeOf(n.X)).Underlying().(*types.Map); ok {
			check(n.Index, m.Key())
		}
	case *ast.CompositeLit:
		t := info.TypeOf(n)
		if t == nil {
			return
		}
		switch u := types.Unalias(t).Underlying().(type) {
		case *types.Struct:
			for i, el := range n.Elts {
				if kv, ok := el.(*ast.KeyValueExpr); ok {
					if k, ok := kv.Key.(*ast.Ident); ok {
						if fv, ok := info.Uses[k].(*types.Var); ok {
							check(kv.Value, fv.Type())
						}
					}
				} else if i < u.NumFields() {
					check(el, u.Field(i).Type())
				}
			}
		case *types.Slice, *types.Array, *types.Map:
			var kt, et types.Type
			switch u := u.(type) {
			case *types.Slice:
				et = u.Elem()
			case *types.Array:
				et = u.Elem()
			case *types.Map:
				kt, et = u.Key(), u.Elem()
			}
			for _, el := range n.Elts {
				if kv, ok := el.(*ast.KeyValueExpr); ok {
					if kt != nil {
						check(kv.Key, kt)
					}
					check(kv.Value, et)
				} else {
					check(el, et)
				}
			}
		}
	}
}

// rangeElem is the type of a range clause's i-th iteration value (0 the key,
// 1 the value) over x of type t, or nil.
func rangeElem(t types.Type, i int) types.Type {
	if t == nil {
		return nil
	}
	if pt, ok := t.Underlying().(*types.Pointer); ok {
		t = pt.Elem() // a pointer to an array
	}
	switch u := t.Underlying().(type) {
	case *types.Slice:
		return []types.Type{types.Typ[types.Int], u.Elem()}[i]
	case *types.Array:
		return []types.Type{types.Typ[types.Int], u.Elem()}[i]
	case *types.Map:
		return []types.Type{u.Key(), u.Elem()}[i]
	case *types.Chan:
		if i == 0 {
			return u.Elem()
		}
	case *types.Basic:
		if u.Info()&types.IsString != 0 {
			return []types.Type{types.Typ[types.Int], types.Typ[types.Rune]}[i]
		}
		if i == 0 && u.Info()&types.IsInteger != 0 {
			return t
		}
	case *types.Signature: // an iterator: func(yield func(K, V) bool)
		if u.Params().Len() == 1 {
			if y, ok := u.Params().At(0).Type().Underlying().(*types.Signature); ok && i < y.Params().Len() {
				return y.Params().At(i).Type()
			}
		}
	}
	return nil
}

// carrierConvert: a conversion T(x) where x holds a sink anywhere in its
// structure and T is another type. The result is a new type whose fields
// (named structs' included) the checker trusts by T's package, so the
// conversion would launder another package's client into a connector field
// (class carrier-conversions). An interface target is conv's.
func (p *pass) carrierConvert(info *types.Info, anc []ast.Node, x ast.Expr, target types.Type) {
	src := info.TypeOf(x)
	if src == nil || target == nil || types.IsInterface(target) || types.Identical(src, target) {
		return
	}
	if k := p.deepHolds(src, map[types.Type]bool{}); k != "" {
		p.site(x, anc, 2, "carrier-convert:"+k, "a "+types.TypeString(src, nil)+", which holds a "+k+", converted to "+types.TypeString(target, nil)+": the checker cannot follow where the sink inside came from", "build the value as its own type, with the kit's transport")
	}
}

func firstOr(xs []ast.Expr) ast.Expr {
	if len(xs) == 0 {
		return nil
	}
	return xs[0]
}

func enclosingSig(info *types.Info, stack []ast.Node) *types.Signature {
	for i := len(stack) - 1; i >= 0; i-- {
		switch f := stack[i].(type) {
		case *ast.FuncLit:
			s, _ := info.TypeOf(f).(*types.Signature)
			return s
		case *ast.FuncDecl:
			if fn, ok := info.Defs[f.Name].(*types.Func); ok {
				return fn.Signature()
			}
			return nil
		}
	}
	return nil
}

func (p *pass) conv(info *types.Info, anc []ast.Node, e ast.Expr, target types.Type) {
	if !types.IsInterface(target) || p.reported[e] {
		return
	}
	p.convType(info, anc, e, info.TypeOf(e), target, true)
}

// convType: a value of type src becomes the interface target at e (an
// assignment, an argument, or a generic type argument meeting its
// constraint). Every method of a connector type that can then be called
// through the interface (any method, since an interface value can be
// asserted to another) is reachable from here: the before-main closure
// follows it, and an environment wrapper reached this way is unresolved.
// A method that would start a sink makes the conversion a finding unless
// the kit takes the value (kitArg).
func (p *pass) convType(info *types.Info, anc []ast.Node, e ast.Expr, src, target types.Type, kitArg bool) {
	if src == nil || types.IsInterface(src) {
		return
	}
	ctx := context(anc)
	ms := types.NewMethodSet(src)
	for i := 0; i < ms.Len(); i++ {
		if fn, ok := ms.At(i).Obj().(*types.Func); ok && fn.Pkg() != nil && p.local[fn.Pkg().Path()] {
			p.exposed[fn.Origin()] = true
			if ctx != nil {
				p.edges[ctx] = append(p.edges[ctx], fn.Origin())
			}
		}
	}
	it, ok := types.Unalias(target).Underlying().(*types.Interface)
	if !ok {
		return
	}
	for i := 0; i < it.NumMethods(); i++ {
		m := it.Method(i)
		obj, _, _ := types.LookupFieldOrMethod(src, true, m.Pkg(), m.Name())
		fn, ok := obj.(*types.Func)
		if !ok {
			continue
		}
		key := objKey(fn)
		switch {
		case execRun[key], key == mcpServerPkg+".StdioServer.Listen", key == mcpServerPkg+".StreamableHTTPServer.Start", key == mcpServerPkg+".SSEServer.Start":
		case key == mcpServerPkg+".StreamableHTTPServer.ServeHTTP", key == mcpServerPkg+".SSEServer.ServeHTTP":
			if kitArg && p.underKit(info, anc, e, "MCPHTTP") {
				continue
			}
		case fn.Name() == "RoundTrip" && fn.Pkg() != nil && fn.Pkg().Path() != p.kit:
			if kitArg && p.underKit(info, anc, e, "Transport") {
				continue
			}
		default:
			continue
		}
		if p.sinkOf(src) != "" {
			return // the sink allow-list (sinkExpr) reports a sink type itself
		}
		p.site(e, anc, 2, "convert:"+key, fmt.Sprintf("a %s becomes a %s here, so its %s can run where the checker cannot see it", types.TypeString(src, nil), types.TypeString(target, nil), m.Name()), "pass it straight to the kit (credkit.Transport, credkit.MCPHTTP) or keep its concrete type")
		return
	}
}

// instance: a generic function or type instantiated here; each type
// argument meets its type parameter's constraint, which is a conversion.
func (p *pass) instance(info *types.Info, id *ast.Ident, stack []ast.Node) {
	inst, ok := info.Instances[id]
	if !ok {
		return
	}
	var tps *types.TypeParamList
	switch o := info.Uses[id].(type) {
	case *types.Func:
		tps = o.Signature().TypeParams()
	case *types.TypeName:
		if n, ok := types.Unalias(o.Type()).(*types.Named); ok {
			tps = n.TypeParams()
		}
	}
	if tps == nil {
		return
	}
	anc := append(stack[:len(stack):len(stack)], id)
	for i := 0; i < inst.TypeArgs.Len(); i++ {
		if k := p.holds(inst.TypeArgs.At(i), genericSinks, map[types.Type]bool{}); k != "" {
			e, _ := exprOf(id, stack)
			p.site(e, stack, 2, "generic:"+k, "a generic instantiated with "+types.TypeString(inst.TypeArgs.At(i), nil)+", which holds a "+k+": the checker does not follow a sink through a type parameter", "write the code for the concrete type, without a type parameter")
			return
		}
	}
	// A type argument that holds a sink anywhere, named structs' fields
	// included: inside the generic it is a type parameter, so neither the
	// carrier checks nor field reads see the sink (class generic-carriers).
	for i := 0; i < inst.TypeArgs.Len(); i++ {
		if k := p.deepHolds(inst.TypeArgs.At(i), map[types.Type]bool{}); k != "" {
			e, _ := exprOf(id, stack)
			p.site(e, stack, 2, "generic:"+k, "a generic instantiated with "+types.TypeString(inst.TypeArgs.At(i), nil)+", which holds a "+k+" in its structure: the checker does not follow a sink through a type parameter", "write the code for the concrete type, without a type parameter")
			return
		}
	}
	for i := 0; i < inst.TypeArgs.Len(); i++ {
		if k := p.ifaceSink(inst.TypeArgs.At(i), map[types.Type]bool{}, true); k != "" {
			e, _ := exprOf(id, stack)
			p.site(e, stack, 2, "generic:assert:"+k, "a generic instantiated with "+types.TypeString(inst.TypeArgs.At(i), nil)+", an interface a "+k+" satisfies with a method that runs or sends: an assertion to the type parameter recovers one unseen", "write the code for the concrete type, without a type parameter")
			return
		}
	}
	for i := 0; i < inst.TypeArgs.Len() && i < tps.Len(); i++ {
		p.convType(info, anc, id, inst.TypeArgs.At(i), tps.At(i).Constraint(), false)
	}
}

// ifaceSink: t is or carries (through pointers, containers, functions and
// structs, named ones included) an interface a sink satisfies with a method
// that runs or sends (see sinkSatisfies), or "". withTransport counts a
// transport's RoundTrip; without it only clients and commands count.
func (p *pass) ifaceSink(t types.Type, seen map[types.Type]bool, withTransport bool) string {
	if t == nil || seen[t] {
		return ""
	}
	seen[t] = true
	if _, tp := t.(*types.TypeParam); tp {
		return ""
	}
	switch u := t.Underlying().(type) {
	case *types.Interface:
		k := p.sinkSatisfies(u)
		if k == "net/http.Transport" && !withTransport {
			return ""
		}
		return k
	case *types.Pointer:
		return p.ifaceSink(u.Elem(), seen, withTransport)
	case *types.Slice:
		return p.ifaceSink(u.Elem(), seen, withTransport)
	case *types.Array:
		return p.ifaceSink(u.Elem(), seen, withTransport)
	case *types.Chan:
		return p.ifaceSink(u.Elem(), seen, withTransport)
	case *types.Map:
		if k := p.ifaceSink(u.Key(), seen, withTransport); k != "" {
			return k
		}
		return p.ifaceSink(u.Elem(), seen, withTransport)
	case *types.Signature:
		if k := p.ifaceSink(u.Params(), seen, withTransport); k != "" {
			return k
		}
		return p.ifaceSink(u.Results(), seen, withTransport)
	case *types.Tuple:
		for i := 0; u != nil && i < u.Len(); i++ {
			if k := p.ifaceSink(u.At(i).Type(), seen, withTransport); k != "" {
				return k
			}
		}
	case *types.Struct:
		// A named struct is read field by field: the connector's own fields
		// are set by code the checker walks, another package's are values of
		// foreign origin (foreignOrigin).
		if _, named := types.Unalias(t).(*types.Named); named {
			return ""
		}
		for i := 0; i < u.NumFields(); i++ {
			if k := p.ifaceSink(u.Field(i).Type(), seen, withTransport); k != "" {
				return k
			}
		}
	}
	return ""
}

// envRead: os.Getenv and os.LookupEnv. The name is a constant, a parameter of
// a wrapper (resolved at the wrapper's call sites), or unresolvable.
func (p *pass) envRead(info *types.Info, e ast.Expr, stack []ast.Node, call *ast.CallExpr, key string) {
	if call == nil || len(call.Args) != 1 {
		p.envUnresolved(e, stack, key)
		return
	}
	if s, ok := constString(info, call.Args[0]); ok {
		p.envNames = append(p.envNames, envSite{s, p.at(call, stack, 3, key, "", "")})
		return
	}
	if fn := encl(info, stack); fn != nil {
		if i := p.paramIndex(info, fn, call.Args[0]); i >= 0 {
			p.wrappers = append(p.wrappers, [2]any{fn, i})
			return
		}
	}
	p.envUnresolved(e, stack, key)
}

// paramIndex is the index of the parameter e names in fn, or -1. A variadic
// parameter does not count, and neither does one fn assigns, increments,
// ranges into or takes the address of anywhere in its body: its value at the
// read is then not what the caller passed.
func (p *pass) paramIndex(info *types.Info, fn *types.Func, e ast.Expr) int {
	id, ok := unparen(e).(*ast.Ident)
	if !ok {
		return -1
	}
	v := info.Uses[id]
	ps := fn.Signature().Params()
	for i := 0; i < ps.Len(); i++ {
		if ps.At(i) == v && !(fn.Signature().Variadic() && i == ps.Len()-1) {
			if p.modified(info, fn, ps.At(i)) {
				return -1
			}
			return i
		}
	}
	return -1
}

// modified reports whether fn's body may change the variable v.
func (p *pass) modified(info *types.Info, fn *types.Func, v *types.Var) bool {
	d, ok := p.funcDecl[fn].(*ast.FuncDecl)
	if !ok || d.Body == nil {
		return true
	}
	is := func(e ast.Expr) bool {
		id, ok := unparen(e).(*ast.Ident)
		return ok && (info.Uses[id] == v || info.Defs[id] == v)
	}
	found := false
	ast.Inspect(d.Body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.AssignStmt:
			for _, l := range n.Lhs {
				found = found || is(l)
			}
		case *ast.IncDecStmt:
			found = found || is(n.X)
		case *ast.RangeStmt:
			found = found || (n.Key != nil && is(n.Key)) || (n.Value != nil && is(n.Value))
		case *ast.UnaryExpr:
			found = found || (n.Op == token.AND && is(n.X))
		}
		return !found
	})
	return found
}

func (p *pass) envUnresolved(e ast.Expr, stack []ast.Node, key string) {
	f := p.at(e, stack, 3, key, "an environment read whose variable name is not a constant the checker can follow", `read a constant name, or list the site with "names" (every variable it may read, each then classified in the Declarations)`)
	f.Site, f.Env = true, true
	p.out = append(p.out, f)
}

// envTags: a struct field tag env:"NAME" reads that variable.
func (p *pass) envTags(info *types.Info, st *ast.StructType, stack []ast.Node) {
	for _, fl := range st.Fields.List {
		if fl.Tag == nil {
			continue
		}
		tag, err := strconv.Unquote(fl.Tag.Value)
		if err != nil {
			continue
		}
		if v, ok := reflect.StructTag(tag).Lookup("env"); ok {
			name, _, _ := strings.Cut(v, ",")
			if name != "" && name != "-" {
				p.envNames = append(p.envNames, envSite{name, p.at(fl, stack, 3, "tag env", "", "")})
			}
		}
	}
}

// prologue: rule 1, and the before-main closure for output calls.
func (p *pass) prologue() {
	good := map[*ast.CallExpr]bool{}
	for _, pk := range p.pkgs {
		if pk.Name != "main" {
			continue
		}
		var mainDecl *ast.FuncDecl
		for _, f := range pk.Syntax {
			for _, d := range f.Decls {
				if fd, ok := d.(*ast.FuncDecl); ok && fd.Recv == nil && fd.Name.Name == "main" {
					mainDecl = fd
				}
			}
		}
		if mainDecl == nil {
			continue
		}
		stack := []ast.Node{nil, mainDecl}
		var first *ast.CallExpr
		if mainDecl.Body != nil && len(mainDecl.Body.List) > 0 {
			if es, ok := mainDecl.Body.List[0].(*ast.ExprStmt); ok {
				if c, ok := unparen(es.X).(*ast.CallExpr); ok && p.isKit(calleeObj(pk.TypesInfo, c.Fun), "Start") {
					first = c
				}
			}
		}
		if first == nil {
			// Not a site: the ledger never waives the prologue.
			p.out = append(p.out, p.at(mainDecl.Name, stack, 1, "credkit.Start", "main's first statement is not credkit.Start(...), so output before it is unchecked and startup credentials are not held", "make credkit.Start(<the Declarations variable>) the first statement of main"))
			continue
		}
		good[first] = true
	}
	for _, s := range p.starts {
		if s.call == nil || !good[s.call] {
			f := s.f
			f.Msg, f.Fix, f.Site = "credkit.Start used other than as the first statement of a main", "call it only as the first statement of each main", false
			p.out = append(p.out, f)
		}
	}
	// The before-main closure.
	in := map[ast.Node]bool{}
	work := append([]ast.Node(nil), p.roots...)
	for len(work) > 0 {
		n := work[len(work)-1]
		work = work[:len(work)-1]
		if n == nil || in[n] {
			continue
		}
		in[n] = true
		for _, fn := range p.edges[n] {
			if d, ok := p.funcDecl[fn]; ok && !in[d] {
				work = append(work, d)
			}
		}
	}
	for _, w := range p.writes {
		if in[w.ctx] {
			p.out = append(p.out, w.f)
		}
	}
}

// decls is the connector's Declarations literal, read as plain data.
type decls struct {
	found, files                                                        bool
	filesExpr                                                           ast.Expr // the Files value
	filesInfo                                                           *types.Info
	at                                                                  Finding
	SecretEnv, ConfigFields, StoredRecords, CredentialFlags, StdinFlags []string
	CredentialHeaders, CredentialFields, ResponseFields, MCPArguments   []string
	BaseConfigKey, BaseEnv, BaseDefault                                 string
	Origins                                                             []origin
	Identifiers                                                         [][2]string
}

type origin struct {
	source  string
	allowed []string
}

func (p *pass) declFinding(at Finding, msg, fix string) {
	f := at
	f.Rule, f.What, f.Msg, f.Fix, f.Site = 1, "credkit.Declarations", msg, fix, false
	p.out = append(p.out, f)
}

// declarations reads the one Declarations literal and checks that every main's
// Start receives it.
func (p *pass) declarations() *decls {
	d := &decls{}
	if len(p.declLits) != 1 {
		at := Finding{File: ".", Func: "file"}
		if len(p.declLits) > 1 {
			at = p.declLits[1].f
		}
		p.declFinding(at, fmt.Sprintf("the connector has %d credkit.Declarations literals; it must have exactly one", len(p.declLits)), "write one Declarations literal in the wiring file")
		return d
	}
	dl := p.declLits[0]
	d.found, d.at = true, dl.f
	info := dl.info
	bad := func(n ast.Node, msg string) {
		f := dl.f
		pos := p.fset.Position(n.Pos())
		f.Line, f.Col = pos.Line, pos.Column
		p.declFinding(f, msg, "write it as constant strings in the literal")
	}
	strs := func(e ast.Expr) []string {
		if id, ok := unparen(e).(*ast.Ident); ok && id.Name == "nil" {
			return nil
		}
		cl, ok := unparen(e).(*ast.CompositeLit)
		if !ok {
			bad(e, "a declaration the checker cannot read (not a []string literal)")
			return nil
		}
		var out []string
		for _, el := range cl.Elts {
			s, ok := constString(info, el)
			if !ok {
				bad(el, "a declaration the checker cannot read (not a constant string)")
				continue
			}
			out = append(out, s)
		}
		return out
	}
	fields := func(e ast.Expr) map[string]ast.Expr {
		cl, ok := unparen(e).(*ast.CompositeLit)
		if !ok {
			bad(e, "a declaration the checker cannot read (not a composite literal)")
			return nil
		}
		m := map[string]ast.Expr{}
		for _, el := range cl.Elts {
			kv, ok := el.(*ast.KeyValueExpr)
			if !ok {
				bad(el, "a positional element; the checker reads keyed fields only")
				continue
			}
			if id, ok := kv.Key.(*ast.Ident); ok {
				m[id.Name] = kv.Value
			}
		}
		return m
	}
	str := func(m map[string]ast.Expr, k string) string {
		e, ok := m[k]
		if !ok {
			return ""
		}
		s, ok := constString(info, e)
		if !ok {
			bad(e, "a declaration the checker cannot read (not a constant string)")
		}
		return s
	}
	elems := func(e ast.Expr) []ast.Expr {
		if id, ok := unparen(e).(*ast.Ident); ok && id.Name == "nil" {
			return nil
		}
		cl, ok := unparen(e).(*ast.CompositeLit)
		if !ok {
			bad(e, "a declaration the checker cannot read (not a slice literal)")
			return nil
		}
		return cl.Elts
	}
	for k, v := range fields(dl.lit) {
		switch k {
		case "SecretEnv":
			d.SecretEnv = strs(v)
		case "ConfigFields":
			d.ConfigFields = strs(v)
		case "StoredRecords":
			d.StoredRecords = strs(v)
		case "CredentialFlags":
			d.CredentialFlags = strs(v)
		case "StdinFlags":
			d.StdinFlags = strs(v)
		case "CredentialHeaders":
			d.CredentialHeaders = strs(v)
		case "CredentialFields":
			d.CredentialFields = strs(v)
		case "ResponseFields":
			d.ResponseFields = strs(v)
		case "MCPArguments":
			d.MCPArguments = strs(v)
		case "BaseURL":
			m := fields(v)
			d.BaseConfigKey, d.BaseEnv, d.BaseDefault = str(m, "ConfigKey"), str(m, "Env"), str(m, "Default")
		case "Origins":
			for _, el := range elems(v) {
				m := fields(el)
				o := origin{source: str(m, "Source")}
				if a, ok := m["Allowed"]; ok {
					o.allowed = strs(a)
				}
				d.Origins = append(d.Origins, o)
			}
		case "Identifiers":
			for _, el := range elems(v) {
				m := fields(el)
				d.Identifiers = append(d.Identifiers, [2]string{str(m, "Source"), str(m, "Reason")})
			}
		case "Files":
			d.filesExpr, d.filesInfo = v, info
			switch f := unparen(v).(type) {
			case *ast.FuncLit:
				d.files = true
			default:
				if id, ok := f.(*ast.Ident); ok && id.Name == "nil" {
					break
				}
				if isPkgFunc(orNil(calleeObj(info, f))) {
					d.files = true
				} else {
					bad(v, "Files is not a function literal or a package-level function")
				}
			}
		default:
			bad(v, "an unknown Declarations field "+k)
		}
	}
	// Every Start receives this literal: directly, or as the variable it initializes.
	for _, s := range p.starts {
		if s.call == nil || len(s.call.Args) != 1 {
			continue
		}
		a := unparen(s.call.Args[0])
		if a == dl.lit {
			continue
		}
		if dl.v != nil && s.info.Uses[identOf(a)] == dl.v {
			continue
		}
		f := s.f
		p.declFinding(f, "credkit.Start is given something other than the connector's one Declarations literal", "pass the Declarations variable the wiring file initializes")
	}
	// The variable is read only by Start, so nothing changes it after the checker read it.
	for _, r := range p.declRefs {
		if r.f.What == "credkit.Declarations" && dl.v != nil && p.isStartArg(r) {
			continue
		}
		p.declFinding(r.f, "a use of a Declarations variable other than as credkit.Start's argument (it could change what the checker read)", "use the Declarations variable only in credkit.Start(...)")
	}
	for _, e := range p.ledger.Sites {
		if e.Kind == "source" && !d.files {
			p.declFinding(d.at, fmt.Sprintf("the ledger lists %s in %s as a credential source, but the Declarations have no Files function, so Start reads no credential file", e.What, e.File), "declare Files, returning every credential file the program reads")
			break
		}
	}
	for _, id := range d.Identifiers {
		if strings.TrimSpace(id[0]) == "" || strings.TrimSpace(id[1]) == "" {
			p.declFinding(d.at, fmt.Sprintf("identifier %q has no source or no reason", id[0]), "give each identifier its source and the reason it is not a credential")
		}
	}
	return d
}

func identOf(e ast.Expr) *ast.Ident {
	switch x := e.(type) {
	case *ast.Ident:
		return x
	case *ast.SelectorExpr:
		return x.Sel
	}
	return nil
}

// isStartArg reports whether a Declarations variable use is the argument of
// a recorded first-statement Start call.
func (p *pass) isStartArg(r ref) bool {
	for _, s := range p.starts {
		if s.call == nil || len(s.call.Args) != 1 {
			continue
		}
		a := p.fset.Position(identOrSelf(unparen(s.call.Args[0])).Pos())
		if a.Filename != "" && filepath.ToSlash(relTo(p.cli, a.Filename)) == r.f.File && a.Line == r.f.Line && a.Column == r.f.Col {
			return true
		}
	}
	return false
}

func identOrSelf(e ast.Expr) ast.Node {
	if id := identOf(e); id != nil {
		return id
	}
	return e
}

func relTo(base, p string) string {
	r, err := filepath.Rel(base, p)
	if err != nil {
		return p
	}
	return r
}

func (d *decls) identifier(source string) bool {
	for _, id := range d.Identifiers {
		if id[0] == source && strings.TrimSpace(id[1]) != "" {
			return true
		}
	}
	return false
}

func containsFold(xs []string, s string) bool {
	for _, x := range xs {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// envs: rule 3 for environment variables. Wrappers resolve at their call
// sites; every resolved name, struct tag name and ledger-listed name must be
// declared secret or an identifier with a reason.
func (p *pass) envs(d *decls) {
	done := map[[2]any]bool{}
	work := append([][2]any(nil), p.wrappers...)
	for len(work) > 0 {
		w := work[len(work)-1]
		work = work[:len(work)-1]
		if done[w] {
			continue
		}
		done[w] = true
		fn, idx := w[0].(*types.Func), w[1].(int)
		if p.exposed[fn] {
			pos := p.fset.Position(fn.Pos())
			p.out = append(p.out, Finding{Rule: 3, File: filepath.ToSlash(relTo(p.cli, pos.Filename)), Line: pos.Line, Col: pos.Column, Func: fn.Name(), What: objKey(fn),
				Msg: "a wrapper of an environment read that can be called through an interface, where the checker cannot follow its argument", Fix: "read each variable by a constant name in a plain function, not a method an interface reaches", Env: true})
		}
		for _, r := range p.refs[fn] {
			f := r.f
			f.Func, f.What = r.f.Func, objKey(fn)
			if r.call == nil || r.method || idx >= len(r.call.Args) {
				f.Msg, f.Fix = "a wrapper of an environment read used where the checker cannot follow its argument (as a value or a method expression)", `call it directly with a constant name, or list the site with "names"`
				f.Site, f.Env = true, true
				p.out = append(p.out, f)
				continue
			}
			a := r.call.Args[idx]
			if s, ok := constString(r.info, a); ok {
				p.envNames = append(p.envNames, envSite{s, f})
				continue
			}
			if r.encl != nil {
				if j := p.paramIndex(r.info, r.encl, a); j >= 0 {
					work = append(work, [2]any{r.encl, j})
					continue
				}
			}
			f.Msg, f.Fix = "a call to a wrapper of an environment read whose name argument is not a constant", `pass a constant name, or list the site with "names"`
			f.Site, f.Env = true, true
			p.out = append(p.out, f)
		}
	}
	classified := func(n string) bool {
		return contains(d.SecretEnv, n) || (d.BaseEnv != "" && n == d.BaseEnv) || d.identifier("env:"+n)
	}
	for _, e := range p.envNames {
		if !classified(e.name) {
			f := e.f
			f.Rule, f.Site = 3, false
			f.Msg = fmt.Sprintf("environment variable %s is read but not classified", e.name)
			f.Fix = fmt.Sprintf(`add %q to SecretEnv, or Identifiers: {Source: "env:%s", Reason: "<why it is not a credential>"}`, e.name, e.name)
			p.out = append(p.out, f)
		}
	}
	for _, n := range ledgerNames(p.ledger) {
		if !classified(n) {
			p.out = append(p.out, Finding{Rule: 3, File: ledgerName, What: "names", Msg: fmt.Sprintf("environment variable %s (listed in the ledger) is not classified", n),
				Fix: fmt.Sprintf(`add %q to SecretEnv, or Identifiers: {Source: "env:%s", Reason: "..."}`, n, n)})
		}
	}
	for _, n := range d.SecretEnv {
		if d.identifier("env:" + n) {
			p.declFinding(d.at, fmt.Sprintf("env:%s is declared both secret and an identifier", n), "keep one classification")
		}
	}
}

// configTypes: rule 3 for every string-bearing field of the connector's
// Config type(s): each key it is written under is a credential field, a
// stored record, a key the kit always holds, the base URL key, or an
// identifier with a reason.
func (p *pass) configTypes(d *decls) {
	type named struct {
		t   *types.TypeName
		pkg *packages.Package
	}
	var ts []named
	for _, pk := range p.pkgs {
		if tn, ok := pk.Types.Scope().Lookup("Config").(*types.TypeName); ok {
			ts = append(ts, named{tn, pk})
		}
	}
	for _, ct := range p.ledger.ConfigTypes {
		path, name, ok := cutLast(ct, ".")
		var found *types.TypeName
		for _, pk := range p.pkgs {
			if ok && pk.PkgPath == p.mod+"/"+path {
				found, _ = pk.Types.Scope().Lookup(name).(*types.TypeName)
			}
		}
		if found == nil {
			p.out = append(p.out, Finding{Rule: 3, File: ledgerName, What: "config_types", Msg: fmt.Sprintf("config type %q is not a type of this connector", ct), Fix: `name it as "<package path in the module>.<Type>"`})
			continue
		}
		ts = append(ts, named{found, nil})
	}
	if len(ts) == 0 && len(p.decoded) == 0 {
		p.out = append(p.out, Finding{Rule: 3, File: ".", What: "Config", Msg: "the connector has no Config type for the checker to classify", Fix: fmt.Sprintf("name the connector's config type in %s config_types", ledgerName)})
		return
	}
	baseKey := "base_url"
	if d.BaseConfigKey != "" {
		parts := strings.Split(d.BaseConfigKey, ".")
		baseKey = parts[len(parts)-1]
	}
	classified := func(k string, t types.Type) bool {
		return containsFold(append(append([]string{}, d.ConfigFields...), d.StoredRecords...), k) ||
			strings.EqualFold(k, "auth_header") || (strings.EqualFold(k, "headers") && table(t)) || strings.EqualFold(k, baseKey) ||
			d.identifier("config:"+k)
	}
	type walked struct {
		t      types.Type
		format string
	}
	seen := map[walked]bool{}
	var walk func(t types.Type, format string)
	walk = func(t types.Type, format string) {
		st, ok := types.Unalias(t).Underlying().(*types.Struct)
		if !ok || seen[walked{t, format}] {
			return
		}
		seen[walked{t, format}] = true
		for i := 0; i < st.NumFields(); i++ {
			fv := st.Field(i)
			if fv.Pkg() == nil || (!p.local[fv.Pkg().Path()] && !fv.Exported()) {
				continue
			}
			for _, inner := range structsIn(fv.Type()) {
				walk(inner, format)
			}
			if il := inlineMap(fv, st.Tag(i)); il != "" {
				pos := p.fset.Position(fv.Pos())
				p.out = append(p.out, Finding{Rule: 3, File: filepath.ToSlash(relTo(p.cli, pos.Filename)), Line: pos.Line, Col: pos.Column, What: "config:" + fv.Name() + ",inline",
					Msg: fmt.Sprintf("Config field %s (%s) takes every key no other field takes, so no declaration can classify them", fv.Name(), il),
					Fix: "decode into named fields, or a map under a key of its own"})
				continue
			}
			// A type that decodes itself counts only where a decoder can
			// reach it: an exported field (every decoder skips the others).
			if !valueBearing(fv.Type(), map[types.Type]bool{}, fv.Exported()) {
				continue
			}
			for _, k := range formatKeys(configKeys(fv.Name(), st.Tag(i)), fv.Name(), st.Tag(i), format) {
				if classified(k, fv.Type()) {
					continue
				}
				pos := p.fset.Position(fv.Pos())
				p.out = append(p.out, Finding{Rule: 3, File: filepath.ToSlash(relTo(p.cli, pos.Filename)), Line: pos.Line, Col: pos.Column, What: "config:" + k,
					Msg: fmt.Sprintf("Config field %s (key %q) can hold a string or a number but is not classified", fv.Name(), k),
					Fix: fmt.Sprintf(`add %q to ConfigFields or StoredRecords, or Identifiers: {Source: "config:%s", Reason: "<why it is not a credential>"}`, k, k)})
			}
		}
	}
	// A config type that decodes itself (a promoted UnmarshalJSON from an
	// embedded big.Int) is read whole, under no key its fields name.
	selfRoot := func(t types.Type, at Finding) bool {
		if !selfDecodes(t) {
			return false
		}
		var obj token.Pos
		if n, ok := types.Unalias(t).(*types.Named); ok {
			obj = n.Obj().Pos()
		} else if st, ok := t.Underlying().(*types.Struct); ok && at.File == "" && st.NumFields() > 0 {
			obj = st.Field(0).Pos() // the SSA pass records no read site
		}
		if obj.IsValid() {
			pos := p.fset.Position(obj)
			at = Finding{File: filepath.ToSlash(relTo(p.cli, pos.Filename)), Line: pos.Line, Col: pos.Column}
		}
		name := types.TypeString(t, func(*types.Package) string { return "" })
		p.out = append(p.out, Finding{Rule: 3, File: at.File, Line: at.Line, Col: at.Col, What: "config type " + name + " decodes itself",
			Msg: "config type " + name + " has a decoding method, so a decoder reads it whole and its fields do not name its keys", Fix: "decode into a struct whose fields name every key"})
		return true
	}
	for _, dt := range p.decoded {
		if selfRoot(dt.t, dt.f) {
			continue
		}
		walk(dt.t, "") // structTarget proved it a struct
		for f := range p.formats[dt.t] {
			walk(dt.t, f)
		}
	}
	for _, t := range ts {
		root, ok := derefStruct(t.t.Type())
		if !ok {
			pos := p.fset.Position(t.t.Pos())
			p.out = append(p.out, Finding{Rule: 3, File: filepath.ToSlash(relTo(p.cli, pos.Filename)), Line: pos.Line, Col: pos.Column, What: "config type " + t.t.Name(),
				Msg: "config type " + t.t.Name() + " is not a struct (or a pointer to one), so the checker cannot read its fields", Fix: "make it a struct whose fields name every key"})
			continue
		}
		pos := p.fset.Position(t.t.Pos())
		if selfRoot(root, Finding{File: filepath.ToSlash(relTo(p.cli, pos.Filename)), Line: pos.Line, Col: pos.Column}) {
			continue
		}
		walk(root, "")
	}
	for _, id := range d.Identifiers {
		if k, ok := strings.CutPrefix(id[0], "config:"); ok && containsFold(append(append([]string{}, d.ConfigFields...), d.StoredRecords...), k) {
			p.declFinding(d.at, fmt.Sprintf("config:%s is declared both a credential and an identifier", k), "keep one classification")
		}
	}
}

// table reports whether a config field is a table (a map or a struct, through
// pointers): the kit holds every value beneath a headers table, not a scalar
// named headers.
func table(t types.Type) bool {
	for {
		switch u := types.Unalias(t).Underlying().(type) {
		case *types.Pointer:
			t = u.Elem()
			continue
		case *types.Map, *types.Struct:
			return true
		}
		return false
	}
}

func cutLast(s, sep string) (string, string, bool) {
	i := strings.LastIndex(s, sep)
	if i < 0 {
		return "", "", false
	}
	return s[:i], s[i+1:], true
}

// configKeys is every key a field is written under: its toml, json, yaml and
// mapstructure tag names, or its Go name when it has none.
// formatKeys adds to keys the key the decoder of format reads the field
// under: its tag for that format, or its Go name without one ("?", an
// unknown decoder: the Go name too). A tag of another format does not
// rename it (class decoder-keys: toml:"token" decoded as JSON is "Shadow").
func formatKeys(keys []string, name, tag, format string) []string {
	if format == "" {
		return keys
	}
	k := name
	if v, ok := reflect.StructTag(tag).Lookup(format); ok && format != "?" {
		n, skip := tagKey(format, v)
		if skip {
			return keys
		}
		if n != "" {
			k = n
		}
	}
	if !containsFold(keys, k) {
		keys = append(append([]string{}, keys...), k)
	}
	return keys
}

func configKeys(name, tag string) []string {
	var keys []string
	for _, k := range []string{"toml", "json", "yaml", "mapstructure"} {
		if v, ok := reflect.StructTag(tag).Lookup(k); ok {
			n, skip := tagKey(k, v)
			if skip {
				continue
			}
			if n == "" && k == "json" {
				n = name // a json tag with no usable name: encoding/json reads the Go name
			}
			if n != "" && !contains(keys, n) {
				keys = append(keys, n)
			}
		}
	}
	if len(keys) == 0 {
		keys = []string{name}
	}
	return keys
}

// tagKey is the key a decoder of format reads under the tag value v: "" for
// the Go name, skip for a field it does not read. encoding/json reads "-,"
// as the key "-" and ignores a name it rejects (validJSONTag), reading the Go
// name instead.
func tagKey(format, v string) (key string, skip bool) {
	if v == "-" {
		return "", true
	}
	n, _, _ := strings.Cut(v, ",")
	if format == "json" && n != "-" && !validJSONTag(n) {
		return "", false
	}
	return n, false
}

// validJSONTag is encoding/json's isValidTag.
func validJSONTag(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		switch {
		case strings.ContainsRune("!#$%&()*+-./:;<=>?@[]^_{|}~ ", c):
		case !unicode.IsLetter(c) && !unicode.IsDigit(c):
			return false
		}
	}
	return true
}

// selfDecodes: t (or *t) has a decoding method (an Unmarshal* or *Decode
// method, as selfDecoder looks for), its own or promoted from a type it
// embeds (struct{ big.Int }), so a decoder fills it from a scalar however
// its fields look.
func selfDecodes(t types.Type) bool {
	if types.IsInterface(t) {
		return false
	}
	ms := types.NewMethodSet(types.NewPointer(t))
	for i := 0; i < ms.Len(); i++ {
		if n := ms.At(i).Obj().Name(); strings.HasPrefix(n, "Unmarshal") || strings.HasSuffix(n, "Decode") {
			return true
		}
	}
	return false
}

// inlineMap: a field a decoder fills with every key it finds that no other
// field takes (yaml ",inline" or mapstructure ",remain" on a map or an
// interface): no key list can classify it.
func inlineMap(fv *types.Var, tag string) string {
	switch fv.Type().Underlying().(type) {
	case *types.Map, *types.Interface:
	default:
		return ""
	}
	for _, f := range [][2]string{{"yaml", "inline"}, {"mapstructure", "remain"}} {
		if v, ok := reflect.StructTag(tag).Lookup(f[0]); ok {
			_, opts, _ := strings.Cut(v, ",")
			for _, o := range strings.Split(opts, ",") {
				if o == f[1] {
					return f[0] + `:",` + f[1] + `"`
				}
			}
		}
	}
	return ""
}

// structsIn is every struct type a field's type holds (through pointers,
// slices, arrays, maps and channels).
func structsIn(t types.Type) []types.Type { return structsInSeen(t, map[types.Type]bool{}) }

func structsInSeen(t types.Type, seen map[types.Type]bool) []types.Type {
	if seen[t] {
		return nil // a type that holds itself (type L []L)
	}
	seen[t] = true
	switch u := types.Unalias(t).Underlying().(type) {
	case *types.Struct:
		return []types.Type{t}
	case *types.Pointer:
		return structsInSeen(u.Elem(), seen)
	case *types.Slice:
		return structsInSeen(u.Elem(), seen)
	case *types.Array:
		return structsInSeen(u.Elem(), seen)
	case *types.Map:
		return append(structsInSeen(u.Key(), seen), structsInSeen(u.Elem(), seen)...)
	case *types.Chan:
		return structsInSeen(u.Elem(), seen)
	}
	return nil
}

// valueBearing reports whether a value of t can hold a string or a number
// (numbers count, contract section 1; a bool holds one bit) outside a
// nested struct (which is walked field by field): a string, a []byte, an
// interface, or a pointer, slice, array, map or channel of one.
func valueBearing(t types.Type, seen map[types.Type]bool, self bool) bool {
	if seen[t] {
		return false
	}
	seen[t] = true
	if self && selfDecodes(t) {
		return true // a type that decodes itself (big.Int, time.Time) holds whatever its text says
	}
	switch u := types.Unalias(t).Underlying().(type) {
	case *types.Basic:
		return u.Info()&(types.IsString|types.IsNumeric) != 0
	case *types.Interface:
		return true
	case *types.Pointer:
		return valueBearing(u.Elem(), seen, self)
	case *types.Slice:
		if b, ok := u.Elem().Underlying().(*types.Basic); ok && b.Kind() == types.Byte {
			return true
		}
		return valueBearing(u.Elem(), seen, self)
	case *types.Array:
		return valueBearing(u.Elem(), seen, self)
	case *types.Map:
		return valueBearing(u.Key(), seen, self) || valueBearing(u.Elem(), seen, self)
	case *types.Chan:
		return valueBearing(u.Elem(), seen, self)
	}
	return false
}

// origins: rule 5. Every declared credential has an allowed origin.
func (p *pass) origins(d *decls) {
	if !d.found {
		return
	}
	var creds []string
	add := func(prefix string, xs []string) {
		for _, x := range xs {
			creds = append(creds, prefix+x)
		}
	}
	add("env:", d.SecretEnv)
	add("config:", d.ConfigFields)
	add("config:", d.StoredRecords)
	add("flag:", d.CredentialFlags)
	add("header:", d.CredentialHeaders)
	add("field:", d.CredentialFields)
	add("response:", d.ResponseFields)
	add("mcp:", d.MCPArguments)
	if len(d.StdinFlags) > 0 {
		creds = append(creds, "stdin")
	}
	valid := func(o origin) bool {
		if len(o.allowed) == 0 {
			return false
		}
		for _, a := range o.allowed {
			if a == "base_url" {
				continue
			}
			u, err := url.Parse(a)
			if err != nil || u.Scheme == "" || u.Host == "" || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.User != nil {
				p.declFinding(d.at, fmt.Sprintf("allowed origin %q (source %q) is not scheme://host[:port] or base_url", a, o.source), "write the origin as scheme://host[:port]")
				return false
			}
		}
		return true
	}
	ok := map[int]bool{}
	for i, o := range d.Origins {
		ok[i] = valid(o)
	}
	for _, c := range creds {
		// The kit's precedence (allowedLocked): the credential's own first
		// entry decides; only without one does the last "*" entry apply.
		has := false
		for i, o := range d.Origins {
			if sameSource(o.source, c) {
				has = ok[i]
				break
			}
			if o.source == "*" {
				has = ok[i]
			}
		}
		if !has {
			f := d.at
			f.Rule, f.What, f.Site = 5, c, false
			f.Msg = fmt.Sprintf("credential %s has no allowed origin, so the kit refuses to send it anywhere", c)
			f.Fix = fmt.Sprintf(`add Origins: {Source: %q, Allowed: []string{"https://..."}} (or a "*" default)`, c)
			p.out = append(p.out, f)
		}
	}
}

// sameSource is the kit's own comparison: exact, except a header name ignores case.
func sameSource(declared, source string) bool {
	const h = "header:"
	if strings.HasPrefix(strings.ToLower(declared), h) && strings.HasPrefix(strings.ToLower(source), h) {
		return strings.EqualFold(declared, source)
	}
	return declared == source
}

// sqlOpen: every sql.Open names a driver registered through credkit.SQLDriver.
// The registration must be a statement of an init function in the opening
// package or a package it imports, so it has run before the open.
func (p *pass) sqlOpen() {
	for _, o := range p.sqlOpens {
		ok := false
		for _, r := range p.sqlRegs {
			ok = ok || (o.name != "" && r.name == o.name && imports(o.pkg, r.pkg, map[*types.Package]bool{}))
		}
		if !ok {
			p.out = append(p.out, o.f)
		}
	}
}

// imports reports whether a is b or imports it, directly or not.
func imports(a, b *types.Package, seen map[*types.Package]bool) bool {
	if a == b {
		return true
	}
	if seen[a] {
		return false
	}
	seen[a] = true
	for _, i := range a.Imports() {
		if imports(i, b, seen) {
			return true
		}
	}
	return false
}

// leadingRegister reports whether call is one of the leading statements of
// an init function, every statement before it a kit registration too, so
// nothing (an early return, a branch) can skip it.
func (p *pass) leadingRegister(info *types.Info, call *ast.CallExpr, stack []ast.Node) bool {
	if len(stack) < 2 {
		return false
	}
	fd, ok := stack[1].(*ast.FuncDecl)
	if !ok || fd.Recv != nil || fd.Name.Name != "init" || fd.Body == nil {
		return false
	}
	for _, st := range fd.Body.List {
		es, ok := st.(*ast.ExprStmt)
		if !ok {
			return false
		}
		c, ok := ast.Unparen(es.X).(*ast.CallExpr)
		if !ok {
			return false
		}
		if c == call {
			return true
		}
		if objKey(orNil(calleeObj(info, c.Fun))) != "database/sql.Register" || len(c.Args) != 2 || !p.isKitCall(info, c.Args[1], "SQLDriver") {
			return false
		}
	}
	return false
}

// reflectWrites: the reflect calls that write, call or escape through a value
// (with every reflect.Value.Set method).
var reflectWrites = set("reflect.Value.Call", "reflect.Value.CallSlice", "reflect.Value.Method", "reflect.Value.MethodByName",
	"reflect.Value.UnsafePointer", "reflect.Value.UnsafeAddr", "reflect.Value.Addr", "reflect.Value.Send", "reflect.Value.TrySend",
	"reflect.Value.Clear", "reflect.Copy", "reflect.Swapper", "reflect.NewAt", "reflect.MakeFunc")

// reflectArgs: reflect and unsafe reach inside a named struct, where the
// allow-lists do not follow a sink (class named-carriers), so a value that
// holds a sink anywhere in its structure is never handed to either.
func (p *pass) reflectArgs(info *types.Info, call *ast.CallExpr, stack []ast.Node) {
	if why := unsafeForge(info, call); why != "" {
		p.site(call, stack, 2, "unsafe", why+", a pointer the checker cannot see the origin of", "use unsafe only as unsafe.Pointer(&x) turned into a uintptr for a system call, or Sizeof, Offsetof and Alignof")
	}
	k := ""
	if tv := info.Types[call.Fun]; tv.IsType() {
		if b, ok := tv.Type.Underlying().(*types.Basic); ok && b.Kind() == types.UnsafePointer {
			k = "unsafe.Pointer"
		}
	} else if fn, ok := calleeObj(info, call.Fun).(*types.Func); ok && fn.Pkg() != nil && fn.Pkg().Path() == "reflect" {
		k = objKey(fn)
	}
	if k == "" {
		return
	}
	for _, a := range call.Args {
		if s := p.deepHolds(info.TypeOf(a), map[types.Type]bool{}); s != "" {
			p.site(a, stack, 2, k, "a value holding a "+s+" handed to "+k+", which reaches inside it where the checker cannot see", "do not reflect over, or take an unsafe pointer to, a value that holds a command, client or transport")
		}
	}
}

// deepHolds is holds over every structure, named structs' fields included.
func (p *pass) deepHolds(t types.Type, seen map[types.Type]bool) string {
	if t == nil || seen[t] {
		return ""
	}
	seen[t] = true
	if k := p.holds(t, sinkTypes, map[types.Type]bool{}); k != "" {
		return k
	}
	switch u := t.Underlying().(type) {
	case *types.Tuple: // f(g()): each of g's results
		for i := 0; i < u.Len(); i++ {
			if k := p.deepHolds(u.At(i).Type(), seen); k != "" {
				return k
			}
		}
	case *types.Pointer:
		return p.deepHolds(u.Elem(), seen)
	case *types.Slice:
		return p.deepHolds(u.Elem(), seen)
	case *types.Array:
		return p.deepHolds(u.Elem(), seen)
	case *types.Chan:
		return p.deepHolds(u.Elem(), seen)
	case *types.Map:
		if k := p.deepHolds(u.Key(), seen); k != "" {
			return k
		}
		return p.deepHolds(u.Elem(), seen)
	case *types.Struct:
		for i := 0; i < u.NumFields(); i++ {
			if k := p.deepHolds(u.Field(i).Type(), seen); k != "" {
				return k
			}
		}
	}
	return ""
}

func isUnsafePointer(t types.Type) bool {
	b, ok := t.(*types.Basic)
	if t != nil {
		b, ok = t.Underlying().(*types.Basic)
	}
	return ok && b.Kind() == types.UnsafePointer
}

// unsafeForge: the unsafe operations that make a pointer to memory the
// checker did not see allocated (pointer arithmetic, a uintptr made a
// pointer, a pointer reinterpreted as another type), or "".
func unsafeForge(info *types.Info, call *ast.CallExpr) string {
	if s, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr); ok {
		if id, ok := s.X.(*ast.Ident); ok {
			if pn, ok := info.Uses[id].(*types.PkgName); ok && pn.Imported().Path() == "unsafe" {
				switch s.Sel.Name {
				case "Add", "Slice", "String", "SliceData", "StringData":
					return "unsafe." + s.Sel.Name
				}
			}
		}
	}
	tv := info.Types[call.Fun]
	if !tv.IsType() || len(call.Args) != 1 {
		return ""
	}
	at := info.TypeOf(call.Args[0])
	if isUnsafePointer(tv.Type) {
		if _, ptr := at.Underlying().(*types.Pointer); !ptr && !isUnsafePointer(at) {
			return "an unsafe.Pointer made from a " + types.TypeString(at, nil)
		}
		return ""
	}
	if isUnsafePointer(at) {
		if b, ok := tv.Type.Underlying().(*types.Basic); !ok || b.Kind() != types.Uintptr {
			return "an unsafe.Pointer converted to " + types.TypeString(tv.Type, nil)
		}
	}
	return ""
}

// sinkSatisfies names a sink that satisfies it with a method beyond the ones
// that neither send nor run (class reflect-read: a value recovered from an
// interface becomes a sink only through an assertion), or "".
func (p *pass) sinkSatisfies(it *types.Interface) string {
	inert := map[string]map[string]bool{"os/exec.Cmd": set("String", "Environ"), "net/http.Client": set("CloseIdleConnections"),
		"net/http.Transport": set("Clone", "CloseIdleConnections")}
	for _, k := range sinkKinds {
		t := p.sinkNamed[k]
		if t == nil || !types.Implements(types.NewPointer(t), it) {
			continue
		}
		for i := 0; i < it.NumMethods(); i++ {
			if !inert[k][it.Method(i).Name()] {
				return k
			}
		}
	}
	return ""
}

// pathPkgs: the packages whose functions a path function may call.
var pathPkgs = set("strings", "strconv", "unicode", "unicode/utf8", "path", "path/filepath")

// pathArg: e is an argument of a call of one of pathPkgs' functions, which
// only read it (class std-mutable-state: unicode.Is(unicode.Cf, r)).
func (p *pass) pathArg(info *types.Info, e ast.Expr, estack []ast.Node) bool {
	n, st := outerParen(e, estack)
	if len(st) == 0 {
		return false
	}
	c, ok := st[len(st)-1].(*ast.CallExpr)
	if !ok || c.Fun == n {
		return false
	}
	fn, ok := calleeObj(info, c.Fun).(*types.Func)
	return ok && fn.Pkg() != nil && pathPkgs[fn.Pkg().Path()] && fn.Signature().Recv() == nil
}

// components: a tuple's element types, or t alone.
func components(t types.Type) []types.Type {
	tu, ok := t.(*types.Tuple)
	if !ok {
		return []types.Type{t}
	}
	var ts []types.Type
	for i := 0; i < tu.Len(); i++ {
		ts = append(ts, tu.At(i).Type())
	}
	return ts
}

// foreignCarrier: a value holding a command, client or transport anywhere in
// its structure (a named struct's fields included) handed to another
// package can have it replaced there, as a decoder fills a struct (class
// carrier-to-foreign). A sink handed on as itself is goesTo's.
func (p *pass) foreignCarrier(info *types.Info, call *ast.CallExpr, stack []ast.Node) {
	dest := p.foreignCallee(info, call)
	if dest == "" {
		return
	}
	for _, a := range call.Args {
		t := info.TypeOf(a)
		if p.holds(t, sinkTypes, map[types.Type]bool{}) != "" {
			continue
		}
		if k := p.deepHolds(t, map[types.Type]bool{}); k != "" {
			p.site(a, stack, 2, "carrier:"+k, "a value holding a "+k+" handed to "+dest+", which can replace it (a decoder fills the struct)", "hand on only the fields that hold no command, client or transport")
		}
	}
}

// foreignCallee names the other package's function or function value a
// call calls (not the kit's), or "".
func (p *pass) foreignCallee(info *types.Info, call *ast.CallExpr) string {
	if tv := info.Types[call.Fun]; tv.IsType() || tv.IsBuiltin() {
		return ""
	}
	if fn, ok := calleeObj(info, call.Fun).(*types.Func); ok {
		if fn.Pkg() != nil && !p.local[fn.Pkg().Path()] && fn.Pkg().Path() != p.kit {
			return objKey(fn)
		}
		return ""
	}
	what, _ := p.foreignOrigin(info, call.Fun)
	return what
}
