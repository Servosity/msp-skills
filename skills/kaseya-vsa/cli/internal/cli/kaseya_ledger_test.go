// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Review round 1 (Codex, 2026-10-28), one P1 class: auxiliary ledgers bypassed
// the output layer. An unknown flag spelled as the credential reached the
// learn journal (argv_shape key, failed_flag), feedback text reached
// feedback.jsonl and the feedback endpoint. The class is every file the
// program writes, and every body it sends, outside the 0600 credential store:
// each goes through the credential kit, or is recorded below with the reason
// it cannot carry a credential. TestR1CEveryFileAndNetworkSinkIsRouted fails on
// a sink nobody has decided about.

import (
	"bytes"
	"errors"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"kaseya-vsa-pp-cli/internal/client"
	"kaseya-vsa-pp-cli/internal/cliutil"
	"kaseya-vsa-pp-cli/internal/cliutil/testenv"
	"kaseya-vsa-pp-cli/internal/credkit"
	"kaseya-vsa-pp-cli/internal/learn"
)

// No in-process test runs credkit.Start, so the outbound guard would refuse
// every credentialed request and then fail closed for the whole test process.
// The built binaries' tests (internal/credwire, internal/kaseyablackbox) prove it.
func init() {
	client.OutboundGuard = func(rt http.RoundTripper) http.RoundTripper { return rt }
}

const r1cSecret = "Secret" + "Alpha901" // split: a synthetic fixture, not a credential for secret scanners

func r1cRot13(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return 'a' + (r-'a'+13)%26
		case r >= 'A' && r <= 'Z':
			return 'A' + (r-'A'+13)%26
		}
		return r
	}, s)
}

func r1cAssertNone(t *testing.T, where string, data []byte) {
	t.Helper()
	low := bytes.ToLower(data)
	for _, f := range []string{r1cSecret, r1cRot13(r1cSecret)} {
		if bytes.Contains(low, []byte(strings.ToLower(f))) {
			t.Errorf("%s holds the credential as %q", where, f)
		}
	}
}

func TestR1CLedgersAndSendsWithholdHeldValues(t *testing.T) {
	home := testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	credkit.Hold("config:vsa_token_secret", r1cSecret)
	var mu sync.Mutex
	var sent []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		sent = append(sent, b...)
		mu.Unlock()
	}))
	defer srv.Close()

	// The learn journal: an unknown flag spelled as the credential.
	withheldOK := func(what string, err error) {
		t.Helper()
		if err != nil && !errors.Is(err, credkit.ErrWithheld) {
			t.Fatalf("%s: %v", what, err)
		}
	}
	// One ordinary write per ledger, so each scanned file exists.
	withheldOK("journal", learn.AppendJournalEntry(learn.JournalEntry{Cmd: []string{"kaseya-vsa-cli"}, ExitCode: 1}))
	withheldOK("journal", learn.AppendJournalEntry(learn.JournalEntry{Cmd: []string{"kaseya-vsa-cli"},
		ArgvShape: map[string]string{r1cSecret: "bool", "--" + r1cRot13(r1cSecret): "bool"}, ExitCode: 1,
		FailedFlag: "--" + r1cSecret}))
	withheldOK("journal offset", learn.StoreJournalOffset(learn.JournalOffset{Segment: r1cSecret}))
	// feedback.jsonl and the feedback endpoint.
	withheldOK("feedback ledger", appendFeedback(FeedbackEntry{Text: "plain note", CLI: "kaseya-vsa-cli"}))
	withheldOK("feedback ledger", appendFeedback(FeedbackEntry{Text: "note " + r1cSecret, CLI: "kaseya-vsa-cli"}))
	if err := postFeedback(srv.URL, FeedbackEntry{Text: "plain words survive", CLI: "kaseya-vsa-cli"}); err != nil {
		t.Fatalf("feedback send: %v", err)
	}
	if err := postFeedback(srv.URL, FeedbackEntry{Text: "echo " + r1cRot13(r1cSecret), CLI: "kaseya-vsa-cli"}); err == nil {
		t.Error("the feedback send carrying a held credential was not refused")
	}
	// The teach logs and the learnings audit.
	withheldOK("teach log", learn.AppendTeachLogWarning("teach", "q", learn.Warning{Code: "no_entity_overlap", Detail: "plain"}))
	withheldOK("teach log", learn.AppendTeachLogWarning("teach", "q", learn.Warning{Code: "no_entity_overlap", Detail: r1cSecret}))
	writeTeachErrLog("teach: missing --query (args=[" + r1cSecret + "])")
	withheldOK("learnings audit", appendLearningsAudit(map[string]any{"normalized": "plain"}))
	withheldOK("learnings audit", appendLearningsAudit(map[string]any{"normalized": r1cSecret}))
	// A log line written before the kit does not survive a scrub rewrite.
	errLog, err := teachErrLogPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(errLog, []byte("old "+r1cSecret+"\n{\"query\":\"drop-me\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	withheldOK("scrub", scrubLogFile(errLog, "drop-me", ""))
	if err := os.WriteFile(errLog, []byte("plain\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Saved flag profiles.
	withheldOK("profiles", saveProfileStore(&profileStore{Profiles: map[string]Profile{"p": {Name: "p",
		Values: map[string]string{"token-secret": "plain"}}}}))
	withheldOK("profiles", saveProfileStore(&profileStore{Profiles: map[string]Profile{"p": {Name: "p",
		Values: map[string]string{"token-secret": r1cSecret}}}}))

	var files []string
	_ = filepath.WalkDir(home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		rel, _ := filepath.Rel(home, path)
		files = append(files, rel)
		r1cAssertNone(t, "file "+rel, data)
		return nil
	})
	t.Logf("scanned %v", files)
	if len(files) < 6 {
		t.Fatalf("only %d files written (%v); the scan proves nothing", len(files), files)
	}
	mu.Lock()
	defer mu.Unlock()
	if !bytes.Contains(sent, []byte("plain words")) {
		t.Fatalf("the feedback endpoint received %q; the check proves nothing", sent)
	}
	r1cAssertNone(t, "feedback endpoint body", sent)
}

// r1cSink is the decision for one function that writes a file, sends a body
// or opens a database for writing.
type r1cSink struct {
	// routed: this code appears in the function itself.
	routed string
	// viaFile/viaCode: the function's only caller routes it (or the store
	// driver does); viaCode must appear in viaFile.
	viaFile, viaCode string
	// exempt: why the sink can never carry a credential.
	exempt string
}

var r1cSinks = map[string]r1cSink{
	"internal/learn/journal.go:repairJournalSegmentTail":         {routed: "credkit.NewWriter(f, credkit.SinkFile)"},
	"internal/credsql/credsql.go:applyLengthLimit":               {exempt: "lends a connection for one sqlite.Limit call; no statement runs through it"},
	"internal/cli/feedback.go:postFeedback":                      {viaFile: "internal/cli/feedback.go", viaCode: "Transport: credkit.Transport(nil)"},
	"internal/cli/deliver.go:deliverWebhook":                     {viaFile: "internal/cli/deliver.go", viaCode: "Transport: credkit.Transport(nil)"},
	"internal/cli/deliver.go:writeDownloadUnder":                 {routed: "credkit.WriteFile(tmp, body, 0o600)"},
	"internal/cliutil/paths.go:atomicWritePrivate":               {exempt: "creates the empty temp file; AtomicWritePrivateFile writes through credkit.WriteFile, AtomicWriteCredentialFile is the credential store's own writer"},
	"internal/platform/profile.go:writePrivateFile":              {routed: "credkit.WriteFile(tmpName, data, 0o600)"},
	"internal/store/store.go:OpenWithContext":                    {routed: "sql.Open(credsql.Driver, dsn)"},
	"internal/store/store.go:openReadOnlySQLite":                 {routed: "sql.Open(credsql.Driver, dsn)"},
	"internal/store/store.go:ensureSQLiteDriverInitialized":      {routed: "sql.Open(credsql.Driver, dsn)"},
	"internal/store/store.go:rejectNewerSchemaBeforeJournalMode": {routed: "sql.Open(credsql.Driver,"},
	"internal/platform/migration.go:AdoptVerifiedLegacyDatabase": {routed: "sql.Open(credsql.Driver,"},

	"internal/client/client.go:doInternal":                    {routed: "sendClient.Transport = OutboundGuard(httpClient.Transport)"},
	"internal/cliutil/probe.go:ProbeReachable":                {routed: "Transport: credkit.Transport(nil)"},
	"internal/cliutil/credentials.go:SaveCredentials":         {exempt: "the 0600 credential store, a credential source named in Declarations.Files"},
	"internal/config/config.go:save":                          {exempt: "the config file, a credential source named in Declarations.Files"},
	"internal/config/config.go:restoreCredentialsFile":        {exempt: "restores the credential store's own earlier bytes"},
	"internal/config/config.go:scrubLegacyCredentials":        {exempt: "rewrites a legacy config with its credential fields cleared"},
	"internal/cli/helpers.go:claimLegacyUnscopedDB":           {routed: "credkit.WriteFile(tempName, []byte(scopeHash+\"\\n\"), 0o600)"},
	"internal/cli/store_adopt_linux.go:pinClosedStoreFile":    {exempt: "opens read-write to take a lease; writes nothing"},
	"internal/cliutil/filelock.go:openLockFile":               {routed: "credkit.NewWriter(f, credkit.SinkFile)"},
	"internal/platform/profile.go:loadOrCreateFingerprintKey": {routed: "credkit.NewWriter(file, credkit.SinkFile)"},
	"internal/platform/ratelimit.go:acquireLedgerLock":        {routed: "credkit.WriteFile(path, []byte(strconv.FormatInt(time.Now().UnixNano(), 10)), 0o600)"},
	"internal/store/store.go:ensureSQLiteJournalPrivate":      {exempt: "creates an empty journal file"},
}

// r1cSinkCalls are the calls that write a file, send a body or open a
// database. os.OpenFile counts when its flags can write.
var r1cSinkCalls = map[string]bool{
	"os.WriteFile": true, "os.Create": true, "os.CreateTemp": true, "os.OpenFile": true,
	"http.NewRequest": true, "http.NewRequestWithContext": true, "http.Post": true, "http.PostForm": true,
	"sql.Open": true, "sql.OpenDB": true, "cliutil.AtomicWriteCredentialFile": true, "AtomicWriteCredentialFile": true,
}

func TestR1CEveryFileAndNetworkSinkIsRouted(t *testing.T) {
	root := filepath.Join("..", "..")
	found := map[string]string{} // sink key -> printed function body
	var walkErr error
	for _, dir := range []string{"internal", "cmd"} {
		walkErr = filepath.WalkDir(filepath.Join(root, dir), func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return err
			}
			fset := token.NewFileSet()
			f, perr := parser.ParseFile(fset, p, nil, 0)
			if perr != nil {
				return perr
			}
			rel, _ := filepath.Rel(root, p)
			rel = filepath.ToSlash(rel)
			for _, decl := range f.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				ast.Inspect(fn.Body, func(n ast.Node) bool {
					call, ok := n.(*ast.CallExpr)
					if !ok {
						return true
					}
					name := r1cPrint(fset, call.Fun)
					if !r1cSinkCalls[name] {
						return true
					}
					if name == "os.OpenFile" && len(call.Args) > 1 {
						flags := r1cPrint(fset, call.Args[1])
						if !strings.Contains(flags, "WRONLY") && !strings.Contains(flags, "RDWR") &&
							!strings.Contains(flags, "APPEND") && !strings.Contains(flags, "CREATE") {
							return true
						}
					}
					found[rel+":"+fn.Name.Name] = r1cPrint(fset, fn.Body)
					return true
				})
			}
			return nil
		})
		if walkErr != nil {
			t.Fatal(walkErr)
		}
	}
	keys := make([]string, 0, len(found))
	for k := range found {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		rule, ok := r1cSinks[k]
		switch {
		case !ok:
			t.Errorf("%s writes a file, sends a body or opens a database: route it through the credential kit, or record in r1cSinks why it can never carry a credential", k)
		case rule.routed != "":
			if !strings.Contains(found[k], rule.routed) {
				t.Errorf("%s: want %s before the write", k, rule.routed)
			}
		case rule.viaFile != "":
			b, err := os.ReadFile(filepath.Join(root, rule.viaFile))
			if err != nil || !strings.Contains(string(b), rule.viaCode) {
				t.Errorf("%s: want %s in %s", k, rule.viaCode, rule.viaFile)
			}
		case rule.exempt == "":
			t.Errorf("%s: an exemption needs its reason", k)
		}
	}
	for k := range r1cSinks {
		if _, ok := found[k]; !ok {
			t.Errorf("r1cSinks lists %s, which no longer writes: drop the entry", k)
		}
	}
}

func r1cPrint(fset *token.FileSet, n ast.Node) string {
	var b strings.Builder
	_ = printer.Fprint(&b, fset, n)
	return b.String()
}
