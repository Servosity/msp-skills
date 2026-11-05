// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"zoom-phone-pp-cli/internal/cliutil"
	"zoom-phone-pp-cli/internal/cliutil/testenv"
)

// zoom-phone hand-fix: zoom-phone-store-scope-full-identity. The local store
// is keyed by the full scope identity (API host, Zoom account and app), and a
// credential this CLI did not mint for that app is keyed by itself, never by a
// partial key or a leftover alias.

func zoomScopeTestEnv(t *testing.T) string {
	t.Helper()
	// The press sandbox: HOME and USERPROFILE (Windows resolves the data dir
	// from it), with every directory lookup proven inside the throwaway home.
	dir := testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	for _, k := range []string{"ZOOM_PHONE_CLIENT_ID", "ZOOM_PHONE_CLIENT_SECRET", "ZOOM_PHONE_ACCOUNT_ID", "ZOOM_PHONE_BASE_URL", "ZOOM_PHONE_TOKEN_URL"} {
		t.Setenv(k, "")
	}
	return dir
}

// Codex review r2 (open P1, class store-scope-identity): auth set-token with
// tenant B's token cleared client_id but kept phone_client_id/phone_account_id,
// so app A's store stayed selected under B's token.
func TestStoreScopeSetTokenNeverKeepsAppStore(t *testing.T) {
	dir := zoomScopeTestEnv(t)
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("base_url = \"https://api.zoom.us/v2\"\nclient_id = \"cid-A\"\nphone_client_id = \"cid-A\"\nphone_account_id = \"acct-A\"\nclient_secret = \"secret-A\"\naccess_token = \"tok-A\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	scopeA := cfg.StoreScopeCredential()
	if scopeA == "" {
		t.Fatal("app A's profile has no store scope")
	}
	if err := cfg.SaveTokens("", "", "tenant-B-token", "", time.Time{}); err != nil { // what auth set-token does
		t.Fatal(err)
	}
	after, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	got := after.StoreScopeCredential()
	if got == scopeA || got == "" {
		t.Fatalf("tenant B's token selects scope %q (app A's is %q)", got, scopeA)
	}
	if strings.Contains(got, "tenant-B-token") {
		t.Fatalf("the scope identity carries the raw token: %q", got)
	}
	// A different bare token never shares B's store either.
	other := &Config{BaseURL: "https://api.zoom.us/v2", AccessToken: "tenant-C-token", ZoomPhoneClientId: "cid-A", ZoomPhoneAccountId: "acct-A"}
	if other.StoreScopeCredential() == got || other.StoreScopeCredential() == scopeA {
		t.Fatal("two bare tokens (or a bare token and app A) share a store")
	}
}

func TestStoreScopeKeysAccountAndApp(t *testing.T) {
	base := "https://api.zoom.us/v2"
	full := func(acct, app string) string {
		return (&Config{BaseURL: base, ZoomPhoneAccountId: acct, ZoomPhoneClientId: app}).StoreScopeCredential()
	}
	a1 := full("acct-1", "cid-A")
	if a1 == "" {
		t.Fatal("a complete identity has no scope")
	}
	if full("acct-2", "cid-A") == a1 {
		t.Fatal("two Zoom accounts behind one app share a store")
	}
	if full("acct-1", "cid-B") == a1 {
		t.Fatal("two apps on one account share a store")
	}
	if (&Config{BaseURL: "https://api.zoomgov.com/v2", ZoomPhoneAccountId: "acct-1", ZoomPhoneClientId: "cid-A"}).StoreScopeCredential() == a1 {
		t.Fatal("two API hosts share a store")
	}
	// Partial identities never share the complete one, nor each other.
	appOnly := (&Config{BaseURL: base, ZoomPhoneClientId: "cid-A"}).StoreScopeCredential()
	acctOnly := (&Config{BaseURL: base, ZoomPhoneAccountId: "acct-1"}).StoreScopeCredential()
	savedAppOnly := (&Config{BaseURL: base, ClientID: "cid-A"}).StoreScopeCredential()
	for name, s := range map[string]string{"app only": appOnly, "account only": acctOnly, "saved app only": savedAppOnly} {
		if s == a1 {
			t.Fatalf("%s shares the complete identity's store", name)
		}
	}
	if appOnly == acctOnly {
		t.Fatal("app-only and account-only share a store")
	}
	// The minted token stays bound to its app and account: the scope does not
	// move when the token rotates. Round-1 fix (class store-scope-identity):
	// this fixture set AccessToken with no mint record and called it minted,
	// which is the contract the finding broke; a mint now records the binding
	// (ZoomRecordMint) and each re-mint records it again.
	minted := &Config{BaseURL: base, ClientID: "cid-A", ZoomPhoneClientId: "cid-A", ZoomPhoneAccountId: "acct-1", AccessToken: "tok-1"}
	minted.ZoomRecordMint("acct-1", "cid-A", "tok-1")
	if minted.StoreScopeCredential() != a1 {
		t.Fatal("a minted token left its app's store")
	}
	minted.AccessToken = "tok-2"
	minted.ZoomRecordMint("acct-1", "cid-A", "tok-2")
	if minted.StoreScopeCredential() != a1 {
		t.Fatal("token rotation moved the store")
	}
}

// The first run on environment credentials mints and saves the token; the next
// run (profile only) must open the same store.
func TestStoreScopeEnvMintThenProfile(t *testing.T) {
	dir := zoomScopeTestEnv(t)
	path := filepath.Join(dir, "config.toml")
	t.Setenv("ZOOM_PHONE_CLIENT_ID", "cid-A")
	t.Setenv("ZOOM_PHONE_CLIENT_SECRET", "secret-A")
	t.Setenv("ZOOM_PHONE_ACCOUNT_ID", "acct-A")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	before := cfg.StoreScopeCredential()
	cfg.ZoomPersistAccountID()
	cfg.ZoomRecordMint("acct-A", "cid-A", "tok-A")                                                      // round-1 fix: the automatic mint records its binding before SaveTokens
	if err := cfg.SaveTokens("cid-A", "secret-A", "tok-A", "", time.Now().Add(time.Hour)); err != nil { // the automatic mint
		t.Fatal(err)
	}
	for _, k := range []string{"ZOOM_PHONE_CLIENT_ID", "ZOOM_PHONE_CLIENT_SECRET", "ZOOM_PHONE_ACCOUNT_ID"} {
		t.Setenv(k, "")
	}
	after, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := after.StoreScopeCredential(); got != before || before == "" {
		t.Fatalf("env session %q and its saved profile %q use different stores", before, got)
	}
}

// zoom-phone hand-fix: credkit-wiring. Loading a profile holds its
