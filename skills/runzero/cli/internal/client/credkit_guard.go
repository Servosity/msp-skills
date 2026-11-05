// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

// Hand-written (handfixes.json credkit-wiring): the credential kit's outbound
// guard around every transport this package builds, and the response field mask
// that keeps an answer carrying a credential printable.

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"

	"runzero-pp-cli/internal/credkit"
)

// OutboundGuard is the credential kit's outbound guard, around every transport
// a client sends through. Production code never reassigns it. In-process unit
// tests do, to the identity: they never run credkit.Start, so the kit has no
// declarations and no base origin and would refuse every credentialed request
// to a loopback test server. The built binaries' black-box tests
// (internal/runzeroblackbox) prove the guard itself.
var OutboundGuard = credkit.Transport

// guardedTransport is OutboundGuard around tr, with tr kept so a streaming
// client can clone it with its own header timeout and guard the clone.
type guardedTransport struct {
	http.RoundTripper
	inner *http.Transport
}

func guardTransport(tr *http.Transport) guardedTransport {
	return guardedTransport{RoundTripper: OutboundGuard(tr), inner: tr}
}

// WithheldFieldText replaces a credential field's value in an answer.
const WithheldFieldText = "[credential withheld]"

// maskedFields is every field the kit holds by name in a request or response
// body: credwire.Declarations' ResponseFields then CredentialFields.
// TestMaskedFieldsAreTheDeclaredOnes keeps the two lists one; the literal is
// repeated here because the kit's checker reads the Declarations variable only
// as credkit.Start's argument.
var maskedFields = []string{"token", "export_token", "download_token", "access_token", "api_client_secret", "password", "api_key", "client_secret", "secret", "password", "api_key"}

// maskedField reports whether name is one of maskedFields, in any case.
func maskedField(name string) bool {
	return slices.ContainsFunc(maskedFields, func(f string) bool { return strings.EqualFold(f, name) })
}

// maskCredentialFields replaces the value of every maskedFields member, at any
// depth, in the one answer every sink is fed from: an API key's token, an export
// or download token, an access token, a client secret. The kit holds those
// values as the response arrives; masking them by name keeps the rest of the
// record (an API key's comment and usage, an organization's counts) printable
// instead of withholding the whole answer. A body that is not exactly one JSON
// value is returned as it came, and the kit withholds it if it carries a held
// value.
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
		return json.RawMessage(`"` + WithheldFieldText + `"`)
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
						x[k], changed = WithheldFieldText, true
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
