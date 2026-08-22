// Copyright 2026 Abhi Saini and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored. Kept in its own file so `generate --force` preserves it.
//
// Regression cover for the env-supplied client secret leaking to disk.
//
// #268 established the contract: a credential supplied through the environment
// is never written to the config or the credentials file. It is enforced by two
// guards that both read Config.envOverrides, configForSave() and
// updateFileConfigField(). SaveTokens cleared the ClientID and ClientSecret
// markers before invoking either, so `auth login` persisted the operator's
// long-lived client secret into credentials.toml while doctor still reported
// the source as env:*_CLIENT_SECRET. An operator who deliberately keeps
// secrets in session-scoped environment variables got a copy on disk anyway.
//
// The negative control matters as much as the main case: when the secret is
// passed explicitly rather than through the environment, persisting it is the
// correct and expected behaviour, and this test proves the fix did not simply
// disable saving.

package config

import (
	"testing"
	"time"

	"immybot-pp-cli/internal/cliutil"
)

const (
	envSuppliedSecret      = "ENV-SUPPLIED-CLIENT-SECRET-DO-NOT-PERSIST"
	explicitSuppliedSecret = "EXPLICIT-CLIENT-SECRET-SAVE-IS-CORRECT"
	mintedAccessToken      = "MINTED-ACCESS-TOKEN"
)

// TestSaveTokens_KeepsEnvSuppliedClientSecretOffDisk is the regression: an
// env-supplied client secret must not reach the credentials file when
// `auth login` mints a token.
func TestSaveTokens_KeepsEnvSuppliedClientSecretOffDisk(t *testing.T) {
	clearCredEnv(t)
	t.Setenv("IMMYBOT_CLIENT_ID", "env-client-id")
	t.Setenv("IMMYBOT_CLIENT_SECRET", envSuppliedSecret)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if !cfg.envOverrides["ClientSecret"] {
		t.Fatal("precondition failed: ClientSecret should be marked as env-supplied after Load")
	}

	// This is what `auth login` does once Entra returns a token.
	if err := cfg.SaveTokens(cfg.ClientID, cfg.ClientSecret, mintedAccessToken, "", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("SaveTokens error: %v", err)
	}

	creds, found, err := cliutil.LoadCredentials()
	if err != nil {
		t.Fatalf("LoadCredentials error: %v", err)
	}
	if !found {
		t.Fatal("expected a credentials file: the minted access token still has to persist")
	}
	if creds.ClientSecret == envSuppliedSecret {
		t.Error("env-supplied client secret was written to the credentials file; #268 contract broken")
	}
	if creds.ClientID == "env-client-id" {
		t.Error("env-supplied client id was written to the credentials file; #268 contract broken")
	}
	// The token is the whole point of login, so it must survive.
	if creds.AccessToken != mintedAccessToken {
		t.Errorf("minted access token should persist, got %q", creds.AccessToken)
	}
	// The in-memory config still needs the secret for the current process.
	if cfg.ClientSecret != envSuppliedSecret {
		t.Error("in-memory config must keep the env secret so the running command can use it")
	}
}

// TestSaveTokens_PersistsExplicitlyPassedClientSecret is the negative control.
// With no environment variable in play the operator passed the secret on the
// command line, and saving it is the documented behaviour of `auth login`.
func TestSaveTokens_PersistsExplicitlyPassedClientSecret(t *testing.T) {
	clearCredEnv(t)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.envOverrides["ClientSecret"] {
		t.Fatal("precondition failed: no env var was set, so ClientSecret must not be marked")
	}

	if err := cfg.SaveTokens("explicit-client-id", explicitSuppliedSecret, mintedAccessToken, "", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("SaveTokens error: %v", err)
	}

	creds, found, err := cliutil.LoadCredentials()
	if err != nil {
		t.Fatalf("LoadCredentials error: %v", err)
	}
	if !found {
		t.Fatal("expected a credentials file")
	}
	if creds.ClientSecret != explicitSuppliedSecret {
		t.Errorf("explicitly supplied secret should persist, got %q", creds.ClientSecret)
	}
	if creds.ClientID != "explicit-client-id" {
		t.Errorf("explicitly supplied client id should persist, got %q", creds.ClientID)
	}
}
