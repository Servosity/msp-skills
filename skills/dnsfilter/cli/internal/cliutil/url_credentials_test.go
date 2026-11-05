// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json under
// the "base-url-credentials-held-and-redacted" entry (class "secret-echo").

package cliutil

import (
	"dnsfilter-pp-cli/internal/credkit"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"dnsfilter-pp-cli/internal/cliutil/testenv"
)

// heldNow reports whether the funnel withholds text that repeats v.
func heldNow(v string) bool {
	return errors.Is(credkit.WriteFile(filepath.Join(os.TempDir(), "dnsfilter-heldnow-"+strconv.Itoa(os.Getpid())), []byte("before "+v+" after"), 0o600), credkit.ErrWithheld)
}

// TestRedactURLCredentialsByShape: every userinfo (username and password
// together, never user:****) and every credential-named query or fragment
// value (to the end of its parameter) is shown as ****, held or not, in plain
// and JSON-escaped text and in a URL that does not parse; a URL without
// credentials is unchanged.
func TestRedactURLCredentialsByShape(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://api.dnsfilter.com", "https://api.dnsfilter.com"},
		{"base_url: https://api.dnsfilter.com\n", "base_url: https://api.dnsfilter.com\n"},
		{"https://api.dnsfilter.com/v1/policies/all?include_global_policies=true&page%5Bsize%5D=100", "https://api.dnsfilter.com/v1/policies/all?include_global_policies=true&page%5Bsize%5D=100"},
		{"https://api.dnsfilter.com/v1/users/a@example.com", "https://api.dnsfilter.com/v1/users/a@example.com"},
		{"https://u:pw@api.dnsfilter.com/x", "https://****@api.dnsfilter.com/x"},
		{"https://tokenvalue@api.dnsfilter.com", "https://****@api.dnsfilter.com"},
		{"https://u:ab/cd@api.dnsfilter.com/x", "https://****@api.dnsfilter.com/x"},
		{"https://u:p@ss@api.dnsfilter.com", "https://****@api.dnsfilter.com"},
		{"https://api.dnsfilter.com/?api_key=abc", "https://api.dnsfilter.com/?api_key=****"},
		{"https://h.example/x?page=2&access_token=abc&x=1", "https://h.example/x?page=2&access_token=****&x=1"},
		{"https://h.example/x?Api%5FKey=abc", "https://h.example/x?Api%5FKey=****"},
		{"https://h.example/cb#access_token=abc&state=s", "https://h.example/cb#access_token=****&state=s"},
		{"https://h.example/?sig=abc;password=def", "https://h.example/?sig=****"},
		{`creating request: parse "ht!tp://u:pw@[::1/v1/current_user": first path segment in URL cannot contain colon`,
			`creating request: parse "ht!tp://****@[::1/v1/current_user": first path segment in URL cannot contain colon`},
		{`Get "http://127.0.0.1:9/?api_key=abc/v1/current_user": dial tcp`, `Get "http://127.0.0.1:9/?api_key=****": dial tcp`},
		{`network error (Get "https://u:***@h.example/x": EOF)`, `network error (Get "https://****@h.example/x": EOF)`},
		{`{"error":"parse \"https://u:pw@h.example/x\": bad"}`, `{"error":"parse \"https://****@h.example/x\": bad"}`},
		{`{"error":"parse \"https://h.example/?api_key=abc\": bad"}`, `{"error":"parse \"https://h.example/?api_key=****\": bad"}`},
		{`{"u":"https:\/\/u:pw@h.example\/x"}`, `{"u":"https:\/\/****@h.example\/x"}`},
		{"one https://u:a@h1.example two http://h2.example/?token=b three", "one https://****@h1.example two http://h2.example/?token=****"},
		{"already https://u:****@h.example", "already https://****@h.example"},
		{"mail a@example.com and key=value", "mail a@example.com and key=value"},
	}
	for _, c := range cases {
		if got := RedactURLCredentials(c.in); got != c.want {
			t.Errorf("RedactURLCredentials(%q)\n got %q\nwant %q", c.in, got, c.want)
		}
		if strings.HasPrefix(c.in, "{") {
			var v any
			if err := json.Unmarshal([]byte(RedactURLCredentials(c.in)), &v); err != nil {
				t.Errorf("redacting %q broke its JSON: %v", c.in, err)
			}
		}
	}
}

// TestHoldURLCredentialsReadsTheRawText: a URL is held from its text, so one
// url.Parse rejects (or one with no scheme) is held too, and a held URL
// credential is masked as written outside any URL shape.
func TestHoldURLCredentialsReadsTheRawText(t *testing.T) {
	if inFreshProcess(t) {
		return
	}
	cases := []struct{ raw, secret string }{
		{"ht!tp://u:HoldRawA1b2c3d4@[::1", "HoldRawA1b2c3d4"},
		{"https://u:HoldRawSlash/e5f6g7@h.example/x", "HoldRawSlash/e5f6g7"},
		{"u:HoldRawNoScheme9h8@h.example", "HoldRawNoScheme9h8"},
		{"https://HoldRawUserSlot77k@h.example", "HoldRawUserSlot77k"},
		{"https://h.example/?api_key=HoldRawQuery55m6", "HoldRawQuery55m6"},
		{"https://h.example/?token=HoldRaw%2BEsc44n", "HoldRaw+Esc44n"},
		{"https://h.example/cb#access_token=hold-raw-fragment-0000", "hold-raw-fragment-0000"},
	}
	for _, c := range cases {
		if heldNow(c.secret) {
			t.Fatalf("%s held before the test held it", c.secret)
		}
		holdBaseURL(c.raw)
		if !heldNow(c.secret) {
			t.Errorf("holdBaseURL(%q) did not hold %q", c.raw, c.secret)
		}
	}
	holdBaseURL("https://api.dnsfilter.com/v1")
	if heldNow("api.dnsfilter.com") || heldNow("https://api.dnsfilter.com") {
		t.Error("a URL without credentials was held")
	}
}

// TestHoldConfigFileCredentialsWhenTOMLFails: the bytes of a config file are
// held as read, whether or not TOML accepts them: credential-named keys at any
// depth, every [headers] value, and a URL's credentials; a setting that is
// not a credential is not held.
func TestHoldConfigFileCredentialsWhenTOMLFails(t *testing.T) {
	if inFreshProcess(t) {
		return
	}
	data := []byte(strings.Join([]string{
		`base_url = "ht!tp://u:CfgBaseUrlPw1234@[::1"`,
		`ca_cert = "/etc/ssl/CfgNotASecretPath.pem"`,
		`token_expiry = 2026-10-28T00:00:00Z`,
		`api_key = "CfgApiKeyValue5678" # trailing comment`,
		`auth_header = 'Bearer CfgAuthHeaderLit90'`,
		`client_secret = CfgBareSecretValue12`,
		`refresh_token = "CfgUnterminatedValue34`,
		`access_token = """`,
		`CfgMultiLineToken56`,
		`"""`,
		`headers = { X-Inline = "CfgInlineHeader78" }`,
		`[headers]`,
		`X-Org = "CfgHeaderValue9012"`,
		`[profiles.tenant]`,
		`token = "CfgNestedToken3456"`,
		`broken = = 1`,
	}, "\n"))
	secrets := []string{"CfgBaseUrlPw1234", "CfgApiKeyValue5678", "CfgAuthHeaderLit90", "CfgBareSecretValue12",
		"CfgUnterminatedValue34", "CfgMultiLineToken56", "CfgInlineHeader78", "CfgHeaderValue9012", "CfgNestedToken3456"}
	for _, s := range secrets {
		if heldNow(s) {
			t.Fatalf("%s held before the file was read", s)
		}
	}
	HoldConfigFile(data)
	for _, s := range secrets {
		if !heldNow(s) {
			t.Errorf("a config TOML rejects did not hold %q", s)
		}
	}
	if heldNow("/etc/ssl/CfgNotASecretPath.pem") || heldNow("2026-10-28T00:00:00Z") {
		t.Error("a setting that is not a credential was held")
	}
}

// TestHoldConfigFileCredentialsDecodesAParsedFile: a file TOML accepts is
// also walked, so an escaped value is held as it decodes.
func TestHoldConfigFileCredentialsDecodesAParsedFile(t *testing.T) {
	if inFreshProcess(t) {
		return
	}
	HoldConfigFile([]byte("base_url = \"https://u:CfgEsc\\u0041ped77@h.example\"\n[headers]\nX-Org = \"CfgHdr\\u0042Esc88\"\n"))
	for _, s := range []string{"CfgEscAped77", "CfgHdrBEsc88"} {
		if !heldNow(s) {
			t.Errorf("the decoded value %q was not held", s)
		}
	}
}

// TestCredentialRefusalRedactsURLCredentials: a refusal that names a base
// URL never shows its credentials.
func TestCredentialRefusalRedactsURLCredentials(t *testing.T) {
	r := CredentialRefusal{Source: "config-kind path", Path: "/p/config.toml", Err: errors.New(`untrusted config names base_url "https://u:RefusalPw12@h.example/?api_key=RefusalKey34"; credentials were not applied`), CredentialsPresent: true}
	got := r.Error()
	if strings.Contains(got, "RefusalPw12") || strings.Contains(got, "RefusalKey34") {
		t.Fatalf("refusal shows a URL credential: %s", got)
	}
	if !strings.Contains(got, "https://****@h.example/?api_key=****") {
		t.Fatalf("refusal lost its redacted URL: %s", got)
	}
}

// TestCredentialsFileBytesHeldWhenRead: a credentials file is held the moment
// its bytes are read, whether it is accepted and fails to parse or is refused
// for its permissions.
func TestCredentialsFileBytesHeldWhenRead(t *testing.T) {
	if inFreshProcess(t) {
		return
	}
	testenv.Isolate(t, ConfigDir, DataDir, StateDir, CacheDir)
	path, err := credentialsPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	write := func(body string, perm os.FileMode) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), perm); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, perm); err != nil {
			t.Fatal(err)
		}
	}
	write("base_url = \"https://u:CredsMalformedPw56@h.example\"\napi_key = \"CredsMalformedKey78\"\nbroken = = 1\n", 0o600)
	_, _, _ = LoadCredentialsWithStatus()
	for _, s := range []string{"CredsMalformedPw56", "CredsMalformedKey78"} {
		if !heldNow(s) {
			t.Errorf("an accepted credentials file TOML rejects did not hold %q", s)
		}
	}
	if runtime.GOOS == "windows" {
		return // a mode refusal is a Unix verdict
	}
	write("base_url = \"https://u:CredsRefusedPw90@h.example\"\napi_key = \"CredsRefusedKey12\"\n", 0o644)
	_, status, _ := LoadCredentialsWithStatus()
	if status.Refusal == nil {
		t.Fatal("a 0644 credentials file was not refused")
	}
	for _, s := range []string{"CredsRefusedPw90", "CredsRefusedKey12"} {
		if !heldNow(s) {
			t.Errorf("a refused credentials file did not hold %q", s)
		}
	}
}
