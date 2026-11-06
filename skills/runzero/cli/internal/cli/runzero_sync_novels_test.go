// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"runzero-pp-cli/internal/cliutil/testenv"

	_ "modernc.org/sqlite"
)

// fakeRunzero serves the export rows runZero's OpenAPI documents (Service,
// Software and Vulnerability rows embed their asset: `id` is the asset's id)
// under /api/v1.0 and records every request line it was asked.
type fakeRunzero struct {
	mu     sync.Mutex
	bodies map[string]string
	seen   []string
}

func newFakeRunzero(t *testing.T) *fakeRunzero {
	f := &fakeRunzero{bodies: map[string]string{
		"/api/v1.0/export/org/assets.json": `[{"id":"asset-1","names":["web01"],"addresses":["10.0.0.5"],"os":"Ubuntu","alive":true}]`,
		"/api/v1.0/export/org/services.json": `[` +
			`{"id":"asset-1","names":["web01"],"service_id":"svc-22","service_asset_id":"asset-1","service_address":"10.0.0.5",` +
			`"service_transport":"tcp","service_port":22,"service_protocol":"ssh"},` +
			`{"id":"asset-1","names":["web01"],"service_id":"svc-443","service_asset_id":"asset-1","service_address":"10.0.0.5",` +
			`"service_transport":"tcp","service_port":443,"service_protocol":"tls"}]`,
		"/api/v1.0/export/org/vulnerabilities.json": `[{"id":"asset-1","vulnerability_id":"v-1","vulnerability_asset_id":"asset-1",` +
			`"vulnerability_name":"regreSSHion","vulnerability_cve":"CVE-2024-6387","vulnerability_severity":"Critical"}]`,
	}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		p := strings.TrimSuffix(r.URL.Path, "/")
		f.mu.Lock()
		f.seen = append(f.seen, p+"?"+r.URL.RawQuery)
		b, ok := f.bodies[p]
		f.mu.Unlock()
		if ok {
			fmt.Fprint(w, b)
			return
		}
		fmt.Fprint(w, `[]`)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("RUNZERO_BASE_URL", srv.URL+"/api/v1.0")
	t.Setenv("RUNZERO_API_KEY", "OTsyncNovelsKey3f8a2c71d9e4")
	return f
}

func (f *fakeRunzero) requests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.seen...)
}

func runRunzero(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := RootCmd()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errb.String(), err
}

func mustRunRunzero(t *testing.T, args ...string) string {
	t.Helper()
	out, errOut, err := runRunzero(t, args...)
	if err != nil {
		t.Fatalf("%v: %v\n%s%s", args, err, out, errOut)
	}
	return out
}

// Class export-pages-not-truncated (spec repair, tools/spec_fix.py): with
// `page_size` the export endpoints answer one page and a `next_key` press
// 4.34.2 does not follow, so the generated sync stopped at 100 rows. Sync now
// asks each export without page_size (the vendor then returns every row) and
// keys service rows on service_id (export-record-keys: their `id` is the
// asset's, so both services of asset-1 were one row).
func TestDefaultSyncTakesWholeExports(t *testing.T) {
	testenv.Isolate(t)
	fake := newFakeRunzero(t)
	db := filepath.Join(t.TempDir(), "data.db")
	mustRunRunzero(t, "sync", "--db", db, "--json", "--no-learn")

	asked := fake.requests()
	sawExport := false
	for _, req := range asked {
		if strings.HasPrefix(req, "/api/v1.0/export/org/services.json") {
			sawExport = true
		}
		if strings.Contains(req, "page_size=") || strings.Contains(req, "start_key=") {
			t.Errorf("sync paged an export it cannot follow: %s", req)
		}
	}
	if !sawExport {
		t.Fatalf("default sync never asked the services export (asked: %v)", asked)
	}
	h, err := sql.Open("sqlite", db)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	var n int
	if err := h.QueryRow(`SELECT COUNT(*) FROM resources WHERE resource_type = 'runzero-export-services-json'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("stored service rows = %d, want 2 (one per service_id)", n)
	}
}

// The local novels read what `inventory sync` stores from vendor-shaped rows
// (inventory-vendor-row-fields): triage counts both services and the finding.
func TestInventorySyncFeedsTheNovels(t *testing.T) {
	testenv.Isolate(t)
	newFakeRunzero(t)
	db := filepath.Join(t.TempDir(), "data.db")
	mustRunRunzero(t, "inventory", "sync", "--db", db, "--json")
	var rows []struct {
		AssetID      string `json:"asset_id"`
		ServiceCount int    `json:"service_count"`
		VulnCount    int    `json:"vuln_count"`
		TopCVE       string `json:"top_cve"`
	}
	out := mustRunRunzero(t, "triage", "--db", db, "--json")
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &rows); err != nil {
		var env struct {
			Results json.RawMessage `json:"results"`
		}
		if json.Unmarshal([]byte(out), &env) != nil || json.Unmarshal(env.Results, &rows) != nil {
			t.Fatalf("triage output not JSON rows: %v\n%s", err, out)
		}
	}
	if len(rows) != 1 || rows[0].AssetID != "asset-1" || rows[0].ServiceCount != 2 || rows[0].VulnCount != 1 || rows[0].TopCVE != "CVE-2024-6387" {
		t.Errorf("triage = %+v, want asset-1 with 2 services, 1 finding, CVE-2024-6387", rows)
	}
}
