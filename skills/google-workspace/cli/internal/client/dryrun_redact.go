// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (handfixes.json dry-run-redacts-secrets).

package client

import (
	"bytes"
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
)

// redactDryRunBody parses a request body for the dry-run preview and replaces
// every non-empty, non-boolean value of a secret-named field (password,
// secret, private key, API key, *token but not page/sync tokens) with
// "[redacted]".
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
			if secretFieldName(k) && child != nil && child != "" {
				if _, isBool := child.(bool); !isBool { // booleans such as changePasswordAtNextLogin stay visible
					t[k] = "[redacted]"
					continue
				}
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

var queryParamPattern = regexp.MustCompile(`([?&])([A-Za-z0-9_.%-]+)=([^&\s"'#]*)`)

// redactSecretQueryParams masks the value of every secret-named query
// parameter in text (URLs inside error messages, dry-run lines, bodies).
func redactSecretQueryParams(text string) string {
	if !strings.Contains(text, "=") {
		return text
	}
	return queryParamPattern.ReplaceAllStringFunc(text, func(m string) string {
		parts := queryParamPattern.FindStringSubmatch(m)
		name, err := url.QueryUnescape(parts[2])
		if err != nil || parts[3] == "" || !secretFieldName(name) {
			return m
		}
		return parts[1] + parts[2] + "=[redacted]"
	})
}
