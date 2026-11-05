// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"

	"zoom-phone-pp-cli/internal/client"
	"zoom-phone-pp-cli/internal/config"
)

func TestZoomRequireSecureURLIsConfigError(t *testing.T) {
	if err := zoomRequireSecureURL("token URL", "http://evil.example/oauth/token"); err == nil || ExitCode(err) != 10 {
		t.Fatalf("plain http token URL: err=%v exit=%d, want config error 10", err, ExitCode(err))
	}
	if err := zoomRequireSecureURL("base URL", "http://127.0.0.1:9/v2"); err != nil {
		t.Fatalf("loopback refused: %v", err)
	}
}

func TestZoomRefuseEnvAppMismatch(t *testing.T) {
	t.Setenv("ZOOM_PHONE_CLIENT_ID", "cid-B")
	if err := zoomRefuseEnvAppMismatch("cid-A"); err == nil || ExitCode(err) != 10 {
		t.Fatalf("env app B over saved app A must be refused, got %v", err)
	}
	if err := zoomRefuseEnvAppMismatch("cid-B"); err != nil {
		t.Fatalf("same app refused: %v", err)
	}
	if err := zoomRefuseEnvAppMismatch(""); err != nil {
		t.Fatalf("no saved profile refused: %v", err)
	}
	t.Setenv("ZOOM_PHONE_CLIENT_ID", "")
	if err := zoomRefuseEnvAppMismatch("cid-A"); err != nil {
		t.Fatalf("no env app refused: %v", err)
	}
}

func TestZoomRefuseEnvAccountMismatch(t *testing.T) {
	t.Setenv("ZOOM_PHONE_ACCOUNT_ID", "acct-B")
	if err := zoomRefuseEnvAccountMismatch("acct-A", true); err == nil || ExitCode(err) != 10 {
		t.Fatalf("env account B over saved account A with a cached token must be refused, got %v", err)
	}
	if err := zoomRefuseEnvAccountMismatch("acct-A", false); err != nil {
		t.Fatalf("no saved app or token: nothing to mix, got %v", err)
	}
	if err := zoomRefuseEnvAccountMismatch("acct-B", true); err != nil {
		t.Fatalf("same account refused: %v", err)
	}
	if err := zoomRefuseEnvAccountMismatch("", true); err != nil {
		t.Fatalf("no saved account refused: %v", err)
	}
}

// Codex review r1 (class store-scope-identity): a profile whose two client-id
// fields name different Zoom apps is refused before any API call, so its token
// can never pair with the other app's data.
func TestZoomRefuseConflictingAppAliases(t *testing.T) {
	t.Setenv("ZOOM_PHONE_CLIENT_ID", "")
	t.Setenv("ZOOM_PHONE_ACCOUNT_ID", "")
	c := &client.Client{Config: &config.Config{BaseURL: "https://api.zoom.us/v2", ClientID: "cid-B", ZoomPhoneClientId: "cid-A"}}
	if err := ApplyClientHooks(c); err == nil || ExitCode(err) != 10 {
		t.Fatalf("conflicting client_id / phone_client_id accepted: %v", err)
	}
	ok := &client.Client{Config: &config.Config{BaseURL: "https://api.zoom.us/v2", ClientID: "cid-A", ZoomPhoneClientId: "cid-A"}}
	if err := ApplyClientHooks(ok); err != nil {
		t.Fatalf("agreeing aliases refused: %v", err)
	}
}
