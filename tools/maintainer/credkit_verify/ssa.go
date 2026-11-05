// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package main

// The flow analysis over SSA (loop ruling 13). The AST walk models a Go
// construct by listing it, and every list left the next construct out (a
// conversion, a range clause, a type parameter, a decode site). SSA lowers
// every construct to a closed set of instruction kinds: a conversion is a
// ChangeType, Convert or MakeInterface, a type parameter is gone (each
// instantiation has its own body), a range clause is Next or Range plus
// Store. Each kind is modeled below or fails; the default fails any
// instruction that touches a value holding a sink. TestSSAKindsCovered lists
// every kind in the pinned golang.org/x/tools/go/ssa and fails when the
// switch lacks one, so an x/tools bump that adds a kind fails closed.
//
// The AST walk stays as a second check: its findings and these are unioned,
// so it never accepts what this rejects.

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/callgraph/cha"
	"golang.org/x/tools/go/callgraph/vta"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

// ssaSinks: a value whose type reaches one of these (ssaHolds) is a sink
// value, checked against the allowed uses at every instruction.
var ssaSinks = set("os/exec.Cmd", "net/http.Client", "net/http.Transport", "net/http.RoundTripper")

type declRange struct {
	pos, end token.Pos
	name     string
}

type ssaPass struct {
	p     *pass
	prog  *ssa.Program
	memo  map[types.Type]bool // ssaHolds results that are final
	funs  map[*ssa.Function]bool
	decls map[*token.File][]declRange
	calls map[token.Pos]token.Pos // a call's Lparen: its function expression's position, as the AST rules place a call
	cg    *callgraph.Graph        // static callees plus VTA for dynamic calls
	out   []Finding
	flds  *fieldIndex // built on first use
	escT  types.Type  // the concrete type escapes follows, as an interface value
	taken *takenIndex // built on first use
}

// takenIndex: functions whose value goes somewhere other than a direct call
// (another package may call them), and methods an interface or another
// package's generic can reach. Their call-graph callers are not all callers.
type takenIndex struct {
	fns  map[*ssa.Function]bool
	meth map[*types.Func]bool
}

func (s *ssaPass) takenFns() *takenIndex {
	if s.taken != nil {
		return s.taken
	}
	ix := &takenIndex{fns: map[*ssa.Function]bool{}, meth: map[*types.Func]bool{}}
	// methods marks the methods of t and of every type inside it (elements,
	// fields, type arguments, signatures): code that holds a value of t
	// reaches them all, by reflection if nothing else.
	seenM := map[types.Type]bool{}
	var methods func(t types.Type)
	methods = func(t types.Type) {
		if t == nil || seenM[t] {
			return
		}
		seenM[t] = true
		for _, t := range []types.Type{t, types.NewPointer(t)} {
			ms := s.prog.MethodSets.MethodSet(t)
			for i := 0; i < ms.Len(); i++ {
				if f, ok := ms.At(i).Obj().(*types.Func); ok {
					ix.meth[f] = true
				}
			}
			if _, ok := t.Underlying().(*types.Pointer); ok {
				break // no pointer to a pointer's methods
			}
		}
		if n, ok := types.Unalias(t).(*types.Named); ok {
			for i := 0; i < n.TypeArgs().Len(); i++ {
				methods(n.TypeArgs().At(i))
			}
		}
		switch u := t.Underlying().(type) {
		case *types.Pointer:
			methods(u.Elem())
		case *types.Slice:
			methods(u.Elem())
		case *types.Array:
			methods(u.Elem())
		case *types.Chan:
			methods(u.Elem())
		case *types.Map:
			methods(u.Key())
			methods(u.Elem())
		case *types.Struct:
			for i := 0; i < u.NumFields(); i++ {
				methods(u.Field(i).Type())
			}
		case *types.Signature:
			for _, tu := range []*types.Tuple{u.Params(), u.Results()} {
				for i := 0; i < tu.Len(); i++ {
					methods(tu.At(i).Type())
				}
			}
		}
	}
	// foreign: another package names a connector type only as a type
	// argument, so every type argument of another package's generic type
	// in t has its methods marked.
	seenT := map[types.Type]bool{}
	var foreign func(t types.Type)
	targs := func(ts []types.Type) {
		for _, t := range ts {
			methods(t)
			foreign(t)
		}
	}
	foreign = func(t types.Type) {
		if t == nil || seenT[t] {
			return
		}
		seenT[t] = true
		switch u := types.Unalias(t).(type) {
		case *types.Named:
			var ts []types.Type
			for i := 0; i < u.TypeArgs().Len(); i++ {
				ts = append(ts, u.TypeArgs().At(i))
			}
			if u.Obj().Pkg() == nil || !s.p.local[u.Obj().Pkg().Path()] {
				targs(ts)
			} else {
				for _, t := range ts {
					foreign(t)
				}
			}
		case *types.Pointer:
			foreign(u.Elem())
		case *types.Slice:
			foreign(u.Elem())
		case *types.Array:
			foreign(u.Elem())
		case *types.Chan:
			foreign(u.Elem())
		case *types.Map:
			foreign(u.Key())
			foreign(u.Elem())
		case *types.Struct:
			for i := 0; i < u.NumFields(); i++ {
				foreign(u.Field(i).Type())
			}
		case *types.Signature:
			for _, tu := range []*types.Tuple{u.Params(), u.Results()} {
				for i := 0; i < tu.Len(); i++ {
					foreign(tu.At(i).Type())
				}
			}
		}
	}
	// called: every use of v is as the function a call calls.
	called := func(v ssa.Value) bool {
		for _, r := range *v.Referrers() {
			switch r := r.(type) {
			case ssa.CallInstruction:
				if c := r.Common(); c.IsInvoke() || c.Value != v || slices.Contains(c.Args, v) {
					return false
				}
			case *ssa.DebugRef:
			default:
				return false
			}
		}
		return true
	}
	for fn := range ssautil.AllFunctions(s.prog) {
		for _, b := range fn.Blocks {
			for _, ins := range b.Instrs {
				if mi, ok := ins.(*ssa.MakeInterface); ok {
					methods(mi.X.Type())
				}
				if v, ok := ins.(ssa.Value); ok {
					foreign(v.Type())
				}
				for _, op := range ins.Operands(nil) {
					if *op == nil {
						continue
					}
					foreign((*op).Type())
					f, ok := (*op).(*ssa.Function)
					if !ok {
						continue
					}
					if !s.localFn(f) {
						targs(f.TypeArgs()) // called or used as a value
					}
					if c, ok := ins.(ssa.CallInstruction); ok && !c.Common().IsInvoke() && op == &c.Common().Value {
						continue
					}
					if mc, ok := ins.(*ssa.MakeClosure); ok && op == &mc.Fn && called(mc) {
						continue
					}
					ix.fns[f] = true
					if m, ok := f.Object().(*types.Func); ok {
						ix.meth[m] = true // a bound method value's wrapper calls the method itself
					}
				}
			}
		}
	}
	s.taken = ix
	return ix
}

// fieldIndex: for each field of a struct a connector package declares, the
// values every load of it produces, whether any code stores into it, and
// whether its address goes anywhere but a load or a store.
type fieldIndex struct {
	reads  map[*types.Var][]ssa.Value
	stored map[*types.Var]bool
	addr   map[*types.Var]bool
}

// fieldVar: the connector struct field a Field or FieldAddr selects, or nil.
func (s *ssaPass) fieldVar(v ssa.Value) *types.Var {
	var x ssa.Value
	var i int
	switch v := v.(type) {
	case *ssa.Field:
		x, i = v.X, v.Field
	case *ssa.FieldAddr:
		x, i = v.X, v.Field
	default:
		return nil
	}
	t := x.Type()
	if _, ok := v.(*ssa.FieldAddr); ok {
		t = types.Unalias(t).Underlying().(*types.Pointer).Elem()
	}
	if !s.connectorNamed(t) {
		return nil
	}
	st, ok := t.Underlying().(*types.Struct)
	if !ok {
		return nil
	}
	return st.Field(i)
}

func (s *ssaPass) fields() *fieldIndex {
	if s.flds != nil {
		return s.flds
	}
	ix := &fieldIndex{reads: map[*types.Var][]ssa.Value{}, stored: map[*types.Var]bool{}, addr: map[*types.Var]bool{}}
	for fn := range ssautil.AllFunctions(s.prog) {
		for _, b := range fn.Blocks {
			for _, ins := range b.Instrs {
				v, ok := ins.(ssa.Value)
				if !ok {
					continue
				}
				f := s.fieldVar(v)
				if f == nil {
					continue
				}
				if _, ok := v.(*ssa.Field); ok {
					ix.reads[f] = append(ix.reads[f], v)
					continue
				}
				for _, r := range *v.Referrers() {
					switch r := r.(type) {
					case *ssa.UnOp:
						if r.Op == token.MUL {
							ix.reads[f] = append(ix.reads[f], r)
						} else {
							ix.addr[f] = true
						}
					case *ssa.Store:
						if r.Addr == v {
							ix.stored[f] = true
						} else {
							ix.addr[f] = true
						}
					case *ssa.DebugRef:
					default:
						ix.addr[f] = true
					}
				}
			}
		}
	}
	s.flds = ix
	return ix
}

// neverSet: v loads a function-typed field of a connector struct that no
// code stores into and whose address goes nowhere: it is always nil, so a
// call through it never reaches a function.
func (s *ssaPass) neverSet(v ssa.Value) bool {
	if u, ok := v.(*ssa.UnOp); ok && u.Op == token.MUL {
		v = u.X
	} else if _, ok := v.(*ssa.Field); !ok {
		return false
	}
	f := s.fieldVar(v)
	if f == nil {
		return false
	}
	ix := s.fields()
	return !ix.stored[f] && !ix.addr[f]
}

// unreached: a call of a function value (not an interface method) whose
// callee set VTA finds empty, where the function type names a type a
// connector package declares. No other package can build a function of that
// type without first being handed a value holding a sink (which fails where
// it is handed), so only connector code, all of which VTA sees, makes one:
// none reaches this call. An interface method call gets no such pass: another
// package's generic type can implement a connector interface.
func (s *ssaPass) unreached(c *ssa.CallCommon) bool {
	return !c.IsInvoke() && s.namesConnector(c.Signature(), map[types.Type]bool{})
}

func (s *ssaPass) namesConnector(t types.Type, seen map[types.Type]bool) bool {
	if seen[t] {
		return false
	}
	seen[t] = true
	switch u := types.Unalias(t).(type) {
	case *types.Named:
		if s.connectorNamed(u) {
			return true
		}
		for i := 0; i < u.TypeArgs().Len(); i++ {
			if s.namesConnector(u.TypeArgs().At(i), seen) {
				return true
			}
		}
	case *types.Pointer:
		return s.namesConnector(u.Elem(), seen)
	case *types.Slice:
		return s.namesConnector(u.Elem(), seen)
	case *types.Array:
		return s.namesConnector(u.Elem(), seen)
	case *types.Chan:
		return s.namesConnector(u.Elem(), seen)
	case *types.Map:
		return s.namesConnector(u.Key(), seen) || s.namesConnector(u.Elem(), seen)
	case *types.Signature:
		return s.namesConnector(u.Params(), seen) || s.namesConnector(u.Results(), seen)
	case *types.Tuple:
		for i := 0; i < u.Len(); i++ {
			if s.namesConnector(u.At(i).Type(), seen) {
				return true
			}
		}
	}
	return false
}

// ssaCheck builds the program and runs every SSA rule.
func (p *pass) ssaCheck(all []*packages.Package) {
	prog, _ := ssautil.Packages(all, ssa.InstantiateGenerics)
	prog.Build()
	s := &ssaPass{p: p, prog: prog, memo: map[types.Type]bool{}, funs: map[*ssa.Function]bool{},
		decls: map[*token.File][]declRange{}, calls: map[token.Pos]token.Pos{}}
	for _, pk := range p.pkgs {
		for _, f := range pk.Syntax {
			tf := p.fset.File(f.Pos())
			for _, d := range f.Decls {
				s.decls[tf] = append(s.decls[tf], declRange{d.Pos(), d.End(), enclosing([]ast.Node{nil, d})})
			}
			ast.Inspect(f, func(n ast.Node) bool {
				if c, ok := n.(*ast.CallExpr); ok {
					s.calls[c.Lparen] = c.Fun.Pos()
				}
				return true
			})
		}
	}
	all2 := ssautil.AllFunctions(prog)
	s.cg = vta.CallGraph(all2, cha.CallGraph(prog))
	s.cg.DeleteSyntheticNodes()
	var fns []*ssa.Function
	for fn := range all2 {
		if s.checked(fn) {
			s.funs[fn] = true
			fns = append(fns, fn)
		}
	}
	sort.Slice(fns, func(i, j int) bool { return fns[i].String() < fns[j].String() })
	for _, fn := range fns {
		for _, b := range fn.Blocks {
			for _, ins := range b.Instrs {
				s.instr(fn, ins)
				var ops []*ssa.Value
				for _, op := range ins.Operands(ops) {
					if op != nil && *op != nil {
						s.operand(fn, ins, *op)
					}
				}
			}
		}
	}
	s.region()
	p.ssaOut = append(p.ssaOut, s.out...)
	p.ssa = s
}

// checked: a function of a connector package (not the kit), with a concrete
// body: an instantiation stands in for its generic origin.
func (s *ssaPass) checked(fn *ssa.Function) bool {
	if !s.localFn(fn) || len(fn.Blocks) == 0 {
		return false
	}
	for f := fn; f != nil; f = f.Parent() {
		if f.TypeParams().Len() > 0 && len(f.TypeArgs()) == 0 {
			return false // a generic body: each instantiation is checked instead
		}
		for _, ta := range f.TypeArgs() {
			if openType(ta, map[types.Type]bool{}) {
				return false // instantiated inside another generic body: not one the program runs
			}
		}
	}
	return true
}

// openType: t still mentions a type parameter.
func openType(t types.Type, seen map[types.Type]bool) bool {
	if seen[t] {
		return false
	}
	seen[t] = true
	switch u := types.Unalias(t).(type) {
	case *types.TypeParam:
		return true
	case *types.Named:
		for i := 0; i < u.TypeArgs().Len(); i++ {
			if openType(u.TypeArgs().At(i), seen) {
				return true
			}
		}
		return false
	case *types.Pointer:
		return openType(u.Elem(), seen)
	case *types.Slice:
		return openType(u.Elem(), seen)
	case *types.Array:
		return openType(u.Elem(), seen)
	case *types.Chan:
		return openType(u.Elem(), seen)
	case *types.Map:
		return openType(u.Key(), seen) || openType(u.Elem(), seen)
	case *types.Signature:
		return openType(u.Params(), seen) || openType(u.Results(), seen)
	case *types.Tuple:
		for i := 0; i < u.Len(); i++ {
			if openType(u.At(i).Type(), seen) {
				return true
			}
		}
	case *types.Struct:
		for i := 0; i < u.NumFields(); i++ {
			if openType(u.Field(i).Type(), seen) {
				return true
			}
		}
	case *types.Interface:
		for i := 0; i < u.NumMethods(); i++ {
			if openType(u.Method(i).Type(), seen) {
				return true
			}
		}
	}
	return false
}

// fnPkg is the package a function's source belongs to: an instantiation's
// origin's, a function literal's enclosing function's.
func fnPkg(fn *ssa.Function) *types.Package {
	for fn != nil {
		if fn.Pkg != nil {
			return fn.Pkg.Pkg
		}
		if o := fn.Origin(); o != nil && o != fn {
			fn = o
			continue
		}
		fn = fn.Parent()
	}
	return nil
}

// localFn: a connector function (not the kit's) with source, not a
// synthetic wrapper of another package's method.
func (s *ssaPass) localFn(fn *ssa.Function) bool {
	if fn == nil {
		return false
	}
	pk := fnPkg(fn)
	if pk == nil || !s.p.local[pk.Path()] {
		return false
	}
	if m, ok := fn.Object().(*types.Func); ok && fn.Synthetic != "" {
		return m.Pkg() != nil && s.p.local[m.Pkg().Path()]
	}
	return true
}

func (s *ssaPass) kitFn(fn *ssa.Function, names ...string) bool {
	if fn == nil || fnPkg(fn) == nil || fnPkg(fn).Path() != s.p.kit || fn.Signature.Recv() != nil || fn.Parent() != nil {
		return false
	}
	for _, n := range names {
		if fn.Name() == n {
			return true
		}
	}
	return len(names) == 0
}

// ---- holds a sink: a fixed point on the type graph ----

// holds reports whether a value of t is or can reach a sink: a sink type,
// or any field, element, key, pointer base, type argument, signature
// parameter or result, or method parameter or result that reaches one.
// Cycles terminate.
func (s *ssaPass) holds(t types.Type) bool {
	if t == nil {
		return false
	}
	visited := map[types.Type]bool{}
	r := s.holdsRec(t, visited)
	if !r {
		// Nothing reachable from t reached a sink: every type visited is final.
		for v := range visited {
			s.memo[v] = false
		}
	}
	return r
}

func (s *ssaPass) holdsRec(t types.Type, visited map[types.Type]bool) bool {
	if v, ok := s.memo[t]; ok {
		return v
	}
	if visited[t] {
		return false
	}
	visited[t] = true
	r := s.holdsStep(t, visited)
	if r {
		s.memo[t] = true
	}
	return r
}

func (s *ssaPass) holdsStep(t types.Type, visited map[types.Type]bool) bool {
	rec := func(u types.Type) bool { return u != nil && s.holdsRec(u, visited) }
	sig := func(sg *types.Signature) bool {
		for i := 0; i < sg.Params().Len(); i++ {
			if rec(sg.Params().At(i).Type()) {
				return true
			}
		}
		for i := 0; i < sg.Results().Len(); i++ {
			if rec(sg.Results().At(i).Type()) {
				return true
			}
		}
		return false
	}
	switch u := t.(type) {
	case *types.Alias:
		return rec(types.Unalias(u))
	case *types.Named:
		if ssaSinks[namedKey(u)] {
			return true
		}
		if ta := u.TypeArgs(); ta != nil {
			for i := 0; i < ta.Len(); i++ {
				if rec(ta.At(i)) {
					return true
				}
			}
		}
		for _, mt := range []types.Type{u, types.NewPointer(u)} {
			ms := s.prog.MethodSets.MethodSet(mt)
			for i := 0; i < ms.Len(); i++ {
				if sg, ok := ms.At(i).Type().(*types.Signature); ok && sig(sg) {
					return true
				}
			}
		}
		return rec(u.Underlying())
	case *types.TypeParam:
		return rec(u.Constraint())
	case *types.Pointer:
		return rec(u.Elem())
	case *types.Slice:
		return rec(u.Elem())
	case *types.Array:
		return rec(u.Elem())
	case *types.Chan:
		return rec(u.Elem())
	case *types.Map:
		return rec(u.Key()) || rec(u.Elem())
	case *types.Signature:
		return sig(u)
	case *types.Tuple:
		for i := 0; i < u.Len(); i++ {
			if rec(u.At(i).Type()) {
				return true
			}
		}
	case *types.Struct:
		if s.p.isClient(u) {
			return true
		}
		for i := 0; i < u.NumFields(); i++ {
			if rec(u.Field(i).Type()) {
				return true
			}
		}
	case *types.Interface:
		if s.p.sinkSatisfies(u) != "" {
			return true // a command, client or transport satisfies it by running or sending
		}
		for i := 0; i < u.NumMethods(); i++ {
			if sig(u.Method(i).Type().(*types.Signature)) {
				return true
			}
		}
	}
	return false
}

// kind names the sink t is or points to ("" for a carrier or no sink).
func (s *ssaPass) kind(t types.Type) string {
	if pt, ok := types.Unalias(t).(*types.Pointer); ok {
		t = pt.Elem()
	}
	if k := namedKey(t); ssaSinks[k] {
		return k
	}
	if _, named := types.Unalias(t).(*types.Named); !named && s.p.isClient(t) {
		return "net/http.Client"
	}
	return ""
}

// what is the finding's "what" for a sink value of type t.
func (s *ssaPass) what(t types.Type) string {
	if k := s.kind(t); k != "" {
		return "sink:" + k
	}
	return "carrier"
}

// byValue names a client or command t holds by value (in its own struct
// fields or array elements, not behind a pointer), or "": its zero value
// sends with http.DefaultTransport or is a command exec.Command never built.
func (s *ssaPass) byValue(t types.Type) string {
	return s.byValueSeen(t, map[types.Type]bool{})
}

func (s *ssaPass) byValueSeen(t types.Type, seen map[types.Type]bool) string {
	if t == nil || seen[t] {
		return ""
	}
	seen[t] = true
	switch k := namedKey(t); k {
	case "os/exec.Cmd", "net/http.Client":
		return k
	}
	switch u := types.Unalias(t).Underlying().(type) {
	case *types.Struct:
		if s.p.isClient(u) {
			return "net/http.Client"
		}
		for i := 0; i < u.NumFields(); i++ {
			if k := s.byValueSeen(u.Field(i).Type(), seen); k != "" {
				return k
			}
		}
	case *types.Array:
		return s.byValueSeen(u.Elem(), seen)
	}
	return ""
}

// ---- findings ----

func (s *ssaPass) posOf(fn *ssa.Function, ins ssa.Instruction) token.Pos {
	if c, ok := ins.(ssa.CallInstruction); ok {
		if fp, ok := s.calls[c.Common().Pos()]; ok {
			return fp
		}
	}
	if ins != nil && ins.Pos().IsValid() {
		return ins.Pos()
	}
	if ins != nil {
		for _, op := range ins.Operands(nil) {
			if op != nil && *op != nil {
				if _, isConst := (*op).(*ssa.Const); !isConst && (*op).Pos().IsValid() {
					return (*op).Pos()
				}
			}
		}
		if b := ins.Block(); b != nil {
			idx := -1
			for i, x := range b.Instrs {
				if x == ins {
					idx = i
				}
			}
			for i := idx + 1; i < len(b.Instrs); i++ {
				if b.Instrs[i].Pos().IsValid() {
					return b.Instrs[i].Pos()
				}
			}
			for i := idx - 1; i >= 0; i-- {
				if b.Instrs[i].Pos().IsValid() {
					return b.Instrs[i].Pos()
				}
			}
		}
	}
	for f := fn; f != nil; f = f.Parent() {
		if f.Pos().IsValid() {
			return f.Pos()
		}
	}
	return token.NoPos
}

func (s *ssaPass) fail(fn *ssa.Function, ins ssa.Instruction, rule int, what, msg, fix string) {
	s.out = append(s.out, s.finding(s.posOf(fn, ins), rule, what, msg, fix))
}

func (s *ssaPass) finding(at token.Pos, rule int, what, msg, fix string) Finding {
	f := Finding{Rule: rule, File: ".", Func: "file", What: what, Msg: msg, Fix: fix, Site: true, SSA: true}
	if !at.IsValid() {
		return f
	}
	pos := s.p.fset.Position(at)
	f.File, f.Line, f.Col = filepath.ToSlash(relTo(s.p.cli, pos.Filename)), pos.Line, pos.Column
	for _, d := range s.decls[s.p.fset.File(at)] {
		if d.pos <= at && at < d.end {
			f.Func = d.name
		}
	}
	return f
}

const ssaFix = "build a command with exec.Command and run it with credkit.StartCommand or credkit.RunCommand; build a client as &http.Client{Transport: credkit.Transport(...)}; keep a value that holds one its own type, inside the connector: never an interface, a conversion, a type argument or another package's code"

// ---- the closed instruction set ----

// instr models one instruction. Every concrete instruction kind of the
// pinned x/tools has a case; the default fails any instruction that touches
// a sink value (TestSSAKindsCovered).
func (s *ssaPass) instr(fn *ssa.Function, ins ssa.Instruction) {
	switch x := ins.(type) {
	case *ssa.Alloc:
		elem := x.Type().(*types.Pointer).Elem()
		if k := s.byValue(elem); k != "" && !(k == "net/http.Client" && s.kind(elem) == k && (s.clientLiteral(x) || s.clientCopy(x))) {
			what := "sink:" + k
			if k == "net/http.Client" && s.kind(elem) == k {
				what = "net/http.Client{}" // as the AST rule names a client literal
			}
			s.fail(fn, x, 2, what, "a "+k+" made here by value (a zero value, new, var, or a literal without credkit.Transport): it is a client whose transport the kit does not wrap, or a command exec.Command did not build", ssaFix)
		}
	case *ssa.BinOp, *ssa.Extract, *ssa.Index, *ssa.IndexAddr, *ssa.Slice, *ssa.SliceToArrayPointer, *ssa.Next, *ssa.Range,
		*ssa.Phi, *ssa.Send, *ssa.Select, *ssa.MapUpdate, *ssa.If, *ssa.Jump, *ssa.Return, *ssa.RunDefers, *ssa.Panic, *ssa.DebugRef:
		// Each moves or reads a value within its own type (a channel's
		// element, a map's, a slice's, a tuple's, a phi's operands all have
		// the result's type) or does not move one: nothing changes type.
	case *ssa.Call:
		s.call(fn, x, x.Common(), x)
	case *ssa.Go:
		s.call(fn, x, x.Common(), nil)
	case *ssa.Defer:
		s.call(fn, x, x.Common(), nil)
	case *ssa.ChangeInterface:
		s.conv(fn, x, x.X)
	case *ssa.ChangeType:
		s.conv(fn, x, x.X)
	case *ssa.Convert:
		s.conv(fn, x, x.X)
	case *ssa.MultiConvert:
		s.conv(fn, x, x.X)
	case *ssa.Field:
		s.field(fn, x, x.X.Type(), x.Field)
	case *ssa.FieldAddr:
		s.field(fn, x, x.X.Type().Underlying().(*types.Pointer).Elem(), x.Field)
		s.transportAddr(fn, x)
	case *ssa.Lookup:
		t := x.Type()
		if tu, ok := t.(*types.Tuple); ok {
			t = tu.At(0).Type()
		}
		if k := s.byValue(t); k != "" {
			s.fail(fn, x, 2, "sink:"+k, "a map lookup yields a "+k+" by value, the zero one when the key is missing", ssaFix)
		}
	case *ssa.MakeChan:
		s.zeroElems(fn, x, x.Type())
	case *ssa.MakeMap:
		s.zeroElems(fn, x, x.Type())
	case *ssa.MakeSlice:
		s.zeroElems(fn, x, x.Type())
	case *ssa.MakeClosure:
		s.closure(fn, x)
	case *ssa.MakeInterface:
		s.makeInterface(fn, x)
	case *ssa.Store:
		s.store(fn, x)
	case *ssa.TypeAssert:
		t := x.AssertedType
		if s.holds(t) && s.kind(t) != "net/http.Transport" && (!s.connectorNamed(t) || types.IsInterface(t)) {
			s.fail(fn, x, 2, s.what(t), "a type assertion recovers a value holding a sink from an interface, where the checker cannot see how it was built", ssaFix)
		}
	case *ssa.UnOp:
		s.unop(fn, x)
	default:
		touches := false
		if v, ok := ins.(ssa.Value); ok && s.holds(v.Type()) {
			touches = true
		}
		for _, op := range ins.Operands(nil) {
			if op != nil && *op != nil && s.holds((*op).Type()) {
				touches = true
			}
		}
		if touches {
			s.fail(fn, ins, 2, "ssa:"+fmt.Sprintf("%T", ins), "an instruction kind the checker does not model touches a value holding a sink", ssaFix)
		}
	}
}

// operand models a value that is not itself an instruction.
func (s *ssaPass) operand(fn *ssa.Function, ins ssa.Instruction, v ssa.Value) {
	switch x := v.(type) {
	case *ssa.Const:
		if k := s.byValue(x.Type()); k != "" {
			s.fail(fn, ins, 2, "sink:"+k, "a zero "+k+" by value", ssaFix)
		}
		if x.IsNil() && s.kind(x.Type()) == "net/http.RoundTripper" {
			if c, ok := ins.(*ssa.Call); ok && s.kitFn(c.Call.StaticCallee(), "Transport") {
				return // the kit wraps http.DefaultTransport itself
			}
			if _, cmp := ins.(*ssa.BinOp); !cmp {
				s.fail(fn, ins, 2, "sink:net/http.RoundTripper", "a nil http.RoundTripper used other than in a comparison: a client given it sends with http.DefaultTransport", "set a client's Transport only to credkit.Transport(...)")
			}
		}
	case *ssa.Function:
		if c, ok := ins.(ssa.CallInstruction); ok && c.Common().Value == v {
			return // the callee: call checks it
		}
		if s.localFn(x) || s.kitFn(x) {
			return
		}
		held := s.holds(x.Signature)
		for _, ta := range x.TypeArgs() {
			held = held || s.holds(ta)
		}
		if held {
			s.fail(fn, ins, 2, ssaKey(x), "a function of another package that takes or returns a value holding a sink, or is instantiated with one ("+ssaKey(x)+"), used as a value", ssaFix)
		}
	case *ssa.Global:
		if s.localGlobal(x) {
			return
		}
		if u, ok := ins.(*ssa.UnOp); ok && u.Op == token.MUL {
			return // a load: unop checks it
		}
		if s.holds(x.Type()) {
			s.fail(fn, ins, 2, objKey(x.Object()), "another package's variable holding a sink ("+objKey(x.Object())+") used other than read", ssaFix)
		}
	case *ssa.Parameter, *ssa.FreeVar, *ssa.Builtin:
		// A parameter is filled by a call checked at its call site; a free
		// variable by the MakeClosure that binds it; a builtin is a callee.
	case ssa.Instruction:
		// Checked as an instruction.
	default:
		if s.holds(v.Type()) {
			s.fail(fn, ins, 2, "ssa:"+fmt.Sprintf("%T", v), "a value kind the checker does not model holds a sink", ssaFix)
		}
	}
}

// connectorNamed: t is, or points to, a named type a connector package
// declares, whose values only connector code (checked here) builds.
func (s *ssaPass) connectorNamed(t types.Type) bool {
	if pt, ok := types.Unalias(t).(*types.Pointer); ok {
		t = pt.Elem()
	}
	n, ok := types.Unalias(t).(*types.Named)
	return ok && n.Obj().Pkg() != nil && s.p.local[n.Obj().Pkg().Path()]
}

func (s *ssaPass) localGlobal(g *ssa.Global) bool {
	return g.Pkg != nil && (s.p.local[g.Pkg.Pkg.Path()] || g.Pkg.Pkg.Path() == s.p.kit)
}

// ssaKey names a function as the ledger does.
func ssaKey(fn *ssa.Function) string {
	if o := fn.Object(); o != nil {
		return objKey(o)
	}
	return fn.String()
}

// conv: a conversion of a value holding a sink to another type (an
// interface, a named type with the same structure, a pointer through
// unsafe) loses what the allow-lists know about it.
func (s *ssaPass) conv(fn *ssa.Function, ins ssa.Instruction, x ssa.Value) {
	if s.holds(x.Type()) || s.holds(ins.(ssa.Value).Type()) {
		s.fail(fn, ins, 2, s.what(x.Type()), "a "+types.TypeString(x.Type(), nil)+", which holds a sink, converted to "+types.TypeString(ins.(ssa.Value).Type(), nil)+": the checker cannot follow it as another type", ssaFix)
	}
}

// field: a sink held in another package's field is one the checker never
// saw built (net/http's and os/exec's own fields are the sinks themselves).
func (s *ssaPass) field(fn *ssa.Function, ins ssa.Instruction, st types.Type, i int) {
	su, ok := st.Underlying().(*types.Struct)
	if !ok || i >= su.NumFields() {
		return
	}
	f := su.Field(i)
	if f.Pkg() == nil || s.p.local[f.Pkg().Path()] || f.Pkg().Path() == s.p.kit || sinkPkg(f.Pkg().Path()) {
		return
	}
	if s.holds(f.Type()) {
		s.fail(fn, ins, 2, objKey(f), "a value holding a sink in a field of another package's type ("+objKey(f)+")", "build the client or command in the connector")
	}
}

// isTransportField: field i of struct st is http.Client's Transport.
func (s *ssaPass) isTransportField(st types.Type, i int) bool {
	if s.kind(st) != "net/http.Client" {
		return false
	}
	su, ok := st.Underlying().(*types.Struct)
	return ok && i < su.NumFields() && su.Field(i).Name() == "Transport"
}

// transportAddr: the address of a client's Transport may only be stored
// through (with credkit.Transport's result) or loaded.
func (s *ssaPass) transportAddr(fn *ssa.Function, x *ssa.FieldAddr) {
	if !s.isTransportField(x.X.Type().Underlying().(*types.Pointer).Elem(), x.Field) {
		return
	}
	for _, r := range *x.Referrers() {
		switch r := r.(type) {
		case *ssa.Store:
			if r.Addr == x {
				continue
			}
		case *ssa.UnOp:
			if r.Op == token.MUL {
				continue
			}
		case *ssa.DebugRef:
			continue
		}
		s.fail(fn, x, 2, "net/http.Client.Transport", "the address of an http.Client's transport goes where it can be replaced out of the checker's sight", "assign credkit.Transport(base) directly")
		return
	}
}

// kitTransport: v is a call of credkit.Transport.
func (s *ssaPass) kitTransport(v ssa.Value) bool {
	c, ok := v.(*ssa.Call)
	return ok && s.kitFn(c.Call.StaticCallee(), "Transport")
}

// clientLiteral: an http.Client allocated here has its Transport stored
// from credkit.Transport in this function (the composite literal).
func (s *ssaPass) clientLiteral(a *ssa.Alloc) bool {
	for _, r := range *a.Referrers() {
		fa, ok := r.(*ssa.FieldAddr)
		if !ok || fa.X != a || !s.isTransportField(a.Type().(*types.Pointer).Elem(), fa.Field) {
			continue
		}
		for _, rr := range *fa.Referrers() {
			if st, ok := rr.(*ssa.Store); ok && st.Addr == fa && s.kitTransport(st.Val) {
				return true
			}
		}
	}
	return false
}

// clientCopy: an http.Client allocated here is a copy of a client loaded
// from memory (c := *base), whose construction is checked where it was
// built; a later store into the copy's Transport is checked as any other.
func (s *ssaPass) clientCopy(a *ssa.Alloc) bool {
	for _, r := range *a.Referrers() {
		if st, ok := r.(*ssa.Store); ok && st.Addr == a {
			if u, ok := st.Val.(*ssa.UnOp); ok && u.Op == token.MUL {
				return true
			}
		}
	}
	return false
}

func (s *ssaPass) zeroElems(fn *ssa.Function, ins ssa.Instruction, t types.Type) {
	var elems []types.Type
	switch u := t.Underlying().(type) {
	case *types.Chan:
		elems = []types.Type{u.Elem()}
	case *types.Map:
		elems = []types.Type{u.Key(), u.Elem()}
	case *types.Slice:
		elems = []types.Type{u.Elem()}
	}
	for _, e := range elems {
		if k := s.byValue(e); k != "" {
			s.fail(fn, ins, 2, "sink:"+k, "a container of "+k+" values by value, whose zero elements the kit never saw built", ssaFix)
			return
		}
	}
}

// store: a value holding a sink is stored only into the connector's own
// memory; a client's Transport only from credkit.Transport.
func (s *ssaPass) store(fn *ssa.Function, x *ssa.Store) {
	if fa, ok := x.Addr.(*ssa.FieldAddr); ok && s.isTransportField(fa.X.Type().Underlying().(*types.Pointer).Elem(), fa.Field) {
		if a, ok := fa.X.(*ssa.Alloc); ok && !s.clientLiteral(a) && !s.clientCopy(a) {
			return // the client made here is the finding (instr's Alloc case)
		}
		if !s.kitTransport(x.Val) {
			s.fail(fn, x, 2, "net/http.Client.Transport", "an http.Client's transport set to something other than credkit.Transport(...)", "assign credkit.Transport(base)")
		}
		return
	}
	if !s.holds(x.Val.Type()) {
		return
	}
	switch a := x.Addr.(type) {
	case *ssa.Global:
		if !s.localGlobal(a) {
			s.fail(fn, x, 2, objKey(a.Object()), "a value holding a sink stored into another package's variable", ssaFix)
		}
	case *ssa.FieldAddr:
		st := a.X.Type().Underlying().(*types.Pointer).Elem()
		if su, ok := st.Underlying().(*types.Struct); ok {
			f := su.Field(a.Field)
			if f.Pkg() != nil && !s.p.local[f.Pkg().Path()] && f.Pkg().Path() != s.p.kit {
				s.fail(fn, x, 2, objKey(f), "a value holding a sink stored into another package's field ("+objKey(f)+")", ssaFix)
			}
		}
	}
}

func (s *ssaPass) unop(fn *ssa.Function, x *ssa.UnOp) {
	if x.Op != token.MUL {
		return
	}
	g, ok := x.X.(*ssa.Global)
	if !ok || s.localGlobal(g) || !s.holds(x.Type()) {
		return
	}
	key := objKey(g.Object())
	if key == "net/http.DefaultTransport" {
		ok := true
		for _, r := range *x.Referrers() {
			switch r := r.(type) {
			case *ssa.Call:
				ok = ok && s.kitFn(r.Call.StaticCallee(), "Transport")
			case *ssa.TypeAssert:
				ok = ok && s.kind(r.AssertedType) == "net/http.Transport"
			case *ssa.DebugRef:
			default:
				ok = false
			}
		}
		if ok {
			return
		}
	}
	s.fail(fn, x, 2, key, "a sink another package holds ("+key+")", ssaFix)
}

// closure: a function literal's body is checked as its own function; a
// method value binds its receiver, which must be allowed that method.
func (s *ssaPass) closure(fn *ssa.Function, x *ssa.MakeClosure) {
	f := x.Fn.(*ssa.Function)
	if s.localFn(f) && f.Synthetic == "" {
		return
	}
	m, _ := f.Object().(*types.Func)
	for _, b := range x.Bindings {
		if !s.holds(b.Type()) {
			continue
		}
		if m != nil {
			if k := s.kind(b.Type()); k != "" && allowedMethods[k][m.Name()] && sinkPkg(m.Pkg().Path()) {
				continue
			}
			if m.Pkg() != nil && s.p.local[m.Pkg().Path()] {
				continue
			}
		}
		s.fail(fn, x, 2, s.what(b.Type()), "a value holding a sink bound into "+f.String()+", a function the kit does not allow it", ssaFix)
	}
}

// makeInterface: a value holding a sink becomes an interface only as
// credkit.Transport's argument; a value whose methods start a sink (a
// child process's Run, an MCP server's ServeHTTP, another package's
// RoundTrip) only under the kit's wrapper for it.
func (s *ssaPass) makeInterface(fn *ssa.Function, x *ssa.MakeInterface) {
	t := x.X.Type()
	underKit := func(names ...string) bool {
		refs := *x.Referrers()
		if len(refs) == 0 {
			return false
		}
		for _, r := range refs {
			c, ok := r.(*ssa.Call)
			if !ok || !s.kitFn(c.Call.StaticCallee(), names...) {
				return false
			}
		}
		return true
	}
	if s.holds(t) {
		if underKit("Transport") {
			return // the kit wraps the transport; a connector's own one's RoundTrip body is checked
		}
		if s.intoTransportField(x) {
			return // the store into a client's Transport (or the client literal) is the finding
		}
		s.escT = t
		why := s.escapes(x, map[ssa.Value]bool{})
		if why == "" {
			return // the interface value stays in connector code the checker walks
		}
		s.fail(fn, x, 2, s.what(t), "a "+types.TypeString(t, nil)+", which holds a sink, becomes the interface "+types.TypeString(x.Type(), nil)+", and the interface value "+why, ssaFix)
		return
	}
	// Every method of the value can run once it is an interface (it can be
	// asserted to any other), so the source's whole method set counts.
	ms := s.prog.MethodSets.MethodSet(t)
	for i := 0; i < ms.Len(); i++ {
		m, ok := ms.At(i).Obj().(*types.Func)
		if !ok {
			continue
		}
		key := objKey(m)
		wrapper := ""
		switch {
		case execRun[key], key == mcpServerPkg+".StdioServer.Listen":
		case strings.HasPrefix(key, mcpServerPkg+".StreamableHTTPServer.") || strings.HasPrefix(key, mcpServerPkg+".SSEServer."):
			if !mcpServe[key] {
				continue
			}
			wrapper = "MCPHTTP"
		case m.Name() == "RoundTrip" && m.Pkg() != nil && !s.p.local[m.Pkg().Path()] && m.Pkg().Path() != s.p.kit:
			wrapper = "Transport"
		default:
			continue
		}
		if wrapper != "" && underKit(wrapper) {
			continue
		}
		s.fail(fn, x, 2, "convert:"+key, fmt.Sprintf("a %s becomes a %s here, so its %s can run where the checker cannot see it", types.TypeString(t, nil), types.TypeString(x.Type(), nil), m.Name()), "pass it straight to the kit (credkit.Transport, credkit.MCPHTTP) or keep its concrete type")
		return
	}
}

// intoTransportField: every use of v stores it into a client's Transport.
func (s *ssaPass) intoTransportField(v ssa.Value) bool {
	refs := *v.Referrers()
	for _, r := range refs {
		st, ok := r.(*ssa.Store)
		if !ok || st.Val != v {
			return false
		}
		fa, ok := st.Addr.(*ssa.FieldAddr)
		if !ok || !s.isTransportField(fa.X.Type().Underlying().(*types.Pointer).Elem(), fa.Field) {
			return false
		}
	}
	return len(refs) > 0
}

// escapes follows an interface value made from a value holding a sink
// through the connector code it moves in (phis, interface conversions and
// assertions, calls of connector functions, local variables, closures,
// returns to connector callers) and says where it leaves what the checker
// follows (another package, memory other than a local variable), or "".
func (s *ssaPass) escapes(v ssa.Value, seen map[ssa.Value]bool) string {
	if seen[v] {
		return ""
	}
	seen[v] = true
	refs := v.Referrers()
	if refs == nil {
		return ""
	}
	follow := func(w ssa.Value) string { return s.escapes(w, seen) }
	for _, r := range *refs {
		switch r := r.(type) {
		case *ssa.Phi, *ssa.ChangeInterface, *ssa.Extract:
			if why := follow(r.(ssa.Value)); why != "" {
				return why
			}
		case *ssa.TypeAssert:
			if types.IsInterface(r.AssertedType) {
				if why := follow(r); why != "" {
					return why
				}
			} // a concrete type: the value the rules check at every use
		case *ssa.BinOp, *ssa.DebugRef, *ssa.If:
		case ssa.CallInstruction:
			if why := s.escapesCall(r, v, seen); why != "" {
				return why
			}
		case *ssa.Store:
			if r.Val != v {
				continue
			}
			if ia, ok := r.Addr.(*ssa.IndexAddr); ok {
				if why := s.variadic(ia); why != "" {
					return why
				}
			}
			if f := s.fieldVar(r.Addr); f != nil && !f.Exported() {
				// ponytail: field-based, not per object: every load of the field anywhere is followed.
				ix := s.fields()
				if ix.addr[f] {
					return "is stored into a field whose address the checker does not follow (" + s.where(r.Pos()) + ")"
				}
				for _, rd := range ix.reads[f] {
					if !s.funs[rd.Parent()] {
						continue // a generic body: each instantiation's read is in the list
					}
					if why := follow(rd); why != "" {
						return why
					}
				}
				continue
			}
			a, ok := r.Addr.(*ssa.Alloc)
			if !ok {
				return "is stored into memory the checker does not follow (" + s.where(r.Pos()) + ")"
			}
			if why := s.escapesLocal(a, seen); why != "" {
				return why
			}
		case *ssa.MakeClosure:
			for i, b := range r.Bindings {
				if b == v {
					if why := follow(r.Fn.(*ssa.Function).FreeVars[i]); why != "" {
						return why
					}
				}
			}
		case *ssa.Return:
			fn := r.Parent()
			n := s.cg.Nodes[fn]
			if n == nil || len(n.In) == 0 {
				return "is returned by " + fn.String() + ", whose callers the checker cannot see"
			}
			ix := s.takenFns()
			if m, ok := fn.Object().(*types.Func); ix.fns[fn] || ok && fn.Signature.Recv() != nil && ix.meth[m] {
				return "is returned by " + fn.String() + ", which can be called from where the checker cannot see"
			}
			for _, e := range n.In {
				if !s.localFn(e.Caller.Func) {
					return "is returned by " + fn.String() + " to another package"
				}
				if cv := e.Site.Value(); cv != nil {
					if why := follow(cv); why != "" {
						return why
					}
				}
			}
		default:
			return "goes where the checker does not follow it (" + fmt.Sprintf("%T", r) + ")"
		}
	}
	return ""
}

// where names a position relative to the connector's cli directory.
func (s *ssaPass) where(at token.Pos) string {
	if !at.IsValid() {
		return "?"
	}
	pos := s.p.fset.Position(at)
	return fmt.Sprintf("%s:%d", filepath.ToSlash(relTo(s.p.cli, pos.Filename)), pos.Line)
}

// variadic: a store into an element of a call's variadic list names the
// call the list goes to, or "".
func (s *ssaPass) variadic(ia *ssa.IndexAddr) string {
	a, ok := ia.X.(*ssa.Alloc)
	if !ok {
		return ""
	}
	for _, r := range *a.Referrers() {
		sl, ok := r.(*ssa.Slice)
		if !ok {
			continue
		}
		for _, u := range *sl.Referrers() {
			if ci, ok := u.(ssa.CallInstruction); ok {
				name := "a function value"
				if f := ci.Common().StaticCallee(); f != nil {
					name = ssaKey(f)
				} else if ci.Common().IsInvoke() {
					name = objKey(ci.Common().Method)
				}
				return "is handed to " + name + " in its variadic list, which the checker does not follow"
			}
		}
	}
	return ""
}

// escapesLocal: a local variable holding the interface value; its loads are
// followed, and its address may only be stored through, loaded, or bound
// into a closure (whose free variable is followed the same way).
func (s *ssaPass) escapesLocal(a *ssa.Alloc, seen map[ssa.Value]bool) string {
	if seen[a] {
		return ""
	}
	seen[a] = true
	for _, r := range *a.Referrers() {
		switch r := r.(type) {
		case *ssa.Store:
			if r.Addr != a {
				return "has the address of the variable holding it taken"
			}
		case *ssa.UnOp:
			if r.Op != token.MUL {
				return "goes where the checker does not follow it"
			}
			if why := s.escapes(r, seen); why != "" {
				return why
			}
		case *ssa.DebugRef:
		case *ssa.MakeClosure:
			for i, b := range r.Bindings {
				if b != a {
					continue
				}
				fv := r.Fn.(*ssa.Function).FreeVars[i]
				for _, fr := range *fv.Referrers() {
					switch fr := fr.(type) {
					case *ssa.UnOp:
						if why := s.escapes(fr, seen); why != "" {
							return why
						}
					case *ssa.Store:
						if fr.Addr != fv {
							return "has the address of the variable holding it taken"
						}
					case *ssa.DebugRef:
					default:
						return "goes where the checker does not follow it"
					}
				}
			}
		default:
			return "has the address of the variable holding it taken"
		}
	}
	return ""
}

// escapesCall: v at a call. Called through (the receiver of an interface
// method), it is fine when every method VTA resolves is the connector's;
// handed as an argument, it is followed into each connector callee's
// parameter. Anything else hands it to another package.
func (s *ssaPass) escapesCall(ci ssa.CallInstruction, v ssa.Value, seen map[ssa.Value]bool) string {
	c := ci.Common()
	callees := []*ssa.Function{}
	if c.IsInvoke() && c.Value == v && s.escT != nil {
		// Called through: the method that runs for the followed value is
		// its concrete type's, whatever VTA resolves for the interface.
		sel := s.prog.MethodSets.MethodSet(s.escT).Lookup(c.Method.Pkg(), c.Method.Name())
		if sel == nil {
			return "" // the type has no such method: an assertion before this call fails for it
		}
		f := s.prog.MethodValue(sel)
		if f == nil || !s.localFn(f) || len(f.Blocks) == 0 {
			return "has its method " + c.Method.Name() + " called where the checker cannot see into it"
		}
		callees = append(callees, f) // and an argument that is the value too is followed into it
	} else if f := c.StaticCallee(); f != nil {
		callees = append(callees, f)
	} else if n := s.cg.Nodes[ci.Parent()]; n != nil {
		for _, e := range n.Out {
			if e.Site == ci {
				callees = append(callees, e.Callee.Func)
			}
		}
	}
	if len(callees) == 0 {
		return "is used in a call the checker cannot resolve"
	}
	for _, f := range callees {
		if !s.localFn(f) || len(f.Blocks) == 0 {
			return "is handed to " + ssaKey(f) + ", which the checker cannot see into"
		}
	}
	shift := 0
	if c.IsInvoke() {
		shift = 1 // the callee's receiver parameter
	}
	for i, a := range c.Args {
		if a != v {
			continue
		}
		for _, f := range callees {
			if i+shift >= len(f.Params) {
				return "is handed to a variadic or bound parameter the checker does not follow"
			}
			if why := s.escapes(f.Params[i+shift], seen); why != "" {
				return why
			}
		}
	}
	if !c.IsInvoke() && c.Value == v {
		return "is called as a function value"
	}
	return ""
}

// call models a call, go or defer: who may receive a value holding a sink.
func (s *ssaPass) call(fn *ssa.Function, ins ssa.Instruction, c *ssa.CallCommon, res ssa.Value) {
	resHolds := res != nil && s.holds(res.Type())
	argHolds := -1
	for i, a := range c.Args {
		if s.holds(a.Type()) {
			argHolds = i
			break
		}
	}
	if c.IsInvoke() {
		if c.Method.Name() == "RoundTrip" && s.kitTransport(c.Value) && argHolds < 0 && !resHolds {
			return // the kit's own transport sends
		}
		if s.holds(c.Value.Type()) || argHolds >= 0 || resHolds {
			if s.localCallees(fn, ins) {
				// Every method VTA finds is the connector's, checked. A value
				// of such an interface only comes from connector code: another
				// package's result, variable or field holding one fails where
				// read, and an assertion to one fails (TypeAssert).
				return
			}
			s.fail(fn, ins, 2, objKey(c.Method), "a method called through an interface ("+objKey(c.Method)+") that takes, returns or is a value holding a sink: the checker cannot see which method runs", ssaFix)
		}
		return
	}
	if b, ok := c.Value.(*ssa.Builtin); ok {
		switch b.Name() {
		case "append", "len", "cap", "copy", "delete", "close", "clear", "min", "max", "ssa:wrapnilchk":
		default:
			if argHolds >= 0 {
				s.fail(fn, ins, 2, "builtin."+b.Name(), "a value holding a sink handed to the builtin "+b.Name(), ssaFix)
			}
		}
		return
	}
	callee := c.StaticCallee()
	switch {
	case callee == nil:
		if (argHolds >= 0 || resHolds) && !s.localCallees(fn, ins) && !s.neverSet(c.Value) && !(s.noCallees(fn, ins) && s.unreached(c)) {
			s.fail(fn, ins, 2, "call:dynamic", "a value holding a sink passed to or returned by a function value the checker cannot resolve to one function", ssaFix)
		}
	case s.localFn(callee):
		if callee.Parent() != nil {
			break // a function literal of an instantiation: the call that instantiated it is the finding
		}
		for _, ta := range callee.TypeArgs() {
			if s.holds(ta) {
				s.fail(fn, ins, 2, "generic:"+s.what(ta), "a generic instantiated with "+types.TypeString(ta, nil)+", which holds a sink", "write the code for the concrete type, without a type parameter")
				return
			}
		}
	case s.kitFn(callee, "Transport", "StartCommand", "RunCommand"):
	case s.kitFn(callee):
		if argHolds >= 0 {
			s.fail(fn, ins, 2, ssaKey(callee), "a value holding a sink handed to "+ssaKey(callee)+", which does not take one", ssaFix)
		}
	default:
		for _, ta := range callee.TypeArgs() {
			if s.holds(ta) {
				s.fail(fn, ins, 2, "generic:"+s.what(ta), "another package's generic instantiated with "+types.TypeString(ta, nil)+", which holds a sink: its code can make or use one where the checker cannot see", "write the code for the concrete type, without another package's type parameter")
				return
			}
		}
		key := ssaKey(callee)
		if m, ok := callee.Object().(*types.Func); ok && m.Signature().Recv() != nil && len(c.Args) > 0 {
			if k := s.kind(c.Args[0].Type()); k != "" && sinkPkg(m.Pkg().Path()) {
				if !allowedMethods[k][m.Name()] {
					s.fail(fn, ins, 2, key, m.Name()+" is not a method the kit allows on a "+k+" (allowed: "+methodList(k)+")", ssaFix)
					return
				}
				if res != nil && s.holds(res.Type()) && key != "net/http.Transport.Clone" {
					s.fail(fn, ins, 2, key, "a sink another package builds ("+key+")", ssaFix)
				}
				for _, a := range c.Args[1:] {
					if s.holds(a.Type()) {
						s.fail(fn, ins, 2, key, "a value holding a sink handed to "+key, ssaFix)
						return
					}
				}
				return
			}
		}
		switch key {
		case "os/exec.Command", "os/exec.CommandContext":
			return
		}
		if argHolds >= 0 {
			s.fail(fn, ins, 2, s.what(c.Args[argHolds].Type()), "a value holding a sink handed to "+key+", which the checker cannot see into", ssaFix)
			return
		}
		if resHolds {
			s.fail(fn, ins, 2, key, "a value holding a sink another package builds ("+key+")", ssaFix)
		}
	}
}

// localCallees: VTA resolves a dynamic call to one or more functions, all
// the connector's own (their bodies are checked like any other).
// noCallees: VTA finds no callee for the call ins.
func (s *ssaPass) noCallees(fn *ssa.Function, ins ssa.Instruction) bool {
	if n := s.cg.Nodes[fn]; n != nil {
		for _, e := range n.Out {
			if e.Site == ins {
				return false
			}
		}
	}
	return true
}

func (s *ssaPass) localCallees(fn *ssa.Function, ins ssa.Instruction) bool {
	n := s.cg.Nodes[fn]
	if n == nil {
		return false
	}
	found := false
	for _, e := range n.Out {
		if e.Site != ins {
			continue
		}
		if !s.localFn(e.Callee.Func) {
			return false
		}
		found = true
	}
	return found
}

// ---- before Start: the init region on the call graph ----

// region: every connector function an init function (with the package-level
// initializers it runs) calls, on the call graph of static callees plus VTA
// for dynamic calls. A function value or a connector value's methods handed
// to another package's call there count as called: that package may call
// them before it returns. In the region, any output, sink use or writer
// handed to another package fails unless it is the kit's call; a dynamic
// call whose callees VTA does not resolve fails.
func (s *ssaPass) region() {
	in := map[*ssa.Function]bool{}
	var work []*ssa.Function
	add := func(f *ssa.Function) {
		if f != nil && s.funs[f] && !in[f] {
			in[f] = true
			work = append(work, f)
		}
	}
	for _, pk := range s.prog.AllPackages() {
		if pk.Pkg != nil && s.p.local[pk.Pkg.Path()] {
			add(pk.Func("init"))
		}
	}
	// handedOn: what another package's call may call back into.
	handedOn := func(c *ssa.CallCommon) {
		for _, a := range c.Args {
			for {
				switch x := a.(type) {
				case *ssa.MakeInterface:
					ms := s.prog.MethodSets.MethodSet(x.X.Type())
					for i := 0; i < ms.Len(); i++ {
						add(s.prog.MethodValue(ms.At(i)))
					}
					a = x.X
					continue
				case *ssa.ChangeType:
					a = x.X
					continue
				case *ssa.ChangeInterface:
					a = x.X
					continue
				case *ssa.Function:
					add(x)
				case *ssa.MakeClosure:
					add(x.Fn.(*ssa.Function))
				}
				break
			}
		}
	}
	for len(work) > 0 {
		fn := work[len(work)-1]
		work = work[:len(work)-1]
		node := s.cg.Nodes[fn]
		for _, b := range fn.Blocks {
			for _, ins := range b.Instrs {
				ci, ok := ins.(ssa.CallInstruction)
				if !ok {
					continue
				}
				c := ci.Common()
				if callee := c.StaticCallee(); callee != nil {
					if s.funs[callee] {
						add(callee)
					} else {
						handedOn(c)
						s.preStart(fn, ins, c, callee)
					}
					continue
				}
				if _, ok := c.Value.(*ssa.Builtin); ok {
					s.preStart(fn, ins, c, nil)
					continue
				}
				var callees []*ssa.Function
				if node != nil {
					for _, e := range node.Out {
						if e.Site == ci {
							callees = append(callees, e.Callee.Func)
						}
					}
				}
				if len(callees) == 0 && !s.neverSet(c.Value) && !s.unreached(c) {
					s.fail(fn, ins, 2, "call:unresolved", "a dynamic call before credkit.Start (in an init function or a package-level initializer, or a function one of those calls) whose callees the checker cannot resolve", "make the call after Start, out of init and package-level initializers")
					continue
				}
				if c.IsInvoke() && writeMethods[c.Method.Name()] {
					s.preStart(fn, ins, c, nil)
				}
				for _, f := range callees {
					if s.funs[f] {
						add(f)
					} else {
						handedOn(c)
						s.preStart(fn, ins, c, f)
					}
				}
			}
		}
	}
}

// preStart: a call before Start to another package's function (or a
// builtin when callee is nil).
func (s *ssaPass) preStart(fn *ssa.Function, ins ssa.Instruction, c *ssa.CallCommon, callee *ssa.Function) {
	const msg = "output or a sink used before credkit.Start installs the output layer (this runs in an init function or a package-level initializer, or in a function one of those reaches)"
	const fix = "move it after Start, out of init and package-level initializers"
	if callee != nil && s.kitFn(callee) {
		return
	}
	key := ""
	switch {
	case callee != nil:
		key = ssaKey(callee)
	case c.IsInvoke():
		key = objKey(c.Method)
	default:
		if b, ok := c.Value.(*ssa.Builtin); ok {
			key = "builtin." + b.Name()
		}
	}
	out := writeCalls[key] || preStartSink[key] || key == "builtin.panic" || key == "database/sql.Open" || key == "database/sql.OpenDB" || execRun[key]
	if callee != nil && callee.Pkg != nil && callee.Pkg.Pkg.Path() == "log/slog" && slogOut[callee.Name()] {
		out = true
	}
	var recv types.Type
	if c.IsInvoke() {
		recv = c.Value.Type()
		out = out || writeMethods[c.Method.Name()]
	} else if callee != nil && callee.Signature.Recv() != nil && len(c.Args) > 0 {
		recv = c.Args[0].Type()
		out = out || (writeMethods[callee.Name()] && !inMemoryType(recv))
	}
	if recv != nil && !inMemoryType(recv) && s.p.writer != nil {
		impl := func(it *types.Interface) bool {
			return it != nil && (types.Implements(recv, it) || (!types.IsInterface(recv) && types.Implements(types.NewPointer(recv), it)))
		}
		out = out || impl(s.p.writer) || impl(s.p.slogHandler)
	}
	if out {
		s.fail(fn, ins, 2, key, msg, fix)
		return
	}
	switch key {
	case "log.New", "log.Logger.SetOutput", "log.SetOutput", "log/slog.NewTextHandler", "log/slog.NewJSONHandler", "builtin.append":
		return
	}
	args := c.Args
	if !c.IsInvoke() && callee != nil && callee.Signature.Recv() != nil && len(args) > 0 {
		args = args[1:]
	}
	for _, a := range args {
		t := a.Type()
		if mi, ok := a.(*ssa.MakeInterface); ok {
			t = mi.X.Type() // the value inside: an in-memory buffer stays one
		}
		if s.p.writer != nil && types.Implements(t, s.p.writer) && !inMemoryType(t) {
			s.fail(fn, ins, 2, "writer:"+types.TypeString(t, nil), "a writer handed to a call before credkit.Start installs the output layer, which may write to it", fix)
			return
		}
	}
}

// ---- sources: decode sites on SSA (ruling 13f) ----

// sourceDecoded follows each read a "source" ledger entry lists to the
// decoding calls it reaches and records the dynamic type of each destination
// (through conversions) with the decoder's format, beside what the AST
// follows: a conversion of the destination (json.Unmarshal(b,
// (*alias)(&cfg))) no longer hides the struct whose tags the decoder reads.
func (s *ssaPass) sourceDecoded(e entry) {
	for fn := range s.funs {
		for _, b := range fn.Blocks {
			for _, ins := range b.Instrs {
				c, ok := ins.(*ssa.Call)
				if !ok {
					continue
				}
				callee := c.Call.StaticCallee()
				if callee == nil || callee.Object() == nil || ssaKey(callee) != e.What {
					continue
				}
				f := s.finding(s.posOf(fn, c), 4, "", "", "")
				if f.File != e.File || f.Func != e.Func {
					continue
				}
				if callee.Name() == "DecodeFile" {
					if len(c.Call.Args) > 1 {
						s.decodeDest(c.Call.Args[1], callee.Object())
					}
					continue
				}
				s.followRead(c, map[ssa.Value]bool{})
			}
		}
	}
}

// followRead: every decoding call of another package a read's result (its
// bytes, its file, a reader or decoder made of them) reaches.
func (s *ssaPass) followRead(v ssa.Value, seen map[ssa.Value]bool) {
	if v == nil || seen[v] || v.Referrers() == nil {
		return
	}
	seen[v] = true
	for _, r := range *v.Referrers() {
		switch r := r.(type) {
		case *ssa.Extract:
			s.followRead(r, seen)
		case *ssa.MakeInterface, *ssa.ChangeType, *ssa.Convert, *ssa.ChangeInterface, *ssa.Phi:
			s.followRead(r.(ssa.Value), seen)
		case ssa.CallInstruction:
			c := r.Common()
			callee := c.StaticCallee()
			var dec types.Object
			if callee != nil {
				if s.localFn(callee) {
					continue
				}
				dec = callee.Object()
			} else if c.IsInvoke() {
				dec = c.Method
			}
			if dec == nil {
				continue
			}
			args := c.Args
			if !c.IsInvoke() && callee.Signature.Recv() != nil && len(args) > 0 {
				args = args[1:]
			}
			if (dec.Name() == "Decode" || strings.HasPrefix(dec.Name(), "Unmarshal")) && len(args) > 0 {
				s.decodeDest(args[len(args)-1], dec)
			}
			if cv, ok := r.(*ssa.Call); ok {
				s.followRead(cv, seen) // io.ReadAll, NewDecoder: their results carry the bytes on
			}
		}
	}
}

// decodeDest records the struct a decode destination points to, seen
// through every conversion, with the decoder's format.
func (s *ssaPass) decodeDest(v ssa.Value, dec types.Object) {
	for {
		switch x := v.(type) {
		case *ssa.MakeInterface:
			v = x.X
			continue
		case *ssa.ChangeType:
			v = x.X
			continue
		case *ssa.Convert:
			v = x.X
			continue
		case *ssa.ChangeInterface:
			v = x.X
			continue
		}
		break
	}
	pt, ok := v.Type().Underlying().(*types.Pointer)
	if !ok {
		return
	}
	root, ok := derefStruct(pt.Elem())
	if !ok {
		return
	}
	s.p.noteFormat(root, dec)
	for _, d := range s.p.decoded {
		if types.Identical(d.t, root) {
			return
		}
	}
	s.p.decoded = append(s.p.decoded, decoded{t: root})
}

// ---- reflect, unsafe, cgo, go:linkname, assembly (ruling 13g) ----

// marker is a site for a file the checker cannot read as Go source (loop ruling
// 15b): it fails closed unless the ledger names it with a reason.
func marker(file, what, msg string) Finding {
	return Finding{Rule: 2, File: file, Func: "file", What: what, Msg: msg, Site: true, Fix: "remove it, or list it in the ledger with its reason (the checker reads Go source only)"}
}

// binaries: the cli-relative path of every executable or object file in the
// connector's tree, git-ignored build output and the vendored kit aside. A file
// that cannot be read counts: it has not been shown not to be one.
func binaries(cli string) []string {
	var out []string
	_ = filepath.WalkDir(cli, func(path string, d fs.DirEntry, err error) error {
		rel := filepath.ToSlash(relTo(cli, path))
		if err != nil {
			out = append(out, rel)
			return nil
		}
		if d.IsDir() {
			if rel == "internal/credkit" || d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			out = append(out, rel)
			return nil
		}
		head := make([]byte, 64)
		n, _ := io.ReadFull(f, head)
		f.Close()
		if isBinary(head[:n]) && !gitIgnored(path) {
			out = append(out, rel)
		}
		return nil
	})
	sort.Strings(out)
	return out
}

// isBinary: the file starts like ELF, Mach-O, a fat binary, WebAssembly, a Go or
// Unix archive, or a Windows executable (text can start with MZ, so that one
// also needs a NUL byte).
func isBinary(head []byte) bool {
	for _, magic := range []string{"\x7fELF", "\xfe\xed\xfa\xce", "\xfe\xed\xfa\xcf", "\xce\xfa\xed\xfe", "\xcf\xfa\xed\xfe", "\xca\xfe\xba\xbe", "\x00asm", "!<arch>\n"} {
		if bytes.HasPrefix(head, []byte(magic)) {
			return true
		}
	}
	return bytes.HasPrefix(head, []byte("MZ")) && bytes.IndexByte(head, 0) >= 0
}

// gitIgnored: git says the path is ignored, which it never says of a tracked
// file. No git, or no repository, means it is not.
func gitIgnored(path string) bool {
	c := exec.Command("git", "check-ignore", "-q", "--", filepath.Base(path))
	c.Dir = filepath.Dir(path)
	return c.Run() == nil
}

// lowLevel: a connector file that uses cgo, go:linkname or assembly, or a
// binary in the connector's tree, fails unless the ledger names it. Every file
// of a connector package counts, those the build constraints leave out included.
func (p *pass) lowLevel(pkgs []*packages.Package) {
	for _, rel := range binaries(p.cli) {
		p.out = append(p.out, marker(rel, "binary", "a binary in the connector's tree, code the checker cannot read"))
	}
	// A connector type is trusted as built by connector code (an assertion
	// to it, its fields read as the connector's). That holds only while no
	// other package imports a connector package: one that does can build
	// the type where the checker cannot see.
	packages.Visit(pkgs, nil, func(pk *packages.Package) {
		if p.local[pk.PkgPath] || pk.PkgPath == p.kit {
			return
		}
		var paths []string
		for ip := range pk.Imports {
			if p.local[ip] {
				paths = append(paths, ip)
			}
		}
		sort.Strings(paths)
		for _, ip := range paths {
			p.out = append(p.out, Finding{Rule: 2, File: pk.PkgPath, Func: "package", What: "import:" + ip,
				Msg: "another package (" + pk.PkgPath + ") imports the connector's package " + ip + ", so it can build the connector's types (a client inside one among them) where the checker cannot see",
				Fix: "import connector packages only from connector packages"})
		}
	})
	seen := map[string]bool{}
	for _, pk := range pkgs {
		if pk.PkgPath == p.kit || !p.local[pk.PkgPath] {
			continue
		}
		var files []string
		files = append(files, pk.GoFiles...)
		files = append(files, pk.IgnoredFiles...)
		files = append(files, pk.OtherFiles...)
		for _, f := range files {
			if seen[f] {
				continue
			}
			seen[f] = true
			rel := filepath.ToSlash(relTo(p.cli, f))
			bad := func(what, msg string) { p.out = append(p.out, marker(rel, what, msg)) }
			switch strings.ToLower(filepath.Ext(f)) {
			case ".go":
			case ".s", ".sx":
				bad("assembly", "an assembly file, whose code the checker cannot read")
				continue
			case ".c", ".cc", ".cpp", ".cxx", ".m", ".h", ".hh", ".hpp", ".hxx", ".f", ".f90", ".for", ".swig", ".swigcxx", ".syso":
				bad("cgo", "a C, C++, Objective-C, Fortran, SWIG or object file, code the checker cannot read")
				continue
			default:
				continue
			}
			af, err := parser.ParseFile(token.NewFileSet(), f, nil, parser.ImportsOnly|parser.ParseComments)
			if err != nil {
				p.out = append(p.out, Finding{Rule: 2, File: rel, Func: "file", What: "unreadable", Msg: "a Go file the checker cannot parse: " + err.Error(), Fix: "make it parse"})
				continue
			}
			for _, im := range af.Imports {
				if im.Path.Value == `"C"` {
					bad("cgo", `a file that imports "C" (cgo), whose C code the checker cannot read`)
				}
			}
			for _, cg := range af.Comments {
				for _, c := range cg.List {
					if strings.HasPrefix(c.Text, "//go:linkname") {
						bad("go:linkname", "a go:linkname directive, which reaches another package's unexported code out of the checker's sight")
					}
				}
			}
		}
	}
}

// mergeSSA adds the SSA findings that the AST rules did not already report
// at the same place under the same name: one site, one finding.
func (p *pass) mergeSSA() {
	have := map[string]bool{}
	for _, f := range p.out {
		have[fmt.Sprintf("%d|%s|%d|%s", f.Rule, f.File, f.Line, f.What)] = true
	}
	for _, f := range p.ssaOut {
		k := fmt.Sprintf("%d|%s|%d|%s", f.Rule, f.File, f.Line, f.What)
		if have[k] {
			continue
		}
		have[k] = true
		p.out = append(p.out, f)
	}
}
