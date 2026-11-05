// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

// The shipped base_url, https://api.mspbots.ai, is MSPbots' one real Public API
// host, not a placeholder: an operator on the default must get a credential
// probe, not "base_url is still the shipped placeholder".
// Only a true stand-in may be refused before dialling.
// (Press trap: doctorShippedBaseURL is "" unless the shipped value is a placeholder.)
func TestDoctorProbesTheRealMspbotsHost(t *testing.T) {
	for _, host := range []string{"https://api.mspbots.ai", "https://api.mspbots.ai/", "HTTPS://API.MSPBOTS.AI"} {
		if doctorBaseURLIsShippedPlaceholder(host) {
			t.Errorf("%s is the real MSPbots API host, but doctor treats it as a placeholder", host)
		}
	}
	for _, stand := range []string{"https://{host}/api", "https://YOUR_TENANT.mspbots.ai", "https://mspbots.example.com"} {
		if !doctorBaseURLIsShippedPlaceholder(stand) {
			t.Errorf("%s is a stand-in and must be refused before dialling", stand)
		}
	}
}
