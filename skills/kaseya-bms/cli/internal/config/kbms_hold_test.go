// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"os"
	"path/filepath"
	"testing"

	"kaseya-bms-pp-cli/internal/cliutil"
	"kaseya-bms-pp-cli/internal/cliutil/testenv"
)

// isolateHome points every config, data, state, cache and credential path at
// a temp dir (KASEYA_BMS_HOME, HOME and the XDG dirs) and clears every Kaseya BMS
// credential and routing variable, so a Load, a save or a mint in these tests
// never reads or writes the operator's own files or reaches a Kaseya BMS platform.
func isolateHome(t *testing.T) string {
	t.Helper()
	home := testenv.Isolate(t)
	t.Setenv("KASEYA_BMS_HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	for _, k := range []string{KbmsEnvUsername, KbmsEnvLoginVar, KbmsEnvTenant, "KASEYA_BMS_TOKEN", "KASEYA_BMS_BEARER_AUTH",
		"KASEYA_BMS_BASE_URL", "KASEYA_BMS_CONFIG", "KASEYA_BMS_CA_CERT", "KASEYA_BMS_TLS_SERVER_NAME"} {
		t.Setenv(k, "")
	}
	return home
}

// setLogin configures the BMS login (username, password, tenant) in the
// environment, as the CLI and the MCP server read it.
func setLogin(t *testing.T, l KbmsLogin) {
	t.Helper()
	t.Setenv(KbmsEnvUsername, l.Username)
	t.Setenv(KbmsEnvLoginVar, l.Password)
	t.Setenv(KbmsEnvTenant, l.Tenant)
}

// output-chokepoint: the configured credentials (the login password in the
// environment, the stored bms_token / bms_bearer_auth and the token minted
// for the login in effect in the config file, a pre-minted KASEYA_BMS_TOKEN
// overriding them all) are held as soon as the configuration loads, before
// any client is built or anything prints, and so is the Authorization value
// they produce ("Bearer <token>"). The username and tenant are identifiers
// (every BMS record carries its TenantName), so they are not held.
func TestLoadHoldsTheConfiguredCredentials(t *testing.T) {
	isolateHome(t)
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.toml")
	const fileToken, fileBearer, access, refresh = "configHoldUnitFileToken-4d8b2e", "configHoldUnitFileBearer-7c1a", "configHoldUnitToken-9f3a1c", "configHoldUnitRefresh-5e2c8a"
	// The identifiers share no piece of 8 characters with a credential: the
	// kit withholds any such piece of a held value (contract 2).
	login := KbmsLogin{Username: "ops.identity@example.org", Password: "configHoldUnitPassword-6a1f8c", Tenant: "AcmeIdentTenant-81c2"}
	const envToken = "configHoldUnitEnvToken-3d9b1f"
	// The file's token was minted for the login in effect on the default host
	// (its binding), but the environment KASEYA_BMS_TOKEN is the bearer (Codex
	// review r1 class credential identity mismatch); every value stays held.
	binding := KbmsTokenBinding("https://api.bms.kaseya.com", login)
	body := "bms_token = \"" + fileToken + "\"\nbms_bearer_auth = \"" + fileBearer + "\"\nclient_id = \"" + binding + "\"\naccess_token = \"" + access + "\"\nrefresh_token = \"" + refresh + "\"\n"
	if err := os.WriteFile(cfg, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	setLogin(t, login)
	t.Setenv("KASEYA_BMS_TOKEN", envToken)
	c, err := Load(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if c.AuthHeader() != "Bearer "+envToken {
		t.Fatalf("AuthHeader() is not the environment KASEYA_BMS_TOKEN as a Bearer")
	}
	if c.KaseyaBmsToken != envToken || KbmsEnvLogin() != (KbmsLogin{Username: login.Username, Password: login.Password, Tenant: login.Tenant}) {
		t.Fatalf("the environment login and token were not loaded")
	}
	for _, v := range []string{login.Password, envToken, fileToken, fileBearer, access, refresh, "Bearer " + access} {
		if !cliutil.Held(v) {
			t.Fatalf("%s is not held after Load", v)
		}
	}
	for _, v := range []string{login.Username, login.Tenant} {
		if cliutil.Held("x " + v + " x") {
			t.Errorf("the identifier %s is held; only the password and the tokens are secrets", v)
		}
	}
}

// A login with a cached token: the token an earlier mint cached in the 0600
// credential store (bound to the host and login: client_id is the binding)
// is held when Load applies it, it is the Authorization value, and the
// environment password is held too.
func TestLoadHoldsTheCachedMintedToken(t *testing.T) {
	isolateHome(t)
	const access, refresh = "cachedMintAccessUnit-7c3e1a", "cachedMintRefreshUnit-2f9b4d"
	login := KbmsLogin{Username: "cachedMintUserUnit-8a1c", Password: "cachedMintPasswordUnit-4e6b", Tenant: "cachedMintTenantUnit"}
	credPath, err := cliutil.CredentialsFilePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(credPath), 0o700); err != nil {
		t.Fatal(err)
	}
	base := "https://api.bms.kaseya.com"
	store := "base_url = \"" + base + "\"\nclient_id = \"" + KbmsTokenBinding(base, login) + "\"\naccess_token = \"" + access + "\"\nrefresh_token = \"" + refresh + "\"\n"
	if err := os.WriteFile(credPath, []byte(store), 0o600); err != nil {
		t.Fatal(err)
	}
	setLogin(t, login)
	c, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if c.AuthHeader() != "Bearer "+access {
		t.Fatalf("AuthHeader() = %q, want the cached minted token (store at %s)", c.AuthHeader(), credPath)
	}
	if c.ClientID != KbmsTokenBinding(base, login) {
		t.Fatalf("the cached token's binding was not loaded: %q", c.ClientID)
	}
	for _, v := range []string{access, refresh, "Bearer " + access, login.Password} {
		if !cliutil.Held("x " + v + " x") {
			t.Errorf("%s is not held after Load", v)
		}
	}
}

// Codex r1 class output-before-chokepoint: a refusal message repeats a
// config's base_url, so the credentials a base URL carries (its userinfo
// password, as written and decoded, and credential-named query values) are
// held, and so is the file layer an environment value overrode (the file's
// bms_token and bms_bearer_auth, replaced by KASEYA_BMS_TOKEN and
// KASEYA_BMS_BEARER_AUTH).
func TestLoadHoldsURLCredentialsAndTheOverriddenFileLayer(t *testing.T) {
	isolateHome(t)
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.toml")
	const urlPw, queryTok, filePw = "urlUserinfoPw%2Fq7Kd3m", "queryTokenUnit-3c9e1a", "filePasswordUnit-8b2d4f"
	body := "base_url = \"https://ro:" + urlPw + "@kbms.example.com/?api_token=" + queryTok + "\"\nbms_bearer_auth = \"fileBearerUnit-2c4e\"\nbms_token = \"" + filePw + "\"\n"
	if err := os.WriteFile(cfg, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KASEYA_BMS_BASE_URL", "")
	t.Setenv("KASEYA_BMS_BEARER_AUTH", "envBearerUnit-6d1f")
	t.Setenv("KASEYA_BMS_TOKEN", "envTokenUnit-1f7a3c")
	t.Setenv(KbmsEnvLoginVar, "envPasswordUnit-0c5e2b")
	if _, err := Load(cfg); err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"urlUserinfoPw/q7Kd3m", urlPw, queryTok, filePw, "fileBearerUnit-2c4e", "envBearerUnit-6d1f", "envTokenUnit-1f7a3c", "envPasswordUnit-0c5e2b"} {
		if !cliutil.Held(v) {
			t.Errorf("%s is not held after Load", v)
		}
	}
}

// Codex r2 class output-before-chokepoint (fixed after the stop rule): a file
// Load refuses still has every value held, so no diagnostic can print one: a
// global credential store recorded for another Kaseya BMS host, and a loose config
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
	store := "base_url = \"https://kbms-other.example.com\"\naccess_token = \"" + storePw + "\"\n"
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
	loosen(t, loose)
	if _, err := Load(loose); err != nil {
		t.Logf("Load (loose config): %v", err)
	}
	for _, v := range []string{storePw, loosePw, looseURLPw} {
		if !cliutil.Held("x " + v + " x") {
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
	t.Setenv("KASEYA_BMS_BASE_URL", "https://ro:"+urlPw+"@kbms.example.com/?api_token="+queryTok)
	setLogin(t, KbmsLogin{Username: "envUserUnit-3e5a", Password: "envPasswordUnit-9a8b7c", Tenant: "envTenantUnit"})
	if _, err := Load(filepath.Join(t.TempDir(), "config.toml")); err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{urlPw, queryTok} {
		if !cliutil.Held("x " + v + " x") {
			t.Errorf("%s from KASEYA_BMS_BASE_URL is not held after Load", v)
		}
	}
}
