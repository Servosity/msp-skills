// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (afi handfix doctor-real-host-not-placeholder).

package cli

import "testing"

// The shipped base URL is afi's real API host. doctor must dial it and probe the
// credential, not report a working default install as an unconfigured placeholder.
func TestDoctorRealShippedHostIsNotAPlaceholder(t *testing.T) {
	for _, base := range []string{"https://papi.afi.ai", "https://papi.afi.ai/", "HTTPS://PAPI.AFI.AI"} {
		if doctorBaseURLIsShippedPlaceholder(base) || doctorBaseURLIsPlaceholder(base) {
			t.Fatalf("doctor treats the real afi host %q as a placeholder", base)
		}
	}
	for _, base := range []string{"https://{tenant}.example/api", "https://api.example.com", "https://x/YOUR_ID"} {
		if !doctorBaseURLIsShippedPlaceholder(base) && !doctorBaseURLIsPlaceholder(base) {
			t.Fatalf("doctor no longer refuses the placeholder %q", base)
		}
	}
}
