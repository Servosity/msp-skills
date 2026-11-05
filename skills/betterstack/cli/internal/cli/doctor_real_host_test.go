// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

// The shipped base URL is Betterstack's real API root: doctor must probe it, never
// report it as an unconfigured placeholder.
func TestDoctorProbesTheRealBetterstackHost(t *testing.T) {
	for _, base := range []string{"https://uptime.betterstack.com/api/v2", "https://uptime.betterstack.com/api/v2/"} {
		if doctorBaseURLIsPlaceholder(base) || doctorBaseURLIsShippedPlaceholder(base) {
			t.Errorf("doctor treats the real host %s as a placeholder", base)
		}
	}
	if !doctorBaseURLIsPlaceholder("https://{tenant}.example.com/v1") && !doctorBaseURLIsShippedPlaceholder("https://{tenant}.example.com/v1") {
		t.Errorf("a templated base_url must still read as a placeholder")
	}
}
