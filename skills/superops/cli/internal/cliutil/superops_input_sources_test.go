// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): credkit-wiring source chokepoint.

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

// inputReaders is every function in this module that reads stdin, decodes a
// TOML file or reads a local file. "held" functions give what they read to
// the holder where they read it, on every return, refused or not; every other
// entry says why what it reads is no new credential source. A reader "held by
// every caller" returns the bytes it read, and every function that calls it
// must call the holder instead. A file is held as it is read whoever wrote it:
// an earlier build wrote entries the holder never saw.
var inputReaders = map[string]string{
	"cmd/superops-mcp/superops_url_redact_mcp.go:serveHeldStdio":         "MCP requests: credkit.MCPIn reads them, and contract section 1 names only declared MCP arguments",
	"internal/cli/feedback.go:newFeedbackCmd":                            "free text: contract section 1 names no source here (ruling 19): a note",
	"internal/cli/feedback.go:newFeedbackListCmd":                        "free text: contract section 1 names no source here (ruling 19): the notes ledger",
	"internal/cli/jobs.go:readJobRows":                                   "free text: contract section 1 names no source here (ruling 19): the jobs ledger",
	"internal/cli/learnings_candidates.go:confirmingSessionRecallFamily": "free text: contract section 1 names no source here (ruling 19): the learn journal",
	"internal/learn/derive.go:journalHasUnreadTail":                      "free text: contract section 1 names no source here (ruling 19): the learn journal",
	"internal/learn/derive.go:deriveFlagCorrectionsLocked":               "free text: contract section 1 names no source here (ruling 19): the learn journal",
	"internal/learn/synthesize.go:SynthesizePlaybookCandidate":           "free text: contract section 1 names no source here (ruling 19): the learn journal",
	"internal/learn/teach_log.go:ReadTeachLogWarnings":                   "free text: contract section 1 names no source here (ruling 19): the teach error log",
	"internal/credwire/declarations.go:holdProfileStores":                "held",
	"internal/cli/profile.go:loadProfileStore":                           "the run profile store: credwire.holdProfileStores gives it to credkit.HoldBody at startup, and Values keep only savable flags",
	"internal/cli/raw.go:resolveDocArg":                                  "a GraphQL document, or --vars, which newRawOpCmd gives to credkit.HoldBody",
	"internal/cli/teach_playbook.go:resolvePlaybookInputs":               "free text: contract section 1 names no source here (ruling 19): a playbook and its notes",
	"internal/cliutil/credentials.go:credentialsBytesHaveValues":         "held",
	"internal/cliutil/credentials.go:loadCredentialsWithStatus":          "held",
	"internal/config/config.go:baseURLFromConfigBytes":                   "held",
	"internal/config/config.go:parseConfigData":                          "held",
	"internal/config/config.go:Config.scrubLegacyCredentials":            "held",

	"internal/cli/auth.go:newAuthSetTokenCmd":                             "passes to readSecretFromStdin, which holds the token",
	"internal/config/config.go:readConfigFile":                            "passes to parseConfigData",
	"internal/config/config.go:Config.readExplicitConfig":                 "passes to acceptOpenedConfig, which parses with parseConfigData",
	"internal/config/config.go:Config.readDefaultConfig":                  "passes to acceptOpenedConfig, which parses with parseConfigData",
	"internal/config/config.go:Config.trustedGlobalCredentialOrigin":      "passes to baseURLFromConfigBytes",
	"internal/platform/profile.go:LoadProfile":                            "a client profile: credential references and public identity only; a URL in it is free text: contract section 1 names no source here (ruling 19)",
	"internal/config/config.go:openDefaultConfig":                         reader,
	"internal/cliutil/creds_perms_unix.go:ReadOpenedConfig":               reader,
	"internal/cliutil/creds_perms_windows.go:ReadOpenedConfig":            reader,
	"internal/cliutil/creds_perms_unix.go:readExplicitCredentialBytes":    reader,
	"internal/cliutil/creds_perms_windows.go:readExplicitCredentialBytes": reader,
	"internal/cliutil/creds_perms_unix.go:readGlobalCredentialBytes":      reader,
	"internal/cliutil/creds_perms_windows.go:readCredentialInRoot":        reader,
	"internal/cliutil/creds_perms_windows.go:readGlobalCredentialBytes":   reader,
	"internal/cliutil/filelock_unix.go:ReadConfigTrust":                   reader,
	"internal/cliutil/filelock_windows.go:ReadConfigTrust":                reader,
	"internal/cliutil/paths.go:ReadFileWithLegacyFallback":                reader,
	"internal/learn/playbooks.go:ParsePlaybookFile":                       reader,
	"internal/learn/journal.go:ReadJournalFrom":                           reader,
	"internal/learn/derive.go:sessionRecallFamiliesBeforeBatch":           reader,

	"internal/cliutil/filelock_unix.go:TrustConfigAllowsSettings":    "keeps the trust verdict; discards the bytes",
	"internal/cliutil/filelock_windows.go:TrustConfigAllowsSettings": "keeps the trust verdict; discards the bytes",
	"internal/config/config.go:snapshotCredentialsFile":              "operator: keeps the credentials file's bytes to restore them byte for byte; never decoded or shown",
	"internal/cliutil/darwin_acl.go:openDarwinFollowACL":             "opens a file to read its ACL, not its content",
	"internal/cliutil/darwin_acl_darwin.go:openDarwinNoFollow":       "opens a file to read its ACL, not its content",
	"internal/cliutil/filelock.go:openLockFile":                      "a lock file",
	"internal/client/client.go:loadCACertPool":                       "public PEM certificates parsed into a pool, never shown",
	"internal/cache/cache.go:Store.Get":                              "vendor data: a cached API response, no operator input",
	"internal/client/client.go:Client.readCacheWithHeaders":          "vendor data: a cached API response, no operator input",
	"internal/client/client.go:Client.platformCacheMetadataMatches":  "returns only whether the cache metadata matches the session",
	"internal/cli/teach.go:scrubLogFile":                             "rewrites the teach error log without its held lines; never shown",
	"internal/learn/journal.go:repairJournalSegmentTail":             "writes one newline to the journal's tail",
	"internal/learn/journal.go:LoadJournalOffset":                    "a segment name and a byte offset",
	"internal/learn/derive.go:fileHasByteFrom":                       "scans the journal for one byte; returns no text",
	"internal/platform/receipt.go:ReceiptWriter.appendAuditIndex":    "appends to the audit index; never shown",
	"internal/platform/ratelimit.go:EndpointLimiter.Wait":            "the rate-limit ledger: timestamps",
	"internal/config/superops_credential_holder.go:SelectedBaseURL":  "reads base_url and region, no credential, of a file Declarations.Files lists",
	"internal/platform/profile.go:readFingerprintKey":                "32 random bytes this program generated",
	"internal/platform/profile.go:SelectProfile":                     "reads default_client_profile, a validated profile name",
	"internal/platform/profile.go:SetDefaultProfile":                 "writes the printing-press config back to itself through the holder (writePrivateFile); never shown",
	"internal/cli/helpers.go:claimLegacyUnscopedDB":                  "a store owner marker: a tenant scope hash",
	"internal/cli/store_adopt_darwin.go:claimClosedStore":            "opens a store file for a lease; reads no text",
	"internal/cli/store_adopt_linux.go:pinClosedStoreFile":           "opens a store file for a lease; reads no text",
	"internal/cli/store_adopt_linux.go:procSameUID":                  "/proc/<pid>/status, for the owner uid",
}

// reader marks a function that returns what it read: every function that
// calls it reads that input too, so the walk classifies each caller.
const reader = "returns what it read; every caller is classified here"

// inputsTheWalkCannotSee read an input through no call the walk detects (the
// process's own arguments and environment, an MCP tool call's arguments, a
// reader handed to them); each must call the holder.
var inputsTheWalkCannotSee = []string{
	"internal/cli/helpers.go:readSecretFromStdin",
	"internal/cliutil/superops_credential_holder.go:holdProcessCredentials",
}

var holdCall = regexp.MustCompile(`\b[Hh]old\w*\(`)

// deferredHold is a hold in a deferred func: it reads the named results, which
// an error return ("return "", err") zeroes before the defer runs.
var deferredHold = regexp.MustCompile(`defer func\(\)\s*\{[^}]*\b[Hh]old\w*\(`)

type walkedFunc struct {
	key, body string
	fn        *ast.FuncDecl
	src       []byte
	fset      *token.FileSet
}

// Every function that reads an input is classified: a new one (a reprint, a
// new command) fails here until it holds what it read or says why that is no
// credential source. A stale entry fails too, so the list is current.
func TestEveryInputReaderHoldsWhatItReadsOrIsNamed(t *testing.T) {
	root := filepath.Join("..", "..")
	fset := token.NewFileSet()
	var funcs []walkedFunc
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() && d.Name() == "credkit" {
			return filepath.SkipDir // vendored: it reads what it declares and holds it itself
		}
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
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil {
				body := string(src[fset.Position(fn.Pos()).Offset:fset.Position(fn.End()).Offset])
				funcs = append(funcs, walkedFunc{filepath.ToSlash(rel) + ":" + receiverPrefix(fn) + fn.Name.Name, body, fn, src, fset})
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	readers := map[string]bool{} // names of functions that return what they read
	for key, kind := range inputReaders {
		if kind == reader {
			readers[key[strings.LastIndex(key, ":")+1:]] = true
		}
	}
	seen := map[string]bool{}
	bodies := map[string]string{}
	for _, w := range funcs {
		bodies[w.key] = w.body
		if !readsInput(w.fset, w.src, w.fn, readers) {
			continue
		}
		seen[w.key] = true
		kind, ok := inputReaders[w.key]
		switch {
		case !ok:
			t.Errorf("%s reads an input and is not classified: give what it reads to the holder (cliutil.HoldValues, credkit.HoldBody) and list it as held, or say why it is no credential source", w.key)
		case kind == "held" && !holdCall.MatchString(w.body):
			t.Errorf("%s is classified held but never calls the holder", w.key)
		case kind == "held" && deferredHold.MatchString(w.body):
			t.Errorf("%s holds in a defer, which an error return can empty: hold where it reads", w.key)
		case strings.HasPrefix(kind, "passes to "):
			callee := strings.TrimRight(strings.Fields(kind)[2], ",")
			if !strings.Contains(w.body, callee+"(") {
				t.Errorf("%s is classified %q but never calls %s", w.key, kind, callee)
			}
		}
	}
	for key := range inputReaders {
		if !seen[key] {
			t.Errorf("%s is classified but no longer reads an input; remove it", key)
		}
	}
	for _, key := range inputsTheWalkCannotSee {
		body, ok := bodies[key]
		switch {
		case !ok:
			t.Errorf("%s no longer exists; name the function that now reads that input", key)
		case !holdCall.MatchString(body):
			t.Errorf("%s reads an input and never calls the holder", key)
		}
	}
}

// receiverPrefix names a method's receiver type ("Config."), so two methods
// of one name in a file keep their own entries.
func receiverPrefix(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}
	t := fn.Recv.List[0].Type
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	if id, ok := t.(*ast.Ident); ok {
		return id.Name + "."
	}
	return ""
}

// readsInput reports whether fn reads stdin, decodes TOML, opens a file for
// reading, or calls a function that returns what it read.
func readsInput(fset *token.FileSet, src []byte, fn *ast.FuncDecl, readers map[string]bool) bool {
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if found {
			return false
		}
		switch x := n.(type) {
		case *ast.CallExpr:
			if sel, ok := x.Fun.(*ast.SelectorExpr); ok && len(x.Args) > 1 {
				if id, ok := sel.X.(*ast.Ident); ok && id.Name == "os" && sel.Sel.Name == "OpenFile" {
					flags := string(src[fset.Position(x.Args[1].Pos()).Offset:fset.Position(x.Args[1].End()).Offset])
					found = strings.Contains(flags, "O_RDONLY") || strings.Contains(flags, "O_RDWR")
				}
			}
		case *ast.SelectorExpr:
			pkg := ""
			if id, ok := x.X.(*ast.Ident); ok {
				pkg = id.Name
			}
			switch {
			case pkg == "os" && (x.Sel.Name == "Stdin" || x.Sel.Name == "ReadFile" || x.Sel.Name == "Open" || x.Sel.Name == "OpenRoot"):
				found = true
			case pkg == "toml" && (x.Sel.Name == "Unmarshal" || x.Sel.Name == "NewDecoder" || x.Sel.Name == "Decode"):
				found = true
			case x.Sel.Name == "InOrStdin" || readers[x.Sel.Name]:
				found = true
			}
		case *ast.Ident:
			found = readers[x.Name] && x.Name != fn.Name.Name
		}
		return !found
	})
	return found
}
