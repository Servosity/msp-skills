// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

// action1 handfix: doctor-action1-root-not-placeholder. Action1's real API roots
// (North America, the shipped default, plus the EU and AU regions) are not
// placeholders; doctor must dial them and probe the credential.
func TestDoctorTreatsTheAction1APIRootsAsReal(t *testing.T) {
	for _, base := range []string{
		"https://app.action1.com/api/3.0", "https://app.action1.com/api/3.0/", "HTTPS://APP.ACTION1.COM/api/3.0",
		"https://app.eu.action1.com/api/3.0", "https://app.au.action1.com/api/3.0",
	} {
		if doctorBaseURLIsShippedPlaceholder(base) || doctorBaseURLIsPlaceholder(base) {
			t.Errorf("doctor calls %q a placeholder; it is an Action1 API root", base)
		}
	}
	if !doctorBaseURLIsShippedPlaceholder("https://{region}.example.com/api/3.0") {
		t.Error("an unresolved template must still read as a placeholder")
	}
}
