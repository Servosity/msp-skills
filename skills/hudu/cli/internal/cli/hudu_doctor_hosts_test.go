// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: pins doctor's placeholder refusal after its host literals moved
// out of doctor_verdict.go (reapply/doctor_hosts.py in the reprint run).

package cli

import "testing"

func TestDoctorRefusesTheShippedPlaceholderNotARealTenant(t *testing.T) {
	if doctorShippedBaseURL != "https://your-domain.huducloud.com/api/v1" {
		t.Fatalf("doctorShippedBaseURL = %q, want the spec's placeholder base_url", doctorShippedBaseURL)
	}
	for _, base := range []string{
		"https://your-domain.huducloud.com/api/v1",
		"https://your-domain.huducloud.com/api/v1/",
		"https://localhost.example/api/v1",
		"https://hudu.example/api/v1",
		"https://docs.example.com/api/v1",
	} {
		if !doctorBaseURLIsShippedPlaceholder(base) {
			t.Errorf("doctorBaseURLIsShippedPlaceholder(%q) = false, want true", base)
		}
	}
	for _, base := range []string{
		"https://acme.huducloud.com/api/v1",
		"https://docs.your-itdepartment.co.uk/api/v1",
		"https://hudu.example-msp.com/api/v1",
	} {
		if doctorBaseURLIsShippedPlaceholder(base) {
			t.Errorf("doctorBaseURLIsShippedPlaceholder(%q) = true, want false (a real tenant)", base)
		}
	}
}
