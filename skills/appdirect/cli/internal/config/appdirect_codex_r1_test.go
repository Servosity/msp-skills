// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package config

// Codex review round 1 (appdirect 4.34.2 reprint) class
// credential-persisted-outside-the-store: config.toml is not the credential
// store, whatever shape the save takes. With an .agentcookie-managed marker
// beside config.toml the save marshals the whole configuration, and the last
// check (refuseCredentialInConfigFile) returned early for any shape but the
// credential-free one, so `auth set-token` wrote the bearer into config.toml.
// The same check skipped every header value, and the header filter matched a
// credential only as written (a header holding the credential's base32 was
// kept; the salesbuildr round-2 repeat of this class). Each test was written
// to fail on the code before the fix.

import (
	"encoding/base32"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Codex r1 failing input: /tmp/ad/config.toml with only non-secret settings,
// /tmp/ad/.agentcookie-managed present, then
// printf 'NewBearer-739182\n' | appdirect-cli --config /tmp/ad/config.toml auth set-token.
func TestR1AgentcookieSaveNeverWritesACredential(t *testing.T) {
	isolateHome(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("base_url = \"https://tenant.example.net/api\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".agentcookie-managed"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	err = cfg.SaveTokens("", "", "NewBearer-739182", "", cfg.TokenExpiry) // what auth set-token saves
	got, _ := os.ReadFile(path)
	if strings.Contains(string(got), "NewBearer-739182") {
		t.Fatalf("config.toml holds the bearer token (save err = %v):\n%s", err, got)
	}
	if err != nil && !errors.Is(err, ErrConfigHoldsCredential) {
		t.Fatalf("save err = %v, want nil or ErrConfigHoldsCredential", err)
	}
}

// A header config.toml keeps may not carry a credential in any form the
// output chokepoint withholds: its base32 here.
func TestR1HeaderNeverCarriesAnEncodedCredential(t *testing.T) {
	isolateHome(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	const token = "HdrBearer-5c2e81"
	enc := base32.StdEncoding.EncodeToString([]byte(token))
	body := "base_url = \"https://tenant.example.net/api\"\n[headers]\nX-Trace = \"" + enc + "\"\nX-Keep = \"keep-this-header\"\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	err = cfg.SaveTokens("hdrClientId-1a", "hdrClientSecret-2b", token, "", time.Now().Add(time.Hour))
	got, _ := os.ReadFile(path)
	if strings.Contains(string(got), enc) || strings.Contains(string(got), strings.TrimRight(enc, "=")) {
		t.Fatalf("config.toml keeps the base32 of the token (save err = %v):\n%s", err, got)
	}
	if err == nil && !strings.Contains(string(got), "keep-this-header") {
		t.Fatalf("config.toml lost a header that holds no credential:\n%s", got)
	}
}

// The legacy-config scrub writes a config file too (the legacy one, after a
// migration), so it passes the same last check: a legacy base_url whose path
// carries an encoding of the token it stored is not written back.
func TestR1LegacyScrubNeverWritesACredential(t *testing.T) {
	isolateHome(t)
	legacy, err := LegacyConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(legacy), 0o700); err != nil {
		t.Fatal(err)
	}
	const token = "LegacyTok-6f1e9a2c"
	enc := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte(token))
	body := "base_url = \"https://tenant.example.net/api/" + enc + "\"\naccess_token = \"" + token + "\"\n"
	if err := os.WriteFile(legacy, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APPDIRECT_BASE_URL", "https://tenant.example.net/api")
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.legacySourcePath == "" {
		t.Skip("this layout has no separate legacy config path")
	}
	_ = cfg.SaveTokens("legacyClientId-2a", "legacyClientSecret-4b", "NewTok-9c3d71e5", "", time.Now().Add(time.Hour))
	got, _ := os.ReadFile(legacy)
	if strings.Contains(string(got), enc) || strings.Contains(string(got), token) {
		t.Fatalf("the legacy config still holds the token (encoded):\n%s", got)
	}
}
