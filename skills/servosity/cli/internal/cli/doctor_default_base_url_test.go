// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

// TestDoctorDefaultBaseURLIsNotAPlaceholder pins the fix for doctor calling the
// real Servosity API root a placeholder. An operator who sets only
// SERVOSITY_MSP_TOKEN runs against this default; doctor must dial it and probe
// the credential, not report "base_url is a placeholder".
func TestDoctorDefaultBaseURLIsNotAPlaceholder(t *testing.T) {
	for _, base := range []string{"https://api.servosity.com/api/v1", "https://api.servosity.com/api/v1/"} {
		if doctorBaseURLIsPlaceholder(base) || doctorBaseURLIsShippedPlaceholder(base) {
			t.Errorf("doctor treats the real API root %q as a placeholder", base)
		}
	}
	for _, base := range []string{"https://{tenant}.example.com/api", "https://YOUR_HOST/api", "https://api.example.com"} {
		if !doctorBaseURLIsPlaceholder(base) && !doctorBaseURLIsShippedPlaceholder(base) {
			t.Errorf("doctor does not refuse placeholder base_url %q", base)
		}
	}
}
