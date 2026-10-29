// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

// umbrella handfix (store-never-keeps-secrets): the one rule for which Umbrella
// field names carry credentials, shared by the response redaction, what is held
// with the credential kit and the local store, so none of them can disagree. The
// kit (internal/credkit) does the matching and the withholding; nothing here
// decides what leaves.

import (
	"bytes"
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	"cisco-umbrella-pp-cli/internal/credkit"
)

// Hold is credkit.Hold for every credential this connector finds itself. A
// variable because the kit's held set is process-wide with no reset: an
// in-process test (no credkit.Start) would check every later statement and file
// against its fixtures. The built binaries are the proof of the hold
// (internal/umbrellablackbox). Production code never reassigns it.
var Hold = credkit.Hold

// fieldSource is the kit source of a value found by name or by shape in a
// request, a response or a tool argument. No Origins entry names it, so it
// shares the "*" rule: the configured API and Cisco's own host only.
const fieldSource = "response:credential-field"

// secretFieldPattern matches Umbrella fields that carry credentials, read on
// the normalized name (normalizedFieldName: lowercase, letters and digits
// only, so clientSecret, client_secret, client-secret and CLIENT-SECRET are
// one name): an API key's clientSecret and a string-valued key (KeyWithSecret,
// KeyRefreshSecret), the S3 bucket secretAccessKey (KeyRotation), a tunnel's psk
// and secret (POST /deployments/v2/tunnels, /tunnels/{id}/keys), a user's
// password (UserObject), OAuth access and refresh tokens, and the generic
// credential names (private keys, API keys, pass phrases, communities). The
// public half of a credential pair is a credential too: the clientId sent with
// the client secret for a token, an access key ID, and the S3 bucket key's
// currentKeyId and oldKeyId (KeyRotation). It deliberately over-matches
// ("token", "apikey"); TestEverySpecNameThatCouldCarryACredentialIsJudged walks
// every name in the spec against it.
var secretFieldPattern = regexp.MustCompile(`(passw(or)?d|pass$|passhash|passphrase|secret|credential|token|apikey|privatekey|community|^key$|licensekey|^psk$|preshared|clientid|accesskeyid|currentkeyid|oldkeyid)`)

// publicSecretLikeFields match the pattern but are metadata or an ordinary
// identifier, not a credential (normalized names): an API key's own id, which
// every listing and report names, and the token exchange's type word, which
// would otherwise hold the word "bearer".
var publicSecretLikeFields = map[string]bool{
	"passwordcomplexity": true,
	"apikeyid":           true,
	"apikeys":            true,
	"tokentype":          true,
	"tokenurl":           true,
}

// SecretFieldName reports a property, flag, parameter or header name that
// looks like a credential, in any spelling.
func SecretFieldName(name string) bool {
	n := normalizedFieldName(name)
	return !publicSecretLikeFields[n] && secretFieldPattern.MatchString(n)
}

// normalizedFieldName lowercases ASCII letters and drops every separator
// (anything that is not a letter or digit), keeping non-ASCII bytes.
func normalizedFieldName(name string) string {
	b := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		switch c := name[i]; {
		case 'A' <= c && c <= 'Z':
			b = append(b, c+('a'-'A'))
		case 'a' <= c && c <= 'z', '0' <= c && c <= '9', c >= 0x80:
			b = append(b, c)
		}
	}
	return string(b)
}

// recordWrapper reports the one credential name whose object value is a record,
// not a credential: the spec's KeyResponse {key: {id, name, clientId, ...}}. It
// is walked by name, so an API key's name and dates stay readable and unheld. A
// string-valued key is a credential.
func recordWrapper(name string, v any) bool {
	switch v.(type) {
	case map[string]any, duplicateMembers:
		return normalizedFieldName(name) == "key"
	}
	return false
}

// awsAccessKeyID is the shape of an AWS access key ID, the public half of the
// Cisco-managed S3 bucket key: held whatever field carries it.
var awsAccessKeyID = regexp.MustCompile(`^(AKIA|ASIA)[A-Z0-9]{16}$`)

// RedactedValue replaces a credential-bearing value.
const RedactedValue = "[redacted]"

// RedactSecretFields walks a decoded JSON value and masks every value under a
// credential name (containers included, recordWrapper excepted). Null, booleans
// and numbers are kept: flags and counts ("total_tokens"), never a credential.
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
			if isSecret(k) && !recordWrapper(k, val) {
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
// empty, nothing but asterisks or bullets, or this program's own RedactedValue. A
// word is never a mask: a password that is literally "hidden" is a password.
func isMaskValue(v string) bool {
	v = strings.TrimSpace(v)
	return v == "" || strings.Trim(v, "*\u2022") == "" || v == RedactedValue
}

// HoldCredentialValues is the one walker that registers credentials found in
// structured data with the credential kit (handfixes.json credkit-wiring): every
// string, of any length, under a credential-named key at any depth (all strings
// under a credential-named object or array), except masks, and every string
// shaped like an AWS access key ID wherever it sits. A string that is itself a
// JSON object or array is read too. Request bodies and params, responses and MCP
// tool arguments all use it.
func HoldCredentialValues(v any) {
	holdCredentialValues(v, false)
}

func holdCredentialValues(v any, secret bool) {
	switch t := v.(type) {
	case duplicateMembers:
		for _, variant := range t {
			holdCredentialValues(variant, secret)
		}
	case map[string]any:
		for k, child := range t {
			holdCredentialValues(child, secret || (SecretFieldName(k) && !recordWrapper(k, child)))
		}
	case []any:
		for _, child := range t {
			holdCredentialValues(child, secret)
		}
	case string:
		if awsAccessKeyID.MatchString(strings.TrimSpace(t)) {
			Hold(fieldSource, t)
		}
		if secret && !isMaskValue(t) {
			Hold(fieldSource, t)
		}
		if trimmed := strings.TrimSpace(t); len(trimmed) > 1 && (trimmed[0] == '{' || trimmed[0] == '[') {
			for _, inner := range decodeDocuments([]byte(trimmed)) {
				holdCredentialValues(inner, secret)
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
	HoldCredentialValues(m)
	HoldCredentialFields(body)
}

// RedactBody is a JSON response body with every value under a credential name
// replaced by RedactedValue (RedactSecretFields), so a listing that carries a
// clientId stays readable once the clientId is held: the kit withholds a whole
// unit that repeats a held value. Numbers keep their spelling. A body that is not
// JSON, or holds nothing to redact, comes back unchanged.
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
