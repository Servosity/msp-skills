// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED.

package dsm

import (
	"strings"
	"testing"
)

// TestDisplayAddressStripsCredentialsQueryAndFragment: the one rule every
// printed NAS address goes through drops a user, a password, a query and a
// fragment, even from an address that does not parse.
func TestDisplayAddressStripsCredentialsQueryAndFragment(t *testing.T) {
	for in, want := range map[string]string{
		"https://admin:Password7731@nas:5001":               "https://nas:5001",
		"https://admin:P@ss@word7731@nas:5001/":             "https://nas:5001/",
		"https://admin:pa/ss?w#rd7731@nas:5001":             "https://nas:5001",
		"https://admin:pa ss7731@nas:5001":                  "https://nas:5001",
		"https://nas:5001/?tok=QSECRET44#FRAGSECRET55":      "https://nas:5001/",
		"https://nas:5001#FRAGSECRET55":                     "https://nas:5001",
		"admin:Password7731@nas:5001":                       "nas:5001",
		"  https://nas:5001  ":                              "https://nas:5001",
		"http://10.0.0.20:5000/dsm":                         "http://10.0.0.20:5000/dsm",
		"https://u:Password7731@nas:5001/x?tok=QSECRET44#f": "https://nas:5001/x",
		"": "",
	} {
		got := DisplayAddress(in)
		if got != want {
			t.Errorf("DisplayAddress(%q) = %q, want %q", in, got, want)
		}
		for _, s := range []string{"Password7731", "QSECRET44", "FRAGSECRET55", "rd7731", "ss7731"} {
			if strings.Contains(got, s) {
				t.Errorf("DisplayAddress(%q) kept %q: %q", in, s, got)
			}
		}
	}
}
