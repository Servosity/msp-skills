// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: guards handfixes.json error-body-redacts-secrets (ported
// from connectsecure).

package cliutil

import (
	"strings"
	"testing"
	"unicode/utf16"
)

// TestEchoDetectedInEveryEncoderShape: real servers echo with their own JSON
// library's escapes, and the CLI's display path decodes them back to plaintext.
// Every case's text holds the secret only in encoded form.
func TestEchoDetectedInEveryEncoderShape(t *testing.T) {
	for _, tc := range []struct{ name, secret, text string }{
		{"raw", "Plain_Sentinel_77", "{\"message\":\"password 'Plain_Sentinel_77' rejected\"}"},
		{"go html escapes", "Pa&ss<w0rd>9", "body was {\"password\":\"Pa\\u0026ss\\u003cw0rd\\u003e9\"}"},
		{"python ensure_ascii", "Café_Sentinel_1", "{\"message\": \"password 'Caf\\u00e9_Sentinel_1' rejected\"}"},
		{"python surrogate pair", "Emoji🔑Sentinel", "{\"message\": \"Emoji\\ud83d\\udd11Sentinel\"}"},
		{"nested surrogate pair", "Emoji🔑Sentinel", "{\"echo\": \"{\\\"password\\\": \\\"Emoji\\\\ud83d\\\\udd11Sentinel\\\"}\"}"},
		{"php slash", "Pa/ss_Sentinel_2", "{\"message\":\"password 'Pa\\/ss_Sentinel_2' rejected\"}"},
		{".NET uppercase hex", "Pa+ss_O'B<9>", "{\"message\":\"\\u0027Pa\\u002Bss_O\\u0027B\\u003C9\\u003E\\u0027\"}"},
		{"gson", "Base64ish==Sentinel", "{\"message\":\"Base64ish\\u003d\\u003dSentinel\"}"},
		{"json backspace and formfeed", "Pa\bss\f_SENT", "{\"message\":\"Pa\\bss\\f_SENT\"}"},
		{"python repr", "O'Bri\"en_SENT", "{'message': 'O\\'Bri\"en_SENT'}"},
		{"python bytes repr", "Café_Sentinel_1", "body was b'{\"password\":\"Caf\\xc3\\xa9_Sentinel_1\"}'"},
		{"body inside a JSON string", "Tr0ub\"leQ7x", "{\"echo\":\"{\\\"password\\\":\\\"Tr0ub\\\\\\\"leQ7x\\\"}\"}"},
		{"python body inside a JSON string", "Café_SENT_LAT", "{\"echo\": \"{\\\"password\\\": \\\"Caf\\\\u00e9_SENT_LAT\\\"}\"}"},
		{"html entities", "Pa&ss<w0rd>'Q_SENT", "<pre>{&quot;password&quot;:&quot;Pa&amp;ss&lt;w0rd&gt;&#x27;Q_SENT&quot;}</pre>"},
		{"url encoded", "Sec+ret/Sentinel?~>9", "bad value Sec%2Bret%2FSentinel%3F~%3E9"},
		{"utf-16 nul bytes", "Plain_Sentinel_77", "b\u0000o\u0000d\u0000y\u0000 \u0000P\u0000l\u0000a\u0000i\u0000n\u0000_\u0000S\u0000e\u0000n\u0000t\u0000i\u0000n\u0000e\u0000l\u0000_\u00007\u00007\u0000"},
		{"long value cut to head and tail", "test-secret-headXXXXXXXXXXXXXXXXXXXXtest-secret-tail", "input_value='test-secret-headXXXX...XXXXtest-secret-tail'"},
		{"pem key", "unrelated", "-----BEGIN PRIVATE KEY-----\nabc"},
		{"latin-1 bytes", "Café_SENT_LAT", "body was {\"password\":\"Caf\xe9_SENT_LAT\"}"},
	} {
		if tc.name != "raw" && tc.name != "pem key" && strings.Contains(tc.text, tc.secret) {
			t.Fatalf("%s: test text holds the secret in plaintext; the case proves nothing", tc.name)
		}
		if !EchoesSecret(tc.text, []string{tc.secret}) {
			t.Errorf("%s: echo of %q not detected in %q", tc.name, tc.secret, tc.text)
		}
	}
	if !EchoesSecret("\x62\x00\x6f\x00\x64\x00\x79\x00\x20\x00\x43\x00\x61\x00\x66\x00\xe9\x00\x5f\x00\x53\x00\x65\x00\x6e\x00\x74\x00\x69\x00\x6e\x00\x65\x00\x6c\x00\x5f\x00\x31\x00", []string{"Café_Sentinel_1"}) {
		t.Error("utf-16 echo of a non-ASCII secret not detected")
	}
	// A JWT's first 16 characters are its constant header: another JWT in the
	// body is not an echo of ours.
	if EchoesSecret("use refresh token eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiI5OSJ9.otherSigOtherSig", []string{"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiI0MiJ9.SIGSENTINELxyz0123456789"}) {
		t.Error("an unrelated JWT was treated as an echo of the CLI token")
	}
	if EchoesSecret(`{"message":"name already exists","code":1022}`, []string{"Plain_Sentinel_77", "root_password_x"}) {
		t.Error("a body without the secret must not be withheld")
	}
}

// TestEchoDetectedInUTF16 guards the UTF-16 views: a non-ASCII secret echoed
// in UTF-16 (either byte order, either alignment) is seen whole.
func TestEchoDetectedInUTF16(t *testing.T) {
	enc := func(s string, be bool) string {
		var b []byte
		for _, u := range utf16.Encode([]rune(s)) {
			if be {
				b = append(b, byte(u>>8), byte(u))
			} else {
				b = append(b, byte(u), byte(u>>8))
			}
		}
		return string(b)
	}
	const secret = "PassĀword9"
	for _, tc := range []struct {
		name string
		text string
	}{
		{"le", enc(`{"error":"bad `+secret+`"}`, false)},
		{"be", enc(`{"error":"bad `+secret+`"}`, true)},
		{"le odd offset", "x" + enc(`bad `+secret, false)},
	} {
		if strings.Contains(strings.ReplaceAll(tc.text, "\x00", ""), "word9") && strings.Contains(strings.ReplaceAll(tc.text, "\x00", ""), secret) {
			t.Fatalf("%s: NUL-stripped text already holds the secret; the case proves nothing", tc.name)
		}
		if !EchoesSecret(tc.text, []string{secret}) {
			t.Errorf("%s: UTF-16 echo of %q not detected", tc.name, secret)
		}
		if EchoesSecret(tc.text, []string{"OtherĀSecret7"}) {
			t.Errorf("%s: false positive on an unrelated secret", tc.name)
		}
	}
}
