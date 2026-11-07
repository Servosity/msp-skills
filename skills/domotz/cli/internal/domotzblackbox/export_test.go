// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package domotzblackbox

import (
	"bytes"
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
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// The fake answers the custom drivers list 401.
	sv.mu.Lock()
	sv.denied["/custom-driver"] = true
	sv.mu.Unlock()
	cmd := exec.CommandContext(ctx, bbCLI, "export", "custom-driver", "--output", filepath.Join(e.home, "drivers.jsonl"))
	cmd.Env = e.env
	cmd.Dir = e.home
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("export --output did not return after Domotz answered 401:\n%s", out)
	}
	if err == nil {
		t.Fatalf("export --output succeeded on a 401:\n%s", out)
	}
	// The export reached the API (an argument error would fail fast too).
	sv.mu.Lock()
	defer sv.mu.Unlock()
	var asked bool
	for _, m := range sv.methods {
		asked = asked || m == "GET /public-api/v1/custom-driver"
	}
	if !asked {
		t.Fatalf("export never asked the API for the custom drivers; the test did not run:\n%s", out)
	}
}

// pagerServer is a Domotz fake whose collector list (GET /agent) holds total
// collectors, paged as documented (spec.json listAgents; the vendor's
// parameter docs: "page_number: The requested page number, 0-indexed"): a
// bare JSON array of page_size records (default 10), 0-indexed page_number
// (default 0), the total in X-Entities-Count, full pages until the last.
// Every other read is an empty list. It counts the requests it answered and
// the page numbers it served.
func pagerServer(t *testing.T, total int) (*server, *int) {
	t.Helper()
	var mu sync.Mutex
	asked := new(int)
	sv := &server{keys: map[string]int{}, pages: map[string]int{}, denied: map[string]bool{}}
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		*asked++
		mu.Unlock()
		p := strings.TrimPrefix(r.URL.Path, "/public-api/v1")
		w.Header().Set("Content-Type", "application/json")
		if p != "/agent" {
			fmt.Fprint(w, `[]`)
			return
		}
		q := r.URL.Query()
		sv.mu.Lock()
		sv.pages["/agent?page_number="+q.Get("page_number")]++
		sv.mu.Unlock()
		var items []string
		for i := 1; i <= total; i++ {
			items = append(items, fmt.Sprintf(`{"id":%d,"display_name":"Paged site %d","status":{"value":"ONLINE"}}`, 1000+i, i))
		}
		fmt.Fprint(w, pagedArray(w, q, 10, items))
	}))
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	sv.url, sv.caFile = srv.URL+"/public-api/v1", ca
	return sv, asked
}

// A wrong result on a documented response shape (not a credential class):
// export walks every page of the collector list, from the vendor's first
// page (page_number 0), and stops after the last; an account with more
// collectors than one page (Domotz's default page_size is 10) must export
// every one, to a file documented as a backup or migration. sync walks the
// same list (its own pager) and is the control.
//
// Finding (class zero-indexed-page-number, P1 wrong result): export, the MCP
// export tool and `agent list --all` number pages from 1, so every export
// misses the first 10 collectors and --all misses the second page. Fixed by
// two anchored edits in generated files (resource_paths.go resourcePageParams,
// helpers.go nextClientSidePaginationCursor; tools/apply_edits.py, domotz
// handfix page-number-zero-indexed).
func TestExportWalksEveryPage(t *testing.T) {
	t.Parallel()
	const total = 25
	sv, asked := pagerServer(t, total)
	e := newEnv(t, sv, credEnv)
	file := filepath.Join(e.home, "agents.jsonl")
	out := e.run(t, "export", "agent", "--format", "jsonl", "--output", file)
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("export wrote no file: %v\n%s", err, out)
	}
	// countSites counts the distinct collectors s names (raw JSON or a JSON
	// string inside an MCP result).
	countSites := func(s string) int {
		seen := map[string]bool{}
		for _, m := range regexp.MustCompile(`Paged site ([0-9]+)\b`).FindAllStringSubmatch(s, -1) {
			if n, err := strconv.Atoi(m[1]); err == nil && n >= 1 && n <= total {
				seen[m[1]] = true
			}
		}
		return len(seen)
	}
	if n, dup := countSites(string(b)), bytes.Count(b, []byte(`"Paged site `)); n != total || dup != total {
		sv.mu.Lock()
		t.Errorf("export --output wrote %d distinct of %d collectors (%d records, %d requests, pages %v):\n%.600s", n, total, dup, *asked, sv.pages, out)
		sv.mu.Unlock()
	}
	if got := e.run(t, "export", "agent", "--format", "json"); countSites(got) != total || strings.Count(got, `"Paged site `) != total {
		t.Errorf("export --format json printed %d distinct of %d collectors", countSites(got), total)
	}
	if got := e.mcpCalls(t, []map[string]any{tool("export", map[string]any{"resource": "agent", "format": "json"})}); countSites(got) != total {
		t.Errorf("the MCP export tool returned %d distinct of %d collectors:\n%.1500s", countSites(got), total, got)
	}
	// The live list's --all walks the same pages (its own client-side pager).
	if got := e.run(t, "agent", "list", "--all", "--json"); countSites(got) != total {
		sv.mu.Lock()
		t.Errorf("agent list --all printed %d distinct of %d collectors (pages %v)", countSites(got), total, sv.pages)
		sv.mu.Unlock()
	}
	// Control: sync walks every page of the same fake.
	syncOut := e.run(t, "sync", "--resources", "agent", "--json")
	if !strings.Contains(syncOut, fmt.Sprintf(`"total_records": %d`, total)) && !strings.Contains(syncOut, fmt.Sprintf(`"total_records":%d`, total)) {
		t.Errorf("sync did not store every collector; the fake does not page as documented:\n%.800s", syncOut)
	}
}
