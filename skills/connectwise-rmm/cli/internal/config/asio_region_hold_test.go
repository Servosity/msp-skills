// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"testing"

	"connectwise-rmm-pp-cli/internal/cliutil"
	"connectwise-rmm-pp-cli/internal/credkit"
)

// HoldForAsioRegion (handfixes.json credkit-wiring) binds a credential to a
// regional host only when the target is exactly one of the three https
// regional origins; any other target binds nothing, so the kit keeps refusing
// it there.
func TestHoldForAsioRegionBindsOnlyTheRegionalOrigins(t *testing.T) {
	type held struct{ source, value string }
	var got []held
	prev := cliutil.Hold
	cliutil.Hold = func(source credkit.Source, values ...string) {
		for _, v := range values {
			got = append(got, held{source, v})
		}
	}
	t.Cleanup(func() { cliutil.Hold = prev })

	cases := []struct{ target, want string }{
		{"https://openapi.service.itsupport247.net", "minted:asio-region-na"},
		{"https://openapi.service.euplatform.connectwise.com/v1/token", "minted:asio-region-eu"},
		{"https://OpenAPI.service.auplatform.connectwise.com/api/x", "minted:asio-region-au"},
		{"http://openapi.service.euplatform.connectwise.com/v1/token", ""},
		{"https://openapi.service.euplatform.connectwise.com:8443/v1/token", ""},
		{"https://openapi.service.euplatform.connectwise.com.attacker.example/v1/token", ""},
		{"https://u:p@openapi.service.euplatform.connectwise.com/v1/token", ""},
		{"https://127.0.0.1:8443", ""},
		{"not a url", ""},
		{"", ""},
	}
	for _, tc := range cases {
		got = nil
		HoldForAsioRegion(tc.target, "credvalueqzkx", "  ")
		switch {
		case tc.want == "" && len(got) != 0:
			t.Errorf("%q bound %v, want nothing", tc.target, got)
		case tc.want != "" && (len(got) != 1 || got[0] != held{tc.want, "credvalueqzkx"}):
			t.Errorf("%q bound %v, want one value under %s", tc.target, got, tc.want)
		}
	}
}
