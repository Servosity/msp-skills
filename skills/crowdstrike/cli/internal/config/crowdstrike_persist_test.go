// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"errors"
	"fmt"
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
	body := "base_url = \"https://ro:PersistPw-1c9e7a@api.eu-1.crowdstrike.com?api_key=PersistKey-55b3&region=us\"\n" +
		"token_url = \"https://ro:PersistTokPw-9a1c@api.eu-1.crowdstrike.com/oauth2/token?api_key=PersistTokKey-3d7e\"\n" +
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
	for _, s := range []string{"https://api.eu-1.crowdstrike.com?region=us", "https://api.eu-1.crowdstrike.com/oauth2/token", "keep-me"} {
		if !strings.Contains(string(got), s) {
			t.Errorf("config.toml lost the non-credential %q:\n%s", s, got)
		}
	}
}

// The last check reads back the bytes about to be written: a credential key
// with a value, or one of the configuration's credentials as written, is a
// refusal; a clean file is not.
func TestConfigFileWriteRefusedWhenItCarriesACredential(t *testing.T) {
	if err := refuseCredentialInConfigFile(nil, []byte("base_url = \"https://api.crowdstrike.com\"\naccess_token = \"refusedtokenvalue\"\n"), nil); !errors.Is(err, ErrConfigHoldsCredential) {
		t.Fatalf("a credential key: err = %v, want ErrConfigHoldsCredential", err)
	}
	c := &Config{ClientSecret: "refusedsecretvalue"}
	p := persistedConfig{BaseURL: "https://api.crowdstrike.com", TLSServerName: "refusedsecretvalue"}
	if err := refuseCredentialInConfigFile(p, nil, c); !errors.Is(err, ErrConfigHoldsCredential) {
		t.Fatalf("a configured credential: err = %v, want ErrConfigHoldsCredential", err)
	}
	p.TLSServerName = "api.crowdstrike.com"
	if err := refuseCredentialInConfigFile(p, nil, c); err != nil {
		t.Fatalf("a clean file was refused: %v", err)
	}
}

// per-cloud-token-url meets config-file-holds-no-credential: a config.toml
// naming its Falcon cloud's token URL (token_url, a setting like base_url) is
// read, and the next credential save (auth login, auth set-token, a minted
// token) still writes. The file holder read token_url by its name ("token"),
// held the whole URL as a credential, and the last check then refused every
// config.toml write that repeated it, so no credential could be saved at all
// for a configuration that names its cloud's token endpoint. The environment
// rule already read CROWDSTRIKE_*_URL as a URL (its userinfo password and
// credential query values held, the URL itself not); the file rule now reads a
// *_url key the same way. Run alone and after a test that loads another
// token_url, so the result does not depend on test order.
func TestConfigNamingItsCloudTokenURLStillSaves(t *testing.T) {
	isolateHome(t)
	dir := t.TempDir()
	for i, tokenURL := range []string{"https://api.eu-1.crowdstrike.com/oauth2/token", "https://api.laggar.gcw.crowdstrike.com/oauth2/token"} {
		path := filepath.Join(dir, fmt.Sprintf("config-%d.toml", i))
		body := "base_url = \"https://api.eu-1.crowdstrike.com\"\ntoken_url = \"" + tokenURL + "\"\n"
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.TokenURL != tokenURL {
			t.Fatalf("token_url %q not read from the owner-only file (got %q)", tokenURL, cfg.TokenURL)
		}
		if err := cfg.SaveTokens("cloudClientId-2b7e", "cloudClientSecret-5d1a", "cloudAccess-9e3c", "", time.Now().Add(time.Hour)); err != nil {
			t.Fatalf("token_url %s: saving a minted token was refused: %v", tokenURL, err)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(got), tokenURL) {
			t.Errorf("config.toml lost the token URL setting:\n%s", got)
		}
		for _, s := range []string{"cloudClientSecret-5d1a", "cloudAccess-9e3c"} {
			if strings.Contains(string(got), s) {
				t.Errorf("config.toml holds %q:\n%s", s, got)
			}
		}
	}
}
