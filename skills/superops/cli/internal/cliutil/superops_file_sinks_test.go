// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): credkit-wiring file chokepoint.

package cliutil

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// fileWriters is every function in this module that creates or writes a
// local file. "held" functions must call the holder before writing; every
// other entry says why it writes no program text, or why the file is the
// operator's own credential store. A writer "held by every caller" keeps its
// press-template body unchanged (so its annotated lines keep their fleet
// precedent), and every function that calls it must call the holder instead.
var fileWriters = map[string]string{
	"internal/cache/cache.go:Set":                             "held",
	"internal/cli/deliver.go:writeDownloadUnder":              "held",
	"internal/cli/feedback.go:appendFeedback":                 "held",
	"internal/cli/jobs.go:RecordJob":                          "held",
	"internal/cli/jobs.go:newJobsPruneCmd":                    "held",
	"internal/cli/profile.go:saveProfileStore":                "held",
	"internal/cli/teach.go:writeTeachErrLog":                  "held",
	"internal/cli/teach.go:appendLearningsAudit":              "held",
	"internal/cli/teach.go:scrubLogFile":                      "held",
	"internal/client/client.go:writeCacheWithHeaders":         "held",
	"internal/client/client.go:writePlatformCacheMetadata":    "held",
	"internal/learn/journal.go:AppendJournalEntry":            "held",
	"internal/learn/teach_log.go:appendTeachLogEntry":         "held",
	"internal/platform/profile.go:writePrivateFile":           "held",
	"internal/cli/helpers.go:publishStoreFile":                "links a store file written through the kit's driver (credsql)",
	"internal/cli/helpers.go:claimLegacyUnscopedDB":           "held",
	"internal/cli/store_adopt_linux.go:pinClosedStoreFile":    "opens a store file for a lease; writes nothing",
	"internal/cliutil/filelock.go:openLockFile":               "constant: one NUL byte, so a Windows byte-range lock has a byte to lock",
	"internal/cliutil/superops_credential_holder.go:HeldText": "the check itself: writes only to the null device",
	"internal/cliutil/paths.go:AtomicWritePrivateFile":        "operator: the helper the credentials file and config writers below use",
	"internal/cliutil/superops_held_write.go:HeldAtomicWrite": "held",
	"internal/learn/journal.go:repairJournalSegmentTail":      "constant: one newline",
	"internal/platform/profile.go:loadOrCreateFingerprintKey": "held",
	"internal/platform/ratelimit.go:acquireLedgerLock":        "held",
	"internal/store/store.go:ensureSQLiteJournalPrivate":      "an empty file",
	"internal/cliutil/credentials.go:SaveCredentials":         "operator: the credentials file auth set-token saves",
	"internal/config/config.go:save":                          "operator: the config file the operator saves",
	"internal/config/config.go:scrubLegacyCredentials":        "operator: rewrites the operator's own config without its legacy credentials",
	"internal/config/config.go:restoreCredentialsFile":        "operator: restores the operator's own file byte for byte",
}

// writesBytes is a write of content. Only the kit's sinks and a declared
// credential source (an "operator:" entry) may do one, or a write of one fixed
// byte (a "constant:" entry): a name, a hash or a timestamp written unchecked
// can still carry a held 8-character piece.
var writesBytes = regexp.MustCompile(`\.(Write|WriteString|WriteAt|ReadFrom)\(|\bfmt\.Fprint|\bio\.(Copy|WriteString)\(|\bos\.WriteFile\(`)

var heldCall = regexp.MustCompile(`\b(credkit\.(WriteFile|AppendFile|OpenFile|Create|NewWriter)|HeldText)\(`)

// Every call that creates or writes a local file is classified: a new one (a
// reprint, a new command) fails here until it passes the holder or says why
// it writes no program text. A stale entry fails too, so the list is current.
func TestEveryFileWriterPassesTheHolderOrIsNamed(t *testing.T) {
	root := filepath.Join("..", "..")
	fset := token.NewFileSet()
	seen := map[string]bool{}
	callers := map[string][]string{} // writer name -> "file:func" of each caller that never calls the holder
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return err
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		f, err := parser.ParseFile(fset, p, src, 0)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			key := filepath.ToSlash(rel) + ":" + fn.Name.Name
			body := string(src[fset.Position(fn.Pos()).Offset:fset.Position(fn.End()).Offset])
			if !heldCall.MatchString(body) {
				for _, name := range calledNames(fn) {
					callers[name] = append(callers[name], key)
				}
			}
			if !writesFile(fset, src, fn) {
				continue
			}
			seen[key] = true
			kind, ok := fileWriters[key]
			switch {
			case !ok:
				t.Errorf("%s writes a local file and is not classified: pass the kit (credkit.WriteFile, AppendFile, OpenFile) and list it as held, or say why it writes no program text", key)
			case kind == "held" && !heldCall.MatchString(body):
				t.Errorf("%s is classified held but never calls the holder", key)
			case kind != "held" && kind != "held by every caller" && !strings.HasPrefix(kind, "operator:") && !strings.HasPrefix(kind, "constant:") && writesBytes.MatchString(body):
				t.Errorf("%s is named as writing nothing but writes bytes: a metadata file can carry a held piece too, so pass the kit", key)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for key, kind := range fileWriters {
		if !seen[key] {
			t.Errorf("%s is classified but no longer writes a file; remove it", key)
		}
		if kind == "held by every caller" {
			name := key[strings.LastIndex(key, ":")+1:]
			for _, c := range callers[name] {
				t.Errorf("%s calls %s, which is held by its callers, without calling the holder", c, key)
			}
		}
	}
}

func calledNames(fn *ast.FuncDecl) []string {
	var out []string
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			switch fun := call.Fun.(type) {
			case *ast.Ident:
				out = append(out, fun.Name)
			case *ast.SelectorExpr:
				out = append(out, fun.Sel.Name)
			}
		}
		return true
	})
	return out
}

func writesFile(fset *token.FileSet, src []byte, fn *ast.FuncDecl) bool {
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || found {
			return !found
		}
		var pkg, name string
		switch fun := call.Fun.(type) {
		case *ast.SelectorExpr:
			if x, ok := fun.X.(*ast.Ident); ok {
				pkg = x.Name
			}
			name = fun.Sel.Name
		case *ast.Ident:
			name = fun.Name
		}
		switch {
		case pkg == "os" && (name == "WriteFile" || name == "Create" || name == "CreateTemp" || name == "Symlink" || name == "Link"):
			found = true
		case pkg == "os" && name == "OpenFile" && len(call.Args) > 1:
			flags := string(src[fset.Position(call.Args[1].Pos()).Offset:fset.Position(call.Args[1].End()).Offset])
			found = strings.Contains(flags, "O_WRONLY") || strings.Contains(flags, "O_RDWR") || strings.Contains(flags, "O_APPEND") || strings.Contains(flags, "O_CREATE") || strings.Contains(flags, "O_TRUNC")
		case pkg == "credkit" && (name == "WriteFile" || name == "AppendFile" || name == "OpenFile" || name == "Create"):
			found = true
		case name == "AtomicWritePrivateFile" && (pkg == "" || pkg == "cliutil"):
			found = true
		}
		return !found
	})
	return found
}
