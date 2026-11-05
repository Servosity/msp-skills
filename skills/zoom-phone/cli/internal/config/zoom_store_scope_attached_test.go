// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// zoom-phone hand-fix: zoom-phone-store-scope-account-id (class
// store-scope-identity, Codex review r1 of the onboard). The store scope keys
// on the credential actually attached to requests: a credential this CLI did
// not mint for (account, app) is keyed by its own hash, whatever app or
// account aliases sit beside it.

// The round-1 input: a private config with client_id="app-A",
// phone_account_id="acct-A" and auth_header="Bearer B" (a valid account-B
// token) selected app A's store, so `sync --full --resources users` wrote B's
// GET /phone/users rows into it.
func TestStoreScopeRawCredentialNeverKeysAppStore(t *testing.T) {
	dir := zoomScopeTestEnv(t)
	appA := (&Config{BaseURL: "https://api.zoom.us/v2", ClientID: "app-A", ZoomPhoneAccountId: "acct-A"}).StoreScopeCredential()
	cases := map[string]string{
		"auth_header":                 "client_id = \"app-A\"\nphone_account_id = \"acct-A\"\nauth_header = \"Bearer B\"\n",
		"auth_header+phone_client_id": "phone_client_id = \"app-A\"\nphone_account_id = \"acct-A\"\nauth_header = \"Bearer B\"\n",
		"access_token, no mint":       "client_id = \"app-A\"\nphone_account_id = \"acct-A\"\naccess_token = \"B\"\n",
		"access_token+phone_client":   "phone_client_id = \"app-A\"\nphone_account_id = \"acct-A\"\naccess_token = \"B\"\n",
	}
	scopes := map[string]string{}
	for name, body := range cases {
		path := filepath.Join(dir, strings.NewReplacer(" ", "_", ",", "", "+", "_").Replace(name)+".toml")
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		got := cfg.StoreScopeCredential()
		if got == "" || got == appA {
			t.Fatalf("%s: token B selects app A's store (%q)", name, got)
		}
		if strings.Contains(got, "Bearer B") || strings.Contains(got, "\nB") {
			t.Fatalf("%s: the scope carries the raw credential: %q", name, got)
		}
		scopes[name] = got
	}
	// The scope is the credential's own: the same credential, whatever aliases
	// sit beside it, shares one store.
	if scopes["auth_header"] != scopes["auth_header+phone_client_id"] {
		t.Fatalf("one raw auth header keyed two stores: %q vs %q", scopes["auth_header"], scopes["auth_header+phone_client_id"])
	}
	if scopes["access_token, no mint"] != scopes["access_token+phone_client"] {
		t.Fatalf("one unminted token keyed two stores: %q vs %q", scopes["access_token, no mint"], scopes["access_token+phone_client"])
	}
	// auth_header "Bearer B" and access_token "B" attach the same header.
	if scopes["auth_header"] != scopes["access_token, no mint"] {
		t.Fatalf("one attached credential keyed two stores: %q vs %q", scopes["auth_header"], scopes["access_token, no mint"])
	}
}

// The mint record: saved with the token in the 0600 credential store, it keeps
// a minted token on its (account, app) stable scope across a reload; the same
// token read under another account is keyed by its own hash; the record does
// not reveal the token; logout clears it.
func TestStoreScopeMintRecordRoundTrip(t *testing.T) {
	dir := zoomScopeTestEnv(t)
	path := filepath.Join(dir, "config.toml")
	t.Setenv("ZOOM_PHONE_CLIENT_ID", "cid-M")
	t.Setenv("ZOOM_PHONE_CLIENT_SECRET", "secret-M-0001")
	t.Setenv("ZOOM_PHONE_ACCOUNT_ID", "acct-M")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	stable := cfg.StoreScopeCredential()
	cfg.ZoomPersistAccountID()
	cfg.ZoomRecordMint("acct-M", "cid-M", "minted-token-M-0001")
	if err := cfg.SaveTokens("cid-M", "secret-M-0001", "minted-token-M-0001", "", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(cfg.ZoomMintRecord, "minted-token-M-0001") || cfg.ZoomMintRecord == "" {
		t.Fatalf("mint record = %q", cfg.ZoomMintRecord)
	}
	for _, k := range []string{"ZOOM_PHONE_CLIENT_ID", "ZOOM_PHONE_CLIENT_SECRET"} {
		t.Setenv(k, "")
	}
	reloaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.ZoomMintRecord != cfg.ZoomMintRecord {
		t.Fatal("the mint record was not saved with the token")
	}
	if got := reloaded.StoreScopeCredential(); got != stable {
		t.Fatalf("a recorded mint left its stable scope: %q -> %q", stable, got)
	}
	t.Setenv("ZOOM_PHONE_ACCOUNT_ID", "acct-other")
	other, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	otherStable := (&Config{BaseURL: other.BaseURL, ClientID: "cid-M", ZoomPhoneAccountId: "acct-other"}).StoreScopeCredential()
	// The saved secret can mint for acct-other, and the client would do that
	// before sending anything, so the scope is acct-other's and never acct-M's.
	if got := other.StoreScopeCredential(); got != otherStable {
		t.Fatalf("a token minted for acct-M, with a secret to re-mint, keyed %q under acct-other, want %q", got, otherStable)
	}
	// Without a secret the old token is what gets attached, so it keys its own store.
	other.ClientSecret = ""
	if got := other.StoreScopeCredential(); got == stable || got == otherStable || !strings.Contains(got, "zoom_credential=") {
		t.Fatalf("a token minted for acct-M, with no secret, keyed %q under acct-other", got)
	}
	if err := reloaded.ClearTokens(); err != nil {
		t.Fatal(err)
	}
	if reloaded.ZoomMintRecord != "" {
		t.Fatal("logout kept the mint record")
	}
}

// The round-2 input (mint-transition), at the scope: app credentials that can
// mint beside a credential they did not mint select the minted store, and the
// client agrees it must mint first. The same credential with no secret to mint
// keeps its own store.
func TestStoreScopeMintReplacesForeignCredential(t *testing.T) {
	dir := zoomScopeTestEnv(t)
	appA := (&Config{BaseURL: "https://api.zoom.us/v2", ClientID: "app-A", ZoomPhoneAccountId: "acct-A"}).StoreScopeCredential()
	cases := map[string]string{
		"stale token, no expiry": "access_token = \"B\"\n",
		"stale token, expiry":    "access_token = \"B\"\ntoken_expiry = 2099-01-01T00:00:00Z\n",
		"raw header":             "auth_header = \"Bearer B\"\n",
	}
	for name, extra := range cases {
		for _, secret := range []bool{true, false} {
			body := "client_id = \"app-A\"\nphone_account_id = \"acct-A\"\n" + extra
			if secret {
				body = "client_secret = \"secret-A\"\n" + body
			}
			path := filepath.Join(dir, "scope.toml")
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if got := cfg.ZoomMintReplacesCredential(); got != secret {
				t.Fatalf("%s, secret=%v: ZoomMintReplacesCredential = %v", name, secret, got)
			}
			if got := cfg.StoreScopeCredential(); (got == appA) != secret {
				t.Fatalf("%s, secret=%v: scope %q, app A's is %q", name, secret, got, appA)
			}
		}
	}
}
