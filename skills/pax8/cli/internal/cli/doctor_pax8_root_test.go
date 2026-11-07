// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

// pax8 handfix: doctor-pax8-root-not-placeholder. Pax8's real API root is the
// shipped default; doctor must dial it and probe the credential, never call it a
// placeholder.
func TestDoctorTreatsThePax8APIRootAsReal(t *testing.T) {
	for _, base := range []string{"https://api.pax8.com/v1", "https://api.pax8.com/v1/", "HTTPS://API.PAX8.COM/v1"} {
		if doctorBaseURLIsShippedPlaceholder(base) || doctorBaseURLIsPlaceholder(base) {
			t.Errorf("doctor calls %q a placeholder; it is Pax8's API root", base)
		}
	}
	if !doctorBaseURLIsShippedPlaceholder("https://{tenant}.example.com/v1") {
		t.Error("an unresolved template must still read as a placeholder")
	}
}
