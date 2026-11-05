// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

// gradient handfix: doctor-gradient-root-not-placeholder. Gradient's real API
// root is the shipped default; doctor must dial it and probe the credential,
// never call it a placeholder.
func TestDoctorTreatsTheGradientAPIRootAsReal(t *testing.T) {
	for _, base := range []string{"https://app.usegradient.com/api", "https://app.usegradient.com/api/", "HTTPS://APP.USEGRADIENT.COM/api"} {
		if doctorBaseURLIsShippedPlaceholder(base) || doctorBaseURLIsPlaceholder(base) {
			t.Errorf("doctor calls %q a placeholder; it is Gradient's API root", base)
		}
	}
	if !doctorBaseURLIsShippedPlaceholder("https://{tenant}.example.com/api") {
		t.Error("an unresolved template must still read as a placeholder")
	}
}
