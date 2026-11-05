// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/synology/handfixes.json
// under the "dry-run-guards-local-mutations" entry.

package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"synology-pp-cli/internal/cliutil"
)

// TestDryRunChangesNothingLocally: export -o truncated the named file,
// feedback appended to feedback.jsonl (and POSTed with --send), and profile
// delete deleted, all under --dry-run.
func TestDryRunChangesNothingLocally(t *testing.T) {
	sweepIsolate(t)
	var posted atomic.Int32
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { posted.Add(1) }))
	defer sink.Close()
	t.Setenv("SYNOLOGY_FEEDBACK_ENDPOINT", sink.URL)

	keep := filepath.Join(t.TempDir(), "keep.json")
	if err := os.WriteFile(keep, []byte("PRECIOUS"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, code := sweepExecute(t, "export", "group", "-o", keep, "--dry-run"); code != 0 {
		t.Fatalf("export --dry-run exited %d", code)
	}
	if b, _ := os.ReadFile(keep); string(b) != "PRECIOUS" {
		t.Fatalf("export -o --dry-run rewrote the file: %q", b)
	}

	out, _, code := sweepExecute(t, "feedback", "--dry-run", "--send", "--json", "hello there")
	if code != 0 || !strings.Contains(out, `"dry_run":true`) {
		t.Fatalf("feedback --dry-run exited %d: %q", code, out)
	}
	dataDir, err := cliutil.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "feedback.jsonl")); err == nil {
		t.Fatal("feedback --dry-run wrote feedback.jsonl")
	}
	if posted.Load() != 0 {
		t.Fatal("feedback --dry-run --send POSTed")
	}

	if _, _, code := sweepExecute(t, "profile", "save", "p1", "--json"); code != 0 {
		t.Fatalf("profile save exited %d", code)
	}
	if _, _, code := sweepExecute(t, "profile", "delete", "p1", "--yes", "--dry-run"); code != 0 {
		t.Fatalf("profile delete --dry-run exited %d", code)
	}
	if p, err := GetProfile("p1"); err != nil || p == nil {
		t.Fatalf("profile delete --dry-run deleted the profile (err=%v)", err)
	}
}
