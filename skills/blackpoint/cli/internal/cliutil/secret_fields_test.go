// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (secret-field-persistence class).

package cliutil

import (
	"strings"
	"testing"
)

func TestSecretFieldName(t *testing.T) {
	for _, n := range []string{"password", "Password", "client_secret", "skey", "apiKey", "X-Api-Key",
		"accessToken", "refresh_token", "secretKey", "privateKey", "credentials", "passwd", "token", "key"} {
		if !SecretFieldName(n) {
			t.Errorf("%q must name a secret", n)
		}
	}
	for _, n := range []string{"id", "username", "breachName", "ikey", "host", "tokenType", "monkeyCount", "status"} {
		if SecretFieldName(n) {
			t.Errorf("%q must not name a secret", n)
		}
	}
}

func TestRedactSecretFieldsEveryShape(t *testing.T) {
	const s = "Hunter2SentinelPW"
	for name, in := range map[string]string{
		"top":       `{"id":"e1","password":"` + s + `"}`,
		"array":     `[{"id":"e1","Password":"` + s + `"}]`,
		"nested":    `{"data":[{"cisco":{"skey":"` + s + `"}}]}`,
		"under-obj": `{"credentials":{"value":"` + s + `","n":[1,"` + s + `"]}}`,
		"number":    `{"pin":12345678}`,
	} {
		t.Run(name, func(t *testing.T) {
			if !HasSecretFields([]byte(in)) {
				t.Fatalf("HasSecretFields missed %s", in)
			}
			out := string(RedactSecretFields([]byte(in)))
			if strings.Contains(out, s) || strings.Contains(out, "12345678") {
				t.Fatalf("redacted text still holds the secret: %s", out)
			}
			if HasSecretFields([]byte(out)) {
				t.Fatalf("redacted text still reports a secret: %s", out)
			}
		})
	}
	keep := `{"id":"e1","passwordExposed":true,"password":null,"username":"bob"}`
	if HasSecretFields([]byte(keep)) || string(RedactSecretFields([]byte(keep))) != keep {
		t.Fatalf("flags, nulls and plain fields must pass unchanged")
	}
	if HasSecretFields([]byte("password=" + s)) {
		t.Fatalf("non-JSON text is not a field match")
	}
}

// TestSecretDetectorSeesEveryKeyOccurrence: the decoded value can hide a
// secret the raw bytes hold (encoding/json keeps the last duplicate key and
// reads only the first top-level value). The detector walks every key
// occurrence, and redaction never returns the original bytes of such text.
func TestSecretDetectorSeesEveryKeyOccurrence(t *testing.T) {
	const s = "breached-secret-123"
	for name, in := range map[string]string{
		"duplicate-key":        `{"data":[{"id":"e1","password":"` + s + `","password":null}]}`,
		"duplicate-case":       `{"data":[{"id":"e1","Password":"` + s + `","password":null}]}`,
		"duplicate-bool":       `{"password":"` + s + `","password":false}`,
		"duplicate-nested":     `{"credentials":{"value":"` + s + `"},"credentials":null}`,
		"duplicate-in-array":   `[{"x":[{"skey":"` + s + `","skey":"[withheld]"}]}]`,
		"duplicate-number":     `{"pin":"` + s + `","pin":null}`,
		"escaped-key":          `{"password":"` + s + `","password":null}`,
		"second-value":         `{"data":[]} {"data":[{"password":"` + s + `"}]}`,
		"second-value-array":   `[] [{"token":"` + s + `"}]`,
		"trailing-text":        `{"data":[]}garbage"password":"` + s + `"`,
		"trailing-misaligned":  `{"data":[]} "it's "password":"` + s + `"`,
		"invalid-after-prefix": `{"data":[{"password":"` + s + `"},]}`,
		"truncated":            `{"data":[{"password":"` + s + `"}`,
		"bad-escape-in-secret": `{"password":"` + s + `\q"}`,
		"missing-colon":        `{"password" "` + s + `"}`,
		"bom":                  "\xEF\xBB\xBF" + `{"password":"` + s + `"}`,
		"inside-plain-tag":     `[INFO] {"password":"` + s + `"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if !HasSecretFields([]byte(in)) {
				t.Errorf("HasSecretFields missed %s", in)
			}
			out := RedactSecretFields([]byte(in))
			if strings.Contains(string(out), s) {
				t.Errorf("redacted text still holds the secret: %s", out)
			}
			if HasSecretFields(out) {
				t.Errorf("redacted text still reports a secret: %s", out)
			}
		})
	}
	for _, clean := range []string{
		`{"id":"e1","token_hint":"abc","keyboard":"us","monkeyCount":3}`,
		`{"password":null,"password":true,"passwordExposed":false,"secret":""}`,
		`[{"id":1},{"id":2}]`, `"just a string"`, `42`, ``, "  \n",
	} {
		if HasSecretFields([]byte(clean)) || string(RedactSecretFields([]byte(clean))) != clean {
			t.Errorf("clean text must pass unchanged: %q -> %q", clean, RedactSecretFields([]byte(clean)))
		}
	}
	// Plain column text that only starts like JSON is not a field match, so
	// the store driver keeps writing names like these.
	for _, text := range []string{"[TEST] Acme Dental", "[1] Acme", "{draft} notes", "[2024] review: done", "password=" + s} {
		if HasSecretFields([]byte(text)) {
			t.Errorf("plain text %q is not a field match", text)
		}
	}
}

// MayHoldSecretFields is the test for writing original bytes to disk: only
// empty text or one JSON value read to its end with no secret passes. What
// RedactSecretFields returns for anything else always passes, so a sink that
// persists the sanitized form is never refused by the store driver.
func TestMayHoldSecretFieldsFailsClosed(t *testing.T) {
	const s = "breached-secret-123"
	deep := strings.Repeat("[", 20000) + `"x"` + strings.Repeat("]", 20000)
	for _, in := range []string{
		`{"data":[{"id":"e1","password":"` + s + `","password":null}]}`,
		`{"data":[]} {"data":[]}`,
		`{"data":[]}garbage`,
		`{"data":[{"id":"e1"}]`,
		`{"data":[{"id":"e1"},]}`,
		"\xEF\xBB\xBF" + `{"data":[]}`,
		")]}'" + `{"password":"` + s + `"}`,
		"password=" + s,
		"[TEST] Acme Dental",
		`{"password":` + deep + `}`,
		deep,
	} {
		if !MayHoldSecretFields([]byte(in)) {
			t.Errorf("MayHoldSecretFields cleared %.80q", in)
		}
		out := RedactSecretFields([]byte(in))
		if strings.Contains(string(out), s) || MayHoldSecretFields(out) || HasSecretFields(out) {
			t.Errorf("the sanitized form of %.80q is not safe to persist: %.120s", in, out)
		}
	}
	if !HasSecretFields([]byte(`{"password":` + deep + `}`)) {
		t.Error("text nested past the walk limit under a secret key must count as a secret")
	}
	for _, in := range []string{``, " \n\t", `{}`, `[]`, `"plain"`, `42`, `null`, `true`,
		`{"id":"e1","token_hint":"abc","keyboard":"us","password":null,"passwordExposed":true}`,
		`{"credentials":{"value":"[withheld]","n":[]}}`,
	} {
		if MayHoldSecretFields([]byte(in)) {
			t.Errorf("clean text %q must be safe to persist as-is", in)
		}
	}
}

func TestHasSecretFieldsReadsEveryTopLevelValue(t *testing.T) {
	if !HasSecretFields([]byte(`"note" {"password":"breached"}`)) {
		t.Error("a secret in a later top-level value must be seen even after a scalar")
	}
	if HasSecretFields([]byte(`User wrote {"password":"x"} in a ticket`)) {
		t.Error("plain text that only mentions JSON is not a field match")
	}
}
