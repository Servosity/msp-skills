// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestLvlComputeStale(t *testing.T) {
	devices := lvlFxDevices()
	now := lvlFxNow()

	tests := []struct {
		name         string
		days         float64
		excludeMaint bool
		wantCount    int
		wantFirst    string
	}{
		{"7d include maintenance", 7, false, 2, "bravo"}, // bravo (10d) + charlie (no last_seen, offline)
		{"7d exclude maintenance", 7, true, 1, "bravo"},  // charlie is maintenance -> excluded
		{"1d include maintenance", 1, false, 2, "bravo"}, // bravo still dark, charlie unknown
		{"100d", 100, false, 1, "charlie"},               // only charlie (unknown/offline); bravo only 10d
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := lvlComputeStale(devices, tc.days, tc.excludeMaint, now)
			if res.Count != tc.wantCount {
				t.Fatalf("count = %d, want %d (devices: %+v)", res.Count, tc.wantCount, res.Devices)
			}
			if tc.wantCount > 0 && res.Devices[0].Hostname != tc.wantFirst {
				t.Errorf("first = %q, want %q", res.Devices[0].Hostname, tc.wantFirst)
			}
		})
	}
}

func TestLvlComputeStaleDaysDark(t *testing.T) {
	res := lvlComputeStale(lvlFxDevices(), 7, false, lvlFxNow())
	for _, d := range res.Devices {
		if d.Hostname == "bravo" && (d.DaysDark < 9.9 || d.DaysDark > 10.1) {
			t.Errorf("bravo days_dark = %.2f, want ~10", d.DaysDark)
		}
		if d.Hostname == "charlie" && d.HasLastSeen {
			t.Errorf("charlie should have HasLastSeen=false")
		}
	}
}
