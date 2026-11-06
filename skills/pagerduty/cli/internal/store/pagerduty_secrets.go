// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

// pagerduty handfix (store-never-keeps-secrets): every store write passes
// mergeIncomingResourceData (sync batches, typed upserts, the write-through
// cache and the keep-richer merge), which returns withoutStoredSecrets of the
// record it is about to write. A value under one of PagerDuty's credential
// field names (storedSecretKeys) is replaced by "[redacted]" at any depth,
// whether or not anything held it, so a resource row stays a row. A record
// that cannot be decoded is stored as {}. A held value anywhere else in a
// statement is the credential kit's SQL driver's to refuse
// (credkit-sqlite-every-handle).

import (
	"bytes"
	"encoding/json"
	"strings"
	"unicode"
)

// storedSecretKeys are PagerDuty's credential field names, normalized to
// lowercase letters and digits: the names the earlier layer's credential rule
// matched among the spec's property names (testdata/pagerduty-spec-names.json),
// webhook custom headers, and the Events API integration key. "Authentication
// Headers" is not one: in a response it keys a workflow integration's
// secrets_schema, a definition that is stored as written (its values go out
// under secrets).
var storedSecretKeys = map[string]bool{
	"authorization":  true,
	"clientid":       true,
	"clientsecret":   true,
	"customheaders":  true,
	"integrationkey": true,
	"routingkey":     true,
	"routingkeys":    true,
	"runbookapikey":  true,
	"secret":         true,
	"secrets":        true,
	"servicekey":     true,
}

const storedSecretMarker = "[redacted]"

func isStoredSecretKey(key string) bool {
	var b strings.Builder
	for _, r := range strings.ToLower(key) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return storedSecretKeys[b.String()]
}

func withoutStoredSecrets(data json.RawMessage) json.RawMessage {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return json.RawMessage("{}")
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(redactStoredSecrets(v)); err != nil {
		return json.RawMessage("{}")
	}
	return json.RawMessage(bytes.TrimRight(buf.Bytes(), "\n"))
}

func redactStoredSecrets(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			if isStoredSecretKey(k) {
				if child != nil {
					t[k] = storedSecretMarker
				}
				continue
			}
			t[k] = redactStoredSecrets(child)
		}
	case []any:
		for i, child := range t {
			t[i] = redactStoredSecrets(child)
		}
	}
	return v
}
