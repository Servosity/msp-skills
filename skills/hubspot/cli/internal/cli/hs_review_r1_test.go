// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Regression tests for the 2026-10-24 Codex review (round 1) of the hand
// layer. Each test seeds the store through the production upsert path (the
// data column is written as TEXT) and fails on the pre-fix code.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"hubspot-pp-cli/internal/cliutil/testenv"
	"hubspot-pp-cli/internal/store"
)

func r1Store(t *testing.T) (*store.Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "data.db")
	s, err := store.Open(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s, path
}

func r1Upsert(t *testing.T, fn func(json.RawMessage) error, v any) {
	t.Helper()
	if m, ok := v.(map[string]any); ok {
		if props, ok := m["properties"].(map[string]any); ok {
			nbAsSynced(props)
		}
	}
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if err := fn(raw); err != nil {
		t.Fatalf("upsert %s: %v", raw, err)
	}
}

// r1Pipeline is HubSpot's documented PipelineStage shape: stages carry "id",
// and metadata values are strings.
func r1Pipeline(t *testing.T, s *store.Store) {
	nfStoreDealPipeline(t, s, map[string]any{
		"id": "default", "label": "Sales Pipeline", "displayOrder": 0, "archived": false,
		"stages": []map[string]any{
			{"id": "qualifiedtobuy", "label": "Qualified", "displayOrder": 1, "metadata": map[string]any{"isClosed": "false", "probability": "0.4"}},
			{"id": "12345", "label": "Signed", "displayOrder": 2, "metadata": map[string]any{"isClosed": "true", "probability": "1.0"}},
			{"id": "67890", "label": "Gone", "displayOrder": 3, "metadata": map[string]any{"isClosed": "true", "probability": "0.0"}},
		},
	})
}

func r1Deal(t *testing.T, s *store.Store, id string, props map[string]any) {
	r1Upsert(t, s.UpsertHubspotDealsCrm, map[string]any{
		"id": id, "archived": false, "properties": props,
		"createdAt": "2026-01-01T00:00:00Z", "updatedAt": "2026-10-01T00:00:00Z",
	})
}

func r1Run(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	cmd := RootCmd()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs(args)
	err := cmd.Execute()
	if err != nil {
		t.Logf("stderr: %s", errb.String())
	}
	return out.String(), err
}

// Class: pipeline stages decoded from a non-documented key (stageId).
func TestR1PipelineStagesDecodeTheDocumentedID(t *testing.T) {
	s, _ := r1Store(t)
	r1Pipeline(t, s)
	_, stages, err := resolvePipeline(s, "default")
	if err != nil {
		t.Fatal(err)
	}
	if len(stages) != 3 || stages[0].StageID != "qualifiedtobuy" {
		t.Fatalf("stages = %+v, want ids from PipelineStage.id", stages)
	}
	probs, err := loadAllStageProbabilities(s)
	if err != nil {
		t.Fatal(err)
	}
	if probs["qualifiedtobuy"] != 0.4 {
		t.Fatalf("probabilities = %v, want qualifiedtobuy=0.4", probs)
	}
}

// Class: store rows scanned into json.RawMessage fail on TEXT and the error
// is swallowed, so a forecast reads as a valid zero.
func TestR1ForecastReadsPipelinesStoredAsText(t *testing.T) {
	s, _ := r1Store(t)
	r1Pipeline(t, s)
	r1Deal(t, s, "1", map[string]any{"dealstage": "qualifiedtobuy", "amount": "10000", "pipeline": "default", "closedate": "2026-11-01T00:00:00Z"})
	cmd := newNovelDealsForecastCmd(&rootFlags{})
	cmd.SetContext(context.Background())
	view, err := queryDealsForecast(cmd, s, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if view.TotalWeighted != 4000 {
		t.Fatalf("total_weighted = %v, want 4000", view.TotalWeighted)
	}
}

func TestR1PropertySchemaReadsTextRows(t *testing.T) {
	s, _ := r1Store(t)
	r1Upsert(t, s.UpsertHubspotPropertiesCrm, map[string]any{"name": "firstname", "type": "string", "objectType": "contacts"})
	schema, err := loadContactsPropertySchema(s)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := schema["firstname"]; !ok {
		t.Fatalf("schema = %v, want firstname", schema)
	}
}

// Class: closed/won decided by HubSpot's default stage ids only.
func TestR1CustomClosedStagesAreNotOpenLoad(t *testing.T) {
	testenv.Isolate(t)
	s, path := r1Store(t)
	r1Pipeline(t, s)
	r1Deal(t, s, "1", map[string]any{"dealstage": "12345", "hubspot_owner_id": "7", "amount": "9000", "pipeline": "default"})
	r1Deal(t, s, "2", map[string]any{"dealstage": "qualifiedtobuy", "hubspot_owner_id": "7", "amount": "500", "pipeline": "default", "hs_is_closed": "true"})
	r1Deal(t, s, "3", map[string]any{"dealstage": "qualifiedtobuy", "hubspot_owner_id": "7", "amount": "100", "pipeline": "default"})
	s.Close()
	out, err := r1Run(t, "", "owner-load", "--db", path, "--json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "9000") || strings.Contains(out, "12345") || strings.Contains(out, "600") {
		t.Fatalf("closed deals counted as open load: %s", out)
	}
	if !strings.Contains(out, "qualifiedtobuy") {
		t.Fatalf("open deal missing: %s", out)
	}
	s2, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	fc := newNovelDealsForecastCmd(&rootFlags{})
	fc.SetContext(context.Background())
	view, err := queryDealsForecast(fc, s2, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if view.Undated.DealCount != 1 {
		t.Fatalf("forecast counts %d deals, want only the open deal: %+v", view.Undated.DealCount, view)
	}
}

// Class: relative cutoffs rendered in local time with a literal Z, and
// timestamps compared as strings.
func TestR1SinceCutoffIsUTC(t *testing.T) {
	prev := time.Local
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip(err)
	}
	time.Local = ny
	t.Cleanup(func() { time.Local = prev })
	got, err := parseDurationOrTimestamp("1h")
	if err != nil {
		t.Fatal(err)
	}
	ts, err := time.Parse(time.RFC3339, got)
	if err != nil {
		t.Fatalf("cutoff %q is not RFC3339: %v", got, err)
	}
	if d := time.Since(ts); d < 59*time.Minute || d > 61*time.Minute {
		t.Fatalf("cutoff %q is %v ago, want 1h", got, d)
	}
	if _, err := parseDurationOrTimestamp("yesterday-ish"); err == nil {
		t.Fatal("an unparseable cutoff must be refused, not compared as a string")
	}
}

func TestR1SinceComparesChronologically(t *testing.T) {
	testenv.Isolate(t)
	s, path := r1Store(t)
	r1Upsert(t, s.UpsertHubspotDealsCrm, map[string]any{"id": "1", "properties": map[string]any{"dealname": "A"}, "updatedAt": "2026-10-24T14:00:00.500Z"})
	s.Close()
	out, err := r1Run(t, "", "since", "2026-10-24T14:00:00Z", "--types", "deals", "--db", path, "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"1"`) {
		t.Fatalf("a deal updated 0.5s after the cutoff is missing: %s", out)
	}
}

// Class: truncation (LIMIT / candidate cap) applied before --filter.
func TestR1StaleFiltersBeforeLimit(t *testing.T) {
	testenv.Isolate(t)
	s, path := r1Store(t)
	for i := 1; i <= 20; i++ {
		r1Upsert(t, s.UpsertHubspotContactsCrm, map[string]any{"id": fmt.Sprintf("%03d", i), "properties": map[string]any{"lifecyclestage": "lead", "notes_last_contacted": "2026-01-01T00:00:00Z"}})
	}
	r1Upsert(t, s.UpsertHubspotContactsCrm, map[string]any{"id": "021", "properties": map[string]any{"lifecyclestage": "customer", "notes_last_contacted": "2026-02-01T00:00:00Z"}})
	s.Close()
	out, err := r1Run(t, "", "stale", "contacts", "--days", "30", "--limit", "1", "--filter", "lifecyclestage=customer", "--db", path, "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"021"`) {
		t.Fatalf("the only matching contact was truncated away: %s", out)
	}
}

// r1Server records every request and answers batch writes with body.
type r1Server struct {
	mu   sync.Mutex
	reqs []string
	srv  *httptest.Server
}

func newR1Server(t *testing.T, status int, body string) *r1Server {
	rs := &r1Server{}
	rs.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		rs.mu.Lock()
		rs.reqs = append(rs.reqs, r.Method+" "+r.URL.String()+" "+string(b))
		rs.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(rs.srv.Close)
	t.Setenv("HUBSPOT_BASE_URL", rs.srv.URL)
	t.Setenv("HUBSPOT_ACCESS_TOKEN", "pat-na1-test-token")
	return rs
}

func (rs *r1Server) writes() int {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	n := 0
	for _, r := range rs.reqs {
		if strings.HasPrefix(r, "POST ") {
			n++
		}
	}
	return n
}

func r1BulkStore(t *testing.T) string {
	s, path := r1Store(t)
	r1Upsert(t, s.UpsertHubspotPropertiesCrm, map[string]any{"name": "firstname", "type": "string", "objectType": "contacts"})
	// The pre-fix loader scanned json.RawMessage, which only a BLOB satisfies.
	if _, err := s.DB().Exec(`UPDATE hubspot_properties_crm SET data = CAST(data AS BLOB)`); err != nil {
		t.Fatal(err)
	}
	s.Close()
	return path
}

// Class: a live write without confirmation.
func TestR1BulkUpdateRefusesAnUnconfirmedWrite(t *testing.T) {
	testenv.Isolate(t)
	rs := newR1Server(t, 200, `{"status":"COMPLETE","results":[{"id":"1","properties":{"firstname":"X"}}]}`)
	path := r1BulkStore(t)
	_, err := r1Run(t, `{"id":"1","firstname":"X"}`+"\n", "contacts", "bulk-update", "--jsonl", "--db", path)
	if err == nil || rs.writes() != 0 {
		t.Fatalf("err=%v writes=%d: a live bulk update must need --yes (or the digest confirm)", err, rs.writes())
	}
	if _, err := r1Run(t, `{"id":"1","firstname":"X"}`+"\n", "contacts", "bulk-update", "--jsonl", "--yes", "--db", path); err != nil || rs.writes() != 1 {
		t.Fatalf("err=%v writes=%d: --yes must send the confirmed write", err, rs.writes())
	}
}

// Class: batch acceptance (PENDING) or an uncorrelated result counted as a
// completed update.
func TestR1BulkUpdateCountsOnlyConfirmedRows(t *testing.T) {
	for name, body := range map[string]string{
		"pending":      `{"status":"PENDING","results":[]}`,
		"other-record": `{"status":"COMPLETE","results":[{"id":"999","properties":{}}]}`,
		"context-ids":  `{"status":"COMPLETE","results":[],"numErrors":1,"errors":[{"status":"error","category":"OBJECT_NOT_FOUND","message":"not found","context":{"ids":["1"]}}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			testenv.Isolate(t)
			newR1Server(t, 200, body)
			path := r1BulkStore(t)
			out, err := r1Run(t, `{"id":"1","firstname":"X"}`+"\n", "contacts", "bulk-update", "--jsonl", "--yes", "--db", path)
			if err == nil {
				t.Fatalf("an unconfirmed row must fail the run; out=%s", out)
			}
			if strings.Contains(out, `"ok":true`) {
				t.Fatalf("row reported ok: %s", out)
			}
		})
	}
}

// Class: --with-history narrows the regular snapshot to the history
// properties (HubSpot returns only the properties= list when it is set).
func TestR1WithHistoryKeepsTheDefaultSnapshot(t *testing.T) {
	testenv.Isolate(t)
	rs := newR1Server(t, 200, `{"results":[{"id":"1","properties":{"dealname":"A","amount":"10000","dealstage":"qualifiedtobuy"},`+
		`"propertiesWithHistory":{"hubspot_owner_id":[{"value":"7","timestamp":"2026-10-01T00:00:00.000Z","sourceType":"CRM_UI"},{"value":"3","timestamp":"2026-09-01T00:00:00.000Z","sourceType":"CRM_UI"}],`+
		`"dealstage":[{"value":"qualifiedtobuy","timestamp":"2026-10-02T00:00:00.000Z","sourceType":"CRM_UI"}]}}]}`)
	_, path := r1Store(t)
	if _, err := r1Run(t, "", "sync", "--resources", "hubspot-deals-crm", "--with-history", "dealstage,hubspot_owner_id", "--db", path, "--json"); err != nil {
		t.Fatal(err)
	}
	rs.mu.Lock()
	reqs := append([]string(nil), rs.reqs...)
	rs.mu.Unlock()
	saw := false
	for _, r := range reqs {
		if !strings.Contains(r, "/crm/v3/objects/") {
			continue
		}
		saw = true
		if !strings.Contains(r, "propertiesWithHistory=dealstage") {
			t.Errorf("history not requested: %s", r)
		}
		// Sync may request properties= (it adds hs_is_closed), but never fewer
		// than HubSpot's default deal set: history must not narrow the snapshot.
		if i := strings.Index(r, "properties="); i >= 0 && !strings.Contains(r[:i], "With") {
			got := r[i:]
			for _, p := range []string{"dealname", "amount", "closedate", "pipeline", "dealstage", "createdate", "hs_lastmodifieddate", "hs_object_id"} {
				if !strings.Contains(got, p) {
					t.Errorf("history narrowed the snapshot's properties (no %s): %s", p, r)
				}
			}
		}
	}
	if !saw {
		t.Fatalf("no deals request: %v", reqs)
	}
	s, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var amount, owner, source string
	if err := s.DB().QueryRow(`SELECT COALESCE(json_extract(data,'$.properties.amount'),''), COALESCE(json_extract(data,'$.properties.hubspot_owner_id'),'') FROM hubspot_deals_crm WHERE id='1'`).Scan(&amount, &owner); err != nil {
		t.Fatal(err)
	}
	if amount != "10000" || owner != "7" {
		t.Fatalf("snapshot amount=%q owner=%q, want 10000 and the newest history value 7", amount, owner)
	}
	if err := s.DB().QueryRow(`SELECT COALESCE(source,'') FROM hubspot_property_history WHERE object_id='1' AND property='hubspot_owner_id' AND value='7'`).Scan(&source); err != nil || source != "CRM_UI" {
		t.Fatalf("history source = %q (%v), want the documented sourceType CRM_UI", source, err)
	}
}

// --data-source local must never reach the API, even with an empty mirror.
// With no association rows it refuses (an empty timeline would claim the
// record has none; Codex r1 2026-10-24) and still sends nothing.
func TestR1EngagementsLocalNeverGoesLive(t *testing.T) {
	testenv.Isolate(t)
	rs := newR1Server(t, 200, `{"results":[]}`)
	_, path := r1Store(t)
	if _, err := r1Run(t, "", "engagements", "of", "deal:1", "--data-source", "local", "--db", path, "--json"); err == nil || !strings.Contains(err.Error(), "associations") {
		t.Fatalf("err = %v, want a refusal naming associations", err)
	}
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if len(rs.reqs) != 0 {
		t.Fatalf("--data-source local made live requests: %v", rs.reqs)
	}
}

// A pipeline stage with no id must not turn "stage IN (closed ids)" into NULL
// and silently drop every open deal.
func TestR1StageWithoutIDKeepsOpenDeals(t *testing.T) {
	testenv.Isolate(t)
	s, path := r1Store(t)
	nfStoreDealPipeline(t, s, map[string]any{"id": "p1", "label": "P", "stages": []map[string]any{
		{"label": "no id", "metadata": map[string]any{"isClosed": "true", "probability": "0.0"}},
		{"id": "open1", "label": "Open", "metadata": map[string]any{"isClosed": "false", "probability": "0.5"}},
	}})
	r1Deal(t, s, "1", map[string]any{"dealstage": "open1", "hubspot_owner_id": "7", "amount": "250", "pipeline": "p1"})
	s.Close()
	out, err := r1Run(t, "", "owner-load", "--db", path, "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "open1") {
		t.Fatalf("open deal dropped: %s", out)
	}
}

// properties:null must not panic the history fill (Codex r2 P2).
func TestR2FillPropertiesFromHistoryNullProperties(t *testing.T) {
	out := fillPropertiesFromHistory(json.RawMessage(`{"properties":null,"propertiesWithHistory":{"hubspot_owner_id":[{"value":"7","timestamp":"2026-10-24T14:00:00Z"}]}}`))
	if !strings.Contains(string(out), `"hubspot_owner_id":"7"`) {
		t.Fatalf("out = %s", out)
	}
}
