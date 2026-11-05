// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package umbrellablackbox

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// fileWrite matches every way this program writes a file past the credential
// kit. A write through credkit.WriteFile, AppendFile, OpenFile, Create or a
// credkit.Writer is checked by the kit, so it does not match.
var fileWrite = regexp.MustCompile(`\.WriteFile\(|ioutil\.WriteFile\(|os\.Create\(|\b[rR]oot\.Create\(|os\.CreateTemp\(|ioutil\.TempFile\(|os\.NewFile\(|AtomicWriteCredentialFile\(|\b(syscall|unix)\.(Write|Pwrite)\(|\.OpenFile\(.*O_(WRONLY|RDWR|APPEND|CREATE)`)

// unfilteredWrites are the raw opens and writes that may skip the kit, with the
// reason; the key is the file and a fragment of the line. Anything else fails this
// test, so a writer a reprint adds fails until it is routed through the kit or
// reviewed, and an entry nothing matches any more fails too, so no reason outlives
// its code. An open is only allowed here when nothing is written through its handle
// (rawHandleWrites) unless rawWrites names that write.
var unfilteredWrites = map[string]string{
	"internal/cliutil/paths.go|os.CreateTemp(dir, \".\"+filepath.Base(path)":             "the private-file writers' temp file: AtomicWritePrivateFile's body goes through the kit, AtomicWriteCredentialFile's is the declared exemption",
	"internal/cliutil/paths.go|func AtomicWriteCredentialFile(":                          "the declared exemption itself: the 0600 credential store and config.toml, both named in Declarations.Files",
	"internal/cliutil/credentials.go|AtomicWriteCredentialFile(path, data":               "the 0600 credential store: the one file allowed to hold a credential",
	"internal/config/config.go|AtomicWriteCredentialFile(":                               "config.toml: named in Declarations.Files, so whatever it holds is held at startup",
	"internal/config/config.go|os.WriteFile(snap.path, snap.data":                        "puts config.toml's own earlier bytes back after a failed save",
	"internal/cliutil/filelock.go|root.OpenFile(filepath.Base(lockPath)":                 "opens the lock file to lock it; it stays empty",
	"internal/cli/store_adopt_linux.go|os.OpenFile(path, os.O_RDWR, 0)":                  "pins the existing store file for a write lease; writes nothing through the handle",
	"internal/store/store.go|os.OpenFile(journalPath":                                    "pre-creates the empty SQLite journal at 0600 and closes it",
	"internal/cli/deliver.go|os.CreateTemp(dir, downloadTempPattern)":                    "an empty temp file it closes; its content goes through credkit.WriteFile",
	"internal/cli/helpers.go|os.CreateTemp(dir, \".data-db-owner-*\")":                   "the ownership marker's temp file, closed empty; the scope hash goes through credkit.WriteFile",
	"internal/platform/profile.go|os.CreateTemp(dir, \".tmp-*\")":                        "the profile writer's temp file, closed empty; the profile goes through credkit.WriteFile",
	"internal/platform/profile.go|os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL":   "claims the fingerprint key's path, closed empty; the key goes through credkit.WriteFile",
	"internal/cliutil/creds_perms_unix.go|os.NewFile(uintptr(fd), name)":                 "wraps the read-only descriptor openatNoFollow opened; writes nothing through it",
	"internal/platform/ratelimit.go|os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL": "claims the rate-limit lock, closed empty; its timestamp goes through credkit.WriteFile",
}

// rawWrites are the writes through a raw handle that may skip the kit, with the
// reason, keyed as unfilteredWrites is. The credential store's temp file is the only
// one: the kit would withhold the store by contract.
var rawWrites = map[string]string{
	"internal/cliutil/paths.go|tmp.Write(data)": "AtomicWriteCredentialFile, the declared exemption",
}

func allowed(rel, line string, list map[string]string) bool {
	return matchedKey(rel, line, list) != ""
}

// matchedKey is the key of list that allows this line, or "".
func matchedKey(rel, line string, list map[string]string) string {
	for key := range list {
		file, frag, _ := strings.Cut(key, "|")
		if file == rel && strings.Contains(line, frag) {
			return key
		}
	}
	return ""
}

// eachSourceLine calls fn for every line of code in the program's own Go files.
func eachSourceLine(t *testing.T, fn func(rel string, n int, code string)) {
	t.Helper()
	root := filepath.Join("..", "..")
	for _, dir := range []string{"internal", "cmd"} {
		_ = filepath.WalkDir(filepath.Join(root, dir), func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if d.Name() == "credkit" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			rel, _ := filepath.Rel(root, path)
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			for i, line := range strings.Split(string(src), "\n") {
				if code := strings.TrimSpace(line); !strings.HasPrefix(code, "//") {
					fn(filepath.ToSlash(rel), i+1, code)
				}
			}
			return nil
		})
	}
}

// sourceFiles is every Go file of the program, name to source: every build tag,
// tests and the vendored kit left out.
func sourceFiles(t *testing.T) map[string]string {
	t.Helper()
	root := filepath.Join("..", "..")
	out := map[string]string{}
	for _, dir := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "credkit" {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
				rel, _ := filepath.Rel(root, path)
				b, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				out[filepath.ToSlash(rel)] = string(b)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return out
}

// handleWrite is one place a raw file handle's bytes can go.
type handleWrite struct {
	rel  string
	line int
	code string
}

// A handle may be read and closed. These reach a writer.
var handleWriters = map[string]bool{"Write": true, "WriteString": true, "WriteAt": true, "ReadFrom": true}

// handleReaders may be handed a raw handle: they only read it.
var handleReaders = map[string]bool{"io.ReadAll": true, "bufio.NewReader": true, "bufio.NewScanner": true, "json.NewDecoder": true, "runtime.KeepAlive": true}

// isOSFile reports the type *os.File.
func isOSFile(e ast.Expr) bool {
	star, ok := e.(*ast.StarExpr)
	if !ok {
		return false
	}
	sel, ok := star.X.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	x, ok := sel.X.(*ast.Ident)
	return ok && x.Name == "os" && sel.Sel.Name == "File"
}

func listHasFile(l *ast.FieldList) bool {
	return l != nil && slices.ContainsFunc(l.List, func(f *ast.Field) bool { return isOSFile(f.Type) })
}

// takesHandle reports a parameter list a raw handle may safely be given: a *os.File,
// whose function is walked in its own right, or an io.Reader, which cannot write.
func takesHandle(l *ast.FieldList) bool {
	return l != nil && slices.ContainsFunc(l.List, func(f *ast.Field) bool { return isOSFile(f.Type) || exprName(f.Type) == "io.Reader" })
}

func exprName(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		if x := exprName(v.X); x != "" {
			return x + "." + v.Sel.Name
		}
	}
	return ""
}

// rawHandleWrites walks every function of files for bytes written through a handle
// from a raw open (os.OpenFile, Create, CreateTemp, NewFile, or a helper that returns
// *os.File) or a *os.File parameter: a Write method, or the handle given to anything
// that is not known to only read it (io.Copy, fmt.Fprint, bufio.NewWriter,
// json.NewEncoder, an interface conversion), or stored where it stops being a
// handle. A local function that takes *os.File is walked in its own right, so handing
// it the handle is not by itself a write; credkit.NewWriter is the kit.
func rawHandleWrites(t *testing.T, files map[string]string) []handleWrite {
	t.Helper()
	type parsed struct {
		rel  string
		file *ast.File
		fset *token.FileSet
	}
	var all []parsed
	takesFile, returnsFile, fileFields := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for rel, src := range files {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, rel, src, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("%s: %v", rel, err)
		}
		all = append(all, parsed{rel, f, fset})
		ast.Inspect(f, func(n ast.Node) bool {
			switch v := n.(type) {
			case *ast.FuncDecl:
				if takesHandle(v.Type.Params) {
					takesFile[v.Name.Name] = true
				}
				if listHasFile(v.Type.Results) {
					returnsFile[v.Name.Name] = true
				}
			case *ast.StructType:
				for _, field := range v.Fields.List {
					if isOSFile(field.Type) {
						for _, name := range field.Names {
							fileFields[name.Name] = true
						}
					}
				}
			case *ast.Field: // a parameter or field of a function type that takes a handle
				if ft, ok := v.Type.(*ast.FuncType); ok && listHasFile(ft.Params) {
					for _, name := range v.Names {
						takesFile[name.Name] = true
					}
				}
			case *ast.ValueSpec: // a variable of a function type that takes a handle
				if ft, ok := v.Type.(*ast.FuncType); ok && listHasFile(ft.Params) {
					for _, name := range v.Names {
						takesFile[name.Name] = true
					}
				}
			}
			return true
		})
	}
	opensRaw := func(e ast.Expr) bool {
		call, ok := e.(*ast.CallExpr)
		if !ok {
			return false
		}
		name := exprName(call.Fun)
		switch base := name[strings.LastIndex(name, ".")+1:]; {
		case strings.HasPrefix(name, "credkit."):
			return false
		case base == "OpenFile" || name == "os.Create" || name == "os.CreateTemp" || name == "os.NewFile":
			return true // an os.Root's OpenFile too
		case base == "Create":
			return strings.HasSuffix(strings.ToLower(name), "root.create") // an os.Root's; a store's Create is no file
		default:
			return returnsFile[name]
		}
	}
	var out []handleWrite
	for _, p := range all {
		lines := strings.Split(files[p.rel], "\n")
		report := func(pos token.Pos) {
			line := p.fset.Position(pos).Line
			out = append(out, handleWrite{p.rel, line, strings.TrimSpace(lines[line-1])})
		}
		ast.Inspect(p.file, func(n ast.Node) bool {
			fd, ok := n.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				return true
			}
			handles := map[string]bool{}
			addParams := func(l *ast.FieldList) {
				for _, field := range l.List {
					if isOSFile(field.Type) {
						for _, name := range field.Names {
							handles[name.Name] = true
						}
					}
				}
			}
			addParams(fd.Type.Params)
			isHandle := func(e ast.Expr) bool {
				switch v := e.(type) {
				case *ast.Ident:
					return handles[v.Name]
				case *ast.SelectorExpr:
					return fileFields[v.Sel.Name]
				}
				return false
			}
			// To a fixed point, so an alias of an alias is a handle wherever it is declared.
			for grew := true; grew; {
				before := len(handles)
				ast.Inspect(fd.Body, func(m ast.Node) bool {
					switch v := m.(type) {
					case *ast.FuncLit:
						addParams(v.Type.Params)
					case *ast.AssignStmt:
						for i, rhs := range v.Rhs {
							if !opensRaw(rhs) && !isHandle(rhs) || i >= len(v.Lhs) {
								continue
							}
							if id, ok := v.Lhs[i].(*ast.Ident); ok && id.Name != "_" {
								handles[id.Name] = true
							}
						}
					case *ast.ValueSpec:
						for i, name := range v.Names {
							if isOSFile(v.Type) || i < len(v.Values) && (opensRaw(v.Values[i]) || isHandle(v.Values[i])) {
								handles[name.Name] = true
							}
						}
					}
					return true
				})
				grew = len(handles) > before
			}
			returns := listHasFile(fd.Type.Results)
			ast.Inspect(fd.Body, func(m ast.Node) bool {
				switch v := m.(type) {
				case *ast.CallExpr:
					if sel, ok := v.Fun.(*ast.SelectorExpr); ok && isHandle(sel.X) && handleWriters[sel.Sel.Name] {
						report(v.Pos())
					}
					name := exprName(v.Fun)
					base := name[strings.LastIndex(name, ".")+1:]
					if name == "credkit.NewWriter" || handleReaders[name] || takesFile[base] {
						return true
					}
					if slices.ContainsFunc(v.Args, isHandle) {
						report(v.Pos())
					}
				case *ast.ReturnStmt:
					if !returns && slices.ContainsFunc(v.Results, isHandle) {
						report(v.Pos())
					}
				case *ast.KeyValueExpr:
					if key, ok := v.Key.(*ast.Ident); ok && isHandle(v.Value) && !fileFields[key.Name] {
						report(v.Pos())
					}
				}
				return true
			})
			return false
		})
	}
	return out
}

// The walk above catches each way a raw handle reaches a writer, and leaves a handle
// that is only read alone: a check that cannot fail proves nothing.
func TestRawHandleWalkSeesEveryShape(t *testing.T) {
	t.Parallel()
	const header = "package p\nimport (\"bufio\";\"encoding/json\";\"fmt\";\"io\";\"os\";\"p/credkit\")\nvar _ = []any{bufio.NewReader, json.NewDecoder, fmt.Sprint, io.EOF, os.Args, credkit.Hold}\n"
	for name, body := range map[string]string{
		"Write":             `func a() { f, _ := os.OpenFile("x", os.O_RDWR, 0); f.Write(nil) }`,
		"WriteString":       `func a() { f, _ := os.Create("x"); f.WriteString("") }`,
		"WriteAt":           `func a() { f, _ := os.CreateTemp("", ""); f.WriteAt(nil, 0) }`,
		"ReadFrom":          `func a() { f := os.NewFile(3, "x"); f.ReadFrom(nil) }`,
		"io.Copy":           `func a() { f, _ := os.Create("x"); io.Copy(f, nil) }`,
		"Fprintf":           `func a() { f, _ := os.Create("x"); fmt.Fprintf(f, "") }`,
		"bufio":             `func a() { f, _ := os.Create("x"); bufio.NewWriter(f) }`,
		"json":              `func a() { f, _ := os.Create("x"); json.NewEncoder(f) }`,
		"alias":             `func a() { f, _ := os.Create("x"); g := f; g.Write(nil) }`,
		"interface":         `func a() { f, _ := os.Create("x"); var w io.Writer = f; w.Write(nil) }`,
		"a helper":          `func o() (*os.File, error) { return os.Create("x") }; func a() { f, _ := o(); f.Write(nil) }`,
		"an os.Root":        `func a(r *os.Root) { f, _ := r.OpenFile("x", os.O_RDWR, 0); f.Write(nil) }`,
		"an os.Root create": `func a(root *os.Root) { f, _ := root.Create("x"); f.Write(nil) }`,
		"a parameter":       `func a(f *os.File) { f.Write(nil) }`,
		"a closure":         `func a() { _ = func(f *os.File) { f.Write(nil) } }`,
		"a local var":       `func a() { var f *os.File; f.Write(nil) }`,
		"a field":           `type s struct{ f *os.File }; func a(v s) { v.f.Write(nil) }`,
		"a wrapper":         `type w struct{ o io.Writer }; func a() { f, _ := os.Create("x"); _ = w{o: f} }`,
		"as an io.Writer":   `func a() io.Writer { f, _ := os.Create("x"); return f }`,
	} {
		if got := rawHandleWrites(t, map[string]string{"p.go": header + body}); len(got) == 0 {
			t.Errorf("%s: the walk missed it: %s", name, body)
		}
	}
	for name, body := range map[string]string{
		"closed":             `func a() { f, _ := os.Create("x"); f.Close() }`,
		"read":               `func a() { f, _ := os.Create("x"); f.Read(nil); f.Stat(); f.Seek(0, 0); io.ReadAll(f) }`,
		"a reader":           `func a() { f, _ := os.Open("x"); bufio.NewReader(f) }`,
		"read-only open":     `func a(b []byte) { f, _ := os.Open("x"); _ = f; var h interface{ Write([]byte) (int, error) }; h.Write(b) }`,
		"the kit":            `func a() { f, _ := os.Create("x"); credkit.NewWriter(f, "s") }`,
		"a local that reads": `func r(f *os.File) { f.Stat() }; func a() { f, _ := os.Create("x"); r(f) }`,
		"returned as a file": `func o() (*os.File, error) { f, err := os.Create("x"); return f, err }`,
	} {
		if got := rawHandleWrites(t, map[string]string{"p.go": header + body}); len(got) != 0 {
			t.Errorf("%s: the walk flagged it: %+v", name, got)
		}
	}
}

// TestEveryFileWriteGoesThroughTheKit: no file but the credential store and the
// config file its exemption names can receive data the kit did not check
// (handfixes.json credkit-wiring). Both halves of a write are checked: the raw open
// the line scan sees, and the handle it returns, which the walk follows.
func TestEveryFileWriteGoesThroughTheKit(t *testing.T) {
	t.Parallel()
	kitWrites := regexp.MustCompile(`credkit\.(WriteFile|AppendFile|OpenFile|Create|NewWriter)\(`)
	var sites, routed int
	used := map[string]bool{}
	eachSourceLine(t, func(rel string, n int, code string) {
		if kitWrites.MatchString(code) {
			routed++
		}
		if !fileWrite.MatchString(kitWrites.ReplaceAllString(code, "")) {
			return
		}
		sites++
		key := matchedKey(rel, code, unfilteredWrites)
		if key == "" {
			t.Errorf("%s:%d writes a file past the kit: %s", rel, n, code)
		}
		used[key] = true
	})
	if sites < 3 || routed < 5 {
		t.Fatalf("found %d raw and %d routed write sites; the scan is not reading the tree", sites, routed)
	}
	for _, w := range rawHandleWrites(t, sourceFiles(t)) {
		key := matchedKey(w.rel, w.code, rawWrites)
		if key == "" {
			t.Errorf("%s:%d writes through a raw file handle, past the kit: %s", w.rel, w.line, w.code)
		}
		used[key] = true
	}
	for _, list := range []map[string]string{unfilteredWrites, rawWrites} {
		for key := range list {
			if !used[key] {
				t.Errorf("%s allows a write the source no longer has; delete it, or its reason is untrue", key)
			}
		}
	}
}

// bareClients are the http.Client literals with no kit transport on their line,
// with the reason.
var bareClients = map[string]string{
	"internal/client/client.go|return &http.Client{Timeout: timeout, Jar: jar, Transport: tr}, nil": "client.New's client: every client.New is followed by cli.ApplyClientHooks (checked below), which puts it under client.OutboundGuard",
	"internal/client/client.go|return &http.Client{Transport: tr}":                                  "StreamingHTTPClient with a nil base; no caller passes nil (checked below)",
	"internal/cli/deliver.go|httpClient := &http.Client{":                                           "the next lines of deliver.go set Transport: credkit.Transport(nil) (checked below)",
}

// Class 6: every HTTP client in the program sends through the kit. A client a
// reprint adds, or a package-level helper that bypasses a client, fails here until
// it is routed or reviewed. TestNoCredentialInAnyOutput and the proxy test drive
// the clients this names; this is what stops a new one arriving unnoticed.
func TestEveryHTTPClientGoesThroughTheKit(t *testing.T) {
	t.Parallel()
	raw := regexp.MustCompile(`http\.Client\{|http\.DefaultClient|http\.(Get|Post|PostForm|Head)\(`)
	var sites, newClients, hooks, delivers int
	eachSourceLine(t, func(rel string, n int, code string) {
		if strings.Contains(code, "client.New(") {
			newClients++
		}
		if strings.Contains(code, "ApplyClientHooks(c)") {
			hooks++
		}
		if rel == "internal/cli/deliver.go" && strings.Contains(code, "Transport: credkit.Transport(nil)") {
			delivers++
		}
		if strings.Contains(code, "StreamingHTTPClient(nil") {
			t.Errorf("%s:%d: a streaming client with no base has no kit transport: %s", rel, n, code)
		}
		if !raw.MatchString(code) {
			return
		}
		sites++
		if !strings.Contains(code, "credkit.Transport(") && !allowed(rel, code, bareClients) {
			t.Errorf("%s:%d builds an HTTP client past the kit: %s", rel, n, code)
		}
	})
	if sites < 4 {
		t.Fatalf("found only %d HTTP clients; the scan is not reading the tree", sites)
	}
	if newClients == 0 || hooks < newClients {
		t.Errorf("%d client.New call(s), %d ApplyClientHooks call(s): a client can go unguarded", newClients, hooks)
	}
	if delivers != 1 {
		t.Errorf("deliver.go's webhook client has %d kit transport(s), want 1", delivers)
	}
}
