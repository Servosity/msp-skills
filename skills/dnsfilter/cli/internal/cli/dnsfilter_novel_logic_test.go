// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Behavior tests for the hand-written DNSFilter novel-command logic.

package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func ja(t *testing.T, s string) jaRes {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("bad fixture %s: %v", s, err)
	}
	return toJARes(m)
}

func TestNormalizeDomainAndCovers(t *testing.T) {
	for in, want := range map[string]string{"HTTPS://Cdn.Example.com/x?y": "cdn.example.com", "example.com.": "example.com", "*.example.com": "example.com", "example.com:443": "example.com", " Example.COM ": "example.com"} {
		if got := normalizeDomain(in); got != want {
			t.Errorf("normalizeDomain(%q) = %q, want %q", in, got, want)
		}
	}
	cases := []struct {
		entry, fqdn string
		want        bool
	}{
		{"example.com", "example.com", true},
		{"example.com", "cdn.example.com", true},
		{"cdn.example.com", "example.com", false},
		{"example.com", "badexample.com", false}, // suffix without a dot boundary is not a parent
		{"", "example.com", false},
	}
	for _, c := range cases {
		if got := domainCovers(c.entry, c.fqdn); got != c.want {
			t.Errorf("domainCovers(%q, %q) = %v, want %v", c.entry, c.fqdn, got, c.want)
		}
	}
}

func TestScheduleSlot(t *testing.T) {
	cases := []struct {
		at   string
		want int
	}{
		{"2026-10-12T00:00:00Z", 0},   // Monday 00:00
		{"2026-10-12T00:14:59Z", 0},   // still the first 15-minute block
		{"2026-10-12T09:30:00Z", 38},  // Monday 09:30 -> 9*4+2
		{"2026-10-13T00:00:00Z", 96},  // Tuesday 00:00
		{"2026-10-18T23:59:00Z", 671}, // Sunday 23:59 -> last slot
	}
	for _, c := range cases {
		at, _ := time.Parse(time.RFC3339, c.at)
		if got := scheduleSlot(at, "UTC"); got != c.want {
			t.Errorf("scheduleSlot(%s) = %d, want %d", c.at, got, c.want)
		}
	}
	// Timezone shifts the slot: 2026-10-12 04:00 UTC is Monday 00:00 in New York (EDT).
	at, _ := time.Parse(time.RFC3339, "2026-10-12T04:00:00Z")
	if got := scheduleSlot(at, "America/New_York"); got != 0 {
		t.Errorf("scheduleSlot in America/New_York = %d, want 0", got)
	}
}

func TestCompareSemver(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"3.1.0", "3.1.0", 0},
		{"3.1.0", "3.10.0", -1},
		{"3.2", "3.1.9", 1},
		{"v2.3.9", "2.3.10", -1},
		{"3.1.0-beta", "3.1.0", -1}, // a prerelease sorts before its release
		{"3.1.0", "3.1.0-rc1", 1},
		{"3.1.0-alpha", "3.1.0-beta", -1},
		{"3.1.1-beta", "3.1.0", 1},
		{"3.1.0-beta.10", "3.1.0-beta.2", 1}, // numeric identifiers compare numerically
		{"1.0.0-alpha", "1.0.0-alpha.1", -1}, // shorter identifier set sorts first
		{"1.0.0-1", "1.0.0-alpha", -1},       // numeric identifiers sort below alphanumeric
		{"1.0.0-alpha.beta", "1.0.0-beta", -1},
		{"1.0.0+build.5", "1.0.0", 0},                                    // build metadata is ignored
		{"1.0.0-100000000000000000000", "1.0.0-99999999999999999999", 1}, // beyond int64
		{"100000000000000000000.0.0", "99999999999999999999.9.9", 1},
		{"3.01.0", "3.1.0", 0},
		{"", "1.0", -1},
	}
	for _, c := range cases {
		if got := compareSemver(c.a, c.b); got != c.want {
			t.Errorf("compareSemver(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestEvaluatePolicyPrecedence(t *testing.T) {
	social := []categoryRef{{ID: "7", Name: "Social Networking"}}
	policy := func(attrs string) jaRes { return ja(t, `{"id":"10","attributes":{"name":"Standard",`+attrs+`}}`) }
	cases := []struct {
		name     string
		fqdn     string
		p        jaRes
		cats     []categoryRef
		decision string
		rule     string
	}{
		{"allow list beats block list", "www.example.com", policy(`"whitelist_domains":["www.example.com"],"blacklist_domains":["example.com"]`), social, "allowed", "allow_list"},
		{"allow list beats allow-list-only", "www.example.com", policy(`"whitelist_domains":["www.example.com"],"allow_list_only":true`), social, "allowed", "allow_list"},
		{"allow list only blocks the rest", "example.net", policy(`"allow_list_only":true`), social, "blocked", "allow_list_only"},
		{"parent block decides alone -> unknown", "cdn.example.org", policy(`"blacklist_domains":["example.org"],"allow_unknown_domains":true`), nil, "unknown", "parent_entry_unverified"},
		{"parent allow vs blocked category -> unknown", "cdn.example.org", policy(`"whitelist_domains":["example.org"],"blacklist_categories":[7]`), social, "unknown", "parent_entry_unverified"},
		{"parent allow, nothing blocks -> allowed either way", "cdn.example.org", policy(`"whitelist_domains":["example.org"]`), social, "allowed", "no_matching_rule"},
		{"parent block + blocked category -> blocked either way", "cdn.example.org", policy(`"blacklist_domains":["example.org"],"blacklist_categories":[7]`), social, "blocked", "category"},
		{"parent allow cannot mask an exact block", "cdn.example.org", policy(`"whitelist_domains":["example.org"],"blacklist_domains":["cdn.example.org"]`), social, "unknown", "parent_entry_unverified"},
		// Only the "block parents apply, allow parents do not" combination blocks;
		// the old coupled check (all parents on / all off) said allowed both ways.
		{"broad parent allow vs narrower parent block", "child.evil.example.com", policy(`"whitelist_domains":["example.com"],"blacklist_domains":["evil.example.com"],"allow_unknown_domains":true`), nil, "unknown", "parent_entry_unverified"},
		{"uncategorized blocked", "example.net", policy(`"allow_unknown_domains":false`), nil, "blocked", "uncategorized"},
		{"uncategorized allowed", "example.net", policy(`"allow_unknown_domains":true`), nil, "allowed", "no_matching_rule"},
		{"blocked category", "example.net", policy(`"blacklist_categories":[7]`), social, "blocked", "category"},
		{"nothing matches", "example.net", policy(`"blacklist_categories":[9]`), social, "allowed", "no_matching_rule"},
		{"global policy is just a policy", "example.net", ja(t, `{"id":"1","attributes":{"name":"Global","is_global_policy":true,"blacklist_domains":["example.net"]}}`), social, "blocked", "block_list"},
	}
	unknownCats := []struct {
		name, fqdn, decision, rule string
		p                          jaRes
	}{
		{"list decision survives unknown categories", "www.example.com", "allowed", "allow_list", policy(`"whitelist_domains":["www.example.com"]`)},
		{"exact block survives unknown categories", "cdn.example.org", "blocked", "block_list", policy(`"blacklist_domains":["cdn.example.org"]`)},
		{"parent block with unknown categories is unknown", "cdn.example.org", "unknown", "parent_entry_unverified", policy(`"blacklist_domains":["example.org"]`)},
		{"category-dependent verdict is unknown", "example.net", "unknown", "categorization_unavailable", policy(`"allow_unknown_domains":false,"blacklist_categories":[7]`)},
	}
	for _, c := range unknownCats {
		got := evaluatePolicy(c.fqdn, c.p, nil, false)
		if got.Decision != c.decision || got.Rule != c.rule {
			t.Errorf("%s: got %s/%s, want %s/%s", c.name, got.Decision, got.Rule, c.decision, c.rule)
		}
	}
	for _, c := range cases {
		got := evaluatePolicy(c.fqdn, c.p, c.cats, true)
		if got.Decision != c.decision || got.Rule != c.rule {
			t.Errorf("%s: got %s/%s (%s), want %s/%s", c.name, got.Decision, got.Rule, got.Detail, c.decision, c.rule)
		}
	}
}

func TestScheduledPolicyNow(t *testing.T) {
	at, _ := time.Parse(time.RFC3339, "2026-10-12T09:30:00Z") // Monday slot 38 in UTC
	full := func(tz string, slot38 string) *jaRes {
		ids := make([]string, 672)
		for i := range ids {
			ids[i] = "10"
		}
		ids[38] = slot38
		raw, _ := json.Marshal(map[string]any{"id": "300", "attributes": map[string]any{"name": "Sched", "timezone": tz, "policy_ids": ids}})
		r := ja(t, string(raw))
		return &r
	}
	if id, _, ok := scheduledPolicyNow(full("UTC", "11"), at); !ok || id != "11" {
		t.Fatalf("valid schedule = %q, %v; want 11, true", id, ok)
	}
	short := ja(t, `{"id":"300","attributes":{"timezone":"UTC","policy_ids":["10","11"]}}`)
	for name, sp := range map[string]*jaRes{"nil schedule": nil, "too few slots": &short, "bad timezone": full("Mars/Olympus", "11"), "empty timezone": full("", "11"), "zero slot": full("UTC", "0"), "blank slot": full("UTC", "")} {
		if id, _, ok := scheduledPolicyNow(sp, at); ok {
			t.Errorf("%s resolved to %q; must be unresolved", name, id)
		}
	}
}

func TestIsUninstalled(t *testing.T) {
	for raw, want := range map[string]bool{
		`{"id":"1","attributes":{"status":"active"}}`:                  false,
		`{"id":"1","attributes":{"status":"uninstalled"}}`:             true,
		`{"id":"1","attributes":{"status":"uninstalled_from_client"}}`: true,
		`{"id":"1","attributes":{"agent_state":"uninstalled"}}`:        true,
		`{"id":"1","attributes":{"uninstalled_at":"2026-01-01"}}`:      true,
		`{"id":"1","attributes":{"uninstalled_at":null}}`:              false,
	} {
		if got := isUninstalled(ja(t, raw)); got != want {
			t.Errorf("isUninstalled(%s) = %v, want %v", raw, got, want)
		}
	}
}

func TestQueryLogShapeErrors(t *testing.T) {
	for _, raw := range []string{`{"data":{"values":[null]}}`, `{"data":{"values":[1]}}`, `{"data":null}`, `{"data":{"page":{}}}`, `{"data":{"values":"oops"}}`, `{"values":[]}`, `not json`} {
		if _, err := parseQueryLogHits([]byte(raw)); err == nil {
			t.Errorf("parseQueryLogHits(%s) = nil error; a malformed log must not read as zero hits", raw)
		}
	}
	if hits, err := parseQueryLogHits([]byte(`{"data":{"values":[]}}`)); err != nil || len(hits) != 0 {
		t.Errorf("empty values list = %v, %v; want zero hits, no error", hits, err)
	}
}

func TestDomainRecordCategories(t *testing.T) {
	cases := []struct {
		raw   string
		known bool
		ids   int
	}{
		{`{"id":"d1","attributes":{"host":"example.com"},"relationships":{"categories":{"data":[{"id":"7"}]}}}`, true, 1},
		{`{"id":"d1","attributes":{"host":"example.com"},"relationships":{"categories":{"data":[]}}}`, true, 0},
		{`{"error":"lookup unavailable"}`, false, 0},
		{`{"id":"d1","attributes":{"host":"example.com"}}`, false, 0},
		{`{"id":"d1","attributes":{"host":"example.com"},"relationships":{"categories":{"data":null}}}`, false, 0},
		{`{"id":"d1","attributes":{"host":"example.com"},"relationships":{"categories":{"data":[{"name":"no id"}]}}}`, false, 0},
	}
	for _, c := range cases {
		known, ids := domainRecordCategories(ja(t, c.raw))
		if known != c.known || len(ids) != c.ids {
			t.Errorf("domainRecordCategories(%s) = %v, %v; want %v with %d ids", c.raw, known, ids, c.known, c.ids)
		}
	}
}

func TestParseReleaseFeed(t *testing.T) {
	got, err := parseReleaseFeed([]byte(`{"user_agent_releases":[{"agent_type":"windows","agent_version":"3.1.0","release_channels":[{"value":"stable"}]}]}`))
	if err != nil || len(got) != 1 || got[0].str("agent_version") != "3.1.0" {
		t.Fatalf("parseReleaseFeed = %+v, %v", got, err)
	}
	for _, raw := range []string{`{"data":[]}`, `{"user_agent_releases":null}`, `{"user_agent_releases":[null]}`, `{"user_agent_releases":[{}]}`, `{"user_agent_releases":[{"agent_type":"windows","agent_version":"3.1.0"}]}`, `not json`} {
		if _, err := parseReleaseFeed([]byte(raw)); err == nil {
			t.Errorf("parseReleaseFeed(%s) = nil error; a missing feed must not read as no releases", raw)
		}
	}
	// A rollback is visible immediately: the feed, not a mirror, defines latest.
	agents, _, networks, orgs := driftFixtures(t)
	rolledBack := []jaRes{ja(t, `{"agent_type":"windows","agent_version":"3.0.4","release_channels":[{"value":"stable"}]}`)}
	if v := computeDrift(agents, rolledBack, networks, orgs, driftOptions{}); v.Latest["windows"] != "3.0.4" {
		t.Fatalf("latest after rollback = %q, want 3.0.4", v.Latest["windows"])
	}
}

func TestMatchHosts(t *testing.T) {
	agents := []jaRes{
		ja(t, `{"id":"a1","attributes":{"hostname":"FRONT-01","status":"active"}}`),
		ja(t, `{"id":"a2","attributes":{"hostname":"front-01","friendly_name":"Reception","status":"active"}}`),
		ja(t, `{"id":"a3","attributes":{"hostname":"FRONT-010","status":"active"}}`),
		ja(t, `{"id":"a4","attributes":{"hostname":"LAB-01","status":"uninstalled"}}`),
	}
	if got, err := matchHosts(agents, "front-01"); err == nil {
		t.Fatalf("two exact hostname matches must be ambiguous, got %d", len(got))
	}
	if got, err := matchHosts(agents, "a2"); err != nil || len(got) != 1 || got[0].ID != "a2" {
		t.Fatalf("id selector = %v, %v; want a2", got, err)
	}
	if got, err := matchHosts(agents, "reception"); err != nil || len(got) != 1 || got[0].ID != "a2" {
		t.Fatalf("unique friendly name = %v, %v; want a2", got, err)
	}
	if got, err := matchHosts(agents, "FRONT-010"); err != nil || len(got) != 1 || got[0].ID != "a3" {
		t.Fatalf("unique exact = %v, %v; want a3", got, err)
	}
	if got, err := matchHosts(agents, "LAB"); err != nil || len(got) != 0 {
		t.Fatalf("uninstalled-only match = %v, %v; want none", got, err)
	}
}

func TestMatchOrg(t *testing.T) {
	orgs := []jaRes{ja(t, `{"id":"1","attributes":{"name":"Acme Dental"}}`), ja(t, `{"id":"2","attributes":{"name":"Acme Dental West"}}`), ja(t, `{"id":"3","attributes":{"name":"Bolt Legal"}}`)}
	if o, err := matchOrg(orgs, "acme dental"); err != nil || o.ID != "1" {
		t.Fatalf("exact case-insensitive match = %v, %v; want id 1", o.ID, err)
	}
	if o, err := matchOrg(orgs, "bolt"); err != nil || o.ID != "3" {
		t.Fatalf("unique substring = %v, %v; want id 3", o.ID, err)
	}
	if _, err := matchOrg(orgs, "acme"); err == nil {
		t.Fatal("ambiguous substring must error, got nil")
	}
	dup := append(orgs, ja(t, `{"id":"9","attributes":{"name":"Bolt Legal"}}`))
	if _, err := matchOrg(dup, "bolt legal"); err == nil || !strings.Contains(err.Error(), "(9)") {
		t.Fatalf("duplicate exact org names must be ambiguous and list ids, got %v", err)
	}
	if o, err := matchOrg(dup, "9"); err != nil || o.ID != "9" {
		t.Fatalf("id selects one of the duplicates: %v, %v", o.ID, err)
	}
	if _, err := matchOrg(orgs, "zeta"); err == nil {
		t.Fatal("no match must error, got nil")
	}
}

func driftFixtures(t *testing.T) (agents, releases, networks, orgs []jaRes) {
	releases = []jaRes{
		ja(t, `{"agent_type":"windows","agent_version":"3.1.0","release_channels":[{"value":"stable"}]}`),
		ja(t, `{"agent_type":"windows","agent_version":"3.2.0","release_channels":[{"value":"beta"}]}`),
		ja(t, `{"agent_type":"macos","agent_version":"2.3.9","release_channels":[{"value":"stable"}]}`),
		ja(t, `{"agent_type":"windows","agent_version":"9.9.9","white_label":true,"release_channels":[{"value":"stable"}]}`),
	}
	networks = []jaRes{ja(t, `{"id":"50","attributes":{"name":"HQ"},"relationships":{"organization":{"data":{"id":"1"}}}}`), ja(t, `{"id":"51","attributes":{"name":"Office"},"relationships":{"organization":{"data":{"id":"2"}}}}`)}
	orgs = []jaRes{ja(t, `{"id":"1","attributes":{"name":"Acme Dental","user_agents_auto_update":false}}`), ja(t, `{"id":"2","attributes":{"name":"Bolt Legal","user_agents_auto_update":true}}`)}
	agents = []jaRes{
		ja(t, `{"id":"a1","attributes":{"hostname":"FRONT-01","agent_type":"windows","agent_version":"3.0.4","status":"active","last_sync":"2026-10-13"},"relationships":{"network":{"data":{"id":"50"}}}}`),
		ja(t, `{"id":"a2","attributes":{"hostname":"FRONT-02","agent_type":"windows","agent_version":"3.1.0","status":"active","last_sync":"2026-10-13"},"relationships":{"network":{"data":{"id":"50"}}}}`),
		ja(t, `{"id":"a3","attributes":{"hostname":"MAC-01","agent_type":"macos","agent_version":"2.3.9","status":"active","last_sync":"2026-10-01"},"relationships":{"network":{"data":{"id":"51"}}}}`),
		ja(t, `{"id":"a4","attributes":{"hostname":"OLD-01","agent_type":"windows","agent_version":"2.0.0","status":"uninstalled","uninstalled_at":"2026-01-01"},"relationships":{"network":{"data":{"id":"51"}}}}`),
		ja(t, `{"id":"a6","attributes":{"hostname":"GONE-02","agent_type":"windows","agent_version":"2.0.0","status":"uninstalled_from_client","last_sync":"2025-01-01"},"relationships":{"network":{"data":{"id":"51"}}}}`),
	}
	return
}

func TestComputeDrift(t *testing.T) {
	agents, releases, networks, orgs := driftFixtures(t)
	v := computeDrift(agents, releases, networks, orgs, driftOptions{})
	if v.Latest["windows"] != "3.1.0" || v.Latest["macos"] != "2.3.9" {
		t.Fatalf("latest stable = %v, want windows 3.1.0 macos 2.3.9", v.Latest)
	}
	if v.AgentsScanned != 3 {
		t.Fatalf("agents scanned = %d, want 3 (both uninstall statuses excluded)", v.AgentsScanned)
	}
	if len(v.Orgs) != 1 || v.Orgs[0].OrgName != "Acme Dental" || v.Orgs[0].BehindLatest != 1 || v.Orgs[0].SampleHosts[0] != "FRONT-01" {
		t.Fatalf("drift orgs = %+v, want only Acme Dental with FRONT-01 behind", v.Orgs)
	}
	if v.Orgs[0].AutoUpdate == nil || *v.Orgs[0].AutoUpdate {
		t.Fatalf("auto_update = %v, want false for Acme Dental", v.Orgs[0].AutoUpdate)
	}
	beta := computeDrift(agents, releases, networks, orgs, driftOptions{Channel: "beta"})
	if beta.Latest["windows"] != "3.2.0" || beta.ChannelSource != "flag" || beta.Orgs[0].BehindLatest != 2 {
		t.Fatalf("beta channel drift = %+v, want windows latest 3.2.0 and both Acme agents behind", beta)
	}
	noVer := append(append([]jaRes{}, agents...), ja(t, `{"id":"a9","attributes":{"hostname":"NOVER","agent_type":"windows","status":"active"},"relationships":{"network":{"data":{"id":"50"}}}}`))
	// Bolt gains a version-less client next to its versioned one (a mixed org);
	// onlyNoVer below covers an org made solely of version-less clients.
	noVer = append(noVer, ja(t, `{"id":"b9","attributes":{"hostname":"NOVER-BOLT","agent_type":"macos","status":"active"},"relationships":{"network":{"data":{"id":"51"}}}}`))
	nv := computeDrift(noVer, releases, networks, orgs, driftOptions{BadVersion: "0.0.0"})
	rows := map[string]driftOrgRow{}
	for _, o := range nv.Orgs {
		rows[o.OrgName] = o
	}
	acme, ok := rows["Acme Dental"]
	if !ok || acme.VersionUnknown != 1 || acme.OnBadVersion != 0 || acme.BehindLatest != 1 {
		t.Fatalf("Acme row = %+v (present=%v); version-less client must be counted apart, never compared", acme, ok)
	}
	if bolt, ok := rows["Bolt Legal"]; !ok || bolt.VersionUnknown != 1 || bolt.OnBadVersion != 0 {
		t.Fatalf("Bolt row = %+v (present=%v); an org with a version-less client must still be listed", bolt, ok)
	}
	onlyNoVer := computeDrift([]jaRes{ja(t, `{"id":"c1","attributes":{"hostname":"ONLY","agent_type":"windows","status":"active"},"relationships":{"network":{"data":{"id":"51"}}}}`)}, releases, networks, orgs, driftOptions{})
	if len(onlyNoVer.Orgs) != 1 || onlyNoVer.Orgs[0].VersionUnknown != 1 || onlyNoVer.Orgs[0].BehindLatest != 0 {
		t.Fatalf("org with only version-less clients = %+v; want one row with version_unknown=1", onlyNoVer.Orgs)
	}
	bad := computeDrift(agents, releases, networks, orgs, driftOptions{BadVersion: "3.1.0"})
	if bad.Orgs[0].OnBadVersion != 1 {
		t.Fatalf("on_bad_version = %d, want 1", bad.Orgs[0].OnBadVersion)
	}
	// Absence of drift: everyone current returns an empty list, not fabricated rows.
	current := computeDrift(agents[1:3], releases, networks, orgs, driftOptions{})
	if len(current.Orgs) != 0 {
		t.Fatalf("all-current fleet returned drift rows: %+v", current.Orgs)
	}
	// No releases in the feed: an honest note, and every org shows its
	// clients as unknown_latest instead of disappearing.
	none := computeDrift(agents, nil, networks, orgs, driftOptions{})
	unknown := 0
	for _, o := range none.Orgs {
		unknown += o.UnknownLatest
	}
	if none.Note == "" || unknown != 3 {
		t.Fatalf("no-release drift = %+v, want a note and 3 unknown_latest clients", none)
	}
}

func TestComputeStale(t *testing.T) {
	agents, _, networks, orgs := driftFixtures(t)
	agents = append(agents, ja(t, `{"id":"a5","attributes":{"hostname":"LAPTOP-9","agent_state":"bypassed","status":"active","last_sync":"2026-10-14"},"relationships":{"network":{"data":{"id":"51"}}}}`))
	now, _ := time.Parse(time.RFC3339, "2026-10-14T12:00:00Z")
	rows, scanned := computeStale(agents, networks, orgs, "", 7*24*time.Hour, now)
	if scanned == 0 {
		t.Fatalf("stale scan checked no clients")
	}
	if len(rows) != 1 || rows[0].OrgName != "Bolt Legal" || rows[0].Count != 2 {
		t.Fatalf("stale rows = %+v, want Bolt Legal with MAC-01 silent and LAPTOP-9 bypassed", rows)
	}
	reasons := map[string]string{}
	for _, a := range rows[0].Agents {
		reasons[a.Hostname] = a.Reason
	}
	if reasons["MAC-01"] != "silent" || reasons["LAPTOP-9"] != "bypassed" {
		t.Fatalf("reasons = %v", reasons)
	}
	if got, _ := computeStale(agents, networks, orgs, "acme", 7*24*time.Hour, now); len(got) != 0 {
		t.Fatalf("org filter acme returned %+v, want none (Acme agents are current)", got)
	}
}

func TestComputeListsWhere(t *testing.T) {
	agents, _, networks, orgs := driftFixtures(t)
	networks[0].Rels["policy"] = map[string]any{"data": map[string]any{"id": "10"}}
	agents[0].Rels["policy"] = map[string]any{"data": map[string]any{"id": "10"}}
	policies := []jaRes{
		ja(t, `{"id":"10","attributes":{"name":"Acme Std","organization_id":1,"whitelist_domains":["example.com"],"blacklist_domains":[]}}`),
		ja(t, `{"id":"11","attributes":{"name":"Bolt Std","organization_id":2,"whitelist_domains":[],"blacklist_domains":["cdn.example.com"]}}`),
		ja(t, `{"id":"12","attributes":{"name":"Other","organization_id":2,"whitelist_domains":["badexample.com"]}}`),
	}
	rows := computeListsWhere("cdn.example.com", policies, orgs, networks, agents)
	if len(rows) != 2 {
		t.Fatalf("rows = %+v, want 2 (parent allow + exact block)", rows)
	}
	// FRONT-01 is assigned policy 10; FRONT-02 has no assignment and inherits
	// site 50's policy 10, so both count.
	if rows[0].List != "allow" || rows[0].PolicyID != "10" || rows[0].OrgName != "Acme Dental" || rows[0].Sites != 1 || rows[0].RoamingClients != 2 {
		t.Fatalf("allow row = %+v", rows[0])
	}
	if rows[1].List != "block" || rows[1].PolicyID != "11" {
		t.Fatalf("block row = %+v", rows[1])
	}
	if got := computeListsWhere("example.org", policies, orgs, networks, agents); len(got) != 0 {
		t.Fatalf("unrelated domain matched: %+v", got)
	}
}

func TestAuditPolicies(t *testing.T) {
	cats := []jaRes{ja(t, `{"id":"60","attributes":{"name":"Phishing","security":true}}`), ja(t, `{"id":"7","attributes":{"name":"Social","security":false}}`)}
	policies := []jaRes{
		ja(t, `{"id":"10","attributes":{"name":"Loose","organization_id":1,"whitelist_domains":["bad.example.com"],"blacklist_categories":[],"allow_unknown_domains":true}}`),
		ja(t, `{"id":"11","attributes":{"name":"Tight","organization_id":2,"whitelist_domains":["ok.example.com"],"blacklist_categories":[60,7],"google_safesearch":true}}`),
	}
	domainCats := map[string][]string{"bad.example.com": {"60"}, "ok.example.com": {"7"}}
	baseline := policies[1]
	findings := auditPolicies(policies, cats, domainCats, &baseline)
	kinds := map[string]bool{}
	for _, f := range findings {
		kinds[f.PolicyID+":"+f.Kind] = true
	}
	listOnly := []jaRes{ja(t, `{"id":"31","attributes":{"name":"ListOnly","allow_list_only":true,"allow_unknown_domains":true,"blacklist_categories":[]}}`)}
	for _, f := range auditPolicies(listOnly, cats, nil, nil) {
		if f.Kind == "unknown_domains_allowed" || f.Kind == "security_category_not_blocked" {
			t.Errorf("allow-list-only policy got exposure finding %s", f.Kind)
		}
	}
	ext := []jaRes{ja(t, `{"id":"30","attributes":{"name":"Ext","blacklist_categories":[60,7],"google_safesearch":true,"allow_list_only":true,"youtube_restricted_level":"strict"}}`)}
	extFindings := auditPolicies(ext, cats, nil, &baseline)
	details := ""
	for _, f := range extFindings {
		details += f.Detail + ";"
	}
	if !strings.Contains(details, "allow_list_only=true") || !strings.Contains(details, "youtube_restricted_level=\"strict\"") {
		t.Errorf("baseline drift missed extended flags: %s", details)
	}
	for _, want := range []string{"10:threat_allowlisted", "10:security_category_not_blocked", "10:unknown_domains_allowed", "10:baseline_category_missing", "10:baseline_flag_differs"} {
		if !kinds[want] {
			t.Errorf("missing finding %s in %+v", want, findings)
		}
	}
	for k := range kinds {
		if k[:3] == "11:" {
			t.Errorf("clean baseline policy produced finding %s", k)
		}
	}
	if findings[0].Severity != "high" {
		t.Errorf("findings not sorted high first: %+v", findings[0])
	}
}

func TestGroupUnblockRequests(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-10-14T12:00:00Z")
	reqs := []jaRes{
		ja(t, `{"id":"u1","attributes":{"domain":"example.com","organization":{"id":1,"name":"Acme Dental"},"policy":{"id":10},"category":{"names":["Social"],"is_threat":false},"blocked_by":"Category","time_of_attempt":"2026-10-14T10:00:00Z"}}`),
		ja(t, `{"id":"u2","attributes":{"domain":"EXAMPLE.com","organization":{"id":2,"name":"Bolt Legal"},"policy":null,"category":{"is_threat":false},"blocked_by":"Category","time_of_attempt":"2026-10-14T08:00:00Z"}}`),
		ja(t, `{"id":"u3","attributes":{"domain":"evil.example.net","organization":{"id":1,"name":"Acme Dental"},"policy":{"id":10},"category":{"names":["Phishing"],"is_threat":true},"time_of_attempt":"2026-10-14T11:00:00Z"}}`),
		ja(t, `{"id":"u4","attributes":{"domain":"docs.example.org","organization":{"id":1,"name":"Acme Dental"},"policy":{"id":10},"category":{"is_threat":false}}}`),
		ja(t, `{"id":"u5","attributes":{"domain":"docs.example.org","organization":{"id":2,"name":"Bolt Legal"},"category":{"is_threat":false}}}`),
	}
	coverage := map[string]map[string]any{
		"u1": {"covered": true, "list": "policy"},
		// u2 (null policy) unchecked: the group must not be called already-allowed
		"u4": {"covered": true, "list": "policy"},
		"u5": {"covered": true, "list": "universal_org"},
	}
	groups := groupUnblockRequests(reqs, coverage, now)
	byDomain := map[string]triageGroup{}
	for _, g := range groups {
		byDomain[g.Domain] = g
	}
	if g := byDomain["example.com"]; g.Requests != 2 || len(g.Orgs) != 2 || g.AlreadyAllowed || g.Unchecked != 1 || g.Hint != "review" || g.AgeHours != 4 {
		t.Fatalf("example.com group = %+v, want 2 requests, 2 orgs, one unchecked -> review, age 4h", g)
	}
	if g := byDomain["evil.example.net"]; g.Hint != "deny" || !g.IsThreat {
		t.Fatalf("threat group = %+v, want deny", g)
	}
	if g := byDomain["docs.example.org"]; !g.AlreadyAllowed || g.Hint != "resolve-already-allowed" || len(g.CoveredBy) != 2 {
		t.Fatalf("already-allowed group = %+v (universal coverage must count)", g)
	}
	if groups[0].Requests != 2 {
		t.Fatalf("groups not sorted by request count: %+v", groups)
	}
	if empty := groupUnblockRequests(nil, nil, now); len(empty) != 0 {
		t.Fatalf("empty queue produced %+v", empty)
	}
}

func TestPlanAllowTargets(t *testing.T) {
	policies := map[string]jaRes{
		"10": ja(t, `{"id":"10","attributes":{"name":"Std","is_global_policy":false,"can_edit":true}}`),
		"11": ja(t, `{"id":"11","attributes":{"name":"Has it","is_global_policy":false,"can_edit":true,"whitelist_domains":["example.com"]}}`),
		"15": ja(t, `{"id":"15","attributes":{"name":"Parent only","is_global_policy":false,"can_edit":true,"whitelist_domains":["com"]}}`),
		"12": ja(t, `{"id":"12","attributes":{"name":"Global","is_global_policy":true,"can_edit":true}}`),
		"13": ja(t, `{"id":"13","attributes":{"name":"Locked","can_edit":false}}`),
		"14": ja(t, `{"id":"14","attributes":{"name":"No flag"}}`),
		"16": ja(t, `{"id":"16","attributes":{"name":"Global unknown","can_edit":true}}`),
	}
	deps := []deployment{{Kind: "site", Name: "HQ", PolicyID: "10"}, {Kind: "site", Name: "Annex", PolicyID: "10"}, {Kind: "site", Name: "Lab", PolicyID: "11"}, {Kind: "roaming_client", Name: "LAPTOP", PolicyID: "12"}, {Kind: "site", Name: "Vault", PolicyID: "13"}, {Kind: "site", Name: "Quiet", PolicyID: "14"}, {Kind: "site", Name: "Parent", PolicyID: "15"}, {Kind: "site", Name: "NoGlobalFlag", PolicyID: "16"}, {Kind: "site", Name: "Ghost", PolicyID: "99"}, {Kind: "site", Name: "Sched", PolicyID: ""}}
	got := map[string]allowTarget{}
	for _, tg := range planAllowTargets("example.com", policies, assignmentUse(deps), false) {
		got[tg.PolicyID] = tg
	}
	want := map[string]string{"10": "add", "11": "noop", "12": "skip", "13": "skip", "14": "skip", "15": "add", "16": "skip", "99": "skip"}
	for id, action := range want {
		if got[id].Action != action {
			t.Errorf("policy %s action = %s (%s), want %s", id, got[id].Action, got[id].Reason, action)
		}
	}
	if _, ok := got[""]; ok {
		t.Errorf("unresolved deployment produced a target")
	}
	if len(got["10"].UsedBy) != 2 {
		t.Errorf("policy 10 used_by = %v, want both sites", got["10"].UsedBy)
	}
	for _, tg := range planAllowTargets("example.com", policies, assignmentUse(deps), true) {
		if tg.PolicyID == "12" && tg.Action != "add" {
			t.Errorf("--include-global should add to global policy, got %s", tg.Action)
		}
	}
}

func TestDecodeJAShapes(t *testing.T) {
	for _, raw := range []string{`{"data":"oops"}`, `{"data":[1,2]}`, `{"data":[{"id":"1"},null]}`} {
		if _, err := decodeJA([]byte(raw)); err == nil {
			t.Errorf("decodeJA(%s) = nil error; malformed collections must not decode as partial lists", raw)
		}
	}
	for _, raw := range []string{`{"data":[{"id":"1","type":"x","attributes":{"name":"a"}}]}`, `{"data":{"id":1,"attributes":{"name":"a"}}}`, `[{"id":"1","name":"a"}]`} {
		got, err := decodeJA([]byte(raw))
		if err != nil || len(got) != 1 || got[0].ID != "1" || got[0].str("name") != "a" {
			t.Errorf("decodeJA(%s) = %+v, %v", raw, got, err)
		}
	}
	qp := []byte(`{"data":{"values":[{"time":"2026-10-14T10:00:00Z","result":"blocked","fqdn":"example.com","policy_name":"Std","method_name":"Category","categories_names":["Social"],"agentname":"FRONT-01"}]}}`)
	hits, err := parseQueryLogHits(qp)
	if err != nil {
		t.Fatalf("parseQueryLogHits: %v", err)
	}
	if len(hits) != 1 || hits[0].PolicyName != "Std" || hits[0].Source != "FRONT-01" || hits[0].Categories[0] != "Social" {
		t.Fatalf("parseQueryLogHits = %+v", hits)
	}
}

func TestStaleOrgFilterMatchingNothingScansZero(t *testing.T) {
	agents := []jaRes{ja(t, `{"id":"a1","attributes":{"hostname":"PC","status":"active","last_sync":"2020-01-01T00:00:00Z"},"relationships":{"network":{"data":{"id":"50"}}}}`)}
	networks := []jaRes{ja(t, `{"id":"50","relationships":{"organization":{"data":{"id":"1"}}}}`)}
	orgs := []jaRes{ja(t, `{"id":"1","attributes":{"name":"Acme Dental"}}`)}
	now := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	if rows, scanned := computeStale(agents, networks, orgs, "no-such-org", 7*24*time.Hour, now); scanned != 0 || len(rows) != 0 {
		t.Fatalf("rows=%v scanned=%d; a filter matching nothing must report zero scanned", rows, scanned)
	}
	if _, scanned := computeStale(agents, networks, orgs, "acme", 7*24*time.Hour, now); scanned != 1 {
		t.Fatalf("scanned=%d, want 1", scanned)
	}
}

func TestTriageOldestAttemptIsOrderIndependent(t *testing.T) {
	now := time.Date(2026, 10, 14, 20, 0, 0, 0, time.UTC)
	// 12:00Z is the oldest instant; 09:00-05:00 (14:00Z) sorts first as text and
	// 10:00+0000 is the colon-less offset form. "garbage" never parses.
	stamps := []string{"2026-10-14T09:00:00-05:00", "2026-10-14T13:00:00+0000", "2026-10-14T12:00:00Z", "garbage"}
	perms := [][]int{{0, 1, 2, 3}, {3, 2, 1, 0}, {1, 3, 0, 2}, {2, 0, 3, 1}}
	for _, p := range perms {
		reqs := make([]jaRes, 0, len(p))
		for i, k := range p {
			reqs = append(reqs, ja(t, fmt.Sprintf(`{"id":"r%d","attributes":{"domain":"example.com","time_of_attempt":%q}}`, i, stamps[k])))
		}
		g := groupUnblockRequests(reqs, nil, now)
		if len(g) != 1 || g[0].OldestAttempt != "2026-10-14T12:00:00Z" || g[0].AgeHours != 8 {
			t.Fatalf("order %v: oldest=%q age=%d; want 2026-10-14T12:00:00Z and 8h", p, g[0].OldestAttempt, g[0].AgeHours)
		}
	}
	// The colon-less offset form can win, and equal instants with different
	// offsets resolve the same way in either order.
	for _, pair := range [][2]string{
		{"2026-10-14T11:00:00+0000", "2026-10-14T12:00:00Z"},
		{"2026-10-14T12:00:00Z", "2026-10-14T11:00:00+0000"},
	} {
		reqs := []jaRes{
			ja(t, fmt.Sprintf(`{"id":"a","attributes":{"domain":"example.com","time_of_attempt":%q}}`, pair[0])),
			ja(t, fmt.Sprintf(`{"id":"b","attributes":{"domain":"example.com","time_of_attempt":%q}}`, pair[1])),
		}
		if g := groupUnblockRequests(reqs, nil, now); g[0].OldestAttempt != "2026-10-14T11:00:00+0000" || g[0].AgeHours != 9 {
			t.Fatalf("pair %v: oldest=%q age=%d; want the colon-less 11:00 and 9h", pair, g[0].OldestAttempt, g[0].AgeHours)
		}
	}
	for _, pair := range [][2]string{
		{"2026-10-14T12:00:00Z", "2026-10-14T13:00:00+01:00"},
		{"2026-10-14T13:00:00+01:00", "2026-10-14T12:00:00Z"},
	} {
		reqs := []jaRes{
			ja(t, fmt.Sprintf(`{"id":"a","attributes":{"domain":"example.com","time_of_attempt":%q}}`, pair[0])),
			ja(t, fmt.Sprintf(`{"id":"b","attributes":{"domain":"example.com","time_of_attempt":%q}}`, pair[1])),
		}
		if g := groupUnblockRequests(reqs, nil, now); g[0].OldestAttempt != "2026-10-14T12:00:00Z" {
			t.Fatalf("pair %v: equal instants resolved to %q; want the text-first 12:00:00Z in either order", pair, g[0].OldestAttempt)
		}
	}
}

func TestAttemptBeforeComparesInstants(t *testing.T) {
	// 09:00-05:00 is 14:00Z, later than 12:00Z, though it sorts first as text.
	if attemptBefore("2026-10-14T09:00:00-05:00", "2026-10-14T12:00:00Z") {
		t.Fatal("offset timestamps must compare by instant")
	}
	if !attemptBefore("2026-10-14T12:00:00Z", "2026-10-14T09:00:00-05:00") {
		t.Fatal("12:00Z is before 14:00Z")
	}
}
