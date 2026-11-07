// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"microsoft-graph-pp-cli/internal/cliutil/testenv"
	"microsoft-graph-pp-cli/internal/credkit"
)

// heldNow reports whether the credential kit withholds text from a file write.
func heldNow(t *testing.T, text string) bool {
	t.Helper()
	err := credkit.WriteFile(filepath.Join(t.TempDir(), "probe"), []byte(text), 0o600)
	if err != nil && !errors.Is(err, credkit.ErrWithheld) {
		t.Fatalf("probe write: %v", err)
	}
	return errors.Is(err, credkit.ErrWithheld)
}

// credkit-wiring: the configured bearer token is held as soon as the
// configuration loads, before any client is built or anything prints, and so
// is the Authorization header built from it.
func TestLoadHoldsTheConfiguredToken(t *testing.T) {
	t.Setenv("MICROSOFT_GRAPH_TOKEN", "configHoldUnitToken-9f3a1c")
	if _, err := Load(filepath.Join(t.TempDir(), "config.toml")); err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"configHoldUnitToken-9f3a1c", "Bearer configHoldUnitToken-9f3a1c"} {
		if !heldNow(t, v) {
			t.Fatalf("%s is not held after Load", v)
		}
	}
}

// A refusal message repeats a config's base_url, so the credentials a base URL
// carries (its userinfo password and query values) are held when the config
// loads, and so is the file layer an environment value overrode. A file the
// run reads at startup is held by the kit itself (internal/credwire Files).
func TestLoadHoldsURLCredentialsAndTheOverriddenFileLayer(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.toml")
	const urlPw, queryTok, filePw = "urlUserinfoPw%2Fq7Kd3m", "queryTokenUnit-3c9e1a", "filePasswordUnit-8b2d4f"
	body := "base_url = \"https://ro:" + urlPw + "@graph.example.com/?api_token=" + queryTok + "\"\naccess_token = \"" + filePw + "\"\n"
	if err := os.WriteFile(cfg, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MICROSOFT_GRAPH_BASE_URL", "")
	t.Setenv("MICROSOFT_GRAPH_TOKEN", "envPasswordUnit-1f7a3c")
	if _, err := Load(cfg); err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"urlUserinfoPw/q7Kd3m", urlPw, queryTok, filePw} {
		if !heldNow(t, v) {
			t.Errorf("%s is not held after Load", v)
		}
	}
}

// A base URL from the environment is held through the kit's own URL reader
// when the configuration loads (the MCP server loads it on every call).
func TestLoadHoldsEnvironmentURLCredentials(t *testing.T) {
	testenv.Isolate(t)
	const urlPw, queryTok = "envUrlUserinfoPw-5b7c9d", "envQueryTokenUnit-2e4f6a"
	t.Setenv("MICROSOFT_GRAPH_BASE_URL", "https://ro:"+urlPw+"@graph.example.com/?api_token="+queryTok)
	t.Setenv("MICROSOFT_GRAPH_TOKEN", "envPasswordUnit-9a8b7c")
	if _, err := Load(filepath.Join(t.TempDir(), "config.toml")); err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{urlPw, queryTok} {
		if !heldNow(t, "x "+v+" x") {
			t.Errorf("%s from MICROSOFT_GRAPH_BASE_URL is not held after Load", v)
		}
	}
}
