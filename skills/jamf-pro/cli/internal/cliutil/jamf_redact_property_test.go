// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

// TestRedactionProperty generates every container shape times every secret
// class and asserts the secret never reaches the output in any form, and
// that benign values in the same shapes survive (a funnel that withheld
// everything would otherwise pass).
func TestRedactionProperty(t *testing.T) {
	const marker = "Zq7Sekrit9"
	bs := string(rune(92)) // a backslash, built here so no tool or editor decodes the escape
	b64 := base64.StdEncoding.EncodeToString
	classes := []struct {
		name, value string
		keyOnly     bool // a bare secret: only its key path can reveal it
	}{
		{"bare-under-secret-key", marker, true},
		{"kv-equals", "password=" + marker, false},
		{"kv-colon", "client_secret: " + marker, false},
		{"kv-in-text", "rotate it: token=" + marker + " tomorrow", false},
		{"json-kv", `{"apiKey":"` + marker + `"}`, false},
		{"bearer", "Bearer " + marker + "abcdefgh", false},
		{"basic", "Basic " + b64([]byte("svc:"+marker)), false},
		{"url-userinfo", "https://svc:" + marker + "@host.example/path", false},
		{"url-user-only", "https://" + marker + "@host.example/", false},
		{"url-query", "https://host.example/hook?sig=" + marker, false},
		{"url-fragment", "https://host.example/cb#access_token=" + marker, false},
		{"url-in-text", "repo at ftp://u:" + marker + "@h.example/x, ask IT", false},
		{"url-encoded", url.QueryEscape("https://svc:" + marker + "@host.example/p"), false},
		{"double-url-encoded", url.QueryEscape(url.QueryEscape("password=" + marker)), false},
		{"html-escaped", "password&#61;" + marker, false},
		{"json-escaped", "password" + bs + "u003d" + marker, false},
		{"base64-kv", b64([]byte("password=" + marker)), false},
		{"base64url-userinfo", base64.URLEncoding.EncodeToString([]byte("https://u:" + marker + "@h.example/x")), false},
		{"base64-of-url-encoded", b64([]byte(url.QueryEscape("secret=" + marker))), false},
		{"pem", "-----BEGIN RSA PRIVATE " + "KEY-----\n" + marker + "\n-----END RSA PRIVATE KEY-----", false},
		{"jwt", "eyJhbGciOiJIUzI1NiJ9." + "eyJzdWIiOiIxMjM0In0." + marker, false},
		{"embedded-json-object", `{"note":"x","apiToken":"` + marker + `"}`, false},
		{"embedded-json-array", `[{"label":"Service Password","value":"` + marker + `"}]`, false},
		{"escaped-json-object", "{" + bs + `"password` + bs + `":` + bs + `"` + marker + bs + `"}`, false},
		{"escaped-slash-url", "https:" + bs + "/" + bs + "/svc:" + marker + "@h.example" + bs + "/p", false},
		{"unpadded-base64-kv", base64.RawStdEncoding.EncodeToString([]byte("password=" + marker)), false},
		{"base64url-kv", base64.URLEncoding.EncodeToString([]byte("client_secret=" + marker + "??>>")), false},
		{"unpadded-base64url-userinfo", base64.RawURLEncoding.EncodeToString([]byte("https://u:" + marker + "@h.example/x")), false},
		{"malformed-percent", "token%3D" + marker + "%", false},
		{"malformed-json-escape", "secret" + bs + "q=" + marker, false},
		// All letters, unpadded: decodes to "Bearer aaaaaaaaa" (no digits or symbols to spot).
		{"all-letter-unpadded-base64", base64.RawStdEncoding.EncodeToString([]byte("Bearer aaaaaaaaa")), false},
		{"pin-kv", "device pin: " + marker, false},
		{"json-fragment-in-text", `Result: {"name":"API Token","value":"` + marker + `"} (from the import)`, false},
		{"json-array-fragment-in-text", `note [ {"label":"Admin Password","value":"` + marker + `"} ] end`, false},
		{"json-fragment-secret-key", `see {"apiPassword":"` + marker + `"} above`, false},
		{"json-fragment-repeated-key", `Result: {"name":"API Token","value":"` + marker + `","value":"[redacted]"} done`, false},
		{"json-document-repeated-key", `{"note":"` + marker + `","note":"x"}`, false},
		{"passphrase-kv", "wifi passphrase=" + marker, false},
	}
	shapes := []struct {
		name      string
		secretKey bool // the value sits on a secret key path
		build     func(v any) any
	}{
		{"top-scalar", false, func(v any) any { return v }},
		{"field", false, func(v any) any { return map[string]any{"note": v} }},
		{"url-field", false, func(v any) any { return map[string]any{"webhookUrl": v} }},
		{"array-top", false, func(v any) any { return []any{"x", v} }},
		{"array-field", false, func(v any) any { return map[string]any{"notes": []any{"x", v}} }},
		{"nested-array-map", false, func(v any) any {
			return map[string]any{"a": []any{map[string]any{"b": map[string]any{"c": []any{v}}}}}
		}},
		{"array-of-arrays", false, func(v any) any { return []any{[]any{[]any{v}}} }},
		{"deep", false, func(v any) any {
			for i := 0; i < 6; i++ {
				v = map[string]any{"level": []any{v}}
			}
			return v
		}},
		{"secret-field", true, func(v any) any { return map[string]any{"apiPassword": v} }},
		{"secret-subtree", true, func(v any) any { return map[string]any{"credentials": map[string]any{"note": []any{v}}} }},
		{"record-scalar", true, func(v any) any { return map[string]any{"name": "Service Password", "value": v} }},
		{"record-values", true, func(v any) any { return map[string]any{"displayName": "API Token", "values": []any{v}} }},
		{"records-in-array", true, func(v any) any {
			return []any{map[string]any{"name": "Owner", "value": "Ada"}, map[string]any{"label": "Admin Password", "value": v}}
		}},
		{"record-encoded-label", true, func(v any) any { return map[string]any{"name": "Admin%20Password", "value": v} }},
		{"record-double-encoded-label", true, func(v any) any {
			return map[string]any{"name": "%2570%2561%2573%2573%2577%256f%2572%2564", "value": v}
		}},
		{"record-passcode", true, func(v any) any { return map[string]any{"name": "Passcode", "value": v} }},
		{"record-passphrase", true, func(v any) any { return map[string]any{"name": "Passphrase", "value": v} }},
		{"record-device-pin", true, func(v any) any { return map[string]any{"label": "Device PIN", "values": []any{v}} }},
		{"field-pwd", true, func(v any) any { return map[string]any{"pwd": v} }},
		{"field-lockPin", true, func(v any) any { return map[string]any{"lockPin": v} }},
		{"field-recovery_pin", true, func(v any) any { return map[string]any{"recovery_pin": v} }},
		{"field-PINCode", true, func(v any) any { return map[string]any{"PINCode": v} }},
		// Benign names that contain "pin": the word rule must not withhold them.
		{"field-shipping", false, func(v any) any { return map[string]any{"shippingAddress": v} }},
		{"field-mapping", false, func(v any) any { return map[string]any{"mapping": v} }},
	}
	cases := 0
	for _, c := range classes {
		for _, sh := range shapes {
			if c.keyOnly && !sh.secretKey {
				continue
			}
			cases++
			in, _ := json.Marshal(sh.build(c.value))
			out, err := RedactJSON(in)
			if err != nil {
				t.Fatalf("%s/%s: %v", c.name, sh.name, err)
			}
			esc, _ := json.Marshal(c.value)
			if strings.Contains(string(out), marker) || strings.Contains(string(out), string(esc[1:len(esc)-1])) {
				t.Errorf("%s in %s leaked: %s", c.name, sh.name, out)
			}
		}
	}
	if cases < 500 {
		t.Fatalf("only %d cases generated", cases)
	}
	// Benign values in non-secret shapes must survive unchanged.
	for _, benign := range []string{"Ada's MacBook Pro", "https://acme.jamfcloud.com", "14.4.1", "Token Expiration Notice", "C02XK1JHJG5H",
		"Finance Laptops", "Battery 85%", "[Archive] Old Policies", "MacBookPro18,1", "InformationTechnologyDepartment",
		"3f2a9c0b7d4e1f6a8b2c5d9e0f1a2b3c4d5e6f708192a3b4c5d6e7f8091a2b3c", "com.apple.Safari", "Ada's iPad (2)",
		"Use {server} in templates", `config {"site":"HQ","floor":3} applied`} {
		for _, sh := range shapes {
			if sh.secretKey || sh.name == "url-field" {
				continue
			}
			in, _ := json.Marshal(sh.build(benign))
			out, _ := RedactJSON(in)
			esc, _ := json.Marshal(benign)
			if !strings.Contains(string(out), string(esc[1:len(esc)-1])) {
				t.Errorf("benign %q in %s was withheld: %s", benign, sh.name, out)
			}
		}
	}
}

// TestCredentialNameWords: one word list decides names and labels; short
// words match only as whole words.
func TestCredentialNameWords(t *testing.T) {
	for _, n := range []string{"password", "adminPwd", "Passcode", "lockPin", "PINCode", "recovery_pin", "pinNumber", "clientSecret", "apiKey", "tokenValue", "Credential"} {
		if !credentialName(n) {
			t.Errorf("%s must name a credential", n)
		}
	}
	for _, n := range []string{"shippingAddress", "mapping", "spinner", "typing", "Department", "serialNumber"} {
		if credentialName(n) {
			t.Errorf("%s must not name a credential", n)
		}
	}
}
