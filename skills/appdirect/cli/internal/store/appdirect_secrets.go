// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package store

// appdirect handfix (store-never-keeps-secrets): every store write passes
// mergeIncomingResourceData (sync batches, typed upserts, the write-through
// cache and the keep-richer merge), which returns withoutStoredSecrets of the
// record it is about to write: values under credential field names (API key
// secrets, passwords, tokens) are replaced by "[redacted]" at any depth. A
// record that cannot be decoded is stored as {}. Every statement then passes
// the shared credential kit's SQLite driver (internal/kitsql), which refuses
// one that repeats a held credential; redacting first keeps a record with a
// credential field storable.

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
)

func withoutStoredSecrets(data json.RawMessage) json.RawMessage {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return json.RawMessage("{}")
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(redactSecretFieldsIn(v)); err != nil {
		return json.RawMessage("{}")
	}
	return json.RawMessage(bytes.TrimRight(buf.Bytes(), "\n"))
}

// secretFieldPattern matches field names that carry credentials, read on the
// normalized name (normalizedFieldName: lowercase, letters and digits only, so
// clientSecret, client_secret, client-secret and CLIENT-SECRET are one name):
// OAuth access and refresh tokens, client secrets, passwords, and the generic
// credential names (private keys, API keys, pass phrases, communities), and
// the standard auth carriers (Authorization and Proxy-Authorization, the
// authenticate challenges, cookies, sessions, bearer and JWT values,
// signatures, assertions, one-time codes). It guards the local store. The
// public half of a credential pair (clientId, accessKeyId) counts too. It
// deliberately over-matches ("token", "apikey", "session"); the AppDirect API
// names it matches are its real credential fields (password, client_secret,
// secret_key, mfa_code, the token pair) plus the exemptions below
// (TestStoredRecordKeepsNoCredentialField). Codex r1 (4.34.2 reprint) class
// holder-incomplete: "authorization" was not a credential name, so a response
// credential under it was neither held nor redacted. Codex r1 (4.34.2 appdirect
// review) class holder-incomplete: a PIN, passcode or verification code (often
// a number) is a credential too; no AppDirect spec name matches them.
var secretFieldPattern = regexp.MustCompile(`(passw(or)?d|pass$|passhash|passphrase|secret|credential|token|apikey|privatekey|community|^key$|licensekey|^psk$|preshared|clientid|accesskeyid|authoriz|authenticat|^auth$|^xauth|cookie|session|bearer|jwt|signature|^sig$|assertion|saml|^otp$|^totp$|^mfa|^pin$|pincode|passcode|verificationcode)`)

// publicSecretLikeFields match the pattern but are data, not a credential
// (normalized names; appdirect handfix credential-names-match-appdirect-data).
// The AppDirect client ID is half of the client-credentials pair, so clientId
// stays a credential name. Exempt:
//   - remote_session: the remote assistance connection URL the operator opens
//     (remote-session-assistance-discovery); withholding it would break that
//     workflow, and the terminal is where it belongs.
//   - authentication: the CVSS v2 metric on vulnerability records (NONE,
//     SINGLE, MULTIPLE).
//   - session_timeout: a user/security setting (seconds).
//   - token_type: always "Bearer" on the token response.
//   - token_expiry: the credential store's record of when the minted token
//     expires (a timestamp). Held, it withheld every output carrying a time
//     close to it (a feedback entry's own timestamp spelled it).
var publicSecretLikeFields = map[string]bool{
	"passwordcomplexity": true,
	"remotesession":      true,
	"authentication":     true,
	"sessiontimeout":     true,
	"tokentype":          true,
	"tokenexpiry":        true,
}

// secretFieldName reports a property, flag, parameter or header name that
// looks like a credential, in any spelling.
func secretFieldName(name string) bool {
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

// redactedValue replaces a credential-bearing value.
const redactedValue = "[redacted]"

// redactSecretFieldsIn walks a decoded JSON value and masks every value under a
// credential name (containers included). Null and booleans are kept: neither
// can be a credential. A number is a credential too (a one-time code, a PIN:
// Codex r1 4.34.2 class holder-incomplete) unless its name reads as a measure
// (numericMeasureName: "total_tokens", "passwordMinLength", "sessionCount").
func redactSecretFieldsIn(v any) any {
	return redactSecretFields(v, map[string]bool{})
}

// redactSecretFields memoises secretFieldName per name: a large collection
// repeats the same few dozen keys, and the regex dominates the walk.
func redactSecretFields(v any, seen map[string]bool) any {
	isSecret := func(k string) bool {
		hit, ok := seen[k]
		if !ok {
			hit = secretFieldName(k)
			seen[k] = hit
		}
		return hit
	}
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if isSecret(k) {
				switch val.(type) {
				case nil, bool:
				case float64, json.Number, int, int64, uint64:
					if !numericMeasureName(k) {
						t[k] = redactedValue
						continue
					}
				default:
					t[k] = redactedValue
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

// measureWords are the words of a field name that make a number under it a
// measure (a count, a size, a limit, a time) or an identifier, not a
// credential, although the name reads as one ("total_tokens",
// "passwordMinLength", "tokenTTL", "token_expires_at", AppDirect's
// keychainCredentialsId). The name is split into its words first, so "admin"
// is not "min".
var measureWords = map[string]bool{
	"id": true, "ids": true, "identifier": true, "uuid": true, "guid": true, "number": true, "num": true, "no": true,
	"ref": true, "type": true, "kind": true, "status": true, "state": true, "level": true, "mode": true, "port": true,
	"priority": true, "balance": true, "quota": true, "lifetime": true, "interval": true, "period": true,
	"count": true, "counts": true, "total": true, "totals": true, "length": true, "len": true, "size": true,
	"age": true, "ttl": true, "timeout": true, "duration": true, "seconds": true, "secs": true, "sec": true,
	"minutes": true, "mins": true, "hours": true, "days": true, "ms": true, "millis": true, "max": true,
	"min": true, "maximum": true, "minimum": true, "limit": true, "limits": true, "version": true,
	"attempts": true, "attempt": true, "retries": true, "retry": true, "index": true, "position": true,
	"rank": true, "score": true, "percent": true, "ratio": true, "rate": true, "remaining": true, "used": true,
	"created": true, "updated": true, "changed": true, "issued": true, "expires": true, "expiry": true,
	"expiration": true, "expired": true, "modified": true, "date": true, "time": true, "timestamp": true,
	"epoch": true, "since": true, "until": true, "last": true, "start": true, "end": true, "at": true, "in": true,
}

// numericMeasureName reports a field name a number under which is not a
// credential: one with a measure or identifier word in it, or one whose last
// word is a plural ("tokens", "sessions", "credentials": a count of them; the
// rate limiter's ledger writes "tokens": 0).
func numericMeasureName(name string) bool {
	words := nameWords(name)
	for _, w := range words {
		if measureWords[w] {
			return true
		}
	}
	if n := len(words); n > 0 {
		last := words[n-1]
		return len(last) > 3 && strings.HasSuffix(last, "s") && !strings.HasSuffix(last, "ss")
	}
	return false
}

// nameWords splits a field name into lowercase words at separators, digits
// and camelCase boundaries ("passwordMinLength" -> password, min, length;
// "tokenTTLSeconds" -> token, ttl, seconds).
func nameWords(name string) []string {
	var words []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			words = append(words, strings.ToLower(string(cur)))
			cur = cur[:0]
		}
	}
	rs := []rune(name)
	isUpper := func(r rune) bool { return 'A' <= r && r <= 'Z' }
	isLower := func(r rune) bool { return 'a' <= r && r <= 'z' }
	for i, r := range rs {
		switch {
		case isUpper(r):
			if len(cur) > 0 && (isLower(cur[len(cur)-1]) || (i+1 < len(rs) && isLower(rs[i+1]) && isUpper(cur[len(cur)-1]))) {
				flush()
			}
			cur = append(cur, r)
		case isLower(r):
			cur = append(cur, r)
		default:
			flush()
		}
	}
	flush()
	return words
}
