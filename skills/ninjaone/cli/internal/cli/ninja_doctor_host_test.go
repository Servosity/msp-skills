// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

// The shipped base_url, https://app.ninjarmm.com, is NinjaOne's real US region,
// not a placeholder: an operator on the default must get a credential probe, not
// "not configured". Only a true stand-in may be refused before dialling.
// (Press trap: doctorShippedBaseURL is "" unless the shipped value is a placeholder.)
func TestDoctorProbesEveryRealNinjaOneRegion(t *testing.T) {
	for _, host := range []string{
		"https://app.ninjarmm.com", "https://app.ninjarmm.com/",
		"https://us2.ninjarmm.com", "https://eu.ninjarmm.com",
		"https://ca.ninjarmm.com", "https://oc.ninjarmm.com",
	} {
		if doctorBaseURLIsShippedPlaceholder(host) {
			t.Errorf("%s is a real NinjaOne region, but doctor treats it as a placeholder", host)
		}
	}
	for _, stand := range []string{"https://{region}.ninjarmm.com", "https://YOUR_REGION.ninjarmm.com", "https://ninja.example.com"} {
		if !doctorBaseURLIsShippedPlaceholder(stand) {
			t.Errorf("%s is a stand-in and must be refused before dialling", stand)
		}
	}
}
