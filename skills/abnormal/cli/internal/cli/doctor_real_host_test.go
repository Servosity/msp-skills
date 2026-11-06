// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

// The shipped base URLs are Abnormal's real regional API roots: doctor must
// probe them, never report them as an unconfigured placeholder.
func TestDoctorProbesTheRealAbnormalHosts(t *testing.T) {
	for _, base := range []string{"https://api.abnormalplatform.com/v1", "https://eu.rest.abnormalsecurity.com/v1"} {
		if doctorBaseURLIsPlaceholder(base) || doctorBaseURLIsShippedPlaceholder(base) {
			t.Errorf("doctor treats the real host %s as a placeholder", base)
		}
	}
	if !doctorBaseURLIsPlaceholder("https://{tenant}.example.com/v1") && !doctorBaseURLIsShippedPlaceholder("https://{tenant}.example.com/v1") {
		t.Errorf("a templated base_url must still read as a placeholder")
	}
}
