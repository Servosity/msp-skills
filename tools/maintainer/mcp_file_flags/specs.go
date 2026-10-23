// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package main

import (
	"go/types"
	"strings"
)

// The analysis treats every function outside the connector module as opaque:
// it has no body, so what it does with its arguments comes from the rules in
// this file. Three kinds of rule exist.
//
//   - A SINK names the parameters of a function that turn a string into a
//     filesystem access (open, read, write, stat, mkdir, walk, exec). A tainted
//     value reaching one of those parameters is a finding.
//   - A SANITIZER returns a value the caller cannot steer into a path (a hash,
//     a parsed number, an HTTP response chosen by the vendor's server).
//   - Everything else PROPAGATES: its results carry the taint of all of its
//     arguments, so a path built by filepath.Join or strings.TrimPrefix stays
//     tainted. Propagating by default is what keeps an unlisted function from
//     hiding a flow; it can only add findings, never remove one.

// stdSinks lists, per standard-library function or method, the zero-based
// SIGNATURE parameter indexes (the receiver is not counted) that name a
// filesystem location. Keys are "pkgpath.Func" or "pkgpath.Type.Method".
var stdSinks = map[string][]int{
	// os
	"os.Open": {0}, "os.OpenFile": {0}, "os.Create": {0}, "os.ReadFile": {0},
	"os.WriteFile": {0}, "os.Stat": {0}, "os.Lstat": {0}, "os.ReadDir": {0},
	"os.Mkdir": {0}, "os.MkdirAll": {0}, "os.MkdirTemp": {0}, "os.CreateTemp": {0},
	"os.Remove": {0}, "os.RemoveAll": {0}, "os.Rename": {0, 1}, "os.Link": {0, 1},
	"os.Symlink": {0, 1}, "os.Readlink": {0}, "os.Chmod": {0}, "os.Chown": {0},
	"os.Lchown": {0}, "os.Chtimes": {0}, "os.Chdir": {0}, "os.Truncate": {0},
	"os.DirFS": {0}, "os.CopyFS": {0}, "os.OpenRoot": {0}, "os.OpenInRoot": {0, 1},
	"os.StartProcess": {0},
	// io/ioutil
	"io/ioutil.ReadFile": {0}, "io/ioutil.WriteFile": {0}, "io/ioutil.ReadDir": {0},
	"io/ioutil.TempDir": {0}, "io/ioutil.TempFile": {0},
	// path/filepath: the functions that touch the disk (Join, Clean, Base, Abs
	// and friends only compute strings and propagate).
	"path/filepath.Walk": {0}, "path/filepath.WalkDir": {0}, "path/filepath.Glob": {0},
	"path/filepath.EvalSymlinks": {0},
	// os/exec: choosing the program to run is choosing a file to execute.
	"os/exec.Command": {0}, "os/exec.CommandContext": {1}, "os/exec.LookPath": {0},
	// database/sql: the DSN of a file-backed driver (sqlite) is a path.
	"database/sql.Open": {1},
	// templates and certificates read named files.
	"html/template.ParseFiles": {0}, "html/template.ParseGlob": {0},
	"html/template.Template.ParseFiles": {0}, "html/template.Template.ParseGlob": {0},
	"text/template.ParseFiles": {0}, "text/template.ParseGlob": {0},
	"text/template.Template.ParseFiles": {0}, "text/template.Template.ParseGlob": {0},
	"crypto/tls.LoadX509KeyPair": {0, 1},
	"archive/zip.OpenReader":     {0},
	"debug/elf.Open":             {0}, "debug/macho.Open": {0}, "debug/pe.Open": {0},
	"plugin.Open":        {0},
	"net/http.ServeFile": {2},
	// io/fs: a name inside a file system. Only a host file system counts; an
	// embed.FS argument is excluded at the call site (see fsNameSinks).
	"io/fs.ReadFile": {1}, "io/fs.ReadDir": {1}, "io/fs.Stat": {1}, "io/fs.Glob": {1},
	"io/fs.WalkDir": {1}, "io/fs.Sub": {1}, "io/fs.Lstat": {1}, "io/fs.ReadLink": {1},
	"html/template.ParseFS": {1}, "text/template.ParseFS": {1},
	"net/http.ServeFileFS": {3},
}

// fsNameSinks are the stdSinks whose first argument is the file system the
// name is resolved in; the call is a sink only when that file system is not
// an embed.FS compiled into the binary.
var fsNameSinks = map[string]bool{
	"io/fs.ReadFile": true, "io/fs.ReadDir": true, "io/fs.Stat": true, "io/fs.Glob": true,
	"io/fs.WalkDir": true, "io/fs.Sub": true, "io/fs.Lstat": true, "io/fs.ReadLink": true,
	"html/template.ParseFS": true, "text/template.ParseFS": true, "net/http.ServeFileFS": true,
}

// syscallPathParams is the parameter-name rule for the thin system-call
// wrappers (syscall, x/sys/unix, x/sys/windows), whose surface is too large to
// list by hand. Every string parameter with one of these names is a path.
var syscallPathParams = map[string]bool{
	"path": true, "pathname": true, "oldpath": true, "newpath": true, "argv0": true,
	"linkpath": true, "target": true, "source": true, "from": true, "to": true,
	"dir": true, "file": true, "filename": true, "name": true, "oldname": true, "newname": true,
}

var syscallPackages = map[string]bool{
	"syscall": true, "golang.org/x/sys/unix": true, "golang.org/x/sys/windows": true,
}

// syscallNotFS are wrapper functions whose "name" parameter is not a path.
var syscallNotFS = []string{"Sysctl", "Getenv", "Setenv", "Unsetenv", "Getpagesize", "Uname"}

// thirdPartyPathParams is the rule for every module outside the standard
// library that the connector depends on (TOML and YAML loaders, SDK config
// loaders, completion-file writers): a string, []string or ...string
// parameter with one of these names is treated as a path. A false hit here is
// a finding a maintainer reads; a missed one would be a hidden file read.
var thirdPartyPathParams = map[string]bool{
	"path": true, "paths": true, "pathname": true, "file": true, "files": true,
	"filename": true, "filenames": true, "filepath": true, "fpath": true, "dir": true,
	"dirname": true, "directory": true, "dirpath": true, "root": true, "rootdir": true,
	"dsn": true, "datasourcename": true, "configfile": true, "cfgfile": true,
	"outfile": true, "infile": true, "outpath": true, "inpath": true,
}

// thirdPartyNotSinks are dependency packages handled by dedicated rules (the
// flag and MCP sources) or known to touch no files through named parameters.
var thirdPartyNotSinks = []string{
	"github.com/spf13/pflag",
	"github.com/mark3labs/mcp-go",
}

// sanitizers return values a caller of the CLI cannot turn into a path of
// their choosing: digests, parsed numbers and times, and HTTP responses (the
// vendor's server, not the MCP caller, chooses their content). Keys are
// "pkgpath.Func", "pkgpath.Type.Method", or "pkgpath." for a whole package.
var sanitizers = map[string]bool{
	"crypto/sha256.": true, "crypto/sha1.": true, "crypto/sha512.": true, "crypto/md5.": true,
	"crypto/hmac.": true, "hash/fnv.": true, "hash/crc32.": true, "hash/crc64.": true,
	"hash/adler32.": true, "hash/maphash.": true,
	"encoding/hex.EncodeToString": true, "encoding/hex.Encode": true,
	"strconv.Atoi": true, "strconv.ParseInt": true, "strconv.ParseUint": true,
	"strconv.ParseFloat": true, "strconv.ParseBool": true,
	"time.Parse": true, "time.ParseInLocation": true, "time.ParseDuration": true,
	"net/http.Client.Do": true, "net/http.Client.Get": true, "net/http.Client.Post": true,
	"net/http.Client.PostForm": true, "net/http.Client.Head": true,
	"net/http.Get": true, "net/http.Post": true, "net/http.PostForm": true, "net/http.Head": true,
	"net/http.Transport.RoundTrip": true,
}

// receiverAccumulators are external types whose methods store their
// arguments in the receiver (a builder, a buffer, a header map), so a value
// written into one can be read back out. For every other external method the
// receiver is not updated: tainting a shared writer such as os.Stdout would
// spread one flag's taint to every value later printed.
var receiverAccumulators = map[string]bool{
	"strings.Builder": true, "bytes.Buffer": true, "bufio.Writer": true,
	"net/url.Values": true, "net/url.URL": true, "net/http.Header": true,
	"net/http.Request": true, "strings.Replacer": true, "encoding/csv.Writer": true,
	"text/template.Template": true, "html/template.Template": true,
}

func funcKey(fn *types.Func) (pkg, key string) {
	if fn == nil || fn.Pkg() == nil {
		return "", ""
	}
	pkg = fn.Pkg().Path()
	sig, _ := fn.Type().(*types.Signature)
	if sig != nil && sig.Recv() != nil {
		return pkg, pkg + "." + typeName(sig.Recv().Type()) + "." + fn.Name()
	}
	return pkg, pkg + "." + fn.Name()
}

// typeName is the bare name of a (possibly pointer, possibly generic) named
// type, or "" for an unnamed one.
func typeName(t types.Type) string {
	if p, ok := types.Unalias(t).(*types.Pointer); ok {
		t = p.Elem()
	}
	if n, ok := types.Unalias(t).(*types.Named); ok {
		return n.Obj().Name()
	}
	return ""
}

func qualifiedTypeName(t types.Type) string {
	if p, ok := types.Unalias(t).(*types.Pointer); ok {
		t = p.Elem()
	}
	if n, ok := types.Unalias(t).(*types.Named); ok && n.Obj().Pkg() != nil {
		return n.Obj().Pkg().Path() + "." + n.Obj().Name()
	}
	return ""
}

func isSanitizer(fn *types.Func) bool {
	pkg, key := funcKey(fn)
	return sanitizers[key] || sanitizers[pkg+"."]
}

func isStdPackage(path string) bool {
	first := path
	if i := strings.Index(path, "/"); i >= 0 {
		first = path[:i]
	}
	return !strings.Contains(first, ".")
}

// sinkParams returns the signature parameter indexes of fn that name a
// filesystem location, or nil.
func sinkParams(fn *types.Func) []int {
	pkg, key := funcKey(fn)
	if key == "" {
		return nil
	}
	if idx, ok := stdSinks[key]; ok {
		return idx
	}
	if pkg == "os" {
		// Every string parameter of an os.Root method names a location; the
		// engine decides per call whether the root confines it (see
		// isRootMethod).
		if isRootMethod(fn) {
			return stringParams(fn, nil)
		}
		return nil
	}
	if syscallPackages[pkg] {
		for _, n := range syscallNotFS {
			if strings.HasPrefix(fn.Name(), n) {
				return nil
			}
		}
		return stringParams(fn, syscallPathParams)
	}
	if isStdPackage(pkg) {
		return nil
	}
	for _, p := range thirdPartyNotSinks {
		if pkg == p || strings.HasPrefix(pkg, p+"/") {
			return nil
		}
	}
	return stringParams(fn, thirdPartyPathParams)
}

// isRootMethod reports whether fn is a method of os.Root (Open, Create,
// OpenFile, ReadFile, WriteFile, Stat, Mkdir, Remove, Rename, OpenRoot, ...).
// An os.Root resolves every name relative to the directory it was opened at
// and refuses one that leaves it: an absolute path, a ".." hop, or a symlink
// whose target is outside. So such a name is confined only as far as the root
// itself is: the engine (confinedSink) treats it as a component inside the
// root when no caller-chosen value reached the root's own path (os.OpenRoot's
// argument, itself a sink), and as a path otherwise. os.OpenInRoot is a plain
// function and keeps its stdSinks rule.
func isRootMethod(fn *types.Func) bool {
	pkg, key := funcKey(fn)
	sig, _ := fn.Type().(*types.Signature)
	return pkg == "os" && sig != nil && sig.Recv() != nil && strings.HasPrefix(key, "os.Root.")
}

// rootUnconfinedParams are os.Root method parameters the root does not
// resolve: Symlink stores its target (oldname) verbatim, and whoever later
// opens the link outside the root follows it there. They stay plain sinks.
var rootUnconfinedParams = map[string][]int{
	"os.Root.Symlink": {0},
}

func rootUnconfinedParam(key string, i int) bool {
	for _, j := range rootUnconfinedParams[key] {
		if j == i {
			return true
		}
	}
	return false
}

// stringParams returns the indexes of fn's string-like parameters, all of
// them when names is nil, or only those whose lower-cased name is in names.
func stringParams(fn *types.Func, names map[string]bool) []int {
	sig, ok := fn.Type().(*types.Signature)
	if !ok {
		return nil
	}
	var out []int
	for i := 0; i < sig.Params().Len(); i++ {
		p := sig.Params().At(i)
		if !isStringLike(p.Type()) {
			continue
		}
		if names != nil && !names[strings.ToLower(p.Name())] {
			continue
		}
		out = append(out, i)
	}
	return out
}

func isStringLike(t types.Type) bool {
	switch u := t.Underlying().(type) {
	case *types.Basic:
		return u.Info()&types.IsString != 0
	case *types.Slice:
		if b, ok := u.Elem().Underlying().(*types.Basic); ok {
			return b.Info()&types.IsString != 0
		}
	}
	return false
}
