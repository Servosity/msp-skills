// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package avanansig

import (
	"net/url"
	"testing"
	"time"
)

// TestSignMatchesVendorWorkedExample pins the algorithm to the worked example
// published in the Avanan API Reference Guide ("Calculating API Request
// Signature (x-av-sig) for Token Generation").
//
// The guide's prose claims the secret is "client_secret", but both the base64
// pre-image and the final signature it prints were produced with
// "my_avanan_secret" — a documentation typo. The published hex is the
// authoritative artifact, so this test reproduces it exactly. If this test ever
// fails, the CLI is signing differently from the vendor's own reference and
// every request will 401.
func TestSignMatchesVendorWorkedExample(t *testing.T) {
	const (
		reqID  = "d290f1ee-6c54-4b01-90e6"
		appID  = "US:myapp29"
		date   = "2021-04-10T00:00:00.000Z"
		secret = "my_avanan_secret"
		want   = "2462b23346ab0642b65d7d094aca5fb4c29fd96d0468deceae2704d258e81497"
	)

	got := Sign(reqID, appID, date, "", secret)
	if got != want {
		t.Fatalf("Sign() = %q, want %q (vendor worked example)", got, want)
	}
}

func TestSign(t *testing.T) {
	tests := []struct {
		name          string
		reqID         string
		appID         string
		date          string
		requestString string
		secret        string
		wantSame      bool // same as the baseline case
	}{
		{name: "baseline", reqID: "r1", appID: "a1", date: "d1", requestString: "", secret: "s1", wantSame: true},
		{name: "identical inputs are deterministic", reqID: "r1", appID: "a1", date: "d1", requestString: "", secret: "s1", wantSame: true},
		{name: "different request id changes signature", reqID: "r2", appID: "a1", date: "d1", requestString: "", secret: "s1"},
		{name: "different app id changes signature", reqID: "r1", appID: "a2", date: "d1", requestString: "", secret: "s1"},
		{name: "different date changes signature", reqID: "r1", appID: "a1", date: "d2", requestString: "", secret: "s1"},
		{name: "different secret changes signature", reqID: "r1", appID: "a1", date: "d1", requestString: "", secret: "s2"},
		{name: "request string changes signature", reqID: "r1", appID: "a1", date: "d1", requestString: "/v1.0/scopes", secret: "s1"},
		{name: "query string changes signature", reqID: "r1", appID: "a1", date: "d1", requestString: "/v1.0/scopes?scope=f%3At", secret: "s1"},
	}

	baseline := Sign("r1", "a1", "d1", "", "s1")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Sign(tt.reqID, tt.appID, tt.date, tt.requestString, tt.secret)
			if len(got) != 64 {
				t.Errorf("Sign() length = %d, want 64 hex chars", len(got))
			}
			if tt.wantSame && got != baseline {
				t.Errorf("Sign() = %q, want baseline %q", got, baseline)
			}
			if !tt.wantSame && got == baseline {
				t.Errorf("Sign() collided with baseline %q; the field is not contributing to the signature", baseline)
			}
		})
	}
}

func TestRequestString(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "path only", raw: "https://host.example/v1.0/scopes", want: "/v1.0/scopes"},
		{name: "path with query", raw: "https://host.example/v1.0/scopes?scope=farm%3Atenant", want: "/v1.0/scopes?scope=farm%3Atenant"},
		{name: "empty path", raw: "https://host.example", want: ""},
		{name: "root path", raw: "https://host.example/", want: "/"},
		{name: "escaped path segment is preserved", raw: "https://host.example/v1.0/exceptions/spam_whitelist/12%2F3", want: "/v1.0/exceptions/spam_whitelist/12%2F3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.raw)
			if err != nil {
				t.Fatalf("url.Parse(%q): %v", tt.raw, err)
			}
			if got := RequestString(u); got != tt.want {
				t.Errorf("RequestString(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}

	if got := RequestString(nil); got != "" {
		t.Errorf("RequestString(nil) = %q, want empty", got)
	}
}

func TestFormatDate(t *testing.T) {
	// A non-UTC input must be normalized before formatting; emitting a local
	// wall-clock time with a Z suffix would silently shift the signed window.
	loc := time.FixedZone("UTC+5", 5*60*60)
	ts := time.Date(2021, 4, 10, 5, 0, 0, 0, loc)

	if got, want := FormatDate(ts), "2021-04-10T00:00:00.000000"; got != want {
		t.Errorf("FormatDate() = %q, want %q", got, want)
	}
}

func TestIsInfinityHost(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{host: "cloudinfra-gw.portal.checkpoint.com", want: true},
		{host: "cloudinfra-gw-us.portal.checkpoint.com", want: true},
		{host: "CLOUDINFRA-GW.PORTAL.CHECKPOINT.COM", want: true},
		{host: "smart-api-production-1-us.avanan.net", want: false},
		{host: "smart-api-production-1-eu.avanan.net", want: false},
		{host: "", want: false},
	}

	for _, tt := range tests {
		if got := IsInfinityHost(tt.host); got != tt.want {
			t.Errorf("IsInfinityHost(%q) = %v, want %v", tt.host, got, tt.want)
		}
	}
}

func TestApplyInfinityPrefix(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "adds prefix", path: "/v1.0/scopes", want: "/app/hec-api/v1.0/scopes"},
		{name: "is idempotent", path: "/app/hec-api/v1.0/scopes", want: "/app/hec-api/v1.0/scopes"},
		{name: "normalizes missing leading slash", path: "v1.0/scopes", want: "/app/hec-api/v1.0/scopes"},
		{name: "leaves empty path alone", path: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ApplyInfinityPrefix(tt.path); got != tt.want {
				t.Errorf("ApplyInfinityPrefix(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestRegionBaseURL(t *testing.T) {
	tests := []struct {
		region string
		want   string
		wantOK bool
	}{
		{region: "us", want: "https://smart-api-production-1-us.avanan.net", wantOK: true},
		{region: "US", want: "https://smart-api-production-1-us.avanan.net", wantOK: true},
		{region: " eu ", want: "https://smart-api-production-1-eu.avanan.net", wantOK: true},
		{region: "uk", want: "https://smart-api-production-1-euw2.avanan.net", wantOK: true},
		{region: "euw2", want: "https://smart-api-production-1-euw2.avanan.net", wantOK: true},
		{region: "uae", want: "https://smart-api-production-1-mec1.avanan.net", wantOK: true},
		{region: "in", want: "https://smart-api-production-1-aps1.avanan.net", wantOK: true},
		{region: "moon", want: "", wantOK: false},
		{region: "", want: "", wantOK: false},
	}

	for _, tt := range tests {
		got, ok := RegionBaseURL(tt.region)
		if ok != tt.wantOK || got != tt.want {
			t.Errorf("RegionBaseURL(%q) = (%q, %v), want (%q, %v)", tt.region, got, ok, tt.want, tt.wantOK)
		}
	}
}

func TestRegionsAreAllResolvable(t *testing.T) {
	regions := Regions()
	if len(regions) != 7 {
		t.Errorf("Regions() returned %d entries, want 7 (the vendor documents seven isolated regions)", len(regions))
	}
	for _, r := range regions {
		if _, ok := RegionBaseURL(r); !ok {
			t.Errorf("Regions() advertises %q but RegionBaseURL cannot resolve it", r)
		}
	}
}

func TestInfinityRegionBaseURL(t *testing.T) {
	tests := []struct {
		region string
		want   string
		wantOK bool
	}{
		{region: "infinity", want: "https://cloudinfra-gw.portal.checkpoint.com", wantOK: true},
		{region: "INFINITY-US", want: "https://cloudinfra-gw-us.portal.checkpoint.com", wantOK: true},
		{region: " infinity-us ", want: "https://cloudinfra-gw-us.portal.checkpoint.com", wantOK: true},
		{region: "us", want: "", wantOK: false},
		{region: "", want: "", wantOK: false},
	}

	for _, tt := range tests {
		got, ok := InfinityRegionBaseURL(tt.region)
		if ok != tt.wantOK || got != tt.want {
			t.Errorf("InfinityRegionBaseURL(%q) = (%q, %v), want (%q, %v)", tt.region, got, ok, tt.want, tt.wantOK)
		}
	}
}

func TestInfinityRegionsAreAllResolvableAndInfinity(t *testing.T) {
	for _, r := range InfinityRegions() {
		base, ok := InfinityRegionBaseURL(r)
		if !ok {
			t.Errorf("InfinityRegions() advertises %q but InfinityRegionBaseURL cannot resolve it", r)
			continue
		}
		// A code that resolves to a host the transport does not treat as
		// Infinity would silently route the bearer exchange through the
		// signed handshake instead.
		u, err := url.Parse(base)
		if err != nil {
			t.Errorf("InfinityRegionBaseURL(%q) = %q, which does not parse: %v", r, base, err)
			continue
		}
		if !IsInfinityHost(u.Host) {
			t.Errorf("InfinityRegionBaseURL(%q) = %q, which IsInfinityHost rejects", r, base)
		}
	}
}

func TestLegacyAndInfinityRegionCodesDoNotOverlap(t *testing.T) {
	for _, r := range Regions() {
		if _, ok := InfinityRegionBaseURL(r); ok {
			t.Errorf("region %q resolves as both legacy and Infinity; --region resolution order would decide silently", r)
		}
	}
}

// TestIsInfinityHostRejectsLookalikeHosts covers the classification leg of the
// credential-disclosure path. IsInfinityHost decides which handshake runs, and
// the Infinity handshake POSTs the raw client secret as {"accessKey": ...}. A
// substring test for "cloudinfra" hands that to any host that merely contains
// the word, so these are the cases that must come back false.
func TestIsInfinityHostRejectsLookalikeHosts(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		// Real gateways, including the regional forms.
		{"cloudinfra-gw.portal.checkpoint.com", true},
		{"cloudinfra-gw-us.portal.checkpoint.com", true},
		{"cloudinfra-gw.ap.portal.checkpoint.com", true},
		{"CLOUDINFRA-GW.PORTAL.CHECKPOINT.COM", true},
		{"cloudinfra-gw.portal.checkpoint.com:443", true},
		{"cloudinfra-gw.portal.checkpoint.com.", true},

		// The disclosure cases. Every one of these was true under the
		// substring test.
		{"cloudinfra.attacker.example", false},
		{"cloudinfra-gw.portal.checkpoint.com.attacker.example", false},
		{"attacker-cloudinfra.example", false},
		{"cloudinfra.portal.checkpoint.com.evil.test", false},

		// Domain-suffix confusion: ends with the right string but is a
		// different domain.
		{"evil-portal.checkpoint.com", false},
		{"notportal.checkpoint.com", false},

		// Legacy hosts must stay legacy, or they get the bearer exchange.
		{"smart-api-production-1-us.avanan.net", false},
		{"", false},
	}

	for _, tt := range tests {
		if got := IsInfinityHost(tt.host); got != tt.want {
			t.Errorf("IsInfinityHost(%q) = %v, want %v", tt.host, got, tt.want)
		}
	}
}
