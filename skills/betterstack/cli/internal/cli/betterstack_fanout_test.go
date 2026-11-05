// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"betterstack-pp-cli/internal/cliutil/testenv"
	"betterstack-pp-cli/internal/store"
)

// Class all-fetches-failed-read-as-zero (press-traps s3): statuspage-audit
// fans out one live GET /status-pages/{id}/resources per page. When every one
// of those fetches failed nothing was audited live, so "No status-page drift
// found" (exit 0) would be a wrong answer: the command fails, naming the first
// error beside fetch_failures. One failed page of two still reports, with the
// failure listed.
func TestStatuspageAuditFailsWhenEveryLiveFetchFailed(t *testing.T) {
	testenv.Isolate(t)
	failAll := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failAll || strings.Contains(r.URL.Path, "/status-pages/11/") {
			http.Error(w, `{"errors":"internal"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[],"pagination":{"next":null}}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("BETTERSTACK_BASE_URL", srv.URL+"/api/v2")
	t.Setenv("BETTERSTACK_API_TOKEN", "fanoutUnitToken-6d2b9e")
	db := filepath.Join(t.TempDir(), "data.db")
	s, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"11", "12"} {
		rec := `{"id":"` + id + `","type":"status_page","attributes":{"company_name":"Acme ` + id + `","subdomain":"acme` + id + `","aggregate_state":"operational"}}`
		if err := s.Upsert("status-pages", id, []byte(rec)); err != nil {
			t.Fatal(err)
		}
	}
	for _, r := range defaultSyncResources() {
		if err := s.SaveSyncState(r, "", 0); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	run := func() (string, error) {
		cmd := RootCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{"statuspage-audit", "--db", db, "--json"})
		err := cmd.Execute()
		return out.String(), err
	}
	out, err := run()
	if err == nil || !strings.Contains(err.Error(), "first error") {
		t.Errorf("every live fetch failed, want a failure naming the first error; err %v, out %s", err, out)
	}
	failAll = false
	out, err = run()
	if err != nil || !strings.Contains(out, "fetch_failures") || !strings.Contains(out, "first_error") {
		t.Errorf("one of two fetches failed: want a report listing it; err %v, out %s", err, out)
	}
}
