// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/sophos-central/handfixes.json
// under the "runtime-routing-not-persisted" entry.

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

// TestTenantSelectionNotPersisted: a --tenant run that mints a token must not
// pin X-Tenant-ID (or the env base/token URLs) into config.toml for later runs.
func TestTenantSelectionNotPersisted(t *testing.T) {
	var lastTenant string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			_, _ = io.WriteString(w, `{"access_token":"tok","expires_in":3600}`)
			return
		}
		lastTenant = r.Header.Get("X-Tenant-ID")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"items":[],"pages":{}}`)
	}))
	defer srv.Close()
	home := t.TempDir()
	t.Setenv("SOPHOS_CENTRAL_HOME", home)
	t.Setenv("SOPHOS_CENTRAL_BASE_URL", srv.URL)
	t.Setenv("SOPHOS_CENTRAL_TOKEN_URL", srv.URL+"/token")
	t.Setenv("SOPHOS_CENTRAL_CLIENT_ID", "client-a")
	t.Setenv("SOPHOS_CENTRAL_CLIENT_SECRET", "secret-a")
	t.Setenv("SOPHOS_CENTRAL_TENANT_ID", "")
	run := func(args ...string) {
		t.Helper()
		cmd := RootCmd()
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		cmd.SetArgs(append(args, "--no-cache", "--json"))
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	run("endpoints", "list", "--tenant", "tenant-one")
	if lastTenant != "tenant-one" {
		t.Fatalf("--tenant run sent X-Tenant-ID %q", lastTenant)
	}
	b, _ := os.ReadFile(filepath.Join(home, "config", "config.toml"))
	if s := string(b); strings.Contains(s, "tenant-one") || strings.Contains(s, srv.URL) {
		t.Fatalf("config.toml kept runtime routing: %s", s)
	}
	run("endpoints", "list")
	if lastTenant != "" {
		t.Fatalf("a later run without --tenant still sent X-Tenant-ID %q", lastTenant)
	}
}
