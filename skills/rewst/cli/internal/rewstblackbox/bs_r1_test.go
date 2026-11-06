// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package rewstblackbox

// Codex review round 1 of the domotz 4.34.2 reprint, class
// command-line-config-unheld-at-startup (seen/r1-command-line-config-unheld-
// at-startup-seen-failing.log in the run dir): the startup holder loaded only
// the configuration the environment names (REWST_CONFIG or the default
// path), so a configuration the command line names (--config, --home) was
// read only when a command ran: a parse error, an unknown command or a flag
// error printed its token. Every configuration source the command line names
// is held before the command line is parsed.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBsR1CommandLineConfigIsHeldBeforeParse(t *testing.T) {
	budget(t)
	const (
		cfgTok  = "AltToken-7x9k4m6n2q"
		homeTok = "HomeStoreToken-5b8d1e3f"
	)
	home := t.TempDir()
	cfg := filepath.Join(home, "alt.toml")
	if err := os.WriteFile(cfg, []byte("api_token = '"+cfgTok+"'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	alt := filepath.Join(home, "althome")
	if err := os.MkdirAll(alt, 0o700); err != nil {
		t.Fatal(err)
	}
	env := cleanEnv(home)
	runWithIn(t, env, homeTok+"\n", "auth", "set-token", "--home", alt)
	for _, c := range []struct {
		tok  string
		args []string
	}{
		{cfgTok, []string{cfgTok, "--config", cfg, "--no-learn"}},
		{cfgTok, []string{cfgTok, "--config=" + cfg, "--no-learn"}},
		{cfgTok, []string{"api-clients", "--no-such-flag", cfgTok, "--config", cfg}},
		{cfgTok, []string{"--config", cfg, "pages", "get", "--limit", cfgTok}},
		{cfgTok, []string{"--config", cfg, "org-variables", "get", "--visible-for-org-id", bbOrgID, "--limit", cfgTok}},
		{cfgTok, []string{"--config", cfg, "failures", "--org", bbOrgID, "--max-scan", cfgTok}},
		{homeTok, []string{homeTok, "--home", alt, "--no-learn"}},
		{homeTok, []string{"--home=" + alt, "api-clients", "--no-such-flag", homeTok}},
	} {
		out := runWith(t, env, c.args...)
		if strings.Contains(out, c.tok) {
			t.Errorf("%v printed the credential its command-line configuration holds:\n%s", c.args, out)
		}
	}
}
