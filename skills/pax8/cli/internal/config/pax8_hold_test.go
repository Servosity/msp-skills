// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"os"
	"path/filepath"
	"testing"

	"pax8-pp-cli/internal/cliutil"
	"pax8-pp-cli/internal/cliutil/testenv"
)

// output-chokepoint: the configured client credentials and a stored access
// token are held as soon as the configuration loads, before any client is
// built or anything prints, and so is the Authorization header built from the
// token.
func TestLoadHoldsTheConfiguredCredentials(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfg, []byte("access_token = \"configHoldUnitToken-9f3a1c\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PAX8_CLIENT_ID", "configHoldUnitId-2b7e4d")
	t.Setenv("PAX8_CLIENT_SECRET", "configHoldUnitSecret-6a1f8c")
	if _, err := Load(cfg); err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"configHoldUnitId-2b7e4d", "configHoldUnitSecret-6a1f8c", "configHoldUnitToken-9f3a1c", "Bearer configHoldUnitToken-9f3a1c"} {
		if !cliutil.HeldEchoes(v) {
			t.Fatalf("%s is not held after Load", v)
		}
	}
}

// Codex r1 class output-before-chokepoint: a refusal message repeats a
// config's base_url, so the credentials a base URL carries (its userinfo
// password, as written and decoded, and credential-named query values) are
// held, and so is the file layer an environment value overrode.
func TestLoadHoldsURLCredentialsAndTheOverriddenFileLayer(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.toml")
	const urlPw, queryTok, filePw = "urlUserinfoPw%2Fq7Kd3m", "queryTokenUnit-3c9e1a", "filePasswordUnit-8b2d4f"
	body := "base_url = \"https://ro:" + urlPw + "@pax8.example.com/?api_token=" + queryTok + "\"\naccess_token = \"" + filePw + "\"\n"
	if err := os.WriteFile(cfg, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PAX8_BASE_URL", "")
	t.Setenv("PAX8_TOKEN", "envPasswordUnit-1f7a3c")
	if _, err := Load(cfg); err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"urlUserinfoPw/q7Kd3m", urlPw, queryTok, filePw} {
		if !cliutil.HeldEchoes(v) {
			t.Errorf("%s is not held after Load", v)
		}
	}
}

// Codex r2 class output-before-chokepoint (fixed after the stop rule): a file
// Load refuses still has every value held, so no diagnostic can print one: a
// global credential store recorded for another Pax8 host, and a loose config
// whose base_url does not parse.
func TestLoadHoldsRefusedFileValues(t *testing.T) {
	testenv.Isolate(t)
	const storePw, loosePw, looseURLPw = "RefusedStorePw-4d5e6f", "RefusedLoosePw-7a8b9c", "RefusedUrlPw-1c2d3e"
	credPath, err := cliutil.CredentialsFilePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(credPath), 0o700); err != nil {
		t.Fatal(err)
	}
	store := "base_url = \"https://pax8-other.example.com\"\naccess_token = \"" + storePw + "\"\n"
	if err := os.WriteFile(credPath, []byte(store), 0o600); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(other, []byte("base_url = \"https://other.example.com\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(other); err != nil {
		t.Logf("Load (refused store): %v", err)
	}
	loose := filepath.Join(t.TempDir(), "loose.toml")
	if err := os.WriteFile(loose, []byte("base_url = \"https://ro:"+looseURLPw+"@other.example.com:bad/\"\naccess_token = \""+loosePw+"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.Chmod(loose, 0o644)
	if _, err := Load(loose); err != nil {
		t.Logf("Load (loose config): %v", err)
	}
	for _, v := range []string{storePw, loosePw, looseURLPw} {
		if !cliutil.HeldEchoes("x " + v + " x") {
			t.Errorf("%s from a refused file is not held", v)
		}
	}
}

// A base URL from the environment never passes a file parser, so only the
// configuration holder sees its userinfo password and credential-named query
// values.
func TestLoadHoldsEnvironmentURLCredentials(t *testing.T) {
	testenv.Isolate(t)
	const urlPw, queryTok = "envUrlUserinfoPw-5b7c9d", "envQueryTokenUnit-2e4f6a"
	t.Setenv("PAX8_BASE_URL", "https://ro:"+urlPw+"@pax8.example.com/?api_token="+queryTok)
	t.Setenv("PAX8_TOKEN", "envPasswordUnit-9a8b7c")
	if _, err := Load(filepath.Join(t.TempDir(), "config.toml")); err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{urlPw, queryTok} {
		if !cliutil.HeldEchoes("x " + v + " x") {
			t.Errorf("%s from PAX8_BASE_URL is not held after Load", v)
		}
	}
}
