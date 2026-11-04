// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package main

// Allow-lists by type (loop ruling 12). The checker does not name the unsafe
// shapes of a sink; it names the few safe ones and fails every other
// expression of a sink's type, whatever shape carries it: a tuple, a
// conversion, an interface, a generic, a composite literal.

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"sort"
	"strings"
)

// sinkTypes: every expression of one of these types (or a pointer to one, or
// a slice, map, channel, function or tuple holding one) is checked against
// the allowed uses below.
var sinkTypes = set("os/exec.Cmd", "net/http.Client", "net/http.Transport")

// genericSinks: no generic function or type is instantiated with a type that
// holds one of these. The checker does not follow a sink through a type
// parameter, so the instantiation itself fails.
var genericSinks = set("os/exec.Cmd", "net/http.Client", "net/http.Transport", "net/http.RoundTripper", "os.File",
	"database/sql.DB", "database/sql.Conn", "database/sql/driver.Driver", "database/sql/driver.Connector", "log/slog.Handler", "log.Logger")

// allowedMethods: the only methods a value of each sink type may be the
// receiver of. A command runs only through the kit; a transport sends only
// inside credkit.Transport; a client was built with the kit's transport.
var allowedMethods = map[string]map[string]bool{
	"os/exec.Cmd":        set("String", "Environ", "Wait", "StdoutPipe", "StderrPipe"),
	"net/http.Client":    set("Do", "Get", "Head", "Post", "PostForm", "CloseIdleConnections"),
	"net/http.Transport": set("Clone", "CloseIdleConnections"),
}

func namedKey(t types.Type) string {
	n, ok := types.Unalias(t).(*types.Named)
	if !ok || n.Obj().Pkg() == nil {
		return ""
	}
	return n.Obj().Pkg().Path() + "." + n.Obj().Name()
}

// sinkOf is the sink type t is or points to, or "". An unnamed struct
// identical to http.Client's is one: a conversion can make a client of it.
func (p *pass) sinkOf(t types.Type) string {
	if pt, ok := types.Unalias(t).(*types.Pointer); ok {
		t = pt.Elem()
	}
	if k := namedKey(t); sinkTypes[k] {
		return k
	}
	if _, named := types.Unalias(t).(*types.Named); !named && p.isClient(t) {
		return "net/http.Client"
	}
	return ""
}

// holds is a type of keys that t is or holds in its unnamed structure
// (pointers, slices, arrays, maps, channels, function signatures, tuples and
// unnamed structs), or "". A named type counts only as itself.
func (p *pass) holds(t types.Type, keys map[string]bool, seen map[types.Type]bool) string {
	if t == nil || seen[t] {
		return ""
	}
	seen[t] = true
	if k := namedKey(t); k != "" {
		if keys[k] {
			return k
		}
		// A named struct is checked field by field where its fields are read;
		// any other named type is its underlying structure (class named-carriers).
		if _, isStruct := t.Underlying().(*types.Struct); isStruct || types.IsInterface(t) {
			return ""
		}
		return p.holds(t.Underlying(), keys, seen)
	}
	if p.isClient(t) && keys["net/http.Client"] {
		return "net/http.Client"
	}
	switch u := types.Unalias(t).(type) {
	case *types.Pointer:
		return p.holds(u.Elem(), keys, seen)
	case *types.Slice:
		return p.holds(u.Elem(), keys, seen)
	case *types.Array:
		return p.holds(u.Elem(), keys, seen)
	case *types.Map:
		if k := p.holds(u.Key(), keys, seen); k != "" {
			return k
		}
		return p.holds(u.Elem(), keys, seen)
	case *types.Chan:
		return p.holds(u.Elem(), keys, seen)
	case *types.Signature:
		if k := p.holds(u.Params(), keys, seen); k != "" {
			return k
		}
		return p.holds(u.Results(), keys, seen)
	case *types.Tuple:
		for i := 0; u != nil && i < u.Len(); i++ {
			if k := p.holds(u.At(i).Type(), keys, seen); k != "" {
				return k
			}
		}
	case *types.Struct:
		for i := 0; i < u.NumFields(); i++ {
			if k := p.holds(u.Field(i).Type(), keys, seen); k != "" {
				return k
			}
		}
	}
	return ""
}

func (p *pass) localFunc(fn *types.Func) bool { return fn.Pkg() != nil && p.local[fn.Pkg().Path()] }

// sinkExpr checks one expression whose type holds a sink: how it was made
// and where it goes must both be allowed uses.
func (p *pass) sinkExpr(info *types.Info, e ast.Expr, stack []ast.Node) {
	tv, ok := info.Types[e]
	if !ok || tv.IsType() || tv.IsBuiltin() || tv.Type == nil {
		return
	}
	if len(stack) > 0 {
		if s, ok := stack[len(stack)-1].(*ast.SelectorExpr); ok && s.Sel == e {
			return // the selector is the expression
		}
	}
	if s, ok := e.(*ast.SelectorExpr); ok {
		p.promoted(info, s, stack)
	}
	if callOf(e, stack) != nil {
		return // a function called: its arguments and result are the expressions checked
	}
	// Every kind the expression holds is checked (class sink-kinds): a
	// transport beside a client exempts nothing.
	for _, k := range sinkKinds {
		if p.holds(tv.Type, sinkKindSet[k], map[types.Type]bool{}) != "" && p.sinkKind(info, e, stack, k, tv.Type) {
			return
		}
	}
	// An interface a client or command satisfies by sending or running may
	// not come from another package (class sending-interfaces): the value in
	// it may be one the checker never saw built.
	if k := p.ifaceSink(tv.Type, map[types.Type]bool{}, false); k != "" {
		if what, why := p.foreignOrigin(info, e); why != "" {
			p.site(e, stack, 2, what, why+", whose type an "+k+" satisfies with a method that sends or runs: the value in it may be one the checker never saw built", "build the value in the connector, or route it through the kit")
		}
	}
	// The same interface reaching connector code as a parameter another
	// package fills: a callback or a method handed to it (class
	// inbound-interfaces).
	if dest := p.foreignDest(info, e, stack); dest != "" {
		if k := p.deepHolds(tv.Type, map[types.Type]bool{}); k != "" {
			p.site(e, stack, 2, "carrier:"+k, "a value holding a "+k+" handed to "+dest+", which can replace it (a decoder fills the struct)", "hand on only the fields that hold no command, client or transport")
		}
		if k := p.inbound(tv.Type, map[types.Type]bool{}); k != "" {
			p.site(e, stack, 2, "inbound:"+k, "a function or method handed to "+dest+" whose parameter is an interface a "+k+" satisfies with a method that sends or runs: that package fills it with a value the checker never saw built", "take the concrete value the kit routes, or keep the callback inside the connector")
		}
	}
}

// inbound: t is (or holds in pointers and containers) a function, or a type
// with a method, one of whose parameters carries an interface a client or
// command satisfies by sending or running; the name of that sink, or "".
func (p *pass) inbound(t types.Type, seen map[types.Type]bool) string {
	if t == nil || seen[t] {
		return ""
	}
	seen[t] = true
	// A function's parameters are filled by its caller; a function among its
	// results (a callback factory) is called by whoever called it, so its
	// parameters are filled there too.
	params := func(sig *types.Signature) string {
		if k := p.ifaceSink(sig.Params(), map[types.Type]bool{}, false); k != "" {
			return k
		}
		return p.inbound(sig.Results(), seen)
	}
	for _, mt := range []types.Type{t, types.NewPointer(t)} {
		ms := types.NewMethodSet(mt)
		for i := 0; i < ms.Len(); i++ {
			if k := params(ms.At(i).Obj().Type().(*types.Signature)); k != "" {
				return k
			}
		}
	}
	switch u := t.Underlying().(type) {
	case *types.Tuple:
		for i := 0; i < u.Len(); i++ {
			if k := p.inbound(u.At(i).Type(), seen); k != "" {
				return k
			}
		}
	case *types.Signature:
		return params(u)
	case *types.Pointer:
		return p.inbound(u.Elem(), seen)
	case *types.Slice:
		return p.inbound(u.Elem(), seen)
	case *types.Array:
		return p.inbound(u.Elem(), seen)
	case *types.Chan:
		return p.inbound(u.Elem(), seen)
	case *types.Map:
		return p.inbound(u.Elem(), seen)
	}
	return ""
}

// foreignDest names the other package's function, variable or field e goes
// straight into (an argument, an assignment, a composite-literal element), or "".
func (p *pass) foreignDest(info *types.Info, e ast.Expr, stack []ast.Node) string {
	// net/http's and os/exec's own fields are the sinks the allow-lists check.
	foreign := func(o types.Object) bool {
		return o != nil && o.Pkg() != nil && !p.local[o.Pkg().Path()] && o.Pkg().Path() != p.kit && !sinkPkg(o.Pkg().Path())
	}
	n, st := outerParen(e, stack)
	if len(st) == 0 {
		return ""
	}
	switch x := st[len(st)-1].(type) {
	case *ast.CallExpr:
		if x.Fun == n {
			return ""
		}
		return p.foreignCallee(info, x) // a function or a function value (class func-value-callees)
	case *ast.AssignStmt:
		for i, r := range x.Rhs {
			if r != n || len(x.Lhs) != len(x.Rhs) {
				continue
			}
			switch l := ast.Unparen(x.Lhs[i]).(type) {
			case *ast.Ident:
				if o := info.Uses[l]; foreign(o) {
					return objKey(o)
				}
			case *ast.SelectorExpr:
				if sel := info.Selections[l]; sel != nil && foreign(sel.Obj()) {
					return objKey(sel.Obj())
				} else if o := info.Uses[l.Sel]; sel == nil && foreign(o) {
					return objKey(o)
				}
			}
		}
	case *ast.KeyValueExpr:
		if x.Value == n && len(st) > 1 {
			if cl, ok := st[len(st)-2].(*ast.CompositeLit); ok {
				return p.foreignLit(info, cl)
			}
		}
	case *ast.CompositeLit:
		return p.foreignLit(info, x)
	}
	return ""
}

func (p *pass) foreignLit(info *types.Info, cl *ast.CompositeLit) string {
	t := info.TypeOf(cl)
	if pt, ok := t.(*types.Pointer); ok {
		t = pt.Elem()
	}
	if n, ok := types.Unalias(t).(*types.Named); ok && n.Obj().Pkg() != nil && !p.local[n.Obj().Pkg().Path()] && n.Obj().Pkg().Path() != p.kit && !sinkPkg(n.Obj().Pkg().Path()) {
		return types.TypeString(n, nil)
	}
	return ""
}

// promoted: a selection that reaches its field or method through embedded
// fields reads each of them; one of another package's types that holds a
// sink is a sink of unknown origin (class promoted-methods).
func (p *pass) promoted(info *types.Info, s *ast.SelectorExpr, stack []ast.Node) {
	sel := info.Selections[s]
	if sel == nil || len(sel.Index()) < 2 {
		return
	}
	t := sel.Recv()
	for _, i := range sel.Index()[:len(sel.Index())-1] {
		if pt, ok := t.Underlying().(*types.Pointer); ok {
			t = pt.Elem()
		}
		st, ok := t.Underlying().(*types.Struct)
		if !ok {
			return
		}
		f := st.Field(i)
		if k := p.deepHolds(f.Type(), map[types.Type]bool{}); k != "" && f.Pkg() != nil && !p.local[f.Pkg().Path()] && f.Pkg().Path() != "net/http" && f.Pkg().Path() != "os/exec" {
			p.site(s, stack, 2, objKey(f), fmt.Sprintf("a %s held in an embedded field of another package's type (%s), reached by promotion", k, objKey(f)), "build the client or command in the connector")
			return
		}
		t = f.Type()
	}
}

// foreignOrigin: e is made by another package (its function, method,
// variable or field, or a call of one), or "".
func (p *pass) foreignOrigin(info *types.Info, e ast.Expr) (string, string) {
	foreign := func(o types.Object) bool {
		return o != nil && o.Pkg() != nil && !p.local[o.Pkg().Path()] && o.Pkg().Path() != p.kit
	}
	switch x := ast.Unparen(e).(type) {
	case *ast.Ident:
		if o := info.Uses[x]; foreign(o) && (isFunc(o) || o.Parent() == o.Pkg().Scope()) {
			return objKey(o), "a value another package holds (" + objKey(o) + ")"
		}
	case *ast.SelectorExpr:
		if sel := info.Selections[x]; sel != nil {
			if foreign(sel.Obj()) {
				return objKey(sel.Obj()), "a value of another package's field or method (" + objKey(sel.Obj()) + ")"
			}
		} else if o := info.Uses[x.Sel]; foreign(o) {
			return objKey(o), "a value another package holds (" + objKey(o) + ")"
		}
	case *ast.CallExpr:
		if tv := info.Types[x.Fun]; tv.IsType() || tv.IsBuiltin() {
			return "", ""
		}
		if fn, ok := calleeObj(info, x.Fun).(*types.Func); ok {
			if foreign(fn) {
				return objKey(fn), "a value another package returns (" + objKey(fn) + ")"
			}
			return "", ""
		}
		return p.foreignOrigin(info, x.Fun)
	}
	return "", ""
}

// sinkKinds: sinkTypes in a fixed order, so a finding is the same each run.
var sinkKinds = []string{"os/exec.Cmd", "net/http.Client", "net/http.Transport"}

var sinkKindSet = map[string]map[string]bool{"os/exec.Cmd": set("os/exec.Cmd"), "net/http.Client": set("net/http.Client"), "net/http.Transport": set("net/http.Transport")}

// sinkKind checks e for one sink kind it holds and reports whether it found
// something.
func (p *pass) sinkKind(info *types.Info, e ast.Expr, stack []ast.Node, k string, t types.Type) bool {
	fix := "build a command with exec.Command and run it with credkit.StartCommand or credkit.RunCommand; build a client as &http.Client{Transport: credkit.Transport(...)}; keep the value its own type, never an interface, a conversion or a type argument"
	// A transport's origin never matters: it sends only inside credkit.Transport,
	// which inspects it at run time, so only where it goes is checked.
	if k != "net/http.Transport" {
		if what, why := p.produced(info, e, k); why != "" {
			p.site(e, stack, 2, what, why, fix)
			return true
		}
	}
	if what, why := p.goesTo(info, e, stack, k, t); why != "" {
		p.site(nodeFor(e, stack, what), stack, 2, what, why, fix)
		return true
	}
	return false
}

// nodeFor: a method finding is positioned at its selector, as the ident rule
// positions it, so the two report one site.
func nodeFor(e ast.Expr, stack []ast.Node, what string) ast.Node {
	if !strings.HasPrefix(what, "sink:") && len(stack) > 0 {
		return stack[len(stack)-1]
	}
	return e
}

// produced reports why e is not an allowed way to make a value holding the
// sink k, or "".
func (p *pass) produced(info *types.Info, e ast.Expr, k string) (string, string) {
	sk := "sink:" + k
	switch x := e.(type) {
	case *ast.Ident:
		switch o := info.Uses[x].(type) {
		case *types.Func:
			if !p.localFunc(o) {
				return objKey(o), fmt.Sprintf("%s, a function of another package, used as a value that takes or returns a %s", objKey(o), k)
			}
		case *types.Var:
			if p.implicit[o] {
				return sk, "a " + k + " recovered from an interface by a type switch, where the checker cannot see how it was built"
			}
			if !o.IsField() && o.Pkg() != nil && o.Parent() == o.Pkg().Scope() && !p.local[o.Pkg().Path()] {
				return objKey(o), fmt.Sprintf("a %s another package holds (%s)", k, objKey(o))
			}
		}
		return "", ""
	case *ast.SelectorExpr:
		sel := info.Selections[x]
		if sel == nil { // a qualified identifier
			switch o := info.Uses[x.Sel].(type) {
			case *types.Func:
				if !p.localFunc(o) {
					return objKey(o), fmt.Sprintf("%s, a function of another package, used as a value that takes or returns a %s", objKey(o), k)
				}
			case *types.Var:
				if !p.local[o.Pkg().Path()] {
					return objKey(o), fmt.Sprintf("a %s another package holds (%s)", k, objKey(o))
				}
			}
			return "", ""
		}
		switch sel.Kind() {
		case types.FieldVal:
			if f := sel.Obj(); f.Pkg() != nil && !p.local[f.Pkg().Path()] && f.Pkg().Path() != "net/http" && f.Pkg().Path() != "os/exec" {
				return objKey(f), fmt.Sprintf("a %s held in a field of another package's type (%s)", k, objKey(f))
			}
		case types.MethodVal, types.MethodExpr:
			if fn, ok := sel.Obj().(*types.Func); ok && !p.localFunc(fn) {
				return objKey(fn), fmt.Sprintf("%s, a method of another package, used as a value that takes or returns a %s", objKey(fn), k)
			}
		}
		return "", ""
	case *ast.CallExpr:
		ftv := info.Types[x.Fun]
		if ftv.IsType() {
			return sk, "a conversion makes this " + k + " out of another value, so the checker cannot see how it was built"
		}
		if ftv.IsBuiltin() {
			switch builtinName(info, x.Fun) {
			case "append", "make":
				return "", ""
			case "new":
				if k == "net/http.Transport" {
					return "", ""
				}
				return sk, "new makes a zero " + k + ", which the kit never saw built"
			}
			return "", ""
		}
		if fn, ok := calleeObj(info, x.Fun).(*types.Func); ok {
			switch objKey(fn) {
			case "os/exec.Command", "os/exec.CommandContext", "net/http.Transport.Clone":
				return "", ""
			}
			if !p.localFunc(fn) {
				return objKey(fn), fmt.Sprintf("a %s another package builds (%s)", k, objKey(fn))
			}
			if res := fn.Origin().Signature().Results(); !sameResult(res, info.TypeOf(x)) {
				return sk, "a " + k + " returned through a type parameter of " + fn.Name() + ", which the checker does not follow"
			}
			return "", ""
		}
		// A call of a function value: the value is checked as the producer
		// (class func-value-producers: another package's function variable).
		return p.produced(info, x.Fun, k)
	case *ast.CompositeLit:
		if k == "os/exec.Cmd" && p.sinkOf(info.TypeOf(x)) == k {
			return sk, "an exec.Cmd literal: a command may only be built by exec.Command or exec.CommandContext"
		}
		return "", ""
	case *ast.UnaryExpr, *ast.StarExpr, *ast.IndexExpr, *ast.IndexListExpr, *ast.SliceExpr, *ast.ParenExpr, *ast.FuncLit:
		return "", ""
	case *ast.TypeAssertExpr:
		return sk, "a type assertion recovers this " + k + " from an interface, where the checker cannot see how it was built or what ran it"
	}
	return sk, "an expression the checker does not know as a way to make a " + k
}

// isClientTransport reports whether e is a client's Transport field.
func isClientTransport(info *types.Info, e ast.Expr) bool {
	s, ok := ast.Unparen(e).(*ast.SelectorExpr)
	if !ok {
		return false
	}
	v, ok := info.Uses[s.Sel].(*types.Var)
	return ok && v.IsField() && v.Pkg() != nil && v.Pkg().Path() == "net/http" && v.Name() == "Transport"
}

func sameResult(res *types.Tuple, t types.Type) bool {
	if res.Len() == 1 {
		return types.Identical(res.At(0).Type(), t)
	}
	return types.Identical(res, t)
}

func builtinName(info *types.Info, fun ast.Expr) string {
	if id, ok := ast.Unparen(fun).(*ast.Ident); ok {
		if b, ok := info.Uses[id].(*types.Builtin); ok {
			return b.Name()
		}
	}
	return ""
}

func blank(e ast.Expr) bool { id, ok := e.(*ast.Ident); return ok && id.Name == "_" }

// goesTo reports where e goes when that is not an allowed use: the finding's
// "what" and why, or "" for an allowed use. A value holding a sink may only
// move to a place of its own type (never an interface, a type parameter or
// another type), be compared, be read through, or be handed to the kit.
func (p *pass) goesTo(info *types.Info, e ast.Expr, stack []ast.Node, k string, t types.Type) (string, string) {
	if len(stack) == 0 {
		return "", ""
	}
	what := "sink:" + k
	same := func(dst types.Type) bool { return dst != nil && types.Identical(dst, t) }
	moved := func(dst types.Type) (string, string) {
		if same(dst) {
			return "", ""
		}
		return what, fmt.Sprintf("a %s becomes a %s here, where the checker cannot see what uses it", types.TypeString(t, nil), typeName(dst))
	}
	// tuple: every element that holds a sink lands in a place of its own type.
	tuple := func(dsts []types.Type) (string, string) {
		tu, ok := t.(*types.Tuple)
		if !ok || tu.Len() != len(dsts) {
			return what, "a multi-value " + k + " the checker cannot match to its destinations"
		}
		for i := 0; i < tu.Len(); i++ {
			et := tu.At(i).Type()
			if p.holds(et, sinkTypes, map[types.Type]bool{}) == "" || dsts[i] == nil {
				continue
			}
			if !types.Identical(et, dsts[i]) {
				return what, fmt.Sprintf("a %s becomes a %s here, where the checker cannot see what uses it", types.TypeString(et, nil), typeName(dsts[i]))
			}
		}
		return "", ""
	}
	switch x := stack[len(stack)-1].(type) {
	case *ast.ParenExpr, *ast.ExprStmt, *ast.StarExpr, *ast.RangeStmt:
		return "", ""
	case *ast.UnaryExpr:
		if x.Op == token.AND || x.Op == token.ARROW {
			return "", ""
		}
	case *ast.BinaryExpr:
		if x.Op == token.EQL || x.Op == token.NEQ {
			return "", ""
		}
	case *ast.SliceExpr:
		if x.X == e {
			return "", ""
		}
	case *ast.IndexExpr:
		if x.X == e {
			return "", ""
		}
		if m, ok := types.Unalias(info.TypeOf(x.X)).Underlying().(*types.Map); ok {
			return moved(m.Key())
		}
	case *ast.IndexListExpr:
		if x.X == e {
			return "", ""
		}
	case *ast.SelectorExpr:
		sel := info.Selections[x]
		if sel == nil || sel.Kind() == types.FieldVal {
			return "", ""
		}
		if sel.Kind() == types.MethodVal {
			sk := p.sinkOf(t)
			if sk != "" && allowedMethods[sk][sel.Obj().Name()] {
				return "", ""
			}
			return objKey(sel.Obj()), fmt.Sprintf("%s is not a method the kit allows on a %s (allowed: %s)", sel.Obj().Name(), k, methodList(sk))
		}
	case *ast.CallExpr:
		if x.Fun == e {
			return "", ""
		}
		ftv := info.Types[x.Fun]
		if ftv.IsType() {
			return what, "a " + k + " converted to " + typeName(ftv.Type) + ", where the checker cannot see what uses it"
		}
		if ftv.IsBuiltin() {
			switch builtinName(info, x.Fun) {
			case "len", "cap", "copy", "delete", "close":
				return "", ""
			case "append":
				if len(x.Args) > 0 && x.Args[0] == e {
					return "", ""
				}
				if sl, ok := types.Unalias(info.TypeOf(x)).Underlying().(*types.Slice); ok {
					if x.Ellipsis.IsValid() {
						return moved(sl)
					}
					return moved(sl.Elem())
				}
			}
			return what, "a " + k + " handed to a builtin that may print or keep it"
		}
		obj := calleeObj(info, x.Fun)
		if (k == "os/exec.Cmd" && p.isKit(obj, "StartCommand", "RunCommand")) || (k == "net/http.Transport" && p.isKit(obj, "Transport")) {
			if p.sinkOf(t) == k {
				return "", ""
			}
		}
		var sig *types.Signature
		if fn, ok := obj.(*types.Func); ok {
			if !p.localFunc(fn) {
				return what, fmt.Sprintf("a %s handed to %s, which the checker cannot see into", k, objKey(fn))
			}
			if r := fn.Signature().Recv(); r != nil && types.IsInterface(r.Type()) {
				return what, "a " + k + " handed to a method called through an interface, where the checker cannot see which method runs"
			}
			sig = fn.Origin().Signature()
		} else {
			sig, _ = types.Unalias(ftv.Type).Underlying().(*types.Signature)
		}
		if sig == nil {
			return what, "a " + k + " handed to a call the checker cannot type"
		}
		ps := sig.Params()
		param := func(i int) types.Type {
			switch {
			case sig.Variadic() && i >= ps.Len()-1:
				sl := ps.At(ps.Len() - 1).Type()
				if x.Ellipsis.IsValid() {
					return sl
				}
				if s, ok := sl.Underlying().(*types.Slice); ok {
					return s.Elem()
				}
				return nil
			case i < ps.Len():
				return ps.At(i).Type()
			}
			return nil
		}
		if _, ok := t.(*types.Tuple); ok && len(x.Args) == 1 {
			var dsts []types.Type
			for i := 0; i < t.(*types.Tuple).Len(); i++ {
				dsts = append(dsts, param(i))
			}
			return tuple(dsts)
		}
		for i, a := range x.Args {
			if a == e {
				return moved(param(i))
			}
		}
	case *ast.AssignStmt:
		for _, l := range x.Lhs {
			if l == e {
				return "", ""
			}
		}
		dst := func(l ast.Expr) types.Type {
			if blank(l) {
				return nil
			}
			return info.TypeOf(l)
		}
		if len(x.Lhs) == len(x.Rhs) {
			for i, r := range x.Rhs {
				if r == e {
					if blank(x.Lhs[i]) || isClientTransport(info, x.Lhs[i]) {
						return "", "" // clientTransportField checks a client's transport
					}
					return moved(dst(x.Lhs[i]))
				}
			}
		}
		var dsts []types.Type
		for _, l := range x.Lhs {
			dsts = append(dsts, dst(l))
		}
		return tuple(dsts)
	case *ast.ValueSpec:
		if x.Type == nil {
			return "", ""
		}
		dt := info.TypeOf(x.Type)
		if len(x.Values) == len(x.Names) {
			return moved(dt)
		}
		var dsts []types.Type
		for _, n := range x.Names {
			if n.Name == "_" {
				dsts = append(dsts, nil)
			} else {
				dsts = append(dsts, dt)
			}
		}
		return tuple(dsts)
	case *ast.ReturnStmt:
		sig := enclosingSig(info, stack[:len(stack)-1])
		if sig == nil {
			return what, "a " + k + " returned where the checker cannot type the result"
		}
		res := sig.Results()
		if len(x.Results) == 1 && res.Len() > 1 {
			var dsts []types.Type
			for i := 0; i < res.Len(); i++ {
				dsts = append(dsts, res.At(i).Type())
			}
			return tuple(dsts)
		}
		for i, r := range x.Results {
			if r == e && i < res.Len() {
				return moved(res.At(i).Type())
			}
		}
	case *ast.CompositeLit:
		for i, el := range x.Elts {
			if el == e {
				return moved(elemType(info.TypeOf(x), i, nil))
			}
		}
	case *ast.KeyValueExpr:
		if len(stack) > 1 {
			if lit, ok := stack[len(stack)-2].(*ast.CompositeLit); ok {
				if id, ok := x.Key.(*ast.Ident); ok && id.Name == "Transport" && p.isClient(info.TypeOf(lit)) {
					return "", "" // compositeLit checks a client literal's transport
				}
				if x.Value == e {
					return moved(elemType(info.TypeOf(lit), -1, x.Key))
				}
				if m, ok := types.Unalias(info.TypeOf(lit)).Underlying().(*types.Map); ok && x.Key == e {
					return moved(m.Key())
				}
			}
		}
	case *ast.SendStmt:
		if x.Chan == e {
			return "", ""
		}
		if ch, ok := types.Unalias(info.TypeOf(x.Chan)).Underlying().(*types.Chan); ok {
			return moved(ch.Elem())
		}
	}
	return what, "a " + k + " used where the checker allows none (allowed: assigned, passed or returned as its own type, compared, read through, or handed to the kit)"
}

// elemType is the type a composite literal's element i (or keyed by key) takes.
func elemType(lt types.Type, i int, key ast.Expr) types.Type {
	if lt == nil {
		return nil
	}
	switch u := types.Unalias(lt).Underlying().(type) {
	case *types.Struct:
		if key != nil {
			if id, ok := key.(*ast.Ident); ok {
				for j := 0; j < u.NumFields(); j++ {
					if u.Field(j).Name() == id.Name {
						return u.Field(j).Type()
					}
				}
			}
			return nil
		}
		if i >= 0 && i < u.NumFields() {
			return u.Field(i).Type()
		}
	case *types.Slice:
		return u.Elem()
	case *types.Array:
		return u.Elem()
	case *types.Map:
		return u.Elem()
	}
	return nil
}

func typeName(t types.Type) string {
	if t == nil {
		return "a value of another type"
	}
	return types.TypeString(t, nil)
}

func methodList(k string) string {
	var out []string
	for m := range allowedMethods[k] {
		out = append(out, m)
	}
	if len(out) == 0 {
		return "none"
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}

// ---- Sources (rule 4): via is a pure function of the command line. ----

// pureStd: the functions of other packages a path function may call. Each
// returns the same value for the same arguments and environment.
var pureStd = set("path/filepath.Join", "path/filepath.Clean", "path/filepath.Dir", "path/filepath.Base", "path/filepath.Ext",
	"path/filepath.IsAbs", "path/filepath.Split", "path/filepath.ToSlash", "path/filepath.FromSlash", "path/filepath.VolumeName",
	"path/filepath.Rel", "path/filepath.IsLocal", "path/filepath.SplitList",
	"path.Join", "path.Clean", "path.Dir", "path.Base", "path.Ext", "path.IsAbs", "path.Split")

// envStd: pure path functions that read the environment.
var envStd = set("os.Getenv", "os.LookupEnv", "os.UserConfigDir", "os.UserHomeDir", "os.TempDir")

func (p *pass) pureCallee(fn *types.Func) bool {
	k := objKey(fn)
	if pureStd[k] || envStd[k] {
		return true
	}
	// The kit's documented way to read --config: it only reads its arguments.
	if k == p.kit+".FlagValues" {
		return true
	}
	if fn.Signature().Recv() == nil && fn.Pkg() != nil {
		switch fn.Pkg().Path() {
		case "strings", "strconv", "unicode", "unicode/utf8":
			return true
		}
	}
	return false
}

// purity proves a function returns the same path for the same command line:
// it reads no package-level variable, writes only its own locals, never takes
// an address, and calls only builtins, conversions, the pure functions above
// and connector functions that are pure the same way. A restricted parameter
// (the command line) is only read: by len, range, an index, or as an
// argument of a pure function.
type purity struct {
	p    *pass
	seen map[proof]bool
	env  bool // the closure reads the environment
}

// proof: a function proved pure under one set of restricted parameters
// (class purity-memo: a proof without the restriction is not one with it).
type proof struct {
	fn         *types.Func
	restricted string
}

func proofOf(fn *types.Func, restricted map[int]bool) proof {
	return proof{fn.Origin(), fmt.Sprint(restricted)} // fmt prints a map in key order
}

func (q *purity) fn(fn *types.Func, restricted map[int]bool) (ast.Node, string) {
	fn = fn.Origin()
	if q.seen[proofOf(fn, restricted)] {
		return nil, ""
	}
	q.seen[proofOf(fn, restricted)] = true
	d, ok := q.p.funcDecl[fn].(*ast.FuncDecl)
	if !ok || d.Body == nil {
		return nil, "has no body the checker can read"
	}
	info := q.p.infos[fn.Pkg()]
	params := map[types.Object]bool{}
	i := 0
	for _, f := range d.Type.Params.List {
		for _, n := range f.Names {
			if restricted[i] {
				params[info.Defs[n]] = true
			}
			i++
		}
		if len(f.Names) == 0 {
			i++
		}
	}
	if d.Recv != nil {
		return d, "is a method; a path function is a plain function"
	}
	return q.body(info, d.Body, params)
}

func (q *purity) body(info *types.Info, body ast.Node, params map[types.Object]bool) (bad ast.Node, why string) {
	local := func(e ast.Expr) bool {
		id, ok := ast.Unparen(e).(*ast.Ident)
		if !ok {
			return false
		}
		if id.Name == "_" {
			return true
		}
		o := info.ObjectOf(id)
		v, ok := o.(*types.Var)
		return ok && !v.IsField() && v.Pkg() != nil && v.Parent() != v.Pkg().Scope() && !params[o]
	}
	ast.PreorderStack(body, nil, func(n ast.Node, stack []ast.Node) bool {
		if bad != nil {
			return false
		}
		switch x := n.(type) {
		case *ast.Ident:
			o := info.Uses[x]
			if v, ok := o.(*types.Var); ok && !v.IsField() && v.Pkg() != nil && v.Parent() == v.Pkg().Scope() {
				bad, why = x, "reads the package-level variable "+objKey(v)+", which can change between Start and the read"
			} else if f, ok := o.(*types.Func); ok && !calledAt(x, stack) && (!q.p.pureCallee(f) || envStd[objKey(f)]) {
				bad, why = x, "uses a function as a value ("+objKey(f)+"), whose body the checker does not prove"
			} else if params[o] && !q.readOnly(info, x, stack) {
				bad, why = x, "uses its command-line parameter other than to read it (len, range, an index, or an argument of a pure function)"
			}
		case *ast.CallExpr:
			if w := q.call(info, x, params); w != "" {
				bad, why = x, w
			}
		case *ast.AssignStmt:
			if x.Tok != token.DEFINE {
				for _, l := range x.Lhs {
					if !local(l) {
						bad, why = l, "writes something other than its own local variable"
					}
				}
			}
		case *ast.IncDecStmt:
			if !local(x.X) {
				bad, why = x, "writes something other than its own local variable"
			}
		case *ast.UnaryExpr:
			if x.Op == token.AND || x.Op == token.ARROW {
				bad, why = x, "takes an address or receives from a channel"
			}
		case *ast.GoStmt, *ast.DeferStmt, *ast.SendStmt, *ast.SelectStmt:
			bad, why = x, "starts, defers or sends"
		case *ast.RangeStmt:
			if _, isMap := types.Unalias(info.TypeOf(x.X)).Underlying().(*types.Map); isMap {
				bad, why = x, "ranges over a map, whose order changes from run to run"
			}
		}
		return bad == nil
	})
	return bad, why
}

// readOnly: a use of a restricted parameter that cannot change it.
func (q *purity) readOnly(info *types.Info, id *ast.Ident, stack []ast.Node) bool {
	n, stack := outerParen(id, stack)
	if len(stack) == 0 {
		return false
	}
	switch x := stack[len(stack)-1].(type) {
	case *ast.RangeStmt:
		return x.X == n
	case *ast.IndexExpr:
		return x.X == n && indexRead(x, stack[:len(stack)-1])
	case *ast.CallExpr:
		if x.Fun == n {
			return false
		}
		switch builtinName(info, x.Fun) {
		case "len", "cap":
			return true
		case "append":
			return len(x.Args) > 0 && x.Args[0] != n
		case "":
		default:
			return false
		}
		if fn, ok := calleeObj(info, x.Fun).(*types.Func); ok {
			return q.p.localFunc(fn) || q.p.pureCallee(fn)
		}
	}
	return false
}

func (q *purity) call(info *types.Info, c *ast.CallExpr, params map[types.Object]bool) string {
	ftv := info.Types[c.Fun]
	if ftv.IsType() {
		return ""
	}
	if ftv.IsBuiltin() {
		switch n := builtinName(info, c.Fun); n {
		case "len", "cap", "make", "append", "min", "max", "panic":
			return ""
		default:
			return "calls the builtin " + n
		}
	}
	fn, ok := calleeObj(info, c.Fun).(*types.Func)
	if !ok {
		return "calls a function value, which the checker cannot see"
	}
	if q.p.localFunc(fn) {
		restricted := map[int]bool{}
		for i, a := range c.Args {
			if id, ok := ast.Unparen(a).(*ast.Ident); ok && params[info.Uses[id]] {
				restricted[i] = true
			}
		}
		if n, w := q.fn(fn, restricted); w != "" {
			at := ""
			if n != nil {
				pos := q.p.fset.Position(n.Pos())
				at = fmt.Sprintf(" (%s:%d)", relTo(q.p.cli, pos.Filename), pos.Line)
			}
			return "calls " + objKey(fn) + ", which " + w + at
		}
		return ""
	}
	if q.p.pureCallee(fn) {
		if envStd[objKey(fn)] {
			q.env = true
		}
		return ""
	}
	return "calls " + objKey(fn) + ", which is not one of the pure functions a path function may call"
}

// viaShape: a path function is func() string, or func([]string) string given
// the command line.
func viaShape(fn *types.Func) (argsForm, ok bool) {
	sig := fn.Signature()
	if sig.TypeParams() != nil || sig.Recv() != nil || sig.Results().Len() != 1 || !types.Identical(sig.Results().At(0).Type(), types.Typ[types.String]) {
		return false, false
	}
	switch sig.Params().Len() {
	case 0:
		return false, true
	case 1:
		sl, isSlice := sig.Params().At(0).Type().(*types.Slice)
		return true, isSlice && !sig.Variadic() && types.Identical(sl.Elem(), types.Typ[types.String])
	}
	return false, false
}

// isOSArgs1 reports whether e is exactly os.Args[1:], the command line Start
// hands Files.
func isOSArgs1(info *types.Info, e ast.Expr) bool {
	s, ok := ast.Unparen(e).(*ast.SliceExpr)
	if !ok || s.High != nil || s.Max != nil || s.Low == nil || !isOSArgs(info, s.X) {
		return false
	}
	tv := info.Types[s.Low]
	if tv.Value == nil {
		return false
	}
	v, exact := constant.Int64Val(tv.Value)
	return exact && v == 1
}

func isOSArgs(info *types.Info, e ast.Expr) bool {
	var id *ast.Ident
	switch x := ast.Unparen(e).(type) {
	case *ast.Ident:
		id = x
	case *ast.SelectorExpr:
		id = x.Sel
	}
	return id != nil && objKey(orNil(info.Uses[id])) == "os.Args"
}

// viaCall reports whether c calls via the way the kit calls Files: with no
// argument, or with the command line (param in Files, os.Args[1:] at a read).
func viaCall(info *types.Info, c *ast.CallExpr, via *types.Func, argsForm bool, param types.Object) bool {
	if fn, ok := calleeObj(info, c.Fun).(*types.Func); !ok || fn.Origin() != via {
		return false
	}
	if !argsForm {
		return len(c.Args) == 0
	}
	if len(c.Args) != 1 || c.Ellipsis.IsValid() {
		return false
	}
	if param != nil {
		id, ok := ast.Unparen(c.Args[0]).(*ast.Ident)
		return ok && info.Uses[id] == param
	}
	return isOSArgs1(info, c.Args[0])
}

// filesFunc is the body, info and command-line parameter of Files.
func (p *pass) filesFunc(d *decls) (*ast.BlockStmt, *types.Info, types.Object, *types.Func) {
	if d.filesExpr == nil {
		return nil, nil, nil, nil
	}
	switch f := ast.Unparen(d.filesExpr).(type) {
	case *ast.FuncLit:
		var param types.Object
		if ps := f.Type.Params.List; len(ps) > 0 && len(ps[0].Names) > 0 {
			param = d.filesInfo.Defs[ps[0].Names[0]]
		}
		return f.Body, d.filesInfo, param, nil
	default:
		fn, ok := calleeObj(d.filesInfo, f).(*types.Func)
		if !ok {
			return nil, nil, nil, nil
		}
		fd, ok := p.funcDecl[fn.Origin()].(*ast.FuncDecl)
		if !ok || fd.Body == nil {
			return nil, nil, nil, nil
		}
		info := p.infos[fn.Pkg()]
		var param types.Object
		if ps := fd.Type.Params.List; len(ps) > 0 && len(ps[0].Names) > 0 {
			param = info.Defs[ps[0].Names[0]]
		}
		return fd.Body, info, param, fn
	}
}

// filesReturns reports why Files does not return via's path on every return,
// or "".
func (p *pass) filesReturns(d *decls, via *types.Func, argsForm bool) string {
	body, info, param, _ := p.filesFunc(d)
	if body == nil {
		return "Declarations.Files is not a function literal or a connector function the checker can read"
	}
	n, bad := 0, ""
	ast.Inspect(body, func(x ast.Node) bool {
		if _, ok := x.(*ast.FuncLit); ok {
			return false
		}
		r, ok := x.(*ast.ReturnStmt)
		if !ok || bad != "" {
			return bad == ""
		}
		n++
		found := false
		if len(r.Results) == 1 {
			if cl, ok := ast.Unparen(r.Results[0]).(*ast.CompositeLit); ok {
				for _, el := range cl.Elts {
					if c, ok := ast.Unparen(el).(*ast.CallExpr); ok && viaCall(info, c, via, argsForm, param) {
						found = true
					}
				}
			}
		}
		if !found {
			pos := p.fset.Position(r.Pos())
			bad = fmt.Sprintf("Declarations.Files returns at %s:%d without %s in a []string literal there", relTo(p.cli, pos.Filename), pos.Line, viaSpelling(via, argsForm))
		}
		return true
	})
	if bad == "" && n == 0 {
		bad = "Declarations.Files has no return statement the checker can read"
	}
	return bad
}

func viaSpelling(via *types.Func, argsForm bool) string {
	if argsForm {
		return via.Name() + "(args)"
	}
	return via.Name() + "()"
}

// outerParen: e with every parenthesis around it, and the stack above them
// (class argv-writes: (os.Args[1]) = x is a write).
func outerParen(e ast.Node, stack []ast.Node) (ast.Node, []ast.Node) {
	for len(stack) > 0 {
		pe, ok := stack[len(stack)-1].(*ast.ParenExpr)
		if !ok {
			break
		}
		e, stack = pe, stack[:len(stack)-1]
	}
	return e, stack
}

// indexRead: the index expression x (its ancestors in stack) only reads.
func indexRead(x ast.Expr, stack []ast.Node) bool {
	n, stack := outerParen(x, stack)
	if len(stack) == 0 {
		return true
	}
	switch g := stack[len(stack)-1].(type) {
	case *ast.AssignStmt:
		for _, l := range g.Lhs {
			if l == n {
				return false
			}
		}
	case *ast.RangeStmt:
		return g.Key != n && g.Value != n
	case *ast.UnaryExpr:
		return g.Op != token.AND
	case *ast.IncDecStmt:
		return false
	}
	return true
}

// calledAt: the function named by id is called there, not used as a value.
func calledAt(id *ast.Ident, stack []ast.Node) bool {
	var n ast.Node = id
	if len(stack) > 0 {
		if s, ok := stack[len(stack)-1].(*ast.SelectorExpr); ok && s.Sel == id {
			n, stack = s, stack[:len(stack)-1]
		}
	}
	n, stack = outerParen(n, stack)
	if len(stack) == 0 {
		return false
	}
	c, ok := stack[len(stack)-1].(*ast.CallExpr)
	return ok && c.Fun == n
}

func sinkPkg(path string) bool { return path == "net/http" || path == "os/exec" }
