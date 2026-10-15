// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/sophos-central/handfixes.json
// under the "dry-run-never-mints" entry.

package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAuthLoginDryRunSendsNothing: auth login mints through its own path,
// so the client's dry-run guard does not cover it.
func TestAuthLoginDryRunSendsNothing(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = io.WriteString(w, `{"access_token":"stub","expires_in":3600}`)
	}))
	defer srv.Close()
	home := t.TempDir()
	t.Setenv("SOPHOS_CENTRAL_HOME", home)
	t.Setenv("SOPHOS_CENTRAL_TOKEN_URL", srv.URL+"/token")
	t.Setenv("SOPHOS_CENTRAL_CLIENT_ID", "stub-id-1234")
	t.Setenv("SOPHOS_CENTRAL_CLIENT_SECRET", "stub-secret-1234")

	cmd := RootCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"auth", "login", "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("auth login --dry-run: %v", err)
	}
	if hits != 0 {
		t.Fatalf("auth login --dry-run sent %d request(s)", hits)
	}
	_ = filepath.WalkDir(home, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.Contains(d.Name(), "credentials") {
			t.Fatalf("auth login --dry-run wrote %s", path)
		}
		return nil
	})
}
