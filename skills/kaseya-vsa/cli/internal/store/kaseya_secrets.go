// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

// kaseya-vsa handfix (store-never-keeps-secrets): VSA 10 responses carry
// secret-bearing values that MSPs must not find copied into the local
// database, its typed columns or its FTS index: custom-field values
// (GlobalValue, DefaultValue, Value), webhook signing keys (SecretKey) and the
// Windows "Product Key" in asset details (vendor-docs/api.vsax.net.html).
// Every store write passes through mergeIncomingResourceData, which applies
// withoutStoredSecrets to the record it is about to write, so sync, the
// write-through cache and the keep-richer merge all store the stripped
// record. Keys match at any depth, normalized to lowercase letters and digits;
// a record that cannot be decoded is stored as {}.
//
// The same pass is the store's side of the output chokepoint: a string, or a
// key, that repeats a credential this process holds, in any form
// cliutil.HeldEchoes decodes, is replaced by the funnel's notice, and a record
// that still repeats one after that is stored as {}.

import (
	"bytes"
	"encoding/json"
	"strings"
	"unicode"

	"kaseya-vsa-pp-cli/internal/cliutil"
)

// storedSecretKeys are removed wherever they appear (normalized match).
var storedSecretKeys = map[string]bool{
	"value":            true,
	"values":           true,
	"defaultvalue":     true,
	"globalvalue":      true,
	"customfieldvalue": true,
	"secretkey":        true,
}

// storedSecretFragments remove any key that contains them (normalized match).
// VSA 10 documents no token-named field, so "token" is not listed.
var storedSecretFragments = []string{
	"secret", "password", "passwd", "passphrase", "apikey",
	"privatekey", "productkey", "licensekey", "recoverykey", "credential",
}

func isStoredSecretKey(key string) bool {
	var b strings.Builder
	for _, r := range strings.ToLower(key) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	norm := b.String()
	if storedSecretKeys[norm] {
		return true
	}
	for _, frag := range storedSecretFragments {
		if strings.Contains(norm, frag) {
			return true
		}
	}
	return false
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
	if err := enc.Encode(stripStoredSecrets(v)); err != nil {
		return json.RawMessage("{}")
	}
	out := bytes.TrimRight(buf.Bytes(), "\n")
	if cliutil.HeldEchoes(string(out)) {
		return json.RawMessage("{}")
	}
	return json.RawMessage(out)
}

func stripStoredSecrets(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			if isStoredSecretKey(k) || cliutil.HeldEchoes(k) {
				delete(t, k)
				continue
			}
			t[k] = stripStoredSecrets(child)
		}
	case string:
		if cliutil.HeldEchoes(t) {
			return cliutil.FunnelWithheldText
		}
	case []any:
		for i, child := range t {
			t[i] = stripStoredSecrets(child)
		}
	}
	return v
}
