// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Regression tests for the 2026-10-24 Codex review (round 1) deal-state and
// stage-history P1s, owned in round 3. Each fails on the pre-fix code.

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"hubspot-pp-cli/internal/cliutil/testenv"
	"hubspot-pp-cli/internal/store"
)

// r3Run runs the root command and returns stdout and stderr.
func r3Run(t *testing.T, args ...string) (string, string, error) {
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

// r3StatePipeline is HubSpot's documented PipelineStage shape with the
// default closed ids and a custom closed id, all marked by metadata.isClosed.
func r3StatePipeline(t *testing.T, s *store.Store) {
	nfStoreDealPipeline(t, s, map[string]any{
		"id": "default", "label": "Sales Pipeline", "displayOrder": 0, "archived": false,
		"stages": []map[string]any{
			{"id": "qualifiedtobuy", "label": "Qualified", "displayOrder": 1, "metadata": map[string]any{"isClosed": "false", "probability": "0.4"}},
			{"id": "presentationscheduled", "label": "Presentation", "displayOrder": 2, "metadata": map[string]any{"isClosed": "false", "probability": "0.6"}},
			{"id": "closedlost", "label": "Closed Lost", "displayOrder": 3, "metadata": map[string]any{"isClosed": "true", "probability": "0.0"}},
			{"id": "12345", "label": "Signed", "displayOrder": 4, "metadata": map[string]any{"isClosed": "true", "probability": "1.0"}},
		},
	})
}

type r3HealthRow struct {
	StageID        string  `json:"stage_id"`
	StageIsClosed  bool    `json:"stage_is_closed"`
	Count          int64   `json:"count"`
	TotalAmount    float64 `json:"total_amount"`
	OpenCount      int64   `json:"open_count"`
	OpenAmount     float64 `json:"open_amount"`
	AtRiskAmount   float64 `json:"at_risk_amount"`
	OldestIdleDays int64   `json:"oldest_idle_days"`
}

// Class: an open-deal figure that does not use the one deal-state predicate.
// Codex repro: a closedlost deal idle since January reported as $100,000 at
// risk. Every closed signal (default closed id, custom isClosed stage,
// hs_is_closed on a deal whose stage the mirror calls open) must keep a deal
// out of open_count, open_amount, at_risk_amount and oldest_idle_days, while
// the per-stage count and total still include it.
func TestR3PipelineHealthClosedDealsAreNeverOpenAtRisk(t *testing.T) {
	testenv.Isolate(t)
	s, path := r1Store(t)
	r3StatePipeline(t, s)
	old := "2026-01-01T00:00:00Z"
	recent := time.Now().UTC().Format(time.RFC3339)
	r1Deal(t, s, "1", map[string]any{"pipeline": "default", "dealstage": "closedlost", "amount": "100000", "notes_last_contacted": old})
	r1Deal(t, s, "2", map[string]any{"pipeline": "default", "dealstage": "12345", "amount": "9000", "notes_last_contacted": old})
	r1Deal(t, s, "3", map[string]any{"pipeline": "default", "dealstage": "qualifiedtobuy", "amount": "500", "hs_is_closed": "true", "notes_last_contacted": old})
	r1Deal(t, s, "4", map[string]any{"pipeline": "default", "dealstage": "qualifiedtobuy", "amount": "700", "notes_last_contacted": "2026-06-01T00:00:00Z"})
	r1Deal(t, s, "5", map[string]any{"pipeline": "default", "dealstage": "qualifiedtobuy", "amount": "300", "notes_last_contacted": recent})
	s.Close()

	out, stderr, err := r3Run(t, "pipeline-health", "default", "--idle-days", "14", "--json", "--db", path)
	if err != nil {
		t.Fatalf("pipeline-health: %v\n%s", err, stderr)
	}
	var view struct {
		Stages []r3HealthRow `json:"stages"`
	}
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	byID := map[string]r3HealthRow{}
	for _, r := range view.Stages {
		byID[r.StageID] = r
	}
	for _, id := range []string{"closedlost", "12345"} {
		r := byID[id]
		if r.AtRiskAmount != 0 || r.OpenCount != 0 || r.OpenAmount != 0 || r.OldestIdleDays != 0 {
			t.Errorf("closed stage %s reports open money or age: %+v\n%s", id, r, out)
		}
		if !r.StageIsClosed || r.Count != 1 {
			t.Errorf("closed stage %s must still count its deal, labeled closed: %+v", id, r)
		}
	}
	if byID["closedlost"].TotalAmount != 100000 {
		t.Errorf("closedlost total_amount = %v, want the per-stage total 100000", byID["closedlost"].TotalAmount)
	}
	q := byID["qualifiedtobuy"]
	if q.StageIsClosed || q.Count != 3 || q.TotalAmount != 1500 {
		t.Errorf("qualifiedtobuy per-stage figures = %+v, want count 3 total 1500", q)
	}
	if q.OpenCount != 2 || q.OpenAmount != 1000 {
		t.Errorf("qualifiedtobuy open figures = %+v, want open_count 2 open_amount 1000 (the hs_is_closed deal is closed)", q)
	}
	if q.AtRiskAmount != 700 {
		t.Errorf("qualifiedtobuy at_risk_amount = %v, want 700 (the idle open deal only)", q.AtRiskAmount)
	}
	// The oldest open idle age belongs to deal 4 (June), not closed deal 3 (January).
	june := int64(time.Since(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)).Hours() / 24)
	if q.OldestIdleDays < june-1 || q.OldestIdleDays > june+1 {
		t.Errorf("qualifiedtobuy oldest_idle_days = %d, want about %d (open deal 4)", q.OldestIdleDays, june)
	}
}

// Same class, the silent-drop member: the stored pipeline is a one-time fetch
// (sync does not collect pipelines), so a stage added since is not listed.
// Its open deal is still open money at risk and must not vanish.
func TestR3PipelineHealthUnlistedStageDealsDoNotVanish(t *testing.T) {
	testenv.Isolate(t)
	s, path := r1Store(t)
	r3StatePipeline(t, s)
	r1Deal(t, s, "6", map[string]any{"pipeline": "default", "dealstage": "99999", "amount": "400", "hs_is_closed": "false", "notes_last_contacted": "2026-01-01T00:00:00Z"})
	r1Deal(t, s, "7", map[string]any{"pipeline": "default", "dealstage": "99999", "amount": "50", "hs_is_closed": "true", "notes_last_contacted": "2026-01-01T00:00:00Z"})
	s.Close()
	out, stderr, err := r3Run(t, "pipeline-health", "default", "--json", "--db", path)
	if err != nil {
		t.Fatalf("pipeline-health: %v\n%s", err, stderr)
	}
	var view struct {
		Stages []struct {
			r3HealthRow
			StageListed bool `json:"stage_listed"`
		} `json:"stages"`
		Unlisted []string `json:"unlisted_stage_ids"`
	}
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	var row *r3HealthRow
	for i := range view.Stages {
		r := view.Stages[i]
		if r.StageListed != (r.StageID != "99999") {
			t.Errorf("stage %s stage_listed = %v", r.StageID, r.StageListed)
		}
		if r.StageID == "99999" {
			row = &view.Stages[i].r3HealthRow
		}
	}
	if row == nil || row.Count != 2 || row.OpenCount != 1 || row.AtRiskAmount != 400 {
		t.Fatalf("unlisted stage row = %+v, want count 2, open_count 1, at_risk_amount 400\n%s", row, out)
	}
	if len(view.Unlisted) != 1 || view.Unlisted[0] != "99999" {
		t.Errorf("unlisted_stage_ids = %v, want [99999]", view.Unlisted)
	}
	if !strings.Contains(stderr, "2 deal(s) in pipeline default") || !strings.Contains(stderr, "sync --resources hubspot-pipelines-crm") {
		t.Errorf("stderr must warn and name the pipelines fetch, got %q", stderr)
	}
}

// HubSpot's default deal pipeline id is "default"; a custom numeric pipeline
// id sorts before it and must not answer for 'pipeline-health default'.
func TestR3PipelineHealthDefaultIsHubSpotsDefaultPipeline(t *testing.T) {
	s, _ := r1Store(t)
	nfStoreDealPipeline(t, s, map[string]any{
		"id": "12345", "label": "Renewals", "displayOrder": 1, "archived": false,
		"stages": []map[string]any{{"id": "r1", "label": "Renewal", "displayOrder": 0, "metadata": map[string]any{"isClosed": "false", "probability": "0.5"}}},
	})
	r3StatePipeline(t, s)
	id, _, err := resolvePipeline(s, "default")
	if err != nil {
		t.Fatal(err)
	}
	if id != "default" {
		t.Fatalf("resolvePipeline(default) = %q, want HubSpot's default pipeline", id)
	}
}

type r3VelocityView struct {
	Deals []struct {
		ID             string `json:"id"`
		Stage          string `json:"stage"`
		EnteredStageAt string `json:"entered_stage_at"`
	} `json:"deals"`
	StageStats []struct {
		Stage     string `json:"stage"`
		DealCount int64  `json:"deal_count"`
	} `json:"stage_stats"`
	SkippedCount    int            `json:"skipped_count"`
	SkippedByReason map[string]int `json:"skipped_by_reason"`
	Skipped         []struct {
		ID           string `json:"id"`
		Stage        string `json:"stage"`
		Reason       string `json:"reason"`
		HistoryStage string `json:"history_stage"`
	} `json:"skipped"`
}

// Class: a stage-history derived figure computed from history that does not
// describe the current stage. Codex repro: deal 1 synced --with-history while
// in qualifiedtobuy (January entry), then moved to presentationscheduled and
// synced without history. The January entry must not date the new stage.
func TestR3DealsVelocityStaleHistoryIsNotTheCurrentStage(t *testing.T) {
	testenv.Isolate(t)
	s, path := r1Store(t)
	r3StatePipeline(t, s)
	// 1: the Codex repro, history ends at an older stage.
	r1Deal(t, s, "1", map[string]any{"dealname": "Stale", "dealstage": "presentationscheduled", "pipeline": "default"})
	r2WriterHistory(t, s, "deals", "1", "dealstage", "qualifiedtobuy", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	// 2: consistent, entered qualifiedtobuy on September 1.
	r1Deal(t, s, "2", map[string]any{"dealname": "Fresh", "dealstage": "qualifiedtobuy", "pipeline": "default", "hs_lastmodifieddate": "2026-09-01T00:00:00Z"})
	r2History(t, s, "deals", "2", "dealstage", "appointmentscheduled", "2026-08-01T00:00:00Z")
	r2WriterHistory(t, s, "deals", "2", "dealstage", "qualifiedtobuy", time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
	// 3: open, never captured with history.
	r1Deal(t, s, "3", map[string]any{"dealname": "Unknown", "dealstage": "qualifiedtobuy", "pipeline": "default"})
	// 4: the newest instant holds two stages (two storage shapes, one
	// instant); the history does not say which is current, so it is unknown.
	r1Deal(t, s, "4", map[string]any{"dealname": "Ambiguous", "dealstage": "qualifiedtobuy", "pipeline": "default"})
	r2History(t, s, "deals", "4", "dealstage", "qualifiedtobuy", "2026-02-01T00:00:00Z")
	r2WriterHistory(t, s, "deals", "4", "dealstage", "presentationscheduled", time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC))
	s.Close()

	out, stderr, err := r3Run(t, "deals", "velocity", "--json", "--db", path)
	if err != nil {
		t.Fatalf("deals velocity: %v\n%s", err, stderr)
	}
	var view r3VelocityView
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	if len(view.Deals) != 1 || view.Deals[0].ID != "2" || !strings.HasPrefix(view.Deals[0].EnteredStageAt, "2026-09-01") {
		t.Fatalf("deals = %+v, want only deal 2 entered 2026-09-01 (stale history must not date a stage)\n%s", view.Deals, out)
	}
	// Per-stage stats only use deals whose history describes the current stage.
	if len(view.StageStats) != 1 || view.StageStats[0].Stage != "qualifiedtobuy" || view.StageStats[0].DealCount != 1 {
		t.Fatalf("stage_stats = %+v, want qualifiedtobuy deal_count 1 only\n%s", view.StageStats, out)
	}
	if view.SkippedCount != 3 || view.SkippedByReason["stale_history"] != 2 || view.SkippedByReason["no_history"] != 1 {
		t.Fatalf("skipped_count = %d by_reason = %v, want 3 (2 stale_history, 1 no_history)\n%s", view.SkippedCount, view.SkippedByReason, out)
	}
	found := false
	for _, sk := range view.Skipped {
		if sk.ID == "1" {
			found = true
			if sk.Reason != "stale_history" || sk.Stage != "presentationscheduled" || sk.HistoryStage != "qualifiedtobuy" {
				t.Errorf("skipped deal 1 = %+v, want stale_history from qualifiedtobuy", sk)
			}
		}
	}
	if !found {
		t.Errorf("deal 1 missing from skipped: %+v", view.Skipped)
	}
	if !strings.Contains(stderr, "3 open deal(s) left out") || !strings.Contains(stderr, "--with-history dealstage") {
		t.Errorf("stderr must warn and name the refreshing sync, got %q", stderr)
	}

	// --limit trims the skipped list, never the count.
	out, _, err = r3Run(t, "deals", "velocity", "--json", "--limit", "1", "--db", path)
	if err != nil {
		t.Fatal(err)
	}
	view = r3VelocityView{}
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatal(err)
	}
	if view.SkippedCount != 3 || len(view.Skipped) != 1 {
		t.Fatalf("with --limit 1: skipped_count = %d, len(skipped) = %d; want 3 and 1", view.SkippedCount, len(view.Skipped))
	}
}

// A deal in a stage no stored pipeline lists, with no hs_is_closed, has an
// unknown open/closed state: every open-deal report refuses instead of
// counting it as open.
func TestR3OpenDealReportsRefuseUnknownDealState(t *testing.T) {
	testenv.Isolate(t)
	s, path := r1Store(t)
	r3StatePipeline(t, s)
	r1Deal(t, s, "6", map[string]any{"pipeline": "default", "dealstage": "99999", "amount": "400", "notes_last_contacted": "2026-01-01T00:00:00Z"})
	s.Close()
	for _, argv := range [][]string{
		{"pipeline-health", "default"},
		{"owner-load"},
		{"deals", "unowned"},
		{"deals", "velocity"},
		{"stale", "deals"},
		{"nurture-mine", "--owner", "1"},
	} {
		_, stderr, err := r3Run(t, append(argv, "--json", "--db", path)...)
		if err == nil || !strings.Contains(err.Error()+stderr, "open or closed is unknown") {
			t.Errorf("%v: err=%v; want a refusal naming the unknown deal state", argv, err)
		}
	}
}
