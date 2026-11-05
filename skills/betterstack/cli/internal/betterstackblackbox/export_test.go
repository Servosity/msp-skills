// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package betterstackblackbox

import (
	"bytes"
	"context"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
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
	// The fake answers the heartbeat groups list 401.
	sv.mu.Lock()
	sv.denied["/heartbeat-groups"] = true
	sv.mu.Unlock()
	cmd := exec.CommandContext(ctx, bbCLI, "export", "heartbeat-groups", "--output", filepath.Join(e.home, "groups.jsonl"))
	cmd.Env = e.env
	cmd.Dir = e.home
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("export --output did not return after Better Stack answered 401:\n%s", out)
	}
	if err == nil {
		t.Fatalf("export --output succeeded on a 401:\n%s", out)
	}
	// The export reached the API (an argument error would fail fast too).
	sv.mu.Lock()
	defer sv.mu.Unlock()
	var asked bool
	for _, m := range sv.methods {
		asked = asked || m == "GET /api/v2/heartbeat-groups"
	}
	if !asked {
		t.Fatalf("export never asked the API for the heartbeat groups; the test did not run:\n%s", out)
	}
}

// pagerServer is a Better Stack list fake for /heartbeat-groups with total
// records, paged as documented: per_page records a page (default 50, max
// 250), full pages until the last, an absolute next link while more remain.
// It counts the requests it answered.
func pagerServer(t *testing.T, total int) (*server, *int) {
	t.Helper()
	var mu sync.Mutex
	asked := new(int)
	origin := ""
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		*asked++
		mu.Unlock()
		q := r.URL.Query()
		per, page := 50, 1
		if n, err := strconv.Atoi(q.Get("per_page")); err == nil && n > 0 {
			per = min(n, 250)
		}
		if n, err := strconv.Atoi(q.Get("page")); err == nil && n > 0 {
			page = n
		}
		var items []string
		for i := (page-1)*per + 1; i <= min(page*per, total); i++ {
			items = append(items, jsonItem(strconv.Itoa(1000+i), "heartbeat_group", fmt.Sprintf(`"name":"Paged group %d","sort_index":%d,"paused":false`, i, i)))
		}
		next := "null"
		if page*per < total {
			next = fmt.Sprintf("%q", fmt.Sprintf("%s%s?page=%d&per_page=%d", origin, r.URL.Path, page+1, per))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"data":[%s],"pagination":{"first":null,"last":null,"prev":null,"next":%s}}`, strings.Join(items, ","), next)
	}))
	origin = srv.URL
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	return &server{url: srv.URL + "/api/v2", caFile: ca}, asked
}

// P1 (wrong result on a documented response shape, not a credential class):
// export walked one page. Every resource's read config (resource_paths.go,
// generated) declares no pagination, so export stopped after its first
// request: an account with more records than one page (Better Stack's
// default page is 50) exported the first 50, silently, exit 0, to a file
// documented as a backup or migration. sync walks every page (its own pager)
// and is the control. The fix is the anchored generated-file edit named in
// the black-box report (resourceReadConfigs: paginationType "page",
// cursorParam "page", limitParam "per_page"), re-applied on every render by
// tools/apply_edits.py (betterstack handfix export-walks-every-page).
func TestExportWalksEveryPage(t *testing.T) {
	const total = 130
	sv, asked := pagerServer(t, total)
	e := newEnv(t, sv, credEnv)
	file := filepath.Join(e.home, "groups.jsonl")
	out := e.run(t, "export", "heartbeat-groups", "--format", "jsonl", "--output", file)
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("export wrote no file: %v\n%s", err, out)
	}
	if n := bytes.Count(b, []byte(`"Paged group `)); n != total {
		t.Errorf("export --output wrote %d of %d records (%d requests):\n%.600s", n, total, *asked, out)
	}
	if n := strings.Count(e.run(t, "export", "heartbeat-groups", "--format", "json"), `"Paged group `); n != total {
		t.Errorf("export --format json printed %d of %d records", n, total)
	}
	// Control: sync walks every page of the same fake.
	syncOut := e.run(t, "sync", "--resources", "heartbeat-groups", "--json")
	if !strings.Contains(syncOut, fmt.Sprintf(`"total_records": %d`, total)) {
		t.Errorf("sync did not store every record; the fake does not page as documented:\n%.800s", syncOut)
	}
}
