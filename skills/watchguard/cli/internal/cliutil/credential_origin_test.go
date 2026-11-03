// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import (
	"net/url"
	"testing"
)

// TestCredentialOriginsMatchComparesHostsLikeTheNetwork (handfixes.json
// stored-credential-origin-exact-host): the stored-credential comparator uses
// the one host rule SameOrigin uses. An IP literal is the same address with
// its IPv6 zone compared exactly (fe80::1%lan0 and fe80::1%LAN0 are two
// links on Linux), an ASCII DNS name ignores case, a name with any non-ASCII
// byte must match exactly; scheme, default port and trailing slash fold, and
// the path must match.
func TestCredentialOriginsMatchComparesHostsLikeTheNetwork(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"https://[fe80::1%25lan0]", "https://[fe80::1%25LAN0]", false},
		{"https://[fe80::1%25lan0]", "https://[fe80::1%25lan0]", true},
		{"https://[fe80::1%25lan0]", "https://[fe80::1%25lan0]:443/", true},
		{"https://[fe80::1%25lan0]", "https://[fe80::1]", false},
		{"https://[2001:DB8::1]", "https://[2001:db8:0::1]", true},
		{"https://API.usa.cloud.watchguard.com", "https://api.usa.cloud.watchguard.com/", true},
		{"HTTPS://api.usa.cloud.watchguard.com:443", "https://api.usa.cloud.watchguard.com", true},
		{"https://api.usa.cloud.watchguard.com", "https://api.deu.cloud.watchguard.com", false},
		{"https://api.usa.cloud.watchguard.com", "http://api.usa.cloud.watchguard.com", false},
		{"https://api.usa.cloud.watchguard.com", "https://api.usa.cloud.watchguard.com:8443", false},
		{"https://api.usa.cloud.watchguard.com/a", "https://api.usa.cloud.watchguard.com/b", false},
		{"https://bücher.example", "https://BÜCHER.example", false},
		{"https://bücher.example", "https://bücher.example", true},
		{"https://127.0.0.1", "https://[::ffff:127.0.0.1]", false},
		{"https://{region}.example", "https://{region}.example", false},
		{"ftp://example.com", "ftp://example.com", false},
	}
	for _, c := range cases {
		if got := CredentialOriginsMatch(c.a, c.b); got != c.want {
			t.Errorf("CredentialOriginsMatch(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
		if got := CredentialOriginsMatch(c.b, c.a); got != c.want {
			t.Errorf("CredentialOriginsMatch(%q, %q) = %v, want %v (reversed)", c.b, c.a, got, c.want)
		}
		ka, okA := NormalizeCredentialOrigin(c.a)
		kb, okB := NormalizeCredentialOrigin(c.b)
		if keysMatch := okA && okB && ka == kb; keysMatch != c.want {
			t.Errorf("NormalizeCredentialOrigin keys %q and %q: equal=%v, want %v", ka, kb, keysMatch, c.want)
		}
	}
}

// TestHostComparatorsAgreeWithSameHost (handfixes.json
// stored-credential-origin-exact-host): every host decision in this package
// agrees with SameHost on the inputs that separate the rules.
func TestHostComparatorsAgreeWithSameHost(t *testing.T) {
	hosts := []string{"fe80::1%lan0", "fe80::1%LAN0", "fe80::1", "FE80::1%lan0", "2001:db8::1", "2001:DB8:0::1",
		"127.0.0.1", "::ffff:127.0.0.1", "api.example.com", "API.Example.COM", "bücher.example", "BÜCHER.example", "K.example", "K.example"}
	for _, a := range hosts {
		for _, b := range hosts {
			want := SameHost(a, b)
			if got := oauthTokenSameHost(a, b); got != want {
				t.Errorf("oauthTokenSameHost(%q, %q) = %v, SameHost = %v", a, b, got, want)
			}
			if got := CanonicalHost(a) == CanonicalHost(b); got != want {
				t.Errorf("CanonicalHost(%q) == CanonicalHost(%q) is %v, SameHost = %v", a, b, got, want)
			}
			ua := &url.URL{Scheme: "https", Host: bracketIPv6(a)}
			ub := &url.URL{Scheme: "https", Host: bracketIPv6(b)}
			if got := SameOrigin(ua, ub); got != want {
				t.Errorf("SameOrigin(%s, %s) = %v, SameHost = %v", ua, ub, got, want)
			}
		}
	}
}

func bracketIPv6(h string) string {
	for i := 0; i < len(h); i++ {
		if h[i] == ':' {
			return "[" + h + "]"
		}
	}
	return h
}
