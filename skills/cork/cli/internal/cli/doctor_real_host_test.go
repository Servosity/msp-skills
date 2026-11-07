// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

// cork handfix doctor-dials-the-real-cork-host: Cork is one SaaS host, so the
// shipped base_url is the real API root an operator never needs to override.
// Treating it as a placeholder made every default install report
// "ERROR base_url is still the shipped placeholder" and never probe the key.
// Real placeholders must still be refused.
func TestDoctorDialsTheRealCorkHost(t *testing.T) {
	for _, base := range []string{"https://api.corkinc.com/api/v1", "https://api.corkinc.com/api/v1/"} {
		if doctorBaseURLIsShippedPlaceholder(base) || doctorBaseURLIsPlaceholder(base) {
			t.Errorf("%s is Cork's real API root, not a placeholder", base)
		}
	}
	for _, base := range []string{"https://{tenant}.corkinc.com/api/v1", "https://api.example.com", "https://cork.YOUR_HOST/api"} {
		if !doctorBaseURLIsShippedPlaceholder(base) && !doctorBaseURLIsPlaceholder(base) {
			t.Errorf("%s is a placeholder and must be refused", base)
		}
	}
}
