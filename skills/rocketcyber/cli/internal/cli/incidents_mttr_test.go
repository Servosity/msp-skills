// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored tests for the incidents mttr novel command.

package cli

import (
	"testing"
	"time"
)

func TestComputeMTTR(t *testing.T) {
	now := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)
	mk := func(created string, resolved string) mttrIncident {
		inc := mttrIncident{CreatedAt: parseAPITime(created)}
		if resolved != "" {
			inc.ResolvedAt = parseAPITime(resolved)
			inc.Resolved = true
		}
		return inc
	}
	tests := []struct {
		name         string
		incidents    []mttrIncident
		window       time.Duration
		wantTotal    int
		wantResolved int
		wantOpen     int
		wantMean     float64
		wantMedian   float64
	}{
		{
			name: "mixed resolved and open",
			incidents: []mttrIncident{
				mk("2026-06-01T00:00:00Z", "2026-06-01T10:00:00Z"), // 10h
				mk("2026-06-02T00:00:00Z", "2026-06-03T00:00:00Z"), // 24h
				mk("2026-06-04T00:00:00Z", ""),                     // open, 2d old -> 0-7d bucket
			},
			window:       90 * 24 * time.Hour,
			wantTotal:    3,
			wantResolved: 2,
			wantOpen:     1,
			wantMean:     17.0,
			wantMedian:   17.0,
		},
		{
			name: "outside window excluded",
			incidents: []mttrIncident{
				mk("2025-01-01T00:00:00Z", "2025-01-02T00:00:00Z"),
			},
			window:    30 * 24 * time.Hour,
			wantTotal: 0,
		},
		{
			name:      "empty input",
			incidents: nil,
			window:    30 * 24 * time.Hour,
			wantTotal: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			view := computeMTTR(tt.incidents, now, tt.window, "test")
			if view.TotalIncidents != tt.wantTotal {
				t.Errorf("TotalIncidents = %d, want %d", view.TotalIncidents, tt.wantTotal)
			}
			if view.ResolvedCount != tt.wantResolved {
				t.Errorf("ResolvedCount = %d, want %d", view.ResolvedCount, tt.wantResolved)
			}
			if view.OpenCount != tt.wantOpen {
				t.Errorf("OpenCount = %d, want %d", view.OpenCount, tt.wantOpen)
			}
			if view.MTTRHoursMean != tt.wantMean {
				t.Errorf("MTTRHoursMean = %v, want %v", view.MTTRHoursMean, tt.wantMean)
			}
			if view.MTTRHoursMedian != tt.wantMedian {
				t.Errorf("MTTRHoursMedian = %v, want %v", view.MTTRHoursMedian, tt.wantMedian)
			}
		})
	}
}

func TestComputeMTTRAgingBuckets(t *testing.T) {
	now := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)
	incidents := []mttrIncident{
		{CreatedAt: now.Add(-2 * 24 * time.Hour)},  // 0-7d
		{CreatedAt: now.Add(-10 * 24 * time.Hour)}, // 7-30d
		{CreatedAt: now.Add(-45 * 24 * time.Hour)}, // over-30d
	}
	view := computeMTTR(incidents, now, 90*24*time.Hour, "90d")
	if view.OpenAging["0-7d"] != 1 || view.OpenAging["7-30d"] != 1 || view.OpenAging["over-30d"] != 1 {
		t.Errorf("OpenAging = %v, want one in each bucket", view.OpenAging)
	}
}

func TestComputeMTTRMonthly(t *testing.T) {
	now := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)
	incidents := []mttrIncident{
		{CreatedAt: parseAPITime("2026-04-01T00:00:00Z"), ResolvedAt: parseAPITime("2026-04-01T12:00:00Z"), Resolved: true},
		{CreatedAt: parseAPITime("2026-05-01T00:00:00Z"), ResolvedAt: parseAPITime("2026-05-02T00:00:00Z"), Resolved: true},
	}
	view := computeMTTR(incidents, now, 365*24*time.Hour, "1y")
	if len(view.Monthly) != 2 {
		t.Fatalf("Monthly = %d entries, want 2", len(view.Monthly))
	}
	if view.Monthly[0].Month != "2026-04" || view.Monthly[0].MeanHours != 12.0 {
		t.Errorf("Monthly[0] = %+v, want 2026-04 / 12.0h", view.Monthly[0])
	}
}
