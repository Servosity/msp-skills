// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func deletedSite(id, policy, deletedAt string) string {
	return `{"id":"` + id + `","attributes":{"name":"Closed Office","deleted_at":` + deletedAt + `},"relationships":{"organization":{"data":{"id":"1"}},"policy":{"data":{"id":"` + policy + `"}}}}`
}

// A live site that blocks nothing, plus a second site on a policy that blocks the
// domain. Deleted, the second is no reason the domain is blocked and no policy is
// written for it; live, the same answer is blocked, so the fixture does block.
func TestDeletedSiteIsNotADeployment(t *testing.T) {
	routes := func(deletedAt string) map[string]string {
		r := r18BlockRoutes()
		r["/v1/policies/all"] = r18List(r18Policy("10", nil), r18Policy("13", map[string]any{"name": "Closed", "allow_unknown_domains": false}))
		r["/v1/networks/all"] = r18List(r18Site("50", `"policy":{"data":{"id":"10"}}`), deletedSite("51", "13", deletedAt))
		return r
	}
	for _, deleted := range []string{`"2026-01-15"`, `"2026-01-15T10:00:00Z"`} {
		api := &r18API{routes: routes(deleted)}
		run := r17Runner(t, api.handler(t))
		if m := r18Decision(t, "deleted site", run); m["decision"] == "blocked" || m["blocked"] == true {
			t.Errorf("deleted_at %s: decision=%v blocked=%v, a deleted site filters nothing", deleted, m["decision"], m["blocked"])
		}
		_, _, _ = run("allow", "example.com", "--org", "1", "--note", "T-1", "--confirm", "--json")
		if len(api.writes) != 0 {
			t.Errorf("deleted_at %s: allow wrote %v for a policy only a deleted site uses", deleted, api.writes)
		}
	}
	run := r17Runner(t, (&r18API{routes: routes("null")}).handler(t))
	if m := r18Decision(t, "live site", run); m["decision"] != "blocked" {
		t.Errorf("the same site live: decision=%v, want blocked", m["decision"])
	}
}

func TestListsWhereSkipsDeletedSites(t *testing.T) {
	run := mirrorRunner(t, &fixtureAPI{})
	db := filepath.Join(t.TempDir(), "mirror.db")
	seedMirror(t, db, map[string][]string{
		"organizations":      {mirrorOrg},
		"policies":           {mirrorPolicy},
		"scheduled-policies": {},
		"user-agents":        {},
		"networks": {
			`{"id":"50","attributes":{"name":"HQ"},"relationships":{"organization":{"data":{"id":"1"}},"policy":{"data":{"id":"10"}}}}`,
			deletedSite("51", "10", `"2026-01-15T10:00:00Z"`),
		},
	}, nil)
	out, _ := run("lists", "where", "example.com", "--db", db, "--json")
	var answer struct {
		Results []listsWhereRow `json:"results"`
	}
	if err := json.Unmarshal([]byte(out), &answer); err != nil || len(answer.Results) == 0 {
		t.Fatalf("lists where: %v\n%s", err, out)
	}
	if answer.Results[0].Sites != 1 {
		t.Errorf("sites = %d, want 1: a deleted site is governed by nothing", answer.Results[0].Sites)
	}
}
