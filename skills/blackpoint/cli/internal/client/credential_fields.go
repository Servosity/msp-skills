// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

// Hand-written (handfixes.json credential-fields-withheld).

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"

	"blackpoint-pp-cli/internal/cliutil"
)

// maskCredentialFields replaces the value of every cliutil.MaskedResponseFields
// member, at any depth, in the one answer every sink is fed from. A body that is
// not exactly one JSON value is returned as it came: the kit still withholds it if
// it carries a held value, and the persistence rules still refuse it
// (cliutil.MayHoldSecretFields).
// ponytail: an answer with a masked field is re-encoded, so its keys come back sorted.
func maskCredentialFields(body []byte) json.RawMessage {
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
		return json.RawMessage(`"` + cliutil.SecretFieldWithheld + `"`)
	}
	return bytes.TrimRight(out.Bytes(), "\n")
}

func maskFields(v any) (any, bool) {
	changed := false
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			if maskedField(k) {
				switch e.(type) {
				case nil, bool:
				default:
					if s, ok := e.(string); !ok || s != "" {
						x[k], changed = cliutil.SecretFieldWithheld, true
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
	for _, f := range cliutil.MaskedResponseFields {
		if strings.EqualFold(f, name) {
			return true
		}
	}
	return false
}
