// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package autotaskblackbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Codex r1 class holder-incomplete (--config and --home): the holder loaded
// only the default configuration before Cobra parsed the command line, so
// `autotask-cli --config /tmp/custom/config.toml --<its credential>`
// printed the credential in the unknown-flag error. The configuration a run
// names is held before anything prints (credkit.FlagValues reads every
// --config and --home at Start).
func TestFlagErrorUnderConfigOrHomeWithholdsItsCredentials(t *testing.T) {
	t.Parallel()
	const code, user, secret = "PrivateStoredCode-123x", "private-user-77@example.com", "PrivateStoredSecret-9q8w"
	h := t.TempDir()
	custom := filepath.Join(h, "custom")
	_ = os.MkdirAll(custom, 0o700)
	cfg := filepath.Join(custom, "config.toml")
	if err := os.WriteFile(cfg, []byte("api_integration_code = \""+code+"\"\npsa_user_name = \""+user+"\"\npsa_secret = \""+secret+"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	empty := filepath.Join(custom, "empty.toml")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(h, "other-home")
	_ = os.MkdirAll(other, 0o700)
	const stored = "PrivateHomeToken-5t6y7u"
	runWithIn(t, cleanEnv(h), stored+"\n", "--home", other, "auth", "set-token")
	for name, args := range map[string][]string{
		"--config path":  {"--config", cfg, "--" + code, "--" + secret},
		"--config=path":  {"--config=" + cfg, "tickets", "--" + code},
		"after the verb": {"version", "--" + secret, "--config", cfg},
		"--home path":    {"--home", other, "--" + stored},
		"--home=path":    {"--home=" + other, "version", "--" + stored},
		// Codex r2: --help stopped the holder's scan before --config, and a
		// value of another flag that looks like --config was read as one.
		"--help first":        {code, "--help", "--config", cfg},
		"--config as a value": {"--config", cfg, "tickets", "query", "--filter", "--config=" + empty, "--" + code},
		"-h first":            {"version", "-h", "--home", other, "--" + stored},
	} {
		out := runWith(t, cleanEnv(h), args...)
		if !strings.Contains(out, "unknown flag") && !strings.Contains(out, "withheld") && !strings.Contains(out, "Usage") {
			t.Errorf("%s: the flag error did not run:\n%s", name, out)
		}
		// psa_user_name is the API user's login name, an identifier: not checked.
		for _, v := range []string{code, secret, stored} {
			if strings.Contains(out, v) {
				t.Errorf("%s: printed %q:\n%s", name, v, out)
			}
		}
	}
}
