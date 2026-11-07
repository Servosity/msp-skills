// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

// Blumira has one real public API host for every customer. Shipping it as the
// default base_url is not a placeholder: doctor must dial it and probe the
// credential, not tell a healthy install to "set BLUMIRA_BASE_URL"
// (press-traps s5, doctorShippedBaseURL holding a real vendor host). True
// placeholders stay refused.
func TestDoctorDialsTheRealVendorHost(t *testing.T) {
	for _, base := range []string{"https://api.blumira.com/public-api/v1", "https://api.blumira.com/public-api/v1/"} {
		if doctorBaseURLIsPlaceholder(base) || doctorBaseURLIsShippedPlaceholder(base) {
			t.Errorf("doctor treats the real Blumira API host %q as an unset placeholder", base)
		}
	}
	for _, base := range []string{"https://{tenant}.example.com/api", "https://api.example.com/company/YOUR_ID"} {
		if !doctorBaseURLIsPlaceholder(base) && !doctorBaseURLIsShippedPlaceholder(base) {
			t.Errorf("doctor dials the placeholder %q", base)
		}
	}
}
