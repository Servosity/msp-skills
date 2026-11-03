// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (NOT generated): tactical-rmm handfix action-gets-never-read.

package cli

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spf13/cobra"
)

// Two Tactical RMM GETs act on the server (amidaware/tacticalrmm
// agents/views.py bulk_agent_recovery queues fleet-wide agent recovery;
// core/views.py clear_cache clears the server cache). The spec marks them
// x-pp-mutation, so their commands carry no mcp:read-only. No path the CLI
// reads on its own (sync, export, tail, workflow archive, local-first reads)
// may resolve to such a GET: only the operator's explicit command may call it.
func TestActionGETsAreNeverReadPaths(t *testing.T) {
	action := map[string]bool{}
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		if c.Annotations["pp:method"] == "GET" && c.Annotations["mcp:read-only"] != "true" {
			action[c.Annotations["pp:path"]] = true
		}
		for _, child := range c.Commands() {
			walk(child)
		}
	}
	walk(RootCmd())
	// The first two act on the server; the rest dispatch a command to the
	// managed agent (vendor GET handlers calling nats_cmd or send_*).
	for _, p := range []string{"/agents/bulkrecovery/", "/core/clearcache/", "/agents/{agent_id}/ping/",
		"/agents/{agent_id}/processes/", "/agents/{agent_id}/processes/{pid}/", "/agents/{agent_id}/eventlog/{logtype}/{days}/",
		"/agents/{agent_id}/registry/", "/services/{agent_id}/", "/services/{agent_id}/{svcname}/"} {
		if !action[p] {
			t.Fatalf("GET %s is not marked a mutation; the spec repair (x-pp-mutation) was lost", p)
		}
	}
	for _, r := range append(knownSyncResourceNames(), defaultSyncResources()...) {
		if p, err := syncResourcePath(r); err == nil && action[p] {
			t.Errorf("sync resource %q calls the action GET %s", r, p)
		}
	}
	for name, table := range map[string]map[string]string{"resourceReadPaths": resourceReadPaths, "resourceDetailPaths": resourceDetailPaths} {
		for r, p := range table {
			if action[p] {
				t.Errorf("%s[%q] reads the action GET %s", name, r, p)
			}
		}
	}
}

// Codex review r2: an id must not retarget a remaining read template onto an
// action route (export agents bulkrecovery -> GET /agents/bulkrecovery/).
func TestExportIDCannotRetargetAnActionRoute(t *testing.T) {
	novelTestEnv(t)
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`"Agents will now be recovered"`))
	}))
	defer srv.Close()
	t.Setenv("TACTICAL_RMM_BASE_URL", srv.URL)
	t.Setenv("TRMM_API_KEY", "test-key")
	for _, id := range []string{"bulkrecovery", "abc/ping", "abc%2Fping"} {
		if _, _, err := runNovel(t, "export", "agents", id, "--no-cache"); err == nil {
			t.Errorf("export agents %q succeeded", id)
		}
	}
	if hits != 0 {
		t.Fatalf("export reached the server %d time(s) on an action route", hits)
	}
}
