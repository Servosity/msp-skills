// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

// Codex review round 1 (crowdstrike 4.34.2 reprint) class
// credential-persisted-outside-the-store: config.toml is not the credential
// store, whatever shape the save takes. With an .agentcookie-managed marker
// beside config.toml the save marshals the whole configuration, and the last
// check (refuseCredentialInConfigFile) returned early for any shape but the
// credential-free one, so `auth set-token` wrote the bearer into config.toml.
// Written to fail on the code before the fix. (The encoded-header and
// encoded-legacy-path cases were the retired output layer's matcher; config
// writes are the credential kit's declared exemption, handfixes.json
// credkit-wiring.)

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"crowdstrike-pp-cli/internal/cliutil/testenv"
)

func isolateHome(t *testing.T) string {
	t.Helper()
	home := testenv.Isolate(t)
	t.Setenv("CROWDSTRIKE_HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	for _, k := range []string{"FALCON_CLIENT_ID", "FALCON_CLIENT_SECRET", "CROWDSTRIKE_BASE_URL", "CROWDSTRIKE_TOKEN_URL", "CROWDSTRIKE_NO_CONFIG_WRITE"} {
		t.Setenv(k, "")
	}
	return home
}

// Codex r1 failing input: /tmp/ad/config.toml with only non-secret settings,
// /tmp/ad/.agentcookie-managed present, then
// printf 'NewBearer-739182\n' | crowdstrike-cli --config /tmp/ad/config.toml auth set-token.
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
