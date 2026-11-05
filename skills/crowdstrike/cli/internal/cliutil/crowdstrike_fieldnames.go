// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

// crowdstrike handfix (store-never-keeps-secrets, config-file-holds-no-credential):
// the one rule for which field names carry credentials, shared by the local
// store, the fleet store and the saved config, so they can never disagree.
// Which values are held, and every sink, is the shared credential kit's
// (handfixes.json credkit-wiring).

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
)

// secretFieldPattern matches field names that carry credentials, read on the
// normalized name (normalizedFieldName: lowercase, letters and digits only, so
// clientSecret, client_secret, client-secret and CLIENT-SECRET are one name):
// OAuth access and refresh tokens, client secrets, passwords, the generic
// credential names (private keys, API keys, pass phrases, communities), the
// standard auth carriers (Authorization, cookies, sessions, bearer and JWT
// values, signatures, assertions) and one-time codes. The public half of a
// credential pair (clientId, accessKeyId) counts too. It deliberately
// over-matches ("token", "apikey", "session").
var secretFieldPattern = regexp.MustCompile(`(passw(or)?d|pass$|passhash|passphrase|secret|credential|token|apikey|privatekey|community|^key$|licensekey|^psk$|preshared|clientid|accesskeyid|authoriz|authenticat|^auth$|^xauth|cookie|session|bearer|jwt|signature|^sig$|assertion|saml|^otp$|^totp$|^mfa|^pin$|pincode|passcode|verificationcode)`)

// publicSecretLikeFields match the pattern but are data, not a credential
// (normalized names): the remote assistance connection URL
// (remote_session), the CVSS v2 authentication metric, a session timeout
// setting, the token response's token_type and the credential store's
// token_expiry timestamp.
var publicSecretLikeFields = map[string]bool{
	"passwordcomplexity": true,
	"remotesession":      true,
	"authentication":     true,
	"sessiontimeout":     true,
	"tokentype":          true,
	"tokenexpiry":        true,
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

// RedactedValue replaces a credential-bearing value.
const RedactedValue = "[redacted]"

// RedactSecretFields walks a decoded JSON value and masks every value under a
// credential name (containers included). Null and booleans are kept: neither
// can be a credential. A number is a credential too (a one-time code, a PIN)
// unless its name reads as a measure (numericMeasureName: "total_tokens",
// "passwordMinLength", "sessionCount").
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
				case nil, bool:
				case float64, json.Number, int, int64, uint64:
					if !numericMeasureName(k) {
						t[k] = RedactedValue
						continue
					}
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

// measureWords are the words of a field name that make a number under it a
// measure (a count, a size, a limit, a time) or an identifier, not a
// credential, although the name reads as one ("total_tokens",
// "passwordMinLength", "tokenTTL", CrowdStrike's keychainCredentialsId). The
// name is split into its words first, so "admin" is not "min".
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
// word is a plural ("tokens", "sessions": a count of them).
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
// and camelCase boundaries ("passwordMinLength" -> password, min, length).
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

// authSchemes are the Authorization schemes whose value is a credential.
var authSchemes = []string{"bearer ", "basic ", "token ", "digest ", "negotiate ", "ntlm ", "apikey ", "sso-key "}

// CredentialShaped reports a value that is a credential by its shape whatever
// it is named: an auth-scheme value ("Bearer x", "Basic x", ...) or a JWT.
func CredentialShaped(v string) bool {
	t := strings.TrimSpace(v)
	for _, scheme := range authSchemes {
		if len(t) > len(scheme) && strings.EqualFold(t[:len(scheme)], scheme) && strings.TrimSpace(t[len(scheme):]) != "" {
			return true
		}
	}
	return LooksLikeJWT(t)
}

// URLWithoutCredentials returns raw without the credentials a URL setting can
// carry (crowdstrike handfix config-file-holds-no-credential): its userinfo,
// each query parameter whose name reads as a credential or whose value
// contains one of held (the configuration's credentials, as written), and a
// fragment that contains one. changed reports a removal. A URL that does not
// parse and has userinfo or one of held comes back "" (the caller must not
// keep any of it). The kit holds the password and every query value of each
// URL setting from startup (Declarations.BaseURL and URLEnv).
func URLWithoutCredentials(raw string, held ...string) (clean string, changed bool) {
	carries := func(v string) bool {
		for _, h := range held {
			if h != "" && strings.Contains(v, h) {
				return true
			}
		}
		return false
	}
	s := strings.TrimSpace(raw)
	if s == "" {
		return raw, false
	}
	u, err := url.Parse(s)
	if err != nil {
		authority := s
		if _, after, ok := strings.Cut(s, "://"); ok {
			authority = after
		}
		if i := strings.IndexAny(authority, "?#"); i >= 0 {
			authority = authority[:i]
		}
		if strings.Contains(authority, "@") || carries(s) {
			return "", true
		}
		return raw, false
	}
	if u.User != nil {
		u.User = nil
		changed = true
	}
	if u.RawQuery != "" {
		var kept []string
		for _, pair := range strings.FieldsFunc(u.RawQuery, func(r rune) bool { return r == '&' || r == ';' }) {
			k, v, _ := strings.Cut(pair, "=")
			dk, dv := k, v
			if d, derr := url.QueryUnescape(k); derr == nil {
				dk = d
			}
			if d, derr := url.QueryUnescape(v); derr == nil {
				dv = d
			}
			if SecretFieldName(dk) || carries(v) || carries(dv) {
				changed = true
				continue
			}
			kept = append(kept, pair)
		}
		u.RawQuery = strings.Join(kept, "&")
	}
	if u.Fragment != "" && carries(u.Fragment) {
		u.Fragment, u.RawFragment = "", ""
		changed = true
	}
	if !changed {
		return raw, false
	}
	return u.String(), true
}
