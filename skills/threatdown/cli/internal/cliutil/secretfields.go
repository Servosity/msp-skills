// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import (
	"bytes"
	"encoding/json"
	"strings"
)

// hand-fix (handfixes.json secret-fields-never-persisted): OneView GET
// responses carry credentials inside otherwise ordinary rows: every policy
// from GET /v1/policies holds as_uninstall_password (the tamper-protection
// password that uninstalls the agent), the policy/site secret endpoints
// return secret, v2 policies carry secret_hash, and notification and webhook
// subscriptions carry the HMAC signing key secret_token plus custom_headers,
// whose values are outbound auth headers (the spec's example key is
// "authorization"), so the whole array is dropped. The local store
// drops these keys and the response cache refuses bodies that contain them.
var secretFieldKeys = map[string]bool{
	"as_uninstall_password": true,
	"uninstall_password":    true,
	"password":              true,
	"secret":                true,
	"secret_hash":           true,
	"secret_token":          true,
	"token":                 true,
	"custom_headers":        true,
}

// IsSecretBearingPath reports whether a GET path returns credentials in
// fields whose names are not secret-shaped: the installers response carries
// account-specific download links and signed URLs that can enroll an agent.
func IsSecretBearingPath(path string) bool {
	return strings.HasSuffix(strings.TrimRight(path, "/"), "/installers")
}

// IsSecretFieldKey reports whether a JSON key names a credential value.
func IsSecretFieldKey(key string) bool { return secretFieldKeys[key] }

// RedactSecretFields returns raw with every secret-named key removed at any
// depth, and whether anything was removed. Unchanged input is returned as-is;
// numbers keep their exact text (UseNumber). Invalid JSON is returned as-is.
func RedactSecretFields(raw json.RawMessage) (json.RawMessage, bool) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return raw, false
	}
	if !dropSecretKeys(v) {
		return raw, false
	}
	out, err := json.Marshal(v)
	if err != nil {
		return raw, false
	}
	return out, true
}

// ContainsSecretField reports whether raw holds a secret-named key at any depth.
func ContainsSecretField(raw json.RawMessage) bool {
	_, removed := RedactSecretFields(raw)
	return removed
}

func dropSecretKeys(v any) bool {
	removed := false
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			if secretFieldKeys[k] {
				delete(t, k)
				removed = true
				continue
			}
			if dropSecretKeys(child) {
				removed = true
			}
		}
	case []any:
		for _, child := range t {
			if dropSecretKeys(child) {
				removed = true
			}
		}
	}
	return removed
}
