// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"pax8-pp-cli/internal/cli"
)

// TestAuthLoginDryRunSendsNothing: `auth login --dry-run` minted through its
// own path (mintClientCredentialsToken), ignoring the root --dry-run flag, so a
// preview sent the client secret to the token URL and saved the token. It must
// send nothing and write nothing; the live control proves the same command
// does reach the token URL. Hand-owned (handfixes.json: dry-run-never-mints).
func TestAuthLoginDryRunSendsNothing(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"stub-access-token","token_type":"Bearer","expires_in":3600}`)
	}))
	defer srv.Close()
	home := t.TempDir()
	for _, k := range []string{"HOME", "USERPROFILE", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "PAX8_HOME"} {
		t.Setenv(k, home)
	}
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	t.Setenv("PAX8_NO_CONFIG_WRITE", "")
	t.Setenv("PAX8_CONFIG", filepath.Join(home, "config.toml"))
	t.Setenv("PAX8_TOKEN_URL", srv.URL+"/token")
	t.Setenv("PAX8_CLIENT_ID", "stub-client-id")
	t.Setenv("PAX8_CLIENT_SECRET", "stub-client-secret")

	var stderr bytes.Buffer
	cmd := cli.RootCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"auth", "login", "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("auth login --dry-run: %v", err)
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("auth login --dry-run sent %d request(s)", n)
	}
	if !strings.Contains(stderr.String(), "dry run - no request sent, nothing saved") {
		t.Fatalf("auth login --dry-run did not say it sent nothing: %q", stderr.String())
	}
	_ = filepath.WalkDir(home, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if data, readErr := os.ReadFile(path); readErr == nil && (bytes.Contains(data, []byte("stub-client-secret")) || bytes.Contains(data, []byte("stub-access-token"))) {
			t.Errorf("auth login --dry-run wrote credential material to %s", path)
		}
		return nil
	})

	live := cli.RootCmd()
	live.SetOut(io.Discard)
	live.SetErr(io.Discard)
	live.SetArgs([]string{"auth", "login"})
	_ = live.Execute()
	if hits.Load() == 0 {
		t.Fatal("control: auth login without --dry-run sent nothing, so the dry-run check proved nothing")
	}
}
