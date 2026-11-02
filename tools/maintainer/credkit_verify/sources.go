// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package main

// Rule 4, sources: a read the ledger calls a source reads the file Start
// held. The proof is by shape, never by equal-looking values: via is a pure
// function of the command line (it returns the same path for the same
// command line), Files returns via's call on every return, and the read's
// path is via's call with the same command line. Rule 3 then classifies the
// type the read actually decodes into, wherever it is declared.

import (
	"fmt"
	"go/ast"
	"go/types"
	"path/filepath"
	"strings"
)

type decoded struct {
	t types.Type
	f Finding // the read
}

// sources proves each "source" ledger entry and follows its reads into the
// types they decode into.
func (p *pass) sources(d *decls) {
	envVia, anyVia := false, false
	argsVias := map[*types.Func]bool{}
	for _, e := range p.ledger.Sites {
		if e.Kind != "source" || e.Via == "" {
			continue
		}
		at := Finding{Rule: 4, File: ledgerName, What: "via"}
		path, name, _ := cutLast(e.Via, ".")
		var via *types.Func
		for _, pk := range p.pkgs {
			if pk.PkgPath == p.mod+"/"+path {
				via, _ = pk.Types.Scope().Lookup(name).(*types.Func)
			}
		}
		if via == nil {
			at.Msg, at.Fix = fmt.Sprintf("via %q is not a package-level function of this connector", e.Via), `name it as "<package path in the module>.<Func>"`
			p.out = append(p.out, at)
			continue
		}
		argsForm, ok := viaShape(via)
		if !ok {
			at.Msg, at.Fix = fmt.Sprintf("via %s is not func() string or func(args []string) string, so the path it gives could depend on what each caller passes", e.Via), "make via take nothing, or only the command line"
			p.out = append(p.out, at)
			continue
		}
		anyVia = true
		q := &purity{p: p, seen: map[proof]bool{}}
		restricted := map[int]bool{}
		if argsForm {
			restricted[0] = true
			argsVias[via.Origin()] = true
		}
		if n, why := q.fn(via, restricted); why != "" {
			f := at
			if n != nil {
				f = p.at(n, nil, 4, "via", "", "")
			}
			f.What, f.Msg, f.Fix = "via", fmt.Sprintf("via %s is not a pure function of the command line: it %s", e.Via, why), "compute the path from constants, the environment and the command line only, with no package-level state"
			p.out = append(p.out, f)
		}
		envVia = envVia || q.env
		if why := p.filesReturns(d, via, argsForm); why != "" {
			at.Msg, at.Fix = fmt.Sprintf("%s, so the file the entry for %s in %s reads may not be one Start holds", why, e.What, e.File), "return []string{"+viaSpelling(via, argsForm)+", ...} on every return of Files"
			p.out = append(p.out, at)
		} else if body, info, param, _ := p.filesFunc(d); body != nil {
			fq := &purity{p: p, seen: map[proof]bool{proofOf(via, restricted): true}} // via is proved above
			params := map[types.Object]bool{}
			if param != nil {
				params[param] = true
			}
			if n, why := fq.body(info, body, params); why != "" {
				f := p.at(n, nil, 4, "Files", "", "")
				f.Msg, f.Fix = "Declarations.Files is not a pure function of the command line: it "+why, "build the list from path functions only"
				p.out = append(p.out, f)
			}
			envVia = envVia || fq.env
		}
		if p.ssa != nil {
			p.ssa.sourceDecoded(e)
		}
		for _, r := range p.reads {
			if r.f.File != e.File || r.f.Func != e.Func || r.f.What != e.What {
				continue
			}
			if r.call == nil || isFunc(orNil(calleeObj(r.info, r.call.Fun))) && !isPkgFunc(calleeObj(r.info, r.call.Fun)) || len(r.call.Args) == 0 ||
				!p.fromVia(r.info, r.encl, r.call.Args[0], via, argsForm, map[*types.Func]bool{}) {
				f := r.f
				f.Site, f.Read = false, false
				f.Msg = "a read listed as a source whose path is not " + viaSpelling(via, argsForm) + " (as " + e.Via + ", with os.Args[1:] for the command line), directly or through parameters every caller fills with one, so the checker cannot prove Files names the file"
				f.Fix = "pass " + e.Via + "(...) as the path, or list the read as exempt if it reads no credential file"
				p.out = append(p.out, f)
			}
			ts, why := p.followRead(r)
			if why != "" {
				f := r.f
				f.Rule, f.Site, f.Read = 3, false, false
				f.Msg = "a source read whose content the checker cannot follow into the struct it decodes into: " + why
				f.Fix = "decode what it reads straight into a struct in the same function: Unmarshal(b, &cfg), or NewDecoder(f).Decode(&cfg)"
				p.out = append(p.out, f)
			}
			for _, t := range ts {
				p.decoded = append(p.decoded, decoded{t, r.f})
				if m := p.selfDecoder(t, map[types.Type]bool{}); m != "" {
					f := r.f
					f.Rule, f.Site, f.Read = 3, false, false
					f.What = m
					f.Msg = "a source read decodes into a type that decodes itself (" + m + "), so what the file holds goes where the checker cannot follow it"
					f.Fix = "decode into plain structs and fields; convert after decoding"
					p.out = append(p.out, f)
				}
			}
		}
	}
	if envVia {
		for _, f := range p.setenvs {
			f.Msg, f.Fix = "an environment write, while a credential file's path reads the environment: the path Start held may then not be the path read", "do not change the environment in the program"
			p.out = append(p.out, f)
		}
	}
	if anyVia {
		for _, f := range p.stdState {
			f.Msg, f.Fix = "a variable of a package path functions call, used other than as an argument of that package's functions, while a credential file is declared: changing it changes the path a path function returns", "pass it only to its own package's functions"
			p.out = append(p.out, f)
		}
		for _, f := range p.chdirs {
			f.Msg, f.Fix = "a working-directory change, while a credential file is declared: a relative path Start held may then not be the file read", "do not change the working directory in the program"
			p.out = append(p.out, f)
		}
	}
	if len(argsVias) > 0 {
		for _, a := range p.osArgs {
			if a.encl != nil && argsVias[a.encl] {
				continue // os.Args[1:] handed to a via, proved pure above
			}
			f := a.f
			f.Msg, f.Fix = "os.Args used where it can change (assigned, sliced or handed on) while a credential file's path is read from the command line", "only read os.Args (len, an index), and pass os.Args[1:] only to the path function"
			p.out = append(p.out, f)
		}
	}
}

// fromVia reports whether e is via's call, or a parameter of encl (never
// changed in its body) that every call of encl fills from via.
func (p *pass) fromVia(info *types.Info, encl *types.Func, e ast.Expr, via *types.Func, argsForm bool, seen map[*types.Func]bool) bool {
	if c, ok := ast.Unparen(e).(*ast.CallExpr); ok {
		return viaCall(info, c, via, argsForm, nil)
	}
	if encl == nil || seen[encl] || p.exposed[encl] {
		return false
	}
	j := p.paramIndex(info, encl, e)
	if j < 0 || len(p.refs[encl]) == 0 {
		return false
	}
	seen[encl] = true
	defer delete(seen, encl)
	for _, r := range p.refs[encl] {
		if r.call == nil || r.method || j >= len(r.call.Args) || !p.fromVia(r.info, r.encl, r.call.Args[j], via, argsForm, seen) {
			return false
		}
	}
	return true
}

// byteReads return the file's bytes; fileOpens return the open file.
var (
	byteReads = set("os.ReadFile", "io/ioutil.ReadFile", "os.Root.ReadFile")
	fileOpens = set("os.Open", "os.OpenFile", "os.Root.Open", "os.Root.OpenFile", "os.OpenInRoot")
)

// followRead follows what a source read returns to the types it decodes
// into: its bytes (or the file, through io.ReadAll or a decoder) may only be
// handed to a decoding call of another package, with the target a pointer to
// a struct. Anything else is a reason.
func (p *pass) followRead(r ref) ([]types.Type, string) {
	if r.call == nil {
		return nil, "the read is used as a function value"
	}
	if fn, ok := calleeObj(r.info, r.call.Fun).(*types.Func); ok && fn.Name() == "DecodeFile" && isPkgFunc(fn) {
		if len(r.call.Args) < 2 {
			return nil, "a DecodeFile call without a target"
		}
		t, why := p.structTarget(r.info, r.call.Args[1], fn)
		return one1(t), why
	}
	parent, decl := p.parentOf(r)
	if decl == nil {
		return nil, "the checker cannot find the function the read is in"
	}
	v, why := resultVar(r.info, parent, r.call)
	if why != "" || v == nil {
		return nil, why
	}
	switch {
	case byteReads[r.f.What]:
		return p.followBytes(r.info, decl, v)
	case fileOpens[r.f.What]:
		return p.followFile(r.info, decl, v)
	}
	return nil, r.f.What + " reads in a way the checker does not follow"
}

func one1(t types.Type) []types.Type {
	if t == nil {
		return nil
	}
	return []types.Type{t}
}

// parentOf finds the read call's parent node and its top-level declaration.
func (p *pass) parentOf(r ref) (ast.Node, ast.Node) {
	for i := len(r.stack) - 1; i > 0; i-- {
		if r.stack[i] == r.call {
			return r.stack[i-1], r.stack[1]
		}
	}
	return nil, nil
}

// resultVar is the variable a call's first result lands in: nil (with no
// reason) when it is discarded, a reason when it goes anywhere else.
func resultVar(info *types.Info, parent ast.Node, call *ast.CallExpr) (types.Object, string) {
	var lhs ast.Expr
	switch x := parent.(type) {
	case *ast.ExprStmt:
		return nil, ""
	case *ast.AssignStmt:
		if len(x.Rhs) != 1 || x.Rhs[0] != call || len(x.Lhs) == 0 {
			return nil, "its result is not assigned on its own"
		}
		lhs = x.Lhs[0]
	case *ast.ValueSpec:
		if len(x.Values) != 1 || x.Values[0] != call || len(x.Names) == 0 {
			return nil, "its result is not assigned on its own"
		}
		lhs = x.Names[0]
	default:
		return nil, "its result is used where the checker cannot follow it"
	}
	id, ok := lhs.(*ast.Ident)
	if !ok {
		return nil, "its result is stored somewhere other than a local variable"
	}
	if id.Name == "_" {
		return nil, ""
	}
	o := info.ObjectOf(id)
	if v, ok := o.(*types.Var); !ok || v.Pkg() == nil || v.Parent() == v.Pkg().Scope() {
		return nil, "its result is stored somewhere other than a local variable"
	}
	return o, ""
}

// uses visits every use of v in decl with its stack.
func uses(info *types.Info, decl ast.Node, v types.Object, f func(id *ast.Ident, stack []ast.Node) string) string {
	why := ""
	ast.PreorderStack(decl, nil, func(n ast.Node, stack []ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && why == "" && info.Uses[id] == v {
			why = f(id, stack)
		}
		return why == ""
	})
	return why
}

func isLHS(id *ast.Ident, stack []ast.Node) bool {
	if len(stack) == 0 {
		return false
	}
	if a, ok := stack[len(stack)-1].(*ast.AssignStmt); ok {
		for _, l := range a.Lhs {
			if l == id {
				return true
			}
		}
	}
	return false
}

// decodeCall: id is argument 0 of a call to another package's function with
// a target as argument 1 (json.Unmarshal(b, &cfg), toml.Unmarshal, yaml).
func (p *pass) decodeCall(info *types.Info, id *ast.Ident, stack []ast.Node) (*ast.CallExpr, bool) {
	c, ok := stack[len(stack)-1].(*ast.CallExpr)
	if !ok || len(c.Args) < 1 || c.Args[0] != id {
		return nil, false
	}
	fn, ok := calleeObj(info, c.Fun).(*types.Func)
	if !ok || fn.Pkg() == nil || p.local[fn.Pkg().Path()] || fn.Pkg().Path() == p.kit {
		return nil, false
	}
	return c, true
}

func (p *pass) followBytes(info *types.Info, decl ast.Node, v types.Object) ([]types.Type, string) {
	var ts []types.Type
	why := uses(info, decl, v, func(id *ast.Ident, stack []ast.Node) string {
		if isLHS(id, stack) {
			return ""
		}
		switch x := stack[len(stack)-1].(type) {
		case *ast.BinaryExpr:
			return ""
		case *ast.CallExpr:
			if n := builtinName(info, x.Fun); n == "len" {
				return ""
			}
			if c, ok := p.decodeCall(info, id, stack); ok && len(c.Args) >= 2 && isPkgFunc(calleeObj(info, c.Fun)) {
				t, why := p.structTarget(info, c.Args[1], calleeObj(info, c.Fun))
				if t != nil {
					ts = append(ts, t)
				}
				return why
			}
		}
		return "the bytes it read go where the checker cannot follow them (" + p.posOf(id) + ")"
	})
	return ts, why
}

func (p *pass) followFile(info *types.Info, decl ast.Node, v types.Object) ([]types.Type, string) {
	var ts []types.Type
	why := uses(info, decl, v, func(id *ast.Ident, stack []ast.Node) string {
		if isLHS(id, stack) {
			return ""
		}
		switch x := stack[len(stack)-1].(type) {
		case *ast.BinaryExpr:
			return ""
		case *ast.SelectorExpr:
			if x.X == id && x.Sel.Name == "Close" {
				return ""
			}
		case *ast.CallExpr:
			c, ok := p.decodeCall(info, id, stack)
			if !ok || len(c.Args) != 1 || len(stack) < 2 {
				break
			}
			if objKey(orNil(calleeObj(info, c.Fun))) == "io.ReadAll" {
				b, why := resultVar(info, stack[len(stack)-2], c)
				if why != "" || b == nil {
					return why
				}
				bt, why := p.followBytes(info, decl, b)
				ts = append(ts, bt...)
				return why
			}
			// A decoder: NewDecoder(f).Decode(&cfg), or d := NewDecoder(f) then d.Decode(&cfg).
			if s, ok := stack[len(stack)-2].(*ast.SelectorExpr); ok && s.X == c && s.Sel.Name == "Decode" && len(stack) > 2 {
				if dc, ok := stack[len(stack)-3].(*ast.CallExpr); ok && dc.Fun == s && len(dc.Args) == 1 {
					t, why := p.structTarget(info, dc.Args[0], calleeObj(info, dc.Fun))
					ts = append(ts, one1(t)...)
					return why
				}
			}
			dv, why := resultVar(info, stack[len(stack)-2], c)
			if why != "" || dv == nil {
				return "the file it opened goes where the checker cannot follow it (" + p.posOf(id) + ")"
			}
			return uses(info, decl, dv, func(did *ast.Ident, dstack []ast.Node) string {
				if isLHS(did, dstack) {
					return ""
				}
				if s, ok := dstack[len(dstack)-1].(*ast.SelectorExpr); ok && s.X == did && len(dstack) > 1 {
					switch s.Sel.Name {
					case "DisallowUnknownFields", "UseNumber":
						return ""
					case "Decode":
						if dc, ok := dstack[len(dstack)-2].(*ast.CallExpr); ok && dc.Fun == s && len(dc.Args) == 1 {
							t, why := p.structTarget(info, dc.Args[0], calleeObj(info, dc.Fun))
							ts = append(ts, one1(t)...)
							return why
						}
					}
				}
				return "the decoder reading it goes where the checker cannot follow it (" + p.posOf(did) + ")"
			})
		}
		return "the file it opened goes where the checker cannot follow it (" + p.posOf(id) + ")"
	})
	return ts, why
}

// structTarget is the struct a decode target points to. It records dec's
// format for it (noteFormat), so no decode site can find a target without
// the keys its decoder reads (class decoder-keys).
func (p *pass) structTarget(info *types.Info, e ast.Expr, dec types.Object) (types.Type, string) {
	var t types.Type
	if u, ok := ast.Unparen(e).(*ast.UnaryExpr); ok && u.Op.String() == "&" {
		t = info.TypeOf(u.X)
	} else if pt, ok := types.Unalias(info.TypeOf(e)).(*types.Pointer); ok {
		t = pt.Elem()
	}
	if t == nil {
		return nil, "it decodes into something other than a pointer (" + p.posOf(e) + ")"
	}
	root, ok := derefStruct(t)
	if !ok {
		return nil, "it decodes into " + types.TypeString(t, nil) + ", not a struct whose fields name every key (" + p.posOf(e) + ")"
	}
	p.noteFormat(root, dec)
	return root, ""
}

// derefStruct follows pointers (stopping at a cycle) to a struct type.
func derefStruct(t types.Type) (types.Type, bool) {
	seen := map[types.Type]bool{}
	for !seen[t] {
		seen[t] = true
		pt, ok := types.Unalias(t).Underlying().(*types.Pointer)
		if !ok {
			break
		}
		t = pt.Elem()
	}
	_, ok := types.Unalias(t).Underlying().(*types.Struct)
	return t, ok
}

func (p *pass) posOf(n ast.Node) string {
	pos := p.fset.Position(n.Pos())
	return fmt.Sprintf("%s:%d", filepath.ToSlash(relTo(p.cli, pos.Filename)), pos.Line)
}

// selfDecoder names what in t's structure decodes where the checker cannot
// follow, or "": an interface (a decoder fills whatever value it holds), or a
// decoding method (UnmarshalJSON, UnmarshalText, UnmarshalTOML, GobDecode...)
// other than a concrete standard-library type's, such as time.Time's
// (classes custom-decoding, decode-dynamic).
func (p *pass) selfDecoder(t types.Type, seen map[types.Type]bool) string {
	if t == nil || seen[t] {
		return ""
	}
	seen[t] = true
	if types.IsInterface(t) {
		return "the interface " + types.TypeString(t, nil)
	}
	ms := types.NewMethodSet(types.NewPointer(t))
	for i := 0; i < ms.Len(); i++ {
		m := ms.At(i).Obj().(*types.Func)
		if !strings.HasPrefix(m.Name(), "Unmarshal") && !strings.HasSuffix(m.Name(), "Decode") {
			continue
		}
		recv := m.Signature().Recv().Type()
		if m.Pkg() == nil || !p.std[m.Pkg().Path()] || types.IsInterface(recv) {
			return types.TypeString(t, nil) + "." + m.Name() + " (from " + types.TypeString(recv, nil) + ")"
		}
	}
	switch u := t.Underlying().(type) {
	case *types.Pointer:
		return p.selfDecoder(u.Elem(), seen)
	case *types.Slice:
		return p.selfDecoder(u.Elem(), seen)
	case *types.Array:
		return p.selfDecoder(u.Elem(), seen)
	case *types.Map:
		if m := p.selfDecoder(u.Key(), seen); m != "" {
			return m
		}
		return p.selfDecoder(u.Elem(), seen)
	case *types.Struct:
		for i := 0; i < u.NumFields(); i++ {
			if m := p.selfDecoder(u.Field(i).Type(), seen); m != "" {
				return m
			}
		}
	}
	return ""
}

// noteFormat records the format t is decoded as, by the decoding call's
// package, so its fields are classified under the keys that decoder reads
// (class decoder-keys); "?" when the checker does not know the decoder.
func (p *pass) noteFormat(t types.Type, dec types.Object) {
	if t == nil {
		return
	}
	f := "?"
	if dec != nil && dec.Pkg() != nil {
		switch path := dec.Pkg().Path(); {
		case path == "encoding/json":
			f = "json"
		case strings.Contains(path, "toml"):
			f = "toml"
		case strings.Contains(path, "yaml"):
			f = "yaml"
		case strings.Contains(path, "mapstructure"):
			f = "mapstructure"
		}
	}
	if p.formats[t] == nil {
		p.formats[t] = map[string]bool{}
	}
	p.formats[t][f] = true
}
