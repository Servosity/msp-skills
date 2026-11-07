// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"salesbuildr-pp-cli/internal/cliutil/testenv"
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
	body := "base_url = \"https://ro:PersistPw-1c9e7a@acme.salesbuildr.com/public-api?api_key=PersistKey-55b3&region=us\"\n" +
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
	for _, s := range []string{"PersistPw-1c9e7a", "PersistKey-55b3", "PersistHeader-3f7b", "persistClientSecret-0a", "persistAccess-7c"} {
		if strings.Contains(string(got), s) {
			t.Errorf("config.toml holds %q:\n%s", s, got)
		}
	}
	for _, s := range []string{"base_url = 'https://acme.salesbuildr.com/public-api'", "keep-me"} {
		if !strings.Contains(string(got), s) {
			t.Errorf("config.toml lost the non-credential %q:\n%s", s, got)
		}
	}
}

// config-file-holds-no-credential: config.toml keeps base_url without its
// userinfo and query (the credwire TestConfigHeaderNeverKeepsTheNewKey runs
// the header class through auth set-token on the built binary).
func TestBaseURLWithoutCredentials(t *testing.T) {
	for in, want := range map[string]string{
		"https://ro:Pw-1c9e@acme.salesbuildr.com/public-api?api_key=K-55b3&region=us": "https://acme.salesbuildr.com/public-api",
		"https://acme.salesbuildr.com/public-api":                                     "https://acme.salesbuildr.com/public-api",
		"https://acme.salesbuildr.com/public-api#Frag-9a":                             "https://acme.salesbuildr.com/public-api",
		"https://ro:Pw-1c9e@other.example.com:bad/":                                   builtinBaseURL(),
		"https://{tenant}.salesbuildr.com/public-api":                                 "https://{tenant}.salesbuildr.com/public-api",
		"": "",
	} {
		if got := baseURLWithoutCredentials(in); got != want {
			t.Errorf("baseURLWithoutCredentials(%q) = %q, want %q", in, got, want)
		}
	}
}

// Codex r1 (salesbuildr 4.34.2 reprint review) class
// credential-persisted-outside-the-store: with an agentcookie marker beside
// config.toml, a save wrote the whole configuration, credential fields
// included, and the last check passed it unread. No save path writes a
// credential to config.toml, whoever manages the file.
func TestAgentcookieManagedSaveNeverWritesACredential(t *testing.T) {
	testenv.Isolate(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(filepath.Join(dir, ".agentcookie-managed"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	const secret = "VendorSecret-aaaa1111"
	saveErr := cfg.SaveCredential(secret)
	got, _ := os.ReadFile(path)
	if strings.Contains(string(got), secret) {
		t.Fatalf("config.toml holds the credential (save err %v):\n%s", saveErr, got)
	}
	if saveErr == nil {
		t.Fatal("SaveCredential reported success although the credential was kept nowhere")
	}
	// Clearing credentials under agentcookie still writes the file (it holds none).
	if err := cfg.ClearTokens(); err != nil {
		t.Fatalf("ClearTokens under agentcookie: %v", err)
	}
	got, _ = os.ReadFile(path)
	if strings.Contains(string(got), secret) {
		t.Fatalf("config.toml holds the credential after ClearTokens:\n%s", got)
	}
}

// The whole Config (an agentcookie-managed save) is refused while a
// credential field is set, and passes without one.
func TestConfigFileCheckCoversEveryShape(t *testing.T) {
	if err := refuseCredentialInConfigFile(Config{SalesbuildrApiKey: "ShapeKey-aaaa11"}); !errors.Is(err, ErrConfigHoldsCredential) {
		t.Fatalf("Config with an API key: err = %v", err)
	}
	if err := refuseCredentialInConfigFile(Config{BaseURL: "https://acme.salesbuildr.com/public-api"}); err != nil {
		t.Fatalf("a Config without credentials was refused: %v", err)
	}
	if err := refuseCredentialInConfigFile(persistedConfig{BaseURL: "https://acme.salesbuildr.com/public-api"}); err != nil {
		t.Fatalf("persistedConfig was refused: %v", err)
	}
}

// credential-persisted-outside-the-store: a [headers] value that carries a
// credential this configuration holds, plain or in a standard encoding, and a
// credential-named header are left out of config.toml; the rest are kept.
func TestHeadersWithoutCredentials(t *testing.T) {
	const key = "HeaderKey-xxxxyyyy"
	c := &Config{SalesbuildrApiKey: key}
	enc := map[string]string{
		"X-Plain":   "pre-" + key + "-post",
		"X-B64":     base64.StdEncoding.EncodeToString([]byte(key)),
		"X-B64Url":  base64.RawURLEncoding.EncodeToString([]byte("v=" + key)),
		"X-B32":     strings.TrimRight(base32.StdEncoding.EncodeToString([]byte(key)), "="),
		"X-B32Low":  strings.ToLower(base32.StdEncoding.EncodeToString([]byte(key))),
		"X-Hex":     hex.EncodeToString([]byte(key)),
		"X-Pct":     url.QueryEscape("k=" + key + "&x"),
		"X-Nested":  base64.StdEncoding.EncodeToString([]byte(hex.EncodeToString([]byte(key)))),
		"X-Api-Key": "unrelated-value-1",
		"X-Team":    "team-blue-west",
	}
	got := headersWithoutCredentials(enc, c)
	if len(got) != 1 || got["X-Team"] != "team-blue-west" {
		t.Fatalf("kept %v, want only X-Team", got)
	}
	if len(enc) != 10 {
		t.Fatal("the caller's map was changed")
	}
	if got := headersWithoutCredentials(map[string]string{"X-Team": "team-blue-west"}, &Config{}); got["X-Team"] != "team-blue-west" {
		t.Fatalf("a header was dropped with no credential configured: %v", got)
	}
}
