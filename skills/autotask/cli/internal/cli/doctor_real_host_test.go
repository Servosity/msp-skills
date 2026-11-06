// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

// Every Autotask zone host (webservicesN.autotask.net) is a real tenant API
// root, including the shipped default webservices5: doctor must probe it,
// never report it as an unconfigured placeholder.
func TestDoctorProbesTheRealAutotaskZoneHosts(t *testing.T) {
	for _, base := range []string{
		"https://webservices5.autotask.net/atservicesrest/V1.0",
		"https://webservices5.autotask.net/atservicesrest/V1.0/",
		"https://webservices1.autotask.net/atservicesrest/V1.0",
		"https://webservices29.autotask.net/atservicesrest/V1.0",
	} {
		if doctorBaseURLIsPlaceholder(base) || doctorBaseURLIsShippedPlaceholder(base) {
			t.Errorf("doctor treats the real zone host %s as a placeholder", base)
		}
	}
	if !doctorBaseURLIsPlaceholder("https://{tenant}.example.com/v1") && !doctorBaseURLIsShippedPlaceholder("https://{tenant}.example.com/v1") {
		t.Errorf("a templated base_url must still read as a placeholder")
	}
}
