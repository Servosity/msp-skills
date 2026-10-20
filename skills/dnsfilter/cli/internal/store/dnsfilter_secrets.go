// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored (handfixes.json: secrets-never-persisted). A regen preserves
// this file by construction; the one hook into generated store.go is the
// ScrubPersistedSecrets call in mergeIncomingResourceData.

package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

// withheldRow is what reaches the store when a row cannot be decoded, so its
// keys cannot be checked: an empty object, never the raw bytes.
var withheldRow = json.RawMessage(`{}`)

// ScrubPersistedSecrets returns data with every secret-named key removed, at
// any depth of nested objects and arrays, before a row reaches SQLite (the
// generic resources blob, its FTS index, and every typed table, which all
// write the value mergeIncomingResourceData returns).
//
// DNSFilter GET responses carry live credentials in synced resources:
// Network.secret_key (every networks list and show) and ApiKey.token (the
// api-keys list, and the full token in a create response that is written
// through). The CLI command that fetched them still prints them; the local
// mirror never keeps them.
//
// Matching is by key name on the decoded JSON (see IsSecretKey), never by
// string search. Data with no secret key is returned byte-for-byte. Data that
// is not a single decodable JSON value is withheld whole (fail closed).
func ScrubPersistedSecrets(data json.RawMessage) json.RawMessage {
	if len(bytes.TrimSpace(data)) == 0 {
		return data
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return withheldRow
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return withheldRow
	}
	cleaned, removed := scrubSecretValue(value)
	if !removed {
		return data
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(cleaned); err != nil {
		return withheldRow
	}
	return json.RawMessage(bytes.TrimRight(buf.Bytes(), "\n"))
}

// scrubSecretValue drops secret-named keys from every object reachable from
// value. A secret-named key whose value is a boolean or null is kept: it
// carries no secret and may be a flag the API reports.
func scrubSecretValue(value any) (any, bool) {
	switch v := value.(type) {
	case map[string]any:
		removed := false
		for key, child := range v {
			if IsSecretKey(key) {
				switch child.(type) {
				case nil, bool:
				default:
					delete(v, key)
					removed = true
					continue
				}
			}
			cleaned, childRemoved := scrubSecretValue(child)
			if childRemoved {
				v[key] = cleaned
				removed = true
			}
		}
		return v, removed
	case []any:
		removed := false
		for i, child := range v {
			cleaned, childRemoved := scrubSecretValue(child)
			if childRemoved {
				v[i] = cleaned
				removed = true
			}
		}
		return v, removed
	}
	return value, false
}

// secretKeyFragments match anywhere in a key once it is folded to lower-case
// letters and digits, so secret_key, client_secret, ep-grd-subscriber-secret,
// new_password, reset_password_url, private_key and X-Api-Key all match.
var secretKeyFragments = []string{
	"secret",
	"password",
	"passwd",
	"passphrase",
	"privatekey",
	"apikey",
	"sharedkey",
	"credential",
}

// secretKeySuffixes match at the end of a folded key: token, access_token,
// payment_token, refresh_token. token_type, last_four and counters such as
// total_tokens do not match.
var secretKeySuffixes = []string{
	"token",
}

// identifierWords end a key that names a reference to a secret-bearing
// object rather than the secret itself: api_keys_id (the parent key every
// revoke row carries), secretKeyId, token_uuid.
var identifierWords = map[string]bool{"id": true, "ids": true, "uuid": true, "uuids": true}

// IsSecretKey reports whether a JSON object key names a secret. Case, '_',
// '-' and any other non-alphanumeric separators are ignored; a key whose last
// word is id, ids, uuid or uuids is an identifier, not a secret.
func IsSecretKey(key string) bool {
	words := keyWords(key)
	if len(words) == 0 || identifierWords[words[len(words)-1]] {
		return false
	}
	folded := strings.Join(words, "")
	for _, f := range secretKeyFragments {
		if strings.Contains(folded, f) {
			return true
		}
	}
	for _, s := range secretKeySuffixes {
		if strings.HasSuffix(folded, s) {
			return true
		}
	}
	return false
}

// keyWords splits a key into lower-case words on every non-alphanumeric rune
// and on lower-to-upper camelCase boundaries: "apiKeyId" and "api_key_id"
// both give [api key id].
func keyWords(key string) []string {
	var words []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			words = append(words, cur.String())
			cur.Reset()
		}
	}
	var prev rune
	for _, r := range key {
		switch {
		case r >= 'A' && r <= 'Z':
			if (prev >= 'a' && prev <= 'z') || (prev >= '0' && prev <= '9') {
				flush()
			}
			cur.WriteRune(r + ('a' - 'A'))
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			cur.WriteRune(r)
		default:
			flush()
		}
		prev = r
	}
	flush()
	return words
}
