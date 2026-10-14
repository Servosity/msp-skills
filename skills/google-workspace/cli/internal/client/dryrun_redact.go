// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (handfixes.json dry-run-redacts-secrets).

package client

import (
	"bytes"
	"encoding/json"
	"strings"
)

// redactDryRunBody parses a request body for the dry-run preview and replaces
// every non-empty string value of a secret-named field (password, secret,
// private key, API key, *token but not page/sync tokens) with "[redacted]".
// ok is false when the body is not JSON.
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
			if s, isString := child.(string); isString && s != "" && secretFieldName(k) {
				t[k] = "[redacted]" // booleans such as changePasswordAtNextLogin stay visible
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
	// Paging cursors are not credentials.
	return strings.HasSuffix(k, "token") && !strings.HasSuffix(k, "pagetoken") && !strings.HasSuffix(k, "synctoken")
}
