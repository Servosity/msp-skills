// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

// Hand-written (handfixes.json credential-fields-masked-by-name).

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

// MaskedResponseFields are the response members whose value the client
// replaces by name before any sink: the declared credential kit
// ResponseFields (internal/credwire; TestDeclarationsMatchTheMaskedFields
// keeps the two lists one). The kit holds each value as the response passes
// its transport, so without this an answer carrying one (an AVD session's
// sessionToken) would be withheld whole; masked, the rest of it stays usable.
// A tenant secret (a LAPS password, a BitLocker key, a secure variable's
// value) is under none of these names: it is shown to the caller who asked.
var MaskedResponseFields = []string{"access_token", "refresh_token", "id_token", "accessToken", "refreshToken", "idToken", "sessionToken"}

// fieldMask replaces a masked member's value.
const fieldMask = "[withheld]"

// maskCredentialFields replaces the value of every MaskedResponseFields member,
// at any depth. A body that is not exactly one JSON value is returned as it
// came: the kit still withholds it if it carries a held value.
// ponytail: an answer with a masked field is re-encoded, so its keys come back sorted.
func maskCredentialFields(body []byte) []byte {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var v any
	if dec.Decode(&v) != nil {
		return body
	}
	if _, err := dec.Token(); err != io.EOF {
		return body
	}
	v, changed := maskFields(v)
	if !changed {
		return body
	}
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	if enc.Encode(v) != nil {
		return []byte(`"` + fieldMask + `"`)
	}
	return bytes.TrimRight(out.Bytes(), "\n")
}

func maskFields(v any) (any, bool) {
	changed := false
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			if maskedField(k) {
				if s, ok := e.(string); e != nil && (!ok || s != "") {
					if _, isBool := e.(bool); !isBool {
						x[k], changed = fieldMask, true
					}
				}
				continue
			}
			if m, c := maskFields(e); c {
				x[k], changed = m, true
			}
		}
	case []any:
		for i, e := range x {
			if m, c := maskFields(e); c {
				x[i], changed = m, true
			}
		}
	}
	return v, changed
}

func maskedField(name string) bool {
	for _, f := range MaskedResponseFields {
		if strings.EqualFold(f, name) {
			return true
		}
	}
	return false
}
