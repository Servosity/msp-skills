// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"threecx-pp-cli/internal/cliutil/testenv"
	"threecx-pp-cli/internal/fleet"
)

// Class dry-run-file-mutation (Codex r2 input): `export users --dry-run
// --output <existing file>` must leave the destination untouched; a preview
// opens no local sink.
func TestExportDryRunLeavesOutputFileUntouched(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("--dry-run sent %s %s", r.Method, r.URL.Path)
	}))
	t.Cleanup(srv.Close)
	testenv.Isolate(t)
	t.Setenv("THREECX_BASE_URL", srv.URL+"/xapi/v1")
	t.Setenv("THREECX_CLIENT_ID", "900")
	t.Setenv("THREECX_CLIENT_SECRET", "dryRunSinkSecret-0a1b2c3d")

	dst := filepath.Join(t.TempDir(), "users.jsonl")
	const keep = "{\"Id\":1,\"FirstName\":\"kept\"}\n"
	if err := os.WriteFile(dst, []byte(keep), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _ = runCLI(t, "export", "users", "--dry-run", "--output", dst)
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("destination gone after --dry-run: %v", err)
	}
	if string(got) != keep {
		t.Fatalf("--dry-run changed the destination: %q", got)
	}

	// A destination that did not exist is not created either.
	fresh := filepath.Join(t.TempDir(), "sub", "new.jsonl")
	_, _ = runCLI(t, "export", "users", "--dry-run", "--output", fresh)
	if _, err := os.Stat(fresh); !os.IsNotExist(err) {
		t.Fatalf("--dry-run created %s (stat err %v)", fresh, err)
	}
}

// Class misleading-pin-wording (post-review P2): a refused URL with no host
// (THREECX_TOKEN_URL=mock-token-for-testing) is reported as having no host,
// never as "a non-ASCII host".
func TestDisplayOriginNamesAMissingHost(t *testing.T) {
	for raw, want := range map[string]string{
		"mock-token-for-testing":    "no host",
		"/connect/token":            "no host",
		"https://pbx.example.com/x": "https://pbx.example.com:443",
		"https://bücher.example/":   "a non-ASCII host",
	} {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("parse %q: %v", raw, err)
		}
		if got := displayOrigin(u); got != want {
			t.Errorf("displayOrigin(%q) = %q, want %q", raw, got, want)
		}
	}
}

// applyPBXSelection reads THREECX_PBX as a literal (the env-schema gate cannot
// follow a constant in another package); it must stay the runner's variable.
func TestPBXSelectionReadsTheRunnersVariable(t *testing.T) {
	if fleet.SelectEnv != "THREECX_PBX" {
		t.Fatalf("fleet.SelectEnv is %q; update the literal in applyPBXSelection", fleet.SelectEnv)
	}
}
