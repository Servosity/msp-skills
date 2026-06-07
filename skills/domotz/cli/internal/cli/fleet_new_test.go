// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Table-driven tests for the fleet-new time-window parser.

package cli

import (
	"testing"
	"time"
)

func TestParseSinceCutoff(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		in      string
		want    time.Time
		wantErr bool
	}{
		{"empty defaults to 24h", "", now.Add(-24 * time.Hour), false},
		{"hours", "6h", now.Add(-6 * time.Hour), false},
		{"minutes", "90m", now.Add(-90 * time.Minute), false},
		{"days suffix", "7d", now.AddDate(0, 0, -7), false},
		{"zero days", "0d", now, false},
		{"whitespace trimmed", "  12h  ", now.Add(-12 * time.Hour), false},
		{"bad unit", "5x", time.Time{}, true},
		{"negative", "-3h", time.Time{}, true},
		{"garbage days", "xd", time.Time{}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseSinceCutoff(tc.in, now)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got none", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.in, err)
			}
			if !got.Equal(tc.want) {
				t.Fatalf("parseSinceCutoff(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
