// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Twoclass REC 2 (2026-10-26, handfixes.json sync-collects-deal-pipelines):
// on a fresh install every pipeline reader works after the documented
// command, sync collects the deal pipelines and each deal's hs_is_closed into
// the store the command reads (--db included), and every reader goes through
// the one shared pipeline reader.

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hubspot-pp-cli/internal/cliutil/testenv"
)

const tcPipelines = `{"results":[{"id":"default","label":"Sales Pipeline","displayOrder":0,"archived":false,"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-01T00:00:00Z","stages":[` +
	`{"id":"qualifiedtobuy","label":"Qualified","displayOrder":1,"archived":false,"metadata":{"isClosed":"false","probability":"0.4"}},` +
	`{"id":"123456","label":"Lost to competitor","displayOrder":2,"archived":false,"metadata":{"isClosed":"true","probability":"0.0"}},` +
	`{"id":"closedwon","label":"Closed won","displayOrder":3,"archived":false,"metadata":{"isClosed":"true","probability":"1.0"}}]}]}`

const tcDeals = `{"results":[` +
	`{"id":"1","properties":{"dealname":"Acme","amount":"10000","dealstage":"qualifiedtobuy","pipeline":"default","closedate":"2026-11-01T00:00:00Z","hubspot_owner_id":"7","notes_last_contacted":"2026-01-01T00:00:00Z","hs_is_closed":"false","hs_is_closed_won":"false"},"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-02T00:00:00Z","archived":false},` +
	`{"id":"2","properties":{"dealname":"Lost","amount":"5000","dealstage":"123456","pipeline":"default","closedate":"2026-11-01T00:00:00Z","hubspot_owner_id":"7","notes_last_contacted":"2026-01-01T00:00:00Z","hs_is_closed":"true","hs_is_closed_won":"false"},"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-02T00:00:00Z","archived":false}]}`

func newTwoclassServer(t *testing.T) *[]string {
	t.Helper()
	var reqs []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqs = append(reqs, r.Method+" "+r.URL.String())
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/crm/v3/pipelines/deals":
			_, _ = fmt.Fprint(w, tcPipelines)
		case "/crm/v3/objects/0-3":
			_, _ = fmt.Fprint(w, nbHonourProperties(r, tcDeals))
		default:
			_, _ = fmt.Fprint(w, `{"results":[]}`)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("HUBSPOT_BASE_URL", srv.URL)
	t.Setenv("HUBSPOT_ACCESS_TOKEN", "pat-na1-"+"7f3c9e2a-b14d-4c8e-9a6f-2d5e8b1c7a31")
	return &reqs
}

// The documented remedy, on a fresh --db store, makes every pipeline reader
// answer, and a deal in a custom closed stage is never counted open.
func TestTwoclassFreshInstallWorksAfterTheDocumentedSync(t *testing.T) {
	testenv.Isolate(t)
	reqs := newTwoclassServer(t)
	path := t.TempDir() + "/fresh.db"
	if _, _, err := r3dRun(t, "pipeline-health", "default", "--db", path, "--json"); err == nil {
		t.Fatal("pipeline-health answered on an empty store")
	}
	if _, stderr, err := r3dRun(t, "sync", "--resources", "hubspot-pipelines-crm,hubspot-deals-crm", "--db", path, "--json"); err != nil {
		t.Fatalf("the documented sync failed: %v\n%s", err, stderr)
	}
	sawProps := false
	for _, r := range *reqs {
		if strings.HasPrefix(r, "GET /crm/v3/objects/0-3") && strings.Contains(r, "hs_is_closed") {
			sawProps = true
		}
	}
	if !sawProps {
		t.Fatalf("deal sync did not request hs_is_closed: %v", *reqs)
	}
	out, stderr, err := r3dRun(t, "pipeline-health", "default", "--db", path, "--json")
	if err != nil {
		t.Fatalf("pipeline-health after the documented sync: %v\n%s", err, stderr)
	}
	if !strings.Contains(out, `"open_amount": 10000`) || strings.Contains(out, `"open_amount": 15000`) {
		t.Fatalf("open figures wrong (the custom closed stage must not count): %s", out)
	}
	if out, _, err := r3dRun(t, "deals", "forecast", "--db", path, "--json"); err != nil || !strings.Contains(out, `"total_weighted": 4000`) {
		t.Fatalf("deals forecast after the documented sync: err=%v out=%s", err, out)
	}
	for _, argv := range [][]string{
		{"deals", "top"}, {"owner-load"}, {"deals", "unowned"}, {"stale", "deals"}, {"deals", "velocity"}, {"nurture-mine", "--owner", "7"},
	} {
		if _, stderr, err := r3dRun(t, append(argv, "--db", path, "--json")...); err != nil {
			t.Errorf("%v after the documented sync: %v\n%s", argv, err, stderr)
		}
	}
}

// Pipelines a live read wrote through (the generic resources table) are found
// by pipeline-health too: every reader uses the one shared pipeline reader.
func TestTwoclassPipelineHealthReadsLiveReadPipelines(t *testing.T) {
	testenv.Isolate(t)
	newTwoclassServer(t)
	if _, stderr, err := r3dRun(t, "hubspot-pipelines-crm", "get-v3-pipelines-object-type-get-all", "deals", "--json"); err != nil {
		t.Fatalf("live pipelines read: %v\n%s", err, stderr)
	}
	if out, stderr, err := r3dRun(t, "pipeline-health", "default", "--json"); err != nil {
		t.Fatalf("pipeline-health after a live pipelines read: %v\n%s\n%s", err, stderr, out)
	}
}
