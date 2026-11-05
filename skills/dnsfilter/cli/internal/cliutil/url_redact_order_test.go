// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json under
// the "held-values-redacted-before-url-rewrite" entry (class "secret-echo").

package cliutil

import (
	"encoding/json"
	"strings"
	"testing"
)

// assertRedactedWithout fails when RedactURLCredentials(in) still shows any of
// the secrets.
func assertRedactedWithout(t *testing.T, in string, secrets ...string) string {
	t.Helper()
	got := RedactURLCredentials(in)
	for _, s := range secrets {
		if strings.Contains(got, s) {
			t.Errorf("RedactURLCredentials(%q) = %q still shows %q", in, got, s)
		}
	}
	return got
}

// TestRedactCredentialValueToEndOfParameter: a credential-named query or
// fragment value is masked to the end of its parameter (the next '&' or '#',
// the end of the URL), never only to the first space or quote, held or not,
// in plain, quoted and JSON text and in URLs url.Parse rejects.
func TestRedactCredentialValueToEndOfParameter(t *testing.T) {
	const key = "OrdShapeKey" + "0123456789abcd"
	cases := []struct{ in, want string }{
		// The whole text is the URL (doctor's base_url).
		{"http://127.0.0.1:1/?api_key=prefix " + key, "http://127.0.0.1:1/?api_key=****"},
		{"http://127.0.0.1:1/?api_key=prefix " + key + "&page=2", "http://127.0.0.1:1/?api_key=****&page=2"},
		{"http://127.0.0.1:1/?api_key=prefix " + key + "#frag", "http://127.0.0.1:1/?api_key=****#frag"},
		{"http://127.0.0.1:1/?api_key=a'b\"c " + key, "http://127.0.0.1:1/?api_key=****"},
		{"http://127.0.0.1:1/?api_key=abc;" + key, "http://127.0.0.1:1/?api_key=****"},
		{"http://127.0.0.1:1/?api_key=abc?" + key, "http://127.0.0.1:1/?api_key=****"},
		// Go's %q quoting (url.Error, parse errors): the URL runs to its
		// closing quote, through spaces and escaped quotes.
		{`Get "http://127.0.0.1:1/?api_key=prefix ` + key + `/v1/x": dial tcp`, `Get "http://127.0.0.1:1/?api_key=****": dial tcp`},
		{`Get "http://127.0.0.1:1/?token=a\"b ` + key + `": EOF`, `Get "http://127.0.0.1:1/?token=****": EOF`},
		// A URL url.Parse rejects.
		{`parse "ht!tp://127.0.0.1:1/?api_key=prefix ` + key + `": first path segment`, `parse "ht!tp://127.0.0.1:1/?api_key=****": first path segment`},
		{"ht!tp://[::1/?api_key=prefix " + key, "ht!tp://[::1/?api_key=****"},
		// A percent-encoded key.
		{"http://127.0.0.1:1/?api_key=" + strings.ReplaceAll(key, "Key", "%4Bey"), "http://127.0.0.1:1/?api_key=****"},
		// A credential name behind a second '?' or a ';' is still found.
		{"http://h.example/?a=1?api_key=" + key, "http://h.example/?a=1?api_key=****"},
		{"http://h.example/?page=2;token=" + key + " tail", "http://h.example/?page=2;token=****"},
		// A non-credential parameter is unchanged and still ends at a space.
		{"http://h.example/?page=2 and more", "http://h.example/?page=2 and more"},
		// An already-masked value keeps the text after it.
		{"GET http://h.example/x?token=****: HTTP 401", "GET http://h.example/x?token=****: HTTP 401"},
		// Text that only starts like a mask is still a credential.
		{"http://h.example/x?token=****" + key + " tail", "http://h.example/x?token=****"},
	}
	for _, c := range cases {
		got := assertRedactedWithout(t, c.in, key, "prefix", strings.ReplaceAll(key, "Key", "%4Bey"))
		if got != c.want {
			t.Errorf("RedactURLCredentials(%q)\n got %q\nwant %q", c.in, got, c.want)
		}
	}
	// JSON text: the value runs to the string's closing quote and the JSON
	// stays valid.
	for _, in := range []string{
		`{"error":"Get \"http://127.0.0.1:1/?api_key=prefix ` + key + `\": refused","code":1}`,
		`{"base_url":"http://127.0.0.1:1/?api_key=prefix ` + key + `"}`,
		`{"u":"http:\/\/127.0.0.1:1\/?api_key=prefix ` + key + `"}`,
	} {
		got := assertRedactedWithout(t, in, key, "prefix")
		var v any
		if err := json.Unmarshal([]byte(got), &v); err != nil {
			t.Errorf("redacting %q broke its JSON: %v (%q)", in, err, got)
		}
	}
}

// TestRedactWholeUserinfo: a URL's userinfo is masked whole, username and
// password, never user:****; held or not.
func TestRedactWholeUserinfo(t *testing.T) {
	const user = "OrdUserSlot0123456789abcd"
	cases := []struct{ in, want string }{
		{"https://" + user + ":x@api.dnsfilter.com", "https://****@api.dnsfilter.com"},
		{"https://" + user + "@api.dnsfilter.com", "https://****@api.dnsfilter.com"},
		{"https://" + user + ":pw/with/slash@h.example/x", "https://****@h.example/x"},
		{`Get "https://` + user + `:***@h.example/x": EOF`, `Get "https://****@h.example/x": EOF`},
		{`Get "https://` + user + ` space:pw@h.example/x": EOF`, `Get "https://****@h.example/x": EOF`},
		{"ht!tp://" + user + ":pw@[::1", "ht!tp://****@[::1"},
		{"already https://****@h.example", "already https://****@h.example"},
	}
	for _, c := range cases {
		if got := assertRedactedWithout(t, c.in, user); got != c.want {
			t.Errorf("RedactURLCredentials(%q)\n got %q\nwant %q", c.in, got, c.want)
		}
	}
}

// TestHoldURLCredentialsHoldsEveryPart: a base URL holds its userinfo
// username even when a password follows, its password, and each
// credential-named value as the URL carries it, raw and percent-decoded.
func TestHoldURLCredentialsHoldsEveryPart(t *testing.T) {
	if inFreshProcess(t) {
		return
	}
	cases := []struct{ raw, secret string }{
		{"https://OrdHoldUser0123456:x@api.dnsfilter.com", "OrdHoldUser0123456"},
		{"https://OrdHoldUser%41Enc789:x@api.dnsfilter.com", "OrdHoldUserAEnc789"},
		{"https://u:OrdHoldPw0123456789@api.dnsfilter.com", "OrdHoldPw0123456789"},
		{"http://127.0.0.1:1/?api_key=prefix OrdHoldSpace0123", "prefix OrdHoldSpace0123"},
		{"http://127.0.0.1:1/?api_key=OrdHold%5FPct0123", "OrdHold_Pct0123"},
		{"http://127.0.0.1:1/?api_key=OrdHold%5FPct0123", "OrdHold%5FPct0123"},
		{"http://127.0.0.1:1/?api_key=OrdHoldSemi;tail0123", "OrdHoldSemi;tail0123"},
		{"ht!tp://[::1/?api_key=a b OrdHoldMalformed01", "a b OrdHoldMalformed01"},
		{"http://127.0.0.1:1/?a=1?api_key=" + "OrdHoldSecondQ0123", "OrdHoldSecondQ0123"},
		{"http://127.0.0.1:1/?page=2;token=" + "OrdHoldAfterSemi01", "OrdHoldAfterSemi01"},
	}
	for _, c := range cases {
		holdBaseURL(c.raw)
		if !heldNow(c.secret) {
			t.Errorf("holdBaseURL(%q) did not hold %q", c.raw, c.secret)
		}
	}
}

// TestHoldConfigFileQuotedURLWithSpace: a config file whose URL value carries
// a space inside a credential-named value holds the whole value, from the raw
// text even when the key is not a *_url key and TOML rejects the file.
func TestHoldConfigFileQuotedURLWithSpace(t *testing.T) {
	if inFreshProcess(t) {
		return
	}
	HoldConfigFile([]byte("endpoint = \"http://127.0.0.1:1/?api_key=prefix OrdCfgSpace0123\"\nbroken = = 1\n"))
	if !heldNow("prefix OrdCfgSpace0123") {
		t.Error("the whole credential value of a quoted URL with a space was not held")
	}
}
