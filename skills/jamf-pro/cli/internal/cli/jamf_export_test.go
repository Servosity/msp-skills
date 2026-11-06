// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"jamf-pro-pp-cli/internal/cliutil/testenv"
	"jamf-pro-pp-cli/internal/jamfauth"
)

// TestExportReadsEveryZeroBasedPage: Jamf Pro API pages are 0-based, so an
// export of 150 buildings (pages 0 and 1 at the default size) writes all
// 150, the first page included.
func TestExportReadsEveryZeroBasedPage(t *testing.T) {
	home := testenv.Isolate(t)
	t.Setenv("JAMF_PRO_HOME", home)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/oauth/token" {
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":1200}`))
			return
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		var items []string
		for i := page*100 + 1; i <= (page+1)*100 && i <= 150; i++ {
			items = append(items, fmt.Sprintf(`{"id":"%d","name":"b%d"}`, i, i))
		}
		fmt.Fprintf(w, `{"totalCount":150,"results":[%s]}`, strings.Join(items, ","))
	}))
	defer srv.Close()
	pointAt(t, srv, "cid", "csecret")
	out := filepath.Join(home, "buildings.jsonl")
	if _, err := runCLI(t, "export", "buildings", "-o", out); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(b), "\n"); n != 150 || !strings.Contains(string(b), `"id":"1"`) {
		t.Fatalf("exported %d records, want 150 starting at id 1:\n%.300s", n, b)
	}
}

// TestDryRunSendsNothing (jamf-pro handfix dry-run-sends-nothing): under
// --dry-run no request reaches the server, whatever the command does with
// it: export -o and auth login do not honour the flag themselves.
func TestDryRunSendsNothing(t *testing.T) {
	home := testenv.Isolate(t)
	t.Setenv("JAMF_PRO_HOME", home)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/oauth/token" {
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":1200}`))
			return
		}
		_, _ = w.Write([]byte(`{"totalCount":1,"results":[{"id":"1","name":"b1"}]}`))
	}))
	defer srv.Close()
	pointAt(t, srv, "cid", "csecret")
	_, _ = runCLI(t, "export", "buildings", "-o", filepath.Join(home, "b.jsonl"), "--dry-run")
	_, _ = runCLI(t, "auth", "login", "--client-id", "cid", "--client-secret", "csecret", "--dry-run")
	_, _ = runCLI(t, "buildings", "list", "--data-source", "live", "--dry-run")
	if n := hits.Load(); n != 0 {
		t.Fatalf("--dry-run sent %d request(s)", n)
	}
}

// TestMCPStartsUnconfiguredButBuildsNoClient: with no server configured
// anywhere the MCP server starts (tools/list works) with no identity, and
// every client it would build is refused, so nothing can be sent.
func TestMCPStartsUnconfiguredButBuildsNoClient(t *testing.T) {
	freshProcess(t)
	home := testenv.Isolate(t)
	t.Setenv("JAMF_PRO_HOME", home)
	for _, k := range []string{"JAMF_PRO_SERVER", "JAMF_PRO_BASE_URL", "JAMF_PRO_CLIENT_ID", "JAMF_PRO_CLIENT_SECRET", "JAMF_PRO_CONFIG"} {
		t.Setenv(k, "")
	}
	if err := EstablishMCPIdentity(); err != nil {
		t.Fatalf("an unconfigured MCP server must start: %v", err)
	}
	if _, ok := jamfauth.Established(); ok {
		t.Fatal("no identity may be established without a server")
	}
	if _, err := (&rootFlags{}).newClient(); err == nil {
		t.Fatal("a client built with no server and no identity must be refused")
	}
}
