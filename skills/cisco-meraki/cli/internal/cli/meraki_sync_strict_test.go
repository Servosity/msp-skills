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
// next page unread fails --strict, while a plain sync still exits 0 with the
// rows it stored and counts the resource as synced.
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
	if err := run(true); err == nil || !strings.Contains(err.Error(), "stopped before their last page") {
		t.Fatalf("sync --strict truncated by --max-pages: err=%v, want the --max-pages cap failure", err)
	}
}

// TestSyncStrictFailsOnStuckPagination: an API that repeats the same next
// cursor makes sync stop early with a warning; --strict must not exit 0.
func TestSyncStrictFailsOnStuckPagination(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/v1/organizations" {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		// Every page points at the same next cursor with fresh items.
		w.Header().Set("Link", "<http://"+r.Host+"/api/v1/organizations?startingAfter=2>; rel=next")
		after := r.URL.Query().Get("startingAfter")
		_, _ = w.Write([]byte(`[{"id":"1` + after + `","name":"Acme` + after + `","api":{"enabled":true}},{"id":"2` + after + `","name":"Bayside` + after + `","api":{"enabled":true}}]`))
	}))
	defer srv.Close()
	testenv.Isolate(t)
	t.Setenv("CISCO_MERAKI_BASE_URL", srv.URL+"/api/v1")
	t.Setenv("MERAKI_DASHBOARD_API_KEY", "test-key")
	cmd := RootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"sync", "--resources", "organizations", "--rate-limit", "0", "--strict", "--json"})
	err := cmd.Execute()
	if !strings.Contains(out.String(), "stuck_pagination") {
		t.Fatalf("stub did not trigger stuck pagination: %s", out.String())
	}
	if err == nil || !strings.Contains(err.Error(), "stopped before their last page") {
		t.Fatalf("sync --strict after stuck pagination: err=%v, want the truncation failure", err)
	}
}
