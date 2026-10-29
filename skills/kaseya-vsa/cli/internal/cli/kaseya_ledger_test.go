// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Review round 1 (Codex, 2026-10-28) [P1] class: auxiliary ledgers bypassed
// the output chokepoint. An unknown flag spelled as the credential reached
// the learn journal (argv_shape key, failed_flag), feedback text reached
// feedback.jsonl and the feedback endpoint. The class is every file the
// program writes, and every body it sends, outside the 0600 credential store
// and the pinned VSA API request: each passes cliutil.WithholdBytes,
// WithholdWriter or WithholdingFile, or is recorded below with the reason it
// cannot carry a credential. TestR1CEveryFileAndNetworkSinkIsRouted fails on
// a sink nobody has decided about.

import (
	"bytes"
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

	"kaseya-vsa-pp-cli/internal/cliutil"
	"kaseya-vsa-pp-cli/internal/cliutil/testenv"
	"kaseya-vsa-pp-cli/internal/learn"
)

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
	cliutil.FunnelHold(r1cSecret)
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
	if err := learn.AppendJournalEntry(learn.JournalEntry{Cmd: []string{"kaseya-vsa-cli"},
		ArgvShape: map[string]string{r1cSecret: "bool", "--" + r1cRot13(r1cSecret): "bool"}, ExitCode: 1,
		FailedFlag: "--" + r1cSecret}); err != nil {
		t.Fatalf("journal: %v", err)
	}
	if err := learn.StoreJournalOffset(learn.JournalOffset{Segment: r1cSecret}); err != nil {
		t.Fatalf("journal offset: %v", err)
	}
	// feedback.jsonl and the feedback endpoint.
	if err := appendFeedback(FeedbackEntry{Text: "note " + r1cSecret, CLI: "kaseya-vsa-cli"}); err != nil {
		t.Fatalf("feedback ledger: %v", err)
	}
	for _, text := range []string{"plain words survive", "echo " + r1cRot13(r1cSecret)} {
		if err := postFeedback(srv.URL, FeedbackEntry{Text: text, CLI: "kaseya-vsa-cli"}); err != nil {
			t.Fatalf("feedback send: %v", err)
		}
	}
	// The teach logs and the learnings audit.
	if err := learn.AppendTeachLogWarning("teach", "q", learn.Warning{Code: "no_entity_overlap", Detail: r1cSecret}); err != nil {
		t.Fatalf("teach log: %v", err)
	}
	writeTeachErrLog("teach: missing --query (args=[" + r1cSecret + "])")
	if err := appendLearningsAudit(map[string]any{"normalized": r1cSecret}); err != nil {
		t.Fatalf("learnings audit: %v", err)
	}
	// A log line written before this fix survives a scrub rewrite withheld.
	errLog, err := teachErrLogPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(errLog, []byte("old "+r1cSecret+"\n{\"query\":\"drop-me\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := scrubLogFile(errLog, "drop-me", ""); err != nil {
		t.Fatal(err)
	}
	// Saved flag profiles.
	if err := saveProfileStore(&profileStore{Profiles: map[string]Profile{"p": {Name: "p",
		Values: map[string]string{"token-secret": r1cSecret}}}}); err != nil {
		t.Fatalf("profiles: %v", err)
	}

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
	"internal/cli/feedback.go:appendFeedback":              {routed: "cliutil.WithholdWriter(f)"},
	"internal/cli/feedback.go:postFeedback":                {routed: "cliutil.WithholdBytes(body)"},
	"internal/cli/profile.go:saveProfileStore":             {routed: "cliutil.WithholdBytes(data)"},
	"internal/cli/teach.go:scrubLogFile":                   {routed: "cliutil.WithholdBytes([]byte(strings.Join(kept"},
	"internal/cli/teach.go:appendLearningsAudit":           {routed: "cliutil.WithholdWriter(f)"},
	"internal/cli/teach.go:writeTeachErrLog":               {routed: "cliutil.WithholdWriter(f)"},
	"internal/learn/journal.go:AppendJournalEntry":         {viaFile: "internal/learn/kaseya_journal_withhold.go", viaCode: "return cliutil.WithholdBytes(b), nil"},
	"internal/learn/journal.go:StoreJournalOffset":         {routed: "cliutil.WithholdBytes(data)"},
	"internal/learn/teach_log.go:appendTeachLogEntry":      {routed: "cliutil.WithholdBytes(line)"},
	"internal/platform/profile.go:writePrivateFile":        {routed: "withholdPrivateFile(data)"},
	"internal/cli/export.go:openExportOutput":              {viaFile: "internal/cli/export.go", viaCode: "cliutil.WithholdingFile(openExportOutput(outputFile))"},
	"internal/cli/deliver.go:writeDownloadUnder":           {viaFile: "internal/cli/deliver.go", viaCode: "body = cliutil.WithholdBytes(body)"},
	"internal/cli/deliver.go:deliverWebhook":               {viaFile: "internal/cli/deliver.go", viaCode: "body = cliutil.WithholdBytes(body)"},
	"internal/store/store.go:OpenWithContext":              {viaFile: "internal/store/store.go", viaCode: "sql.Open(withholdingSQLiteDriver, dsn)"},
	"internal/client/client.go:writeCacheWithHeaders":      {viaFile: "internal/cli/kaseya_guard.go", viaCode: "c.NoCache = true"},
	"internal/client/client.go:writePlatformCacheMetadata": {viaFile: "internal/cli/kaseya_guard.go", viaCode: "c.NoCache = true"},

	"internal/client/client.go:doInternal":                       {exempt: "the API request itself: the pinned VSA origin (api-origin-pinned) is the credential's owner"},
	"internal/cliutil/probe.go:ProbeReachable":                   {exempt: "a GET with no body to the configured base URL"},
	"internal/cliutil/credentials.go:SaveCredentials":            {exempt: "the 0600 credential store"},
	"internal/config/config.go:save":                             {exempt: "the config file, the credential store's TOML form (vsa_token_secret)"},
	"internal/config/config.go:restoreCredentialsFile":           {exempt: "restores the credential store's own earlier bytes"},
	"internal/config/config.go:scrubLegacyCredentials":           {exempt: "rewrites a legacy config with its credential fields cleared"},
	"internal/cliutil/paths.go:AtomicWritePrivateFile":           {exempt: "write helper: every caller is listed here on its own"},
	"internal/cli/helpers.go:claimLegacyUnscopedDB":              {exempt: "writes a scope hash only"},
	"internal/cli/store_adopt_linux.go:pinClosedStoreFile":       {exempt: "opens read-write to take a lease; writes nothing"},
	"internal/cliutil/filelock.go:openLockFile":                  {exempt: "a lock file holding one zero byte"},
	"internal/learn/journal.go:repairJournalSegmentTail":         {exempt: "writes one newline"},
	"internal/platform/profile.go:loadOrCreateFingerprintKey":    {exempt: "32 random bytes"},
	"internal/platform/ratelimit.go:acquireLedgerLock":           {exempt: "a lock holding a timestamp"},
	"internal/store/store.go:ensureSQLiteJournalPrivate":         {exempt: "creates an empty journal file"},
	"internal/store/store.go:openReadOnlySQLite":                 {exempt: "read-only open"},
	"internal/store/store.go:rejectNewerSchemaBeforeJournalMode": {exempt: "read-only probe"},
	"internal/store/store.go:ensureSQLiteDriverInitialized":      {exempt: "runs the DSN pragmas only; no statement carries a value"},
	"internal/platform/migration.go:AdoptVerifiedLegacyDatabase": {exempt: "read-only opens, then gate timestamps and the HMAC credential fingerprint on the copied store"},
	"internal/store/kaseya_withhold_driver.go:innerSQLite":       {exempt: "resolves the registered sqlite driver; opens no database"},
	"internal/cache/cache.go:Set":                                {exempt: "package internal/cache is linked into no binary (checked below)"},
}

// r1cSinkCalls are the calls that write a file, send a body or open a
// database. os.OpenFile counts when its flags can write.
var r1cSinkCalls = map[string]bool{
	"os.WriteFile": true, "os.Create": true, "os.CreateTemp": true, "os.OpenFile": true,
	"http.NewRequest": true, "http.NewRequestWithContext": true, "http.Post": true, "http.PostForm": true,
	"sql.Open": true, "cliutil.AtomicWritePrivateFile": true, "AtomicWritePrivateFile": true,
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
			t.Errorf("%s writes a file, sends a body or opens a database: route it through cliutil.WithholdBytes, WithholdWriter or WithholdingFile, or record in r1cSinks why it can never carry a credential", k)
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
	// The exemption for internal/cache holds only while no binary links it.
	for _, dir := range []string{"internal", "cmd"} {
		_ = filepath.WalkDir(filepath.Join(root, dir), func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.HasSuffix(p, ".go") && !strings.HasSuffix(p, "_test.go") {
				if b, rerr := os.ReadFile(p); rerr == nil && bytes.Contains(b, []byte(`"kaseya-vsa-`+"pp"+`-cli/internal/cache"`)) {
					t.Errorf("%s imports internal/cache: its Set writes response bodies, so route it", p)
				}
			}
			return nil
		})
	}
}

func r1cPrint(fset *token.FileSet, n ast.Node) string {
	var b strings.Builder
	_ = printer.Fprint(&b, fset, n)
	return b.String()
}

// TestClientMaskerFeedsTheChokepoint: the CLI binary links the client
// package, whose init registers its credential masker with the chokepoint
// (review r1 P1-B). Without it HeldEchoes falls back to the funnel alone.
func TestClientMaskerFeedsTheChokepoint(t *testing.T) {
	if cliutil.EchoDetectorsRegistered() < 1 {
		t.Fatal("no echo detector registered: the client masker is not in the output chokepoint")
	}
}
