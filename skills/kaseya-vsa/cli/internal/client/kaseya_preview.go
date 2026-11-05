// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json body-credential-fields): what a
// write preview shows. The kit holds a body's credentials (credkit.HoldBody,
// and the MCP arguments) and withholds any preview that still carries one, so
// a preview masks those fields to stay readable. Display only: it holds nothing.

package client

import (
	"bytes"
	"encoding/json"
	"strings"
)

// PreviewCredentialFields is Declarations.CredentialFields; a credwire test
// keeps the two equal.
var PreviewCredentialFields = []string{"Headers", "SecretKey"}

// PreviewMasked returns a copy of v with each credential field (any case, any
// depth) replaced.
func PreviewMasked(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, field := range t {
			out[k] = PreviewMasked(field)
			for _, name := range PreviewCredentialFields {
				if strings.EqualFold(k, name) {
					out[k] = "[credential field withheld]"
				}
			}
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = PreviewMasked(item)
		}
		return out
	}
	return v
}

// previewBytes is an encoded body, masked. A body that does not parse is not
// previewed.
func previewBytes(body []byte) []byte {
	if body == nil {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var v any
	if dec.Decode(&v) != nil {
		return nil
	}
	b, err := json.Marshal(PreviewMasked(v))
	if err != nil {
		return nil
	}
	return b
}
