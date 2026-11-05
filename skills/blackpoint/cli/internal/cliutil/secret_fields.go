// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (secret-field-persistence class): an API response can carry a
// secret that is not this process's credential - a dark-web exposure's
// password, a Duo secret key on an onboarding record. Such a value may be shown
// to the operator on a live read, but never written to disk. One name rule
// decides which JSON keys (and store columns) are secret; the response cache
// refuses any body that holds one, and the local store replaces each value
// before it is written. A value under a secret-named key is withheld whatever
// its shape: every string, number and nested value beneath it.
//
// One detector (scanSecretFields) reads the raw bytes token by token rather
// than the decoded value, because encoding/json keeps only the last of a
// duplicate key and reads only the first top-level value: a body like
// {"password":"x","password":null} decodes to a null while its bytes still
// carry "x". Every key occurrence is seen, at any depth, in every top-level
// value. Text the walk cannot read to its end is never persisted as-is.

package cliutil

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

// SecretFieldWithheld replaces a secret field's value in anything persisted.
const SecretFieldWithheld = "[withheld]"

var (
	secretNameExact = map[string]bool{
		"pwd": true, "pass": true, "passphrase": true, "key": true, "skey": true,
		"secret": true, "token": true, "auth": true, "authorization": true,
		"cookie": true, "setcookie": true, "otp": true, "pin": true, "mfasecret": true,
	}
	secretNameContains = []string{
		"password", "passwd", "secret", "apikey", "privatekey", "accesskey",
		"credential", "sessionkey", "signingkey", "encryptionkey",
	}
	secretNameSuffix = []string{"token", "apikey", "key"}
	// Names ending in "key" that identify rather than authenticate.
	secretNameKeyAllow = map[string]bool{"ikey": true, "integrationkey": true, "publickey": true, "idempotencykey": true, "sortkey": true, "partitionkey": true}
)

// SecretFieldName reports whether a JSON key or column name names a secret.
// Matching is case- and separator-insensitive (Password, client_secret,
// api-key and X-Api-Key all match).
func SecretFieldName(name string) bool {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	n := b.String()
	if n == "" || secretNameKeyAllow[n] {
		return false
	}
	if secretNameExact[n] {
		return true
	}
	for _, s := range secretNameContains {
		if strings.Contains(n, s) {
			return true
		}
	}
	// "passwordExposed" is a flag, not the secret: a contains-match on a
	// boolean is handled by value below. Suffix matches catch accessToken,
	// refreshToken, secretKey, xApiKey.
	for _, s := range secretNameSuffix {
		if strings.HasSuffix(n, s) {
			return true
		}
	}
	return false
}

// HasSecretFields reports whether data is JSON holding a secret-named key with
// a value other than null, a boolean or the withheld marker. Every key
// occurrence counts: each of a duplicate key, each top-level value, every
// depth. Text that starts like JSON ({ or [) but cannot be read to its end
// also reports true when the walk stopped inside a secret value, or when the
// text holds a secret-named key occurrence ("name":) anywhere. Text that does
// not start like JSON reports false, so plain column values are not field
// matches.
func HasSecretFields(data []byte) bool {
	shaped := startsLikeJSON(data)
	if !shaped && bytes.IndexByte(data, '{') < 0 {
		// No object, so no key: the store driver checks every bound
		// value, and plain ids, names and timestamps stop here.
		return false
	}
	r := scanSecretFields(data)
	if r.secret {
		return true
	}
	if r.complete || !shaped {
		return false
	}
	return textHoldsSecretKey(data)
}

// MayHoldSecretFields is the test for writing data to disk in its original
// bytes. It reports false only when data is empty or exactly one JSON value
// read to its end with no secret value. Anything else (a secret, trailing
// text, a second value, text that is not JSON) may carry a secret the walk
// could not rule out: persist RedactSecretFields(data) or nothing.
func MayHoldSecretFields(data []byte) bool {
	r := scanSecretFields(data)
	return r.secret || !r.complete
}

// RedactSecretFields returns data with every value under a secret-named key
// replaced by SecretFieldWithheld (booleans and nulls are kept: they say
// whether a secret exists, not what it is). Data that MayHoldSecretFields
// clears is returned unchanged. Anything else is returned only as its first
// JSON value re-encoded with secrets withheld (a duplicate key's earlier
// value, trailing text and later values do not survive), or, when that value
// cannot be decoded, as a withheld marker: never as its original bytes.
func RedactSecretFields(data []byte) []byte {
	if !MayHoldSecretFields(data) {
		return data
	}
	if v, ok := decodeJSONValue(data); ok {
		if out, err := json.Marshal(redact(v, false)); err == nil {
			return out
		}
	}
	// Fail closed: text that held, or may hold, a secret and cannot be
	// re-encoded is not persisted in any form.
	b, _ := json.Marshal(map[string]string{"withheld": SecretFieldWithheld})
	return b
}

func decodeJSONValue(data []byte) (any, bool) {
	t := bytes.TrimSpace(data)
	if len(t) == 0 || (t[0] != '{' && t[0] != '[') {
		return nil, false
	}
	dec := json.NewDecoder(bytes.NewReader(t))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, false
	}
	return v, true
}

// secretScan is what one token walk over data found.
type secretScan struct {
	secret   bool // a secret value was read, or the walk stopped inside one
	complete bool // data is empty or one JSON value, read to its end
}

type secretScanFrame struct {
	object   bool
	under    bool // the container sits under a secret-named key
	wantKey  bool // object: the next token is a key or the closing brace
	valUnder bool // object: the pending value sits under a secret-named key
}

// maxSecretScanDepth matches encoding/json's nesting limit; deeper text is
// not read to its end, so it is never cleared.
const maxSecretScanDepth = 10000

// scanSecretFields walks every token of data, tracking whether the current
// value sits under a secret-named key at any depth inside objects and arrays.
// A secret value is a non-empty string other than the withheld marker, or a
// number. The walk continues past the first top-level value so a second one
// is read too.
func scanSecretFields(data []byte) secretScan {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var stack []secretScanFrame
	values := 0
	pending := func() bool {
		if len(stack) == 0 {
			return false
		}
		f := stack[len(stack)-1]
		if f.object && !f.wantKey {
			return f.valUnder
		}
		return f.under
	}
	ended := func() {
		if len(stack) == 0 {
			values++
		} else if f := &stack[len(stack)-1]; f.object {
			f.wantKey = true
		}
	}
	for {
		tok, err := dec.Token()
		if err != nil {
			if err == io.EOF && len(stack) == 0 {
				return secretScan{complete: values <= 1}
			}
			return secretScan{secret: pending()}
		}
		if n := len(stack); n > 0 && stack[n-1].object && stack[n-1].wantKey {
			if k, ok := tok.(string); ok {
				f := &stack[n-1]
				f.wantKey = false
				f.valUnder = f.under || SecretFieldName(k)
			} else { // the closing brace
				stack = stack[:n-1]
				ended()
			}
			continue
		}
		under := pending()
		switch x := tok.(type) {
		case json.Delim:
			if x == '{' || x == '[' {
				if len(stack) >= maxSecretScanDepth {
					return secretScan{secret: under}
				}
				stack = append(stack, secretScanFrame{object: x == '{', under: under, wantKey: x == '{'})
				continue
			}
			stack = stack[:len(stack)-1] // the closing bracket
		case string:
			if under && x != "" && x != SecretFieldWithheld {
				return secretScan{secret: true}
			}
		case json.Number:
			if under {
				return secretScan{secret: true}
			}
		}
		ended()
	}
}

func startsLikeJSON(data []byte) bool {
	t := bytes.TrimSpace(bytes.TrimPrefix(bytes.TrimSpace(data), []byte("\xEF\xBB\xBF")))
	return len(t) > 0 && (t[0] == '{' || t[0] == '[')
}

// textHoldsSecretKey reports whether text holds a secret-named key
// occurrence: a quoted name, optional whitespace, a colon. It is the reading
// for text the token walk could not finish, so it does not trust the walk's
// quote pairing: it pairs quotes backward from each colon, and a stray quote
// earlier in the text cannot shift the pairing past a key.
func textHoldsSecretKey(rest []byte) bool {
	for i, c := range rest {
		if c != ':' {
			continue
		}
		j := i - 1
		for j >= 0 && (rest[j] == ' ' || rest[j] == '\t' || rest[j] == '\n' || rest[j] == '\r') {
			j--
		}
		if j < 0 || rest[j] != '"' || backslashEscaped(rest, j) {
			continue
		}
		k := j - 1
		for k >= 0 && (rest[k] != '"' || backslashEscaped(rest, k)) {
			k--
		}
		if SecretFieldName(unquoteKey(rest[k+1 : j])) {
			return true
		}
	}
	return false
}

func backslashEscaped(b []byte, i int) bool {
	n := 0
	for i--; i >= 0 && b[i] == '\\'; i-- {
		n++
	}
	return n%2 == 1
}

func unquoteKey(raw []byte) string {
	var s string
	q := append(append([]byte{'"'}, raw...), '"')
	if json.Unmarshal(q, &s) == nil {
		return s
	}
	return string(raw)
}

func redact(v any, under bool) any {
	switch x := v.(type) {
	case map[string]any:
		for k, c := range x {
			x[k] = redact(c, under || SecretFieldName(k))
		}
		return x
	case []any:
		for i, c := range x {
			x[i] = redact(c, under)
		}
		return x
	case string:
		if under && x != "" {
			return SecretFieldWithheld
		}
	case json.Number:
		if under {
			return SecretFieldWithheld
		}
	}
	return v
}
