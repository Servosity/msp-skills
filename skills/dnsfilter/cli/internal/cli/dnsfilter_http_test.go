// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HTTP-fixture tests: drive the real why-blocked / allow command paths against
// an httptest server shaped like the DNSFilter API.

package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"dnsfilter-pp-cli/internal/cliutil/testenv"
)

type fixtureAPI struct {
	mu          sync.Mutex
	lookupBody  string            // user_lookup response
	agents      []string          // user_agents/all data items
	logPages    map[string]string // page[number] -> query_logs values JSON
	bulkResp    string            // bulk add_allowlist_domains response
	allowlisted map[string]bool   // policy id -> read-back contains the domain
	writes      []string
	bodies      []map[string]any
	releases    []string // release feed bodies served in call order (last repeats)
	logFailPage string   // query_logs page that returns HTTP 500
	feedCalls   int
}

func (f *fixtureAPI) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "fixture-key" {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"error":"Not Authorized"}`)
			return
		}
		q := r.URL.Query()
		page := q.Get("page[number]")
		if page == "" {
			page = "1"
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		switch p := r.URL.Path; {
		case r.Method == http.MethodPost:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.writes = append(f.writes, p)
			f.bodies = append(f.bodies, body)
			switch {
			case p == "/v1/policies/bulk/add_allowlist_domains":
				fmt.Fprint(w, f.bulkResp)
			case strings.HasPrefix(p, "/v1/policies/") && strings.HasSuffix(p, "/add_whitelist_domain"):
				fmt.Fprint(w, `{"data":{"id":"10","attributes":{}}}`)
			default:
				w.WriteHeader(http.StatusNotFound)
				fmt.Fprint(w, `{"error":"unexpected write"}`)
			}
		case r.Method != http.MethodGet:
			w.WriteHeader(http.StatusMethodNotAllowed)
		case page != "1" && (p == "/v1/networks/all" || p == "/v1/policies/all" || p == "/v1/user_agents/all" || p == "/v1/categories/all"):
			fmt.Fprint(w, `{"data":[]}`)
		case p == "/v1/categories/all":
			fmt.Fprint(w, `{"data":[{"id":"7","attributes":{"name":"Social","security":false}}]}`)
		case p == "/v1/user_agent_releases":
			body := f.releases[min(f.feedCalls, len(f.releases)-1)]
			f.feedCalls++
			fmt.Fprint(w, body)
		case p == "/v1/organizations/all" && page == "1":
			fmt.Fprint(w, `{"data":[{"id":"1","attributes":{"id":1,"name":"Acme Dental","user_agents_auto_update":false}}]}`)
		case p == "/v1/organizations/all":
			fmt.Fprint(w, `{"data":[]}`)
		case p == "/v1/networks/78":
			fmt.Fprint(w, `{"data":{"id":"78","attributes":{"name":"Unlinked site"},"relationships":{"policy":{"data":{"id":"11"}}}}}`)
		case p == "/v1/networks/77":
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"error":"boom"}`)
		case p == "/v1/networks/70":
			fmt.Fprint(w, `{"data":{"id":"70","attributes":{"name":"Far site"},"relationships":{"organization":{"data":{"id":"1"}},"policy":{"data":{"id":"10"}}}}}`)
		case p == "/v1/networks/71":
			fmt.Fprint(w, `{"data":{"id":"71","attributes":{"name":"Other org"},"relationships":{"organization":{"data":{"id":"2"}},"policy":{"data":{"id":"10"}}}}}`)
		case p == "/v1/organizations/1":
			fmt.Fprint(w, `{"data":{"id":"1","attributes":{"id":1,"name":"Acme Dental"}}}`)
		case p == "/v1/networks/all" && page == "1":
			fmt.Fprint(w, `{"data":[
				{"id":"50","attributes":{"name":"HQ"},"relationships":{"organization":{"data":{"id":"1"}},"policy":{"data":{"id":"10"}}}},
				{"id":"51","attributes":{"name":"Annex"},"relationships":{"organization":{"data":{"id":"1"}},"policy":{"data":{"id":"11"}}}}]}`)
		case p == "/v1/policies/all" && page == "1":
			// Both block example.com by an exact block-list entry, with every
			// attribute the policy evaluation reads present.
			fmt.Fprint(w, `{"data":[
				{"id":"10","attributes":{"name":"Std","can_edit":true,"is_global_policy":false,"whitelist_domains":[],"allow_list_only":false,"blacklist_domains":["example.com"],"blacklist_categories":[],"allow_unknown_domains":true}},
				{"id":"11","attributes":{"name":"Annex","can_edit":true,"is_global_policy":false,"whitelist_domains":[],"allow_list_only":false,"blacklist_domains":["example.com"],"blacklist_categories":[],"allow_unknown_domains":true}}]}`)
		case p == "/v1/policies/95":
			// Wrong identity: answers for a different policy.
			fmt.Fprint(w, `{"data":{"id":"94","attributes":{"name":"Impostor","can_edit":true,"is_global_policy":false,"whitelist_domains":[]}}}`)
		case p == "/v1/policies/96" || p == "/v1/policies/97" || p == "/v1/policies/98":
			id := strings.TrimPrefix(p, "/v1/policies/")
			fmt.Fprintf(w, `{"data":{"id":%q,"attributes":{"name":"Locked %s","can_edit":false,"is_global_policy":false,"whitelist_domains":[]}}}`, id, id)
		case p == "/v1/policies/10" || p == "/v1/policies/11" || p == "/v1/policies/99":
			id := strings.TrimPrefix(p, "/v1/policies/")
			wl := "[]"
			if f.allowlisted[id] {
				wl = `["example.com"]`
			}
			fmt.Fprintf(w, `{"data":{"id":%q,"attributes":{"name":"P%s","can_edit":true,"is_global_policy":false,"whitelist_domains":%s}}}`, id, id, wl)
		case p == "/v1/user_agents/all" && page == "1":
			// Mirror the vendor's search: hostname / friendly name only (never ids).
			want := strings.ToLower(q.Get("search"))
			kept := make([]string, 0)
			for _, a := range f.agents {
				var m struct {
					Attributes map[string]any `json:"attributes"`
				}
				_ = json.Unmarshal([]byte(a), &m)
				h, _ := m.Attributes["hostname"].(string)
				fn, _ := m.Attributes["friendly_name"].(string)
				if want == "" || strings.Contains(strings.ToLower(h), want) || strings.Contains(strings.ToLower(fn), want) {
					kept = append(kept, a)
				}
			}
			fmt.Fprintf(w, `{"data":[%s]}`, strings.Join(kept, ","))
		case strings.HasPrefix(p, "/v1/user_agents/"):
			id := strings.TrimPrefix(p, "/v1/user_agents/")
			for _, a := range f.agents {
				if strings.Contains(a, `"id":"`+id+`"`) {
					fmt.Fprintf(w, `{"data":%s}`, a)
					return
				}
			}
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"error":"Not Found"}`)
		case p == "/v1/domains/user_lookup":
			fmt.Fprint(w, f.lookupBody)
		case p == "/v1/traffic_reports/query_logs" && f.logFailPage != "" && page == f.logFailPage:
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"error":"boom"}`)
		case p == "/v1/traffic_reports/query_logs":
			if q.Get("organization_id") != "1" || q.Get("result") != "blocked" || q.Get("fqdn") == "" || q.Get("page[size]") == "" {
				t.Errorf("query_logs called without org/result/fqdn/page filters: %s", r.URL.RawQuery)
			}
			vals, ok := f.logPages[page]
			if !ok {
				vals = "[]"
			}
			fmt.Fprintf(w, `{"data":{"values":%s}}`, vals)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"error":"not in fixture"}`)
		}
	})
}

func runFixture(t *testing.T, f *fixtureAPI, args ...string) (string, error) {
	t.Helper()
	testenv.Isolate(t)
	srv := httptest.NewServer(f.handler(t))
	t.Cleanup(srv.Close)
	t.Setenv("DNSFILTER_BASE_URL", srv.URL)
	t.Setenv("DNSFILTER_API_KEY", "fixture-key")
	cmd := RootCmd()
	cmd.SetArgs(append(args, "--json"))
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	err := cmd.Execute()
	return out.String(), err
}

func TestDriftSeesReleaseRollbackOnNextRun(t *testing.T) {
	testenv.Isolate(t)
	f := &fixtureAPI{
		agents: []string{`{"id":"a1","attributes":{"hostname":"PC-1","agent_type":"windows","agent_version":"3.1.0","status":"active"},"relationships":{"network":{"data":{"id":"50"}},"policy":{"data":{"id":"10"}}}}`},
		releases: []string{
			`{"user_agent_releases":[{"agent_type":"windows","architecture":"x86_64","agent_version":"3.2.0","white_label":false,"release_channels":[{"value":"stable"}]}]}`,
			`{"user_agent_releases":[{"agent_type":"windows","architecture":"x86_64","agent_version":"3.1.0","white_label":false,"release_channels":[{"value":"stable"}]}]}`,
		},
	}
	srv := httptest.NewServer(f.handler(t))
	t.Cleanup(srv.Close)
	t.Setenv("DNSFILTER_BASE_URL", srv.URL)
	t.Setenv("DNSFILTER_API_KEY", "fixture-key")
	db := t.TempDir() + "/mirror.db"
	run := func(args ...string) string {
		cmd := RootCmd()
		cmd.SetArgs(append(args, "--json"))
		var out, errOut bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&errOut)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%v: %v\n%s%s", args, err, out.String(), errOut.String())
		}
		return out.String()
	}
	run("sync", "--resources", "user-agents", "--db", db)
	behind := func(out string) (string, int) {
		var v driftView
		if err := json.Unmarshal([]byte(out), &v); err != nil {
			t.Fatalf("drift output: %v\n%s", err, out)
		}
		n := 0
		for _, o := range v.Orgs {
			n += o.BehindLatest
		}
		return v.Latest["windows"], n
	}
	if latest, n := behind(run("agents", "drift", "--db", db)); latest != "3.2.0" || n != 1 {
		t.Fatalf("before rollback: latest=%s behind=%d, want 3.2.0 and 1", latest, n)
	}
	if latest, n := behind(run("agents", "drift", "--db", db)); latest != "3.1.0" || n != 0 {
		t.Fatalf("after rollback: latest=%s behind=%d, want 3.1.0 and 0 (feed must be read live, not cached or mirrored)", latest, n)
	}
}

func TestAllowDuplicateExactHostWritesNothing(t *testing.T) {
	agents := []string{
		`{"id":"a1","attributes":{"hostname":"FRONT-01","status":"active"},"relationships":{"network":{"data":{"id":"50"}},"policy":{"data":{"id":"10"}}}}`,
		`{"id":"a2","attributes":{"hostname":"FRONT-01","status":"active"},"relationships":{"network":{"data":{"id":"51"}},"policy":{"data":{"id":"11"}}}}`,
	}
	f := &fixtureAPI{lookupBody: fixtureLookupOK, agents: agents}
	_, err := runFixture(t, f, "allow", "example.com", "--org", "1", "--host", "FRONT-01", "--note", "T", "--confirm")
	if exitCode(err) != 2 || !strings.Contains(err.Error(), "pass one roaming-client id") || len(f.writes) != 0 {
		t.Fatalf("err=%v exit=%d writes=%v; duplicate exact hostnames must refuse with zero writes", err, exitCode(err), f.writes)
	}
}

func TestHostByIDBypassesSearch(t *testing.T) {
	agents := []string{
		`{"id":"a1","attributes":{"hostname":"FRONT-01","status":"active"},"relationships":{"network":{"data":{"id":"50"}},"policy":{"data":{"id":"10"}}}}`,
		`{"id":"a2","attributes":{"hostname":"FRONT-01","status":"active"},"relationships":{"network":{"data":{"id":"51"}},"policy":{"data":{"id":"11"}}}}`,
	}
	f := &fixtureAPI{lookupBody: fixtureLookupOK, agents: agents}
	out, err := runFixture(t, f, "why-blocked", "example.com", "--org", "1", "--host", "a2")
	if err != nil {
		t.Fatalf("--host <id>: %v", err)
	}
	var view whyBlockedView
	_ = json.Unmarshal([]byte(out), &view)
	if len(view.Deployments) != 1 || view.Deployments[0].ID != "a2" || view.Deployments[0].PolicyID != "11" {
		t.Fatalf("--host a2 resolved %+v, want only a2 on policy 11", view.Deployments)
	}
}

func TestHostNameRefusedWhenScanTruncated(t *testing.T) {
	// "LAB" is exact for one client, but the search returns a full page of
	// LAB-* clients, so a duplicate exact "LAB" could sit on an unread page.
	agents := []string{`{"id":"x0","attributes":{"hostname":"LAB","status":"active"},"relationships":{"network":{"data":{"id":"50"}},"policy":{"data":{"id":"10"}}}}`}
	for i := 1; i < dnsfilterPageSize; i++ {
		agents = append(agents, fmt.Sprintf(`{"id":"x%d","attributes":{"hostname":"LAB-%02d","status":"active"},"relationships":{"network":{"data":{"id":"50"}},"policy":{"data":{"id":"10"}}}}`, i, i))
	}
	f := &fixtureAPI{lookupBody: fixtureLookupOK, agents: agents}
	_, err := runFixture(t, f, "allow", "example.com", "--org", "1", "--host", "LAB", "--note", "T", "--confirm", "--max-scan-pages", "1")
	if exitCode(err) != 2 || !strings.Contains(err.Error(), "before it could prove the name is unique") || len(f.writes) != 0 {
		t.Fatalf("err=%v exit=%d writes=%v; a truncated scan must refuse name selection with zero writes", err, exitCode(err), f.writes)
	}
}

func TestHostIDMustMatchExactly(t *testing.T) {
	// "all" is not a client id: GET /v1/user_agents/all returns a collection,
	// which must never be accepted as the named device.
	agents := []string{`{"id":"a1","attributes":{"hostname":"PC-1","status":"active"},"relationships":{"network":{"data":{"id":"50"}},"policy":{"data":{"id":"10"}}}}`}
	f := &fixtureAPI{lookupBody: fixtureLookupOK, agents: agents}
	_, err := runFixture(t, f, "allow", "example.com", "--org", "1", "--host", "all", "--note", "T", "--confirm")
	if exitCode(err) != 2 || len(f.writes) != 0 {
		t.Fatalf("--host all: err=%v exit=%d writes=%v; want exit 2 and zero writes", err, exitCode(err), f.writes)
	}
}

func TestHostIDOnUnscannedSiteChecksOrg(t *testing.T) {
	agents := []string{
		`{"id":"far","attributes":{"hostname":"FAR-PC","status":"active"},"relationships":{"network":{"data":{"id":"70"}},"policy":{"data":{"id":"10"}}}}`,
		`{"id":"alien","attributes":{"hostname":"ALIEN-PC","status":"active"},"relationships":{"network":{"data":{"id":"71"}},"policy":{"data":{"id":"10"}}}}`,
	}
	out, err := runFixture(t, &fixtureAPI{lookupBody: fixtureLookupOK, agents: agents}, "why-blocked", "example.com", "--org", "1", "--host", "far")
	if err != nil {
		t.Fatalf("--host far (org 1 site beyond the scan): %v", err)
	}
	var view whyBlockedView
	_ = json.Unmarshal([]byte(out), &view)
	if len(view.Deployments) != 1 || view.Deployments[0].ID != "far" {
		t.Fatalf("--host far resolved %+v", view.Deployments)
	}
	f := &fixtureAPI{lookupBody: fixtureLookupOK, agents: agents}
	_, err = runFixture(t, f, "allow", "example.com", "--org", "1", "--host", "alien", "--note", "T", "--confirm")
	if exitCode(err) != 2 || len(f.writes) != 0 {
		t.Fatalf("--host alien (another org's site): err=%v exit=%d writes=%v; want exit 2, zero writes", err, exitCode(err), f.writes)
	}
}

func TestAllowAgentOutputKeepsPerTargetResult(t *testing.T) {
	// One add among several skips: "result" appears on a minority of targets,
	// which frequency-based compaction would drop.
	hits := `[{"fqdn":"example.com","result":"blocked","policy_id":"11"},{"fqdn":"example.com","result":"blocked","policy_id":"97"},{"fqdn":"example.com","result":"blocked","policy_id":"98"},{"fqdn":"example.com","result":"blocked","policy_id":"96"}]`
	f := &fixtureAPI{lookupBody: fixtureLookupOK, logPages: map[string]string{"1": hits}, allowlisted: map[string]bool{"11": true}}
	out, err := runFixture(t, f, "allow", "example.com", "--org", "1", "--note", "T", "--confirm", "--agent")
	if err != nil {
		t.Fatalf("allow --agent: %v\n%s", err, out)
	}
	var env struct {
		Results allowView `json:"results"`
	}
	if jerr := json.Unmarshal([]byte(out), &env); jerr != nil {
		t.Fatalf("--agent output is not an envelope: %v\n%s", jerr, out)
	}
	byID := map[string]allowTarget{}
	for _, tg := range env.Results.Targets {
		byID[tg.PolicyID] = tg
	}
	if byID["11"].Action != "add" || byID["11"].Result != "success" {
		t.Fatalf("written target lost its receipt: %+v\n%s", byID["11"], out)
	}
	for _, id := range []string{"96", "97", "98"} {
		if byID[id].Action != "skip" || byID[id].Reason == "" {
			t.Fatalf("skipped target %s lost action/reason: %+v", id, byID[id])
		}
	}
}

func TestAllowPartialQueryLogFallsBackCompletely(t *testing.T) {
	// Page 1 is a full page of hits from policy 99, page 2 fails: nothing from
	// the partial scan may pick targets; the site assignments (10, 11) do.
	f := &fixtureAPI{lookupBody: fixtureLookupOK, logFailPage: "2", logPages: map[string]string{"1": decoyHits(dnsfilterPageSize, "example.com")}}
	s := f.logPages["1"]
	f.logPages["1"] = strings.ReplaceAll(s, `"policy_id":"10"`, `"policy_id":"99"`)
	out, err := runFixture(t, f, "allow", "example.com", "--org", "1", "--note", "T")
	if err != nil {
		t.Fatalf("allow preview: %v", err)
	}
	var view allowView
	_ = json.Unmarshal([]byte(out), &view)
	ids := []string{}
	for _, tg := range view.Targets {
		ids = append(ids, tg.PolicyID)
	}
	if view.Attribution != "assignment" || strings.Join(ids, ",") != "10,11" {
		t.Fatalf("attribution=%s targets=%v; a failed page must discard partial hits", view.Attribution, ids)
	}
}

func TestAllowRejectsWrongPolicyIdentity(t *testing.T) {
	f := &fixtureAPI{lookupBody: fixtureLookupOK, logPages: map[string]string{"1": `[{"fqdn":"example.com","result":"blocked","policy_id":"95"}]`}}
	out, err := runFixture(t, f, "allow", "example.com", "--org", "1", "--note", "T", "--confirm")
	if err != nil {
		t.Fatalf("allow: %v\n%s", err, out)
	}
	var view allowView
	_ = json.Unmarshal([]byte(out), &view)
	if len(view.Targets) != 1 || view.Targets[0].PolicyID != "95" || view.Targets[0].Action != "skip" || len(f.writes) != 0 {
		t.Fatalf("targets=%+v writes=%v; a policy read that answers for another id must never authorize a write", view.Targets, f.writes)
	}
}

func TestStrayPositionalNeverWrites(t *testing.T) {
	f := &fixtureAPI{lookupBody: fixtureLookupOK}
	// `--confirm false` is the flag --confirm plus a stray positional "false".
	_, err := runFixture(t, f, "allow", "example.com", "--confirm", "false", "--org", "1", "--note", "T")
	if exitCode(err) != 2 || !strings.Contains(err.Error(), "--flag=false") || len(f.writes) != 0 {
		t.Fatalf("err=%v exit=%d writes=%v; a stray positional must refuse with zero writes", err, exitCode(err), f.writes)
	}
}

func TestHostSiteWithoutOrgLinkRefusesName(t *testing.T) {
	agents := []string{
		`{"id":"a1","attributes":{"hostname":"TWIN","status":"active"},"relationships":{"network":{"data":{"id":"50"}},"policy":{"data":{"id":"10"}}}}`,
		`{"id":"a2","attributes":{"hostname":"TWIN","status":"active"},"relationships":{"network":{"data":{"id":"78"}},"policy":{"data":{"id":"11"}}}}`,
	}
	f := &fixtureAPI{lookupBody: fixtureLookupOK, agents: agents}
	_, err := runFixture(t, f, "allow", "example.com", "--org", "1", "--host", "TWIN", "--note", "T", "--confirm")
	if err == nil || !strings.Contains(err.Error(), "could not read the site") || len(f.writes) != 0 {
		t.Fatalf("err=%v writes=%v; a twin on a site without an organization link must block name selection with zero writes", err, f.writes)
	}
}

func TestHostSiteLookupFailureRefusesName(t *testing.T) {
	agents := []string{
		`{"id":"a1","attributes":{"hostname":"TWIN","status":"active"},"relationships":{"network":{"data":{"id":"50"}},"policy":{"data":{"id":"10"}}}}`,
		`{"id":"a2","attributes":{"hostname":"TWIN","status":"active"},"relationships":{"network":{"data":{"id":"77"}},"policy":{"data":{"id":"11"}}}}`,
	}
	f := &fixtureAPI{lookupBody: fixtureLookupOK, agents: agents}
	_, err := runFixture(t, f, "allow", "example.com", "--org", "1", "--host", "TWIN", "--note", "T", "--confirm")
	if err == nil || !strings.Contains(err.Error(), "could not read the site") || len(f.writes) != 0 {
		t.Fatalf("err=%v writes=%v; an unverifiable twin must block name selection with zero writes", err, f.writes)
	}
}

func TestDriftRejectsSingleSourceModes(t *testing.T) {
	for _, mode := range []string{"local", "live"} {
		_, err := runFixture(t, &fixtureAPI{}, "agents", "drift", "--data-source", mode)
		if exitCode(err) != 2 {
			t.Errorf("--data-source %s: exit %d (%v), want 2", mode, exitCode(err), err)
		}
	}
}

func exitCode(err error) int {
	var ce *cliError
	if errors.As(err, &ce) {
		return ce.code
	}
	if err != nil {
		return 1
	}
	return 0
}

const fixtureLookupOK = `{"data":{"id":"d1","attributes":{"host":"example.com"},"relationships":{"categories":{"data":[]}}}}`

func decoyHits(n int, fqdn string) string {
	items := make([]string, n)
	for i := range items {
		items[i] = fmt.Sprintf(`{"fqdn":%q,"result":"blocked","policy_id":"10"}`, fqdn)
	}
	return "[" + strings.Join(items, ",") + "]"
}

func TestAllowBulkExplicitFailureExits6(t *testing.T) {
	f := &fixtureAPI{lookupBody: fixtureLookupOK, bulkResp: `{"success":[10],"failure":[11],"noop":[]}`, allowlisted: map[string]bool{"10": true, "11": true}}
	out, err := runFixture(t, f, "allow", "example.com", "--org", "1", "--note", "TKT-1", "--confirm")
	if exitCode(err) != 6 {
		t.Fatalf("exit = %d (%v), want 6; out=%s", exitCode(err), err, out)
	}
	var view allowView
	if jerr := json.Unmarshal([]byte(out), &view); jerr != nil {
		t.Fatalf("output is not JSON: %v\n%s", jerr, out)
	}
	results := map[string]string{}
	for _, tg := range view.Targets {
		results[tg.PolicyID] = tg.Result
	}
	if results["10"] != "success" || results["11"] != "failure" {
		t.Fatalf("results = %v; an explicit bulk failure must stay failure even when read-back shows the entry", results)
	}
	if len(f.writes) != 1 || f.writes[0] != "/v1/policies/bulk/add_allowlist_domains" {
		t.Fatalf("writes = %v, want one bulk write", f.writes)
	}
	body, _ := json.Marshal(f.bodies[0])
	if string(body) != `{"domains":["example.com"],"notes":{"example.com":"TKT-1"},"policy_ids":[10,11]}` {
		t.Fatalf("bulk body = %s", body)
	}
}

func TestAllowSingleWriteBody(t *testing.T) {
	f := &fixtureAPI{lookupBody: fixtureLookupOK, logPages: map[string]string{"1": `[{"fqdn":"example.com","result":"blocked","policy_id":"11"}]`}, allowlisted: map[string]bool{"11": true}}
	out, err := runFixture(t, f, "allow", "example.com", "--org", "1", "--note", "TKT-9", "--confirm")
	if err != nil {
		t.Fatalf("allow: %v\n%s", err, out)
	}
	body, _ := json.Marshal(f.bodies[0])
	if len(f.writes) != 1 || f.writes[0] != "/v1/policies/11/add_whitelist_domain" || string(body) != `{"domain":"example.com","note":"TKT-9"}` {
		t.Fatalf("writes = %v body = %s; want one single-policy write to the query-log policy", f.writes, body)
	}
}

func TestAllowAttributionUsesEveryScannedHit(t *testing.T) {
	// 60 exact hits from policy 10 then one from policy 11 on the same page:
	// both policies blocked the domain and both must be targeted.
	page := strings.TrimSuffix(decoyHits(60, "example.com"), "]") + `,{"fqdn":"example.com","result":"blocked","policy_id":"11"}]`
	f := &fixtureAPI{lookupBody: fixtureLookupOK, logPages: map[string]string{"1": page}}
	out, err := runFixture(t, f, "allow", "example.com", "--org", "1", "--note", "T")
	if err != nil {
		t.Fatalf("allow preview: %v", err)
	}
	var view allowView
	_ = json.Unmarshal([]byte(out), &view)
	ids := []string{}
	for _, tg := range view.Targets {
		ids = append(ids, tg.PolicyID)
	}
	if view.Attribution != "query_log" || strings.Join(ids, ",") != "10,11" {
		t.Fatalf("attribution=%s targets=%v; want query_log 10,11", view.Attribution, ids)
	}
	if len(f.writes) != 0 {
		t.Fatalf("preview wrote: %v", f.writes)
	}
}

func TestAllowRefusesNonDomainLookup(t *testing.T) {
	f := &fixtureAPI{lookupBody: `{"data":{"error":"lookup unavailable"}}`}
	_, err := runFixture(t, f, "allow", "example.com", "--org", "1", "--note", "T", "--confirm")
	if exitCode(err) != 2 || len(f.writes) != 0 {
		t.Fatalf("err=%v exit=%d writes=%v; a non-domain lookup must refuse without --force", err, exitCode(err), f.writes)
	}
}

func TestAllowRefusesUnknownLookupShape(t *testing.T) {
	f := &fixtureAPI{lookupBody: `{"data":null}`}
	_, err := runFixture(t, f, "allow", "example.com", "--org", "1", "--note", "TKT-1", "--confirm")
	if exitCode(err) != 2 || !strings.Contains(err.Error(), "categories (and threat status) are unknown") {
		t.Fatalf("err = %v (exit %d), want exit 2 refusal for an unusable lookup", err, exitCode(err))
	}
	if len(f.writes) != 0 {
		t.Fatalf("refused allow still wrote: %v", f.writes)
	}
}

func TestWhyBlockedAttributionScansPastSubdomainPages(t *testing.T) {
	f := &fixtureAPI{lookupBody: fixtureLookupOK, logPages: map[string]string{
		"1": decoyHits(dnsfilterPageSize, "cdn.example.com"),
		"2": `[{"fqdn":"example.com","result":"blocked","policy_id":"99","policy_name":"Exec Collection"}]`,
	}}
	out, err := runFixture(t, f, "why-blocked", "example.com", "--org", "1")
	if err != nil {
		t.Fatalf("why-blocked: %v", err)
	}
	var view whyBlockedView
	if jerr := json.Unmarshal([]byte(out), &view); jerr != nil {
		t.Fatalf("output is not JSON: %v\n%s", jerr, out)
	}
	if len(view.RecentHits) != 1 || view.RecentHits[0].FQDN != "example.com" {
		t.Fatalf("recent hits = %+v; want only the exact page-2 hit", view.RecentHits)
	}
	if len(view.BlockingPolicyIDs) != 1 || view.BlockingPolicyIDs[0] != "99" {
		t.Fatalf("blocking policies = %v, want [99]", view.BlockingPolicyIDs)
	}
	if !strings.Contains(strings.Join(view.Notes, "\n"), "policy 99, which is not a site or roaming-client assignment") {
		t.Fatalf("notes do not flag the non-assigned blocking policy: %v", view.Notes)
	}
}

func TestHostMatchingExactWinsAmbiguousRefused(t *testing.T) {
	agents := []string{
		`{"id":"a1","attributes":{"hostname":"FRONT-01","status":"active"},"relationships":{"network":{"data":{"id":"50"}},"policy":{"data":{"id":"10"}}}}`,
		`{"id":"a2","attributes":{"hostname":"FRONT-010","status":"active"},"relationships":{"network":{"data":{"id":"50"}},"policy":{"data":{"id":"11"}}}}`,
	}
	f := &fixtureAPI{lookupBody: fixtureLookupOK, agents: agents}
	out, err := runFixture(t, f, "why-blocked", "example.com", "--org", "1", "--host", "front-01")
	if err != nil {
		t.Fatalf("exact host: %v", err)
	}
	var view whyBlockedView
	_ = json.Unmarshal([]byte(out), &view)
	if len(view.Deployments) != 1 || view.Deployments[0].Name != "FRONT-01" {
		t.Fatalf("exact --host matched %+v, want only FRONT-01", view.Deployments)
	}
	_, err = runFixture(t, &fixtureAPI{lookupBody: fixtureLookupOK, agents: agents}, "allow", "example.com", "--org", "1", "--host", "FRONT-0", "--note", "T", "--confirm")
	if exitCode(err) != 2 || !strings.Contains(err.Error(), "matches several roaming clients") {
		t.Fatalf("ambiguous --host: err=%v exit=%d, want exit 2", err, exitCode(err))
	}
}
