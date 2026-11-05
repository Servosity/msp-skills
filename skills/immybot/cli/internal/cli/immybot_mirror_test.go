// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestImmyCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"140.0.1", "9.0.5", 1}, // the bug this exists to prevent
		{"9.0.5", "140.0.1", -1},
		{"1.2.3", "1.2.3", 0},
		{"1.10", "1.9", 1},
		{"2.0", "2.0.0", 0},
		{"v1.2", "1.2", 0},
		{"1.2.3-beta", "1.2.3-alpha", 1},
		{"", "", 0},
	}
	for _, tc := range tests {
		if got := immyCompareVersions(tc.a, tc.b); got != tc.want {
			t.Errorf("immyCompareVersions(%q,%q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestImmyNormalizeReason(t *testing.T) {
	// Only the machine-specific GUID differs: one cluster. (Distinct exit
	// codes are distinct causes; see TestImmyNormalizeReasonKeepsDistinctCodes.)
	a := immyNormalizeReason("Exit code 1603 on host 11111111-2222-3333-4444-555555555555")
	b := immyNormalizeReason("Exit code 1603 on host 99999999-8888-7777-6666-555555555555")
	if a != b {
		t.Fatalf("machine-specific detail not normalised away:\n a=%q\n b=%q", a, b)
	}
	if immyNormalizeReason("Reboot pending") == a {
		t.Fatal("genuinely different reasons must not collapse together")
	}
	if immyNormalizeReason("   ") != "" {
		t.Fatal("blank reason should normalise to empty")
	}
	if got := immyNormalizeReason(`failed at C:\Program Files\app.exe`); !contains(got, "<path>") {
		t.Fatalf("windows path not normalised: %q", got)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

func TestImmyIsFailureStatus(t *testing.T) {
	for _, s := range []string{"Failed", "error", "TimedOut", "cancelled"} {
		if !immyIsFailureStatus(s) {
			t.Errorf("%q should read as a failure", s)
		}
	}
	for _, s := range []string{"Success", "Running", "", "Pending"} {
		if immyIsFailureStatus(s) {
			t.Errorf("%q should not read as a failure", s)
		}
	}
}

func TestAgeBucket(t *testing.T) {
	tests := []struct {
		days float64
		want string
	}{{0.5, "<1d"}, {2, "1-3d"}, {5, "3-7d"}, {10, "7-30d"}, {90, "30d+"}}
	for _, tc := range tests {
		if got := ageBucket(tc.days); got != tc.want {
			t.Errorf("ageBucket(%v) = %q, want %q", tc.days, got, tc.want)
		}
	}
}

func TestTargetSpecificityOrdering(t *testing.T) {
	_, computer, _ := immyTargetApplies("Computer", "7", "42", "42", "7")
	_, tag, _ := immyTargetApplies("TenantTag", "7", "12", "42", "7")
	_, tenant, _ := immyTargetApplies("AllForTenant", "7", "", "42", "7")
	_, global, _ := immyTargetApplies("AllTenants", "", "", "42", "7")
	if !(computer > tag && tag > tenant && tenant > global) {
		t.Fatalf("specificity must rank computer > tag/group > tenant > global; got %d %d %d %d",
			computer, tag, tenant, global)
	}
}

func TestTruthyAndAnyToString(t *testing.T) {
	for _, s := range []string{"1", "true", "TRUE", "yes"} {
		if !truthy(s) {
			t.Errorf("truthy(%q) should be true", s)
		}
	}
	for _, s := range []string{"0", "false", "", "no"} {
		if truthy(s) {
			t.Errorf("truthy(%q) should be false", s)
		}
	}
	if got := anyToString(nil); got != "" {
		t.Errorf("anyToString(nil) = %q, want empty (never \"<nil>\")", got)
	}
	if got := anyToString(float64(42)); got != "42" {
		t.Errorf("anyToString(42.0) = %q, want 42", got)
	}
}
