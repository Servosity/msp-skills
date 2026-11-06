// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// config-file-holds-no-credential (Codex r1 4.34.2 class
// credential-persisted-outside-the-store): a configuration file that carries
// credentials (a base_url or token_url userinfo password and query
// credential, a credential header) is rewritten on the next save without any
// of them, and keeps everything else.
func TestSavedConfigFileKeepsNoCredential(t *testing.T) {
	isolateHome(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	body := "base_url = \"https://ro:PersistPw-1c9e7a@nmm.persist-contoso.example/rest-api?api_key=PersistKey-55b3&region=us\"\n" +
		"token_url = \"https://ro:PersistTokPw-9a1c@login.microsoftonline.com/persist-tenant-4e2a/oauth2/v2.0/token?api_key=PersistTokKey-3d7e\"\n" +
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
	for _, s := range []string{"PersistPw-1c9e7a", "PersistKey-55b3", "PersistTokPw-9a1c", "PersistTokKey-3d7e", "PersistHeader-3f7b", "persistClientSecret-0a", "persistAccess-7c"} {
		if strings.Contains(string(got), s) {
			t.Errorf("config.toml holds %q:\n%s", s, got)
		}
	}
	for _, s := range []string{"https://nmm.persist-contoso.example/rest-api?region=us", "https://login.microsoftonline.com/persist-tenant-4e2a/oauth2/v2.0/token", "keep-me"} {
		if !strings.Contains(string(got), s) {
			t.Errorf("config.toml lost the non-credential %q:\n%s", s, got)
		}
	}
}

// The last check refuses a write whose bytes still repeat one of the
// configuration's own credentials.
func TestConfigFileWriteRefusedWhenItRepeatsACredential(t *testing.T) {
	c := &Config{ClientSecret: "RefusedConfigHeld-4b1d9e"}
	p := persistedConfig{BaseURL: "https://nmm.example.com", TLSServerName: "RefusedConfigHeld-4b1d9e"}
	if err := refuseCredentialInConfigFile(p, nil, c); !errors.Is(err, ErrConfigHoldsCredential) {
		t.Fatalf("err = %v, want ErrConfigHoldsCredential", err)
	}
	p.TLSServerName = "nmm.contoso.example"
	if err := refuseCredentialInConfigFile(p, nil, c); err != nil {
		t.Fatalf("a clean file was refused: %v", err)
	}
}
