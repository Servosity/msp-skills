// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored (not generator output); pinned by skills/eset-protect/handfixes.json
// under "device-token-redacted-at-decode".
//
// ESET device-management returns v1Device.deviceToken (the spec calls it the
// "Identity token for the device", readOnly) on every device object: GET
// /v1/devices, /v1/devices/{uuid}, /v1/devices:batchGet,
// /v1/device_groups/{uuid}/devices and the :batchImport response. It is a
// per-device credential, not inventory. client.do() runs every API response
// body through RedactCredentialFields right after the body is read and
// inflated, so no caller (CLI output, sync, the local mirror and its FTS
// index, the response cache, MCP tools) ever sees the value.
//
// Fail closed (round 3): the rule is cliutil.RedactJSONKeys, so a body is
// redacted only after it parsed as exactly one JSON value; a body that does
// not parse and names deviceToken in any encoding (plain, \u or %XX escaped,
// any case) is withheld whole, because no pattern can find every value in
// bytes that are not JSON.
package client

import (
	"eset-protect-pp-cli/internal/cliutil"
)

// RedactedValue replaces a credential value in an API response.
const RedactedValue = "[redacted]"

// CredentialBodyWithheld stands in for a response body that could not be
// redacted: it is not one JSON value and it names a credential field.
const CredentialBodyWithheld = "[response body withheld: it is not one JSON value and it names a credential field (deviceToken), so its value cannot be redacted]"

// UndecodableBodyWithheld stands in for a response body that is not one JSON
// value and whose escapes are nested too deeply to check for a credential
// field (handfixes.json: withheld-body-names-its-cause). It names no
// credential: this is a proxy page or a hostile body, not a device token.
const UndecodableBodyWithheld = "[response body withheld: it is not one JSON value and its escapes are nested too deeply to check it for a credential field, so it is not shown]"

// WithheldBodyMessage is the stand-in for a body RedactCredentialFields
// refused, worded by the actual cause.
func WithheldBodyMessage(body []byte) string {
	if names, undecodable := cliutil.NamesKeyOrUndecodable(body, credentialResponseKeys); undecodable && !names {
		return UndecodableBodyWithheld
	}
	return CredentialBodyWithheld
}

// SecretBodyWithheld stands in for a response body that echoes a credential
// this process holds and could not be masked in place (handfixes.json:
// secret-registry-masks-every-boundary): the value is only recoverable by
// decoding, or masking it would break the JSON.
const SecretBodyWithheld = "[response body withheld: it holds a credential this process uses, in a form that cannot be masked in place]"

// credentialResponseKeys are response property names whose value is a
// credential. Matched with strings.EqualFold on the decoded key.
var credentialResponseKeys = []string{"deviceToken"}

// RedactCredentialFields returns body with the value of every credential
// property (at any depth, duplicate keys included) replaced by RedactedValue.
// A body that carries no credential value is returned byte for byte; one that
// does is re-encoded (numbers kept exact, HTML not escaped). A body that is
// not one JSON value is returned unchanged when it cannot name a credential
// key, and refused (ok false) when it may.
func RedactCredentialFields(body []byte) ([]byte, bool) {
	if out, ok := cliutil.RedactJSONKeys(body, credentialResponseKeys, RedactedValue); ok {
		return out, true
	}
	if cliutil.NamesKeyInAnyEncoding(body, credentialResponseKeys) {
		return nil, false
	}
	return body, true
}

// RedactCredentialFieldsOrWithhold is RedactCredentialFields for text that is
// only shown (an API error body): a body that cannot be redacted becomes
// WithheldBodyMessage (CredentialBodyWithheld or UndecodableBodyWithheld).
func RedactCredentialFieldsOrWithhold(body []byte) []byte {
	if out, ok := RedactCredentialFields(body); ok {
		return out
	}
	return []byte(WithheldBodyMessage(body))
}
