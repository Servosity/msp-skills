// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
package syncer

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

type fakeAPI struct {
	// responses maps "path?page_token" (token empty for first page) to a body.
	responses map[string]string
	calls     []string
	errors    map[string]error
}

func (f *fakeAPI) Get(_ context.Context, path string, params map[string]string) (json.RawMessage, error) {
	key := path
	if tok := params["page_token"]; tok != "" {
		key = path + "?" + tok
	}
	f.calls = append(f.calls, key)
	if err, ok := f.errors[key]; ok {
		return nil, err
	}
	body, ok := f.responses[key]
	if !ok {
		return nil, fmt.Errorf("404 not found: %s", key)
	}
	return json.RawMessage(body), nil
}

type fakeSink struct {
	rows  map[string]map[string]map[string]any // rtype -> id -> obj
	state map[string]int
}

func newFakeSink() *fakeSink {
	return &fakeSink{rows: map[string]map[string]map[string]any{}, state: map[string]int{}}
}

func (f *fakeSink) Upsert(resourceType, id string, data json.RawMessage) error {
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	if f.rows[resourceType] == nil {
		f.rows[resourceType] = map[string]map[string]any{}
	}
	f.rows[resourceType][id] = obj
	return nil
}

func (f *fakeSink) SaveSyncState(resourceType, _ string, count int) error {
	f.state[resourceType] = count
	return nil
}

func fixedNow() time.Time {
	return time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
}

// fleetFixture wires a one-org, one-tenant happy path.
func fleetFixture() *fakeAPI {
	return &fakeAPI{responses: map[string]string{
		"/api/v1/applications/me/installations":         `{"items":[{"org_id":"org1","tenant_id":""}],"next_page_token":""}`,
		"/api/v1/orgs/org1":                             `{"id":"org1","name":"Partner Org","kind":"partner","external_id":"x-org1"}`,
		"/api/v1/orgs/org1/orgs":                        `{"items":[],"next_page_token":""}`,
		"/api/v1/orgs/org1/tenants":                     `{"items":[{"id":"ten1","name":"Contoso","kind":"o365","external_id":"x-ten1","region":"us","deleted":false}],"next_page_token":""}`,
		"/api/v1/orgs/org1/licensing/subscriptions":     `{"items":[{"id":"sub1","status":"active","tenant_id":"ten1","items":[{"kind":"resource","qty":"25"}]}],"next_page_token":""}`,
		"/api/v1/tenants/ten1/quotas":                   `{"quotas":[{"kind":"resource","used":"20","limit":"25","exceeded":false},{"kind":"storage","used":"99","limit":"100","units":"GB","exceeded":true}]}`,
		"/api/v1/tenants/ten1/resources":                `{"items":[{"id":"res1","tenant_id":"ten1","name":"Jane Doe","kind":"user","external_id":"x-res1","archived":false},{"id":"res2","tenant_id":"ten1","name":"Bob","kind":"user","external_id":"x-res2","archived":false}],"next_page_token":""}`,
		"/api/v1/tenants/ten1/protections":              `{"items":[{"id":"prot1","resource_id":"res1","policy_id":"pol1","job_id":"job1"}],"next_page_token":""}`,
		"/api/v1/tenants/ten1/policies":                 `{"items":[{"id":"pol1","tenant_id":"ten1","name":"Daily"}],"next_page_token":""}`,
		"/api/v1/tenants/ten1/archives":                 `{"items":[{"id":"arc1","tenant_id":"ten1","resource_id":"res1","created_at":"2026-06-05T01:00:00Z","stats":{"size":"1024"}}],"next_page_token":""}`,
		"/api/v1/tenants/ten1/tasks/statistics/summary": `{"total":{"done":10,"failed":1,"warnings":2},"by_action":{"backup":{"done":10,"failed":1,"warnings":2}}}`,
	}}
}

func TestRunHappyPath(t *testing.T) {
	api := fleetFixture()
	sink := newFakeSink()
	sum, err := Run(context.Background(), api, sink, Options{Now: fixedNow})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	cases := []struct {
		rtype string
		want  int
	}{
		{"installations", 1},
		{"orgs", 1},
		{"tenants", 1},
		{"subscriptions", 1},
		{"resources", 2},
		{"protections", 1},
		{"policies", 1},
		{"archives", 1},
		{"quotas", 2},
		{"task_stats", 1},
	}
	for _, c := range cases {
		if got := len(sink.rows[c.rtype]); got != c.want {
			t.Errorf("%s: got %d rows, want %d", c.rtype, got, c.want)
		}
		if sink.state[c.rtype] != c.want {
			t.Errorf("%s sync_state: got %d, want %d", c.rtype, sink.state[c.rtype], c.want)
		}
	}
	if sum.Orgs != 1 || sum.Tenants != 1 {
		t.Errorf("summary orgs/tenants: got %d/%d, want 1/1", sum.Orgs, sum.Tenants)
	}
	if len(sum.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", sum.Warnings)
	}
}

func TestRunInjectsParentIDs(t *testing.T) {
	api := fleetFixture()
	sink := newFakeSink()
	if _, err := Run(context.Background(), api, sink, Options{Now: fixedNow}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	cases := []struct {
		rtype, id, field, want string
	}{
		{"protections", "prot1", "tenant_id", "ten1"}, // injected (API omits it)
		{"tenants", "ten1", "org_id", "org1"},         // injected from path
		{"subscriptions", "sub1", "org_id", "org1"},   // injected from path
		{"quotas", "ten1:storage", "tenant_id", "ten1"},
	}
	for _, c := range cases {
		obj := sink.rows[c.rtype][c.id]
		if obj == nil {
			t.Fatalf("%s/%s missing", c.rtype, c.id)
		}
		if got, _ := obj[c.field].(string); got != c.want {
			t.Errorf("%s/%s %s: got %q, want %q", c.rtype, c.id, c.field, got, c.want)
		}
	}
}

func TestRunPaginatesUntilTokenEmpty(t *testing.T) {
	api := fleetFixture()
	api.responses["/api/v1/tenants/ten1/resources"] = `{"items":[{"id":"res1","tenant_id":"ten1","name":"A","kind":"user"}],"next_page_token":"p2"}`
	api.responses["/api/v1/tenants/ten1/resources?p2"] = `{"items":[{"id":"res2","tenant_id":"ten1","name":"B","kind":"user"}],"next_page_token":""}`
	sink := newFakeSink()
	if _, err := Run(context.Background(), api, sink, Options{Now: fixedNow}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := len(sink.rows["resources"]); got != 2 {
		t.Errorf("resources after pagination: got %d, want 2", got)
	}
}

func TestRunTenantErrorIsWarningNotFatal(t *testing.T) {
	api := fleetFixture()
	api.errors = map[string]error{
		"/api/v1/tenants/ten1/archives": fmt.Errorf("403 forbidden"),
	}
	sink := newFakeSink()
	sum, err := Run(context.Background(), api, sink, Options{Now: fixedNow})
	if err != nil {
		t.Fatalf("Run should tolerate per-tenant failures: %v", err)
	}
	if len(sink.rows["resources"]) != 2 {
		t.Errorf("resources should still sync when archives fail")
	}
	found := false
	for _, w := range sum.Warnings {
		if strings.Contains(w, "archives") && strings.Contains(w, "403") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected an archives warning, got %v", sum.Warnings)
	}
}

func TestRunInstallationsFailureIsFatal(t *testing.T) {
	api := &fakeAPI{responses: map[string]string{}, errors: map[string]error{
		"/api/v1/applications/me/installations": fmt.Errorf("401 unauthenticated"),
	}}
	sink := newFakeSink()
	if _, err := Run(context.Background(), api, sink, Options{Now: fixedNow}); err == nil {
		t.Fatal("Run should fail when the installations root call fails")
	}
}

func TestRunWalksChildOrgs(t *testing.T) {
	api := fleetFixture()
	api.responses["/api/v1/orgs/org1/orgs"] = `{"items":[{"id":"org2","name":"Customer Org","kind":"customer"}],"next_page_token":""}`
	api.responses["/api/v1/orgs/org2"] = `{"id":"org2","name":"Customer Org","kind":"customer"}`
	api.responses["/api/v1/orgs/org2/orgs"] = `{"items":[],"next_page_token":""}`
	api.responses["/api/v1/orgs/org2/tenants"] = `{"items":[{"id":"ten2","name":"Fabrikam","kind":"gsuite"}],"next_page_token":""}`
	api.responses["/api/v1/orgs/org2/licensing/subscriptions"] = `{"items":[],"next_page_token":""}`
	for _, p := range []string{"/api/v1/tenants/ten2/quotas"} {
		api.responses[p] = `{"quotas":[]}`
	}
	for _, p := range []string{"/api/v1/tenants/ten2/resources", "/api/v1/tenants/ten2/protections", "/api/v1/tenants/ten2/policies", "/api/v1/tenants/ten2/archives"} {
		api.responses[p] = `{"items":[],"next_page_token":""}`
	}
	api.responses["/api/v1/tenants/ten2/tasks/statistics/summary"] = `{"total":{"done":0,"failed":0,"warnings":0}}`
	sink := newFakeSink()
	sum, err := Run(context.Background(), api, sink, Options{Now: fixedNow})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if sum.Orgs != 2 {
		t.Errorf("orgs walked: got %d, want 2", sum.Orgs)
	}
	if sink.rows["orgs"]["org2"] == nil {
		t.Errorf("child org org2 not stored")
	}
	if got, _ := sink.rows["orgs"]["org2"]["parent_org_id"].(string); got != "org1" {
		t.Errorf("child org parent_org_id: got %q, want org1", got)
	}
	if sink.rows["tenants"]["ten2"] == nil {
		t.Errorf("child org tenant ten2 not stored")
	}
}

func TestRunTenantFilter(t *testing.T) {
	api := fleetFixture()
	sink := newFakeSink()
	sum, err := Run(context.Background(), api, sink, Options{Now: fixedNow, Tenants: []string{"other-tenant"}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if sum.Tenants != 0 {
		t.Errorf("tenant filter should exclude ten1: got %d tenants", sum.Tenants)
	}
	if len(sink.rows["resources"]) != 0 {
		t.Errorf("resources should be empty under non-matching filter")
	}
}

func TestRunDirectTenantInstallation(t *testing.T) {
	api := &fakeAPI{responses: map[string]string{
		"/api/v1/applications/me/installations":         `{"items":[{"org_id":"org9","tenant_id":"ten9"}],"next_page_token":""}`,
		"/api/v1/orgs/org9":                             `{"id":"org9","name":"Org9"}`,
		"/api/v1/orgs/org9/orgs":                        `{"items":[],"next_page_token":""}`,
		"/api/v1/orgs/org9/tenants":                     `{"items":[],"next_page_token":""}`,
		"/api/v1/orgs/org9/licensing/subscriptions":     `{"items":[],"next_page_token":""}`,
		"/api/v1/tenants/ten9":                          `{"id":"ten9","name":"Direct","kind":"o365"}`,
		"/api/v1/tenants/ten9/quotas":                   `{"quotas":[]}`,
		"/api/v1/tenants/ten9/resources":                `{"items":[],"next_page_token":""}`,
		"/api/v1/tenants/ten9/protections":              `{"items":[],"next_page_token":""}`,
		"/api/v1/tenants/ten9/policies":                 `{"items":[],"next_page_token":""}`,
		"/api/v1/tenants/ten9/archives":                 `{"items":[],"next_page_token":""}`,
		"/api/v1/tenants/ten9/tasks/statistics/summary": `{"total":{"done":3,"failed":0,"warnings":0}}`,
	}}
	sink := newFakeSink()
	sum, err := Run(context.Background(), api, sink, Options{Now: fixedNow})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if sum.Tenants != 1 {
		t.Errorf("direct tenant should be walked: got %d tenants", sum.Tenants)
	}
	if got, _ := sink.rows["tenants"]["ten9"]["org_id"].(string); got != "org9" {
		t.Errorf("direct tenant org_id: got %q, want org9", got)
	}
}

func TestRunSkipArchives(t *testing.T) {
	api := fleetFixture()
	delete(api.responses, "/api/v1/tenants/ten1/archives")
	sink := newFakeSink()
	sum, err := Run(context.Background(), api, sink, Options{Now: fixedNow, SkipArchives: true})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(sink.rows["archives"]) != 0 {
		t.Errorf("archives should be skipped")
	}
	for _, w := range sum.Warnings {
		if strings.Contains(w, "archives") {
			t.Errorf("no archives warning expected when skipped: %v", w)
		}
	}
}
