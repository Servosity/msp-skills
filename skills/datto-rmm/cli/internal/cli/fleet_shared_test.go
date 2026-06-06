// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"testing"
	"time"
)

func rawJSON(s string) json.RawMessage { return json.RawMessage([]byte(s)) }

func TestParseDattoTime(t *testing.T) {
	tests := []struct {
		name    string
		raw     json.RawMessage
		wantOK  bool
		wantStr string // RFC3339 UTC if ok
	}{
		{"epoch-ms", rawJSON("1714521600000"), true, "2024-05-01T00:00:00Z"},
		{"epoch-s", rawJSON("1714521600"), true, "2024-05-01T00:00:00Z"},
		{"rfc3339", rawJSON(`"2026-04-01T12:30:00Z"`), true, "2026-04-01T12:30:00Z"},
		{"datetime", rawJSON(`"2026-04-01T12:30:00"`), true, "2026-04-01T12:30:00Z"},
		{"date-only", rawJSON(`"2026-04-01"`), true, "2026-04-01T00:00:00Z"},
		{"epoch-as-string-ms", rawJSON(`"1714521600000"`), true, "2024-05-01T00:00:00Z"},
		{"empty-string", rawJSON(`""`), false, ""},
		{"null", rawJSON("null"), false, ""},
		{"blank", rawJSON(""), false, ""},
		{"garbage", rawJSON(`"not-a-date"`), false, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseDattoTime(tc.raw)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if ok {
				if s := got.UTC().Format(time.RFC3339); s != tc.wantStr {
					t.Fatalf("time = %s, want %s", s, tc.wantStr)
				}
			}
		})
	}
}

func TestParseWarranty(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantOK  bool
		wantStr string
	}{
		{"date", "2026-08-15", true, "2026-08-15T00:00:00Z"},
		{"rfc3339", "2026-08-15T00:00:00Z", true, "2026-08-15T00:00:00Z"},
		{"empty", "", false, ""},
		{"garbage", "soon", false, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseWarranty(tc.in)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if ok {
				if s := got.UTC().Format(time.RFC3339); s != tc.wantStr {
					t.Fatalf("time = %s, want %s", s, tc.wantStr)
				}
			}
		})
	}
}

func TestDaysSince(t *testing.T) {
	now := time.Date(2026, 5, 30, 0, 0, 0, 0, time.UTC)
	then := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)
	if got := daysSince(then, now); got != 10 {
		t.Fatalf("daysSince = %d, want 10", got)
	}
}

// TestAvIsHealthyConsistency locks the single AV-OK predicate across av-gaps,
// scorecard, and diff so the same device can never get three different
// verdicts (the 2026-06-05 review finding).
func TestAvIsHealthyConsistency(t *testing.T) {
	tests := []struct {
		status string
		want   bool
	}{
		{"Running", true},
		{"running", true},
		{"RunningAndUpToDate", true},
		{"Running_And_Up_To_Date", true},
		{"Running-And-Up-To-Date", true},
		{"Not Running", false},
		{"not-running", false},
		{"Disabled", false},
		{"NotInstalled", false},
		{"OutOfDate", false},
		{"", false},
	}
	for _, tc := range tests {
		t.Run("status="+tc.status, func(t *testing.T) {
			if got := avIsHealthy(tc.status); got != tc.want {
				t.Fatalf("avIsHealthy(%q) = %v, want %v", tc.status, got, tc.want)
			}
			// av-gaps default branch must agree: healthy devices are not gaps.
			dev := fleetDevice{Hostname: "h", SiteName: "s", Antivirus: fleetAntivirus{AntivirusStatus: tc.status}}
			gaps := computeAvGaps([]fleetDevice{dev}, "")
			if flagged := len(gaps) == 1; flagged == tc.want {
				t.Fatalf("computeAvGaps disagrees with avIsHealthy for %q (flagged=%v healthy=%v)", tc.status, flagged, tc.want)
			}
			// diff posture must agree: healthy devices are not AVNotRunning.
			totals := computePostureTotals([]fleetDevice{dev}, time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC))
			if notRunning := totals.AVNotRunning == 1; notRunning == tc.want {
				t.Fatalf("computePostureTotals disagrees with avIsHealthy for %q", tc.status)
			}
		})
	}
}
