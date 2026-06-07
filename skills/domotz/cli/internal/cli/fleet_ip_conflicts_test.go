// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Table-driven tests for the ip-conflict response parser (tolerates array and
// single-object shapes).

package cli

import "testing"

func TestParseIPConflicts(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantCount int
		wantIP    string
	}{
		{"array of conflicts", `[{"ip":"10.0.0.5","conflicting_devices":[1,2]},{"ip":"10.0.0.9","conflicting_devices":[3]}]`, 2, "10.0.0.5"},
		{"single object", `{"ip":"192.168.1.1","conflicting_devices":[7,8,9]}`, 1, "192.168.1.1"},
		{"empty array", `[]`, 0, ""},
		{"garbage", `not json`, 0, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseIPConflicts([]byte(tc.body))
			if len(got) != tc.wantCount {
				t.Fatalf("parseIPConflicts(%s) count = %d, want %d", tc.body, len(got), tc.wantCount)
			}
			if tc.wantCount > 0 && got[0].IP != tc.wantIP {
				t.Fatalf("first IP = %q, want %q", got[0].IP, tc.wantIP)
			}
		})
	}

	// Conflicting-device count maps through correctly.
	got := parseIPConflicts([]byte(`[{"ip":"10.0.0.5","conflicting_devices":[1,2,3]}]`))
	if len(got) != 1 || len(got[0].ConflictingDevices) != 3 {
		t.Fatalf("conflicting_devices = %v, want 3 entries", got)
	}
}
