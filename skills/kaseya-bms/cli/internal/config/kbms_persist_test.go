// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kaseya-bms-pp-cli/internal/credkit"

	"github.com/pelletier/go-toml/v2"
)

// config-file-holds-no-credential (Codex r1 4.34.2 class
// credential-persisted-outside-the-store): a configuration file that carries
// credentials (a base_url userinfo password and query credential, a
// credential header) is rewritten on the next save (a minted token saved
// with its host-and-login binding) without any of them, and keeps everything
// else. The token itself goes to the credential store, never config.toml.
// The credential kit holds every base_url query value (credkit.HoldURL), so
// the rewrite keeps the URL without its query; a Kaseya BMS base URL carries
// none (.review/followups.md records the narrowing).
func TestSavedConfigFileKeepsNoCredential(t *testing.T) {
	isolateHome(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	body := "base_url = \"https://ro:PersistPw-1c9e7a@api.bms.kaseya.com?api_key=PersistKey-55b3\"\n" +
		"[headers]\nX-Api-Key = \"PersistHeader-3f7b\"\nX-Trace = \"keep-me\"\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	binding := KbmsTokenBinding("https://api.bms.kaseya.com", KbmsLogin{Username: "persistApiUser-6e", Tenant: "persistTenant"})
	if err := cfg.SaveTokens(binding, "", "persistAccess-7c", "", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"PersistPw-1c9e7a", "PersistKey-55b3", "PersistHeader-3f7b", "persistApiUser-6e", "persistTenant", "persistAccess-7c"} {
		if strings.Contains(string(got), s) {
			t.Errorf("config.toml holds %q:\n%s", s, got)
		}
	}
	for _, s := range []string{"base_url = 'https://api.bms.kaseya.com'", "keep-me"} {
		if !strings.Contains(string(got), s) {
			t.Errorf("config.toml lost the non-credential %q:\n%s", s, got)
		}
	}
}

// The last check refuses a write whose bytes still repeat a held value.
func TestConfigFileWriteRefusedWhenItRepeatsAHeldValue(t *testing.T) {
	credkit.Hold("test:held", "RefusedConfigHeld-4b1d9e")
	p := persistedConfig{BaseURL: "https://api.bms.kaseya.com", TLSServerName: "RefusedConfigHeld-4b1d9e"}
	data, err := toml.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := refuseCredentialInConfigFile(data, nil); !errors.Is(err, ErrConfigHoldsCredential) {
		t.Fatalf("err = %v, want ErrConfigHoldsCredential", err)
	}
	p.TLSServerName = "api.bms.kaseya.com"
	if data, err = toml.Marshal(p); err != nil {
		t.Fatal(err)
	}
	if err := refuseCredentialInConfigFile(data, nil); err != nil {
		t.Fatalf("a clean file was refused: %v", err)
	}
}
