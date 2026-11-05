// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package dsblackbox

// Codex review round 1 class credential-source-not-held: every credential any
// source carries is held before anything can print, not only the one that
// wins: a stored token the environment overrides, a credential store the
// config file's own token keeps from loading, a store the loader refuses for
// its permissions, and the stores a --home or --config on the command line
// names (Cobra's unknown-command and unknown-flag errors repeat the argument
// before any command loads its configuration).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// assertNotEchoed fails when text holds tok in any form the oracle knows, or
// both of its halves.
func assertNotEchoed(t *testing.T, where, text, tok string) {
	t.Helper()
	for _, f := range echoForms(tok) {
		if strings.Contains(text, f) {
			t.Errorf("%s holds a stored credential as %q", where, f)
		}
	}
	if a, b := halves(tok); strings.Contains(text, a) && strings.Contains(text, b) {
		t.Errorf("%s holds both halves of %q", where, tok)
	}
}

// echoArgs repeats tok where the command line echoes it before any command runs.
func echoArgs(tok string, prefix ...string) [][]string {
	return [][]string{
		append(append([]string{}, prefix...), tok),
		append(append([]string{}, prefix...), "organizations", "list", "--json", "--"+tok),
		append(append([]string{}, prefix...), "organizations", tok),
	}
}

func writePrivate(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func TestEveryStoredCredentialIsHeld(t *testing.T) {
	sv := echoServer(t)

	// 1. The environment token overrides the stored one (Codex r1 input).
	const shadowed = "shadowedStoreTok-5b8e2d91c7a4"
	e := newEnv(t, sv, false)
	e.runIn(t, shadowed+"\n", "auth", "set-token")
	e.env = append(e.env, "DROPSUITE_ACCESS_TOKEN="+bbAccess)
	for _, args := range echoArgs(shadowed) {
		assertNotEchoed(t, "env-overridden store: output of "+strings.Join(args, " "), e.run(t, args...), shadowed)
	}

	// 2. The config file carries its own token, so the credential store is
	// never loaded.
	const unloaded = "unloadedStoreTok-2f7c9a1e6d3b"
	e2 := newEnv(t, sv, false)
	h2 := filepath.Join(e2.home, "ds-home-2")
	writePrivate(t, filepath.Join(h2, "config", "config.toml"), "access_token = \"configFileTok-8a3c5e7b9d1f\"\n", 0o600)
	writePrivate(t, filepath.Join(h2, "data", "credentials.toml"), "access_token = \""+unloaded+"\"\nreseller_token = \"unloadedRslrTok-4d6f8a2c9e1b\"\n", 0o600)
	e2.env = append(e2.env, "DROPSUITE_HOME="+h2)
	for _, args := range echoArgs(unloaded) {
		assertNotEchoed(t, "store behind a config token: output of "+strings.Join(args, " "), e2.run(t, args...), unloaded)
	}
	for _, args := range echoArgs("unloadedRslrTok-4d6f8a2c9e1b") {
		assertNotEchoed(t, "store behind a config token: output of "+strings.Join(args, " "), e2.run(t, args...), "unloadedRslrTok-4d6f8a2c9e1b")
	}

	// 3. A credential store the loader refuses (group and world readable).
	const refused = "refusedStoreTok-9c1e3a5b7d2f"
	e3 := newEnv(t, sv, true)
	h3 := filepath.Join(e3.home, "ds-home-3")
	writePrivate(t, filepath.Join(h3, "data", "credentials.toml"), "access_token = \""+refused+"\"\n", 0o644)
	e3.env = append(e3.env, "DROPSUITE_HOME="+h3)
	for _, args := range echoArgs(refused) {
		assertNotEchoed(t, "refused store: output of "+strings.Join(args, " "), e3.run(t, args...), refused)
	}

	// 4. The store under a --home named on the command line.
	const homeTok = "flagHomeStoreTok-6e2a8c4f1b9d"
	e4 := newEnv(t, sv, false)
	h4 := filepath.Join(e4.home, "ds-home-4")
	if err := os.MkdirAll(h4, 0o700); err != nil {
		t.Fatal(err)
	}
	e4.runIn(t, homeTok+"\n", "--home", h4, "auth", "set-token")
	if b, err := os.ReadFile(filepath.Join(h4, "data", "credentials.toml")); err != nil || !strings.Contains(string(b), homeTok) {
		t.Fatalf("auth set-token --home wrote no store under %s (err %v)", h4, err)
	}
	for _, prefix := range [][]string{{"--home", h4}, {"--home=" + h4}} {
		for _, args := range echoArgs(homeTok, prefix...) {
			assertNotEchoed(t, "--home store: output of "+strings.Join(args, " "), e4.run(t, args...), homeTok)
		}
	}
	// The flag after the argument is still read by Cobra.
	assertNotEchoed(t, "--home after the argument", e4.run(t, homeTok, "--home", h4), homeTok)

	// 6. A config file the loader rejects (so loading stops before it holds
	// anything) next to a valid store: the CLI and the MCP server.
	const brokenCfgTok = "brokenConfigStoreTok-8b2d4f6a1c3e"
	e6 := newEnv(t, sv, true)
	h6 := filepath.Join(e6.home, "ds-home-6")
	writePrivate(t, filepath.Join(h6, "config", "config.toml"), "access_token = [unterminated\n", 0o600)
	writePrivate(t, filepath.Join(h6, "data", "credentials.toml"), "access_token = '"+brokenCfgTok+"'\n", 0o600)
	e6.env = append(e6.env, "DROPSUITE_HOME="+h6)
	for _, args := range echoArgs(brokenCfgTok) {
		assertNotEchoed(t, "store beside a broken config: output of "+strings.Join(args, " "), e6.run(t, args...), brokenCfgTok)
	}
	out := e6.mcpCalls(t, []map[string]any{{"name": brokenCfgTok, "arguments": map[string]any{}}})
	assertNotEchoed(t, "store beside a broken config: MCP session", out, brokenCfgTok)
	if !strings.Contains(out, `"id":1`) {
		t.Fatalf("the MCP server did not answer the call:\n%.2000s", out)
	}

	// 5. The store next to a --config named on the command line.
	const cfgTok = "flagConfigStoreTok-3a7d1f5c9e2b"
	e5 := newEnv(t, sv, false)
	cfgPath := filepath.Join(e5.home, "profiles", "p1", "config.toml")
	writePrivate(t, cfgPath, "", 0o600)
	writePrivate(t, filepath.Join(filepath.Dir(cfgPath), "data", "credentials.toml"), "access_token = \""+cfgTok+"\"\n", 0o600)
	for _, prefix := range [][]string{{"--config", cfgPath}, {"--config=" + cfgPath}} {
		for _, args := range echoArgs(cfgTok, prefix...) {
			assertNotEchoed(t, "--config store: output of "+strings.Join(args, " "), e5.run(t, args...), cfgTok)
		}
	}
}
