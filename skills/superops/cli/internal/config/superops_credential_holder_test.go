// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): credkit-wiring.

package config

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"superops-pp-cli/internal/cliutil"
	"superops-pp-cli/internal/cliutil/testenv"
)

// The non-secret list names only real fields, so a renamed credential field
// cannot hide behind a stale name; every field it does not name is held.
func TestNonSecretConfigFieldsAreRealFields(t *testing.T) {
	typ := reflect.TypeOf(Config{})
	for _, name := range nonSecretConfigFields {
		if _, ok := typ.FieldByName(name); !ok {
			t.Errorf("nonSecretConfigFields names %q, which Config does not have", name)
		}
	}
}

// Every credential a loaded Config holds, from the config file (with its
// [headers] table), the environment and the base URL, is in
// CredentialValues at any length, and Load holds it on every return,
// including a refusal.
func TestLoadHoldsEveryCredentialOnEveryReturn(t *testing.T) {
	for _, tc := range []struct {
		name, baseURL, token, file string
		refused                    bool
		want                       []string
	}{
		{
			name: "headers and file token",
			file: "api_token = \"cfgTokHolder1\"\n[headers]\nAuthorization = \"Bearer sop-secret-123\"\nX-Api-Key = \"Q~z7\"\nCustomerSubDomain = \"acme-holder\"\n",
			want: []string{"cfgTokHolder1", "Bearer sop-secret-123", "Q~z7"},
		},
		{
			name:    "env token and short username",
			baseURL: "https://u~Q:LongPassHolder77@api.superops.ai",
			token:   "envTokHolder22",
			want:    []string{"envTokHolder22", "u~Q", "LongPassHolder77"},
		},
		{
			name:    "refused short token",
			file:    "api_token = \"Zq7\"\n[headers]\nX-Refused = \"R~h\"\n",
			refused: true,
			want:    []string{"Zq7", "R~h"},
		},
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
			var loaded []string
			saved := cliutil.Hold
			cliutil.Hold = func(_ string, values ...string) { loaded = append(loaded, values...) }
			t.Cleanup(func() { cliutil.Hold = saved })
			cfg, err := Load(path)
			if tc.refused != (err != nil) {
				t.Fatalf("Load error %v, want refused=%v", err, tc.refused)
			}
			for _, v := range tc.want {
				if !slices.Contains(loaded, v) {
					t.Errorf("%q is not held after Load: %q", v, loaded)
				}
			}
			if cfg != nil {
				values := cfg.CredentialValues()
				for _, v := range tc.want {
					if !slices.Contains(values, v) {
						t.Errorf("CredentialValues misses %q: %q", v, values)
					}
				}
				for _, v := range values {
					if strings.Contains(v, "acme-holder") || v == cfg.Path {
						t.Errorf("CredentialValues holds a non-secret %q", v)
					}
				}
			}
		})
	}
}

// The CustomerSubDomain header every request sends and the tenant identity
// come from one value (superops-tenant-header-one-source): a [headers]
// spelling of the header is folded into the subdomain, a disagreement is
// refused, and SUPEROPS_SUBDOMAIN replaces both.
func TestTenantHeaderHasOneSource(t *testing.T) {
	for _, tc := range []struct {
		name, file, env, want string
		refused               bool
	}{
		{name: "header only", file: "[headers]\nCustomerSubDomain = \"tenant-a\"\n", want: "tenant-a"},
		{name: "header only, other spelling", file: "[headers]\ncustomersubdomain = \" tenant-b \"\n", want: "tenant-b"},
		{name: "field and header agree", file: "subdomain = \"tenant-a\"\n[headers]\nCUSTOMERSUBDOMAIN = \"tenant-a\"\n", want: "tenant-a"},
		{name: "field and header disagree", file: "subdomain = \"tenant-a\"\n[headers]\nCustomerSubDomain = \"tenant-b\"\n", refused: true},
		{name: "two spellings disagree", file: "[headers]\nCustomerSubDomain = \"tenant-a\"\ncustomersubdomain = \"tenant-b\"\n", refused: true},
		{name: "env replaces both", file: "subdomain = \"tenant-a\"\n[headers]\nCustomerSubDomain = \"tenant-b\"\n", env: "tenant-env", want: "tenant-env"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testenv.Isolate(t)
			clearSuperopsEnv(t)
			t.Setenv("SUPEROPS_API_TOKEN", "tenantHeaderTok1")
			t.Setenv("SUPEROPS_SUBDOMAIN", tc.env)
			path := filepath.Join(t.TempDir(), "config.toml")
			if err := os.WriteFile(path, []byte(tc.file), 0o600); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load(path)
			if tc.refused {
				if err == nil || !strings.Contains(err.Error(), "tenant refused") {
					t.Fatalf("Load = %v, want the tenant refused", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var sent []string
			for name, v := range cfg.Headers {
				if strings.EqualFold(name, "CustomerSubDomain") {
					sent = append(sent, name+"="+v)
				}
			}
			if len(sent) != 1 || sent[0] != "CustomerSubDomain="+tc.want {
				t.Errorf("headers sent %q, want exactly CustomerSubDomain=%s", sent, tc.want)
			}
			if got := cfg.SuperOpsTenant(); !strings.Contains(got, "subdomain="+tc.want+" ") {
				t.Errorf("identity %q does not name the tenant the requests reach (%s)", got, tc.want)
			}
		})
	}
}

// Every decode of a config file holds what it read: the base-URL probe and
// the legacy-file scrub included.
func TestEveryConfigDecodeHoldsWhatItRead(t *testing.T) {
	isHeld := captureHolds(t)
	_ = baseURLFromConfigBytes([]byte(`base_url = "https://cfg-probe-user:CfgProbePass4340@api.example/msp"`))
	legacy := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(legacy, []byte("api_token = \"legacy-scrub-tok-4340\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	(&Config{legacySourcePath: legacy}).scrubLegacyCredentials()
	for _, v := range []string{"cfg-probe-user", "CfgProbePass4340", "legacy-scrub-tok-4340"} {
		if !isHeld(v) {
			t.Errorf("%s is not held", v)
		}
	}
}

// captureHolds records what this test holds; the kit's own matching is proven on
// the built binaries.
func captureHolds(t *testing.T) func(string) bool {
	t.Helper()
	var held []string
	orig := cliutil.Hold
	cliutil.Hold = func(_ string, values ...string) { held = append(held, values...) }
	t.Cleanup(func() { cliutil.Hold = orig })
	return func(v string) bool {
		return slices.ContainsFunc(held, func(h string) bool { return strings.Contains(h, v) })
	}
}
