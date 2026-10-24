// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/acronis/handfixes.json under
// the "credential-fields-never-persisted" entry. See docs/reprint-survival.md.
// Ported from skills/servosity/cli/internal/cliutil/secret_fields.go (the
// fleet precedent); the name rules are the same.

package cliutil

import (
	"bytes"
	"encoding/json"
	"strings"
)

// secretFieldExact and secretFieldSuffixes classify a JSON key as a credential
// after normalizing it: lower case, with '_', '-', '.' and spaces removed, so
// client_secret, clientSecret, access_token, refresh_token, id_token and
// RESTIC_PASSWORD all match. The exact names cover bare secret fields (a key,
// an MFA code, backup codes); the suffixes cover every named variant
// (api_key, agent_provision_token_id, AWS_SECRET_ACCESS_KEY). An identifier
// such as client_id, tenant_id or token_type is not a credential.
var secretFieldExact = map[string]bool{
	"key":      true,
	"code":     true,
	"codes":    true,
	"mfacodes": true,
	"otp":      true,
	"totp":     true,
}

var secretFieldSuffixes = []string{
	"password", "passwd", "passphrase", "passcode", "secret", "secretkey", "secretaccesskey",
	"token", "tokenid", "apikey", "productkey", "privatekey",
	// Plural and credential names (passwords, secrets, tokens, api_keys,
	// apikeys, private_keys, credential, credentials): a list or map of
	// credentials is a credential too.
	"passwords", "secrets", "tokens", "apikeys", "privatekeys", "credential", "credentials",
}

// IsSecretFieldName reports whether a JSON key names a credential.
func IsSecretFieldName(name string) bool {
	n := normalizeFieldName(name)
	if n == "" {
		return false
	}
	if secretFieldExact[n] {
		return true
	}
	for _, s := range secretFieldSuffixes {
		if strings.HasSuffix(n, s) {
			return true
		}
	}
	return false
}

func normalizeFieldName(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		switch r {
		case '_', '-', '.', ' ':
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// IsSecretField reports whether a JSON field holds a credential that must stay
// off disk. A field whose name reads as one (IsSecretFieldName) is a credential
// whatever its value, with two exceptions, each bound to a name that says what
// the value is:
//   - a field named exactly "code" whose value is one of the result words
//     Acronis documents for a task (Task Manager API, result_code enum); the
//     task_manager.result_code column reads result.code. Any other code (an
//     MFA or one-time code, a word or number outside the list) is withheld.
//   - a number under a count name: a qualifier (total, max, ...) directly
//     before a plural credential word (total_tokens, max_api_keys). A number
//     under a bare or other plural name (api_keys, access_tokens) is withheld.
func IsSecretField(name string, value any) bool {
	if !IsSecretFieldName(name) {
		return false
	}
	n := normalizeFieldName(name)
	if n == "code" {
		str, ok := value.(string)
		return !ok || !resultCodeWords[str]
	}
	return !(isJSONNumber(value) && countFieldName(n))
}

var resultCodeWords = map[string]bool{
	"ok": true, "error": true, "warning": true, "cancelled": true, "abandoned": true, "timedout": true,
}

// countFieldName reports whether a normalized name is a count qualifier
// followed directly by a plural credential word, and nothing else.
func countFieldName(n string) bool {
	for _, q := range []string{"total", "input", "output", "prompt", "completion", "max", "min", "num"} {
		rest, ok := strings.CutPrefix(n, q)
		if !ok {
			continue
		}
		for _, s := range []string{"passwords", "secrets", "tokens", "apikeys", "privatekeys", "credentials"} {
			if rest == s {
				return true
			}
		}
	}
	return false
}

func isJSONNumber(v any) bool {
	switch v.(type) {
	case json.Number, float64, float32, int, int64, int32, uint, uint64, uint32:
		return true
	}
	return false
}

// MayCarrySecretField is a cheap prefilter over raw bytes: false means no key
// in data can be a secret field name.
func MayCarrySecretField(data []byte) bool {
	// An escape (k, \/) can spell a key the byte check cannot see, so it
	// sends the text to the parser.
	if bytes.Contains(data, []byte(`\`)) {
		return true
	}
	lower := bytes.ToLower(data)
	// A key is matched after its separators are removed (to_ken reads as
	// token), so the fragments are looked for in the text without them too.
	squeezed := bytes.Map(func(r rune) rune {
		switch r {
		case '_', '-', '.', ' ':
			return -1
		}
		return r
	}, lower)
	for _, frag := range [][]byte{[]byte("pass"), []byte("secret"), []byte("token"), []byte("key"), []byte("code"), []byte("otp"), []byte("credential")} {
		if bytes.Contains(lower, frag) || bytes.Contains(squeezed, frag) {
			return true
		}
	}
	return false
}

// SecretValuePresent reports whether v (decoded JSON) holds a non-empty value
// under a secret field (IsSecretField) at any depth.
func SecretValuePresent(v any) bool {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			if IsSecretField(k, child) && !emptyJSONValue(child) {
				return true
			}
			if SecretValuePresent(child) {
				return true
			}
		}
	case []any:
		for _, child := range t {
			if SecretValuePresent(child) {
				return true
			}
		}
	}
	return false
}

func emptyJSONValue(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return t == ""
	case bool:
		return !t
	case []any:
		return len(t) == 0
	case map[string]any:
		return len(t) == 0
	}
	return false
}

// JSONCarriesSecret reports whether a response body must stay off disk: it is
// not one JSON document (a body that cannot be parsed has no field names to
// judge), or it holds a non-empty secret field at any depth. An empty body
// carries nothing.
func JSONCarriesSecret(data []byte) bool {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return false
	}
	if !json.Valid(trimmed) {
		return true
	}
	if !MayCarrySecretField(trimmed) {
		return false
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.UseNumber()
	var v any
	if dec.Decode(&v) != nil {
		return true
	}
	return SecretValuePresent(v)
}
