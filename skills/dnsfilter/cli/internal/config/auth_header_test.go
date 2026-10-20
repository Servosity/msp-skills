// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import "testing"

// DNSFilter's Authorization header value is the API key itself (vendor spec:
// "The header value is the API key itself. For example: Authorization:
// eyJ..."), so AuthHeader returns the key verbatim with no scheme prefix.
func TestAuthHeaderIsTheRawAPIKey(t *testing.T) {
	c := &Config{DnsfilterApiKey: "eyJraw.key"}
	if got := c.AuthHeader(); got != "eyJraw.key" {
		t.Fatalf("AuthHeader() = %q, want the raw API key", got)
	}
	c = &Config{}
	if got := c.AuthHeader(); got != "" {
		t.Fatalf("AuthHeader() with no key = %q, want empty", got)
	}
}
