// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"superops-pp-cli/internal/cliutil/testenv"
)

// The tenant identity is kept in the local store and the sync checkpoint, and
// the MCP sql tool reads both. A base URL can carry a credential in its
// userinfo or its query, so the identity keeps only scheme, host, port and
// path (superops-tenant-identity-no-credential).
func TestSuperOpsTenantNeverHoldsACredential(t *testing.T) {
	for _, tc := range []struct {
		name, baseURL, want string
		secrets             []string
	}{
		{"userinfo", "http://svc:PwUserinfoSecret@127.0.0.1:8080", "http://127.0.0.1:8080", []string{"PwUserinfoSecret", "svc"}},
		{"userinfo user only", "https://TokenAsUser@api.superops.ai", "https://api.superops.ai", []string{"TokenAsUser"}},
		{"userinfo with path", "https://svc:PwUserinfoSecret@proxy.example:8443/superops/", "https://proxy.example:8443/superops", []string{"PwUserinfoSecret"}},
		{"query", "https://api.superops.ai/?sig=QuerySecretValue", "https://api.superops.ai", []string{"QuerySecretValue", "sig="}},
		{"query no slash", "https://api.superops.ai?sig=QuerySecretValue", "https://api.superops.ai", []string{"QuerySecretValue"}},
		{"fragment", "https://api.superops.ai#FragmentSecret", "https://api.superops.ai", []string{"FragmentSecret"}},
		{"all three", "HTTPS://Svc:PwUserinfoSecret@API.SuperOps.AI:443/Msp/?Sig=QuerySecretValue#FragmentSecret", "https://api.superops.ai:443/msp", []string{"PwUserinfoSecret", "QuerySecretValue", "FragmentSecret"}},
		{"mixed case", "HTTPS://API.SuperOps.AI", "https://api.superops.ai", nil},
		{"trailing slash", "https://api.superops.ai/", "https://api.superops.ai", nil},
		{"trailing slashes on a path", "https://proxy.example/superops//", "https://proxy.example/superops", nil},
		{"surrounding space", "  https://api.superops.ai/  ", "https://api.superops.ai", nil},
		{"ipv6 host and port", "http://svc:PwUserinfoSecret@[::1]:9000", "http://[::1]:9000", []string{"PwUserinfoSecret"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &Config{Subdomain: " Acme-A ", BaseURL: tc.baseURL}
			got := c.SuperOpsTenant()
			want := "subdomain=acme-a base_url=" + tc.want
			if got != want {
				t.Fatalf("SuperOpsTenant() = %q, want %q", got, want)
			}
			for _, secret := range tc.secrets {
				if strings.Contains(strings.ToLower(got), strings.ToLower(secret)) {
					t.Fatalf("SuperOpsTenant() = %q holds %q", got, secret)
				}
			}
		})
	}
}

// A base URL that does not parse, or names no scheme and host, gives an
// identity that holds none of the raw text and cannot equal a real tenant's.
func TestSuperOpsTenantOfAnInvalidBaseURLHoldsNoneOfIt(t *testing.T) {
	for _, raw := range []string{
		"http://svc:PwUserinfoSecret@[::1",           // does not parse
		"http://svc:PwUserinfoSecret\\@host.example", // invalid userinfo
		"http:svc:PwUserinfoSecret@host.example",     // opaque, no host
		"svc:PwUserinfoSecret@host.example",          // no scheme
		"//svc:PwUserinfoSecret@host.example",        // no scheme
		"host.example/PwUserinfoSecret",              // no scheme, no host
		"http://host.example/\x7fPwUserinfoSecret",   // control byte
		"",
	} {
		c := &Config{Subdomain: "acme-a", BaseURL: raw}
		got := c.SuperOpsTenant()
		if got != "subdomain=acme-a base_url=invalid" {
			t.Fatalf("SuperOpsTenant() for %q = %q, want subdomain=acme-a base_url=invalid", raw, got)
		}
		if strings.Contains(strings.ToLower(got), "pwuserinfosecret") || strings.Contains(got, "host.example") {
			t.Fatalf("SuperOpsTenant() for %q = %q holds the raw base URL", raw, got)
		}
	}
}

// A username or password with an unescaped '/', '?' or '#', or an escaped
// '@', is read as host, port, path, query or fragment. The identity of such a
// base URL holds none of it.
func TestSuperOpsTenantOfADisplacedUserinfoHoldsNoneOfIt(t *testing.T) {
	for _, raw := range []string{
		"https://svc:443/PwDisplacedSecret@api.superops.ai",
		"https://svc:443/PwDisplacedSecret%40api.superops.ai",
		"https://svc:443/PwDisplacedSecret%2540api.superops.ai",
		"https://SvcDisplacedUser:4471?x@api.superops.ai",
		"https://SvcDisplacedUser:4471#x%40api.superops.ai",
	} {
		c := &Config{Subdomain: "acme-a", BaseURL: raw}
		got := c.SuperOpsTenant()
		if got != "subdomain=acme-a base_url=invalid" {
			t.Fatalf("SuperOpsTenant() for %q = %q, want subdomain=acme-a base_url=invalid", raw, got)
		}
	}
}

// Load refuses such a base URL, and one with a query or fragment, from the
// environment or the config file, so no request, preview or request error
// ever carries it, and the refusal shows none of it.
func TestLoadRefusesAnUnshowableBaseURL(t *testing.T) {
	const raw = "https://svc:443/PwDisplacedSecret%40api.superops.ai"
	for _, env := range []string{
		raw,
		"https://api.superops.ai/?sig=PwDisplacedSecret",
		"https://api.superops.ai?sig=PwDisplacedSecret",
		"https://api.superops.ai#PwDisplacedSecret",
		"http://svc:PwDisplacedSecret@127.0.0.1:9/?x=1",
	} {
		t.Run("env "+env, func(t *testing.T) {
			testenv.Isolate(t)
			clearSuperopsEnv(t)
			t.Setenv("SUPEROPS_BASE_URL", env)
			cfg, err := Load(filepath.Join(t.TempDir(), "absent.toml"))
			if err == nil || cfg != nil {
				t.Fatalf("Load returned a config (%v), error %v; want the base URL refused", cfg != nil, err)
			}
			if !strings.Contains(err.Error(), "base URL refused") || strings.Contains(strings.ToLower(err.Error()), "pwdisplacedsecret") {
				t.Fatalf("Load error %q", err)
			}
		})
	}
	t.Run("config file", func(t *testing.T) {
		testenv.Isolate(t)
		clearSuperopsEnv(t)
		path := filepath.Join(t.TempDir(), "config.toml")
		if err := os.WriteFile(path, []byte("base_url = \""+raw+"\"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "base URL refused") {
			t.Fatalf("Load error %v, want the base URL refused", err)
		}
	})
	t.Run("ordinary base URLs still load", func(t *testing.T) {
		testenv.Isolate(t)
		clearSuperopsEnv(t)
		for _, ok := range []string{"https://api.superops.ai", "http://svc:p%40ssw0rd%3Fx@127.0.0.1:8080/msp"} {
			t.Setenv("SUPEROPS_BASE_URL", ok)
			if _, err := Load(filepath.Join(t.TempDir(), "absent.toml")); err != nil {
				t.Fatalf("Load with %q: %v", ok, err)
			}
		}
	})
}

// Load refuses a credential shorter than cliutil.MinDetectableSecretLen, from
// the environment, the base URL or the config file, and the refusal shows
// none of it. Longer credentials, and a short account name beside a long
// password, still load.
func TestLoadRefusesACredentialTooShortToDetect(t *testing.T) {
	for _, tc := range []struct{ name, baseURL, token, file string }{
		{"base url password", "https://svc:Zq7x@api.superops.ai", "", ""},
		{"base url escaped password", "https://svc:Zq%407@api.superops.ai", "", ""},
		{"base url username only", "https://Zq7x@api.superops.ai", "", ""},
		{"base url empty password", "https://Zq7x:@api.superops.ai", "", ""},
		{"env token", "", "Zq7xk", ""},
		{"config api_token", "", "", "api_token = \"Zq7\"\n"},
		{"config auth_header", "", "", "auth_header = \"Bearer Zq7\"\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testenv.Isolate(t)
			clearSuperopsEnv(t)
			t.Setenv("SUPEROPS_BASE_URL", tc.baseURL)
			t.Setenv("SUPEROPS_API_TOKEN", tc.token)
			path := filepath.Join(t.TempDir(), "config.toml")
			if tc.file != "" {
				if err := os.WriteFile(path, []byte(tc.file), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			cfg, err := Load(path)
			if err == nil || cfg != nil || !strings.Contains(err.Error(), "credential refused") {
				t.Fatalf("Load returned a config (%v), error %v; want the credential refused", cfg != nil, err)
			}
			if strings.Contains(strings.ToLower(err.Error()), "zq") {
				t.Fatalf("Load error %q shows the credential", err)
			}
		})
	}
	t.Run("long enough", func(t *testing.T) {
		testenv.Isolate(t)
		clearSuperopsEnv(t)
		t.Setenv("SUPEROPS_API_TOKEN", "Zq7xLong8")
		for _, base := range []string{"https://svc:Zq7xLong8@api.superops.ai", "https://Zq7xLong8@api.superops.ai", "https://api.superops.ai"} {
			t.Setenv("SUPEROPS_BASE_URL", base)
			if _, err := Load(filepath.Join(t.TempDir(), "absent.toml")); err != nil {
				t.Fatalf("Load with %q: %v", base, err)
			}
		}
	})
}

// The two SuperOps data centers, and their default spellings, give exactly
// the identity the earlier build gave, so a normal install keeps its store,
// its checkpoint and its default store path.
func TestSuperOpsTenantUnchangedForOrdinaryHosts(t *testing.T) {
	legacy := func(subdomain, baseURL string) string {
		return "subdomain=" + strings.ToLower(strings.TrimSpace(subdomain)) +
			" base_url=" + strings.ToLower(strings.TrimRight(strings.TrimSpace(baseURL), "/"))
	}
	for _, tc := range []struct{ subdomain, baseURL, want string }{
		{"acme", "https://api.superops.ai", "subdomain=acme base_url=https://api.superops.ai"},
		{"acme", "https://euapi.superops.ai", "subdomain=acme base_url=https://euapi.superops.ai"},
		{"Acme", "https://api.superops.ai/", "subdomain=acme base_url=https://api.superops.ai"},
		{"", "https://euapi.superops.ai", "subdomain= base_url=https://euapi.superops.ai"},
		{"acme", "http://127.0.0.1:18791", "subdomain=acme base_url=http://127.0.0.1:18791"},
		{"acme", "https://Proxy.Example:8443/SuperOps/", "subdomain=acme base_url=https://proxy.example:8443/superops"},
	} {
		c := &Config{Subdomain: tc.subdomain, BaseURL: tc.baseURL}
		if got := c.SuperOpsTenant(); got != tc.want || got != legacy(tc.subdomain, tc.baseURL) {
			t.Fatalf("SuperOpsTenant(%q, %q) = %q, want %q (the earlier build's %q)", tc.subdomain, tc.baseURL, got, tc.want, legacy(tc.subdomain, tc.baseURL))
		}
	}
	var nilConfig *Config
	if got := nilConfig.SuperOpsTenant(); got != "" {
		t.Fatalf("nil config tenant = %q, want empty", got)
	}
}

// The default store path hashes the credential and the tenant identity; the
// identity half carries no URL credential either, and two base URLs that
// differ only in their userinfo or query name one store.
func TestStoreScopeIdentityHoldsNoURLCredential(t *testing.T) {
	a := &Config{Subdomain: "acme-a", BaseURL: "http://svc:PwUserinfoSecret@127.0.0.1:8080/?sig=QuerySecretValue", SuperopsApiToken: "sop-test-token"}
	b := &Config{Subdomain: "acme-a", BaseURL: "http://svc:RotatedPassword@127.0.0.1:8080", SuperopsApiToken: "sop-test-token"}
	scopeA, scopeB := a.StoreScopeCredential(), b.StoreScopeCredential()
	for _, secret := range []string{"PwUserinfoSecret", "QuerySecretValue", "RotatedPassword"} {
		if strings.Contains(strings.ToLower(scopeA+scopeB), strings.ToLower(secret)) {
			t.Fatalf("store scope identity holds %q:\n%q\n%q", secret, scopeA, scopeB)
		}
	}
	if scopeA != scopeB {
		t.Fatalf("store scope differs by URL credential only:\n%q\n%q", scopeA, scopeB)
	}
}
