// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package main

import (
	"fmt"
	"go/constant"
	"go/token"
	"go/types"
	"sort"
	"strings"

	"golang.org/x/tools/go/ssa"
)

// The engine is a flow-insensitive taint analysis over the SSA form of the
// connector module.
//
// Memory is abstracted by OBJECT: an allocation site (a local variable,
// a composite literal, a make), a package-level variable, or a struct FIELD
// (every value of a struct type shares one object per field). A pointer
// shares the taint of the objects it may point to, so a load through any
// alias of a tainted object is tainted.
//
// Calls into module functions are summarised per callee, keyed by parameter
// (P(i)) and free variable (FV(k)): which parameters reach a return, a sink,
// an object, or the object behind another pointer parameter. A call site
// applies the summary to its own arguments, so a helper shared by many
// commands does not leak one command's flag into another's result.
// Calls into code outside the module follow the rules in specs.go.

type objKey any

// mapKeys is the object holding the keys of the map whose object is m. Keys
// are kept apart from values so a lookup by a tainted key does not taint the
// value it selects (choosing an entry is not naming a path).
type mapKeys struct{ m objKey }

type root struct {
	obj objKey // nil for a symbolic root
	sym int    // symbolic tag (P(i)/FV(k)) when obj is nil
}

type sinkSite struct {
	Sink string `json:"sink"`
	Pos  string `json:"pos"`
	Via  string `json:"via,omitempty"`
}

type summary struct {
	ret   []tagSet // per result index
	sink  map[int]*sinkSite
	store map[int]map[objKey]bool
	ptrw  map[int]*tagSet
	// ptrwWhole are writes through a pointer parameter made by an opaque
	// decoder (json.Unmarshal into it): they fill every field of the
	// pointee, not one value.
	ptrwWhole map[int]*tagSet
	// rootConds are os.Root operations whose root was opened at a path
	// built from this function's parameters: the name names a path (keeps
	// its lead tags) only at call sites where one of those parameters
	// carries taint. See confinedSink.
	rootConds map[string]*rootCond
}

// A rootCond is one os.Root operation still waiting on its root: recv holds
// the parameter tags the root's location came from, name the tags of the
// name the operation resolved inside it.
type rootCond struct {
	recv tagSet
	name tagSet
	site sinkSite
}

func newSummary(results int) *summary {
	if results < 1 {
		results = 1
	}
	return &summary{ret: make([]tagSet, results), sink: map[int]*sinkSite{}, store: map[int]map[objKey]bool{}, ptrw: map[int]*tagSet{}, ptrwWhole: map[int]*tagSet{}, rootConds: map[string]*rootCond{}}
}

func resultCount(sig *types.Signature) int {
	if n := sig.Results().Len(); n > 1 {
		return n
	}
	return 1
}

func unionAll(per []tagSet) tagSet {
	var t tagSet
	for _, p := range per {
		t.union(p)
	}
	return t
}

func broadcast(n int, t tagSet) []tagSet {
	out := make([]tagSet, n)
	for i := range out {
		out[i] = t.clone()
	}
	return out
}

func (s *summary) size() int {
	n := len(s.sink)
	for _, r := range s.ret {
		n += r.count()
	}
	for _, m := range s.store {
		n += len(m)
	}
	for _, t := range s.ptrw {
		n += t.count()
	}
	for _, t := range s.ptrwWhole {
		n += t.count()
	}
	for _, c := range s.rootConds {
		n += 1 + c.name.count()
	}
	return n
}

type engine struct {
	prog    *ssa.Program
	fset    *token.FileSet
	inMod   map[*ssa.Package]bool
	modPkgs map[string]bool
	funcs   []*ssa.Function
	sums    map[*ssa.Function]*summary
	objTags map[objKey]*tagSet
	// seeds are global tags a function's parameter or free variable carries
	// on entry (a flag Value's Set method, a cobra RunE's positional args,
	// an MCP tool handler's request).
	paramSeeds map[*ssa.Function]map[int]*tagSet
	// fvTags are the global tags each closure's free variables were bound to
	// at its MakeClosure sites.
	fvTags     map[*ssa.Function]map[int]*tagSet
	closures   map[*ssa.Function][]*ssa.MakeClosure
	rootMemo   map[ssa.Value][]root
	rootBusy   map[ssa.Value]bool
	findings   map[int][]sinkSite
	sources    []*source
	sourceAt   map[string]int
	modTypes   []types.Type
	invokeMemo map[string][]*ssa.Function
	bySig      map[string][]*ssa.Function
	changed    bool
	converged  bool
	// explainID is the source whose object writes are recorded in why
	// (debugging only; -1 when off).
	explainID int
	why       map[objKey]string
	// Worklist: a function is re-analysed only when something it read
	// changed - an object it loaded, a callee summary it applied, or its own
	// seeds.
	readers map[objKey]map[*ssa.Function]bool
	users   map[*ssa.Function]map[*ssa.Function]bool
	queue   []*ssa.Function
	queued  map[*ssa.Function]bool
	// sinkSeen holds, per sink site, the global tags already recorded there.
	sinkSeen map[string]*tagSet
	// order is each function's position in funcs, for batching the worklist.
	order map[*ssa.Function]int
}

func newEngine(prog *ssa.Program, inMod map[*ssa.Package]bool, funcs []*ssa.Function) *engine {
	e := &engine{
		prog: prog, fset: prog.Fset, inMod: inMod, funcs: funcs,
		sums:       map[*ssa.Function]*summary{},
		objTags:    map[objKey]*tagSet{},
		paramSeeds: map[*ssa.Function]map[int]*tagSet{},
		fvTags:     map[*ssa.Function]map[int]*tagSet{},
		closures:   map[*ssa.Function][]*ssa.MakeClosure{},
		rootMemo:   map[ssa.Value][]root{},
		rootBusy:   map[ssa.Value]bool{},
		findings:   map[int][]sinkSite{},
		sourceAt:   map[string]int{},
		invokeMemo: map[string][]*ssa.Function{},
		bySig:      map[string][]*ssa.Function{},
		explainID:  -1,
		why:        map[objKey]string{},
		readers:    map[objKey]map[*ssa.Function]bool{},
		users:      map[*ssa.Function]map[*ssa.Function]bool{},
		queued:     map[*ssa.Function]bool{},
		sinkSeen:   map[string]*tagSet{},
		order:      map[*ssa.Function]int{},
	}
	for i, fn := range funcs {
		e.order[fn] = i
	}
	for _, fn := range funcs {
		e.sums[fn] = newSummary(resultCount(fn.Signature))
		for _, b := range fn.Blocks {
			for _, instr := range b.Instrs {
				if mc, ok := instr.(*ssa.MakeClosure); ok {
					if f, ok := mc.Fn.(*ssa.Function); ok {
						e.closures[f] = append(e.closures[f], mc)
					}
				}
			}
		}
	}
	e.modPkgs = map[string]bool{}
	for p := range inMod {
		e.modPkgs[p.Pkg.Path()] = true
	}
	e.indexTypes()
	e.indexAddressTaken()
	return e
}

func (e *engine) pos(p token.Pos) string {
	if !p.IsValid() {
		return ""
	}
	position := e.fset.Position(p)
	return fmt.Sprintf("%s:%d", position.Filename, position.Line)
}

// markDirty queues fn for (re-)analysis.
func (e *engine) markDirty(fn *ssa.Function) {
	if fn == nil || e.queued[fn] || !e.hasBody(fn) {
		return
	}
	e.queued[fn] = true
	e.queue = append(e.queue, fn)
}

// touchObj queues every function that has read object k.
func (e *engine) touchObj(k objKey) {
	for fn := range e.readers[k] {
		e.markDirty(fn)
	}
}

// readObj returns object k's taint and records that st's function read it.
func (st *fnState) readObj(k objKey) *tagSet {
	m := st.e.readers[k]
	if m == nil {
		m = map[*ssa.Function]bool{}
		st.e.readers[k] = m
	}
	m[st.fn] = true
	return st.e.obj(k)
}

// useSummary records that st's function applied callee's summary.
func (st *fnState) useSummary(callee *ssa.Function) {
	m := st.e.users[callee]
	if m == nil {
		m = map[*ssa.Function]bool{}
		st.e.users[callee] = m
	}
	m[st.fn] = true
}

func (e *engine) obj(k objKey) *tagSet {
	t := e.objTags[k]
	if t == nil {
		t = &tagSet{}
		e.objTags[k] = t
	}
	return t
}

func (e *engine) hasBody(fn *ssa.Function) bool {
	if fn == nil || fn.Blocks == nil {
		return false
	}
	_, ok := e.sums[fn]
	return ok
}

// indexTypes collects every named type declared in the module, for resolving
// interface method calls to module implementations.
func (e *engine) indexTypes() {
	for pkg := range e.inMod {
		for _, m := range pkg.Members {
			t, ok := m.(*ssa.Type)
			if !ok {
				continue
			}
			named, ok := t.Type().(*types.Named)
			if !ok || named.TypeParams().Len() > 0 {
				continue
			}
			if _, isIface := named.Underlying().(*types.Interface); isIface {
				continue
			}
			e.modTypes = append(e.modTypes, named, types.NewPointer(named))
		}
	}
}

// indexAddressTaken records module functions used as values (not only
// called), by signature, for resolving calls through function values.
func (e *engine) indexAddressTaken() {
	seen := map[*ssa.Function]bool{}
	add := func(f *ssa.Function) {
		if f == nil || seen[f] || !e.hasBody(f) {
			return
		}
		seen[f] = true
		k := sigKey(f.Signature)
		e.bySig[k] = append(e.bySig[k], f)
	}
	for _, fn := range e.funcs {
		for _, b := range fn.Blocks {
			for _, instr := range b.Instrs {
				var ops []*ssa.Value
				if c, ok := instr.(ssa.CallInstruction); ok {
					common := c.Common()
					if !common.IsInvoke() {
						// The callee position is a call, not an address-take.
						for i := range common.Args {
							ops = append(ops, &common.Args[i])
						}
					} else {
						ops = instr.Operands(nil)
					}
				} else {
					ops = instr.Operands(nil)
				}
				for _, op := range ops {
					if op == nil || *op == nil {
						continue
					}
					switch v := (*op).(type) {
					case *ssa.Function:
						add(v)
					case *ssa.MakeClosure:
						if f, ok := v.Fn.(*ssa.Function); ok {
							add(f)
						}
					}
				}
				if mc, ok := instr.(*ssa.MakeClosure); ok {
					if f, ok := mc.Fn.(*ssa.Function); ok {
						add(f)
					}
				}
			}
		}
	}
}

func sigKey(sig *types.Signature) string {
	// Drop the receiver: a method value has the method's parameters.
	return types.TypeString(types.NewSignatureType(nil, nil, nil, sig.Params(), sig.Results(), sig.Variadic()), nil)
}

// roots returns the objects (or symbolic parameters) a pointer-like value
// may refer to. It is structural and independent of taint.
func (e *engine) roots(v ssa.Value) []root {
	if r, ok := e.rootMemo[v]; ok {
		return r
	}
	if e.rootBusy[v] {
		return nil
	}
	e.rootBusy[v] = true
	var out []root
	add := func(rs []root) { out = append(out, rs...) }
	switch x := v.(type) {
	case *ssa.Alloc, *ssa.Global, *ssa.MakeMap, *ssa.MakeSlice, *ssa.MakeChan, *ssa.MakeClosure:
		out = []root{{obj: v}}
	case *ssa.FieldAddr:
		out = []root{{obj: fieldObj(x.X.Type(), x.Field, v)}}
	case *ssa.Field:
		out = []root{{obj: fieldObj(x.X.Type(), x.Field, v)}}
		add(e.roots(x.X))
	case *ssa.IndexAddr:
		add(e.roots(x.X))
	case *ssa.Index:
		add(e.roots(x.X))
	case *ssa.Slice:
		add(e.roots(x.X))
	case *ssa.Lookup:
		add(e.roots(x.X))
	case *ssa.Phi:
		for _, ed := range x.Edges {
			add(e.roots(ed))
		}
	case *ssa.ChangeType:
		add(e.roots(x.X))
	case *ssa.Convert:
		add(e.roots(x.X))
	case *ssa.MultiConvert:
		add(e.roots(x.X))
	case *ssa.ChangeInterface:
		add(e.roots(x.X))
	case *ssa.SliceToArrayPointer:
		add(e.roots(x.X))
	case *ssa.MakeInterface:
		add(e.roots(x.X))
	case *ssa.TypeAssert:
		add(e.roots(x.X))
	case *ssa.Extract:
		add(e.roots(x.Tuple))
	case *ssa.UnOp:
		// A pointer loaded from memory is treated as pointing back at the
		// memory it was loaded from: indirection levels collapse.
		add(e.roots(x.X))
	case *ssa.Range:
		add(e.roots(x.X))
	case *ssa.Next:
		add(e.roots(x.Iter))
	case *ssa.Parameter:
		fn := x.Parent()
		for i, p := range fn.Params {
			if p == x {
				out = []root{{sym: paramTag(i)}}
			}
		}
	case *ssa.FreeVar:
		fn := x.Parent()
		for k, fv := range fn.FreeVars {
			if fv != x {
				continue
			}
			out = []root{{sym: freeVarTag(k)}}
			for _, mc := range e.closures[fn] {
				if k < len(mc.Bindings) {
					for _, r := range e.roots(mc.Bindings[k]) {
						if r.obj != nil {
							out = append(out, r)
						}
					}
				}
			}
		}
	case *ssa.Call:
		out = []root{{obj: v}}
		common := x.Common()
		if !common.IsInvoke() {
			if b, ok := common.Value.(*ssa.Builtin); ok {
				switch b.Name() {
				case "append", "ssa:wrapnilchk":
					if len(common.Args) > 0 {
						add(e.roots(common.Args[0]))
					}
				}
			}
		}
	case *ssa.Const, *ssa.Function, *ssa.Builtin:
		out = nil
	default:
		out = []root{{obj: v}}
	}
	delete(e.rootBusy, v)
	e.rootMemo[v] = out
	return out
}

// objType is the type of the memory an object denotes.
func objType(k objKey) types.Type {
	switch x := k.(type) {
	case *types.Var:
		return x.Type()
	case *ssa.Alloc:
		if p, ok := x.Type().Underlying().(*types.Pointer); ok {
			return p.Elem()
		}
	case ssa.Value:
		return x.Type()
	}
	return nil
}

// structFields lists the fields of a struct or pointer-to-struct type.
func structFields(t types.Type) []*types.Var {
	if t == nil {
		return nil
	}
	if p, ok := t.Underlying().(*types.Pointer); ok {
		t = p.Elem()
	}
	s, ok := t.Underlying().(*types.Struct)
	if !ok {
		return nil
	}
	out := make([]*types.Var, s.NumFields())
	for i := range out {
		out[i] = s.Field(i)
	}
	return out
}

// fieldObj names the object for field i of the struct that ptrOrStruct
// (a struct or a pointer to one) denotes. Every instance of a struct type
// shares one object per field.
func fieldObj(ptrOrStruct types.Type, i int, fallback ssa.Value) objKey {
	t := ptrOrStruct
	if p, ok := t.Underlying().(*types.Pointer); ok {
		t = p.Elem()
	}
	if s, ok := t.Underlying().(*types.Struct); ok && i < s.NumFields() {
		f := s.Field(i)
		return f.Origin()
	}
	return fallback
}

// ---------------------------------------------------------------------------
// Per-function analysis

type fnState struct {
	e      *engine
	fn     *ssa.Function
	sum    *summary
	tags   map[ssa.Value]*tagSet
	tuple  map[ssa.Value][]tagSet
	symObj map[int]*tagSet
	local  bool // a value's tags grew during this pass
	cur    ssa.Instruction
}

func (e *engine) analyze(fn *ssa.Function) {
	st := &fnState{e: e, fn: fn, sum: e.sums[fn], tags: map[ssa.Value]*tagSet{}, tuple: map[ssa.Value][]tagSet{}, symObj: map[int]*tagSet{}}
	for i, p := range fn.Params {
		t := st.get(p)
		t.add(paramTag(i))
		if seeds := e.paramSeeds[fn]; seeds != nil && seeds[i] != nil {
			t.union(*seeds[i])
		}
	}
	for k, fv := range fn.FreeVars {
		t := st.get(fv)
		t.add(freeVarTag(k))
		if b := e.fvTags[fn]; b != nil && b[k] != nil {
			t.union(*b[k])
		}
	}
	before := st.sum.size()
	for iter := 0; iter < 50; iter++ {
		st.local = false
		for _, b := range fn.Blocks {
			for _, instr := range b.Instrs {
				st.cur = instr
				st.transfer(instr)
			}
		}
		if !st.local {
			break
		}
	}
	if st.sum.size() != before {
		e.changed = true
		for user := range e.users[fn] {
			e.markDirty(user)
		}
	}
}

func (st *fnState) get(v ssa.Value) *tagSet {
	t := st.tags[v]
	if t == nil {
		t = &tagSet{}
		st.tags[v] = t
	}
	return t
}

// tagsOf is the taint a value carries in this function.
func (st *fnState) tagsOf(v ssa.Value) tagSet {
	switch x := v.(type) {
	case nil, *ssa.Const, *ssa.Function, *ssa.Builtin:
		return tagSet{}
	case *ssa.Global:
		return st.readObj(x).clone()
	case *ssa.FreeVar:
		// A captured pointer also shares the taint of the objects its
		// bindings point to.
		t := st.get(x).clone()
		for _, r := range st.e.roots(x) {
			if r.obj != nil {
				t.unionGlobal(*st.readObj(r.obj))
			}
		}
		return t
	}
	if t := st.tags[v]; t != nil {
		return *t
	}
	return tagSet{}
}

func (st *fnState) set(v ssa.Value, t tagSet) {
	if t.empty() {
		return
	}
	if st.get(v).union(t) {
		st.local = true
	}
}

// load is the taint read through a pointer-like value.
func (st *fnState) load(addr ssa.Value) tagSet {
	t := st.tagsOf(addr).clone()
	for _, r := range st.e.roots(addr) {
		if r.obj != nil {
			t.union(*st.readObj(r.obj))
			continue
		}
		t.add(r.sym)
		if so := st.symObj[r.sym]; so != nil {
			t.union(*so)
		}
	}
	return t
}

func (st *fnState) storeObj(k objKey, t tagSet) {
	st.storeObjDepth(k, t, -1)
}

// storeObjWhole writes t into object k as an opaque decoder does: when k is
// a struct, every field object of that struct type receives t too, so a
// field read later (which consults only the field's object) sees it.
func (st *fnState) storeObjWhole(k objKey, t tagSet) {
	st.storeObjDepth(k, t, 0)
}

// storeObjDepth writes t into object k; a non-negative depth also writes
// the field objects of k's struct type, down to three levels.
func (st *fnState) storeObjDepth(k objKey, t tagSet, depth int) {
	if t.empty() {
		return
	}
	if depth >= 0 && depth < 3 {
		for _, f := range structFields(objType(k)) {
			st.storeObjDepth(f.Origin(), t, depth+1)
		}
	}
	if st.e.obj(k).unionGlobal(t) {
		st.e.changed = true
		st.e.touchObj(k)
		if st.e.explainID >= 0 && t.has(st.e.explainID) {
			if _, ok := st.e.why[k]; !ok {
				pos := ""
				if st.cur != nil {
					pos = st.e.pos(st.cur.Pos()) + " " + st.cur.String()
				}
				st.e.why[k] = st.fn.String() + " @ " + pos
			}
		}
	}
	t.eachSymbolic(func(tag int) {
		m := st.sum.store[tag]
		if m == nil {
			m = map[objKey]bool{}
			st.sum.store[tag] = m
		}
		if !m[k] {
			m[k] = true
			st.local = true
		}
	})
}

// storeInto writes t into everything addr may point to.
func (st *fnState) storeInto(addr ssa.Value, t tagSet) {
	st.storeIntoKind(addr, t, false)
}

// storeIntoWhole is storeInto for an opaque decoder's write: a struct
// pointee has every field written.
func (st *fnState) storeIntoWhole(addr ssa.Value, t tagSet) {
	st.storeIntoKind(addr, t, true)
}

func (st *fnState) storeIntoKind(addr ssa.Value, t tagSet, whole bool) {
	if t.empty() {
		return
	}
	for _, r := range st.e.roots(addr) {
		if r.obj != nil {
			if whole {
				st.storeObjWhole(r.obj, t)
			} else {
				st.storeObj(r.obj, t)
			}
			continue
		}
		so := st.symObj[r.sym]
		if so == nil {
			so = &tagSet{}
			st.symObj[r.sym] = so
		}
		if so.union(t) {
			st.local = true
		}
		writes := st.sum.ptrw
		if whole {
			writes = st.sum.ptrwWhole
		}
		pw := writes[r.sym]
		if pw == nil {
			pw = &tagSet{}
			writes[r.sym] = pw
		}
		if pw.union(t) {
			st.local = true
		}
	}
}

func (st *fnState) recordSink(t tagSet, site sinkSite) {
	key := site.Sink + "|" + site.Pos + "|" + site.Via
	seen := st.e.sinkSeen[key]
	if seen == nil {
		seen = &tagSet{}
		st.e.sinkSeen[key] = seen
	}
	fresh := t.globalMinus(*seen)
	seen.unionGlobal(fresh)
	fresh.each(func(tag int) { st.e.addFinding(tag, site) })
	t.eachSymbolic(func(tag int) {
		if _, ok := st.sum.sink[tag]; !ok {
			s := site
			st.sum.sink[tag] = &s
			st.local = true
		}
	})
}

// confinedSink records a name that an os.Root operation resolves inside its
// root, whose location carries taint recv. os.Root refuses a name that
// leaves the root (an absolute path, a ".." hop, an escaping symlink), so the
// name alone only chooses an entry inside the root: its component form always
// reaches the sink. The name chooses a host path (its lead tags reach the
// sink) only when the root is not confined to a location the caller cannot
// steer, that is, when any caller-chosen value reached the root's own path:
//
//   - recv is empty (a constant, the operator's environment): confined;
//   - recv carries a global tag: the root is steered here, so the name keeps
//     its lead tags;
//   - recv carries only parameter tags: the answer depends on the caller, so
//     the operation becomes a summary entry that applySummary resolves at each
//     call site with that call's arguments.
//
// Any other shape (a free variable among recv or name tags) is resolved
// conservatively as steered: it can only add findings.
func (st *fnState) confinedSink(recv, name tagSet, site sinkSite) {
	if name.empty() {
		return
	}
	st.recordSink(name.demoted(), site)
	switch {
	case recv.empty():
	case recv.hasGlobal() || !onlyParamTags(recv) || !onlyParamTags(name):
		st.recordSink(name, site)
	default:
		st.addRootCond(recv, name, site)
	}
}

// onlyParamTags reports whether every symbolic tag of t is a parameter tag.
func onlyParamTags(t tagSet) bool {
	ok := true
	t.eachSymbolic(func(tag int) {
		if !isParamTag(tag) {
			ok = false
		}
	})
	return ok
}

func (st *fnState) addRootCond(recv, name tagSet, site sinkSite) {
	w := recv.w
	for len(w) > 0 && w[len(w)-1] == 0 {
		w = w[:len(w)-1]
	}
	key := fmt.Sprintf("%s|%s|%s|%x", site.Sink, site.Pos, site.Via, w)
	c := st.sum.rootConds[key]
	if c == nil {
		c = &rootCond{recv: recv.clone(), site: site}
		st.sum.rootConds[key] = c
		st.local = true
	}
	if c.name.union(name) {
		st.local = true
	}
}

func (e *engine) addFinding(tag int, site sinkSite) {
	list := e.findings[tag]
	for _, s := range list {
		if s.Sink == site.Sink && s.Pos == site.Pos {
			return
		}
	}
	if len(list) >= 4 {
		return
	}
	e.findings[tag] = append(list, site)
	e.changed = true
}

func (st *fnState) transfer(instr ssa.Instruction) {
	switch x := instr.(type) {
	case *ssa.Alloc:
		st.set(x, *st.readObj(x))
	case *ssa.FieldAddr:
		st.set(x, st.fieldTags(x.X, x.Field, x))
	case *ssa.Field:
		st.set(x, st.fieldTags(x.X, x.Field, x))
	case *ssa.IndexAddr:
		st.set(x, st.tagsOf(x.X))
	case *ssa.Index:
		st.set(x, st.load(x.X))
	case *ssa.Lookup:
		st.set(x, st.load(x.X))
	case *ssa.UnOp:
		if x.Op == token.MUL || x.Op == token.ARROW {
			st.set(x, st.load(x.X))
		} else {
			st.set(x, st.tagsOf(x.X))
		}
	case *ssa.BinOp:
		t := st.tagsOf(x.X).clone()
		yt := st.tagsOf(x.Y)
		if x.Op == token.ADD && isStringType(x.Type()) && !leadingPrefix(x.X) {
			// The right operand follows text the left one supplied: it can
			// no longer choose where the path starts.
			yt = yt.demoted()
		}
		t.union(yt)
		st.set(x, t)
	case *ssa.Phi:
		var t tagSet
		for _, ed := range x.Edges {
			t.union(st.tagsOf(ed))
		}
		st.set(x, t)
	case *ssa.ChangeType:
		st.set(x, st.tagsOf(x.X))
		st.checkHTTPDir(x, x.X)
	case *ssa.Convert:
		st.set(x, st.tagsOf(x.X))
		st.checkHTTPDir(x, x.X)
	case *ssa.MultiConvert:
		st.set(x, st.tagsOf(x.X))
	case *ssa.ChangeInterface:
		st.set(x, st.tagsOf(x.X))
	case *ssa.SliceToArrayPointer:
		st.set(x, st.tagsOf(x.X))
	case *ssa.MakeInterface:
		st.set(x, st.tagsOf(x.X))
	case *ssa.TypeAssert:
		st.set(x, st.tagsOf(x.X))
	case *ssa.Extract:
		if per, ok := st.tuple[x.Tuple]; ok && x.Index < len(per) {
			st.set(x, per[x.Index])
		} else {
			st.set(x, st.tagsOf(x.Tuple))
		}
	case *ssa.Slice:
		st.set(x, st.tagsOf(x.X))
	case *ssa.Range:
		st.set(x, st.load(x.X))
	case *ssa.Next:
		t := st.tagsOf(x.Iter).clone()
		if rng, ok := x.Iter.(*ssa.Range); ok {
			for _, r := range st.e.roots(rng.X) {
				if r.obj != nil {
					t.union(*st.readObj(mapKeys{r.obj}))
				}
			}
		}
		st.set(x, t)
	case *ssa.Select:
		var t tagSet
		for _, s := range x.States {
			if s.Dir == types.RecvOnly {
				t.union(st.load(s.Chan))
			}
		}
		st.set(x, t)
	case *ssa.MakeMap:
		st.set(x, *st.readObj(x))
	case *ssa.MakeSlice:
		st.set(x, *st.readObj(x))
	case *ssa.MakeChan:
		st.set(x, *st.readObj(x))
	case *ssa.MakeClosure:
		st.set(x, *st.readObj(x))
		st.makeClosure(x)
	case *ssa.Store:
		st.storeInto(x.Addr, st.tagsOf(x.Val))
		st.e.noteCommandField(st.fn, x)
		st.checkExecCmdField(x)
	case *ssa.MapUpdate:
		st.storeInto(x.Map, st.tagsOf(x.Value))
		if kt := st.tagsOf(x.Key); !kt.empty() {
			for _, r := range st.e.roots(x.Map) {
				if r.obj != nil {
					st.storeObj(mapKeys{r.obj}, kt)
				} else {
					// A map reached through a parameter: keep the key with the
					// values so the caller still sees it.
					st.storeInto(x.Map, kt)
				}
			}
		}
	case *ssa.Send:
		st.storeInto(x.Chan, st.tagsOf(x.X))
	case *ssa.Return:
		for i, r := range x.Results {
			if i >= len(st.sum.ret) {
				i = len(st.sum.ret) - 1
			}
			if st.sum.ret[i].union(st.tagsOf(r)) {
				st.local = true
			}
		}
	case *ssa.Call:
		per := st.call(x, x.Common())
		st.set(x, unionAll(per))
		if len(per) > 1 {
			cur := st.tuple[x]
			if cur == nil {
				cur = make([]tagSet, len(per))
				st.tuple[x] = cur
			}
			for i := range per {
				if cur[i].union(per[i]) {
					st.local = true
				}
			}
		}
	case *ssa.Go:
		st.call(x, x.Common())
	case *ssa.Defer:
		st.call(x, x.Common())
	}
}

// fieldTags is the taint of field i read from base. A field of a struct
// type declared in the module carries exactly its field object's taint:
// every write to it is a store the analysis sees, and a write into the whole
// struct (a decode) is copied into each field object (storeObjDepth). The
// struct value's own taint is NOT inherited, because one shared type would
// then tie every input to every field: a request body held as `any` and type
// asserted to the multipart shape would carry every flag of every endpoint
// into its file-upload field. A struct declared outside the module (url.URL,
// an SDK type) is filled by code the analysis does not see, so its fields
// inherit the global taint of the value they are read from.
func (st *fnState) fieldTags(base ssa.Value, i int, v ssa.Value) tagSet {
	k := fieldObj(base.Type(), i, v)
	t := st.readObj(k).clone()
	if f, ok := k.(*types.Var); !ok || f.Pkg() == nil || !st.e.modPkgs[f.Pkg().Path()] {
		t.unionGlobal(st.tagsOf(base))
	}
	return t
}

// filteredCharWrite reports whether a one-character builder write
// (strings.Builder / bytes.Buffer WriteRune or WriteByte) is an allowlist
// filter: the CFG is simulated from where the character is defined with each
// of '/', '\\', '.' and NUL in turn, deciding every branch that compares the
// character with a constant, and the write is a filter only when none of them
// can reach it. A raw copy (`for _, r := range s { b.WriteRune(r) }`) writes
// unconditionally and stays tainted; client.cacheResourceTag, which keeps only
// [a-z0-9_-], is recognised and does not taint the cache directory it names.
func filteredCharWrite(callee *ssa.Function, args []ssa.Value, instr ssa.CallInstruction) bool {
	fnObj, ok := callee.Object().(*types.Func)
	if !ok || fnObj == nil || len(args) < 2 {
		return false
	}
	switch _, key := funcKey(fnObj); key {
	case "strings.Builder.WriteRune", "strings.Builder.WriteByte", "bytes.Buffer.WriteRune", "bytes.Buffer.WriteByte":
	default:
		return false
	}
	ch := args[1]
	target := instr.Block()
	start := definingBlock(ch)
	if target == nil || start == nil {
		return false
	}
	for _, forbidden := range []int64{'/', '\\', '.', 0} {
		if charReaches(start, target, ch, forbidden) {
			return false
		}
	}
	return true
}

func definingBlock(v ssa.Value) *ssa.BasicBlock {
	if in, ok := stripChangeType(v).(ssa.Instruction); ok {
		return in.Block()
	}
	return nil
}

// stripChangeType sees through a named-type change only. A numeric Convert is
// NOT stripped: byte(r) narrows, so a rune above 127 the filter let through can
// become '/', and the compared value and the written value must stay distinct.
func stripChangeType(v ssa.Value) ssa.Value {
	for {
		ct, ok := v.(*ssa.ChangeType)
		if !ok {
			return v
		}
		v = ct.X
	}
}

// sameChar reports whether x is the written character ch itself.
func sameChar(x, ch ssa.Value) bool {
	return stripChangeType(x) == stripChangeType(ch)
}

// charReaches simulates the CFG from start with the character fixed to c and
// reports whether target is reachable. A branch whose condition compares the
// character with a constant follows only the side the comparison takes; any
// other branch follows both.
func charReaches(start, target *ssa.BasicBlock, ch ssa.Value, c int64) bool {
	seen := map[*ssa.BasicBlock]bool{}
	stack := []*ssa.BasicBlock{start}
	for len(stack) > 0 {
		b := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if b == target {
			return true
		}
		if seen[b] {
			continue
		}
		seen[b] = true
		if len(b.Instrs) == 0 {
			continue
		}
		if ifInstr, ok := b.Instrs[len(b.Instrs)-1].(*ssa.If); ok && len(b.Succs) == 2 {
			if val, known := evalCharCond(ifInstr.Cond, ch, c); known {
				if val {
					stack = append(stack, b.Succs[0])
				} else {
					stack = append(stack, b.Succs[1])
				}
				continue
			}
		}
		stack = append(stack, b.Succs...)
	}
	return false
}

func evalCharCond(cond, ch ssa.Value, c int64) (bool, bool) {
	if u, ok := cond.(*ssa.UnOp); ok && u.Op == token.NOT {
		v, known := evalCharCond(u.X, ch, c)
		return !v, known
	}
	bin, ok := cond.(*ssa.BinOp)
	if !ok {
		return false, false
	}
	var k int64
	op := bin.Op
	switch {
	case sameChar(bin.X, ch):
		cst, isConst := bin.Y.(*ssa.Const)
		if !isConst || cst.Value == nil || cst.Value.Kind() != constant.Int {
			return false, false
		}
		k, _ = constant.Int64Val(cst.Value)
	case sameChar(bin.Y, ch):
		cst, isConst := bin.X.(*ssa.Const)
		if !isConst || cst.Value == nil || cst.Value.Kind() != constant.Int {
			return false, false
		}
		k, _ = constant.Int64Val(cst.Value)
		// k OP ch  ==  ch OP' k
		switch op {
		case token.LSS:
			op = token.GTR
		case token.LEQ:
			op = token.GEQ
		case token.GTR:
			op = token.LSS
		case token.GEQ:
			op = token.LEQ
		}
	default:
		return false, false
	}
	switch op {
	case token.EQL:
		return c == k, true
	case token.NEQ:
		return c != k, true
	case token.LSS:
		return c < k, true
	case token.LEQ:
		return c <= k, true
	case token.GTR:
		return c > k, true
	case token.GEQ:
		return c >= k, true
	}
	return false, false
}

// pathBuilder models the standard functions that assemble a path from
// parts, keeping only the first part's lead tags and demoting every later
// part to a component. A shape it cannot read (a slice built at run time, a
// format that is not a constant) keeps every part's lead tags: unknown
// structure never hides a flag.
func (st *fnState) pathBuilder(callee *ssa.Function, args []ssa.Value) (tagSet, bool) {
	fnObj, ok := callee.Object().(*types.Func)
	if !ok || fnObj == nil {
		return tagSet{}, false
	}
	_, key := funcKey(fnObj)
	switch key {
	case "path/filepath.Join", "path.Join", "fmt.Sprint":
		if len(args) != 1 {
			return tagSet{}, false
		}
		return st.headAndRest(args[0], true), true
	case "strings.Join":
		if len(args) != 2 {
			return tagSet{}, false
		}
		sep, ok := constString(args[1])
		return st.headAndRest(args[0], ok && strings.ContainsAny(sep, `/\`)), true
	case "fmt.Sprintf":
		if len(args) != 2 {
			return tagSet{}, false
		}
		format, ok := constString(args[0])
		if !ok || strings.Contains(format, "%[") {
			t := st.tagsOf(args[0]).clone()
			t.union(st.tagsOf(args[1]))
			t.union(st.load(args[1]))
			return t, true
		}
		return st.headAndRest(args[1], firstVerbLeads(format)), true
	}
	return tagSet{}, false
}

// headAndRest is the taint of a value built from the elements of slice v:
// element 0 keeps its lead tags when headLeads is set, everything else is a
// component. When the slice is not a literal this function can read, every
// element keeps its tags.
func (st *fnState) headAndRest(v ssa.Value, headLeads bool) tagSet {
	all := st.tagsOf(v).clone()
	all.union(st.load(v))
	head, ok := st.sliceHead(v)
	if !ok {
		return all
	}
	out := all.demoted()
	if headLeads {
		out.union(head)
	}
	return out
}

// sliceHead returns the taint stored into element 0 of a slice literal (or
// variadic argument array) built in this function, possibly extended by
// append. ok is false when the slice's elements are not all stored at
// constant indexes here.
func (st *fnState) sliceHead(v ssa.Value) (tagSet, bool) {
	if c, isCall := v.(*ssa.Call); isCall {
		if b, isBuiltin := c.Call.Value.(*ssa.Builtin); isBuiltin && b.Name() == "append" && len(c.Call.Args) > 0 {
			return st.sliceHead(c.Call.Args[0])
		}
		return tagSet{}, false
	}
	sl, isSlice := v.(*ssa.Slice)
	if !isSlice {
		return tagSet{}, false
	}
	alloc, isAlloc := sl.X.(*ssa.Alloc)
	if !isAlloc || alloc.Referrers() == nil {
		return tagSet{}, false
	}
	var head tagSet
	for _, ref := range *alloc.Referrers() {
		switch r := ref.(type) {
		case *ssa.IndexAddr:
			c, isConst := r.Index.(*ssa.Const)
			if !isConst || r.Referrers() == nil {
				return tagSet{}, false
			}
			idx, exact := constant.Int64Val(constant.ToInt(c.Value))
			if !exact {
				return tagSet{}, false
			}
			for _, r2 := range *r.Referrers() {
				if store, isStore := r2.(*ssa.Store); isStore && store.Addr == r && idx == 0 {
					head.union(st.tagsOf(store.Val))
				}
			}
		case *ssa.Slice, *ssa.DebugRef:
		default:
			return tagSet{}, false
		}
	}
	return head, true
}

// firstVerbLeads reports whether the first formatting verb of a constant
// format string starts the result (nothing, or only a URI scheme, before it).
func firstVerbLeads(format string) bool {
	for i := 0; i < len(format); i++ {
		if format[i] != '%' {
			continue
		}
		if i+1 < len(format) && format[i+1] == '%' {
			i++
			continue
		}
		prefix := format[:i]
		return prefix == "" || (strings.HasSuffix(prefix, ":") && !strings.ContainsAny(prefix, `/\`))
	}
	return false
}

func isStringType(t types.Type) bool {
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Info()&types.IsString != 0
}

// leadingPrefix reports whether v, as the left side of a concatenation,
// leaves the right side at the start of a path: the empty string, or a URI
// scheme such as "file:" (a sqlite DSN), which names no directory.
func leadingPrefix(v ssa.Value) bool {
	s, ok := constString(v)
	if !ok {
		return false
	}
	if s == "" {
		return true
	}
	return strings.HasSuffix(s, ":") && !strings.ContainsAny(s, `/\`)
}

// checkExecCmdField treats a store into an os/exec.Cmd's Path (the program
// that runs) or Dir (the directory it runs in) as a sink: exec.Command("x")
// followed by cmd.Path = v runs a file the value chose.
func (st *fnState) checkExecCmdField(s *ssa.Store) {
	fa, ok := s.Addr.(*ssa.FieldAddr)
	if !ok || qualifiedTypeName(fa.X.Type()) != "os/exec.Cmd" {
		return
	}
	stt, ok := derefStruct(fa.X.Type())
	if !ok || fa.Field >= stt.NumFields() {
		return
	}
	switch name := stt.Field(fa.Field).Name(); name {
	case "Path", "Dir":
		st.recordSink(st.tagsOf(s.Val), sinkSite{Sink: "os/exec.Cmd." + name, Pos: st.e.pos(s.Pos())})
	}
}

// checkHTTPDir treats a conversion to net/http.Dir as a sink: the value
// becomes the root of a file server.
func (st *fnState) checkHTTPDir(v ssa.Value, from ssa.Value) {
	if qualifiedTypeName(v.Type()) == "net/http.Dir" {
		st.recordSink(st.tagsOf(from), sinkSite{Sink: "net/http.Dir", Pos: st.e.pos(v.Pos())})
	}
}

func (st *fnState) makeClosure(mc *ssa.MakeClosure) {
	f, ok := mc.Fn.(*ssa.Function)
	if !ok || !st.e.hasBody(f) {
		return
	}
	for k, b := range mc.Bindings {
		t := st.tagsOf(b)
		if !t.hasGlobal() {
			continue
		}
		m := st.e.fvTags[f]
		if m == nil {
			m = map[int]*tagSet{}
			st.e.fvTags[f] = m
		}
		if m[k] == nil {
			m[k] = &tagSet{}
		}
		if m[k].unionGlobal(t) {
			st.e.changed = true
			st.e.markDirty(f)
		}
	}
	sum := st.e.sums[f]
	st.useSummary(f)
	bind := func(tag int) (tagSet, bool) {
		if isFreeVarTag(tag) {
			k := symIndex(tag)
			if k < len(mc.Bindings) {
				return asComponentIf(st.tagsOf(mc.Bindings[k]), isComponent(tag)), true
			}
		}
		return tagSet{}, false
	}
	for tag, site := range sum.sink {
		if t, ok := bind(tag); ok {
			s := *site
			if s.Via == "" {
				s.Via = st.e.pos(mc.Pos())
			}
			st.recordSink(t, s)
		}
	}
	for tag, objs := range sum.store {
		if t, ok := bind(tag); ok {
			for k := range objs {
				st.storeObj(k, t)
			}
		}
	}
	for i, writes := range []map[int]*tagSet{sum.ptrw, sum.ptrwWhole} {
		for target, written := range writes {
			if !isFreeVarTag(target) {
				continue
			}
			k := symIndex(target)
			if k >= len(mc.Bindings) {
				continue
			}
			var t tagSet
			t.unionGlobal(*written)
			written.eachSymbolic(func(tag int) {
				if bt, ok := bind(tag); ok {
					t.union(bt)
				}
			})
			st.storeIntoKind(mc.Bindings[k], t, i == 1)
		}
	}
}

// call applies the effect of one call and returns the taint of each result.
func (st *fnState) call(instr ssa.CallInstruction, common *ssa.CallCommon) []tagSet {
	n := resultCount(common.Signature())
	per := make([]tagSet, n)
	addAll := func(t tagSet) {
		for i := range per {
			per[i].union(t)
		}
	}
	var args []ssa.Value
	var bindings []ssa.Value
	var callees []*ssa.Function
	dynamic := false
	if common.IsInvoke() {
		args = append([]ssa.Value{common.Value}, common.Args...)
		callees = st.e.invokeCallees(common)
		dynamic = true
	} else {
		args = common.Args
		switch v := common.Value.(type) {
		case *ssa.Builtin:
			return broadcast(n, st.builtin(v, args))
		case *ssa.Function:
			callees = []*ssa.Function{v}
		case *ssa.MakeClosure:
			if f, ok := v.Fn.(*ssa.Function); ok {
				callees = []*ssa.Function{f}
			}
			bindings = v.Bindings
		default:
			callees = st.e.bySig[sigKey(common.Signature())]
			dynamic = true
		}
	}
	external := dynamic
	for _, callee := range callees {
		if st.e.hasBody(callee) {
			r := st.applySummary(callee, args, bindings, instr)
			for i := range per {
				if i < len(r) {
					per[i].union(r[i])
				} else {
					per[i].union(unionAll(r))
				}
			}
			continue
		}
		if r, handled := st.e.externalSpecial(st, callee, args, instr); handled {
			addAll(r)
			continue
		}
		if filteredCharWrite(callee, args, instr) {
			// An allowlist filter: no path separator, dot or NUL can reach
			// this write, so what it builds cannot choose a location.
			external = false
			continue
		}
		if r, handled := st.pathBuilder(callee, args); handled {
			addAll(r)
			continue
		}
		external = true
		if callee.Object() != nil {
			if fnObj, ok := callee.Object().(*types.Func); ok {
				if isSanitizer(fnObj) {
					external = false
					continue
				}
				st.externalSink(fnObj, args, instr, callee)
			}
		}
	}
	if len(callees) == 0 {
		external = true
		if common.IsInvoke() && common.Method != nil {
			if isSanitizer(common.Method) {
				return per
			}
			st.externalSink(common.Method, args, instr, nil)
		}
	}
	if external {
		addAll(st.externalPropagate(common, args))
	}
	return per
}

func (st *fnState) applySummary(callee *ssa.Function, args, bindings []ssa.Value, instr ssa.CallInstruction) []tagSet {
	sum := st.e.sums[callee]
	st.useSummary(callee)
	argTags := func(tag int) (tagSet, bool) {
		comp := isComponent(tag)
		if isParamTag(tag) {
			i := symIndex(tag)
			if i < len(args) {
				if i == maxParamSlot {
					var t tagSet
					for _, a := range args[i:] {
						t.union(st.tagsOf(a))
					}
					return asComponentIf(t, comp), true
				}
				return asComponentIf(st.tagsOf(args[i]), comp), true
			}
			return tagSet{}, false
		}
		if isFreeVarTag(tag) {
			k := symIndex(tag)
			if k < len(bindings) {
				return asComponentIf(st.tagsOf(bindings[k]), comp), true
			}
		}
		return tagSet{}, false
	}
	argValue := func(tag int) ssa.Value {
		if isParamTag(tag) && symIndex(tag) < len(args) {
			return args[symIndex(tag)]
		}
		if isFreeVarTag(tag) && symIndex(tag) < len(bindings) {
			return bindings[symIndex(tag)]
		}
		return nil
	}
	result := make([]tagSet, len(sum.ret))
	for i := range sum.ret {
		result[i].unionGlobal(sum.ret[i])
		sum.ret[i].eachSymbolic(func(tag int) {
			if t, ok := argTags(tag); ok {
				result[i].union(t)
			}
		})
	}
	for tag, site := range sum.sink {
		if t, ok := argTags(tag); ok && !t.empty() {
			s := *site
			if s.Via == "" {
				s.Via = st.e.pos(instr.Pos()) + " " + callee.String()
			}
			st.recordSink(t, s)
		}
	}
	// An os.Root operation inside the callee whose root came from its
	// parameters: decide, with this call's arguments, whether the root is
	// steered (see confinedSink). The name's component form already reached
	// the sink through sum.sink.
	condKeys := make([]string, 0, len(sum.rootConds))
	for k := range sum.rootConds {
		condKeys = append(condKeys, k)
	}
	sort.Strings(condKeys)
	for _, k := range condKeys {
		c := sum.rootConds[k]
		var recv tagSet
		unresolved := false
		c.recv.eachSymbolic(func(tag int) {
			if t, ok := argTags(tag); ok {
				recv.union(t)
			} else {
				unresolved = true
			}
		})
		var name tagSet
		name.unionGlobal(c.name)
		c.name.eachSymbolic(func(tag int) {
			if t, ok := argTags(tag); ok {
				name.union(t)
			}
		})
		s := c.site
		if s.Via == "" {
			s.Via = st.e.pos(instr.Pos()) + " " + callee.String()
		}
		switch {
		case name.empty():
		case unresolved:
			st.recordSink(name, s)
		default:
			st.confinedSink(recv, name, s)
		}
	}
	for tag, objs := range sum.store {
		if t, ok := argTags(tag); ok {
			for k := range objs {
				st.storeObj(k, t)
			}
		}
	}
	for i, writes := range []map[int]*tagSet{sum.ptrw, sum.ptrwWhole} {
		for target, written := range writes {
			dst := argValue(target)
			if dst == nil {
				continue
			}
			var t tagSet
			t.unionGlobal(*written)
			written.eachSymbolic(func(tag int) {
				if at, ok := argTags(tag); ok {
					t.union(at)
				}
			})
			st.storeIntoKind(dst, t, i == 1)
		}
	}
	return result
}

func (st *fnState) builtin(b *ssa.Builtin, args []ssa.Value) tagSet {
	var all tagSet
	for _, a := range args {
		all.union(st.tagsOf(a))
	}
	switch b.Name() {
	case "append":
		if len(args) > 1 {
			st.storeInto(args[0], st.load(args[1]))
			all.union(st.load(args[1]))
		}
		return all
	case "copy":
		if len(args) == 2 {
			st.storeInto(args[0], st.load(args[1]))
		}
		return tagSet{}
	case "len", "cap", "delete", "close", "clear", "print", "println", "recover":
		return tagSet{}
	}
	return all
}

// externalSink records a sink hit when an opaque callee is a filesystem
// function.
func (st *fnState) externalSink(fn *types.Func, args []ssa.Value, instr ssa.CallInstruction, callee *ssa.Function) {
	idx := sinkParams(fn)
	if len(idx) == 0 {
		return
	}
	_, key := funcKey(fn)
	sig := fn.Type().(*types.Signature)
	offset := 0
	if sig.Recv() != nil {
		offset = 1
	}
	if fsNameSinks[key] && len(args) > 0 && isEmbedFS(args[0]) {
		return
	}
	if isRootMethod(fn) && len(args) > 0 {
		// An operation through an os.Root lands at the root's location
		// plus a name the root keeps inside it. Whoever steered the root
		// chose where it lands; the name is resolved by confinedSink.
		site := sinkSite{Sink: key, Pos: st.e.pos(instr.Pos())}
		recv := st.load(args[0])
		if !recv.empty() {
			st.recordSink(recv, site)
		}
		for _, i := range idx {
			if i+1 >= len(args) {
				continue
			}
			t := st.tagsOf(args[i+1])
			if rootUnconfinedParam(key, i) {
				st.recordSink(t, site)
			} else {
				st.confinedSink(recv, t, site)
			}
		}
		return
	}
	for _, i := range idx {
		j := i + offset
		if j >= len(args) {
			continue
		}
		t := st.tagsOf(args[j])
		if sig.Variadic() && i == sig.Params().Len()-1 {
			t = st.load(args[j])
		}
		if t.empty() {
			continue
		}
		st.recordSink(t, sinkSite{Sink: key, Pos: st.e.pos(instr.Pos())})
	}
}

func isEmbedFS(v ssa.Value) bool {
	for {
		switch x := v.(type) {
		case *ssa.MakeInterface:
			v = x.X
			continue
		case *ssa.ChangeInterface:
			v = x.X
			continue
		}
		break
	}
	return qualifiedTypeName(v.Type()) == "embed.FS"
}

// externalPropagate is the default rule for an opaque call: the result
// carries every argument's taint; destination arguments (pointers, maps,
// any) receive the taint of the data arguments; accumulator receivers
// (builders, buffers, header maps) receive the taint of every argument.
func (st *fnState) externalPropagate(common *ssa.CallCommon, args []ssa.Value) tagSet {
	var all, data tagSet
	for _, a := range args {
		t := st.tagsOf(a)
		all.union(t)
		if isDataLike(a.Type()) {
			data.union(t)
			if _, ok := a.Type().Underlying().(*types.Slice); ok {
				data.union(st.load(a))
				all.union(st.load(a))
			}
		}
	}
	recvIndex := -1
	if common.IsInvoke() {
		recvIndex = 0
	} else if sig := common.Signature(); sig.Recv() != nil {
		recvIndex = 0
	}
	// A method's destination argument is filled from the data arguments AND
	// from the receiver: json.NewDecoder(strings.NewReader(v)).Decode(&cfg)
	// carries v through the decoder into cfg.
	fill := data
	if recvIndex == 0 && len(args) > 0 {
		fill = data.clone()
		fill.union(st.tagsOf(args[0]))
	}
	for i, a := range args {
		if i == recvIndex {
			if receiverAccumulators[qualifiedTypeName(a.Type())] {
				st.storeInto(a, all)
			}
			continue
		}
		if isDestination(a.Type()) {
			st.storeIntoWhole(a, fill)
		}
	}
	return all
}

func isDataLike(t types.Type) bool {
	switch u := t.Underlying().(type) {
	case *types.Basic:
		return true
	case *types.Slice:
		switch eu := u.Elem().Underlying().(type) {
		case *types.Basic:
			return true
		case *types.Interface:
			return eu.Empty()
		}
	}
	return false
}

var (
	ioWriter = types.NewInterfaceType([]*types.Func{
		types.NewFunc(token.NoPos, nil, "Write", types.NewSignatureType(nil, nil, nil,
			types.NewTuple(types.NewVar(token.NoPos, nil, "p", types.NewSlice(types.Typ[types.Byte]))),
			types.NewTuple(types.NewVar(token.NoPos, nil, "n", types.Typ[types.Int]), types.NewVar(token.NoPos, nil, "err", types.Universe.Lookup("error").Type())), false)),
	}, nil).Complete()
	ioReader = types.NewInterfaceType([]*types.Func{
		types.NewFunc(token.NoPos, nil, "Read", types.NewSignatureType(nil, nil, nil,
			types.NewTuple(types.NewVar(token.NoPos, nil, "p", types.NewSlice(types.Typ[types.Byte]))),
			types.NewTuple(types.NewVar(token.NoPos, nil, "n", types.Typ[types.Int]), types.NewVar(token.NoPos, nil, "err", types.Universe.Lookup("error").Type())), false)),
	}, nil).Complete()
)

func isDestination(t types.Type) bool {
	if qualifiedTypeName(t) == "context.Context" {
		return false
	}
	switch u := t.Underlying().(type) {
	case *types.Pointer, *types.Map:
	case *types.Interface:
		if !u.Empty() {
			return false
		}
	case *types.Slice:
		if isDataLike(t) {
			return false
		}
	default:
		return false
	}
	if types.Implements(t, ioWriter) || types.Implements(t, ioReader) {
		return false
	}
	return true
}

// invokeCallees resolves an interface method call to the module types that
// implement the interface.
func (e *engine) invokeCallees(common *ssa.CallCommon) []*ssa.Function {
	iface, ok := common.Value.Type().Underlying().(*types.Interface)
	if !ok || common.Method == nil {
		return nil
	}
	key := types.TypeString(common.Value.Type(), nil) + "." + common.Method.Name()
	if r, ok := e.invokeMemo[key]; ok {
		return r
	}
	var out []*ssa.Function
	for _, t := range e.modTypes {
		if !types.Implements(t, iface) {
			continue
		}
		if fn := e.prog.LookupMethod(t, common.Method.Pkg(), common.Method.Name()); fn != nil && e.hasBody(fn) {
			out = append(out, fn)
		}
	}
	e.invokeMemo[key] = out
	return out
}

// ---------------------------------------------------------------------------
// Fixpoint and reporting

// run drives the worklist to a fixpoint. It returns the number of function
// visits divided by the number of functions (the equivalent number of full
// passes) and reports convergence through e.converged.
func (e *engine) run(maxRounds int) int {
	for _, fn := range e.funcs {
		e.markDirty(fn)
	}
	// Rounds: each round analyses every queued function once, in source
	// order, and queues for the next round whatever its writes affected.
	rounds := 0
	for len(e.queue) > 0 {
		if rounds >= maxRounds {
			e.converged = false
			return rounds
		}
		rounds++
		batch := e.queue
		e.queue = nil
		sort.Slice(batch, func(i, j int) bool { return e.order[batch[i]] < e.order[batch[j]] })
		for _, fn := range batch {
			e.queued[fn] = false
		}
		for _, fn := range batch {
			e.analyze(fn)
		}
	}
	e.converged = true
	return rounds
}

func (e *engine) sortedFindings() []int {
	var ids []int
	for id := range e.findings {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids
}

func shortFunc(fn *ssa.Function) string {
	s := fn.String()
	if i := strings.LastIndex(s, "/"); i >= 0 {
		s = s[i+1:]
	}
	return s
}
