// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import "testing"

// config-file-holds-no-credential: config.toml keeps base_url without its
// userinfo and query (the black-box TestConfigFileNeverHoldsACredential runs
// the same class through auth set-token on the built binary).
func TestBaseURLWithoutCredentials(t *testing.T) {
	for in, want := range map[string]string{
		"https://ro:Pw-1c9e@api-us.rocketcyber.com/v3?api_key=K-55b3&tenant=T": "https://api-us.rocketcyber.com/v3",
		"https://api-us.rocketcyber.com/v3":                                    "https://api-us.rocketcyber.com/v3",
		"https://api-us.rocketcyber.com/v3#Frag-9a":                            "https://api-us.rocketcyber.com/v3",
		"https://ro:Pw-1c9e@other.example.com:bad/":                            builtinBaseURL(),
		"https://{tenant}.api-us.rocketcyber.com/v3":                           "https://{tenant}.api-us.rocketcyber.com/v3",
		"": "",
	} {
		if got := baseURLWithoutCredentials(in); got != want {
			t.Errorf("baseURLWithoutCredentials(%q) = %q, want %q", in, got, want)
		}
	}
}
