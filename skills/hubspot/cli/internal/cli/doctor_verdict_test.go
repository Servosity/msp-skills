// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"errors"
	"fmt"
	"net/url"
	"testing"

	"hubspot-pp-cli/internal/client"
)

// HubSpot's one public API root is the real endpoint, never a placeholder;
// real placeholders are still refused.
func TestDoctorBaseURLIsPlaceholder_HubSpotRoot(t *testing.T) {
	if doctorRefusesBaseURL("https://api.hubapi.com") {
		t.Error("https://api.hubapi.com is HubSpot's real API root and must not be refused as a placeholder")
	}
	for _, p := range []string{"https://{portal}.hubapi.com", "https://api.example.com", "https://x/YOUR_PORTAL"} {
		if !doctorRefusesBaseURL(p) {
			t.Errorf("%q must be refused as a placeholder", p)
		}
	}
}

// api.hubapi.com answers GET / with a 302 to http://developers.hubspot.com. A
// refused redirect means the server answered, so doctor must go on to probe
// the credential; a real network failure must not.
func TestDoctorReachIsRefusedRedirect(t *testing.T) {
	wrapped := fmt.Errorf("GET /: %w", &url.Error{Op: "Get", URL: "http://developers.hubspot.com", Err: client.ErrRedirectProtocolDowngrade})
	if !doctorReachIsRefusedRedirect(wrapped) {
		t.Error("a refused https->http redirect must count as the server answering")
	}
	if doctorReachIsRefusedRedirect(errors.New("dial tcp: lookup api.hubapi.com: no such host")) {
		t.Error("a DNS failure must stay unreachable")
	}
	if doctorReachIsRefusedRedirect(nil) {
		t.Error("nil is not a refused redirect")
	}
}

// doctorRefusesBaseURL is the condition doctor.go refuses to dial on: the
// press-native literal-placeholder check or this connector's shipped-value one.
func doctorRefusesBaseURL(base string) bool {
	return doctorBaseURLIsPlaceholder(base) || doctorBaseURLIsShippedPlaceholder(base)
}
