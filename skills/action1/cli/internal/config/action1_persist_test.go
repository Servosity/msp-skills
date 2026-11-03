// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import "testing"

// config-file-holds-no-credential: config.toml keeps base_url without its
// userinfo and query (the black-box TestConfigFileNeverHoldsACredential runs
// the same class through auth set-token on the built binary).
func TestBaseURLWithoutCredentials(t *testing.T) {
	for in, want := range map[string]string{
		"https://ro:Pw-1c9e@app.useaction1.com/api?api_key=K-55b3&tenant=T": "https://app.useaction1.com/api",
		"https://app.useaction1.com/api":                                    "https://app.useaction1.com/api",
		"https://app.useaction1.com/api#Frag-9a":                            "https://app.useaction1.com/api",
		"https://ro:Pw-1c9e@other.example.com:bad/":                         builtinBaseURL(),
		"https://{tenant}.useaction1.com/api":                               "https://{tenant}.useaction1.com/api",
		"":                                                                  "",
	} {
		if got := baseURLWithoutCredentials(in); got != want {
			t.Errorf("baseURLWithoutCredentials(%q) = %q, want %q", in, got, want)
		}
	}
}
