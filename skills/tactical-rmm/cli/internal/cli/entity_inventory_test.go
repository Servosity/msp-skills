// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (NOT generated): tactical-rmm handfix entity-inventory-from-own-list.

package cli

import "testing"

// Clients and sites are counted from their own synced lists (GET /clients/,
// GET /clients/sites/: vendor-src/clients/views.py GetAddClients.get,
// GetAddSites.get), not inferred from agents: a client or site with no agents
// still exists, and two sites with the same name in different clients are two.
func TestReportsCountClientsAndSitesFromTheirOwnLists(t *testing.T) {
	db := novelTestEnv(t)
	seedResource(t, db, "clients", "1", `{"id":1,"name":"Acme","agent_count":1}`)
	seedResource(t, db, "clients", "2", `{"id":2,"name":"Beta","agent_count":1}`)
	seedResource(t, db, "clients", "3", `{"id":3,"name":"Empty Co","agent_count":0}`)
	seedResource(t, db, "clients-sites", "10", `{"id":10,"name":"HQ","client":1,"client_name":"Acme"}`)
	seedResource(t, db, "clients-sites", "20", `{"id":20,"name":"HQ","client":2,"client_name":"Beta"}`)
	seedResource(t, db, "clients-sites", "30", `{"id":30,"name":"Main","client":3,"client_name":"Empty Co"}`)
	seedResource(t, db, "agents", "a1", `{"agent_id":"a1","client_name":"Acme","site_name":"HQ","status":"online"}`)
	seedResource(t, db, "agents", "b1", `{"agent_id":"b1","client_name":"Beta","site_name":"HQ","status":"online"}`)

	out, _, err := runNovel(t, "fleet", "health")
	if err != nil {
		t.Fatalf("fleet health: %v", err)
	}
	var fh struct {
		Clients int `json:"clients"`
		Sites   int `json:"sites"`
	}
	assertJSON(t, out, &fh)
	if fh.Clients != 3 || fh.Sites != 3 {
		t.Errorf("fleet health clients=%d sites=%d, want 3 and 3", fh.Clients, fh.Sites)
	}

	out, _, err = runNovel(t, "clients", "scorecard")
	if err != nil {
		t.Fatalf("clients scorecard: %v", err)
	}
	var rows []struct {
		Client     string `json:"client"`
		AgentCount int    `json:"agent_count"`
	}
	assertJSON(t, out, &rows)
	got := map[string]int{}
	for _, r := range rows {
		got[r.Client] = r.AgentCount
	}
	if len(got) != 3 || got["Empty Co"] != 0 || got["Acme"] != 1 || got["Beta"] != 1 {
		t.Errorf("clients scorecard = %v, want Acme 1, Beta 1 and Empty Co 0", got)
	}
	if _, ok := got["Empty Co"]; !ok {
		t.Error("a client with no agents is missing from the scorecard")
	}
}
