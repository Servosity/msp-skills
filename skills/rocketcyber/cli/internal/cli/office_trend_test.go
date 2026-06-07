// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored tests for the office trend novel command.

package cli

import "testing"

func TestComputeScoreTrend(t *testing.T) {
	tests := []struct {
		name          string
		points        []scorePoint
		wantDirection string
		wantDelta     float64
		wantFirst     float64
		wantLast      float64
	}{
		{
			name: "improving",
			points: []scorePoint{
				{Date: "2026-06-01", Score: 36.76},
				{Date: "2026-06-03", Score: 48.92},
				{Date: "2026-06-02", Score: 40.00},
			},
			wantDirection: "improving",
			wantDelta:     12.16,
			wantFirst:     36.76,
			wantLast:      48.92,
		},
		{
			name: "declining",
			points: []scorePoint{
				{Date: "2026-06-01", Score: 50.0},
				{Date: "2026-06-02", Score: 45.0},
			},
			wantDirection: "declining",
			wantDelta:     -5.0,
			wantFirst:     50.0,
			wantLast:      45.0,
		},
		{
			name: "flat within threshold",
			points: []scorePoint{
				{Date: "2026-06-01", Score: 44.8},
				{Date: "2026-06-02", Score: 45.1},
			},
			wantDirection: "flat",
			wantDelta:     0.3,
			wantFirst:     44.8,
			wantLast:      45.1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			view := computeScoreTrend(tt.points)
			if view.Direction != tt.wantDirection {
				t.Errorf("Direction = %q, want %q", view.Direction, tt.wantDirection)
			}
			if view.Delta != tt.wantDelta {
				t.Errorf("Delta = %v, want %v", view.Delta, tt.wantDelta)
			}
			if view.FirstScore != tt.wantFirst || view.LastScore != tt.wantLast {
				t.Errorf("First/Last = %v/%v, want %v/%v (date-sorted, not input order)", view.FirstScore, view.LastScore, tt.wantFirst, tt.wantLast)
			}
		})
	}
}

func TestComputeScoreTrendEmpty(t *testing.T) {
	view := computeScoreTrend(nil)
	if view.DataPoints != 0 || view.Direction != "flat" {
		t.Errorf("empty series should be 0 points / flat, got %+v", view)
	}
}

func TestComputeScoreTrendMinMaxAvg(t *testing.T) {
	view := computeScoreTrend([]scorePoint{
		{Date: "2026-06-01", Score: 30.0},
		{Date: "2026-06-02", Score: 60.0},
		{Date: "2026-06-03", Score: 45.0},
	})
	if view.MinScore != 30.0 || view.MaxScore != 60.0 || view.AverageScore != 45.0 {
		t.Errorf("min/max/avg = %v/%v/%v, want 30/60/45", view.MinScore, view.MaxScore, view.AverageScore)
	}
}
