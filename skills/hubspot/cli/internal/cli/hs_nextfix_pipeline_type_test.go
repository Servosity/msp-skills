// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Nextfix (2026-10-27, handfixes.json pipeline-object-type-declared): every
// object type's pipelines share one stored resource, so a deal reader takes
// only pipelines declared deals by the request path that fetched them.
// Ticket pipelines (GET /crm/v3/pipelines/tickets, press/tools-manifest.json)
// and pipelines stored from a write response never answer a deal report.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hubspot-pp-cli/internal/cliutil/testenv"
	"hubspot-pp-cli/internal/store"
)

// nfStoreDealPipeline stores one deal pipeline the way sync does: the typed
// upsert, then the object type sync's deal pipelines path declares.
func nfStoreDealPipeline(t *testing.T, s *store.Store, v any) {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertHubspotPipelinesCrm(raw); err != nil {
		t.Fatalf("upsert %s: %v", raw, err)
	}
	if err := hsDeclarePipelineObjectType(s, "hubspot-pipelines-crm", "/crm/v3/pipelines/deals", []json.RawMessage{raw}); err != nil {
		t.Fatalf("declare %s: %v", raw, err)
	}
}

// A HubSpot ticket pipeline: stages carry ticketState. The default one is id
// "0" (which the store does not cache); a custom one has a numeric id.
const nfTicketPipeline = `{"id":"0","label":"Support Pipeline","displayOrder":0,"archived":false,"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-01T00:00:00Z","stages":[` +
	`{"id":"1","label":"New","displayOrder":0,"archived":false,"metadata":{"ticketState":"OPEN","isClosed":"false"}},` +
	`{"id":"4","label":"Closed","displayOrder":3,"archived":false,"metadata":{"ticketState":"CLOSED","isClosed":"true"}}]}`

func newNextfixServer(t *testing.T) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/crm/v3/pipelines/tickets":
			w.WriteHeader(http.StatusCreated)
			_, _ = fmt.Fprint(w, strings.Replace(nfTicketPipeline, `"id":"0","label":"Support Pipeline"`, `"id":"77","label":"Escalations"`, 1))
		case r.URL.Path == "/crm/v3/pipelines/tickets":
			custom := strings.Replace(nfTicketPipeline, `"id":"0","label":"Support Pipeline"`, `"id":"88123","label":"Onboarding"`, 1)
			_, _ = fmt.Fprint(w, `{"results":[`+nfTicketPipeline+`,`+custom+`]}`)
		case r.URL.Path == "/crm/v3/pipelines/deals":
			_, _ = fmt.Fprint(w, tcPipelines)
		case r.URL.Path == "/crm/v3/objects/0-3":
			_, _ = fmt.Fprint(w, nbHonourProperties(r, tcDeals))
		default:
			_, _ = fmt.Fprint(w, `{"results":[]}`)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("HUBSPOT_BASE_URL", srv.URL)
	t.Setenv("HUBSPOT_ACCESS_TOKEN", "pat-na1-"+"7f3c9e2a-b14d-4c8e-9a6f-2d5e8b1c7a31")
}

func TestNextfixDealReadersTakeOnlyDealPipelines(t *testing.T) {
	testenv.Isolate(t)
	newNextfixServer(t)
	if _, stderr, err := r3dRun(t, "sync", "--resources", "hubspot-deals-crm", "--json"); err != nil {
		t.Fatalf("deal sync: %v\n%s", err, stderr)
	}
	refuses := func(when string) {
		t.Helper()
		for _, arg := range []string{"default", "88123", "Onboarding", "77", "Escalations"} {
			out, stderr, err := r3dRun(t, "pipeline-health", arg, "--json")
			if err == nil {
				t.Errorf("%s: pipeline-health %q answered from a non-deal pipeline: %s", when, arg, out)
			} else if !strings.Contains(err.Error()+stderr, hsDealPipelinesSyncCmd) {
				t.Errorf("%s: pipeline-health %q refusal does not name the remedy: %v\n%s", when, arg, err, stderr)
			}
		}
		if out, _, err := r3dRun(t, "deals", "forecast", "--json"); err == nil {
			t.Errorf("%s: deals forecast answered from a non-deal pipeline: %s", when, out)
		}
	}
	// A ticket pipeline a live list read stored.
	if _, stderr, err := r3dRun(t, "hubspot-pipelines-crm", "get-v3-pipelines-object-type-get-all", "tickets", "--json"); err != nil {
		t.Fatalf("live ticket pipelines read: %v\n%s", err, stderr)
	}
	refuses("after a ticket pipelines read")
	// A ticket pipeline stored from a create response (no list path, so undeclared).
	if _, stderr, err := r3dRun(t, "hubspot-pipelines-crm", "post-v3-pipelines-object-type-create", "tickets", "--label", "Escalations", "--display-order", "1", "--stages", "[]", "--yes", "--json"); err != nil {
		t.Fatalf("ticket pipeline create: %v\n%s", err, stderr)
	}
	refuses("after a ticket pipeline create")

	// Kept signal: a live read of the deal pipelines answers, with the custom
	// closed stage still closed.
	if _, stderr, err := r3dRun(t, "hubspot-pipelines-crm", "get-v3-pipelines-object-type-get-all", "deals", "--json"); err != nil {
		t.Fatalf("live deal pipelines read: %v\n%s", err, stderr)
	}
	out, stderr, err := r3dRun(t, "pipeline-health", "default", "--json")
	if err != nil || !strings.Contains(out, `"open_amount": 10000`) {
		t.Fatalf("pipeline-health after a deal pipelines read: err=%v out=%s\n%s", err, out, stderr)
	}
	if _, _, err := r3dRun(t, "pipeline-health", "88123", "--json"); err == nil {
		t.Error("pipeline-health took the ticket pipeline by id beside a deal pipeline")
	}
}

// A store an older binary migrated has no hubspot_pipeline_object_types; a
// report that opens the store read-only takes the migrate path instead of
// failing on the missing table.
func TestNextfixOldStoreGetsTheObjectTypeTable(t *testing.T) {
	testenv.Isolate(t)
	newNextfixServer(t)
	path := t.TempDir() + "/old.db"
	if _, stderr, err := r3dRun(t, "sync", "--resources", "hubspot-deals-crm", "--db", path, "--json"); err != nil {
		t.Fatalf("deal sync: %v\n%s", err, stderr)
	}
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`DROP TABLE hubspot_pipeline_object_types`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	if out, stderr, err := r3dRun(t, "owner-load", "--db", path, "--json"); err != nil {
		t.Fatalf("owner-load on a store without the object type table: %v\n%s\n%s", err, stderr, out)
	}
}

func TestNextfixOnlyAPipelinesListPathDeclares(t *testing.T) {
	for path, want := range map[string]string{
		"/crm/v3/pipelines/deals":             "deals",
		"/crm/v3/pipelines/Deals":             "deals",
		"/crm/v3/pipelines/tickets":           "tickets",
		"/crm/v3/pipelines/deals/1":           "",
		"/crm/v3/pipelines/deals/1/stages":    "",
		"/crm/v3/pipelines/deals/1/audit":     "",
		"/crm/v3/pipelines/":                  "",
		"/crm/v3/objects/0-3":                 "",
		"/crm/v3/pipelines/deals?archived=1":  "",
		"/crm/v3/pipelines/deals%2F1%2Fstage": "deals%2f1%2fstage",
	} {
		got, ok := hsPipelineListObjectType(path)
		if got != want || ok != (want != "") {
			t.Errorf("%s: got %q %v, want %q", path, got, ok, want)
		}
	}
}
