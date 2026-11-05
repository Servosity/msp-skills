// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (netsapiens; handfixes.json netsapiens-client-redacts-every-answer):
// the one rule for which NetSapiens field names carry credentials, and the
// client's ingress.
//
// NetSapiens answers carry credentials in ordinary fields (SMTP, SIP and
// provisioning passwords, voicemail and conference PINs, API keys, OAuth
// tokens) and sometimes inside text ("device-sip-registration-password=...").
// The shared credential kit withholds any unit that repeats a held value, so a
// device list with a SIP password in it would never print. The client therefore
// replaces each credential field with RedactedValue before anything reads the
// answer, and holds its value with the kit for everything else. Every sink
// stays the kit's (internal/credwire).

package cliutil

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"netsapiens-pp-cli/internal/credkit"
)

// Hold is credkit.Hold for every credential this connector finds itself. A
// variable because the kit's held set is process-wide with no reset: an
// in-process test (no credkit.Start) would check every later statement and file
// against its fixtures. The built binaries are the proof of the hold
// (internal/credwire). Production code never reassigns it.
var Hold = credkit.Hold

// fieldSource is the kit source of a credential field found by name in a vendor
// answer, a request or a tool argument. No Origins entry names it, so it shares
// the "*" rule: the configured NetSapiens server only.
const fieldSource = "response:credential-field"

// RedactedValue replaces a credential's value wherever it would be shown or stored.
const RedactedValue = "<redacted>"

var secretWords = map[string]bool{
	"password": true, "passwd": true, "pwd": true, "secret": true, "passcode": true,
	"passphrase": true, "pin": true, "token": true, "idtoken": true, "pass": true,
	"otp": true, "apikey": true, "credential": true, "credentials": true,
}

// describingWords end a field name that is about a credential, not one.
var describingWords = map[string]bool{
	"enabled": true, "required": true, "require": true, "endpoint": true, "type": true,
	"url": true, "uri": true, "length": true, "expires": true, "expiry": true,
	"expiration": true, "id": true, "count": true, "policy": true, "format": true,
	"mode": true, "set": true, "allowed": true, "enforced": true, "press": true,
	"datetime": true, "date": true, "time": true, "reset": true, "changed": true,
}

// notSecretFields are exact names whose words look like a credential but are not.
var notSecretFields = map[string]bool{
	"language-token": true, // a UI language identifier (en_US)
}

// documentedSecretFields are credentials the vendor's spec documents under a name
// the word rule cannot read: meeting-registration-id is a webinar guest's token,
// refresh is a refresh JWT (JwtRequestSuccess). Matched on the name's words, so
// every spelling of one is the same field.
var documentedSecretFields = map[string]bool{
	"refresh":                 true,
	"meeting registration id": true,
}

// fieldWords splits a field name on separators and camelCase boundaries.
func fieldWords(name string) []string {
	var words []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			words = append(words, strings.ToLower(string(cur)))
			cur = cur[:0]
		}
	}
	rs := []rune(name)
	for i, r := range rs {
		switch {
		case !unicode.IsLetter(r) && !unicode.IsDigit(r):
			flush()
		case unicode.IsUpper(r) && i > 0 && (unicode.IsLower(rs[i-1]) || (i+1 < len(rs) && unicode.IsLower(rs[i+1]) && unicode.IsUpper(rs[i-1]))):
			flush()
			cur = append(cur, r)
		default:
			cur = append(cur, r)
		}
	}
	flush()
	return words
}

// IsSecretField reports whether a field, flag, column or variable name holds
// a credential: one of its words is a credential word, or it ends in "key"
// (SipSipAuthKey, authentication_key, key), unless its last word describes the
// credential rather than holding it (meeting-join-require-passcode-enabled,
// token-endpoint, token_type, key-id).
func IsSecretField(name string) bool {
	if notSecretFields[strings.ToLower(name)] {
		return false
	}
	words := fieldWords(name)
	if len(words) == 0 {
		return false
	}
	if documentedSecretFields[strings.Join(words, " ")] {
		return true
	}
	last := words[len(words)-1]
	if describingWords[last] {
		return false
	}
	if last == "key" {
		return true
	}
	for _, w := range words { // pwd_hash: a password hash is a credential too
		if secretWords[w] {
			return true
		}
	}
	return false
}

// pinWords mark a field whose value is a short numeric secret: a voicemail or
// conference PIN, a meeting passcode. Internal/credwire declares each such
// response field an Identifier with this reason: it is replaced by RedactedValue
// wherever it is named, and not looked for elsewhere, since a four-digit value
// held from one user among thousands would withhold every later unit that holds
// the same digits as a token (an extension, a count).
var pinWords = map[string]bool{"pin": true, "passcode": true}

// IsPinField reports whether a credential field holds a short numeric secret.
func IsPinField(name string) bool {
	for _, w := range fieldWords(name) {
		if pinWords[w] {
			return true
		}
	}
	return false
}

// holdVendor holds a credential value read from a vendor answer or a request.
// A credential has no exemption for being short or looking like a word.
func holdVendor(v string) {
	v = strings.TrimSpace(v)
	if v == "" || v == RedactedValue {
		return
	}
	Hold(fieldSource, v)
}

func holdLeaves(v any) {
	switch t := v.(type) {
	case string:
		holdVendor(t)
	case json.Number:
		holdVendor(t.String())
	case map[string]any:
		for _, x := range t {
			holdLeaves(x)
		}
	case []any:
		for _, x := range t {
			holdLeaves(x)
		}
	}
}

// credentialPair matches a credential named inside text: name=value,
// name: value, "name":"value". Group 1 is the name, group 2 the value.
var credentialPair = regexp.MustCompile(`([A-Za-z][A-Za-z0-9_.\-]{0,63})["']?\s*[:=]\s*("(?:[^"\\]|\\.)*"|'[^']*'|[^\s"',;&<>{}\[\]()]+)`)

// RedactCredentialText holds and redacts every credential named inside text.
func RedactCredentialText(text string) string {
	if !strings.ContainsAny(text, ":=") {
		return text
	}
	var b strings.Builder
	last := 0
	for p := 0; p < len(text); {
		m := credentialPair.FindStringSubmatchIndex(text[p:])
		if m == nil {
			break
		}
		name, vs, ve := text[p+m[2]:p+m[3]], p+m[4], p+m[5]
		if !IsSecretField(name) {
			p = vs // rescan the value: "message":"password=..." names one inside it
			continue
		}
		p = ve
		val := text[vs:ve]
		inner, quoted := val, false
		if len(val) >= 2 && (val[0] == '"' || val[0] == '\'') {
			inner, quoted = val[1:len(val)-1], true
			if val[0] == '"' {
				if u, err := strconv.Unquote(val); err == nil {
					inner = u
				}
			}
		}
		switch inner {
		case "", RedactedValue, "null", "true", "false":
			continue
		}
		if !IsPinField(name) {
			holdVendor(inner)
		}
		b.WriteString(text[last:vs])
		if quoted {
			b.WriteString(val[:1] + RedactedValue + val[:1])
		} else {
			b.WriteString(RedactedValue)
		}
		last = ve
	}
	if last == 0 {
		return text
	}
	b.WriteString(text[last:])
	return b.String()
}

func redactValue(v any) (any, bool) {
	switch t := v.(type) {
	case map[string]any:
		changed := false
		for k, val := range t {
			if IsSecretField(k) && val != nil && val != "" && val != RedactedValue {
				if !IsPinField(k) {
					holdLeaves(val)
				}
				t[k] = RedactedValue
				changed = true
				continue
			}
			if nv, ch := redactValue(val); ch {
				t[k] = nv
				changed = true
			}
		}
		return t, changed
	case []any:
		changed := false
		for i, item := range t {
			if nv, ch := redactValue(item); ch {
				t[i] = nv
				changed = true
			}
		}
		return t, changed
	case string:
		// A JWT is a credential whatever the vendor names its field (a scope that
		// carries one, a field the spec does not document).
		if LooksLikeJWT(t) {
			holdVendor(strings.TrimPrefix(strings.TrimSpace(t), "Bearer "))
			return RedactedValue, true
		}
		if red := RedactCredentialText(t); red != t {
			return red, true
		}
	}
	return v, false
}

// RedactCredentials holds every credential in data and returns data with each
// one replaced by RedactedValue: a credential-named field at any depth (any
// value but null and ""), and a credential named inside any string value or
// in a body that is not JSON. changed reports whether anything was replaced.
func RedactCredentials(data []byte) (out []byte, changed bool) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return data, false
	}
	if trimmed[0] == '{' || trimmed[0] == '[' || trimmed[0] == '"' {
		dec := json.NewDecoder(bytes.NewReader(trimmed))
		dec.UseNumber()
		var v any
		if err := dec.Decode(&v); err == nil && !dec.More() {
			nv, ch := redactValue(v)
			if !ch {
				return data, false
			}
			var buf bytes.Buffer
			enc := json.NewEncoder(&buf)
			enc.SetEscapeHTML(false)
			if err := enc.Encode(nv); err != nil {
				return []byte(`{}`), true
			}
			return bytes.TrimRight(buf.Bytes(), "\n"), true
		}
	}
	red := RedactCredentialText(string(data))
	return []byte(red), red != string(data)
}

// ErrRedactedCredential refuses a request that would write the redaction
// placeholder back as a credential.
var ErrRedactedCredential = errors.New("refusing to send a credential set to the redaction placeholder " + RedactedValue + "; pass the real value or leave the field out")

// HoldOutgoing holds the credentials an outgoing request carries (body fields,
// query or form parameters) and refuses one whose value is the placeholder.
func HoldOutgoing(body []byte, params map[string]string) error {
	for k, v := range params {
		if IsSecretField(k) {
			if v == RedactedValue {
				return fmt.Errorf("%s: %w", k, ErrRedactedCredential)
			}
			holdVendor(v)
		}
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var v any
	if dec.Decode(&v) != nil {
		return nil // not JSON (multipart, form): its fields are params
	}
	return holdOutgoingValue(v)
}

func holdOutgoingValue(v any) error {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if IsSecretField(k) {
				if val == RedactedValue {
					return fmt.Errorf("%s: %w", k, ErrRedactedCredential)
				}
				holdLeaves(val)
				continue
			}
			if err := holdOutgoingValue(val); err != nil {
				return err
			}
		}
	case []any:
		for _, item := range t {
			if err := holdOutgoingValue(item); err != nil {
				return err
			}
		}
	}
	return nil
}
