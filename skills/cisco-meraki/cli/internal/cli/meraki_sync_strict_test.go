// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/cisco-meraki/handfixes.json
// under "meraki-sync-partial-exit".

package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cisco-meraki-pp-cli/internal/cliutil/testenv"
)

// TestSyncStrictFailsOnWarnedResource: an access-denied (403) resource is a
// warning (here every organizations dependent), so a plain sync exits 0
// while --strict must exit non-zero.
func TestSyncStrictFailsOnWarnedResource(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/organizations" {
			_, _ = w.Write([]byte(`[{"id":"1","name":"Acme","api":{"enabled":true}}]`))
			return
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errors":["You do not have permission to access this resource"]}`))
	}))
	defer srv.Close()
	run := func(strict bool) error {
		testenv.Isolate(t)
		t.Setenv("CISCO_MERAKI_BASE_URL", srv.URL+"/api/v1")
		t.Setenv("MERAKI_DASHBOARD_API_KEY", "test-key")
		args := []string{"sync", "--resources", "organizations", "--rate-limit", "0", "--json"}
		if strict {
			args = append(args, "--strict")
		}
		cmd := RootCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		err := cmd.Execute()
		if strict && err != nil && !strings.Contains(err.Error(), "completed with warnings") {
			t.Fatalf("--strict failed for the wrong reason: %v", err)
		}
		return err
	}
	if err := run(false); err != nil {
		t.Fatalf("plain sync with a 403 dependent should exit 0: %v", err)
	}
	if err := run(true); err == nil {
		t.Fatal("sync --strict with a 403 (warned) resource exited 0")
	}
}

// TestSyncStrictFailsOnMaxPagesTruncation: a --max-pages cap that leaves a
// next page unread is a warning, so --strict exits non-zero while a plain
// sync still exits 0 with the rows it stored.
func TestSyncStrictFailsOnMaxPagesTruncation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/v1/organizations" {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		if r.URL.Query().Get("startingAfter") == "" {
			w.Header().Set("Link", "<http://"+r.Host+"/api/v1/organizations?startingAfter=2>; rel=next")
			_, _ = w.Write([]byte(`[{"id":"1","name":"Acme","api":{"enabled":true}},{"id":"2","name":"Bayside","api":{"enabled":true}}]`))
			return
		}
		_, _ = w.Write([]byte(`[{"id":"3","name":"Cedar","api":{"enabled":true}}]`))
	}))
	defer srv.Close()
	run := func(strict bool) error {
		testenv.Isolate(t)
		t.Setenv("CISCO_MERAKI_BASE_URL", srv.URL+"/api/v1")
		t.Setenv("MERAKI_DASHBOARD_API_KEY", "test-key")
		args := []string{"sync", "--resources", "organizations", "--max-pages", "1", "--rate-limit", "0", "--json"}
		if strict {
			args = append(args, "--strict")
		}
		cmd := RootCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		return cmd.Execute()
	}
	if err := run(false); err != nil {
		t.Fatalf("plain sync truncated by --max-pages should exit 0: %v", err)
	}
	if err := run(true); err == nil || !strings.Contains(err.Error(), "completed with warnings") {
		t.Fatalf("sync --strict truncated by --max-pages: err=%v, want the warnings failure", err)
	}
}
