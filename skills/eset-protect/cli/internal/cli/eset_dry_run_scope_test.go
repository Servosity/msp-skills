// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"eset-protect-pp-cli/internal/cliutil/testenv"
)

// TestDryRunSendsNothing pins what root --dry-run suppresses on press 4.34.2
// (handfixes.json: skill-doc-dry-run-scope-and-brand), so SKILL.md may say it
// sends nothing: import --dry-run neither mints a token nor posts a record,
// and a --deliver webhook only previews.
func TestDryRunSendsNothing(t *testing.T) {
	testenv.Isolate(t)
	respondClearHarness(t)
	var mints, calls atomic.Int32
	tok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mints.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"tok-1","refresh_token":"ref-1","token_type":"Bearer","expires_in":3600}`)
	}))
	t.Cleanup(tok.Close)
	esetTokenURLOverride = tok.URL
	t.Cleanup(func() { esetTokenURLOverride = "" })
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}))
	t.Cleanup(api.Close)
	t.Setenv("ESET_PROTECT_BASE_URL", api.URL)
	t.Setenv("ESET_PROTECT_REGION", "eu")
	t.Setenv("ESET_PROTECT_TOKEN", "")
	t.Setenv("ESET_PROTECT_USERNAME", "api@msp.example")
	t.Setenv("ESET_PROTECT_PASSWORD", "s3cret")

	input := filepath.Join(t.TempDir(), "rules.jsonl")
	if err := os.WriteFile(input, []byte(`{"rule":{"displayName":"x"}}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, stderr, err := captureExecuteAll(t, "import", "edr-rules", "--input", input, "--dry-run"); err != nil {
		t.Fatalf("import --dry-run: %v\n%s", err, stderr)
	}
	if _, stderr, err := captureExecuteAll(t, "devices", "list", "--dry-run", "--deliver", "webhook:"+api.URL+"/hook"); err != nil {
		t.Fatalf("--deliver webhook --dry-run: %v\n%s", err, stderr)
	}
	if m, c := mints.Load(), calls.Load(); m != 0 || c != 0 {
		t.Fatalf("--dry-run sent requests: %d token mints, %d API calls", m, c)
	}
}
