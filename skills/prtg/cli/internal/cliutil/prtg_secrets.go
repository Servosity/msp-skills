// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

// prtg handfix (store-never-keeps-secrets): the one rule for which PRTG field
// names carry credentials, shared by fleet output redaction, the response
// redaction, what is held with the credential kit and the local store, so none
// of them can disagree. The kit (internal/credkit) does the matching and the
// withholding; nothing here decides what leaves.

import (
	"bytes"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"

	"prtg-pp-cli/internal/credkit"
)

// Hold is credkit.Hold for every credential this connector finds itself. A
// variable because the kit's held set is process-wide with no reset: an
// in-process test (no credkit.Start) would check every later statement and file
// against its fixtures. The built binaries are the proof of the hold
// (internal/prtgblackbox). Production code never reassigns it.
var Hold = credkit.Hold

// fieldSource is the kit source of a value found by name in a request, a
// response or a tool argument. No Origins entry names it, so it shares the "*"
// rule: the configured core only.
const fieldSource = "response:credential-field"

// secretFieldPattern matches PRTG fields that carry credentials: the masked
// "**********" settings in include sections (windowsloginpassword,
// linuxloginpassword, snmpauthpass, esxpassword, ...), API keys, session
// tokens, private keys, SNMP communities and the license key (LicenseInfo
// "key"), and the legacy API's passhash. It deliberately over-matches
// ("token").
var secretFieldPattern = regexp.MustCompile(`(?i)(passw(or)?d|pass$|pass_?hash|secret|credential|token|apikey|api_key|privatekey|private_key|community|^key$|license_?key)`)

// publicSecretLikeFields match the pattern but are metadata, not a credential.
var publicSecretLikeFields = map[string]bool{
	"password_complexity": true, "passwordcomplexity": true,
}

// SecretFieldName reports a property name that looks like a credential.
func SecretFieldName(name string) bool {
	return !publicSecretLikeFields[strings.ToLower(name)] && secretFieldPattern.MatchString(name)
}

// RedactedValue replaces a credential-bearing value.
const RedactedValue = "[redacted]"

// RedactSecretFields walks a decoded JSON value and masks every value under a
// credential name (containers included). Null, booleans and numbers are kept:
// flags and counts ("total_tokens"), never a credential.
func RedactSecretFields(v any) any {
	return redactSecretFields(v, map[string]bool{})
}

// redactSecretFields memoises SecretFieldName per name: a large collection
// repeats the same few dozen keys, and the regex dominates the walk.
func redactSecretFields(v any, seen map[string]bool) any {
	isSecret := func(k string) bool {
		hit, ok := seen[k]
		if !ok {
			hit = SecretFieldName(k)
			seen[k] = hit
		}
		return hit
	}
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if isSecret(k) {
				switch val.(type) {
				case nil, bool, float64, json.Number:
				default:
					t[k] = RedactedValue
					continue
				}
			}
			t[k] = redactSecretFields(val, seen)
		}
		return t
	case []any:
		for i := range t {
			t[i] = redactSecretFields(t[i], seen)
		}
		return t
	default:
		return v
	}
}

// isMaskValue reports a value that already hides a credential, by shape only:
// empty, nothing but asterisks or bullets (PRTG's "****" settings), or this
// program's own RedactedValue. A word is never a mask: a password that is
// literally "hidden" is a password.
func isMaskValue(v string) bool {
	v = strings.TrimSpace(v)
	return v == "" || strings.Trim(v, "*\u2022") == "" || v == RedactedValue
}

// HoldCredentialValues is the one walker that registers credentials found in
// structured data with the credential kit (handfixes.json credkit-wiring): every
// string under a credential-named key at any depth (all strings under a
// credential-named object or array), except masks. A string that is itself a
// JSON object or array is read too. Responses and output members use it.
func HoldCredentialValues(v any) {
	holdCredentialValues(v, false, false)
}

// HoldOperatorValues is HoldCredentialValues for what the operator supplies (a
// tool's arguments, a request's params and body), where a number under a
// credential name counts as well as a string, and nothing is a mask: a mask is
// how a response hides a setting, and an operator's password of asterisks is a
// password.
func HoldOperatorValues(v any) {
	holdCredentialValues(v, false, true)
}

func holdCredentialValues(v any, secret, operator bool) {
	switch t := v.(type) {
	case duplicateMembers:
		for _, variant := range t {
			holdCredentialValues(variant, secret, operator)
		}
	case map[string]any:
		for k, child := range t {
			holdCredentialValues(child, secret || SecretFieldName(k), operator)
		}
	case []any:
		for _, child := range t {
			holdCredentialValues(child, secret, operator)
		}
	case float64:
		if secret && operator {
			Hold(fieldSource, strconv.FormatFloat(t, 'f', -1, 64))
		}
	case json.Number:
		if secret && operator {
			Hold(fieldSource, t.String())
		}
	case string:
		if secret && t != "" && (operator || !isMaskValue(t)) {
			Hold(fieldSource, t)
			if trimmed := strings.TrimSpace(t); len(trimmed) > 7 && strings.EqualFold(trimmed[:7], "bearer ") {
				Hold(fieldSource, strings.TrimSpace(trimmed[7:]))
			}
		}
		if trimmed := strings.TrimSpace(t); len(trimmed) > 1 && (trimmed[0] == '{' || trimmed[0] == '[') {
			for _, inner := range decodeDocuments([]byte(trimmed)) {
				holdCredentialValues(inner, secret, operator)
			}
		}
	}
}

// duplicateMembers is an object that repeats a member, as one map per
// occurrence: the i-th has the i-th of each repeated member (the last when that
// member has fewer). encoding/json keeps only the last, and every member of a
// duplicate key is held.
type duplicateMembers []map[string]any

// maxJSONDepth is encoding/json's own nesting limit.
const maxJSONDepth = 10000

// decodeJSONValue reads one JSON value, keeping every member of a duplicate key.
func decodeJSONValue(dec *json.Decoder, depth int) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	delim, isDelim := tok.(json.Delim)
	if !isDelim {
		return tok, nil
	}
	if depth >= maxJSONDepth {
		return nil, errors.New("JSON nested too deeply")
	}
	if delim == '[' {
		items := []any{}
		for dec.More() {
			item, err := decodeJSONValue(dec, depth+1)
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		}
		_, err := dec.Token()
		return items, err
	}
	members := map[string][]any{}
	most := 1
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return nil, err
		}
		name, _ := key.(string)
		val, err := decodeJSONValue(dec, depth+1)
		if err != nil {
			return nil, err
		}
		members[name] = append(members[name], val)
		most = max(most, len(members[name]))
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	variants := make(duplicateMembers, most)
	for i := range variants {
		variants[i] = make(map[string]any, len(members))
		for name, vals := range members {
			variants[i][name] = vals[min(i, len(vals)-1)]
		}
	}
	if most == 1 {
		return variants[0], nil
	}
	return variants, nil
}

// decodeDocuments reads every JSON document in body, up to the first that does
// not parse.
func decodeDocuments(body []byte) []any {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var docs []any
	for {
		doc, err := decodeJSONValue(dec, 0)
		if err != nil {
			return docs
		}
		docs = append(docs, doc)
	}
}

// HoldCredentialFields decodes a JSON body and holds its credential values.
func HoldCredentialFields(body []byte) {
	for _, doc := range decodeDocuments(body) {
		HoldCredentialValues(doc)
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
	HoldOperatorValues(m)
	for _, doc := range decodeDocuments(body) {
		HoldOperatorValues(doc)
	}
}

// RedactBody is a JSON response body with every value under a credential name
// replaced by RedactedValue (RedactSecretFields), so a listing that carries a
// device's password setting stays readable once the value is held: the kit
// withholds a whole unit that repeats a held value. Numbers keep their spelling.
// A body that is not JSON, or holds nothing to redact, comes back unchanged.
func RedactBody(body []byte) []byte {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var v any
	if dec.Decode(&v) != nil || dec.More() {
		return body
	}
	before, err := json.Marshal(v)
	if err != nil {
		return body
	}
	after, err := json.Marshal(RedactSecretFields(v))
	if err != nil || bytes.Equal(before, after) {
		return body
	}
	return after
}
