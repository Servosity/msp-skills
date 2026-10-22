// Command declgraph prints the resolved reference graph of every top-level
// declaration in one Go module, for check_security_gate.py's template
// approvals. It uses only the standard library, so building it pulls nothing
// from the network.
//
// The gate asks one question of this graph: is the code a security finding
// sits in, the code it depends on, and the code that can feed it, the same
// code a human reviewed in the printing-press template? A text hash alone
// cannot answer that (an identical function can call a different function, or
// a shadowed builtin, or a swapped import), so each declaration is hashed
// together with the fully resolved identity of everything it references, as
// computed by go/types on this exact build target.
//
// Usage (run inside the module directory; GOOS/GOARCH select the target):
//
//	declgraph -slug <slug>
//
// Output: one JSON object on stdout. Any load or type error is reported in
// "errors"; the gate treats a non-empty list as "not established".
package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Placeholders that replace the connector's own names before hashing, so the
// same template rendered for two vendors compares equal. A source file that
// already contains one of them is refused: that keeps the mapping injective.
const (
	phMod   = "{MOD}"
	phEnv   = "{ENV}"
	phSlug  = "{SLUG}"
	phSnake = "{SNAKE}"
)

type listModule struct {
	Path    string
	Version string
	Main    bool
}

type listError struct{ Err string }

type listPkg struct {
	ImportPath string
	Dir        string
	Name       string
	GoFiles    []string
	CgoFiles   []string
	SFiles     []string
	SysoFiles  []string
	Export     string
	Standard   bool
	Module     *listModule
	Deps       []string
	Error      *listError
	DepsErrors []*listError
}

type unit struct {
	Key   string   `json:"key"`
	Kind  string   `json:"kind"`
	File  string   `json:"file"`
	Start int      `json:"start"`
	End   int      `json:"end"`
	Hash  string   `json:"hash"`
	Refs  []string `json:"refs"`
	Ext   []string `json:"ext"`
	Mods  []string `json:"mods"`
	// Embed marks a variable filled by //go:embed: its contents are files the
	// declaration hash does not cover, so it can never be part of an approval.
	Embed bool `json:"embed"`

	refs map[string]bool
	ext  map[string]bool
	mods map[string]bool
	text string
}

type output struct {
	Module    string              `json:"module"`
	GoLine    string              `json:"go_line"`
	Target    string              `json:"target"`
	GoVersion string              `json:"go_version"`
	Units     map[string]*unit    `json:"units"`
	Users     map[string][]string `json:"users"`
	Methods   map[string][]string `json:"methods"`
	Files     map[string]string   `json:"files"`
	Errors    []string            `json:"errors"`
}

type normalizer struct{ pairs [][2]string }

func newNormalizer(modPath, slug string) normalizer {
	env := strings.ToUpper(strings.ReplaceAll(slug, "-", "_"))
	snake := strings.ReplaceAll(slug, "-", "_")
	pairs := [][2]string{{modPath, phMod}, {env, phEnv}, {slug, phSlug}}
	if snake != slug {
		pairs = append(pairs, [2]string{snake, phSnake})
	}
	return normalizer{pairs: pairs}
}

func (n normalizer) apply(s string) string {
	for _, p := range n.pairs {
		if p[0] != "" {
			s = strings.ReplaceAll(s, p[0], p[1])
		}
	}
	return s
}

func hasPlaceholder(s string) bool {
	for _, p := range []string{phMod, phEnv, phSlug, phSnake, "\x00"} {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}

func readModule(dir string) (path, goLine string, err error) {
	f, err := os.Open(filepath.Join(dir, "go.mod"))
	if err != nil {
		return "", "", err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 2 && fields[0] == "module" {
			path = strings.Trim(fields[1], `"`)
		}
		if len(fields) == 2 && fields[0] == "go" {
			goLine = fields[1]
		}
	}
	if path == "" {
		return "", "", fmt.Errorf("go.mod has no module line")
	}
	return path, goLine, sc.Err()
}

func goList(dir string) ([]*listPkg, error) {
	cmd := exec.Command("go", "list", "-e", "-json", "-export", "-deps", "-mod=readonly", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOWORK=off", "GOFLAGS=")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list: %v: %s", err, strings.TrimSpace(stderr.String()))
	}
	dec := json.NewDecoder(bytes.NewReader(out))
	var pkgs []*listPkg
	for {
		p := new(listPkg)
		if err := dec.Decode(p); err == io.EOF {
			break
		} else if err != nil {
			return nil, fmt.Errorf("go list output: %v", err)
		}
		pkgs = append(pkgs, p)
	}
	return pkgs, nil
}

func goEnv(dir, name string) string {
	cmd := exec.Command("go", "env", name)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

type state struct {
	fset     *token.FileSet
	norm     normalizer
	modPath  string
	modDir   string
	byPath   map[string]*listPkg
	modOf    map[string][]string // external package path -> sorted module@version set (itself + deps)
	units    map[string]*unit
	objUnit  map[types.Object]string // package-level object or field -> unit key
	methods  map[string][]string
	named    []*types.TypeName // module named types (for dynamic dispatch targets)
	errs     []string
	srcPkgs  map[string]*types.Package
	gcImport types.Importer
}

func (s *state) Import(path string) (*types.Package, error) {
	if p, ok := s.srcPkgs[path]; ok {
		return p, nil
	}
	return s.gcImport.Import(path)
}

func (s *state) isModulePkg(p *types.Package) bool {
	if p == nil {
		return false
	}
	_, ok := s.srcPkgs[p.Path()]
	return ok
}

func (s *state) npath(p string) string { return s.norm.apply(p) }

func recvBase(t types.Type) *types.TypeName {
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	switch n := t.(type) {
	case *types.Named:
		return n.Origin().Obj()
	case *types.Alias:
		return recvBase(types.Unalias(n))
	}
	return nil
}

func (s *state) objKey(obj types.Object) string {
	pp := s.npath(obj.Pkg().Path())
	switch o := obj.(type) {
	case *types.Func:
		if sig, ok := o.Type().(*types.Signature); ok && sig.Recv() != nil {
			if tn := recvBase(sig.Recv().Type()); tn != nil {
				return "method " + pp + "." + tn.Name() + "." + o.Name()
			}
		}
		return "func " + pp + "." + o.Name()
	case *types.Var:
		return "var " + pp + "." + o.Name()
	case *types.Const:
		return "const " + pp + "." + o.Name()
	case *types.TypeName:
		return "type " + pp + "." + o.Name()
	}
	return "obj " + pp + "." + obj.Name()
}

func (s *state) newUnit(key, kind, file string, fset *token.FileSet, start, end token.Pos, src []byte) *unit {
	sp, ep := fset.Position(start), fset.Position(end)
	u := &unit{Key: key, Kind: kind, File: file, Start: sp.Line, End: ep.Line,
		refs: map[string]bool{}, ext: map[string]bool{}, mods: map[string]bool{}}
	if sp.Offset >= 0 && ep.Offset <= len(src) && sp.Offset <= ep.Offset {
		u.text = string(src[sp.Offset:ep.Offset])
	}
	if hasPlaceholder(u.text) {
		s.errs = append(s.errs, fmt.Sprintf("%s: source contains a normalization placeholder", key))
	}
	if _, dup := s.units[key]; dup {
		s.errs = append(s.errs, fmt.Sprintf("duplicate unit key %s", key))
	}
	s.units[key] = u
	return u
}

// qual prints module packages with the connector's name replaced and every other
// package exactly. External identities are never normalized, so a text match that
// only normalization produced still fails on what the names resolve to.
func (s *state) qual(p *types.Package) string {
	if s.isModulePkg(p) {
		return s.npath(p.Path())
	}
	return p.Path()
}

func (s *state) addExtObj(u *unit, obj types.Object) {
	p := obj.Pkg()
	path := p.Path()
	desc := ""
	switch o := obj.(type) {
	case *types.Func:
		if sig, ok := o.Type().(*types.Signature); ok && sig.Recv() != nil {
			desc = "extmethod " + path + "." + types.TypeString(sig.Recv().Type(), s.qual) + "." + o.Name()
		} else {
			desc = "ext " + path + "." + o.Name()
		}
	case *types.Var:
		if o.IsField() {
			desc = "extfield " + path + "." + o.Name() + " " + types.TypeString(o.Type(), s.qual)
		} else {
			desc = "ext " + path + "." + o.Name()
		}
	default:
		desc = "ext " + path + "." + obj.Name()
	}
	u.ext[desc] = true
	for _, m := range s.modOf[path] {
		u.mods[m] = true
	}
}

func (s *state) addObj(u *unit, obj types.Object) {
	if obj == nil {
		return
	}
	switch o := obj.(type) {
	case *types.Func:
		obj = o.Origin()
	case *types.Var:
		obj = o.Origin()
	}
	if pn, ok := obj.(*types.PkgName); ok {
		ip := pn.Imported().Path()
		if s.isModulePkg(pn.Imported()) {
			u.ext["import "+s.npath(ip)] = true
		} else {
			u.ext["import "+ip] = true
			for _, m := range s.modOf[ip] {
				u.mods[m] = true
			}
		}
		return
	}
	if obj.Pkg() == nil {
		u.ext["universe "+obj.Name()] = true
		return
	}
	if !s.isModulePkg(obj.Pkg()) {
		s.addExtObj(u, obj)
		return
	}
	if k, ok := s.objUnit[obj]; ok {
		if k != u.Key {
			u.refs[k] = true
		}
		return
	}
	if f, ok := obj.(*types.Func); ok {
		if sig, ok := f.Type().(*types.Signature); ok && sig.Recv() != nil {
			if tn := recvBase(sig.Recv().Type()); tn != nil {
				if _, isIface := tn.Type().Underlying().(*types.Interface); isIface {
					u.refs[s.objKey(tn)] = true
					return
				}
			}
			u.refs[s.objKey(f)] = true
			return
		}
	}
	// Anything else is local to a declaration (parameters, locals, type
	// parameters, labels). Its text is part of the enclosing unit's hash.
}

// dynTargets adds every module method that a call through interface method
// `name` of `iface` could dispatch to. This is an over-approximation: any
// module type that implements the interface counts, whether or not a value of
// it can reach the call.
func (s *state) dynTargets(u *unit, iface *types.Interface, name string) {
	for _, tn := range s.named {
		if _, isIface := tn.Type().Underlying().(*types.Interface); isIface {
			continue
		}
		t := tn.Type()
		if types.Implements(t, iface) || types.Implements(types.NewPointer(t), iface) {
			obj, _, _ := types.LookupFieldOrMethod(types.NewPointer(t), true, tn.Pkg(), name)
			if f, ok := obj.(*types.Func); ok && s.isModulePkg(f.Pkg()) {
				u.refs[s.objKey(f.Origin())] = true
			}
		}
	}
}

func (s *state) collect(u *unit, node ast.Node, info *types.Info) {
	ast.Inspect(node, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.Ident:
			s.addObj(u, info.Uses[x])
		case *ast.SelectorExpr:
			// A method whose own receiver is an interface is dispatched
			// dynamically, whether it is selected on the interface itself or
			// promoted through a struct that embeds it.
			if sel, ok := info.Selections[x]; ok {
				if f, ok := sel.Obj().(*types.Func); ok {
					if sig, ok := f.Type().(*types.Signature); ok && sig.Recv() != nil {
						if iface, ok := sig.Recv().Type().Underlying().(*types.Interface); ok {
							s.dynTargets(u, iface, f.Name())
						}
					}
				}
			}
		}
		return true
	})
}

func (s *state) finish(u *unit) {
	// Check the text that is actually hashed, not only the source: a constant
	// is hashed by its evaluated value, and a string escape ("\x7bENV\x7d") can
	// spell a placeholder the source never contains.
	if hasPlaceholder(u.text) {
		s.errs = append(s.errs, fmt.Sprintf("%s: hashed text contains a normalization placeholder", u.Key))
	}
	u.Refs, u.Ext, u.Mods = []string{}, []string{}, []string{}
	for k := range u.refs {
		u.Refs = append(u.Refs, k)
	}
	for k := range u.ext {
		u.Ext = append(u.Ext, k)
	}
	for k := range u.mods {
		u.Mods = append(u.Mods, k)
	}
	sort.Strings(u.Refs)
	sort.Strings(u.Ext)
	sort.Strings(u.Mods)
	h := sha256.New()
	fmt.Fprintf(h, "%s\n%s\n", u.Kind, s.norm.apply(u.text))
	for _, r := range u.Refs {
		fmt.Fprintf(h, "ref %s\n", r)
	}
	for _, r := range u.Ext {
		fmt.Fprintf(h, "%s\n", r)
	}
	u.Hash = hex.EncodeToString(h.Sum(nil))
}

func main() {
	slug := flag.String("slug", "", "connector slug (for name normalization)")
	flag.Parse()
	if *slug == "" {
		fmt.Fprintln(os.Stderr, "declgraph: -slug is required")
		os.Exit(2)
	}
	dir, _ := os.Getwd()
	out := output{Units: map[string]*unit{}, Users: map[string][]string{}, Methods: map[string][]string{},
		Files: map[string]string{}, Errors: []string{}}
	emit := func() {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", " ")
		_ = enc.Encode(out)
	}
	modPath, goLine, err := readModule(dir)
	if err != nil {
		out.Errors = append(out.Errors, err.Error())
		emit()
		return
	}
	out.Module, out.GoLine = modPath, goLine
	if hasPlaceholder(modPath) {
		out.Errors = append(out.Errors, "module path contains a normalization placeholder")
		emit()
		return
	}
	goos, goarch := goEnv(dir, "GOOS"), goEnv(dir, "GOARCH")
	out.Target = goos + "/" + goarch
	out.GoVersion = goEnv(dir, "GOVERSION")

	pkgs, err := goList(dir)
	if err != nil {
		out.Errors = append(out.Errors, err.Error())
		emit()
		return
	}
	s := &state{fset: token.NewFileSet(), norm: newNormalizer(modPath, *slug), modPath: modPath, modDir: dir,
		byPath: map[string]*listPkg{}, modOf: map[string][]string{}, units: map[string]*unit{},
		objUnit: map[types.Object]string{}, methods: map[string][]string{}, srcPkgs: map[string]*types.Package{}}
	for _, p := range pkgs {
		s.byPath[p.ImportPath] = p
	}
	for _, p := range pkgs {
		if p.Module != nil && p.Module.Main {
			continue
		}
		set := map[string]bool{}
		for _, d := range append([]string{p.ImportPath}, p.Deps...) {
			if q := s.byPath[d]; q != nil && q.Module != nil && !q.Module.Main {
				set[q.Module.Path+"@"+q.Module.Version] = true
			}
		}
		var mods []string
		for m := range set {
			mods = append(mods, m)
		}
		sort.Strings(mods)
		s.modOf[p.ImportPath] = mods
	}
	s.gcImport = importer.ForCompiler(s.fset, "gc", func(path string) (io.ReadCloser, error) {
		p := s.byPath[path]
		if p == nil || p.Export == "" {
			return nil, fmt.Errorf("no export data for %s", path)
		}
		return os.Open(p.Export)
	})

	type checked struct {
		lp    *listPkg
		files []*ast.File
		srcs  [][]byte
		rels  []string
		info  *types.Info
		pkg   *types.Package
	}
	var mod []*checked
	sizes := types.SizesFor("gc", goarch)
	goVer := ""
	if goLine != "" {
		goVer = "go" + goLine
	}
	// go list -deps prints dependencies before dependents, so a module
	// package is always checked after the module packages it imports.
	for _, p := range pkgs {
		if p.Module == nil || !p.Module.Main {
			continue
		}
		if p.Error != nil {
			s.errs = append(s.errs, fmt.Sprintf("%s: %s", p.ImportPath, p.Error.Err))
			continue
		}
		for _, de := range p.DepsErrors {
			s.errs = append(s.errs, fmt.Sprintf("%s: dependency error: %s", p.ImportPath, de.Err))
		}
		if len(p.CgoFiles) > 0 || len(p.SFiles) > 0 || len(p.SysoFiles) > 0 {
			// cgo, assembly and prebuilt .syso objects can supply function bodies
			// or call into Go code without a reference go/types can see.
			s.errs = append(s.errs, fmt.Sprintf("%s: cgo, assembly or .syso files are not analyzable", p.ImportPath))
			continue
		}
		c := &checked{lp: p, info: &types.Info{
			Types: map[ast.Expr]types.TypeAndValue{}, Defs: map[*ast.Ident]types.Object{},
			Uses: map[*ast.Ident]types.Object{}, Selections: map[*ast.SelectorExpr]*types.Selection{},
			Implicits: map[ast.Node]types.Object{}, Instances: map[*ast.Ident]types.Instance{}}}
		for _, name := range p.GoFiles {
			full := filepath.Join(p.Dir, name)
			src, err := os.ReadFile(full)
			if err != nil {
				s.errs = append(s.errs, err.Error())
				continue
			}
			f, err := parser.ParseFile(s.fset, full, src, parser.ParseComments|parser.SkipObjectResolution)
			if err != nil {
				s.errs = append(s.errs, err.Error())
				continue
			}
			for _, cg := range f.Comments {
				for _, cm := range cg.List {
					switch {
					case strings.HasPrefix(cm.Text, "//go:linkname"):
						// Reaches an unexported function from another package with
						// no reference go/types can see.
						s.errs = append(s.errs, fmt.Sprintf("%s: //go:linkname directive", full))
					case strings.HasPrefix(cm.Text, "//line"), strings.HasPrefix(cm.Text, "/*line"):
						// A line directive renumbers the positions scanners report:
						// gosec would place a finding from new code at a line
						// inside a reviewed declaration.
						s.errs = append(s.errs, fmt.Sprintf("%s: line directive", full))
					}
				}
			}
			rel, _ := filepath.Rel(dir, full)
			if hasPlaceholder(rel) || hasPlaceholder(p.ImportPath) {
				s.errs = append(s.errs, fmt.Sprintf("%s: path contains a normalization placeholder", rel))
			}
			c.files = append(c.files, f)
			c.srcs = append(c.srcs, src)
			c.rels = append(c.rels, filepath.ToSlash(rel))
		}
		conf := types.Config{Importer: s, Sizes: sizes, GoVersion: goVer,
			Error: func(e error) { s.errs = append(s.errs, e.Error()) }}
		tp, _ := conf.Check(p.ImportPath, s.fset, c.files, c.info)
		c.pkg = tp
		s.srcPkgs[p.ImportPath] = tp
		mod = append(mod, c)
	}

	// Pass 1: create units and map every package-level object and struct
	// field to the unit that declares it.
	type pending struct {
		u    *unit
		node ast.Node
		info *types.Info
	}
	var work []pending
	for _, c := range mod {
		pp := s.npath(c.lp.ImportPath)
		scope := c.pkg.Scope()
		for _, name := range scope.Names() {
			if tn, ok := scope.Lookup(name).(*types.TypeName); ok && !tn.IsAlias() {
				s.named = append(s.named, tn)
			}
		}
		for i, f := range c.files {
			rel := s.norm.apply(c.rels[i])
			out.Files[c.rels[i]] = rel
			src := c.srcs[i]
			fu := s.newUnit("file "+rel, "file", rel, s.fset, f.FileStart, f.FileEnd, src)
			s.finish(fu)
			initN := 0
			var importDecls []*ast.GenDecl
			for _, d := range f.Decls {
				switch d := d.(type) {
				case *ast.FuncDecl:
					if d.Body == nil {
						// A body-less declaration is implemented outside Go source.
						s.errs = append(s.errs, fmt.Sprintf("%s: function %s has no Go body", rel, d.Name.Name))
					}
					start := d.Pos()
					if d.Doc != nil {
						start = d.Doc.Pos()
					}
					obj := c.info.Defs[d.Name]
					var key, kind string
					switch {
					case d.Recv == nil && d.Name.Name == "init":
						initN++
						key, kind = fmt.Sprintf("init %s@%s#%d", pp, rel, initN), "init"
					case d.Recv != nil:
						key, kind = "method ?", "method"
						if fobj, ok := obj.(*types.Func); ok {
							key = s.objKey(fobj)
							if sig, ok := fobj.Type().(*types.Signature); ok && sig.Recv() != nil {
								if tn := recvBase(sig.Recv().Type()); tn != nil {
									tk := s.objKey(tn)
									s.methods[tk] = append(s.methods[tk], key)
								}
							}
						}
					default:
						key, kind = "func "+pp+"."+d.Name.Name, "func"
					}
					u := s.newUnit(key, kind, rel, s.fset, start, d.End(), src)
					if obj != nil {
						s.objUnit[obj] = key
					}
					work = append(work, pending{u, d, c.info})
				case *ast.GenDecl:
					if d.Tok == token.IMPORT {
						importDecls = append(importDecls, d)
						continue
					}
					for _, sp := range d.Specs {
						start := sp.Pos()
						switch sp := sp.(type) {
						case *ast.TypeSpec:
							if sp.Doc != nil {
								start = sp.Doc.Pos()
							} else if len(d.Specs) == 1 && d.Doc != nil {
								start = d.Doc.Pos()
							}
							key := "type " + pp + "." + sp.Name.Name
							u := s.newUnit(key, "type", rel, s.fset, start, sp.End(), src)
							if obj := c.info.Defs[sp.Name]; obj != nil {
								s.objUnit[obj] = key
							}
							work = append(work, pending{u, sp, c.info})
						case *ast.ValueSpec:
							if d.Tok == token.CONST {
								// One unit per constant, hashed by its type and exact
								// value: that is all a constant is, and it makes
								// iota and implicit repetition in a group exact.
								for _, nm := range sp.Names {
									obj, _ := c.info.Defs[nm].(*types.Const)
									key := "const " + pp + "." + nm.Name
									if nm.Name == "_" {
										key = fmt.Sprintf("const %s._@%s:%d", pp, rel, s.fset.Position(nm.Pos()).Line)
									}
									u := s.newUnit(key, "const", rel, s.fset, sp.Pos(), sp.End(), src)
									if obj != nil {
										s.objUnit[obj] = key
										u.text = nm.Name + " " + types.TypeString(obj.Type(), s.qual) + " = " + obj.Val().ExactString()
										if tn := recvBase(obj.Type()); tn != nil && s.isModulePkg(tn.Pkg()) {
											u.refs[s.objKey(tn)] = true
										}
									}
									s.finish(u)
								}
								continue
							}
							var names []string
							for _, nm := range sp.Names {
								names = append(names, nm.Name)
							}
							key := "var " + pp + "." + strings.Join(names, ",")
							if len(names) == 1 && names[0] == "_" {
								key = fmt.Sprintf("var %s._@%s:%d", pp, rel, s.fset.Position(sp.Pos()).Line)
							}
							u := s.newUnit(key, "var", rel, s.fset, start, sp.End(), src)
							for _, cg := range []*ast.CommentGroup{sp.Doc, d.Doc} {
								if cg == nil {
									continue
								}
								for _, cm := range cg.List {
									if strings.HasPrefix(cm.Text, "//go:embed") {
										u.Embed = true
									}
								}
							}
							for _, nm := range sp.Names {
								if obj := c.info.Defs[nm]; obj != nil {
									s.objUnit[obj] = key
								}
							}
							work = append(work, pending{u, sp, c.info})
						}
					}
				}
			}
			if len(importDecls) > 0 {
				u := s.newUnit("imports "+pp+"@"+rel, "imports", rel, s.fset, importDecls[0].Pos(), importDecls[len(importDecls)-1].End(), src)
				var parts []string
				for _, d := range importDecls {
					st, en := s.fset.Position(d.Pos()).Offset, s.fset.Position(d.End()).Offset
					parts = append(parts, string(src[st:en]))
					for _, sp := range d.Specs {
						// Record the resolved import path for every import,
						// including blank ones: a blank import still runs that
						// package's init and links its module into the binary.
						ip := strings.Trim(sp.(*ast.ImportSpec).Path.Value, "`\"")
						if _, ok := s.srcPkgs[ip]; ok {
							u.ext["import "+s.npath(ip)] = true
						} else {
							u.ext["import "+ip] = true
							for _, m := range s.modOf[ip] {
								u.mods[m] = true
							}
						}
					}
				}
				u.text = strings.Join(parts, "\n")
				s.finish(u)
			}
		}
		// Struct fields map to the unit that declares their struct type, so a
		// field access depends on that declaration.
		for _, w := range work {
			if w.info != c.info {
				continue
			}
			ast.Inspect(w.node, func(n ast.Node) bool {
				if st, ok := n.(*ast.StructType); ok {
					for _, fl := range st.Fields.List {
						for _, nm := range fl.Names {
							if obj := c.info.Defs[nm]; obj != nil {
								s.objUnit[obj] = w.u.Key
							}
						}
						if len(fl.Names) == 0 {
							// Embedded field: go/types records the field *Var in
							// Defs under the embedded type's identifier.
							var id *ast.Ident
							switch t := fl.Type.(type) {
							case *ast.Ident:
								id = t
							case *ast.StarExpr:
								if x, ok := t.X.(*ast.Ident); ok {
									id = x
								} else if sel, ok := t.X.(*ast.SelectorExpr); ok {
									id = sel.Sel
								}
							case *ast.SelectorExpr:
								id = t.Sel
							}
							if id != nil {
								if obj := c.info.Defs[id]; obj != nil {
									s.objUnit[obj] = w.u.Key
								}
							}
						}
					}
				}
				return true
			})
		}
	}
	// Pass 2: resolve references now that every unit exists.
	for _, w := range work {
		s.collect(w.u, w.node, w.info)
		s.finish(w.u)
	}
	for k, u := range s.units {
		for _, r := range u.Refs {
			out.Users[r] = append(out.Users[r], k)
		}
	}
	for k := range out.Users {
		sort.Strings(out.Users[k])
	}
	for k := range s.methods {
		sort.Strings(s.methods[k])
	}
	out.Units = s.units
	out.Methods = s.methods
	out.Errors = append(out.Errors, s.errs...)
	emit()
}
