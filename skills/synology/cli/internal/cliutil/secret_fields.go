// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/synology/handfixes.json
// under the "secret-fields-never-persisted" entry.

package cliutil

import (
	"bytes"
	"encoding/json"
	"strings"
)

// RedactedValue replaces a credential-shaped value in every copy of a DSM
// response this CLI writes to disk.
const RedactedValue = "[redacted]"

var secretFieldMarkers = []string{
	"password", "passwd", "passphrase", "secret", "token", "credential",
	"privatekey", "apikey", "accesskey", "authkey", "sessionkey", "cookie",
}

var secretFieldExact = map[string]bool{
	"pwd": true, "pass": true, "psk": true, "sid": true, "did": true, "otp": true, "otpcode": true,
}

// SecretFieldName reports whether a JSON key names credential material
// (password, passwd, secret, token, private key, API key, a DSM sid or
// trusted-device did, ...). Matching ignores case and '_', '-', '.' and spaces.
func SecretFieldName(key string) bool {
	k := strings.ToLower(key)
	k = strings.NewReplacer("_", "", "-", "", ".", "", " ", "").Replace(k)
	if secretFieldExact[k] {
		return true
	}
	for _, m := range secretFieldMarkers {
		if strings.Contains(k, m) {
			return true
		}
	}
	return false
}

// RedactSecretFields returns data with the value of every secret-named key
// replaced by RedactedValue, at any depth. A string value, or an object or
// array held under such a key, is replaced whole; booleans, numbers and null
// carry no secret and are kept (passwd_never_expire, password_last_change).
// Input that is not JSON, or that carries no secret-named key, is returned
// unchanged byte for byte.
//
// Every DSM response this CLI persists (the local store that sync and live
// reads write through to, which MCP sql reads even in read-only mode, and the
// HTTP response cache) passes through here first. The spec declares no
// response schemas, so which fields a NAS returns cannot be enumerated; the
// rule is by name, at the one boundary where data reaches disk.
func RedactSecretFields(data []byte) []byte {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || (trimmed[0] != '{' && trimmed[0] != '[') {
		return data
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return data
	}
	if !redactSecretValues(v) {
		return data
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return data
	}
	return bytes.TrimRight(buf.Bytes(), "\n")
}

func redactSecretValues(v any) bool {
	changed := false
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			if SecretFieldName(k) {
				switch child.(type) {
				case string, map[string]any, []any:
					if s, ok := child.(string); ok && s == "" {
						continue
					}
					t[k] = RedactedValue
					changed = true
					continue
				}
			}
			if redactSecretValues(child) {
				changed = true
			}
		}
	case []any:
		for _, child := range t {
			if redactSecretValues(child) {
				changed = true
			}
		}
	}
	return changed
}
