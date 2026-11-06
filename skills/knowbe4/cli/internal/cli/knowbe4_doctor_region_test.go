// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"

	"knowbe4-pp-cli/internal/client"
)

// knowbe4 handfix doctor-default-region-is-real: KnowBe4 documents `us` as the
// default region, and https://us.api.knowbe4.com/v1 is the live US Reporting
// API host. A base URL resolved with any documented region is an operator's
// real API host, so doctor must dial it and probe the key, not call it an
// unconfigured placeholder (press-traps s5: a real host refused on every
// healthy install). A template left unresolved is still refused.
func TestDoctorDialsTheDocumentedDefaultRegion(t *testing.T) {
	for _, region := range []string{"us", "eu", "ca", "uk", "de"} {
		base := client.ResolvedBaseURL(doctorShippedTemplateBaseURL, map[string]string{"region": region})
		if doctorBaseURLIsPlaceholder(base) || doctorBaseURLIsShippedPlaceholder(base) || doctorBaseURLIsShippedTemplate(base) {
			t.Errorf("doctor refuses %s (region %s) as a placeholder; it is a real KnowBe4 API host", base, region)
		}
	}
	if !doctorBaseURLIsShippedPlaceholder(doctorShippedTemplateBaseURL) {
		t.Errorf("doctor dials the unresolved template %s", doctorShippedTemplateBaseURL)
	}
}
