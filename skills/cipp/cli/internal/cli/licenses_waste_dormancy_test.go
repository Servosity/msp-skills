// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"
)

// dormancyFixture is the shared row set both entry points must agree on.
func dormancyFixture() []map[string]any {
	return []map[string]any{
		// Licensed + long idle -> dormant.
		{"userPrincipalName": "idle@a.example.com", "_tenant": "a.example.com",
			"assignedLicenses":   []any{map[string]any{"skuPartNumber": "SPE_E3"}},
			"lastSignInDateTime": "2020-01-01T00:00:00Z", "accountEnabled": true},
		// Licensed + recent sign-in -> NOT dormant.
		{"userPrincipalName": "active@a.example.com", "_tenant": "a.example.com",
			"assignedLicenses":   []any{map[string]any{"skuPartNumber": "SPE_E3"}},
			"lastSignInDateTime": time.Now().UTC().Format(time.RFC3339), "accountEnabled": true},
		// Licensed + disabled + no sign-in data -> dormant (wasted seat).
		{"userPrincipalName": "disabled@b.example.com", "_tenant": "b.example.com",
			"assignedLicenses": []any{map[string]any{"skuPartNumber": "EXCHANGESTANDARD"}},
			"accountEnabled":   false},
		// UNLICENSED + long idle -> NOT dormant: this is LICENSED dormancy.
		{"userPrincipalName": "nolicense@b.example.com", "_tenant": "b.example.com",
			"lastSignInDateTime": "2020-01-01T00:00:00Z", "accountEnabled": true},
	}
}

// TestComputeDormantUsersSelectsOnlyLicensedDormancy locks the join's contract.
func TestComputeDormantUsersSelectsOnlyLicensedDormancy(t *testing.T) {
	now := time.Now()
	cutoff := now.AddDate(0, 0, -90)
	got := computeDormantUsers(dormancyFixture(), now, cutoff, 90, false)

	if len(got) != 2 {
		t.Fatalf("expected 2 dormant licensed accounts, got %d: %+v", len(got), got)
	}
	byUser := map[string]dormantUserRow{}
	for _, r := range got {
		byUser[r.User] = r
	}
	if _, ok := byUser["idle@a.example.com"]; !ok {
		t.Error("a licensed account idle past the cutoff must be reported")
	}
	if r, ok := byUser["disabled@b.example.com"]; !ok {
		t.Error("a disabled account still holding a seat must be reported")
	} else if r.Reason == "" {
		t.Error("dormant rows must carry a reason")
	}
	if _, ok := byUser["active@a.example.com"]; ok {
		t.Error("a recently active licensed account must NOT be reported")
	}
	if _, ok := byUser["nolicense@b.example.com"]; ok {
		t.Error("an UNLICENSED idle account must NOT be reported; that is general inactivity, not licensed dormancy")
	}
}

// TestComputeDormantUsersStaleDaysBoundary asserts --days/--stale-days actually
// moves the cutoff.
func TestComputeDormantUsersStaleDaysBoundary(t *testing.T) {
	now := time.Now()
	users := []map[string]any{
		{"userPrincipalName": "u@a.example.com", "_tenant": "a.example.com",
			"assignedLicenses":   []any{map[string]any{"skuPartNumber": "SPE_E3"}},
			"lastSignInDateTime": now.AddDate(0, 0, -45).UTC().Format(time.RFC3339), "accountEnabled": true},
	}
	if got := computeDormantUsers(users, now, now.AddDate(0, 0, -90), 90, false); len(got) != 0 {
		t.Errorf("45 days idle must NOT be dormant at a 90-day cutoff, got %d rows", len(got))
	}
	if got := computeDormantUsers(users, now, now.AddDate(0, 0, -30), 30, false); len(got) != 1 {
		t.Errorf("45 days idle MUST be dormant at a 30-day cutoff, got %d rows", len(got))
	}
}

// TestNeverSignedInLicensedUserPolicy pins computeDormantUsers' two modes:
// missingSignInIsDormant=true flags a licensed account with no sign-in data;
// false (what `licenses waste` uses) treats it as unknown, not idle.
// Carried from the 2026-08-13 library print (resolved NOVEL-COLLISION on
// users_stale_test.go during the 2026-10-23 reprint).
func TestNeverSignedInLicensedUserPolicy(t *testing.T) {
	now := time.Now()
	cutoff := now.AddDate(0, 0, -90)
	// Licensed, ENABLED, no sign-in data at all.
	users := []map[string]any{
		{"userPrincipalName": "never@a.example.com", "_tenant": "a.example.com",
			"assignedLicenses": []any{map[string]any{"skuPartNumber": "SPE_E3"}},
			"accountEnabled":   true},
	}

	// missingSignInIsDormant=true: no sign-in data counts as dormant.
	if got := computeDormantUsers(users, now, cutoff, 90, true); len(got) != 1 {
		t.Errorf("missingSignInIsDormant=true must flag a never-signed-in licensed account, got %d rows", len(got))
	} else if got[0].Reason == "" {
		t.Error("the flagged row must carry a reason")
	}

	// `licenses waste` policy: absent sign-in data is unknown, not idle.
	if got := computeDormantUsers(users, now, cutoff, 90, false); len(got) != 0 {
		t.Errorf("licenses waste must NOT report an enabled account with no sign-in data as wasted, got %d rows", len(got))
	}
}
