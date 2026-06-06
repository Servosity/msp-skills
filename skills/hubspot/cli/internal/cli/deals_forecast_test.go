// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"path/filepath"
	"testing"
	"time"

	"hubspot-pp-cli/internal/store"
)

// seedPipeline inserts a pipeline with stage metadata probabilities into
// hubspot_pipelines_crm so the forecast probability lookup has data.
func seedPipeline(t *testing.T, s *store.Store, id string, stages []map[string]any) {
	t.Helper()
	data, err := json.Marshal(map[string]any{"id": id, "label": id, "stages": stages})
	if err != nil {
		t.Fatalf("marshal pipeline %s: %v", id, err)
	}
	// Bind data as []byte so the driver returns []byte on read — matching the
	// production UpsertBatch path that loadAllStageProbabilities scans into a
	// json.RawMessage. (Binding a string makes the driver return a string,
	// which RawMessage's Scan rejects.)
	if _, err := s.DB().Exec(
		`INSERT INTO hubspot_pipelines_crm (id, data, archived, label) VALUES (?, ?, 0, ?)`,
		id, data, id,
	); err != nil {
		t.Fatalf("insert pipeline %s: %v", id, err)
	}
}

func almostEqual(a, b float64) bool { return math.Abs(a-b) < 0.001 }

func TestQueryDealsForecast_HappyPath(t *testing.T) {
	ctx := context.Background()
	s := newNovelTestStore(t)
	now := time.Now().UTC()

	seedPipeline(t, s, "default", []map[string]any{
		{"stageId": "qualifiedtobuy", "label": "Qualified", "metadata": map[string]any{"probability": "0.4"}},
		{"stageId": "presentationscheduled", "label": "Demo", "metadata": map[string]any{"probability": 0.6}},
		{"stageId": "closedwon", "label": "Won", "metadata": map[string]any{"probability": "1.0"}},
		{"stageId": "closedlost", "label": "Lost", "metadata": map[string]any{"probability": "0.0"}},
	})

	// Two deals closing 2026-07, one closing 2026-08, one undated.
	seedDeal(t, s, "d1", map[string]any{
		"dealname": "A", "amount": "10000", "dealstage": "qualifiedtobuy",
		"pipeline": "default", "closedate": "2026-07-15T00:00:00Z",
	}, now)
	seedDeal(t, s, "d2", map[string]any{
		"dealname": "B", "amount": "20000", "dealstage": "presentationscheduled",
		"pipeline": "default", "closedate": "2026-07-20T00:00:00Z",
	}, now)
	seedDeal(t, s, "d3", map[string]any{
		"dealname": "C", "amount": "5000", "dealstage": "qualifiedtobuy",
		"pipeline": "default", "closedate": "2026-08-01T00:00:00Z",
	}, now)
	seedDeal(t, s, "d4", map[string]any{
		"dealname": "D (undated)", "amount": "8000", "dealstage": "qualifiedtobuy",
		"pipeline": "default",
	}, now)
	// closed deals must be excluded from the open-deal forecast.
	seedDeal(t, s, "d5", map[string]any{
		"dealname": "Won", "amount": "99999", "dealstage": "closedwon",
		"pipeline": "default", "closedate": "2026-07-01T00:00:00Z",
	}, now)

	cmd := newNovelDealsForecastCmd(&rootFlags{})
	cmd.SetContext(ctx)
	view, err := queryDealsForecast(cmd, s, "", "")
	if err != nil {
		t.Fatalf("queryDealsForecast: %v", err)
	}
	if len(view.Months) != 2 {
		t.Fatalf("expected 2 dated months, got %d: %+v", len(view.Months), view.Months)
	}
	// Sorted month ASC => 2026-07 first.
	jul := view.Months[0]
	if jul.Month != "2026-07" {
		t.Fatalf("expected 2026-07 first, got %q", jul.Month)
	}
	if jul.DealCount != 2 {
		t.Errorf("2026-07 deal_count expected 2, got %d", jul.DealCount)
	}
	// weighted = 10000*0.4 + 20000*0.6 = 4000 + 12000 = 16000
	if !almostEqual(jul.WeightedAmount, 16000) {
		t.Errorf("2026-07 weighted expected 16000, got %.2f", jul.WeightedAmount)
	}
	if !almostEqual(jul.UnweightedAmount, 30000) {
		t.Errorf("2026-07 unweighted expected 30000, got %.2f", jul.UnweightedAmount)
	}
	// undated: 8000 * 0.4 = 3200 weighted, 1 deal
	if view.Undated.DealCount != 1 {
		t.Errorf("undated deal_count expected 1, got %d", view.Undated.DealCount)
	}
	if !almostEqual(view.Undated.WeightedAmount, 3200) {
		t.Errorf("undated weighted expected 3200, got %.2f", view.Undated.WeightedAmount)
	}
	// total_weighted = 16000 (jul) + 5000*0.4 (aug=2000) + 3200 (undated) = 21200
	if !almostEqual(view.TotalWeighted, 21200) {
		t.Errorf("total_weighted expected 21200, got %.2f", view.TotalWeighted)
	}

	// --month filter restricts to one bucket and drops undated.
	viewJul, err := queryDealsForecast(cmd, s, "", "2026-07")
	if err != nil {
		t.Fatalf("queryDealsForecast month: %v", err)
	}
	if len(viewJul.Months) != 1 || viewJul.Months[0].Month != "2026-07" {
		t.Errorf("--month expected only 2026-07, got %+v", viewJul.Months)
	}
	if viewJul.Undated.DealCount != 0 {
		t.Errorf("--month should drop undated, got undated count %d", viewJul.Undated.DealCount)
	}
}

func TestQueryDealsForecast_EmptyDB(t *testing.T) {
	ctx := context.Background()
	s := newNovelTestStore(t)

	cmd := newNovelDealsForecastCmd(&rootFlags{})
	cmd.SetContext(ctx)
	view, err := queryDealsForecast(cmd, s, "", "")
	if err != nil {
		t.Fatalf("queryDealsForecast: %v", err)
	}
	if len(view.Months) != 0 {
		t.Errorf("expected no months on empty DB, got %d", len(view.Months))
	}
	if view.Undated.DealCount != 0 || view.TotalWeighted != 0 {
		t.Errorf("expected zero undated/total on empty DB, got %+v", view)
	}
}

func TestDealsForecast_EmptyDBJSONShape(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "empty.db")
	s, err := store.OpenWithContext(ctx, dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	s.Close()

	flags := &rootFlags{asJSON: true}
	cmd := newNovelDealsForecastCmd(flags)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"--db", dbPath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	var view forecastView
	if err := json.Unmarshal(out.Bytes(), &view); err != nil {
		t.Fatalf("unmarshal output %q: %v", out.String(), err)
	}
	if view.Months == nil {
		t.Errorf("months should marshal as [] not null")
	}
}
