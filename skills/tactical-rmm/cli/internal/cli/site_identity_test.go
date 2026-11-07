// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (NOT generated): tactical-rmm handfix site-identity-client-and-site.

package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// seedTwoMainSites stores the executed repro (.review/adv/t11.sh): two clients
// that each have a site called "Main". Site names are unique only within a
// client (vendor-src/clients/views.py GetAddSites, SiteSerializer), so these
// are two sites.
func seedTwoMainSites(t *testing.T, db string) {
	t.Helper()
	seedResource(t, db, "clients", "1", `{"id":1,"name":"Acme"}`)
	seedResource(t, db, "clients", "2", `{"id":2,"name":"Beta Corp"}`)
	seedResource(t, db, "clients-sites", "1", `{"id":1,"name":"Main","client":1,"client_name":"Acme"}`)
	seedResource(t, db, "clients-sites", "3", `{"id":3,"name":"Main","client":2,"client_name":"Beta Corp"}`)
	seedResource(t, db, "agents", "a1", `{"agent_id":"a1","hostname":"HOST1","client_name":"Acme","site_name":"Main","has_patches_pending":true,"needs_reboot":true,"status":"online","plat":"windows"}`)
	seedResource(t, db, "agents", "a2", `{"agent_id":"a2","hostname":"HOST2","client_name":"Acme","site_name":"Main","has_patches_pending":false,"needs_reboot":false,"status":"offline","plat":"windows"}`)
	seedResource(t, db, "agents", "a3", `{"agent_id":"a3","hostname":"HOST3","client_name":"Beta Corp","site_name":"Main","has_patches_pending":true,"needs_reboot":false,"status":"overdue","plat":"windows"}`)
	seedResource(t, db, "agents", "a4", `{"agent_id":"a4","hostname":"HOST4","client_name":"Beta Corp","site_name":"Main","has_patches_pending":true,"needs_reboot":false,"status":"online","plat":"windows"}`)
}

type postureRow struct {
	Group          string `json:"group"`
	Client         string `json:"client"`
	Agents         int    `json:"agents"`
	PatchesPending int    `json:"patches_pending"`
	RebootPending  int    `json:"reboot_pending"`
}

// patch posture --by site keeps one row per real site: Acme/Main and
// Beta Corp/Main are two rows, each naming its client, never one merged "Main".
func TestPatchPostureBySiteKeepsSameNamedSitesApart(t *testing.T) {
	db := novelTestEnv(t)
	seedTwoMainSites(t, db)

	out, stderr, err := runNovel(t, "patch", "posture", "--by", "site")
	if err != nil {
		t.Fatalf("patch posture --by site: %v\n%s", err, stderr)
	}
	var rows []postureRow
	assertJSON(t, out, &rows)
	got := map[string]postureRow{}
	for _, r := range rows {
		got[r.Client+"/"+r.Group] = r
	}
	want := map[string]postureRow{
		"Acme/Main":      {Group: "Main", Client: "Acme", Agents: 2, PatchesPending: 1, RebootPending: 1},
		"Beta Corp/Main": {Group: "Main", Client: "Beta Corp", Agents: 2, PatchesPending: 2, RebootPending: 0},
	}
	if len(rows) != len(want) {
		t.Fatalf("patch posture --by site = %s, want two rows (Acme/Main and Beta Corp/Main)", strings.TrimSpace(out))
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("row %s = %+v, want %+v (full output %s)", k, got[k], w, strings.TrimSpace(out))
		}
	}
}

// An agent row from GET /agents/{agent_id}/ (write-through cache) carries the
// site id in "site" and the client name in "client", not client_name. The id
// is preferred and resolved through the synced site list, so the row lands in
// its own site, not in an "(unknown)" client.
func TestPatchPostureBySitePrefersTheSiteIDOnDetailRows(t *testing.T) {
	db := novelTestEnv(t)
	seedTwoMainSites(t, db)
	seedResource(t, db, "agents", "a5", `{"agent_id":"a5","hostname":"HOST5","client":"Beta Corp","site":3,"site_name":"Main","has_patches_pending":false,"needs_reboot":true}`)

	out, stderr, err := runNovel(t, "patch", "posture", "--by", "site")
	if err != nil {
		t.Fatalf("patch posture --by site: %v\n%s", err, stderr)
	}
	var rows []postureRow
	assertJSON(t, out, &rows)
	got := map[string]postureRow{}
	for _, r := range rows {
		got[r.Client+"/"+r.Group] = r
	}
	want := postureRow{Group: "Main", Client: "Beta Corp", Agents: 3, PatchesPending: 2, RebootPending: 1}
	if len(rows) != 2 || got["Beta Corp/Main"] != want {
		t.Errorf("patch posture --by site = %s, want Beta Corp/Main %+v and Acme/Main", strings.TrimSpace(out), want)
	}
}

// --by client output keeps its shape: group is the client name and no client
// key is added.
func TestPatchPostureByClientShapeUnchanged(t *testing.T) {
	db := novelTestEnv(t)
	seedTwoMainSites(t, db)

	out, stderr, err := runNovel(t, "patch", "posture", "--by", "client")
	if err != nil {
		t.Fatalf("patch posture --by client: %v\n%s", err, stderr)
	}
	var raw []map[string]json.RawMessage
	assertJSON(t, out, &raw)
	if len(raw) != 2 {
		t.Fatalf("patch posture --by client = %s, want two rows", strings.TrimSpace(out))
	}
	for _, r := range raw {
		if len(r) != 4 || r["group"] == nil || r["agents"] == nil || r["patches_pending"] == nil || r["reboot_pending"] == nil {
			t.Errorf("--by client row %v, want exactly group, agents, patches_pending, reboot_pending", r)
		}
	}
}

// A cohort write never selects a site by its name alone: site=Main names a
// site in two clients, so maintenance set and agents bulk-run refuse it before
// any request (preview included) and accept it once client= picks the site.
// A site with no agents still counts (the synced site list), so the name
// cannot silently resolve to the other client's agents.
func TestCohortSiteFilterNeedsClientWhenTheNameRepeats(t *testing.T) {
	db := novelTestEnv(t)
	seedTwoMainSites(t, db)
	seedResource(t, db, "clients-sites", "4", `{"id":4,"name":"Lab","client":1,"client_name":"Acme"}`)
	seedResource(t, db, "clients-sites", "5", `{"id":5,"name":"Lab","client":2,"client_name":"Beta Corp"}`)
	seedResource(t, db, "agents", "a6", `{"agent_id":"a6","hostname":"HOST6","client_name":"Acme","site_name":"Lab","status":"online","plat":"windows"}`)
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

	for _, site := range []string{"site=Main", "site=Lab", "site=Main,os=windows"} {
		for _, base := range [][]string{
			{"maintenance", "set", "--execute"},
			{"maintenance", "set"},
			{"agents", "bulk-run", "--command", "whoami", "--execute"},
			{"agents", "bulk-run", "--command", "whoami"},
		} {
			args := append(append([]string{}, base...), "--filter", site)
			out, _, err := runNovel(t, args...)
			if err == nil {
				t.Errorf("%v: accepted (%s), want a usage error asking for client=", args, strings.TrimSpace(out))
			} else if !strings.Contains(err.Error(), "client=") {
				t.Errorf("%v: error %q does not ask for client=", args, err)
			}
		}
	}
	if writes != 0 {
		t.Fatalf("%d write requests sent for a site name held by two clients", writes)
	}

	if _, stderr, err := runNovel(t, "maintenance", "set", "--filter", "client=Acme,site=Main", "--execute"); err != nil {
		t.Fatalf("maintenance set --filter client=Acme,site=Main: %v\n%s", err, stderr)
	}
	if writes != 2 {
		t.Errorf("client=Acme,site=Main sent %d writes, want 2 (HOST1 and HOST2)", writes)
	}

	// A site name held by one client alone stays selectable by name.
	seedResource(t, db, "clients-sites", "6", `{"id":6,"name":"HQ","client":1,"client_name":"Acme"}`)
	seedResource(t, db, "agents", "a7", `{"agent_id":"a7","hostname":"HOST7","client_name":"Acme","site_name":"HQ","status":"online","plat":"windows"}`)
	if _, stderr, err := runNovel(t, "maintenance", "set", "--filter", "site=HQ", "--execute"); err != nil {
		t.Fatalf("maintenance set --filter site=HQ: %v\n%s", err, stderr)
	}
	if writes != 3 {
		t.Errorf("site=HQ sent %d writes in total, want 3 (HOST7 added)", writes)
	}
}
