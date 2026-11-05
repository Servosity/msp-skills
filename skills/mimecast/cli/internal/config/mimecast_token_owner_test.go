// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json mimecast-token-bound-to-client).

package config

import (
	"testing"
	"time"
)

// Adversarial round 1 failing input: two Mimecast API apps (one per customer)
// in the same HOME. A token minted for app A must not be sent when the
// configured client is app B: it would act on customer A's tenant.
func TestStoredTokenIsUsedOnlyByItsClient(t *testing.T) {
	isolateCredentials(t)
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SaveTokens("app-a-client", "app-a-secret", "app-a-token", "", time.Now().Add(30*time.Minute)); err != nil {
		t.Fatal(err)
	}

	t.Setenv("MIMECAST_CLIENT_ID", "app-b-client")
	t.Setenv("MIMECAST_CLIENT_SECRET", "app-b-secret")
	b, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if b.AccessToken != "" || b.RefreshToken != "" || !b.TokenExpiry.IsZero() {
		t.Fatalf("client app-b-client got app A's stored token %q (expiry %v)", b.AccessToken, b.TokenExpiry)
	}
	if got := b.AuthHeader(); got == "Bearer app-a-token" {
		t.Fatalf("auth header carries app A's token for client B: %q", got)
	}

	t.Setenv("MIMECAST_CLIENT_ID", "app-a-client")
	t.Setenv("MIMECAST_CLIENT_SECRET", "app-a-secret")
	a, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if a.AccessToken != "app-a-token" {
		t.Fatalf("client app-a-client lost its own stored token: %q", a.AccessToken)
	}
}
