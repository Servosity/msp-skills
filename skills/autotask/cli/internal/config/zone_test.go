// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
package config

import "testing"

func TestNormalizeZoneBaseURL(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"zoneInformation form (trailing slash, no version)", "https://webservices5.autotask.net/atservicesrest/", "https://webservices5.autotask.net/atservicesrest/V1.0"},
		{"no trailing slash, no version", "https://webservices5.autotask.net/atservicesrest", "https://webservices5.autotask.net/atservicesrest/V1.0"},
		{"already versioned", "https://webservices5.autotask.net/atservicesrest/V1.0", "https://webservices5.autotask.net/atservicesrest/V1.0"},
		{"already versioned trailing slash", "https://webservices5.autotask.net/atservicesrest/V1.0/", "https://webservices5.autotask.net/atservicesrest/V1.0"},
		{"bare host", "https://webservices26.autotask.net", "https://webservices26.autotask.net/atservicesrest/V1.0"},
		{"lowercase version preserved", "https://webservices2.autotask.net/atservicesrest/v1.0", "https://webservices2.autotask.net/atservicesrest/v1.0"},
		{"whitespace", "  https://webservices5.autotask.net/atservicesrest/  ", "https://webservices5.autotask.net/atservicesrest/V1.0"},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeZoneBaseURL(tc.in); got != tc.want {
				t.Errorf("NormalizeZoneBaseURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// zone-discovery-pinned: only an https webservicesN.autotask.net zone on the
// default port with the /atservicesrest path is accepted from discovery.
func TestValidateZoneURL(t *testing.T) {
	for raw, ok := range map[string]bool{
		"https://webservices14.autotask.net/atservicesrest/":              true,
		"https://webservices1.autotask.net/atservicesrest":                true,
		"https://WEBSERVICES5.Autotask.net/atservicesrest/V1.0/":          true,
		"https://webservices24.autotask.net:443/atservicesrest/":          true,
		"https://webservices.autotask.net/atservicesrest/":                false, // the discovery host
		"http://webservices14.autotask.net/atservicesrest/":               false,
		"https://webservices14.autotask.net.evil.example/atservicesrest/": false,
		"https://evil.example/atservicesrest/":                            false,
		"https://u:p@webservices14.autotask.net/atservicesrest/":          false,
		"https://webservices14.autotask.net:8443/atservicesrest/":         false,
		"https://webservices14.autotask.net/other/":                       false,
		"https://webservices14.autotask.net/atservicesrest/?next=x":       false,
		"https://webservices14.autotаsk.net/atservicesrest/":              false, // Cyrillic a
		"": false,
	} {
		if err := ValidateZoneURL(raw); (err == nil) != ok {
			t.Errorf("%q: accepted=%v, want %v (%v)", raw, err == nil, ok, err)
		}
	}
}
