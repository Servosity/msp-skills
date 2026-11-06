// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Codex round 1 of the runzero 4.34.2 reprint review, class
// incomplete-inventory-read-as-complete: `inventory sync` answered exit 0 when
// runZero refused an export (the spec's documented 403), and the novels then
// answered from the incomplete store as if it were whole (triage: vuln_count 0
// for every asset). Both print their result and exit 6 (partial failure)
// naming what is missing, unless --allow-partial-failure.

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"runzero-pp-cli/internal/cliutil/testenv"
)

func newRefusingRunzero(t *testing.T, refuse string) {
	t.Helper()
	bodies := map[string]string{
		"/api/v1.0/export/org/assets.json": `[{"id":"asset-1","names":["web01"],"addresses":["10.0.0.5"],"alive":true}]`,
		"/api/v1.0/export/org/services.json": `[{"id":"asset-1","service_id":"svc-22","service_asset_id":"asset-1",` +
			`"service_address":"10.0.0.5","service_transport":"tcp","service_port":22,"service_protocol":"ssh"}]`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		p := strings.TrimSuffix(r.URL.Path, "/")
		if p == refuse {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"error":"NotAllowedForLicenseError"}`)
			return
		}
		if b, ok := bodies[p]; ok {
			fmt.Fprint(w, b)
			return
		}
		fmt.Fprint(w, `[]`)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("RUNZERO_BASE_URL", srv.URL+"/api/v1.0")
	t.Setenv("RUNZERO_API_KEY", "OTr1IncompleteKey5c2e9a7d31")
}

func exitCodeOf(err error) int {
	var ce *cliError
	if errors.As(err, &ce) {
		return ce.code
	}
	if err != nil {
		return 1
	}
	return 0
}

func TestR1IncompleteInventoryIsAPartialFailure(t *testing.T) {
	testenv.Isolate(t)
	newRefusingRunzero(t, "/api/v1.0/export/org/vulnerabilities.json")
	db := filepath.Join(t.TempDir(), "data.db")

	out, _, err := runRunzero(t, "inventory", "sync", "--db", db, "--json", "--no-learn")
	if exitCodeOf(err) != 6 || !strings.Contains(out, "vulnerabilities") {
		t.Errorf("inventory sync with a refused export: exit %d (%v), want 6 with the error printed:\n%s", exitCodeOf(err), err, out)
	}
	for _, novel := range [][]string{
		{"triage"}, {"affected", "CVE-2024-6387"}, {"exposure-delta"},
	} {
		args := append(append([]string{}, novel...), "--db", db, "--json", "--no-learn")
		out, _, err := runRunzero(t, args...)
		if exitCodeOf(err) != 6 || err == nil || !strings.Contains(err.Error(), "vulnerabilities") {
			t.Errorf("%v on a store without vulnerabilities: exit %d (%v), want 6 naming vulnerabilities\n%s", novel, exitCodeOf(err), err, out)
		}
	}
	if out, _, err := runRunzero(t, "triage", "--db", db, "--json", "--no-learn"); !strings.Contains(out, "asset-1") {
		t.Errorf("triage withheld its rows on a partial store (%v):\n%s", err, out)
	}
	if _, _, err := runRunzero(t, "triage", "--db", db, "--json", "--no-learn", "--allow-partial-failure"); err != nil {
		t.Errorf("triage --allow-partial-failure: %v, want exit 0", err)
	}
	// What the store does hold is complete for a result that reads only it.
	if _, _, err := runRunzero(t, "stale", "--db", db, "--json", "--no-learn"); err != nil {
		t.Errorf("stale reads only assets, which synced: %v", err)
	}
}

// Class filtered-snapshot-read-as-complete (Codex r1 [P1], r1-compare-same-scope):
// a result read from entities synced under one --search filter covers only
// what that filter matched, and says so on stderr (an MCP tool result carries
// the warning line); mixed filters are a partial result (exit 6).
func TestR1FilteredSnapshotIsLabelled(t *testing.T) {
	testenv.Isolate(t)
	newRefusingRunzero(t, "")
	db := filepath.Join(t.TempDir(), "data.db")
	mustRunRunzero(t, "inventory", "sync", "--db", db, "--json", "--no-learn", "--search", "alive:t")
	out, errOut, err := runRunzero(t, "triage", "--db", db, "--json", "--no-learn")
	if err != nil || !strings.Contains(out, "asset-1") || !strings.Contains(errOut, "warning:") || !strings.Contains(errOut, `"alive:t"`) {
		t.Errorf("triage over a filtered snapshot: %v, stderr %q, want rows and a warning naming the filter", err, errOut)
	}
	mustRunRunzero(t, "inventory", "sync", "--db", db, "--json", "--no-learn", "--only", "vulnerabilities")
	if _, _, err := runRunzero(t, "triage", "--db", db, "--json", "--no-learn"); exitCodeOf(err) != 6 || !strings.Contains(err.Error(), "different filters") {
		t.Errorf("triage over mixed filters: exit %d (%v), want 6 naming the filters", exitCodeOf(err), err)
	}
}
