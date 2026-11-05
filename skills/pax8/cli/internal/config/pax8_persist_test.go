// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pax8-pp-cli/internal/cliutil/testenv"
)

// config-file-holds-no-credential (Codex r1 4.34.2 class
// credential-persisted-outside-the-store): a configuration file whose URLs carry
// credentials (a base_url userinfo password and query value, a token_url query
// value) is rewritten on the next save without them, and keeps everything else.
// The [headers] table is config.toml's own credential source and is kept; the
// header the kit refuses to send to the API origin is proven on the built binary
// (pax8blackbox TestKitConfigFileRefusesEncodedHeaderCredential).
func TestSavedConfigFileKeepsNoCredential(t *testing.T) {
	testenv.Isolate(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	body := "base_url = \"https://ro:PersistPw-1c9e7a@api.pax8.com/v1?api_key=PersistKey-55b3&region=us\"\n" +
		"token_url = \"https://token-manager.pax8.com/oauth/token?secret=PersistTokQ-8d2a\"\n" +
		"[headers]\nX-Trace = \"keep-me\"\n"
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
	for _, s := range []string{"PersistPw-1c9e7a", "PersistKey-55b3", "PersistTokQ-8d2a", "region=us", "persistClientSecret-0a", "persistAccess-7c"} {
		if strings.Contains(string(got), s) {
			t.Errorf("config.toml holds %q:\n%s", s, got)
		}
	}
	for _, s := range []string{"https://api.pax8.com/v1", "https://token-manager.pax8.com/oauth/token", "keep-me"} {
		if !strings.Contains(string(got), s) {
			t.Errorf("config.toml lost the non-credential %q:\n%s", s, got)
		}
	}
}

func TestURLWithoutCredentials(t *testing.T) {
	for in, want := range map[string]string{
		"":                                      "",
		"https://api.pax8.com/v1":               "https://api.pax8.com/v1",
		"https://u:p@api.pax8.com/v1?a=1&b=2#f": "https://api.pax8.com/v1",
		"https://{tenant}.example.com/v1":       "https://{tenant}.example.com/v1",
		"https://%zz@api.pax8.com/v1?x=1":       "FALLBACK",
	} {
		if got := urlWithoutCredentials("base_url", in, "FALLBACK"); got != want {
			t.Errorf("urlWithoutCredentials(%q) = %q, want %q", in, got, want)
		}
	}
}
