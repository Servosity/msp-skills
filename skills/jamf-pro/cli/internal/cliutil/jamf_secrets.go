// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

// jamf-pro handfix (output-chokepoint, store-never-keeps-secrets): the
// credential holder and the local store's strip judge credentials with the
// one rule in jamf_redact.go (SecretField for names, secretRecord for
// name/value records), the rule fleet output uses, so none of them can
// disagree.

import (
	"encoding/json"
	"strings"

	"jamf-pro-pp-cli/internal/credkit"
)

// Hold is credkit.Hold for every credential this connector finds itself. A
// variable because the kit's held set is process-wide with no reset: an
// in-process test (no credkit.Start) would check every later statement and file
// against its fixtures. The built binaries are the proof of the hold
// (internal/jamfblackbox). Production code never reassigns it.
var Hold = credkit.Hold

// fieldSource is the kit source of a value found by name or by shape in a
// request, a response or a tool argument. No Origins entry names it, so it
// shares the "*" rule: the configured Jamf Pro server only.
const fieldSource = "response:credential-field"

// holdMinLen is the shortest value found in a request, a response or a tool
// argument that is held; shorter ones are redacted where they are named but
// would match by chance everywhere else.
const holdMinLen = 4

// shortNumericLen: an all-digit value shorter than this (a device PIN, a
// passcode) is redacted where it is named but never held. Holding it would
// withhold every ID, serial and count that contains those digits.
const shortNumericLen = 8

// isMaskValue reports a value that already hides a credential: "****",
// Redacted, the funnel's notices, and placeholder words such as the learn
// journal's "redacted" argv class.
func isMaskValue(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" || strings.Trim(v, "*•") == "" || strings.HasPrefix(v, "credkit: withheld") || strings.HasPrefix(v, RedactedDigestPrefix) {
		return true
	}
	switch strings.ToLower(strings.Trim(v, "[]<>()")) {
	case "redacted", "withheld":
		return true
	}
	return false
}

func shortNumeric(v string) bool {
	v = strings.TrimSpace(v)
	if len(v) >= shortNumericLen {
		return false
	}
	return strings.Trim(v, "0123456789") == ""
}

// embeddedJSON decodes a string that is itself a JSON object or array.
func embeddedJSON(s string) (any, bool) {
	s = strings.TrimSpace(s)
	if len(s) < 2 || (s[0] != '{' && s[0] != '[') {
		return nil, false
	}
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	var v any
	if dec.Decode(&v) != nil {
		return nil, false
	}
	return v, true
}

// HoldCredentialValues is the one walker that registers credentials found in
// structured data with the holder (jamf-pro handfix: output-chokepoint):
// every string under a credential-named key at any depth (all strings under
// a credential-named object or array) and the value of a name/value record
// labelled as a credential, except masks, values under holdMinLen and short
// all-digit values (shortNumeric). A string that is itself a JSON document is
// read too. Request bodies and params, responses, MCP tool arguments and
// output members all use it.
func HoldCredentialValues(v any) {
	holdCredentialValues(v, false, map[string]bool{})
}

func holdCredentialValues(v any, secret bool, seen map[string]bool) {
	switch t := v.(type) {
	case map[string]any:
		record := secretRecord(t)
		for k, child := range t {
			holdCredentialValues(child, secret || secretName(k, seen) || (record && valueField(k)), seen)
		}
	case []any:
		for _, child := range t {
			holdCredentialValues(child, secret, seen)
		}
	case string:
		if secret {
			holdFound(t)
		}
		if inner, ok := embeddedJSON(t); ok {
			holdCredentialValues(inner, secret, seen)
		}
	case json.Number:
		if secret && !shortNumeric(t.String()) && len(t.String()) >= holdMinLen {
			Hold(fieldSource, t.String())
		}
	}
}

// holdFound holds one string the rule judged a credential, unless it is a mask
// or too short to match anything but by chance.
func holdFound(v string) {
	if len(strings.TrimSpace(v)) < holdMinLen || isMaskValue(v) || shortNumeric(v) {
		return
	}
	Hold(fieldSource, v)
	if len(v) > 7 && strings.EqualFold(v[:7], "bearer ") {
		Hold(fieldSource, v[7:])
	}
}

// HoldCredentialFields decodes a JSON body and holds its credential values.
func HoldCredentialFields(body []byte) {
	if len(body) == 0 {
		return
	}
	if v, ok := embeddedJSON(string(body)); ok {
		HoldCredentialValues(v)
	}
}

// HoldRequestCredentials holds the credential values of an outgoing request
// (query params named like a credential, and the JSON body) before it is
// previewed or sent.
func HoldRequestCredentials(params map[string]string, body []byte) {
	m := make(map[string]any, len(params))
	for k, v := range params {
		m[k] = v
	}
	HoldCredentialValues(m)
	HoldCredentialFields(body)
}
