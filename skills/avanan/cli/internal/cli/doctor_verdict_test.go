// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

// TestDoctorAcceptsTheRealDefaultGateway pins doctorShippedBaseURL to "": the
// default base URL is Avanan's live US gateway, so doctor must probe it rather
// than call it a leftover placeholder.
func TestDoctorAcceptsTheRealDefaultGateway(t *testing.T) {
	for _, base := range []string{
		"https://smart-api-production-1-us.avanan.net",
		"https://smart-api-production-1-us.avanan.net/",
		"https://cloudinfra-gw-us.portal.checkpoint.com/app/hec-api",
	} {
		if doctorBaseURLIsShippedPlaceholder(base) {
			t.Errorf("doctorBaseURLIsShippedPlaceholder(%q) = true, want false", base)
		}
	}
}
