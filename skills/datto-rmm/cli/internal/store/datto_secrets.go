// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

// datto-rmm handfix (store-never-keeps-secrets): every store write passes
// mergeIncomingResourceData (sync batches, typed upserts, the write-through cache
// and the keep-richer merge), which returns withoutStoredSecrets of the record.
// The fields the connector declares as response credentials (credwire
// Declarations.ResponseFields, TestDeclarationCopiesMatch keeps the
// two the same) are replaced by "[redacted]" at any depth. The credential kit's
// SQL driver refuses a statement carrying a held value, so without this one site
// whose proxy settings carry a password would fail the sync of every site. A
// record that cannot be decoded is stored as {}.

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
)

// SecretResponseFields is Declarations.ResponseFields.
var SecretResponseFields = []string{"access_token", "refresh_token", "password", "apiAccessKey", "apiSecretKey"}

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
	if err := enc.Encode(redactSecretFields(v)); err != nil {
		return json.RawMessage("{}")
	}
	return json.RawMessage(bytes.TrimRight(buf.Bytes(), "\n"))
}

func redactSecretFields(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			if slices.ContainsFunc(SecretResponseFields, func(f string) bool { return strings.EqualFold(f, k) }) {
				t[k] = "[redacted]"
				continue
			}
			t[k] = redactSecretFields(child)
		}
	case []any:
		for i, child := range t {
			t[i] = redactSecretFields(child)
		}
	}
	return v
}
