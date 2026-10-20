// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Codex review round 18 (and adversarial round 19): continuations, response
// identity, strict types and roaming clients. Unknown data never yields an
// all-clear, and a write never touches more than the operator asked for.
// Like the round-17 tests, these use only symbols that predate the fixes
// (JSON output is read as maps), so each one also runs, and fails, against
// the unfixed code.

package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"dnsfilter-pp-cli/internal/cliutil"
)

// r18API serves GET routes keyed by path, or by path plus one query value
// ("path?page[number]=2", "path?page[cursor]=c1", "path?fqdn=x"). A later
// page with no key of its own is an empty list. POSTs are recorded and
// answered {"data":{}}. A body starting with "!" is sent with HTTP 400.
type r18API struct {
	mu     sync.Mutex
	routes map[string]string
	writes []string
	gets   []string
}

func (a *r18API) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		defer a.mu.Unlock()
		serve := func(body string) {
			if strings.HasPrefix(body, "!") {
				w.WriteHeader(http.StatusBadRequest)
				body = body[1:]
			}
			fmt.Fprint(w, body)
		}
		if r.Method == http.MethodPost {
			a.writes = append(a.writes, r.URL.Path)
			fmt.Fprint(w, `{"data":{}}`)
			return
		}
		a.gets = append(a.gets, r.URL.Path+"?"+r.URL.RawQuery)
		q := r.URL.Query()
		for _, k := range []string{"page[cursor]", "page[number]", "fqdn"} {
			if v := q.Get(k); v != "" {
				if body, ok := a.routes[r.URL.Path+"?"+k+"="+v]; ok {
					serve(body)
					return
				}
			}
		}
		if p := q.Get("page[number]"); (p != "" && p != "1") || q.Get("page[cursor]") != "" {
			if r.URL.Path == "/v1/traffic_reports/query_logs" {
				fmt.Fprint(w, `{"data":{"values":[]}}`)
			} else {
				fmt.Fprint(w, `{"data":[]}`)
			}
			return
		}
		body, ok := a.routes[r.URL.Path]
		if !ok {
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"error":"not in fixture"}`)
			return
		}
		serve(body)
	})
}

func (a *r18API) writesCopy() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.writes...)
}

func (a *r18API) requested(prefix string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, g := range a.gets {
		if strings.HasPrefix(g, prefix) {
			return true
		}
	}
	return false
}

// r18Policy is a spec-complete policy (every attribute the novel commands
// read is present), owned by org 1, with set applied on top (nil is null).
func r18Policy(id string, set map[string]any) string {
	attrs := map[string]any{
		"name": "P" + id, "organization_id": 1, "is_global_policy": false, "can_edit": true,
		"whitelist_domains": []any{}, "blacklist_domains": []any{}, "blacklist_categories": []any{},
		"allow_list_only": false, "allow_unknown_domains": true,
		"google_safesearch": false, "bing_safe_search": false, "duck_duck_go_safe_search": false, "ecosia_safesearch": false,
		"yandex_safe_search": false, "youtube_restricted": false, "interstitial": false, "youtube_restricted_level": "none",
	}
	for k, v := range set {
		attrs[k] = v
	}
	raw, _ := json.Marshal(map[string]any{"id": id, "type": "policies", "attributes": attrs})
	return string(raw)
}

// r18Site is site id of org 1 with the given extra relationships JSON.
func r18Site(id, rels string) string {
	if rels != "" {
		rels = "," + rels
	}
	return `{"id":"` + id + `","attributes":{"name":"Site ` + id + `"},"relationships":{"organization":{"data":{"id":"1"}}` + rels + `}}`
}

// r18List is a JSON:API list page whose links.next is null (the last page).
func r18List(items ...string) string {
	return `{"data":[` + strings.Join(items, ",") + `],"links":{"self":"https://api.example/v1/x","next":null}}`
}

// r18BlockRoutes is org 1 with site 50 on policy 10 (which allows
// everything), an empty query log and an uncategorized example.com.
func r18BlockRoutes() map[string]string {
	return map[string]string{
		"/v1/organizations/1":            `{"data":{"id":"1","type":"organizations","attributes":{"name":"Acme Dental"}}}`,
		"/v1/policies/all":               r18List(r18Policy("10", nil)),
		"/v1/networks/all":               r18List(r18Site("50", `"policy":{"data":{"id":"10"}}`)),
		"/v1/user_agents/all":            r18List(),
		"/v1/domains/user_lookup":        fixtureLookupOK,
		"/v1/traffic_reports/query_logs": `{"data":{"values":[]}}`,
	}
}

func r18Decision(t *testing.T, label string, run func(...string) (string, string, error), args ...string) map[string]any {
	t.Helper()
	out, errOut, err := run(append([]string{"why-blocked", "example.com", "--org", "1", "--json"}, args...)...)
	if err != nil {
		t.Fatalf("%s: why-blocked: %v\n%s%s", label, err, out, errOut)
	}
	return r17JSON(t, label, out)
}

// Finding 1: without --host only sites were evaluated, so a roaming client
// whose own policy blocks the domain left decision not_blocked.
func TestR18WhyBlockedOrgModeEvaluatesRoamingClients(t *testing.T) {
	routes := r18BlockRoutes()
	routes["/v1/policies/all"] = r18List(r18Policy("10", nil), r18Policy("13", map[string]any{"name": "Lab", "allow_unknown_domains": false}))
	routes["/v1/user_agents/all"] = r18List(
		`{"id":"a8","attributes":{"hostname":"LAB-01","status":"active"},"relationships":{"network":{"data":{"id":"50"}},"policy":{"data":{"id":"13"}}}}`,
		`{"id":"a9","attributes":{"hostname":"PC-09","status":"active"},"relationships":{"network":{"data":{"id":"50"}},"policy":{"data":null},"scheduled_policy":{"data":null}}}`)
	run := r17Runner(t, (&r18API{routes: routes}).handler(t))
	if m := r18Decision(t, "own blocking policy", run); m["decision"] != "blocked" || m["blocked"] != true {
		t.Fatalf("a roaming client whose own policy blocks the domain: decision=%v blocked=%v, want blocked/true", m["decision"], m["blocked"])
	}
	// A roaming-client scan that fails or is capped is a gap, never an all-clear.
	for name, body := range map[string]string{
		"listing fails": `!{"error":"forbidden"}`,
		"scan capped":   `{"data":[` + strings.TrimSuffix(strings.Repeat(`{"id":"x","attributes":{"hostname":"X","status":"active"},"relationships":{"network":{"data":{"id":"50"}}}},`, dnsfilterPageSize), ",") + `]}`,
	} {
		routes := r18BlockRoutes()
		routes["/v1/user_agents/all"] = body
		run := r17Runner(t, (&r18API{routes: routes}).handler(t))
		if m := r18Decision(t, name, run, "--max-scan-pages", "1"); m["decision"] != "unknown" || m["blocked"] != nil {
			t.Fatalf("%s: decision=%v blocked=%v, want unknown/null", name, m["decision"], m["blocked"])
		}
	}
}

// Finding 1 (allow): the assignment fallback wrote only site policies, so a
// roaming client's own blocking policy kept the domain blocked.
func TestR18AllowAssignmentIncludesRoamingClientPolicies(t *testing.T) {
	routes := r18BlockRoutes()
	routes["/v1/policies/all"] = r18List(r18Policy("10", nil), r18Policy("13", map[string]any{"name": "Lab", "allow_unknown_domains": false}))
	routes["/v1/user_agents/all"] = r18List(`{"id":"a8","attributes":{"hostname":"LAB-01","status":"active"},"relationships":{"network":{"data":{"id":"50"}},"policy":{"data":{"id":"13"}}}}`)
	routes["/v1/policies/13"] = r18Policy("13", map[string]any{"name": "Lab", "allow_unknown_domains": false, "whitelist_domains": []any{"example.com"}})
	api := &r18API{routes: routes}
	run := r17Runner(t, api.handler(t))
	out, errOut, err := run("allow", "example.com", "--org", "1", "--note", "T", "--confirm", "--json")
	if err != nil {
		t.Fatalf("allow: %v\n%s%s", err, out, errOut)
	}
	if w := api.writesCopy(); strings.Join(w, ",") != "/v1/policies/13/add_whitelist_domain" {
		t.Fatalf("writes = %v; the roaming client's own blocking policy 13 (and only it) must be written\n%s", w, out)
	}
}

// Finding 2: a short page that names a next page ended the walk.
func TestR18FetchPagesFollowsContinuation(t *testing.T) {
	next := func(page int, items ...string) string {
		return `{"data":[` + strings.Join(items, ",") + `],"links":{"next":"https://api.example/v1/policies/all?page%5Bnumber%5D=` + fmt.Sprint(page) + `&page%5Bsize%5D=100"}}`
	}
	compliant := map[string]any{"allow_unknown_domains": false, "blacklist_categories": []any{60}}
	t.Run("policy-audit reads page 2", func(t *testing.T) {
		run := r17Runner(t, (&r18API{routes: map[string]string{
			"/v1/categories/all":              r18List(`{"id":"60","attributes":{"name":"Malware","security":true}}`),
			"/v1/policies/all":                next(2, r18Policy("10", compliant)),
			"/v1/policies/all?page[number]=2": r18List(r18Policy("11", map[string]any{"allow_unknown_domains": true, "blacklist_categories": []any{}})),
		}}).handler(t))
		out, _, err := run("policy-audit", "--json")
		if err != nil {
			t.Fatalf("policy-audit: %v", err)
		}
		if m := r17JSON(t, "policy-audit", out); m["policies_scanned"] != float64(2) || len(m["findings"].([]any)) == 0 {
			t.Fatalf("page 2 (an unsafe policy) was never read:\n%s", out)
		}
	})
	t.Run("a plain page parameter in links.next is followed", func(t *testing.T) {
		run := r17Runner(t, (&r18API{routes: map[string]string{
			"/v1/categories/all":              r18List(`{"id":"60","attributes":{"name":"Malware","security":true}}`),
			"/v1/policies/all":                `{"data":[` + r18Policy("10", compliant) + `],"links":{"next":"/v1/policies/all?page=2"}}`,
			"/v1/policies/all?page[number]=2": r18List(r18Policy("11", map[string]any{"allow_unknown_domains": true, "blacklist_categories": []any{}})),
		}}).handler(t))
		out, _, err := run("policy-audit", "--json")
		if err != nil {
			t.Fatalf("policy-audit: %v", err)
		}
		if m := r17JSON(t, "policy-audit", out); m["policies_scanned"] != float64(2) {
			t.Fatalf("links.next ?page=2 was not followed:\n%s", out)
		}
	})
	t.Run("why-blocked reads site page 2", func(t *testing.T) {
		routes := r18BlockRoutes()
		routes["/v1/policies/all"] = r18List(r18Policy("10", nil), r18Policy("12", map[string]any{"blacklist_domains": []any{"example.com"}}))
		routes["/v1/networks/all"] = strings.ReplaceAll(next(2, r18Site("50", `"policy":{"data":{"id":"10"}}`)), "policies/all", "networks/all")
		routes["/v1/networks/all?page[number]=2"] = r18List(r18Site("51", `"policy":{"data":{"id":"12"}}`))
		run := r17Runner(t, (&r18API{routes: routes}).handler(t))
		if m := r18Decision(t, "site page 2", run); m["decision"] != "blocked" {
			t.Fatalf("the blocking site on page 2 was never evaluated: decision=%v", m["decision"])
		}
	})
	t.Run("unblocks triage reads meta.total_pages", func(t *testing.T) {
		run := r17Runner(t, (&r18API{routes: map[string]string{
			"/v2/unblock_requests":                `{"data":[{"id":"u1","attributes":{"domain":"a.example","category":{"is_threat":false}}}],"meta":{"current_page":1,"total_pages":2,"total_count":2}}`,
			"/v2/unblock_requests?page[number]=2": `{"data":[{"id":"u2","attributes":{"domain":"evil.example","category":{"is_threat":true}}}],"meta":{"current_page":2,"total_pages":2,"total_count":2}}`,
		}}).handler(t))
		out, _, err := run("unblocks", "triage", "--max-coverage-checks", "0", "--json")
		if err != nil {
			t.Fatalf("triage: %v", err)
		}
		if m := r17JSON(t, "triage", out); m["scanned_requests"] != float64(2) {
			t.Fatalf("page 2 of the queue was never read:\n%s", out)
		}
	})
	t.Run("an unfollowable continuation is an error", func(t *testing.T) {
		for name, page1 := range map[string]string{
			"opaque cursor":     `{"data":[` + r18Policy("10", compliant) + `],"links":{"next":"https://api.example/v1/policies/all?page%5Bcursor%5D=abc"}}`,
			"skips a page":      next(3, r18Policy("10", compliant)),
			"link with no page": `{"data":[` + r18Policy("10", compliant) + `],"links":{"next":"https://api.example/v1/policies/all?sort=name"}}`,
		} {
			run := r17Runner(t, (&r18API{routes: map[string]string{
				"/v1/categories/all": r18List(`{"id":"60","attributes":{"name":"Malware","security":true}}`),
				"/v1/policies/all":   page1,
			}}).handler(t))
			if out, _, err := run("policy-audit", "--json"); err == nil {
				t.Fatalf("%s: an unfollowable continuation produced an audit:\n%s", name, out)
			}
		}
	})
}

// Finding 2 (query log): a short query-log page with a continuation (cursor
// or next page) was certified the end of the log.
func TestR18QueryLogFollowsContinuation(t *testing.T) {
	sub := `{"fqdn":"sub.example.com","result":"blocked","policy_id":20}`
	exact := `{"data":{"values":[{"fqdn":"example.com","result":"blocked","policy_id":20}]}}`
	for name, c := range map[string]struct{ page1, key string }{
		"cursor":     {`{"data":{"values":[` + sub + `],"page":{"cursor":"2026-10-20T00:00:00.000Z","cursor_offset":1}}}`, "/v1/traffic_reports/query_logs?page[cursor]=2026-10-20T00:00:00.000Z"},
		"next page":  {`{"data":{"values":[` + sub + `],"page":{"self":1,"next":2}}}`, "/v1/traffic_reports/query_logs?page[number]=2"},
		"links.next": {`{"data":{"values":[` + sub + `]},"links":{"next":"/v1/traffic_reports/query_logs?page%5Bnumber%5D=2"}}`, "/v1/traffic_reports/query_logs?page[number]=2"},
	} {
		t.Run(name, func(t *testing.T) {
			routes := r18BlockRoutes()
			routes["/v1/traffic_reports/query_logs"] = c.page1
			routes[c.key] = exact
			run := r17Runner(t, (&r18API{routes: routes}).handler(t))
			if m := r18Decision(t, name, run); m["decision"] != "blocked" || m["blocked"] != true {
				t.Fatalf("the exact block on the next query-log page was never read: decision=%v blocked=%v", m["decision"], m["blocked"])
			}
		})
	}
}

// Finding 3: a query-log entry with an unreadable fqdn was dropped as a
// non-match and the log certified complete.
func TestR18QueryLogUnreadableFQDNNotComplete(t *testing.T) {
	for name, entry := range map[string]string{
		"fqdn null":    `{"fqdn":null,"result":"blocked","policy_id":"20"}`,
		"fqdn missing": `{"result":"blocked","policy_id":"77"}`,
		"fqdn numeric": `{"fqdn":4821,"result":"blocked","policy_id":"20"}`,
	} {
		routes := r18BlockRoutes()
		routes["/v1/traffic_reports/query_logs"] = `{"data":{"values":[` + entry + `]}}`
		run := r17Runner(t, (&r18API{routes: routes}).handler(t))
		if m := r18Decision(t, name, run); m["decision"] != "unknown" || m["blocked"] != nil {
			t.Fatalf("%s: decision=%v blocked=%v; an entry that cannot be ruled out must leave the decision unknown", name, m["decision"], m["blocked"])
		}
	}
}

// Finding 4a: a release whose release_channels holds an unreadable entry was
// read as outside the channel.
func TestR18DriftUnreadableReleaseChannel(t *testing.T) {
	_, _, networks, orgs := driftFixtures(t)
	agents := []jaRes{ja(t, `{"id":"a1","attributes":{"hostname":"PC","agent_type":"windows","agent_version":"3.2.0","status":"active"},"relationships":{"network":{"data":{"id":"50"}}}}`)}
	stable := ja(t, `{"agent_type":"windows","agent_version":"3.2.0","white_label":false,"release_channels":[{"value":"stable"}]}`)
	for name, rel := range map[string]string{
		"null entry":          `{"agent_type":"windows","agent_version":"9.0.0","white_label":false,"release_channels":[null]}`,
		"entry with no value": `{"agent_type":"windows","agent_version":"9.0.0","white_label":false,"release_channels":[{"label":"Stable"}]}`,
	} {
		v := computeDrift(agents, []jaRes{stable, ja(t, rel)}, networks, orgs, driftOptions{})
		if v.Latest["windows"] != "" || len(v.Orgs) != 1 || v.Orgs[0].UnknownLatest != 1 {
			t.Errorf("%s: latest=%q orgs=%+v; a newer release whose channel is unknown makes latest unknown", name, v.Latest["windows"], v.Orgs)
		}
	}
	// An older release with an unreadable channel cannot be the latest.
	v := computeDrift(agents, []jaRes{stable, ja(t, `{"agent_type":"windows","agent_version":"1.0.0","white_label":false,"release_channels":[null]}`)}, networks, orgs, driftOptions{})
	if v.Latest["windows"] != "3.2.0" || len(v.Orgs) != 0 {
		t.Errorf("an older unreadable-channel release: latest=%q orgs=%+v, want 3.2.0 and an all-clear", v.Latest["windows"], v.Orgs)
	}
}

// Finding 4b: --agent-type dropped a client whose agent_type is unknown.
func TestR18DriftAgentTypeUnknownReported(t *testing.T) {
	_, _, networks, orgs := driftFixtures(t)
	releases := []jaRes{ja(t, `{"agent_type":"windows","agent_version":"3.2.0","release_channels":[{"value":"stable"}]}`)}
	agents := []jaRes{
		ja(t, `{"id":"a1","attributes":{"hostname":"CURRENT","agent_type":"windows","agent_version":"3.2.0","status":"active"},"relationships":{"network":{"data":{"id":"50"}}}}`),
		ja(t, `{"id":"a2","attributes":{"hostname":"NOTYPE","agent_type":null,"agent_version":"1.0.0","status":"active"},"relationships":{"network":{"data":{"id":"50"}}}}`),
	}
	raw, _ := json.Marshal(computeDrift(agents, releases, networks, orgs, driftOptions{AgentType: "windows"}))
	var m struct {
		Orgs []map[string]any `json:"orgs"`
	}
	_ = json.Unmarshal(raw, &m)
	if len(m.Orgs) != 1 || m.Orgs[0]["type_unknown"] != float64(1) {
		t.Fatalf("--agent-type windows dropped a client whose agent_type is null: %s", raw)
	}
}

// Finding 4c: a numeric agent_version (999) was coerced to "999" and read as current.
func TestR18DriftNumericAgentVersionUnknown(t *testing.T) {
	_, _, networks, orgs := driftFixtures(t)
	releases := []jaRes{ja(t, `{"agent_type":"windows","agent_version":"3.2.0","release_channels":[{"value":"stable"}]}`)}
	agents := []jaRes{ja(t, `{"id":"a1","attributes":{"hostname":"ODD","agent_type":"windows","agent_version":999,"status":"active"},"relationships":{"network":{"data":{"id":"50"}}}}`)}
	if v := computeDrift(agents, releases, networks, orgs, driftOptions{}); len(v.Orgs) != 1 || v.Orgs[0].VersionUnknown != 1 {
		t.Fatalf("agent_version 999 (a number) read as a version: orgs=%+v", v.Orgs)
	}
}

// Finding 5: any non-empty uninstalled_at ("not-a-date", 0) excluded a client.
func TestR18UnreadableUninstalledAtNotExcluded(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-10-14T12:00:00Z")
	networks := []jaRes{ja(t, `{"id":"50","relationships":{"organization":{"data":{"id":"1"}},"policy":{"data":{"id":"10"}}}}`)}
	orgs := []jaRes{ja(t, `{"id":"1","attributes":{"name":"Acme Dental"}}`)}
	healthy := ja(t, `{"id":"ok","attributes":{"hostname":"OK","agent_type":"windows","agent_version":"3.2.0","agent_state":"protected","status":"active","last_sync":"2026-10-14T11:00:00Z"},"relationships":{"network":{"data":{"id":"50"}}}}`)
	for _, at := range []string{`"not-a-date"`, `0`, `true`} {
		odd := ja(t, `{"id":"odd","attributes":{"hostname":"ODD","agent_type":"windows","agent_version":"1.0.0","agent_state":"unprotected","status":"active","last_sync":"2026-10-14T11:00:00Z","uninstalled_at":`+at+`},"relationships":{"network":{"data":{"id":"50"}}}}`)
		if isUninstalled(odd) {
			t.Errorf("uninstalled_at %s read as uninstalled", at)
		}
		if rows, _ := computeStale([]jaRes{healthy, odd}, networks, orgs, "", 7*24*time.Hour, now); len(rows) != 1 || rows[0].Agents[0].ID != "odd" {
			t.Errorf("uninstalled_at %s: stale rows %+v; the unprotected client must be reported, not excluded", at, rows)
		}
		releases := []jaRes{ja(t, `{"agent_type":"windows","agent_version":"3.2.0","release_channels":[{"value":"stable"}]}`)}
		if v := computeDrift([]jaRes{healthy, odd}, releases, networks, orgs, driftOptions{}); len(v.Orgs) != 1 || v.Orgs[0].BehindLatest != 1 {
			t.Errorf("uninstalled_at %s: drift orgs %+v; the outdated client must be counted", at, v.Orgs)
		}
		policies := []jaRes{ja(t, `{"id":"10","attributes":{"name":"Std","whitelist_domains":["example.com"],"blacklist_domains":[]}}`)}
		if rows, _ := computeListsWhere("example.com", policies, orgs, networks, []jaRes{healthy, odd}, nil); len(rows) != 1 || rows[0].RoamingClients != 2 {
			t.Errorf("uninstalled_at %s: lists where rows %+v; the client's impact must be counted", at, rows)
		}
	}
	// A parseable timestamp still means uninstalled.
	if !isUninstalled(ja(t, `{"id":"1","attributes":{"uninstalled_at":"2026-01-01T10:00:00Z"}}`)) {
		t.Error("a parseable uninstalled_at must still mean uninstalled")
	}
}

// Finding 6a: a policy named by an historical query-log hit was written
// after it moved to another organization.
func TestR18AllowQueryLogTargetMustBelongToOrg(t *testing.T) {
	for _, c := range []struct {
		org   any
		write bool
	}{{2, false}, {nil, false}, {1, true}} {
		routes := r18BlockRoutes()
		routes["/v1/traffic_reports/query_logs"] = `{"data":{"values":[{"fqdn":"example.com","result":"blocked","policy_id":12}]}}`
		routes["/v1/policies/12"] = r18Policy("12", map[string]any{"name": "Moved", "organization_id": c.org})
		api := &r18API{routes: routes}
		run := r17Runner(t, api.handler(t))
		out, errOut, _ := run("allow", "example.com", "--org", "1", "--note", "T", "--confirm", "--json")
		if w := api.writesCopy(); (len(w) > 0) != c.write {
			t.Fatalf("policy 12 with organization_id %v: writes=%v, want write=%v\n%s%s", c.org, w, c.write, out, errOut)
		}
	}
}

// Finding 6b: /v1/organizations/<id> answering for another org was accepted.
func TestR18ResolveOrgVerifiesIdentity(t *testing.T) {
	routes := r18BlockRoutes()
	routes["/v1/organizations/1"] = `{"data":{"id":"2","attributes":{"name":"Other client"}}}`
	api := &r18API{routes: routes}
	run := r17Runner(t, api.handler(t))
	out, _, err := run("why-blocked", "example.com", "--org", "1", "--json")
	if err == nil || api.requested("/v1/policies/all") {
		t.Fatalf("an organization response naming org 2 scoped --org 1: err=%v\n%s", err, out)
	}
	if _, _, err := run("allow", "example.com", "--org", "1", "--note", "T", "--confirm"); err == nil || len(api.writesCopy()) != 0 {
		t.Fatalf("allow resolved --org 1 to another organization: err=%v writes=%v", err, api.writesCopy())
	}
}

// Finding 7: a relationship whose data is present but whose id is
// unreadable read as "no assignment", enabling a fallback.
func TestR18UnreadableRelationshipIsUnknown(t *testing.T) {
	for name, rel := range map[string]string{
		"id null":         `"scheduled_policy":{"data":{"id":null}}`,
		"id missing":      `"scheduled_policy":{"data":{}}`,
		"data not object": `"scheduled_policy":{"data":"300"}`,
		"no data member":  `"scheduled_policy":{"links":{"related":"/v1/scheduled_policies/300"}}`,
		"fractional id":   `"scheduled_policy":{"data":{"id":300.5}}`,
	} {
		routes := r18BlockRoutes()
		routes["/v1/networks/all"] = r18List(r18Site("50", `"policy":{"data":{"id":"10"}},`+rel))
		run := r17Runner(t, (&r18API{routes: routes}).handler(t))
		if m := r18Decision(t, name, run); m["decision"] != "unknown" || m["blocked"] != nil {
			t.Fatalf("site schedule %s: decision=%v; the effective policy is unknown, never the retained direct policy", name, m["decision"])
		}
	}
	t.Run("roaming client never falls back to its site", func(t *testing.T) {
		routes := r18BlockRoutes()
		routes["/v1/user_agents/a8"] = `{"data":{"id":"a8","attributes":{"hostname":"LAB-01","status":"active"},"relationships":{"network":{"data":{"id":"50"}},"policy":{"data":{"id":null}}}}}`
		run := r17Runner(t, (&r18API{routes: routes}).handler(t))
		if m := r18Decision(t, "host", run, "--host", "a8"); m["decision"] != "unknown" {
			t.Fatalf("--host with an unreadable own policy fell back to the site: decision=%v", m["decision"])
		}
	})
	t.Run("allow never writes the retained direct policy", func(t *testing.T) {
		routes := r18BlockRoutes()
		routes["/v1/policies/all"] = r18List(r18Policy("10", map[string]any{"blacklist_domains": []any{"example.com"}}))
		routes["/v1/networks/all"] = r18List(r18Site("50", `"policy":{"data":{"id":"10"}},"scheduled_policy":{"data":{"id":null}}`))
		api := &r18API{routes: routes}
		run := r17Runner(t, api.handler(t))
		_, _, _ = run("allow", "example.com", "--org", "1", "--note", "T", "--confirm", "--json")
		if w := api.writesCopy(); len(w) != 0 {
			t.Fatalf("writes = %v; a site whose schedule link is unreadable must not have its direct policy written", w)
		}
	})
	t.Run("lists where withholds the all-clear", func(t *testing.T) {
		run := mirrorRunner(t, &fixtureAPI{})
		db := filepath.Join(t.TempDir(), "mirror.db")
		seedMirror(t, db, map[string][]string{
			"organizations": {mirrorOrg}, "policies": {mirrorPolicy}, "user-agents": {mirrorAgent()},
			"networks": {`{"id":"50","attributes":{"name":"HQ"},"relationships":{"organization":{"data":{"id":"1"}},"policy":{"data":{"id":"10"}},"scheduled_policy":{"data":{"id":null}}}}`},
		}, map[string]string{"organizations": "complete", "networks": "complete", "user-agents": "complete", "policies": "complete", "scheduled-policies": "complete"})
		if out, _ := run("lists", "where", "nothing.test", "--db", db, "--json"); out == "[]\n" || !strings.Contains(out, "not an all-clear") {
			t.Fatalf("an unreadable schedule link left a clean all-clear:\n%s", out)
		}
	})
}

// exportBackup writes previous into a fresh backup file and returns its path.
func exportBackup(t *testing.T, previous string) (dir, path string) {
	t.Helper()
	dir = t.TempDir()
	path = filepath.Join(dir, "backup.jsonl")
	if err := os.WriteFile(path, []byte(previous), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir, path
}

func assertBackupUntouched(t *testing.T, label, dir, path, previous string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || string(got) != previous {
		t.Fatalf("%s: the previous backup changed: %q (%v)", label, got, err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Fatalf("%s: files left behind: %v", label, entries)
	}
}

// Finding 8a: {"data":null} was exported as one record over a good backup.
func TestR18ExportRefusesUnreadableCollection(t *testing.T) {
	for name, body := range map[string]string{"data null": `{"data":null}`, "no data key": `{"links":{"self":"x"}}`, "data a string": `{"data":"oops"}`} {
		run := r17Runner(t, (&r18API{routes: map[string]string{"/v1/networks/all": body}}).handler(t))
		dir, path := exportBackup(t, "GOOD-BACKUP\n")
		if _, _, err := run("export", "networks", "--output", path, "--no-cache"); err == nil {
			t.Fatalf("%s: export succeeded over an unreadable collection", name)
		}
		assertBackupUntouched(t, name, dir, path, "GOOD-BACKUP\n")
	}
	run := r17Runner(t, (&r18API{routes: map[string]string{"/v1/policies/all": `{"data":null}`}}).handler(t))
	if out, _, err := run("tail", "policies", "--follow=false"); err == nil {
		t.Fatalf("tail emitted an unreadable collection as data:\n%s", out)
	}
}

// Finding 8b: export stopped on a short page that named a next page.
func TestR18ExportFollowsContinuation(t *testing.T) {
	run := r17Runner(t, (&r18API{routes: map[string]string{
		"/v1/networks/all":                `{"data":[{"id":"1","attributes":{"name":"A"}}],"links":{"next":"https://api.example/v1/networks/all?page%5Bnumber%5D=2&page%5Bsize%5D=100"}}`,
		"/v1/networks/all?page[number]=2": `{"data":[{"id":"2","attributes":{"name":"B"}}],"links":{"next":null}}`,
	}}).handler(t))
	_, path := exportBackup(t, "OLD\n")
	if _, errOut, err := run("export", "networks", "--output", path, "--no-cache"); err != nil {
		t.Fatalf("export: %v\n%s", err, errOut)
	}
	got, _ := os.ReadFile(path)
	if lines := strings.Count(string(got), "\n"); lines != 2 {
		t.Fatalf("export wrote %d records, want 2 (page 2 follows links.next):\n%s", lines, got)
	}
	// A continuation export cannot follow refuses the backup.
	run = r17Runner(t, (&r18API{routes: map[string]string{
		"/v1/networks/all": `{"data":[{"id":"1"}],"links":{"next":"https://api.example/v1/networks/all?page%5Bcursor%5D=zz"}}`,
	}}).handler(t))
	dir, path := exportBackup(t, "OLD\n")
	if _, _, err := run("export", "networks", "--output", path, "--no-cache"); err == nil {
		t.Fatal("export published a backup past a continuation it cannot follow")
	}
	assertBackupUntouched(t, "unfollowable", dir, path, "OLD\n")
	// A --limit the first page already satisfies never needs that page.
	if _, errOut, err := run("export", "networks", "--output", path, "--no-cache", "--limit", "1"); err != nil {
		t.Fatalf("--limit 1 satisfied by page 1: %v\n%s", err, errOut)
	}
}

// Finding 8c: the export file got no private-file check (Windows ACLs), so a
// broadly readable file could replace an owner-only backup.
func TestR18ExportPrivateFileCheck(t *testing.T) {
	orig := cliutil.SavedCredentialsPermsCheck
	t.Cleanup(func() { cliutil.SavedCredentialsPermsCheck = orig })
	checked := 0
	cliutil.SavedCredentialsPermsCheck = func(string) error {
		checked++
		return errors.New("BUILTIN\\Users can read this file")
	}
	run := r17Runner(t, (&r18API{routes: map[string]string{"/v1/networks/all": r18List(`{"id":"1","attributes":{"secret_key":"R18SECRET"}}`)}}).handler(t))
	dir, path := exportBackup(t, "OWNER-ONLY\n")
	if _, _, err := run("export", "networks", "--output", path, "--no-cache"); err == nil || checked == 0 {
		t.Fatalf("export ignored a failing private-file check: err=%v checks=%d", err, checked)
	}
	assertBackupUntouched(t, "refused", dir, path, "OWNER-ONLY\n")
	// The real guard accepts the export's own private temp file.
	cliutil.SavedCredentialsPermsCheck = orig
	if _, errOut, err := run("export", "networks", "--output", path, "--no-cache"); err != nil {
		t.Fatalf("export with the real guard: %v\n%s", err, errOut)
	}
}

// Finding 9: a policy a site (or schedule) uses but the mirror lacks did not
// withhold the all-clear.
func TestR18ListsWhereMissingReferencedPolicy(t *testing.T) {
	for name, site := range map[string]string{
		"site policy":   `{"id":"50","attributes":{"name":"HQ"},"relationships":{"organization":{"data":{"id":"1"}},"policy":{"data":{"id":"20"}}}}`,
		"schedule slot": `{"id":"50","attributes":{"name":"HQ"},"relationships":{"organization":{"data":{"id":"1"}},"scheduled_policy":{"data":{"id":"8"}}}}`,
	} {
		run := mirrorRunner(t, &fixtureAPI{})
		db := filepath.Join(t.TempDir(), "mirror.db")
		seedMirror(t, db, map[string][]string{
			"organizations": {mirrorOrg}, "networks": {site}, "user-agents": {mirrorAgent()},
			"policies":           {`{"id":"10","attributes":{"name":"Std","is_global_policy":false,"whitelist_domains":[],"blacklist_domains":[]}}`},
			"scheduled-policies": {`{"id":"8","attributes":{"name":"Always 20","timezone":"UTC","policy_ids":` + scheduleSlots(20, 20) + `}}`},
		}, map[string]string{"organizations": "complete", "networks": "complete", "user-agents": "complete", "policies": "complete", "scheduled-policies": "complete"})
		out, _ := run("lists", "where", "example.com", "--db", db, "--json")
		if out == "[]\n" || !strings.Contains(out, `"20"`) {
			t.Fatalf("%s: policy 20 is in use but missing from the mirror, yet lists where returned an all-clear:\n%s", name, out)
		}
		if human, _ := run("lists", "where", "example.com", "--db", db, "--human-friendly"); strings.Contains(human, "No synced policy lists") {
			t.Fatalf("%s: human output printed the all-clear:\n%s", name, human)
		}
	}
}

// Finding 10: a fractional id (60.9) was truncated into a real id ("60").
func TestR18FractionalIDsUnreadable(t *testing.T) {
	cats := []jaRes{ja(t, `{"id":"60","attributes":{"name":"Malware","security":true}}`)}
	p := ja(t, `{"id":"10","attributes":{`+auditFullAttrs+`,"name":"P","blacklist_categories":[60.9]}}`)
	found := false
	for _, f := range auditPolicies([]jaRes{p}, cats, nil, nil) {
		if f.Kind == "policy_attributes_unknown" && strings.Contains(f.Detail, "blacklist_categories") {
			found = true
		}
	}
	if !found {
		t.Fatal("blacklist_categories [60.9] certified that security category 60 is blocked")
	}
	if v := evaluatePolicy("example.com", ja(t, `{"id":"10","attributes":{"name":"P","whitelist_domains":[],"allow_list_only":false,"blacklist_domains":[],"allow_unknown_domains":true,"blacklist_categories":[7.5]}}`), []categoryRef{{ID: "7", Name: "Social"}}, true); v.Decision != "unknown" {
		t.Fatalf("blacklist_categories [7.5] decided %s/%s; a fractional id is unreadable", v.Decision, v.Rule)
	}
}

// Finding 11: a numeric domain (4821) was coerced into a domain.
func TestR18TriageNumericDomainIsUnknown(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-10-14T12:00:00Z")
	reqs := []jaRes{ja(t, `{"id":"u1","attributes":{"domain":4821,"category":{"is_threat":false}}}`)}
	groups := groupUnblockRequests(reqs, map[string]map[string]any{"u1": {"covered": true, "list": "policy"}}, now)
	if len(groups) != 1 || groups[0].Domain != "" || groups[0].Hint != "review" {
		t.Fatalf("groups = %+v; a numeric domain belongs to the unknown-domain group (hint review)", groups)
	}
}

// Finding 12 (adversarial r19, P2): an unreadable attribute whose every
// value gives the same verdict left it unknown.
func TestR18EvaluateUnknownAttributeAgreement(t *testing.T) {
	social := []categoryRef{{ID: "7", Name: "Social"}}
	for name, attrs := range map[string]string{
		"block entry, allow_list_only null":        `"whitelist_domains":[],"allow_list_only":null,"blacklist_domains":["example.net"],"allow_unknown_domains":true,"blacklist_categories":[]`,
		"blocked category, blacklist_domains null": `"whitelist_domains":[],"allow_list_only":false,"blacklist_domains":null,"allow_unknown_domains":true,"blacklist_categories":[7]`,
	} {
		if v := evaluatePolicy("example.net", ja(t, `{"id":"10","attributes":{"name":"P",`+attrs+`}}`), social, true); v.Decision != "blocked" {
			t.Errorf("%s: %s/%s; every value of the unreadable attribute blocks", name, v.Decision, v.Rule)
		}
	}
	// Values that disagree stay unknown.
	if v := evaluatePolicy("example.net", ja(t, `{"id":"10","attributes":{"name":"P","whitelist_domains":[],"allow_list_only":null,"blacklist_domains":[],"allow_unknown_domains":true,"blacklist_categories":[]}}`), social, true); v.Decision != "unknown" {
		t.Errorf("allow_list_only null with nothing else blocking: %s/%s, want unknown", v.Decision, v.Rule)
	}
}

// Finding 12 (allow): --confirm with every target left for review exited 0
// with applied:false while the domain stayed blocked.
func TestR18AllowAllReviewExitsNonZero(t *testing.T) {
	routes := r18BlockRoutes()
	routes["/v1/policies/all"] = r18List(r18Policy("10", map[string]any{"allow_list_only": nil}))
	api := &r18API{routes: routes}
	run := r17Runner(t, api.handler(t))
	out, _, err := run("allow", "example.com", "--org", "1", "--note", "T", "--confirm", "--json")
	if exitCode(err) != 6 || len(api.writesCopy()) != 0 {
		t.Fatalf("exit %d (%v) writes=%v; every target review must exit 6 and write nothing\n%s", exitCode(err), err, api.writesCopy(), out)
	}
	if m := r17JSON(t, "allow", out); m["applied"] != false {
		t.Fatalf("applied = %v, want false:\n%s", m["applied"], out)
	}
	// The preview (no --confirm) still exits 0.
	if _, _, err := run("allow", "example.com", "--org", "1", "--note", "T", "--json"); err != nil {
		t.Fatalf("preview: %v", err)
	}
}

// Control: a spec-shaped, complete and healthy fixture still gets definite
// answers from every novel command touched this round.
func TestR18HealthyFixtureStaysDefinite(t *testing.T) {
	t.Run("why-blocked and allow", func(t *testing.T) {
		routes := map[string]string{
			"/v1/organizations/1": `{"data":{"id":"1","type":"organizations","attributes":{"name":"Acme Dental"}}}`,
			"/v1/policies/all": r18List(
				r18Policy("10", map[string]any{"name": "Std", "blacklist_domains": []any{"games.example.net"}}),
				r18Policy("13", map[string]any{"name": "Lab"})),
			"/v1/networks/all": r18List(
				r18Site("50", `"policy":{"data":{"id":"10"}},"scheduled_policy":{"data":null}`),
				r18Site("51", `"policy":{"data":null},"scheduled_policy":{"data":{"id":"300"}}`)),
			"/v1/user_agents/all": r18List(
				`{"id":"a1","attributes":{"hostname":"PC-01","status":"active","uninstalled_at":null},"relationships":{"network":{"data":{"id":"50"}},"policy":{"data":null},"scheduled_policy":{"data":null}}}`,
				`{"id":"a8","attributes":{"hostname":"LAB-01","status":"active"},"relationships":{"network":{"data":{"id":"51"}},"policy":{"data":{"id":"13"}},"scheduled_policy":{"data":null}}}`),
			"/v1/scheduled_policies/300":                     `{"name":"Always 10","timezone":"UTC","policy_ids":` + scheduleSlots(10, 10) + `}`,
			"/v1/domains/user_lookup?fqdn=tools.example.io":  `{"data":{"id":"d1","attributes":{"host":"tools.example.io"},"relationships":{"categories":{"data":[]}}}}`,
			"/v1/domains/user_lookup?fqdn=games.example.net": `{"data":{"id":"d2","attributes":{"host":"games.example.net"},"relationships":{"categories":{"data":[]}}}}`,
			"/v1/traffic_reports/query_logs":                 `{"data":{"values":[],"page":{"self":1,"next":null,"size":100,"total":0}}}`,
			"/v1/policies/10":                                r18Policy("10", map[string]any{"name": "Std", "blacklist_domains": []any{"games.example.net"}, "whitelist_domains": []any{"games.example.net"}}),
		}
		api := &r18API{routes: routes}
		run := r17Runner(t, api.handler(t))
		for domain, want := range map[string]string{"tools.example.io": "not_blocked", "games.example.net": "blocked"} {
			out, errOut, err := run("why-blocked", domain, "--org", "1", "--json")
			if err != nil {
				t.Fatalf("why-blocked %s: %v\n%s", domain, err, errOut)
			}
			if m := r17JSON(t, domain, out); m["decision"] != want {
				t.Fatalf("why-blocked %s: decision=%v, want %s\n%s", domain, m["decision"], want, out)
			}
		}
		out, errOut, err := run("allow", "games.example.net", "--org", "1", "--note", "TKT-1", "--confirm", "--json")
		if err != nil {
			t.Fatalf("allow: %v\n%s%s", err, out, errOut)
		}
		if m := r17JSON(t, "allow", out); m["applied"] != true || strings.Join(api.writesCopy(), ",") != "/v1/policies/10/add_whitelist_domain" {
			t.Fatalf("allow on a healthy fixture: applied=%v writes=%v\n%s", m["applied"], api.writesCopy(), out)
		}
	})
	t.Run("policy-audit, triage and export", func(t *testing.T) {
		clean := map[string]any{"allow_unknown_domains": false, "blacklist_categories": []any{60}, "whitelist_domains": []any{"partner.example.org"}}
		full := make([]string, dnsfilterPageSize)
		for i := range full {
			full[i] = fmt.Sprintf(`{"id":"%d","type":"networks","attributes":{"name":"S%d"}}`, i+1, i+1)
		}
		run := r17Runner(t, (&r18API{routes: map[string]string{
			"/v1/categories/all":              r18List(`{"id":"60","attributes":{"name":"Malware","security":true}}`, `{"id":"8","attributes":{"name":"Business","security":false}}`),
			"/v1/policies/all":                r18List(r18Policy("10", clean), r18Policy("11", clean)),
			"/v1/domains/bulk_lookup":         `{"data":[{"id":"d1","attributes":{"host":"partner.example.org"},"relationships":{"categories":{"data":[{"id":"8"}]}}}]}`,
			"/v2/unblock_requests":            `{"data":[{"id":"u1","attributes":{"domain":"partner.example.org","category":{"is_threat":false}}},{"id":"u2","attributes":{"domain":"evil.example","category":{"is_threat":true}}}],"meta":{"current_page":1,"total_pages":1,"total_count":2}}`,
			"/v2/unblock_requests/u1":         `{"data":{"id":"u1","attributes":{"already_covered":{"covered":true,"list":"policy"}}}}`,
			"/v2/unblock_requests/u2":         `{"data":{"id":"u2","attributes":{"already_covered":{"covered":false}}}}`,
			"/v1/networks/all":                `{"data":[` + strings.Join(full, ",") + `],"links":{"next":"https://api.example/v1/networks/all?page%5Bnumber%5D=2&page%5Bsize%5D=100"}}`,
			"/v1/networks/all?page[number]=2": `{"data":[{"id":"101","attributes":{"name":"Last"}}],"links":{"next":null}}`,
		}}).handler(t))
		out, _, err := run("policy-audit", "--json")
		if err != nil {
			t.Fatalf("policy-audit: %v", err)
		}
		if m := r17JSON(t, "policy-audit", out); len(m["findings"].([]any)) != 0 || m["note"] != nil {
			t.Fatalf("a clean, complete fixture did not audit clean:\n%s", out)
		}
		out, _, err = run("unblocks", "triage", "--json")
		if err != nil {
			t.Fatalf("triage: %v", err)
		}
		hints := map[string]any{}
		for _, g := range r17JSON(t, "triage", out)["groups"].([]any) {
			gm := g.(map[string]any)
			hints[gm["domain"].(string)] = gm["hint"]
		}
		if hints["partner.example.org"] != "resolve-already-allowed" || hints["evil.example"] != "deny" {
			t.Fatalf("triage hints = %v", hints)
		}
		_, path := exportBackup(t, "OLD\n")
		if _, errOut, err := run("export", "networks", "--output", path, "--no-cache"); err != nil {
			t.Fatalf("export: %v\n%s", err, errOut)
		}
		if got, _ := os.ReadFile(path); strings.Count(string(got), "\n") != dnsfilterPageSize+1 {
			t.Fatalf("export wrote %d records, want %d", strings.Count(string(got), "\n"), dnsfilterPageSize+1)
		}
	})
	t.Run("local mirror commands", func(t *testing.T) {
		run := mirrorRunner(t, &fixtureAPI{releases: []string{mirrorFeedLatest}})
		db := filepath.Join(t.TempDir(), "mirror.db")
		seedMirror(t, db, map[string][]string{
			"organizations": {mirrorOrg}, "networks": {mirrorSite}, "policies": {mirrorPolicy},
			"user-agents": {mirrorAgent(), `{"id":"a4","attributes":{"hostname":"OLD","agent_type":"windows","agent_version":"1.0.0","status":"uninstalled","uninstalled_at":"2026-01-01"},"relationships":{"network":{"data":{"id":"50"}}}}`},
		}, map[string]string{"organizations": "complete", "networks": "complete", "user-agents": "complete", "policies": "complete", "scheduled-policies": "complete"})
		if out, _ := run("lists", "where", "nothing.test", "--db", db, "--json"); out != "[]\n" {
			t.Fatalf("lists where all-clear on a complete mirror:\n%s", out)
		}
		if out, _ := run("lists", "where", "example.com", "--db", db, "--json"); !strings.Contains(out, `"roaming_clients": 1`) || strings.Contains(out, `"meta"`) {
			t.Fatalf("lists where on a complete mirror:\n%s", out)
		}
		if out, _ := run("agents", "stale", "--db", db, "--human-friendly"); !strings.Contains(out, "No roaming clients silent for more than") {
			t.Fatalf("agents stale all-clear on a healthy mirror:\n%s", out)
		}
		if out, _ := run("agents", "drift", "--db", db, "--human-friendly"); !strings.Contains(out, "No client org has roaming clients behind the latest release.") {
			t.Fatalf("agents drift all-clear on a current fleet:\n%s", out)
		}
	})
}
