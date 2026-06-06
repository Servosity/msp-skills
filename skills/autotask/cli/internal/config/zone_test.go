// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0.
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
