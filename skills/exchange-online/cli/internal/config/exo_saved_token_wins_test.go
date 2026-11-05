// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json saved-token-wins): a token
// saved by `auth set-token` (or any SaveTokens caller) is the one the next run
// sends, even when an older stored slot that AuthHeader ranks above
// access_token (online_token, auth_header) is in the config.

package config

import (
	"os"
	"path/filepath"
	"testing"

	"exchange-online-pp-cli/internal/cliutil"
)

func TestSavedTokenOutranksOlderStoredSlots(t *testing.T) {
	for name, body := range map[string]string{
		"online_token": "online_token = \"eyJhbGciOiJub25lIn0.eyJleHAiOjF9.c2ln\"\n",
		"auth_header":  "auth_header = \"Bearer OlderStoredHeaderValue\"\n",
	} {
		t.Run(name, func(t *testing.T) {
			isolateCredentials(t)
			dir, err := cliutil.ConfigDir()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			p := filepath.Join(dir, configFileName())
			if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load(p)
			if err != nil {
				t.Fatal(err)
			}
			const fresh = "eyJhbGciOiJub25lIn0.eyJleHAiOjQxMDI0NDQ4MDB9.c2ln"
			cfg.AuthHeaderVal = "" // what auth set-token does before SaveTokens
			if err := cfg.SaveTokens("", "", fresh, "", cfg.TokenExpiry); err != nil {
				t.Fatal(err)
			}
			if got := cfg.AuthHeader(); got != "Bearer "+fresh {
				t.Fatalf("same run sends %q, want the saved token", got)
			}
			again, err := Load(p)
			if err != nil {
				t.Fatal(err)
			}
			if got := again.AuthHeader(); got != "Bearer "+fresh {
				t.Fatalf("next run sends %q, want the saved token", got)
			}
		})
	}
}

// The environment still wins on its own run, and saving never copies it to disk.
func TestSavedTokenKeepsTheEnvironmentOutOfTheFile(t *testing.T) {
	isolateCredentials(t)
	dir, err := cliutil.ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, configFileName())
	if err := os.WriteFile(p, []byte("online_token = \"OlderStoredOnlineToken1\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EXCHANGE_ONLINE_TOKEN", "EnvironmentTokenThisRun9")
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SaveTokens("", "", "SavedTokenFromSetToken7", "", cfg.TokenExpiry); err != nil {
		t.Fatal(err)
	}
	if got := cfg.AuthHeader(); got != "Bearer EnvironmentTokenThisRun9" {
		t.Fatalf("this run sends %q, want the environment token", got)
	}
	t.Setenv("EXCHANGE_ONLINE_TOKEN", "")
	again, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if got := again.AuthHeader(); got != "Bearer SavedTokenFromSetToken7" {
		t.Fatalf("next run sends %q, want the saved token", got)
	}
}
