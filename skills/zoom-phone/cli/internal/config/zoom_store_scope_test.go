// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"os"
	"path/filepath"
	"testing"
	"zoom-phone-pp-cli/internal/cliutil"
	"zoom-phone-pp-cli/internal/cliutil/testenv"
)

func TestStoreScopeFollowsAuthenticatingApp(t *testing.T) {
	// Before the first mint the app comes from the environment (ZoomPhoneClientId);
	// the mint then copies it into ClientID. The scope must not move.
	c := &Config{BaseURL: "https://api.zoom.us/v2", ZoomPhoneAccountId: "acct-1", ZoomPhoneClientId: "cid-A", ZoomPhoneClientSecret: "s1"}
	before := c.StoreScopeCredential()
	c.ClientID, c.ClientSecret, c.AccessToken, c.ZoomPhoneClientSecret = "cid-A", "s2", "tok", "s2"
	// Round-1 fix (class store-scope-identity): the fixture's mint now
	// records the binding, as both real mints do (ZoomRecordMint).
	c.ZoomRecordMint("acct-1", "cid-A", "tok")
	if after := c.StoreScopeCredential(); after != before || before == "" {
		t.Fatalf("store scope changed across mint/rotation: %q -> %q", before, after)
	}
	// A saved profile with no environment uses its saved app and account.
	profile := &Config{BaseURL: "https://api.zoom.us/v2", ClientID: "cid-A", ZoomPhoneAccountId: "acct-1"}
	if profile.StoreScopeCredential() != before {
		t.Fatal("saved profile and env session for the same app must share a store")
	}
	// Environment naming another app wins over the saved one, like minting does.
	switched := &Config{BaseURL: "https://api.zoom.us/v2", ClientID: "cid-A", ZoomPhoneClientId: "cid-B", ZoomPhoneAccountId: "acct-1"}
	if switched.StoreScopeCredential() == before {
		t.Fatal("tenant switch through the environment reused tenant A's store")
	}
}

// Codex review r1 (class store-scope-identity: two client-id aliases). A
// profile whose phone_client_id names app A while client_id (the app that
// mints, saved by auth login) names app B must not open A's store under B's
// token. Conflicting aliases select neither tenant's store.
func TestStoreScopeConflictingAppAliases(t *testing.T) {
	dir := testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	for _, k := range []string{"ZOOM_PHONE_CLIENT_ID", "ZOOM_PHONE_CLIENT_SECRET", "ZOOM_PHONE_ACCOUNT_ID"} {
		t.Setenv(k, "")
	}
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("base_url = \"https://api.zoom.us/v2\"\nphone_client_id = \"cid-A\"\nclient_id = \"cid-B\"\nclient_secret = \"secret-B\"\nphone_account_id = \"acct-B\"\naccess_token = \"tok-B\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ClientID != "cid-B" || loaded.ZoomPhoneClientId != "cid-A" {
		t.Fatalf("fixture did not load both aliases: client_id=%q phone_client_id=%q", loaded.ClientID, loaded.ZoomPhoneClientId)
	}
	scopeA := (&Config{BaseURL: "https://api.zoom.us/v2", ClientID: "cid-A"}).StoreScopeCredential()
	scopeB := (&Config{BaseURL: "https://api.zoom.us/v2", ClientID: "cid-B"}).StoreScopeCredential()
	got := loaded.StoreScopeCredential()
	if got == scopeA {
		t.Fatal("client_id B authenticates but the store selected is app A's")
	}
	if got == scopeB || got == "" {
		t.Fatalf("conflicting client-id aliases must select neither tenant's store, got %q", got)
	}
	if app, conflict := loaded.ZoomAppIdentity(); !conflict || app != "" {
		t.Fatalf("ZoomAppIdentity() = %q, %v; want a conflict", app, conflict)
	}
	if app, conflict := (&Config{ClientID: "cid-A", ZoomPhoneClientId: "cid-A"}).ZoomAppIdentity(); conflict || app != "cid-A" {
		t.Fatalf("agreeing aliases: %q, %v", app, conflict)
	}
}
