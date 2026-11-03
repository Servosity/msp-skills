// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func isHeld(v string) bool { return slices.Contains(heldValues(), v) }

func TestContract1BearerAndBasicParts(t *testing.T) {
	resetForTest(Declarations{})
	Hold("header:authorization", "Bearer tok-0123456789")
	basic := base64.StdEncoding.EncodeToString([]byte("svc-user:pw-9876543210"))
	Hold("header:authorization", "Basic "+basic)
	for _, v := range []string{"Bearer tok-0123456789", "tok-0123456789", basic, "svc-user:pw-9876543210", "pw-9876543210", "svc-user"} {
		if !isHeld(v) {
			t.Errorf("contract 1: %q not held", v)
		}
	}
}

func TestContract1IdentifierExemptOnlyWithReason(t *testing.T) {
	d := Declarations{Identifiers: []Identifier{{Source: "basic-user", Reason: "the account name is printed by whoami"}}}
	resetForTest(d)
	Hold("header:authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("svc-user:pw-9876543210")))
	if isHeld("svc-user") {
		t.Error("a declared identifier was held")
	}
	if !isHeld("pw-9876543210") {
		t.Error("the password was exempted with the identifier")
	}
	bad := Declarations{Identifiers: []Identifier{{Source: "env:ZAMMAD_URL"}}}
	if bad.validate() == nil {
		t.Error("contract 1: an identifier without a reason was accepted")
	}
}

func TestContract1IdentifierDoesNotExemptOtherSources(t *testing.T) {
	resetForTest(Declarations{Identifiers: []Identifier{{Source: "config:user", Reason: "a login name"}}})
	Hold("config:user", "shared-value-123")
	Hold("env:TOKEN", "shared-value-123")
	if !isHeld("shared-value-123") {
		t.Error("a value also held through a credential source was exempted")
	}
}

func TestContract1NoLengthExemption(t *testing.T) {
	resetForTest(Declarations{})
	Hold("env:PIN", "7")
	if !leaks("pin 7 ok") {
		t.Error("contract 1: a one-character credential is not held")
	}
}

func TestContract1HeldUntilExit(t *testing.T) {
	resetForTest(Declarations{})
	Hold("minted:session", "sess-abcdef123456")
	Hold("minted:session", "sess-zzzzzz999999")
	if !isHeld("sess-abcdef123456") || !isHeld("sess-zzzzzz999999") {
		t.Error("a later value replaced an earlier one")
	}
}

const tomlConfig = `# zammad config
base_url = "https://admin:base-pass-777@helpdesk.example.com/api?token=query-secret-1&version=2"
auth_header = "Token token=auth-header-secret"
api_token = "first-token-11111"
api_token = "second-token-22222"
timeout = 30

[headers]
X-Api-Key = "header-secret-333"

[profiles.work]
api_token = 'literal-token-4444'
`

func gatherString(t *testing.T, d Declarations, name, content string) {
	t.Helper()
	resetForTest(d)
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	atStartup(func() { gatherFile(p) })
}

// atStartup runs f as Start runs gather: what it holds is a startup
// credential (contract 2's public-text rule tells the two apart).
func atStartup(f func()) {
	reg.Lock()
	reg.startup = true
	reg.Unlock()
	f()
	reg.Lock()
	reg.startup = false
	reg.Unlock()
}

func TestContract1ConfigFile(t *testing.T) {
	gatherString(t, Declarations{ConfigFields: []string{"api_token"}}, "config.toml", tomlConfig)
	for _, v := range []string{"base-pass-777", "query-secret-1", "2", "auth-header-secret", "first-token-11111", "second-token-22222", "header-secret-333", "literal-token-4444"} {
		if !isHeld(v) {
			t.Errorf("contract 1 config: %q not held", v)
		}
	}
	if isHeld("30") || isHeld("admin") {
		t.Error("a non-credential config value was held")
	}
	reg.Lock()
	h := reg.headers["x-api-key"]
	reg.Unlock()
	if !h {
		t.Error("a [headers] name was not recorded for the outbound guard")
	}
}

func TestContract1ConfigFileThatDoesNotParse(t *testing.T) {
	broken := "api_token = \"unterminated-token-5555\n[headers\nX-Key: \"hdr-6666666\"\n{{{ = = \npassword: yaml-pass-77777 # note\n"
	gatherString(t, Declarations{ConfigFields: []string{"api_token", "password"}}, "broken.toml", broken)
	for _, v := range []string{"unterminated-token-5555", "yaml-pass-77777"} {
		if !isHeld(v) {
			t.Errorf("contract 1 unparseable file: %q not held", v)
		}
	}
}

func TestContract1JSONCredentialStore(t *testing.T) {
	gatherString(t, Declarations{StoredRecords: []string{"oauth.access_token", "refresh_token"}}, "credentials.json",
		`{"oauth":{"access_token":"json-access-8888","expires":3600},"refresh_token":["r1-aaaaaaaa","r2-bbbbbbbb"],"headers":{"X-Tenant":"tenant-hdr-9999"}}`)
	for _, v := range []string{"json-access-8888", "r1-aaaaaaaa", "r2-bbbbbbbb", "tenant-hdr-9999"} {
		if !isHeld(v) {
			t.Errorf("contract 1 credential store: %q not held", v)
		}
	}
	if isHeld("3600") {
		t.Error("a non-credential field was held")
	}
}

func TestContract1UnreadableFileFailsClosed(t *testing.T) {
	resetForTest(Declarations{})
	// A path that exists but cannot be read as a file, on every OS and
	// for every user.
	p := filepath.Join(t.TempDir(), "config.toml")
	os.Mkdir(p, 0o700)
	atStartup(func() { gatherFile(p) })
	if !leaks("clean") {
		t.Error("an unreadable credential file did not make every unit withheld")
	}
	resetForTest(Declarations{})
	atStartup(func() { gatherFile(filepath.Join(t.TempDir(), "missing.toml")) })
	if leaks("clean") {
		t.Error("a missing credential file withheld everything")
	}
}

func TestContract1StartupGather(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.toml")
	legacy := filepath.Join(dir, "legacy.toml")
	flagCfg := filepath.Join(dir, "flag.toml")
	os.WriteFile(cfg, []byte(`api_token = "cfg-token-1234"`), 0o600)
	os.WriteFile(legacy, []byte(`api_token = "legacy-token-5678"`), 0o600)
	os.WriteFile(flagCfg, []byte(`api_token = "flagcfg-token-9012"`), 0o600)
	t.Setenv("CK_TOKEN", "env-token-3456")
	t.Setenv("CK_BASE", "https://u:env-base-pass@api.example.com")
	d := Declarations{
		SecretEnv:       []string{"CK_TOKEN"},
		Files:           func([]string) []string { return []string{cfg, legacy} },
		ConfigFields:    []string{"api_token"},
		CredentialFlags: []string{"--token", "-t"},
		BaseURL:         BaseURL{Env: "CK_BASE", Default: "https://default.example.com"},
	}
	resetForTest(d)
	gather(d, []string{"tickets", "--token", "flag-token-a", "--token=flag-token-b", "-tflag-token-c", "-t", "flag-token-d", "--config=" + flagCfg})
	for _, v := range []string{"cfg-token-1234", "legacy-token-5678", "flagcfg-token-9012", "env-token-3456", "env-base-pass",
		"flag-token-a", "flag-token-b", "flag-token-c", "flag-token-d"} {
		if !isHeld(v) {
			t.Errorf("startup gather: %q not held", v)
		}
	}
	if reg.baseURL != "https://u:env-base-pass@api.example.com" {
		t.Errorf("base URL = %q, want the env override", reg.baseURL)
	}
}

func TestContract1StdinWhenDeclared(t *testing.T) {
	r, w, _ := os.Pipe()
	w.Write([]byte("stdin-secret-424242\n"))
	w.Close()
	old := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = old }()
	d := Declarations{StdinFlags: []string{"--token-stdin"}}
	resetForTest(d)
	gather(d, []string{"login", "--token-stdin"})
	if !isHeld("stdin-secret-424242") {
		t.Fatal("contract 1: a credential read from stdin was not held")
	}
	b := make([]byte, 64)
	n, _ := os.Stdin.Read(b)
	if string(b[:n]) != "stdin-secret-424242\n" {
		t.Errorf("the program did not get stdin back: %q", b[:n])
	}
}

func TestContract7DeclarationValidation(t *testing.T) {
	for _, d := range []Declarations{
		{Origins: []Origin{{Source: "*", Allowed: []string{"api.example.com"}}}},
		{Origins: []Origin{{Source: "*", Allowed: []string{"https://api.example.com/path"}}}},
		{CredentialFlags: []string{"token"}},
	} {
		if d.validate() == nil {
			t.Errorf("declarations %+v accepted", d)
		}
	}
	ok := Declarations{Origins: []Origin{{Source: "*", Allowed: []string{"base_url", "https://*.example.com", "https://api.example.com:8443"}}}}
	if err := ok.validate(); err != nil {
		t.Error(err)
	}
}

func TestContract1AuthParamsNotPadding(t *testing.T) {
	resetForTest(Declarations{})
	Hold("header:authorization", "Token token=zammad-tok-1", "Basic YWI6Y2Q=")
	if !isHeld("zammad-tok-1") {
		t.Error("an auth parameter's value was not held")
	}
	if isHeld("=") || isHeld("") {
		t.Error("base64 padding was held as a credential")
	}
}
