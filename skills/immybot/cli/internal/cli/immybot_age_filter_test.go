// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Behavioural acceptance for age filters: an age compares as a duration, never
// as whole hours or days, on rows shaped like GetOnboardingComputerResponse.

package cli

import (
	"testing"
	"time"
)

func TestOnboardingStalledOlderThanComparesExactAge(t *testing.T) {
	now := time.Now().UTC()
	db := seedStore(t, map[string][]map[string]any{
		"computers-onboarding": {
			{"id": 1, "computerName": "WS-105M", "tenantName": "Contoso", "isOnline": true, "onboardingFailed": false,
				"onboardingStatus": "Onboarding", "onboardingSessionId": nil, "updatedDate": now.Add(-105 * time.Minute).Format(time.RFC3339)},
			{"id": 2, "computerName": "WS-80M", "tenantName": "Contoso", "isOnline": true, "onboardingFailed": false,
				"onboardingStatus": "Onboarding", "onboardingSessionId": nil, "updatedDate": now.Add(-80 * time.Minute).Format(time.RFC3339)},
			{"id": 3, "computerName": "WS-36H", "tenantName": "Contoso", "isOnline": true, "onboardingFailed": false,
				"onboardingStatus": "Onboarding", "onboardingSessionId": nil, "updatedDate": now.Add(-36 * time.Hour).Format(time.RFC3339)},
		},
	})
	out, err := runCLI(t, "onboarding-stalled", "--older-than", "90m", "--json", "--db", db)
	if err != nil {
		t.Fatalf("onboarding-stalled: %v\n%s", err, out)
	}
	v := decode[onboardingStalledView](t, out)
	names := map[string]bool{}
	for _, c := range v.Computers {
		names[c.ComputerName] = true
	}
	if v.StalledCount != 2 || !names["WS-105M"] || !names["WS-36H"] || names["WS-80M"] {
		t.Fatalf("--older-than 90m listed %v (stalled %d), want WS-105M and WS-36H only:\n%s", names, v.StalledCount, out)
	}

	out, err = runCLI(t, "onboarding-stalled", "--older-than", "1d", "--json", "--db", db)
	if err != nil {
		t.Fatalf("onboarding-stalled: %v\n%s", err, out)
	}
	if v := decode[onboardingStalledView](t, out); v.StalledCount != 1 {
		t.Fatalf("--older-than 1d stalled = %d, want 1 (the 36-hour-old machine):\n%s", v.StalledCount, out)
	}
}
