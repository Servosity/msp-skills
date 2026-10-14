// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (handfixes.json dry-run-redacts-secrets).

package client

import (
	"bytes"
	"encoding/json"
	"strings"
)

// redactDryRunBody parses a request body for the dry-run preview and replaces
// the value of every secret-named field (password, secret, private key, API
// key, *token) with "[redacted]". ok is false when the body is not JSON.
func redactDryRunBody(body []byte) (any, bool) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var v any
	if dec.Decode(&v) != nil {
		return nil, false
	}
	return redactSecretFields(v), true
}

func redactSecretFields(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			if secretFieldName(k) {
				if child != nil && child != "" {
					t[k] = "[redacted]"
				}
				continue
			}
			t[k] = redactSecretFields(child)
		}
	case []any:
		for i := range t {
			t[i] = redactSecretFields(t[i])
		}
	}
	return v
}

func secretFieldName(k string) bool {
	k = strings.ToLower(k)
	for _, s := range []string{"password", "secret", "privatekey", "private_key", "apikey", "api_key"} {
		if strings.Contains(k, s) {
			return true
		}
	}
	return strings.HasSuffix(k, "token")
}
