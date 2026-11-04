// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"nerdio-pp-cli/internal/cliutil"
	"nerdio-pp-cli/internal/credkit"
)

// The Entra token URL is a setting, not a credential (NERDIO_TOKEN_URL and
// token_url in config.toml name where the client pair is posted; the tenant
// in it is no secret). A config.toml that names it the way the README says
// (token_url beside base_url) must not make the file unsavable: Load read
// every value under a credential-named key as a credential, "token_url"
// reads as one, so the URL was held, and the next save (auth login, a minted
// token cached beside the stored pair) refused to write config.toml because
// it repeats a held value. The URL's own credentials (a userinfo password, a
// credential-named query value) are still held and still left out.
func TestConfigFileTokenURLIsASettingNotACredential(t *testing.T) {
	isolateHome(t)
	held, _ := recordHolds(t)
	const tokenURL = "https://login.microsoftonline.com/tokurl-setting-tenant-5c2e/oauth2/v2.0/token"
	path := filepath.Join(t.TempDir(), "config.toml")
	body := "base_url = \"https://nmm.tokurl-setting.example\"\ntoken_url = \"" + tokenURL + "\"\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TokenURL != tokenURL {
		t.Fatalf("TokenURL = %q, want the file's %q", cfg.TokenURL, tokenURL)
	}
	if slices.Contains(*held, tokenURL) {
		t.Errorf("the token URL is held as a credential; every output naming it is withheld")
	}
	if err := cfg.SaveTokens("tokurlSettingId-3a9c", "tokurlSettingSecret-7e1b", "tokurlSettingAccess-2d6f", "", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("saving the minted token beside the stored pair: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), tokenURL) {
		t.Errorf("config.toml lost the token URL:\n%s", got)
	}
	for _, s := range []string{"tokurlSettingSecret-7e1b", "tokurlSettingAccess-2d6f"} {
		if strings.Contains(string(got), s) {
			t.Errorf("config.toml holds %q:\n%s", s, got)
		}
	}
}

// The control for the test above: the credentials a token URL carries are
// still held from the file (a userinfo password and a credential-named query
// value), so leaving token_url out of the credential names held nothing less.
func TestConfigFileTokenURLCredentialsAreStillHeld(t *testing.T) {
	isolateHome(t)
	_, urls := recordHolds(t)
	const pw, q = "tokurlUserinfoPw-8b3d1f", "tokurlQueryKey-4c7a2e"
	path := filepath.Join(t.TempDir(), "config.toml")
	body := "base_url = \"https://nmm.tokurl-held.example\"\ntoken_url = \"https://ro:" + pw + "@login.microsoftonline.com/t-1/oauth2/v2.0/token?api_key=" + q + "\"\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, u := range *urls {
		found = found || strings.Contains(u, pw) && strings.Contains(u, q)
	}
	if !found {
		t.Errorf("token_url was not handed to the kit's URL hold (its password and query values): %q", *urls)
	}
}

// recordHolds swaps the kit's holds for recorders for the rest of the test:
// the values Hold received and the raw URLs HoldURL received.
func recordHolds(t *testing.T) (held, urls *[]string) {
	t.Helper()
	held, urls = &[]string{}, &[]string{}
	hold, holdURL, holdBody := cliutil.Hold, cliutil.HoldURL, cliutil.HoldBody
	cliutil.Hold = func(_ credkit.Source, v ...string) { *held = append(*held, v...) }
	cliutil.HoldURL = func(_ credkit.Source, raw string) { *urls = append(*urls, raw) }
	cliutil.HoldBody = func([]byte) {}
	t.Cleanup(func() { cliutil.Hold, cliutil.HoldURL, cliutil.HoldBody = hold, holdURL, holdBody })
	return held, urls
}
