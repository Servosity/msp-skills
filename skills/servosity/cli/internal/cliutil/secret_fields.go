// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/servosity/handfixes.json under
// the "secret-family-never-persisted" entry. See docs/reprint-survival.md.

package cliutil

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
)

// secretFieldExact and secretFieldSuffixes classify a JSON key as a credential
// after normalizing it: lower case, with '_', '-', '.' and spaces removed, so
// password, lan_password, LanPassword, EncryptionPassword and RESTIC_PASSWORD
// all match. The exact names cover the API's bare secret fields (the
// encryption-key reads' key, MFA code, backup codes); the suffixes cover every
// named variant (postmark_server_api_token, agent_provision_token_id,
// product_key, AWS_SECRET_ACCESS_KEY).
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

// MayCarrySecretField is a cheap prefilter over raw bytes: false means no key
// in data can be a secret field name.
func MayCarrySecretField(data []byte) bool {
	// An escape (\u006b, \/) can spell a key the byte check cannot see, so it
	// sends the text to the parser.
	if bytes.Contains(data, []byte(`\`)) {
		return true
	}
	lower := bytes.ToLower(data)
	for _, frag := range [][]byte{[]byte("pass"), []byte("secret"), []byte("token"), []byte("key"), []byte("code"), []byte("otp")} {
		if bytes.Contains(lower, frag) {
			return true
		}
	}
	return false
}

// SecretValuePresent reports whether v (decoded JSON) holds a non-empty value
// under a secret field name at any depth.
func SecretValuePresent(v any) bool {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			if IsSecretFieldName(k) && !emptyJSONValue(child) {
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

// SecretWithheld replaces a secret field's value in MCP output.
const SecretWithheld = "[withheld]"

// WithholdSecretValues replaces, in place, every non-empty string or number
// under a secret field name (and every such string inside an array under one)
// with SecretWithheld. Objects under a secret name are walked, not replaced,
// so a parameter schema named "token" stays readable. It reports whether
// anything changed.
func WithholdSecretValues(v any) bool {
	changed := false
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			if IsSecretFieldName(k) && withholdsValue(k, child) {
				if w, ok := withholdScalar(child); ok {
					t[k] = w
					changed = true
					continue
				}
			}
			if str, ok := child.(string); ok {
				if inner, ok := withholdNestedJSON(str); ok {
					t[k] = inner
					changed = true
				}
				continue
			}
			if WithholdSecretValues(child) {
				changed = true
			}
		}
	case []any:
		for i, child := range t {
			if str, ok := child.(string); ok {
				if inner, ok := withholdNestedJSON(str); ok {
					t[i] = inner
					changed = true
				}
				continue
			}
			if WithholdSecretValues(child) {
				changed = true
			}
		}
	}
	return changed
}

// withholdNestedJSON withholds secret fields inside a string value that is
// itself JSON (an echoed request or response body) or holds key/value pairs.
func withholdNestedJSON(s string) (string, bool) {
	if !MayCarrySecretField([]byte(s)) {
		return "", false
	}
	if out := WithholdSecretFieldsText(s); out != s {
		return out, true
	}
	return "", false
}

// secretPairRE matches a JSON-style "key": value pair whose value is a string,
// a number or an array of strings. It is the fallback for text that does not
// parse as JSON: a document cut at a size limit, or JSON embedded in prose.
var secretPairRE = regexp.MustCompile(`("(?:[^"\\]|\\.)*")(\s*:\s*)("(?:[^"\\]|\\.)*"|-?[0-9][0-9.eE+-]*|\[\s*"(?:[^"\\]|\\.)*"(?:\s*,\s*"(?:[^"\\]|\\.)*")*\s*\])`)

// secretTailRE matches a pair whose string value is cut off: the opening quote
// with no closing quote before the end of the text.
var secretTailRE = regexp.MustCompile(`("(?:[^"\\]|\\.)*")(\s*:\s*)"(?:[^"\\]|\\.)*\\?$`)

func withholdSecretPairs(text string) string {
	text = secretPairRE.ReplaceAllStringFunc(text, func(m string) string {
		sub := secretPairRE.FindStringSubmatch(m)
		var key string
		if json.Unmarshal([]byte(sub[1]), &key) != nil || !IsSecretFieldName(key) || sub[3] == `""` || sub[3] == `"`+SecretWithheld+`"` {
			return m
		}
		var value any
		if json.Unmarshal([]byte(sub[3]), &value) == nil && !withholdsValue(key, value) {
			return m
		}
		return sub[1] + sub[2] + `"` + SecretWithheld + `"`
	})
	if m := secretTailRE.FindStringSubmatchIndex(text); m != nil {
		var key string
		if json.Unmarshal([]byte(text[m[2]:m[3]]), &key) == nil && IsSecretFieldName(key) {
			text = text[:m[5]] + `"` + SecretWithheld + `"`
		}
	}
	return text
}

// withholdsValue reports whether a secret-named field's value is withheld. A
// field named exactly "code" holds an MFA code only when its value has a
// digit; a word such as an API error code (token_not_valid) is kept, so an
// agent still sees why a call failed.
func withholdsValue(key string, value any) bool {
	if normalizeFieldName(key) != "code" {
		return true
	}
	str, ok := value.(string)
	if !ok {
		return true
	}
	return strings.IndexFunc(str, func(r rune) bool { return r >= '0' && r <= '9' }) >= 0
}

func withholdScalar(v any) (any, bool) {
	switch t := v.(type) {
	case string:
		if t == "" || t == SecretWithheld {
			return nil, false
		}
		return SecretWithheld, true
	case json.Number:
		return SecretWithheld, true
	case float64:
		return SecretWithheld, true
	case []any:
		allScalar := len(t) > 0
		for _, e := range t {
			switch e.(type) {
			case string, json.Number, float64:
			default:
				allScalar = false
			}
		}
		if allScalar {
			return SecretWithheld, true
		}
	}
	return nil, false
}

// WithholdSecretFieldsText applies WithholdSecretValues to text that is one
// JSON document or newline-delimited JSON. Other text is returned unchanged.
func WithholdSecretFieldsText(text string) string {
	if !MayCarrySecretField([]byte(text)) {
		return text
	}
	if out, ok := withholdJSONDocument(text); ok {
		return out
	}
	lines := strings.Split(text, "\n")
	changed := false
	for i, line := range lines {
		out, ok := withholdJSONDocument(line)
		if !ok {
			out = withholdSecretPairs(line)
		}
		if out != line {
			lines[i] = out
			changed = true
		}
	}
	if !changed {
		return text
	}
	return strings.Join(lines, "\n")
}

func withholdJSONDocument(text string) (string, bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || (trimmed[0] != '{' && trimmed[0] != '[') || !json.Valid([]byte(trimmed)) {
		return "", false
	}
	dec := json.NewDecoder(strings.NewReader(trimmed))
	dec.UseNumber()
	var v any
	if dec.Decode(&v) != nil {
		return "", false
	}
	if !WithholdSecretValues(v) {
		return text, true
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if strings.Contains(trimmed, "\n") {
		enc.SetIndent("", "  ")
	}
	if enc.Encode(v) != nil {
		return "", false
	}
	out := strings.TrimRight(buf.String(), "\n")
	if strings.HasSuffix(text, "\n") {
		out += "\n"
	}
	return out, true
}
