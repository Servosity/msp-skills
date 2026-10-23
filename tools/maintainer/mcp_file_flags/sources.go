// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package main

import (
	"go/constant"
	"go/types"
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
