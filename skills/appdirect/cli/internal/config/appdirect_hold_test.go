// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"os"
	"path/filepath"
	"testing"

	"appdirect-pp-cli/internal/cliutil"
	"appdirect-pp-cli/internal/cliutil/testenv"
)

// isolateHome points every config, data, state, cache and credential path at
// a temp dir (APPDIRECT_HOME, HOME and the XDG dirs), so a Load, a save or a
// mint in these tests never reads or writes the operator's own files.
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

// output-chokepoint: the configured credentials (the client pair and the
// token minted for it in the config file, the client pair in the environment
// overriding the file's) are held as soon as the configuration loads, before
// any client is built or anything prints, and so is the Authorization value
// they produce (the configured access token: "Bearer <token>"). AppDirect has
// no static-token variable: the client pair is the only environment
// credential.
func TestLoadHoldsTheConfiguredCredentials(t *testing.T) {
	isolateHome(t)
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.toml")
	const fileID, fileSecret, access, refresh = "configHoldUnitFileId-4d8b2e", "configHoldUnitFileSecret-7c1a", "configHoldUnitToken-9f3a1c", "configHoldUnitRefresh-5e2c8a"
	const clientID, clientSecret = "configHoldUnitClientId-2b7e4d", "configHoldUnitSecret-6a1f8c"
	body := "client_id = \"" + fileID + "\"\nclient_secret = \"" + fileSecret + "\"\naccess_token = \"" + access + "\"\nrefresh_token = \"" + refresh + "\"\n"
	if err := os.WriteFile(cfg, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APPDIRECT_CLIENT_ID", clientID)
	t.Setenv("APPDIRECT_CLIENT_SECRET", clientSecret)
	c, err := Load(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if c.AuthHeader() != "Bearer "+access {
		t.Fatalf("AuthHeader() = %q, want the configured access token as a Bearer", c.AuthHeader())
	}
	if c.ClientID != clientID || c.ClientSecret != clientSecret {
		t.Fatalf("the environment client pair was not loaded")
	}
	for _, v := range []string{clientID, clientSecret, fileID, fileSecret, access, refresh, "Bearer " + access} {
		if !cliutil.HeldEchoes(v) {
			t.Fatalf("%s is not held after Load", v)
		}
	}
}

// client pair with a cached token: the access and refresh tokens an earlier
// mint cached in the 0600 credential store are held when Load applies them,
// and the cached access token is the Authorization value.
func TestLoadHoldsTheCachedMintedToken(t *testing.T) {
	isolateHome(t)
	const access, refresh = "cachedMintAccessUnit-7c3e1a", "cachedMintRefreshUnit-2f9b4d"
	credPath, err := cliutil.CredentialsFilePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(credPath), 0o700); err != nil {
		t.Fatal(err)
	}
	store := "base_url = \"https://marketplace.appdirect.com/api\"\naccess_token = \"" + access + "\"\nrefresh_token = \"" + refresh + "\"\n"
	if err := os.WriteFile(credPath, []byte(store), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APPDIRECT_CLIENT_ID", "cachedMintClientIdUnit-8a1c")
	t.Setenv("APPDIRECT_CLIENT_SECRET", "cachedMintSecretUnit-4e6b")
	c, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if c.AuthHeader() != "Bearer "+access {
		t.Fatalf("AuthHeader() = %q, want the cached minted token (store at %s)", c.AuthHeader(), credPath)
	}
	for _, v := range []string{access, refresh, "Bearer " + access, "cachedMintClientIdUnit-8a1c", "cachedMintSecretUnit-4e6b"} {
		if !cliutil.HeldEchoes("x " + v + " x") {
			t.Errorf("%s is not held after Load", v)
		}
	}
}

// Codex r1 class output-before-chokepoint: a refusal message repeats a
// config's base_url, so the credentials a base URL carries (its userinfo
// password, as written and decoded, and credential-named query values) are
// held, and so is the file layer an environment value overrode (the file's
// client secret, replaced by APPDIRECT_CLIENT_SECRET).
func TestLoadHoldsURLCredentialsAndTheOverriddenFileLayer(t *testing.T) {
	isolateHome(t)
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.toml")
	const urlPw, queryTok, filePw = "urlUserinfoPw%2Fq7Kd3m", "queryTokenUnit-3c9e1a", "filePasswordUnit-8b2d4f"
	body := "base_url = \"https://ro:" + urlPw + "@appdirect.example.com/?api_token=" + queryTok + "\"\nclient_id = \"fileClientUnit-2c4e\"\nclient_secret = \"" + filePw + "\"\n"
	if err := os.WriteFile(cfg, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APPDIRECT_BASE_URL", "")
	t.Setenv("APPDIRECT_CLIENT_ID", "envClientUnit-6d1f")
	t.Setenv("APPDIRECT_CLIENT_SECRET", "envPasswordUnit-1f7a3c")
	if _, err := Load(cfg); err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"urlUserinfoPw/q7Kd3m", urlPw, queryTok, filePw, "envPasswordUnit-1f7a3c"} {
		if !cliutil.HeldEchoes(v) {
			t.Errorf("%s is not held after Load", v)
		}
	}
}

// Codex r2 class output-before-chokepoint (fixed after the stop rule): a file
// Load refuses still has every value held, so no diagnostic can print one: a
// global credential store recorded for another AppDirect host, and a loose config
// whose base_url does not parse.
func TestLoadHoldsRefusedFileValues(t *testing.T) {
	isolateHome(t)
	const storePw, loosePw, looseURLPw = "RefusedStorePw-4d5e6f", "RefusedLoosePw-7a8b9c", "RefusedUrlPw-1c2d3e"
	credPath, err := cliutil.CredentialsFilePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(credPath), 0o700); err != nil {
		t.Fatal(err)
	}
	store := "base_url = \"https://appdirect-other.example.com\"\naccess_token = \"" + storePw + "\"\n"
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
	isolateHome(t)
	const urlPw, queryTok = "envUrlUserinfoPw-5b7c9d", "envQueryTokenUnit-2e4f6a"
	t.Setenv("APPDIRECT_BASE_URL", "https://ro:"+urlPw+"@appdirect.example.com/?api_token="+queryTok)
	t.Setenv("APPDIRECT_CLIENT_ID", "envClientUnit-3e5a")
	t.Setenv("APPDIRECT_CLIENT_SECRET", "envPasswordUnit-9a8b7c")
	if _, err := Load(filepath.Join(t.TempDir(), "config.toml")); err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{urlPw, queryTok} {
		if !cliutil.HeldEchoes("x " + v + " x") {
			t.Errorf("%s from APPDIRECT_BASE_URL is not held after Load", v)
		}
	}
}
