// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Regression tests for the 2026-10-24 Codex review round 1 P1s on local
// reports built from data that was never collected (stage probabilities,
// associations) and on live reads that stop after one page. Each test fails
// on the pre-fix code.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"hubspot-pp-cli/internal/cliutil/testenv"
	"hubspot-pp-cli/internal/store"
)

// r3dFetchPipelines is the remediation every unknown-probability error must
// name. Pinned as a literal: TestR3DataPipelineFetchCommandFillsTheStore
// proves it really fills the store the reports read.
const r3dFetchPipelines = "hubspot-cli sync --resources hubspot-pipelines-crm,hubspot-deals-crm"

// r3dRun runs the root command and returns stdout and stderr separately.
func r3dRun(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := RootCmd()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetIn(strings.NewReader(""))
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errb.String(), err
}

// r3dEnvelope decodes a report's JSON envelope.
func r3dEnvelope(t *testing.T, out string) map[string]any {
	t.Helper()
	var env map[string]any
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	return env
}

// r3dNotComputed returns the envelope's not_computed list; a missing field
// fails the test (the marker must always be present).
func r3dNotComputed(t *testing.T, env map[string]any) []string {
	t.Helper()
	raw, ok := env["not_computed"].([]any)
	if !ok {
		t.Fatalf("envelope has no not_computed list: %v", env)
	}
	if _, ok := env["warnings"].([]any); !ok {
		t.Fatalf("envelope has no warnings list: %v", env)
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		out = append(out, fmt.Sprint(v))
	}
	return out
}

func r3dContains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

// r3dCodexDeal is the Codex scenario: an open deal whose stage HubSpot
// weights at 0.4, on a store that never fetched pipelines.
func r3dCodexDeal(t *testing.T, s *store.Store) {
	r1Deal(t, s, "1", map[string]any{"dealname": "Acme", "amount": "10000", "dealstage": "qualifiedtobuy", "pipeline": "default", "closedate": "2026-11-01T00:00:00Z"})
}

func r3dWantUnknownStageError(t *testing.T, err error, out string) {
	t.Helper()
	if err == nil {
		t.Fatalf("an unknown stage probability was reported as a result: %s", out)
	}
	for _, want := range []string{"qualifiedtobuy", "deal 1", "not 0", r3dFetchPipelines} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not name %q", err, want)
		}
	}
}

// Class: an unknown stage probability is never 0. deals forecast.
func TestR3DataForecastRefusesUnknownStageProbability(t *testing.T) {
	testenv.Isolate(t)
	s, path := r1Store(t)
	r3dCodexDeal(t, s)
	s.Close()
	out, _, err := r3dRun(t, "deals", "forecast", "--db", path, "--json")
	r3dWantUnknownStageError(t, err, out)
	if !strings.Contains(err.Error(), "no deal pipelines are stored") {
		t.Fatalf("error %q does not say pipelines were never stored", err)
	}

	// Control: with the pipeline stored, the same deal weighs 0.4.
	s2, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	r1Pipeline(t, s2)
	s2.Close()
	out, _, err = r3dRun(t, "deals", "forecast", "--db", path, "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"total_weighted": 4000`) {
		t.Fatalf("total_weighted is not 10000 x 0.4: %s", out)
	}
}

// A stored pipeline that does not define the deal's stage, and a stage with
// no probability metadata, are both unknown, never 0.
func TestR3DataForecastRefusesStageMissingFromStoredPipelines(t *testing.T) {
	for name, stages := range map[string][]map[string]any{
		"stage-absent":      {{"id": "appointmentscheduled", "label": "Appt", "metadata": map[string]any{"isClosed": "false", "probability": "0.2"}}},
		"no-probability":    {{"id": "qualifiedtobuy", "label": "Qualified", "metadata": map[string]any{"isClosed": "false"}}},
		"unreadable":        {{"id": "qualifiedtobuy", "label": "Qualified", "metadata": map[string]any{"isClosed": "false", "probability": "forty"}}},
		"percent-not-ratio": {{"id": "qualifiedtobuy", "label": "Qualified", "metadata": map[string]any{"isClosed": "false", "probability": "40"}}},
	} {
		t.Run(name, func(t *testing.T) {
			testenv.Isolate(t)
			s, path := r1Store(t)
			nfStoreDealPipeline(t, s, map[string]any{"id": "default", "label": "Sales", "stages": stages})
			r3dCodexDeal(t, s)
			s.Close()
			out, _, err := r3dRun(t, "deals", "forecast", "--db", path, "--json")
			r3dWantUnknownStageError(t, err, out)
			if !strings.Contains(err.Error(), "stored pipeline(s) give no probability") {
				t.Fatalf("error %q does not say the stored pipelines lack the stage", err)
			}
		})
	}
}

// --month prices only the deals in the answer: an unknown stage in another
// month does not block it.
func TestR3DataForecastMonthPricesOnlyItsDeals(t *testing.T) {
	testenv.Isolate(t)
	s, path := r1Store(t)
	r1Pipeline(t, s)
	r1Deal(t, s, "1", map[string]any{"amount": "10000", "dealstage": "qualifiedtobuy", "pipeline": "default", "closedate": "2026-11-01T00:00:00Z"})
	r1Deal(t, s, "2", map[string]any{"amount": "500", "dealstage": "999", "pipeline": "default", "closedate": "2026-12-01T00:00:00Z"})
	s.Close()
	out, _, err := r3dRun(t, "deals", "forecast", "--month", "2026-11", "--db", path, "--json")
	if err != nil || !strings.Contains(out, `"total_weighted": 4000`) {
		t.Fatalf("err=%v out=%s", err, out)
	}
	if _, _, err := r3dRun(t, "deals", "forecast", "--db", path, "--json"); err == nil || !strings.Contains(err.Error(), `"999" (deal 2)`) {
		t.Fatalf("the full forecast must refuse deal 2's unknown stage: %v", err)
	}
}

// Class: an unknown stage probability is never 0. deals top.
func TestR3DataDealsTopRefusesUnknownStageProbability(t *testing.T) {
	testenv.Isolate(t)
	s, path := r1Store(t)
	r3dCodexDeal(t, s)
	s.Close()
	out, _, err := r3dRun(t, "deals", "top", "--db", path, "--json")
	r3dWantUnknownStageError(t, err, out)
}

// Class: an unknown stage probability is never 0.5 either. nurture queue.
func TestR3DataNurtureQueueRefusesUnknownStageProbability(t *testing.T) {
	testenv.Isolate(t)
	s, path := r1Store(t)
	createAssociations(t, s)
	r1Upsert(t, s.UpsertHubspotContactsCrm, map[string]any{"id": "9", "properties": map[string]any{"firstname": "Pat", "notes_last_contacted": "2026-01-01T00:00:00Z"}})
	r3dCodexDeal(t, s)
	seedAssociation(t, s, "contacts", "9", "deals", "1")
	s.Close()
	out, _, err := r3dRun(t, "nurture", "queue", "--owner", "", "--db", path, "--json")
	if err == nil {
		t.Fatalf("an open deal in an unknown stage was scored: %s", out)
	}
	for _, want := range []string{"qualifiedtobuy", "contact 9", r3dFetchPipelines} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not name %q", err, want)
		}
	}

	// Control: with the pipeline stored the queue scores the deal and its
	// marker says the deal ranking was computed.
	s2, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	r1Pipeline(t, s2)
	s2.Close()
	out, _, err = r3dRun(t, "nurture", "queue", "--owner", "", "--db", path, "--json")
	if err != nil {
		t.Fatal(err)
	}
	if nc := r3dNotComputed(t, r3dEnvelope(t, out)); len(nc) != 0 {
		t.Fatalf("not_computed = %v with associations present", nc)
	}
	if !strings.Contains(out, `"top_deal_stage": "qualifiedtobuy"`) {
		t.Fatalf("the open deal is missing from the queue: %s", out)
	}
}

// The remediation the error names really fills the store the default
// commands read: the live pipelines read writes through.
func TestR3DataPipelineFetchCommandFillsTheStore(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/crm/v3/objects/0-3" {
			_, _ = fmt.Fprint(w, `{"results":[{"id":"1","properties":{"dealname":"Acme","amount":"10000","dealstage":"qualifiedtobuy","pipeline":"default","closedate":"2026-11-01T00:00:00Z","hs_is_closed":"false","hs_is_closed_won":"false"},"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-10-01T00:00:00Z","archived":false}]}`)
			return
		}
		if r.URL.Path != "/crm/v3/pipelines/deals" {
			http.NotFound(w, r)
			return
		}
		_, _ = fmt.Fprint(w, `{"results":[{"id":"default","label":"Sales Pipeline","displayOrder":0,"archived":false,"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-01T00:00:00Z",`+
			`"stages":[{"id":"qualifiedtobuy","label":"Qualified","displayOrder":1,"archived":false,"metadata":{"isClosed":"false","probability":"0.4"}}]}]}`)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("HUBSPOT_BASE_URL", srv.URL)
	t.Setenv("HUBSPOT_ACCESS_TOKEN", "pat-na1-test-token")
	s, err := store.Open(defaultDBPath("hubspot-cli"))
	if err != nil {
		t.Fatal(err)
	}
	r3dCodexDeal(t, s)
	s.Close()
	if _, _, err := r3dRun(t, "deals", "forecast", "--json"); err == nil {
		t.Fatal("forecast answered before the pipelines were fetched")
	}
	args := append(strings.Fields(strings.TrimPrefix(r3dFetchPipelines, "hubspot-cli ")), "--json")
	if _, _, err := r3dRun(t, args...); err != nil {
		t.Fatal(err)
	}
	out, _, err := r3dRun(t, "deals", "forecast", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"total_weighted": 4000`) {
		t.Fatalf("after the named fetch the forecast is not 4000: %s", out)
	}
}

// Class: a report whose whole answer is an association join refuses.
// contacts win-back, the Codex scenario (associations embedded in the
// contact's data are not ingested by this build).
func TestR3DataWinBackRefusesWithoutAssociations(t *testing.T) {
	testenv.Isolate(t)
	s, path := r1Store(t)
	r1Upsert(t, s.UpsertHubspotContactsCrm, map[string]any{"id": "1", "properties": map[string]any{"notes_last_contacted": "2026-01-01T00:00:00Z"},
		"associations": map[string]any{"deals": map[string]any{"results": []map[string]any{{"id": "2", "type": "contact_to_deal"}}}}})
	r1Deal(t, s, "2", map[string]any{"dealname": "Won", "amount": "9000", "dealstage": "closedwon"})
	s.Close()
	out, _, err := r3dRun(t, "contacts", "win-back", "--cold-days", "90", "--db", path, "--json")
	if err == nil {
		t.Fatalf("win-back answered without associations: %s", out)
	}
	if !strings.Contains(err.Error(), "contact-to-deal associations") || !strings.Contains(err.Error(), "does not collect") {
		t.Fatalf("error %q does not say associations are not collected", err)
	}
}

// Class: a report that uses associations for one signal marks the part it
// could not compute (envelope + stderr). deals top.
func TestR3DataDealsTopMarksMissingNoteSignals(t *testing.T) {
	testenv.Isolate(t)
	s, path := r1Store(t)
	r1Pipeline(t, s)
	r3dCodexDeal(t, s)
	s.Close()
	out, stderr, err := r3dRun(t, "deals", "top", "--db", path, "--json")
	if err != nil {
		t.Fatal(err)
	}
	if nc := r3dNotComputed(t, r3dEnvelope(t, out)); !r3dContains(nc, "note_signals") {
		t.Fatalf("not_computed = %v, want note_signals: %s", nc, out)
	}
	if !strings.Contains(stderr, "warning: note_signals not computed") {
		t.Fatalf("stderr has no warning: %s", stderr)
	}

	// Control: with associations present nothing is marked.
	s2, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	createAssociations(t, s2)
	seedAssociation(t, s2, "notes", "n1", "deals", "1")
	s2.Close()
	out, stderr, err = r3dRun(t, "deals", "top", "--db", path, "--json")
	if err != nil {
		t.Fatal(err)
	}
	if nc := r3dNotComputed(t, r3dEnvelope(t, out)); len(nc) != 0 || strings.Contains(stderr, "not computed") {
		t.Fatalf("not_computed = %v stderr=%s with associations present", nc, stderr)
	}
}

// nurture-mine without associations: deal fields are unknown (null), never
// zero, and the open-deal filter is marked as not applied.
func TestR3DataNurtureMineMarksUnknownDeals(t *testing.T) {
	testenv.Isolate(t)
	s, path := r1Store(t)
	r1Upsert(t, s.UpsertHubspotContactsCrm, map[string]any{"id": "9", "properties": map[string]any{"firstname": "Pat", "notes_last_contacted": "2026-01-01T00:00:00Z"}})
	s.Close()
	out, stderr, err := r3dRun(t, "nurture-mine", "--owner", "", "--db", path, "--json")
	if err != nil {
		t.Fatal(err)
	}
	env := r3dEnvelope(t, out)
	if nc := r3dNotComputed(t, env); !r3dContains(nc, "open_deals") {
		t.Fatalf("not_computed = %v, want open_deals: %s", nc, out)
	}
	if !strings.Contains(stderr, "warning: open_deals not computed") {
		t.Fatalf("stderr has no warning: %s", stderr)
	}
	rows, _ := env["results"].([]any)
	if len(rows) != 1 {
		t.Fatalf("results = %v", env["results"])
	}
	row, _ := rows[0].(map[string]any)
	for _, k := range []string{"deal_count", "latest_deal_stage", "top_deal_amount"} {
		v, ok := row[k]
		if !ok || v != nil {
			t.Fatalf("%s = %v (present=%v), want null (unknown, not zero): %s", k, v, ok, out)
		}
	}
}

// nurture queue without associations: the deal ranking is marked.
func TestR3DataNurtureQueueMarksMissingDealRanking(t *testing.T) {
	testenv.Isolate(t)
	s, path := r1Store(t)
	r1Upsert(t, s.UpsertHubspotContactsCrm, map[string]any{"id": "9", "properties": map[string]any{"firstname": "Pat", "notes_last_contacted": "2026-01-01T00:00:00Z"}})
	s.Close()
	out, stderr, err := r3dRun(t, "nurture", "queue", "--owner", "", "--db", path, "--json")
	if err != nil {
		t.Fatal(err)
	}
	if nc := r3dNotComputed(t, r3dEnvelope(t, out)); !r3dContains(nc, "deal_ranking") {
		t.Fatalf("not_computed = %v, want deal_ranking: %s", nc, out)
	}
	if !strings.Contains(stderr, "warning: deal_ranking not computed") {
		t.Fatalf("stderr has no warning: %s", stderr)
	}
	if !strings.Contains(out, `"top_deal_amount": null`) {
		t.Fatalf("top_deal_amount is not null: %s", out)
	}
}

// notes signals without associations: per-note rows are marked, and
// --pipeline (which needs associations) refuses instead of returning rows
// from every pipeline.
func TestR3DataNotesSignalsMarksMissingDealGrouping(t *testing.T) {
	testenv.Isolate(t)
	s, path := r1Store(t)
	r1Upsert(t, s.UpsertHubspotNotesCrm, map[string]any{"id": "201", "properties": map[string]any{"hs_note_body": "budget approved"}, "createdAt": "2026-10-01T00:00:00Z", "updatedAt": "2026-10-24T14:00:00Z"})
	s.Close()
	out, stderr, err := r3dRun(t, "notes", "signals", "--db", path, "--json")
	if err != nil {
		t.Fatal(err)
	}
	if nc := r3dNotComputed(t, r3dEnvelope(t, out)); !r3dContains(nc, "deal_grouping") {
		t.Fatalf("not_computed = %v, want deal_grouping: %s", nc, out)
	}
	if !strings.Contains(stderr, "warning: deal_grouping not computed") {
		t.Fatalf("stderr has no warning: %s", stderr)
	}
	out, _, err = r3dRun(t, "notes", "signals", "--pipeline", "default", "--db", path, "--json")
	if err == nil {
		t.Fatalf("--pipeline returned unfiltered rows: %s", out)
	}
	if !strings.Contains(err.Error(), "note-to-deal associations") {
		t.Fatalf("error %q does not name associations", err)
	}
}

// engagements of --data-source local without associations refuses rather
// than reporting an empty timeline, and still never goes live.
func TestR3DataEngagementsLocalRefusesWithoutAssociations(t *testing.T) {
	testenv.Isolate(t)
	rs := newR1Server(t, 200, `{"results":[]}`)
	s, path := r1Store(t)
	r1Upsert(t, s.UpsertHubspotCallsCrm, map[string]any{"id": "101", "properties": map[string]any{"hs_call_title": "Kickoff"}, "updatedAt": "2026-10-24T14:00:00Z"})
	s.Close()
	out, _, err := r3dRun(t, "engagements", "of", "contact:9", "--data-source", "local", "--db", path, "--json")
	if err == nil {
		t.Fatalf("an empty local timeline was reported without associations: %s", out)
	}
	if !strings.Contains(err.Error(), "engagement associations") {
		t.Fatalf("error %q does not name associations", err)
	}
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if len(rs.reqs) != 0 {
		t.Fatalf("--data-source local made live requests: %v", rs.reqs)
	}
}

// r3dPagingServer serves a contact's call associations across pages and each
// call's record. fail names request paths (path + "?after=" cursor for list
// pages) that answer 404.
type r3dPagingServer struct {
	mu    sync.Mutex
	reqs  []string
	pages [][]string // association ids per page; page i+1 is reached by after=strconv.Itoa(i+1)
	// endless makes every list page hand out a fresh cursor.
	endless bool
	stuck   bool // the next cursor never advances
	fail    map[string]bool
	updated map[string]string
}

func newR3dPagingServer(t *testing.T, ps *r3dPagingServer) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ps.mu.Lock()
		ps.reqs = append(ps.reqs, r.URL.String())
		ps.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		after := r.URL.Query().Get("after")
		if ps.fail[r.URL.Path] || ps.fail[r.URL.Path+"?after="+after] {
			w.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprint(w, `{"status":"error","message":"not found","category":"OBJECT_NOT_FOUND"}`)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/crm/v4/objects/contacts/1/associations/") {
			idx := 0
			if after != "" {
				n, err := strconv.Atoi(after)
				if err != nil {
					http.Error(w, "bad cursor", http.StatusBadRequest)
					return
				}
				idx = n
			}
			var ids []string
			if !strings.HasSuffix(r.URL.Path, "/calls") {
				ids = nil
			} else if idx < len(ps.pages) {
				ids = ps.pages[idx]
			}
			results := make([]string, 0, len(ids))
			for _, id := range ids {
				// toObjectId is an integer in the v4 schema.
				results = append(results, `{"toObjectId":`+id+`,"associationTypes":[{"category":"HUBSPOT_DEFINED","typeId":194,"label":null}]}`)
			}
			paging := ""
			switch {
			case ps.stuck:
				paging = `,"paging":{"next":{"after":"7","link":"x"}}`
			case ps.endless || idx+1 < len(ps.pages):
				paging = fmt.Sprintf(`,"paging":{"next":{"after":"%d","link":"x"}}`, idx+1)
			}
			_, _ = fmt.Fprintf(w, `{"results":[%s]%s}`, strings.Join(results, ","), paging)
			return
		}
		if id, ok := strings.CutPrefix(r.URL.Path, "/crm/v3/objects/calls/"); ok {
			ts := ps.updated[id]
			if ts == "" {
				ts = "2026-09-01T00:00:00Z"
			}
			_, _ = fmt.Fprintf(w, `{"id":%q,"createdAt":"2026-01-01T00:00:00Z","updatedAt":%q,"properties":{"hs_call_title":"Call %s"}}`, id, ts, id)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("HUBSPOT_BASE_URL", srv.URL)
	t.Setenv("HUBSPOT_ACCESS_TOKEN", "pat-na1-test-token")
}

func r3dCodexPages() [][]string {
	first := make([]string, 0, 500)
	for i := 1; i <= 500; i++ {
		first = append(first, strconv.Itoa(i))
	}
	return [][]string{first, {"501", "51234567890"}}
}

func r3dRunTimeline(t *testing.T, path string, extra ...string) (string, error) {
	t.Helper()
	args := append([]string{"engagements", "of", "contact:1", "--since", "2026-10-01T00:00:00Z", "--db", path, "--json", "--rate-limit", "0"}, extra...)
	out, _, err := r3dRun(t, args...)
	return out, err
}

// Class: a live read that stops after one page. The Codex scenario: page
// one holds 500 older calls, page two the recent call 501 (and an 11-digit
// id that a float64 decode would render as 5.123456789e+10).
func TestR3DataEngagementsLiveWalksEveryAssociationPage(t *testing.T) {
	testenv.Isolate(t)
	ps := &r3dPagingServer{pages: r3dCodexPages(), updated: map[string]string{"501": "2026-10-24T00:00:00Z", "51234567890": "2026-10-24T01:00:00Z"}}
	newR3dPagingServer(t, ps)
	_, path := r1Store(t)
	out, err := r3dRunTimeline(t, path, "--type", "calls")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"id": "501"`) || !strings.Contains(out, `"id": "51234567890"`) {
		t.Fatalf("the recent calls on page two are missing: %s", out)
	}
	if strings.Contains(out, `"id": "1"`) {
		t.Fatalf("an older call passed --since: %s", out)
	}
	ps.mu.Lock()
	defer ps.mu.Unlock()
	sawLimit := false
	for _, r := range ps.reqs {
		if strings.Contains(r, "/associations/calls") && strings.Contains(r, "limit=500") {
			sawLimit = true
		}
	}
	if !sawLimit {
		t.Fatalf("the association list did not ask for the documented 500 per page: %v", ps.reqs[:2])
	}
}

// Any page, record or per-type failure fails the timeline: never a
// successful partial one.
func TestR3DataEngagementsLiveFailsOnAnyReadFailure(t *testing.T) {
	for name, fail := range map[string]map[string]bool{
		"second-page": {"/crm/v4/objects/contacts/1/associations/calls?after=1": true},
		"record":      {"/crm/v3/objects/calls/1": true},
		"other-type":  {"/crm/v4/objects/contacts/1/associations/emails": true},
	} {
		t.Run(name, func(t *testing.T) {
			testenv.Isolate(t)
			ps := &r3dPagingServer{pages: [][]string{{"1"}, {"501"}}, fail: fail, updated: map[string]string{"1": "2026-10-24T00:00:00Z", "501": "2026-10-24T00:00:00Z"}}
			newR3dPagingServer(t, ps)
			_, path := r1Store(t)
			out, err := r3dRunTimeline(t, path, "--type", "calls,emails")
			if err == nil {
				t.Fatalf("a failed read produced a successful timeline: %s", out)
			}
		})
	}
}

// A walk that never ends, or a cursor that does not advance, fails instead
// of truncating.
func TestR3DataEngagementsLivePagingCapFails(t *testing.T) {
	for name, ps := range map[string]*r3dPagingServer{
		"endless": {pages: [][]string{{"1"}}, endless: true},
		"stuck":   {pages: [][]string{{"1"}}, stuck: true},
	} {
		t.Run(name, func(t *testing.T) {
			testenv.Isolate(t)
			ps.updated = map[string]string{"1": "2026-10-24T00:00:00Z"}
			newR3dPagingServer(t, ps)
			_, path := r1Store(t)
			out, err := r3dRunTimeline(t, path, "--type", "calls")
			if err == nil || !strings.Contains(err.Error(), "partial timeline") {
				t.Fatalf("err=%v: an unbounded association walk must fail, not truncate: %s", err, out)
			}
		})
	}
}

// An unknown --type is refused: a skipped type would make an empty timeline
// look complete.
func TestR3DataEngagementsUnknownTypeRefused(t *testing.T) {
	testenv.Isolate(t)
	ps := &r3dPagingServer{pages: [][]string{{"501"}}, updated: map[string]string{"501": "2026-10-24T00:00:00Z"}}
	newR3dPagingServer(t, ps)
	_, path := r1Store(t)
	for _, typ := range []string{"call", ","} {
		out, err := r3dRunTimeline(t, path, "--type", typ)
		if err == nil {
			t.Fatalf("--type %q produced a timeline: %s", typ, out)
		}
	}
}
