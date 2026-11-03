// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: pins doctor_verdict.go's doctorShippedBaseURL (handfixes.json doctor-verdict-tracks-reality).

package cli

import "testing"

// TestDoctorDialsXerosRealAPIHost: Xero's shipped base URL is its real API
// root, so doctor must dial it and probe the credential. Matching it as a
// placeholder printed "base_url is still the shipped placeholder" on every
// healthy install. The stand-ins stay refused.
func TestDoctorDialsXerosRealAPIHost(t *testing.T) {
	for _, real := range []string{"https://api.xero.com/api.xro/2.0", "https://api.xero.com/api.xro/2.0/"} {
		if doctorBaseURLIsPlaceholder(real) || doctorBaseURLIsShippedPlaceholder(real) {
			t.Fatalf("doctor treats Xero's real API root %q as a placeholder", real)
		}
	}
	for _, stand := range []string{"https://{tenant}.example/api", "https://api.example.com", "https://host/company/YOUR_ID"} {
		if !doctorBaseURLIsPlaceholder(stand) && !doctorBaseURLIsShippedPlaceholder(stand) {
			t.Fatalf("doctor dials the stand-in %q", stand)
		}
	}
}
