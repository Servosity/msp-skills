// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

// liongard handfix (store-never-keeps-secrets): every resource write passes
// mergeIncomingResourceData, which returns withoutStoredSecrets of the record
// it is about to write. A value under a credential response field
// (CredentialResponseFields, the same names as credwire's ResponseFields: a
// launchpoint's SecureConfig, a log's Secrets, a new access key's
// AccessKeySecret), at any depth, is replaced by "[redacted]", so the record is
// stored without it. The shared credential
// kit's SQL driver (internal/credsql) refuses any statement that still carries
// a held value; this redaction only keeps such a record storable rather than
// refused whole.

import (
	"bytes"
	"encoding/json"
	"strings"
)

// CredentialResponseFields are the names credwire.Declarations.ResponseFields
// declares. The kit's checker allows the Declarations variable only as
// credkit.Start's argument, so the list is repeated here;
// credwire TestStoreRedactsTheDeclaredResponseFields keeps the two equal.
var CredentialResponseFields = []string{"AccessKeySecret", "Token", "SecureConfig", "Secrets", "Password", "accessToken", "tokens"}

func withoutStoredSecrets(data json.RawMessage) json.RawMessage {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return data // the kit's SQL driver still checks it
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(redactCredentialFields(v)); err != nil {
		return data
	}
	return json.RawMessage(bytes.TrimRight(buf.Bytes(), "\n"))
}

func redactCredentialFields(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			if isCredentialField(k) {
				t[k] = "[redacted]"
				continue
			}
			t[k] = redactCredentialFields(child)
		}
	case []any:
		for i, child := range t {
			t[i] = redactCredentialFields(child)
		}
	}
	return v
}

func isCredentialField(name string) bool {
	for _, f := range CredentialResponseFields {
		if strings.EqualFold(f, name) {
			return true
		}
	}
	return false
}
