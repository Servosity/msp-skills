// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

// rocketcyber handfix doctor-real-host: the shipped base URL is RocketCyber's
// real US API root, so doctor must probe it, never report it as an
// unconfigured placeholder (which also skipped the credential probe).
func TestDoctorProbesTheRealRocketcyberHost(t *testing.T) {
	for _, base := range []string{"https://api-us.rocketcyber.com/v3", "https://api-us.rocketcyber.com/v3/"} {
		if doctorBaseURLIsPlaceholder(base) || doctorBaseURLIsShippedPlaceholder(base) {
			t.Errorf("doctor treats the real host %s as a placeholder", base)
		}
	}
	if !doctorBaseURLIsPlaceholder("https://{tenant}.example.com/v3") && !doctorBaseURLIsShippedPlaceholder("https://{tenant}.example.com/v3") {
		t.Errorf("a templated base_url must still read as a placeholder")
	}
}
