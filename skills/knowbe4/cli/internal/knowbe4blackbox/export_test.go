// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package knowbe4blackbox

import (
	"context"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Codex r1 P2 export-hangs-after-api-error: export --output writes through the
// withholding file sink (a filter pipe). An API error after the file is open
// must still close it, or the process waits for that pipe forever on exit.
func TestExportOutputReturnsAfterAnAPIError(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// The fake answers the policies list 401.
	sv.mu.Lock()
	sv.denied["/policies"] = true
	sv.mu.Unlock()
	cmd := exec.CommandContext(ctx, bbCLI, "export", "policies", "--output", filepath.Join(e.home, "policies.jsonl"))
	cmd.Env = e.env
	cmd.Dir = e.home
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("export --output did not return after KnowBe4 answered 401:\n%s", out)
	}
	if err == nil {
		t.Fatalf("export --output succeeded on a 401:\n%s", out)
	}
	assertNoEcho(t, "export after a 401", string(out))
	// The export reached the API (an argument error would fail fast too).
	sv.mu.Lock()
	defer sv.mu.Unlock()
	var asked bool
	for _, m := range sv.methods {
		asked = asked || m == "GET /v1/policies"
	}
	if !asked {
		t.Fatalf("export never asked the API for the policies; the test did not run:\n%s", out)
	}
}

// pagerServer is a KnowBe4 fake whose user list (GET /users) holds total
// users, paged as documented (spec.yaml users.list: "page: 1-based page
// number", "per_page: Items per page (default 100, max 500)"): a bare JSON
// array of per_page records (default 100), 1-based page (default 1), full
// pages until the last. Every other read is an empty list. It counts the
// requests it answered and the page numbers it served.
func pagerServer(t *testing.T, total int) (*server, *int) {
	t.Helper()
	var mu sync.Mutex
	asked := new(int)
	sv := newServer()
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		*asked++
		mu.Unlock()
		p := strings.TrimPrefix(r.URL.Path, "/v1")
		w.Header().Set("Content-Type", "application/json")
		if p != "/users" {
			fmt.Fprint(w, `[]`)
			return
		}
		q := r.URL.Query()
		sv.mu.Lock()
		sv.pages["/users?page="+q.Get("page")+"&per_page="+q.Get("per_page")]++
		sv.mu.Unlock()
		var items []string
		for i := 1; i <= total; i++ {
			items = append(items, fmt.Sprintf(`{"id":%d,"first_name":"Paged","last_name":"user %d","email":"paged%d@hq-record.example.com","status":"active"}`, 1000+i, i, i))
		}
		fmt.Fprint(w, pagedArray(q, 100, items))
	}))
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	sv.url, sv.caFile = srv.URL+"/v1", ca
	return sv, asked
}

// A wrong result on a documented response shape (not a credential class):
// export walks every page of the user list, from the vendor's first page
// (page 1), and stops after the last; an account with more users than one
// page (KnowBe4's default per_page is 100) must export every one, to a file
// documented as a backup or migration. The MCP export tool and the live
// list's --all walk the same pages; sync walks the same list (its own pager)
// and is the control.
func TestExportWalksEveryPage(t *testing.T) {
	t.Parallel()
	const total = 250
	sv, asked := pagerServer(t, total)
	e := newEnv(t, sv, credEnv)
	file := filepath.Join(e.home, "users.jsonl")
	out := e.run(t, "export", "users", "--format", "jsonl", "--output", file)
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("export wrote no file: %v\n%s", err, out)
	}
	// countUsers counts the distinct users s names (raw JSON or a JSON
	// string inside an MCP result).
	countUsers := func(s string) int {
		seen := map[string]bool{}
		for _, m := range regexp.MustCompile(`user ([0-9]+)\\?"`).FindAllStringSubmatch(s, -1) {
			if n, err := strconv.Atoi(m[1]); err == nil && n >= 1 && n <= total {
				seen[m[1]] = true
			}
		}
		return len(seen)
	}
	records := func(s string) int { return strings.Count(s, "@hq-record.example.com") }
	if n, dup := countUsers(string(b)), records(string(b)); n != total || dup != total {
		sv.mu.Lock()
		t.Errorf("export --output wrote %d distinct of %d users (%d records, %d requests, pages %v):\n%.600s", n, total, dup, *asked, sv.pages, out)
		sv.mu.Unlock()
	}
	if got := e.run(t, "export", "users", "--format", "json"); countUsers(got) != total || records(got) != total {
		t.Errorf("export --format json printed %d distinct of %d users (%d records)", countUsers(got), total, records(got))
	}
	if got := e.mcpCalls(t, []map[string]any{tool("export", map[string]any{"resource": "users", "format": "json"})}); countUsers(got) != total {
		t.Errorf("the MCP export tool returned %d distinct of %d users:\n%.1500s", countUsers(got), total, got)
	}
	// The live list's --all walks the same pages (its own client-side pager).
	if got := e.run(t, "users", "list", "--all", "--json"); countUsers(got) != total || records(got) != total {
		sv.mu.Lock()
		t.Errorf("users list --all printed %d distinct of %d users (%d records; pages %v)", countUsers(got), total, records(got), sv.pages)
		sv.mu.Unlock()
	}
	// Control: sync walks every page of the same fake.
	syncOut := e.run(t, "sync", "--resources", "users", "--json")
	if !strings.Contains(syncOut, fmt.Sprintf(`"total_records": %d`, total)) && !strings.Contains(syncOut, fmt.Sprintf(`"total_records":%d`, total)) {
		t.Errorf("sync did not store every user; the fake does not page as documented:\n%.800s", syncOut)
	}
}
