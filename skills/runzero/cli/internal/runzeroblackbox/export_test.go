// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package runzeroblackbox

import (
	"context"
	"encoding/json"
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
	// The fake answers the asset export 401.
	sv.mu.Lock()
	sv.denied["/export/org/assets.json"] = true
	sv.mu.Unlock()
	cmd := exec.CommandContext(ctx, bbCLI, "export", "runzero-export", "--output", filepath.Join(e.home, "assets.jsonl"))
	cmd.Env = e.env
	cmd.Dir = e.home
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("export --output did not return after runZero answered 401:\n%s", out)
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
		asked = asked || m == "GET /export/org/assets.json"
	}
	if !asked {
		t.Fatalf("export never asked the API for the assets; the test did not run:\n%s", out)
	}
}

// pagerServer is a runZero fake whose asset export (GET
// /api/v1.0/export/org/assets.json) holds total assets, as documented
// (spec.yaml AssetExportResponse): without page_size one bare array of every
// asset; with page_size an AssetPage {"assets":[...],"next_key":"..."} whose
// next_key the next request sends as start_key, "" on the last page. Every
// other read is an empty array. It counts the requests it answered and the
// pages it served.
func pagerServer(t *testing.T, total int) (*server, *int) {
	t.Helper()
	var mu sync.Mutex
	asked := new(int)
	sv := newServer()
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		*asked++
		mu.Unlock()
		p, q := strings.TrimPrefix(r.URL.Path, apiBase), r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		if p != "/export/org/assets.json" {
			fmt.Fprint(w, `[]`)
			return
		}
		sv.mu.Lock()
		sv.pages[p+"?page_size="+q.Get("page_size")+"&start_key="+q.Get("start_key")]++
		sv.mu.Unlock()
		var items []string
		for i := 1; i <= total; i++ {
			// Compact records, so the MCP result of every asset stays inside its
			// 60000-byte budget.
			items = append(items, fmt.Sprintf(`{"id":"asset-paged-%04d","names":["paged asset %d"],"addresses":["10.9.%d.%d"]}`, i, i, i/250, i%250))
		}
		size, err := strconv.Atoi(q.Get("page_size"))
		if err != nil || size <= 0 {
			fmt.Fprintf(w, "[%s]", strings.Join(items, ","))
			return
		}
		start := 0
		if k := q.Get("start_key"); k != "" {
			n, err := strconv.Atoi(strings.TrimPrefix(k, "key-"))
			if err != nil || n < 0 {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprint(w, `{"error":"invalid start_key"}`)
				return
			}
			start = n
		}
		start = min(start, len(items))
		end := min(start+size, len(items))
		next := ""
		if end < len(items) {
			next = fmt.Sprintf("key-%d", end)
		}
		fmt.Fprintf(w, `{"assets":[%s],"next_key":%q}`, strings.Join(items[start:end], ","), next)
	}))
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	sv.url, sv.caFile = srv.URL, ca
	return sv, asked
}

// countAssets counts the distinct paged assets s names (raw JSON or a JSON
// string inside an MCP result).
func countAssets(s string, total int) int {
	seen := map[string]bool{}
	for _, m := range regexp.MustCompile(`paged asset ([0-9]+)\\?"`).FindAllStringSubmatch(s, -1) {
		if n, err := strconv.Atoi(m[1]); err == nil && n >= 1 && n <= total {
			seen[m[1]] = true
		}
	}
	return len(seen)
}

// A wrong result on a documented response shape (not a credential class):
// the asset export is runZero's whole inventory, and every reader must
// return every asset: export (to a file documented as a backup or
// migration, one record per asset), the MCP export tool, sync, and the live
// command with and without runZero's own paging (page_size and next_key,
// spec.yaml /export/org/assets.json).
func TestExportWalksEveryPage(t *testing.T) {
	t.Parallel()      // its own fake runZero and home: runs beside the other long batteries
	const total = 110 // more than one page at 100 and at 7
	sv, asked := pagerServer(t, total)
	e := newEnv(t, sv, credEnv)
	file := filepath.Join(e.home, "assets.jsonl")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bbCLI, "export", "runzero-export", "--format", "jsonl", "--output", file)
	cmd.Env, cmd.Dir = e.env, e.home
	outb, _ := cmd.CombinedOutput()
	out := string(outb)
	if ctx.Err() != nil {
		sv.mu.Lock()
		t.Fatalf("export runzero-export did not finish in 60s (%d requests):\n%.600s", *asked, out)
		sv.mu.Unlock()
	}
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("export wrote no file: %v\n%s", err, out)
	}
	// records counts the JSONL records that are one asset each.
	records := func(s string) (assets, other int) {
		for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
			if strings.HasPrefix(line, `{"`) && strings.Count(line, "asset-paged-") == 1 {
				assets++
			} else if strings.TrimSpace(line) != "" {
				other++
			}
		}
		return assets, other
	}
	if n := countAssets(string(b), total); n != total {
		t.Errorf("export --output wrote %d distinct of %d assets", n, total)
	}
	if assets, other := records(string(b)); assets != total || other != 0 {
		t.Errorf("export --output wrote %d asset records and %d other lines, want %d and 0:\n%.600s", assets, other, total, b)
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel2()
	cmd = exec.CommandContext(ctx2, bbCLI, "export", "runzero-export", "--format", "json")
	cmd.Env, cmd.Dir = e.env, e.home
	got, _ := cmd.Output()
	var arr []map[string]any
	if err := json.Unmarshal(got, &arr); err != nil || len(arr) != total || countAssets(string(got), total) != total {
		t.Errorf("export --format json printed %d records (%d distinct assets, err %v), want %d", len(arr), countAssets(string(got), total), err, total)
	}
	if got := e.mcpCalls(t, []map[string]any{tool("export", map[string]any{"resource": "runzero-export", "format": "json"})}); countAssets(got, total) != total {
		t.Errorf("the MCP export tool returned %d distinct of %d assets:\n%.600s", countAssets(got, total), total, got)
	}
	// The live command: the whole bare array.
	if got := e.run(t, "runzero-export", "assets-json", "--json"); countAssets(got, total) != total {
		t.Errorf("runzero-export assets-json printed %d distinct of %d assets", countAssets(got, total), total)
	}
	// sync stores every asset of the same fake.
	syncOut := e.run(t, "sync", "--resources", "runzero-export", "--json")
	if !regexp.MustCompile(`"fetched":\s*` + strconv.Itoa(total) + `\b`).MatchString(syncOut) {
		t.Errorf("sync did not fetch every asset:\n%.800s", syncOut)
	}
}

// P1 export-next-key-walked (a wrong result on a documented shape: spec.yaml
// /export/org/assets.json, AssetPage.next_key and the start_key parameter;
// the same on services, software, vulnerabilities, wireless and
// /account/events.json): with --page-size the export answers one page and a
// next_key, and --all ("Fetch all pages") printed the first page only, so an
// MSP exporting the inventory in pages got the first page as the whole
// (RUN/seen/export-next-key-walked-seen-failing.log). The generated commands
// pass pagination "none" (the spec repair that keeps sync from paging), so
// --all never reads next_key. The fix is an anchored edit to the six
// generated commands (cursor pagination on start_key/next_key, page_size the
// limit), proven in a scratch copy
// (RUN/seen/export-next-key-walked-proposed-edit.log); this test fails until
// tools/apply_credential_rules.py carries it.
func TestExportAllWalksTheNextKey(t *testing.T) {
	t.Parallel()      // its own fake runZero and home: runs beside the other long batteries
	const total = 110 // more than one page at 100 and at 7
	sv, _ := pagerServer(t, total)
	e := newEnv(t, sv, credEnv)
	for _, size := range []string{"100", "7"} {
		if got := e.run(t, "runzero-export", "assets-json", "--page-size", size, "--all", "--json"); countAssets(got, total) != total {
			sv.mu.Lock()
			t.Errorf("runzero-export assets-json --page-size %s --all printed %d distinct of %d assets (pages %v):\n%.400s", size, countAssets(got, total), total, sv.pages, got)
			sv.mu.Unlock()
		}
	}
	// Without --all one page prints, and its next_key with it.
	if got := e.run(t, "runzero-export", "assets-json", "--page-size", "100", "--json"); countAssets(got, total) != 100 || !strings.Contains(got, "key-100") {
		t.Errorf("one page of 100 did not print with its next_key:\n%.400s", got)
	}
}
