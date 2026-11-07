// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (NOT generated): tactical-rmm handfix cohort-filter-fails-closed.

package cli

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// A cohort write (maintenance set, agents bulk-run) reaches only the agents the
// operator selected. A filter entry the parser cannot apply (unknown key,
// missing "=", empty value, a non-boolean online) and an empty filter without
// --all are refused before any request, never widened to the whole fleet.
func TestCohortWritesRefuseFiltersTheyCannotApply(t *testing.T) {
	db := novelTestEnv(t)
	seedResource(t, db, "agents", "a1", `{"agent_id":"a1","hostname":"acme-ws","client_name":"Acme","site_name":"HQ","plat":"windows","status":"online"}`)
	seedResource(t, db, "agents", "b1", `{"agent_id":"b1","hostname":"beta-ws","client_name":"Beta","site_name":"HQ","plat":"windows","status":"online"}`)
	writes := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writes++
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	t.Setenv("TACTICAL_RMM_BASE_URL", srv.URL)
	t.Setenv("TRMM_API_KEY", "test-key")

	bad := [][]string{
		{"--filter", "client_name=Acme"},
		{"--filter", "client"},
		{"--filter", "client="},
		{"--filter", "online=yes"},
		{"--filter", "client=Acme,clinet=Beta"},
		{},
	}
	for _, f := range bad {
		for _, base := range [][]string{{"maintenance", "set", "--execute"}, {"agents", "bulk-run", "--command", "whoami", "--execute"}} {
			args := append(append([]string{}, base...), f...)
			if _, _, err := runNovel(t, args...); err == nil {
				t.Errorf("%v: accepted, want a usage error naming the filter", args)
			}
		}
	}
	if writes != 0 {
		t.Fatalf("%d write requests sent for filters the selector cannot apply", writes)
	}

	if _, stderr, err := runNovel(t, "maintenance", "set", "--filter", "client=Acme", "--execute"); err != nil {
		t.Fatalf("maintenance set --filter client=Acme: %v\n%s", err, stderr)
	}
	if writes != 1 {
		t.Errorf("client=Acme sent %d writes, want 1 (only acme-ws)", writes)
	}
}
