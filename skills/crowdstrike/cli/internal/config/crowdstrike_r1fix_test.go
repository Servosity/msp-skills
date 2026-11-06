// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

// Codex review round 1 of the crowdstrike 0.1.11 branch, class token
// persistence loses client identity (handfix minted-token-bound-to-its-client):
// SaveTokens is the one save every minted token passes (auth login, the
// automatic mint, the refresh grant), and it keeps the stored pair when the
// pair in effect came from the environment. The blackbox test
// TestR1FixLoginTokenStaysWithItsClient drives auth login on the built binary;
// this one covers every caller at the rule itself.

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"crowdstrike-pp-cli/internal/cliutil"
)

func TestR1FixTokenSavedOnlyBesideItsClient(t *testing.T) {
	isolateHome(t)
	exp := time.Now().Add(time.Hour)
	store := func() string {
		t.Helper()
		p, err := cliutil.CredentialsFilePath()
		if err != nil {
			t.Fatal(err)
		}
		b, _ := os.ReadFile(p)
		return string(b)
	}
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SaveTokens("rfxStoredClientA", "rfxStoredSecretA", "rfxTokenOfA", "", exp); err != nil {
		t.Fatalf("control: saving client A's token beside client A: %v", err)
	}
	// Client B from the environment: what auth login, the automatic mint and
	// the refresh grant would save (each passes the pair in effect).
	t.Setenv("FALCON_CLIENT_ID", "rfxEnvClientB")
	t.Setenv("FALCON_CLIENT_SECRET", "rfxEnvSecretB")
	cfg, err = Load("")
	if err != nil {
		t.Fatal(err)
	}
	err = cfg.SaveTokens(cfg.ClientID, cfg.ClientSecret, "rfxTokenOfB", "", exp)
	if !errors.Is(err, ErrTokenForAnotherClient) {
		t.Errorf("saving client B's token beside stored client A: err = %v, want ErrTokenForAnotherClient", err)
	}
	if s := store(); strings.Contains(s, "rfxTokenOfB") || !strings.Contains(s, "rfxTokenOfA") {
		t.Errorf("the credential store must keep client A's token and never hold B's beside A's pair:\n%s", s)
	}
	// Control: the stored client from the environment still saves its own token.
	t.Setenv("FALCON_CLIENT_ID", "rfxStoredClientA")
	t.Setenv("FALCON_CLIENT_SECRET", "rfxStoredSecretA")
	if cfg, err = Load(""); err != nil {
		t.Fatal(err)
	}
	if err := cfg.SaveTokens(cfg.ClientID, cfg.ClientSecret, "rfxTokenOfA2", "", exp); err != nil {
		t.Errorf("control: client A from the environment saving its own token: %v", err)
	}
	// Control: a token with no minting client (auth set-token) is not checked.
	t.Setenv("FALCON_CLIENT_ID", "rfxEnvClientB")
	t.Setenv("FALCON_CLIENT_SECRET", "rfxEnvSecretB")
	if cfg, err = Load(""); err != nil {
		t.Fatal(err)
	}
	if err := cfg.SaveTokens("", "", "rfxSetToken", "", exp); err != nil {
		t.Errorf("control: auth set-token's save: %v", err)
	}
	// Control: with no stored client, an environment client's token is saved
	// with no client beside it.
	isolateHome(t)
	t.Setenv("FALCON_CLIENT_ID", "rfxEnvClientB")
	t.Setenv("FALCON_CLIENT_SECRET", "rfxEnvSecretB")
	if cfg, err = Load(""); err != nil {
		t.Fatal(err)
	}
	if err := cfg.SaveTokens(cfg.ClientID, cfg.ClientSecret, "rfxTokenOfB", "", exp); err != nil {
		t.Errorf("control: an environment client with nothing stored: %v", err)
	}
	if s := store(); strings.Contains(s, "rfxEnvClientB") || !strings.Contains(s, "rfxTokenOfB") {
		t.Errorf("control: the token is saved with no client beside it:\n%s", s)
	}
}
