// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

// Codex review round 1 of the crowdstrike 4.34.2 reprint, class
// stored-credential-origin (credential sent to another host): every stored
// credential (a token saved by `auth set-token` or a mint, an auth header, a
// refresh token, the client pair) is presented only to the Falcon cloud it was
// recorded for. The credential store records the base URL each credential was
// saved for, and a trusted config.toml's credentials belong to the base URL it
// names; Load used neither, so
//   - `auth set-token` against US-1, then CROWDSTRIKE_BASE_URL=EU-1, sent the
//     US-1 token to EU-1 (no client pair: the token binding never ran);
//   - a config.toml naming EU-1 over a credential store recorded for US-1 (a
//     pair and its minted token) sent the US-1 token to EU-1: the binding
//     compared the configuration with itself.
// A credential recorded for another cloud is refused (and reported), never
// sent; a credential recorded for this cloud is sent (control).

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"crowdstrike-pp-cli/internal/cliutil"
	"crowdstrike-pp-cli/internal/config"
)

// sendOnce loads the configuration and sends one API read; it returns the
// error (a refused credential is an error, never a send).
func sendOnce(t *testing.T) error {
	t.Helper()
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	_, err = New(cfg, 5*time.Second, 0).Get(context.Background(), "/devices/queries/devices/v1", nil)
	return err
}

func authsCarrying(auths []string, tok string) []string {
	var hit []string
	for _, a := range auths {
		if strings.Contains(a, tok) {
			hit = append(hit, a)
		}
	}
	return hit
}

// auth set-token against cloud A, then the base URL names cloud B: B never
// receives A's token. Control: against A it is sent.
func TestR1SetTokenStaysWithItsCloud(t *testing.T) {
	isolateCredentialStore(t)
	a := newBindingMarketplace(t, "a-")
	b := newBindingMarketplace(t, "b-")
	const tokA = "setTokenForCloudA-4c8e1f2a"
	t.Setenv("CROWDSTRIKE_BASE_URL", a.srv.URL)
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	cfg.AuthHeaderVal = ""
	if err := cfg.SaveTokens("", "", tokA, "", time.Time{}); err != nil {
		t.Fatal(err)
	}
	if err := sendOnce(t); err != nil {
		t.Fatalf("control: the token must be sent to the cloud it was saved for: %v", err)
	}
	if _, auths := a.state(); len(authsCarrying(auths, tokA)) != 1 {
		t.Fatalf("control: cloud A must receive its own token once, got %v", auths)
	}
	t.Setenv("CROWDSTRIKE_BASE_URL", b.srv.URL)
	err = sendOnce(t)
	if _, auths := b.state(); len(authsCarrying(auths, tokA)) != 0 {
		t.Errorf("cloud B received the token saved for cloud A: %v", auths)
	}
	if err == nil {
		t.Errorf("a request with no credential for cloud B must fail, not succeed")
	}
}

// config.toml names cloud B; the credential store was recorded for cloud A
// with a client pair and the token minted for it (unexpired). Nothing stored
// for A reaches B: not the token, not a mint with A's pair.
func TestR1StoreRecordedForAnotherCloudIsNotApplied(t *testing.T) {
	home := isolateCredentialStore(t)
	a := newBindingMarketplace(t, "a-")
	b := newBindingMarketplace(t, "b-")
	const id, tokA = "storeClientA-9d1e", "storedTokenForCloudA-2f7b5c1d"
	if err := cliutil.SaveCredentials(&cliutil.Credentials{BaseURL: a.srv.URL, ClientID: id, ClientSecret: "secret-for-" + id,
		AccessToken: tokA, TokenExpiry: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	dir, err := cliutil.ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	cfgFile := filepath.Join(dir, "config.toml")
	write := func(base string) {
		t.Helper()
		if err := os.WriteFile(cfgFile, []byte("base_url = \""+base+"\"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(a.srv.URL)
	if err := sendOnce(t); err != nil {
		t.Fatalf("control: the stored token must be sent to the cloud it was recorded for: %v", err)
	}
	if _, auths := a.state(); len(authsCarrying(auths, tokA)) != 1 {
		t.Fatalf("control: cloud A must receive its own stored token, got %v (home %s)", auths, home)
	}
	write(b.srv.URL)
	_ = sendOnce(t)
	mints, auths := b.state()
	if hit := authsCarrying(auths, tokA); len(hit) != 0 {
		t.Errorf("cloud B received the token stored for cloud A: %v", hit)
	}
	if mints[id] != 0 {
		t.Errorf("cloud B was sent the client pair stored for cloud A (%d mints)", mints[id])
	}
}

// A trusted config.toml holds its own token for the cloud it names; an
// environment base URL naming another cloud does not take it along.
func TestR1ConfigFileTokenStaysWithItsCloud(t *testing.T) {
	isolateCredentialStore(t)
	a := newBindingMarketplace(t, "a-")
	b := newBindingMarketplace(t, "b-")
	const tokA = "configFileTokenForCloudA-8e3a6b0c"
	dir, err := cliutil.ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte("base_url = \""+a.srv.URL+"\"\naccess_token = \""+tokA+"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := sendOnce(t); err != nil {
		t.Fatalf("control: config.toml's token must be sent to the cloud it names: %v", err)
	}
	if _, auths := a.state(); len(authsCarrying(auths, tokA)) != 1 {
		t.Fatalf("control: cloud A must receive config.toml's token, got %v", auths)
	}
	t.Setenv("CROWDSTRIKE_BASE_URL", b.srv.URL)
	_ = sendOnce(t)
	if _, auths := b.state(); len(authsCarrying(auths, tokA)) != 0 {
		t.Errorf("cloud B received the token config.toml holds for cloud A: %v", auths)
	}
}
