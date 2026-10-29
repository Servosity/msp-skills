// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pax8-pp-cli/internal/cliutil"
	"pax8-pp-cli/internal/cliutil/testenv"
)

// config-file-holds-no-credential (Codex r1 4.34.2 class
// credential-persisted-outside-the-store): a configuration file that carries
// credentials (a base_url userinfo password and query credential, a
// credential header) is rewritten on the next save without any of them, and
// keeps everything else.
func TestSavedConfigFileKeepsNoCredential(t *testing.T) {
	testenv.Isolate(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	body := "base_url = \"https://ro:PersistPw-1c9e7a@api.pax8.com/v1?api_key=PersistKey-55b3&region=us\"\n" +
		"token_url = \"https://token-manager.pax8.com/oauth/token?secret=PersistTokQ-8d2a\"\n" +
		"[headers]\nX-Api-Key = \"PersistHeader-3f7b\"\nX-Trace = \"keep-me\"\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SaveTokens("persistClientId-6e", "persistClientSecret-0a", "persistAccess-7c", "", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"PersistPw-1c9e7a", "PersistKey-55b3", "PersistTokQ-8d2a", "PersistHeader-3f7b", "persistClientSecret-0a", "persistAccess-7c"} {
		if strings.Contains(string(got), s) {
			t.Errorf("config.toml holds %q:\n%s", s, got)
		}
	}
	for _, s := range []string{"https://api.pax8.com/v1?region=us", "keep-me"} {
		if !strings.Contains(string(got), s) {
			t.Errorf("config.toml lost the non-credential %q:\n%s", s, got)
		}
	}
}

// The last check refuses a write whose bytes still repeat a held value.
func TestConfigFileWriteRefusedWhenItRepeatsAHeldValue(t *testing.T) {
	cliutil.FunnelHold("RefusedConfigHeld-4b1d9e")
	p := persistedConfig{BaseURL: "https://api.pax8.com/v1", TLSServerName: "RefusedConfigHeld-4b1d9e"}
	if err := refuseCredentialInConfigFile(p, nil); !errors.Is(err, ErrConfigHoldsCredential) {
		t.Fatalf("err = %v, want ErrConfigHoldsCredential", err)
	}
	p.TLSServerName = "api.pax8.com"
	if err := refuseCredentialInConfigFile(p, nil); err != nil {
		t.Fatalf("a clean file was refused: %v", err)
	}
}
