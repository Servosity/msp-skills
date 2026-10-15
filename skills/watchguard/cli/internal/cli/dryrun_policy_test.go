// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func dryRunEnv(t *testing.T) (*requestRecorder, string) {
	t.Helper()
	isolateAuthEnv(t)
	rec := &requestRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(rec.handler))
	t.Cleanup(srv.Close)
	home := t.TempDir()
	t.Setenv("WATCHGUARD_HOME", home)
	t.Setenv("WATCHGUARD_BASE_URL", srv.URL)
	t.Setenv("WATCHGUARD_TOKEN_URL", srv.URL+"/oauth/token")
	t.Setenv("WATCHGUARD_API_KEY", "KEY")
	t.Setenv("WATCHGUARD_ACCOUNT_ID", "ACC-1")
	t.Setenv("WATCHGUARD_FEEDBACK_ENDPOINT", srv.URL+"/feedback")
	return rec, home
}

// hand-fix (handfixes.json dry-run-side-effect-free): the honoring commands
// print a dry-run envelope and change nothing; the refusing ones stop with a
// usage error (exit 2) before doing anything.
func TestDryRunHonoredAndRefusedCommands(t *testing.T) {
	rec, home := dryRunEnv(t)
	for _, args := range [][]string{
		{"auth", "login", "--client-id", "RW", "--client-secret", "rwsec"},
		{"auth", "logout"},
		{"auth", "set-token"},
		{"feedback", "probe", "--send"},
		{"teach", "firmware lag"},
		{"firebox-devices", "changes"},
	} {
		out, err := runAuthCmd(t, "pasted-token\n", append(args, "--dry-run", "--json")...)
		if err != nil || !strings.Contains(out, `"dry_run":true`) {
			t.Errorf("%v --dry-run: err=%v out=%s", args, err, out)
		}
	}
	for _, args := range [][]string{
		{"jobs", "prune"},
		{"profile", "save", "p1", "--max-age", "5m"},
		{"profile", "delete", "p1", "--yes"},
		{"export", "activations", "--output", filepath.Join(home, "out.jsonl")},
		{"firebox-aliases", "list", "--deliver", "webhook:" + os.Getenv("WATCHGUARD_BASE_URL") + "/hook"},
	} {
		_, err := runAuthCmd(t, "", append(args, "--dry-run", "--json")...)
		if code := ExitCode(err); code != 2 {
			t.Errorf("%v --dry-run: exit %d (err=%v), want a usage refusal (2)", args, code, err)
		}
	}
	if n := rec.count(); n != 0 {
		t.Fatalf("dry runs sent %d requests: %v", n, rec.since(0))
	}
	if _, err := os.Stat(filepath.Join(home, "out.jsonl")); err == nil {
		t.Fatal("export --output --dry-run wrote a file")
	}
	for _, f := range []string{"data/credentials.toml", "data/feedback.jsonl", "config/config.toml"} {
		if _, err := os.Stat(filepath.Join(home, f)); err == nil {
			t.Errorf("a dry run created %s", f)
		}
	}
}

// hand-fix (handfixes.json dry-run-never-mints): with a cached token bound to
// another Access ID, a GET under --dry-run still previews (exit 0) without
// asking the token endpoint for a replacement.
func TestDryRunGETWithTokenBoundElsewherePreviews(t *testing.T) {
	rec, _ := dryRunEnv(t)
	if out, err := runAuthCmd(t, "", "auth", "login", "--client-id", "RW", "--client-secret", "rwsec"); err != nil {
		t.Fatalf("auth login: %v\n%s", err, out)
	}
	t.Setenv("WATCHGUARD_CLIENT_ID", "RO")
	t.Setenv("WATCHGUARD_CLIENT_SECRET", "rosec")
	n := rec.count()
	out, err := runAuthCmd(t, "", "accounts", "get", "--dry-run", "--json")
	if err != nil || !strings.Contains(out, "dry") {
		t.Fatalf("accounts get --dry-run with a token bound elsewhere: err=%v out=%s", err, out)
	}
	if reqs := rec.since(n); len(reqs) > 0 {
		t.Fatalf("dry run sent %v", reqs)
	}
}
