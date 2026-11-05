// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire

import (
	"slices"
	"testing"
)

// A proxy is judged against these same origins, and a proxy cannot be driven
// end to end without dialing it, so this literal carries that case: with only
// the selected base URL allowed, HTTPS_PROXY=https://euapi.superops.ai:443 is
// not a recipient a token may reach, whatever the base URL is.
func TestCredentialsGoOnlyToTheSelectedBaseURL(t *testing.T) {
	for _, o := range Declarations.Origins {
		if o.Source == "*" && !slices.Equal(o.Allowed, []string{"base_url"}) {
			t.Errorf(`Origins "*" = %v, want only base_url`, o.Allowed)
		}
	}
}
