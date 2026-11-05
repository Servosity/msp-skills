// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package config

// Codex review round 1 (appdirect 4.34.2 reprint) class
// credential-persisted-outside-the-store: config.toml is not the credential
// store, whatever shape the save takes. With an .agentcookie-managed marker
// beside config.toml the save marshaled the whole configuration, so `auth
// set-token` wrote the bearer into config.toml (appdirect handfix
// agentcookie-config-holds-no-credential).

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"appdirect-pp-cli/internal/cliutil/testenv"
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

// isolateHome points every config, data, state and cache path at a temp dir
// and clears the AppDirect credential and routing variables.
func isolateHome(t *testing.T) string {
	t.Helper()
	home := testenv.Isolate(t)
	t.Setenv("APPDIRECT_HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	for _, k := range []string{"APPDIRECT_CLIENT_ID", "APPDIRECT_CLIENT_SECRET", "APPDIRECT_BASE_URL", "APPDIRECT_TOKEN_URL", "APPDIRECT_NO_CONFIG_WRITE"} {
		t.Setenv(k, "")
	}
	return home
}

// config-rewrite-keeps-file-values: an environment URL is recorded as its
// marketplace, without the credential its query or userinfo carries.
func TestURLWithoutCredential(t *testing.T) {
	for raw, want := range map[string]string{
		"https://mp.example.net/api":                        "https://mp.example.net/api",
		"https://u:Pw-1@mp.example.net/api?api_key=Q-2#f-3": "https://mp.example.net/api",
		"mp.example.net/api":                                "mp.example.net/api",
		"https://mp.example.net/oauth2/token?":              "https://mp.example.net/oauth2/token",
		"%zz":                                               "fallback",
	} {
		if got := urlWithoutCredential(raw, "fallback"); got != want {
			t.Errorf("urlWithoutCredential(%q) = %q, want %q", raw, got, want)
		}
	}
}
