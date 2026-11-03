// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package main

import (
	"fmt"
	"go/constant"
	"go/token"
	"go/types"
	"slices"
	"strings"

	"golang.org/x/tools/go/ssa"
)

// A source is a value the caller of the CLI (and so, through the MCP shell-out,
// an MCP tool caller) chooses.
type source struct {
	ID    int    `json:"-"`
	Kind  string `json:"kind"` // flag | flag-getter | flag-any | positional | mcp-request | os-args
	Name  string `json:"name,omitempty"`
	Short string `json:"shorthand,omitempty"`
	// Usage and Default are the literal registration arguments when they are
	// constants; the runtime check matches a live flag on (name, usage,
	// default) so a flag name reused with a different meaning elsewhere is
	// not swept in. A nil field means "not a constant": match on name alone.
	Usage   *string `json:"usage"`
	Default *string `json:"default"`
	Type    string  `json:"type,omitempty"`
	Use     string  `json:"use,omitempty"`
	Func    string  `json:"func,omitempty"`
	Pos     string  `json:"pos"`
	// NameUnknown marks a registration whose flag name is not a constant.
	NameUnknown bool `json:"name_unknown,omitempty"`
}

func (e *engine) newSource(s *source) int {
	key := s.Kind + "|" + s.Pos + "|" + s.Name + "|" + s.Use + "|" + s.Func
	if id, ok := e.sourceAt[key]; ok {
		return id
	}
	s.ID = symbolicSlots + 2*len(e.sources) // lead tag; ID+1 is its component form
	e.sources = append(e.sources, s)
	e.sourceAt[key] = s.ID
	return s.ID
}

func constString(v ssa.Value) (string, bool) {
	if c, ok := v.(*ssa.Const); ok && c.Value != nil && c.Value.Kind() == constant.String {
		return constant.StringVal(c.Value), true
	}
	return "", false
}

func strPtr(s string, ok bool) *string {
	if !ok {
		return nil
	}
	return &s
}

type flagSpec struct {
	ptr, name, short, value, usage, fn int // signature parameter indexes, -1 when absent
	kind                               string
	typ                                string
}

const (
	regPointer = "pointer" // the flag value is written into *ptr
	regValue   = "value"   // a pflag.Value implementation receives Set(s)
	regReturn  = "return"  // the call returns a pointer to the flag value
	regGetter  = "getter"  // the call returns the flag value
	regLookup  = "lookup"  // the call returns the *pflag.Flag
	regFunc    = "func"    // a callback receives the value
	regVisit   = "visit"   // a callback receives every flag
)

var pflagSpecs = map[string]flagSpec{}

func init() {
	for _, typ := range []string{"String", "StringSlice", "StringArray", "StringToString"} {
		pflagSpecs[typ+"Var"] = flagSpec{ptr: 0, name: 1, short: -1, value: 2, usage: 3, fn: -1, kind: regPointer, typ: typ}
		pflagSpecs[typ+"VarP"] = flagSpec{ptr: 0, name: 1, short: 2, value: 3, usage: 4, fn: -1, kind: regPointer, typ: typ}
		pflagSpecs[typ] = flagSpec{ptr: -1, name: 0, short: -1, value: 1, usage: 2, fn: -1, kind: regReturn, typ: typ}
		pflagSpecs[typ+"P"] = flagSpec{ptr: -1, name: 0, short: 1, value: 2, usage: 3, fn: -1, kind: regReturn, typ: typ}
		pflagSpecs["Get"+typ] = flagSpec{ptr: -1, name: 0, short: -1, value: -1, usage: -1, fn: -1, kind: regGetter, typ: typ}
	}
	pflagSpecs["Var"] = flagSpec{ptr: 0, name: 1, short: -1, value: -1, usage: 2, fn: -1, kind: regValue, typ: "Value"}
	pflagSpecs["VarP"] = flagSpec{ptr: 0, name: 1, short: 2, value: -1, usage: 3, fn: -1, kind: regValue, typ: "Value"}
	pflagSpecs["VarPF"] = flagSpec{ptr: 0, name: 1, short: 2, value: -1, usage: 3, fn: -1, kind: regValue, typ: "Value"}
	pflagSpecs["TextVar"] = flagSpec{ptr: 0, name: 1, short: -1, value: -1, usage: 3, fn: -1, kind: regValue, typ: "Text"}
	pflagSpecs["Func"] = flagSpec{ptr: -1, name: 0, short: -1, value: -1, usage: 1, fn: 2, kind: regFunc, typ: "Func"}
	pflagSpecs["FuncP"] = flagSpec{ptr: -1, name: 0, short: 1, value: -1, usage: 2, fn: 3, kind: regFunc, typ: "Func"}
	pflagSpecs["Lookup"] = flagSpec{ptr: -1, name: 0, short: -1, value: -1, usage: -1, fn: -1, kind: regLookup}
	pflagSpecs["ShorthandLookup"] = flagSpec{ptr: -1, name: -1, short: 0, value: -1, usage: -1, fn: -1, kind: regLookup}
	pflagSpecs["VisitAll"] = flagSpec{ptr: -1, name: -1, short: -1, value: -1, usage: -1, fn: 0, kind: regVisit}
	pflagSpecs["Visit"] = flagSpec{ptr: -1, name: -1, short: -1, value: -1, usage: -1, fn: 0, kind: regVisit}
}

// kitFlagValues reads a call to the reviewed credential kit's
// FlagValues(args, "--name") as a getter of flag name: it returns only the
// values the program's flag parser could give --name (the kit's own tests pin
// that), so what it returns is that flag's value, whatever argv it scanned.
// The MCP shell-out refuses every path flag by name (the assertions
// check_mcp_file_flags.py drives), which is what makes the os.Args the kit
// scans unable to carry it. Only with a constant name and only for the
// reviewed kit: a computed name, or a kit copy that differs from the
// reviewed one by a byte, is analysed as code, so its os.Args stays os.Args.
func (e *engine) kitFlagValues(st *fnState, callee *ssa.Function, args []ssa.Value, instr ssa.CallInstruction) (tagSet, bool) {
	if e.kitPath == "" || callee.Pkg == nil || callee.Pkg.Pkg.Path() != e.kitPath ||
		callee.Name() != "FlagValues" || callee.Signature.Recv() != nil || len(args) != 2 {
		return tagSet{}, false
	}
	flag, ok := constString(args[1])
	name := strings.TrimLeft(flag, "-")
	if !ok || len(name) < 2 || len(flag)-len(name) != 2 || !e.isArgv(args[0]) || e.argvMayBeWritten() {
		return tagSet{}, false
	}
	spec := flagSpec{ptr: -1, name: 0, short: -1, value: -1, usage: -1, fn: -1, kind: regGetter, typ: "StringSlice"}
	t := e.flagSource(st, spec, []ssa.Value{ssa.NewConst(constant.MakeString(name), args[1].Type())}, nil, instr)
	if c, ok := instr.(*ssa.Call); ok && e.obj(c).union(t) {
		e.changed = true
		e.touchObj(c)
	}
	return t, true
}

// argvReaders are the calls outside the module a value derived from argv
// may be passed to: each reads its slice argument, never writes into it and
// keeps no reference to it (checked against cobra v1.9.1 and v1.10.2 and
// pflag v1.0.5 to v1.0.10, which copy what they keep). The standard flag
// package is not one: FlagSet.Parse keeps argv, and Args() hands it back.
// A result of the same type may share argv's array, so it is followed too.
var argvReaders = map[string]bool{
	"github.com/spf13/cobra.Command.Find":       true,
	"github.com/spf13/cobra.Command.Traverse":   true,
	"github.com/spf13/cobra.Command.ParseFlags": true,
	"github.com/spf13/pflag.FlagSet.Parse":      true,
	"slices.Clone":                              true,
	"slices.Contains":                           true,
	"slices.Index":                              true,
	"strings.Join":                              true,
}

// argvMayBeWritten: some module code may write into the array os.Args
// names, or replace os.Args. FlagValues over argv then returns what the
// program wrote - a constant "--config" carries no taint - so no scan of it
// is tied to a flag. Every value derived from argv is followed through the
// module: a read (index, len, range, a slice of it, a module function's
// parameter or result, the kit's d.Files) is fine; a store into it, a copy
// or in-place append into it, a call outside argvReaders, or any other use
// (an interface, a struct field, a closure) counts as a write.
func (e *engine) argvMayBeWritten() bool {
	if e.argvDone {
		return e.argvWritten
	}
	e.argvDone = true
	e.argvWhy = e.scanArgvWrites()
	e.argvWritten = e.argvWhy != ""
	return e.argvWritten
}

// scanArgvWrites is where module code may write into argv, or "".
func (e *engine) scanArgvWrites() string {
	osPkg := e.prog.ImportedPackage("os")
	if osPkg == nil {
		return ""
	}
	g, _ := osPkg.Members["Args"].(*ssa.Global)
	if g == nil {
		return ""
	}
	e.argvIndex()
	calls := map[*ssa.Function][]*ssa.Call{} // static call sites of each module function
	seen := map[ssa.Value]bool{}
	var work []ssa.Value
	add := func(v ssa.Value) {
		if v != nil && !seen[v] {
			seen[v] = true
			work = append(work, v)
		}
	}
	for _, fn := range e.funcs {
		for _, b := range fn.Blocks {
			for _, instr := range b.Instrs {
				switch x := instr.(type) {
				case *ssa.UnOp:
					if x.X == g && x.Op == token.MUL {
						add(x)
					}
				case *ssa.Store:
					if x.Addr == g {
						return e.pos(x.Pos()) + ": os.Args assigned"
					}
				case *ssa.Call:
					if f, ok := x.Call.Value.(*ssa.Function); ok {
						calls[f] = append(calls[f], x)
					}
				}
				for _, op := range instr.Operands(nil) {
					if op != nil && *op == g {
						if _, ok := instr.(*ssa.UnOp); !ok {
							if st, ok := instr.(*ssa.Store); !ok || st.Addr != g {
								return e.pos(instr.Pos()) + ": &os.Args handed on"
							}
						}
					}
				}
			}
		}
	}
	// results adds what a call returns when its callee returns argv.
	// idx < 0: every result that is a slice (what an argv reader may hand back).
	isSlice := func(v ssa.Value) bool { _, ok := v.Type().Underlying().(*types.Slice); return ok }
	results := func(c *ssa.Call, idx int) {
		if _, ok := c.Type().(*types.Tuple); !ok {
			if idx >= 0 || isSlice(c) {
				add(c)
			}
			return
		}
		for _, r := range *c.Referrers() {
			if ex, ok := r.(*ssa.Extract); ok && (ex.Index == idx || idx < 0 && isSlice(ex)) {
				add(ex)
			}
		}
	}
	for len(work) > 0 {
		v := work[len(work)-1]
		work = work[:len(work)-1]
		refs := v.Referrers()
		if refs == nil {
			continue
		}
		for _, r := range *refs {
			switch x := r.(type) {
			case *ssa.DebugRef, *ssa.Index, *ssa.BinOp, *ssa.If:
			case *ssa.Slice, *ssa.Phi, *ssa.ChangeType:
				add(x.(ssa.Value))
			case *ssa.IndexAddr:
				for _, u := range *x.Referrers() {
					if l, ok := u.(*ssa.UnOp); !ok || l.Op != token.MUL {
						if _, ok := u.(*ssa.DebugRef); !ok {
							return e.pos(u.Pos()) + ": a store into argv, or its element's address"
						}
					}
				}
			case *ssa.Return:
				fn := x.Parent()
				if e.argvEscaped[fn] || fn.Signature.Recv() != nil {
					// Called through a value or an interface: its callers are unseen.
					return e.pos(x.Pos()) + ": argv returned from a function called through a value or an interface"
				}
				for i, res := range x.Results {
					if res == v {
						for _, c := range calls[fn] {
							results(c, i)
						}
					}
				}
			case ssa.CallInstruction:
				common := x.Common()
				if common.Value == v {
					return e.pos(x.Pos()) + ": argv called"
				}
				if b, ok := common.Value.(*ssa.Builtin); ok {
					switch b.Name() {
					case "len", "cap":
						continue
					case "append", "copy":
						if len(common.Args) > 0 && common.Args[0] != v {
							continue // argv is the source, copied out
						}
					}
					return e.pos(x.Pos()) + ": argv is the destination of " + b.Name()
				}
				var callees []*ssa.Function
				resolved := false
				if f, ok := common.Value.(*ssa.Function); ok && !common.IsInvoke() {
					callees, resolved = []*ssa.Function{f}, true
				} else if e.isKitFilesCall(x.Parent(), common) && e.kitFilesOK {
					callees, resolved = e.kitFiles, true // no Files stored: never called
				}
				if len(callees) == 1 && !e.hasBody(callees[0]) {
					fnObj, _ := callees[0].Object().(*types.Func)
					if _, key := funcKey(fnObj); argvReaders[key] {
						if c, ok := x.(*ssa.Call); ok {
							results(c, -1)
						}
						continue
					}
					return e.pos(x.Pos()) + ": argv passed to " + callees[0].String()
				}
				if !resolved {
					return e.pos(x.Pos()) + ": argv passed to a call whose callee is not known"
				}
				for _, f := range callees {
					for i, a := range common.Args {
						if a == v && i < len(f.Params) {
							add(f.Params[i])
						}
					}
				}
			default:
				return e.pos(r.Pos()) + fmt.Sprintf(": argv used as %T", r)
			}
		}
	}
	return ""
}

// kitFilesCallees resolves the reviewed kit's own d.Files(args) call
// (start.go) to the functions the module stores into a Declarations' Files
// field, so the connector's Files function is analysed with its real body
// instead of every argument flowing to the result. ok is false (the call
// stays dynamic, every argument tainting the result) outside the reviewed
// kit, or when some Files value is not a plain function: a closure, a
// variable, a parameter, or a conversion from another struct type.
func (e *engine) kitFilesCallees(st *fnState, common *ssa.CallCommon) ([]*ssa.Function, bool) {
	if !e.isKitFilesCall(st.fn, common) {
		return nil, false
	}
	if !e.kitFilesDone {
		e.kitFilesDone = true
		e.kitFiles, e.kitFilesOK = e.collectKitFiles()
	}
	return e.kitFiles, e.kitFilesOK
}

// isKitFilesCall: common, in fn, is the reviewed kit calling d.Files.
func (e *engine) isKitFilesCall(fn *ssa.Function, common *ssa.CallCommon) bool {
	if e.kitPath == "" || fn.Pkg == nil || fn.Pkg.Pkg.Path() != e.kitPath || common.IsInvoke() {
		return false
	}
	var x ssa.Value
	var i int
	switch v := common.Value.(type) {
	case *ssa.Field: // d.Files on a struct value
		x, i = v.X, v.Field
	case *ssa.UnOp: // d.Files on an addressed d: a load through FieldAddr
		fa, ok := v.X.(*ssa.FieldAddr)
		if !ok || v.Op != token.MUL {
			return false
		}
		x, i = fa.X, fa.Field
	default:
		return false
	}
	return e.isKitDecls(x.Type()) && kitField(x.Type(), i) == "Files"
}

// isArgv: v is the process argv itself - os.Args, a slice of it, or a
// parameter every caller passes argv to (the kit's gather, and a Files
// function the kit calls with that argv). An argv the program builds (a
// literal, an append, an element it chose) is not: FlagValues over it
// returns whatever the program put there, not what the flag holds.
func (e *engine) isArgv(v ssa.Value) bool {
	e.argvIndex()
	return e.isArgvMemo(v)
}

// argvIndex builds argvCalls and argvEscaped once.
func (e *engine) argvIndex() {
	if e.argvMemo == nil {
		e.argvMemo = map[ssa.Value]bool{}
		e.indexArgvCalls()
	}
}

func (e *engine) isArgvMemo(v ssa.Value) bool {
	if r, ok := e.argvMemo[v]; ok {
		return r
	}
	e.argvMemo[v] = false // a value reached again on a cycle is not argv
	r := false
	switch x := v.(type) {
	case *ssa.UnOp:
		g, ok := x.X.(*ssa.Global)
		r = ok && x.Op == token.MUL && g.Pkg != nil && g.Pkg.Pkg.Path() == "os" && g.Name() == "Args"
	case *ssa.Slice:
		r = e.isArgvMemo(x.X)
	case *ssa.Parameter:
		fn := x.Parent()
		i := slices.Index(fn.Params, x)
		calls, ok := e.argvCalls[fn]
		r = ok && len(calls) > 0 && i >= 0 && fn.FreeVars == nil
		for _, args := range calls {
			r = r && i < len(args) && e.isArgvMemo(args[i])
		}
	}
	e.argvMemo[v] = r
	return r
}

// indexArgvCalls records every argument list each module function is called
// with: its static calls, and for a function stored in Declarations.Files,
// the kit's d.Files calls. A function used as a value any other way escapes
// (nil): its callers are unknown.
func (e *engine) indexArgvCalls() {
	e.argvCalls = map[*ssa.Function][][]ssa.Value{}
	escaped := map[*ssa.Function]bool{}
	var filesCalls [][]ssa.Value
	for _, fn := range e.funcs {
		for _, b := range fn.Blocks {
			for _, instr := range b.Instrs {
				var ops []*ssa.Value
				if c, ok := instr.(ssa.CallInstruction); ok {
					common := c.Common()
					if f, ok := common.Value.(*ssa.Function); ok && !common.IsInvoke() {
						e.argvCalls[f] = append(e.argvCalls[f], common.Args)
						for i := range common.Args {
							ops = append(ops, &common.Args[i])
						}
					} else {
						if e.isKitFilesCall(fn, common) {
							filesCalls = append(filesCalls, common.Args)
						}
						ops = instr.Operands(nil)
					}
				} else {
					ops = instr.Operands(nil)
				}
				st, _ := instr.(*ssa.Store)
				for _, op := range ops {
					if op == nil || *op == nil {
						continue
					}
					f, ok := (*op).(*ssa.Function)
					if !ok {
						continue
					}
					if st != nil && st.Val == f {
						if fa, ok := st.Addr.(*ssa.FieldAddr); ok && e.isKitDecls(fa.X.Type()) && kitField(fa.X.Type(), fa.Field) == "Files" {
							continue
						}
					}
					escaped[f] = true
				}
				if mc, ok := instr.(*ssa.MakeClosure); ok {
					if f, ok := mc.Fn.(*ssa.Function); ok {
						escaped[f] = true
					}
				}
			}
		}
	}
	if !e.kitFilesDone {
		e.kitFilesDone = true
		e.kitFiles, e.kitFilesOK = e.collectKitFiles()
	}
	if e.kitFilesOK {
		for _, f := range e.kitFiles {
			e.argvCalls[f] = append(e.argvCalls[f], filesCalls...)
		}
	} else {
		for _, f := range e.kitFiles {
			escaped[f] = true
		}
	}
	for f := range e.argvCalls {
		if f.Signature.Recv() != nil {
			escaped[f] = true // a method can be called through an interface, unseen here
		}
	}
	for f := range escaped {
		delete(e.argvCalls, f)
	}
	e.argvEscaped = escaped
}

// isKitDecls: t is the kit's Declarations, or a pointer to it.
func (e *engine) isKitDecls(t types.Type) bool {
	if p, ok := t.Underlying().(*types.Pointer); ok {
		t = p.Elem()
	}
	n, ok := types.Unalias(t).(*types.Named)
	return ok && n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == e.kitPath && n.Obj().Name() == "Declarations"
}

// kitField is the name of field i of the (pointed-to) struct t.
func kitField(t types.Type, i int) string {
	if p, ok := t.Underlying().(*types.Pointer); ok {
		t = p.Elem()
	}
	if st, ok := t.Underlying().(*types.Struct); ok && i < st.NumFields() {
		return st.Field(i).Name()
	}
	return ""
}

// collectKitFiles is every function stored into a Declarations' Files field
// in the module; ok is false when any store or conversion could put another
// value there.
func (e *engine) collectKitFiles() ([]*ssa.Function, bool) {
	var out []*ssa.Function
	for _, fn := range e.funcs {
		for _, b := range fn.Blocks {
			for _, instr := range b.Instrs {
				switch v := instr.(type) {
				case *ssa.Store:
					fa, ok := v.Addr.(*ssa.FieldAddr)
					if !ok || !e.isKitDecls(fa.X.Type()) || kitField(fa.X.Type(), fa.Field) != "Files" {
						continue
					}
					switch val := v.Val.(type) {
					case *ssa.Function:
						if !slices.Contains(out, val) {
							out = append(out, val)
						}
					case *ssa.Const:
						if !val.IsNil() {
							return nil, false
						}
					default:
						return nil, false
					}
				case *ssa.Convert, *ssa.ChangeType:
					x := v.(ssa.Value)
					op := x.(interface {
						Operands([]*ssa.Value) []*ssa.Value
					}).Operands(nil)
					if e.isKitDecls(x.Type()) && len(op) > 0 && !e.isKitDecls((*op[0]).Type()) {
						return nil, false
					}
				}
			}
		}
	}
	return out, true
}

// externalSpecial handles the opaque calls that create sources: pflag
// registrations and lookups, and cobra's Command.Flag.
func (e *engine) externalSpecial(st *fnState, callee *ssa.Function, args []ssa.Value, instr ssa.CallInstruction) (tagSet, bool) {
	fnObj, ok := callee.Object().(*types.Func)
	if !ok || fnObj == nil {
		return tagSet{}, false
	}
	_, key := funcKey(fnObj)
	switch {
	case strings.HasPrefix(key, "github.com/spf13/pflag.FlagSet."):
		spec, ok := pflagSpecs[fnObj.Name()]
		if !ok {
			// Every other FlagSet method (Changed, Parse, MarkHidden, ...)
			// creates no value a caller chooses, and must not inherit the
			// FlagSet's taint into a fresh object.
			return tagSet{}, true
		}
		return e.flagSource(st, spec, args[1:], args[0], instr), true
	case key == "github.com/spf13/cobra.Command.Flag":
		spec := flagSpec{ptr: -1, name: 0, short: -1, value: -1, usage: -1, fn: -1, kind: regLookup}
		return e.flagSource(st, spec, args[1:], args[0], instr), true
	case strings.HasPrefix(key, "github.com/spf13/cobra.Command."):
		// cobra's own methods (AddCommand, Flags, SetOut, MarkFlagRequired,
		// ...) neither read files named by a flag value nor hand one back;
		// a Command is not an accumulator of its arguments.
		if sinks := sinkParams(fnObj); len(sinks) > 0 {
			return tagSet{}, false
		}
		return tagSet{}, true
	}
	return tagSet{}, false
}

func (e *engine) flagSource(st *fnState, spec flagSpec, params []ssa.Value, recv ssa.Value, instr ssa.CallInstruction) tagSet {
	arg := func(i int) ssa.Value {
		if i < 0 || i >= len(params) {
			return nil
		}
		return params[i]
	}
	s := &source{Type: spec.typ, Pos: e.pos(instr.Pos()), Func: shortFunc(instr.Parent())}
	switch spec.kind {
	case regPointer, regValue, regReturn, regFunc:
		s.Kind = "flag"
	case regGetter, regLookup:
		s.Kind = "flag-getter"
	case regVisit:
		s.Kind = "flag-any"
	}
	if v := arg(spec.name); v != nil {
		if name, ok := constString(v); ok {
			s.Name = name
		} else {
			s.NameUnknown = true
		}
	} else if spec.kind != regVisit {
		s.NameUnknown = spec.name >= 0
	}
	if v := arg(spec.short); v != nil {
		s.Short, _ = constString(v)
	}
	if v := arg(spec.usage); v != nil {
		s.Usage = strPtr(constString(v))
	}
	if v := arg(spec.value); v != nil && spec.typ == "String" {
		s.Default = strPtr(constString(v))
	}
	if spec.kind == regLookup && s.Name == "" && s.Short != "" {
		s.NameUnknown = false
	}
	id := e.newSource(s)
	var t tagSet
	t.add(id)
	switch spec.kind {
	case regPointer:
		if p := arg(spec.ptr); p != nil {
			st.storeInto(p, t)
		}
		return tagSet{}
	case regValue:
		if p := arg(spec.ptr); p != nil {
			st.storeInto(p, t)
			method := "Set"
			idx := 1
			if spec.typ == "Text" {
				method, idx = "UnmarshalText", 1
			}
			for _, ct := range concreteTypes(p) {
				if fn := e.prog.LookupMethod(ct, nil, method); fn != nil && e.hasBody(fn) {
					e.seedParam(fn, idx, id)
				}
			}
		}
		return tagSet{}
	case regReturn:
		if c, ok := instr.(*ssa.Call); ok {
			if e.obj(c).add(id) {
				e.changed = true
				e.touchObj(c)
			}
		}
		return t
	case regGetter, regLookup:
		return t
	case regFunc, regVisit:
		if f := arg(spec.fn); f != nil {
			for _, fn := range funcsOf(f) {
				if e.hasBody(fn) {
					e.seedParam(fn, 0, id)
				}
			}
		}
		return tagSet{}
	}
	return tagSet{}
}

func (e *engine) seedParam(fn *ssa.Function, i, id int) {
	m := e.paramSeeds[fn]
	if m == nil {
		m = map[int]*tagSet{}
		e.paramSeeds[fn] = m
	}
	t := m[i]
	if t == nil {
		t = &tagSet{}
		m[i] = t
	}
	if t.add(id) {
		e.changed = true
		e.markDirty(fn)
	}
}

// concreteTypes is the set of dynamic types an interface value may hold, when
// it is visibly constructed in this function.
func concreteTypes(v ssa.Value) []types.Type {
	var out []types.Type
	seen := map[ssa.Value]bool{}
	var walk func(ssa.Value)
	walk = func(v ssa.Value) {
		if v == nil || seen[v] {
			return
		}
		seen[v] = true
		switch x := v.(type) {
		case *ssa.MakeInterface:
			out = append(out, x.X.Type())
		case *ssa.ChangeInterface:
			walk(x.X)
		case *ssa.Phi:
			for _, ed := range x.Edges {
				walk(ed)
			}
		default:
			if _, isIface := v.Type().Underlying().(*types.Interface); !isIface {
				out = append(out, v.Type())
			}
		}
	}
	walk(v)
	return out
}

func funcsOf(v ssa.Value) []*ssa.Function {
	switch x := v.(type) {
	case *ssa.Function:
		return []*ssa.Function{x}
	case *ssa.MakeClosure:
		if f, ok := x.Fn.(*ssa.Function); ok {
			return []*ssa.Function{f}
		}
	case *ssa.ChangeType:
		return funcsOf(x.X)
	case *ssa.MakeInterface:
		return funcsOf(x.X)
	case *ssa.Phi:
		var out []*ssa.Function
		for _, ed := range x.Edges {
			out = append(out, funcsOf(ed)...)
		}
		return out
	}
	return nil
}

var cobraRunFields = map[string]bool{
	"Run": true, "RunE": true, "PreRun": true, "PreRunE": true, "PostRun": true, "PostRunE": true,
	"PersistentPreRun": true, "PersistentPreRunE": true, "PersistentPostRun": true,
	"PersistentPostRunE": true, "Args": true,
}

// noteCommandField seeds the positional-args parameter of every function
// stored into a cobra.Command Run/RunE/Args field.
func (e *engine) noteCommandField(fn *ssa.Function, s *ssa.Store) {
	fa, ok := s.Addr.(*ssa.FieldAddr)
	if !ok {
		return
	}
	if qualifiedTypeName(fa.X.Type()) != "github.com/spf13/cobra.Command" {
		return
	}
	st, ok := derefStruct(fa.X.Type())
	if !ok || fa.Field >= st.NumFields() || !cobraRunFields[st.Field(fa.Field).Name()] {
		return
	}
	targets := funcsOf(s.Val)
	if len(targets) == 0 {
		return
	}
	use := commandUse(fn, fa.X)
	for _, target := range targets {
		if !e.hasBody(target) || len(target.Params) < 2 {
			continue
		}
		id := e.newSource(&source{Kind: "positional", Use: use, Func: shortFunc(target), Pos: e.pos(target.Pos())})
		e.seedParam(target, 1, id)
	}
}

func derefStruct(t types.Type) (*types.Struct, bool) {
	if p, ok := t.Underlying().(*types.Pointer); ok {
		t = p.Elem()
	}
	s, ok := t.Underlying().(*types.Struct)
	return s, ok
}

// commandUse finds the constant Use string stored into the same cobra.Command.
func commandUse(fn *ssa.Function, cmd ssa.Value) string {
	for _, b := range fn.Blocks {
		for _, instr := range b.Instrs {
			s, ok := instr.(*ssa.Store)
			if !ok {
				continue
			}
			fa, ok := s.Addr.(*ssa.FieldAddr)
			if !ok || fa.X != cmd {
				continue
			}
			st, ok := derefStruct(fa.X.Type())
			if !ok || fa.Field >= st.NumFields() || st.Field(fa.Field).Name() != "Use" {
				continue
			}
			if use, ok := constString(s.Val); ok {
				return use
			}
		}
	}
	return ""
}

// seedEntryPoints seeds the sources that are not calls: MCP tool handlers
// (any module function taking an mcp-go CallToolRequest) and os.Args.
func (e *engine) seedEntryPoints() {
	for _, fn := range e.funcs {
		for i, p := range fn.Params {
			if qualifiedTypeName(p.Type()) == "github.com/mark3labs/mcp-go/mcp.CallToolRequest" {
				id := e.newSource(&source{Kind: "mcp-request", Func: shortFunc(fn), Pos: e.pos(fn.Pos())})
				e.seedParam(fn, i, id)
			}
		}
	}
	if osPkg := e.prog.ImportedPackage("os"); osPkg != nil {
		if g, ok := osPkg.Members["Args"].(*ssa.Global); ok {
			id := e.newSource(&source{Kind: "os-args", Name: "os.Args", Pos: "os.Args"})
			e.obj(g).add(id)
		}
	}
}
