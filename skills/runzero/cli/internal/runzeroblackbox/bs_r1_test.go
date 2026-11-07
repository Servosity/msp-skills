// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package runzeroblackbox

// Codex review round 1 of the domotz 4.34.2 reprint, class
// command-line-config-unheld-at-startup (seen/r1-command-line-config-unheld-
// at-startup-seen-failing.log in the run dir): the startup holder loaded only
// the configuration the environment names (RUNZERO_CONFIG or the default
// path), so a configuration the command line names (--config, --home) was
// read only when a command ran: a parse error, an unknown command or a flag
// error printed its token. Every configuration source the command line names
// is held before the command line is parsed: a config file's api_key,
// access_token and auth_header ("Bearer <key>"), and the key a credential
// store under --home holds (auth set-token, stdin).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBsR1CommandLineConfigIsHeldBeforeParse(t *testing.T) {
	t.Parallel() // its own fake runZero and home
	const (
		cfgKey    = "CTAltConfigKey-7x9k4m6n2q"
		cfgAccess = "AltAccessToken-3c8v1b5n"
		hdrKey    = "ETAltHeaderKey-4d2f8a6c1e" // gitleaks:allow synthetic test fixture
		homeKey   = "OTHomeStoreKey-5b8d1e3f"
	)
	home := t.TempDir()
	cfg := filepath.Join(home, "alt.toml")
	if err := os.WriteFile(cfg, []byte("api_key = '"+cfgKey+"'\naccess_token = '"+cfgAccess+"'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hdr := filepath.Join(home, "hdr.toml")
	if err := os.WriteFile(hdr, []byte("auth_header = 'Bearer "+hdrKey+"'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	alt := filepath.Join(home, "althome")
	if err := os.MkdirAll(alt, 0o700); err != nil {
		t.Fatal(err)
	}
	env := cleanEnv(home)
	if out := runWithIn(t, env, homeKey+"\n", "auth", "set-token", "--home", alt); strings.Contains(out, homeKey) {
		t.Errorf("auth set-token printed the key it stores:\n%s", out)
	}
	for _, c := range []struct {
		tok  string
		args []string
	}{
		{cfgKey, []string{cfgKey, "--config", cfg, "--no-learn"}},
		{cfgAccess, []string{cfgAccess, "--config", cfg, "--no-learn"}},
		{cfgKey, []string{cfgKey, "--config=" + cfg, "--no-learn"}},
		{cfgKey, []string{"org", "get-asset", "--no-such-flag", cfgKey, "--config", cfg}},
		{cfgAccess, []string{"org", "get-asset", "--no-such-flag", cfgAccess, "--config", cfg}},
		{cfgKey, []string{"--config", cfg, "runzero-export", "assets-json", "--page-size", cfgKey}},
		{cfgAccess, []string{"--config", cfg, "runzero-export", "assets-json", "--page-size", cfgAccess}},
		{hdrKey, []string{hdrKey, "--config", hdr, "--no-learn"}},
		{hdrKey, []string{"--config=" + hdr, "account", "get-key", "--no-such-flag", hdrKey}},
		{homeKey, []string{homeKey, "--home", alt, "--no-learn"}},
		{homeKey, []string{"--home=" + alt, "org", "get-asset", "--no-such-flag", homeKey}},
		{homeKey, []string{"--home=" + alt, "runzero-export", "assets-json", "--page-size", homeKey}},
	} {
		out := runWith(t, env, c.args...)
		if strings.Contains(out, c.tok) {
			t.Errorf("%v printed the credential its command-line configuration holds:\n%s", c.args, out)
		}
	}
}
