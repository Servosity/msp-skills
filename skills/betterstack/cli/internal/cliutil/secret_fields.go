// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

// Hand-written (NOT generated; handfixes.json store-never-keeps-secrets and
// spec-names-pinned): the one rule for which field names carry credentials.
// The local store masks a value under such a name before any statement
// carries it, an MCP write preview masks it, and a run profile refuses a flag
// of such a name. Holding a credential and withholding output that carries
// one is the credential kit's job (internal/credkit, wired in
// internal/credwire); this rule reads names, never values, and scans no free
// text.

import (
	"encoding/json"
	"regexp"
	"strings"
)

// secretFieldPattern matches field names that carry credentials, read on the
// normalized name (normalizedFieldName: lowercase, letters and digits only, so
// clientSecret, client_secret, client-secret and CLIENT-SECRET are one name).
// It deliberately over-matches. The Better Stack spec names it matches are
// pinned by TestNoBetterstackSpecNameReadsAsACredential: the Authorization
// header and an escalation policy's incident_token.
var secretFieldPattern = regexp.MustCompile(`(passw(or)?d|pass$|passhash|passphrase|secret|credential|token|apikey|privatekey|community|^key$|licensekey|^psk$|preshared|clientid|accesskeyid|authoriz|authenticat|^auth$|^xauth|cookie|session|bearer|jwt|signature|^sig$|assertion|saml|^otp$|^totp$|^mfa|secure|sensitive)`)

// credentialContainerNames are normalized names whose whole value is held:
// a monitor's request_headers, a list of {name, value} pairs whose values
// carry the probed site's own credentials (an Authorization or X-Api-Key
// header). The credential kit holds every member beneath it (internal/credwire
// ResponseFields), so the store and a preview mask it whole.
var credentialContainerNames = map[string]bool{
	"requestheaders": true,
}

// publicSecretLikeFields match the pattern but are metadata, not a credential
// (normalized names).
var publicSecretLikeFields = map[string]bool{
	"passwordcomplexity": true,
}

// countLikeName matches the credential-shaped names whose NUMBER is a count,
// a size or a duration, never a credential (total_tokens, sessionTimeout).
var countLikeName = regexp.MustCompile(`(count|counts|total|totals|tokens|sessions|limit|length|size|ttl|timeout|expiresin|expires|expiry|lifetime|remaining|used|seconds|minutes|hours|days)$`)

var countLeadWords = map[string]bool{"total": true, "num": true, "number": true, "max": true, "maximum": true, "min": true, "minimum": true}

// SecretFieldName reports a property, flag, parameter or header name that
// reads as a credential, in any spelling.
func SecretFieldName(name string) bool {
	n := normalizedFieldName(name)
	if publicSecretLikeFields[n] {
		return false
	}
	return secretFieldPattern.MatchString(n) || credentialContainerNames[n] || credentialIdentityName(n)
}

// identityQualifiers mark a user or login name as the identity half of a
// credential pair (auth_user, basic_auth_user, proxy_user, db_login).
var identityQualifiers = []string{
	"auth", "basic", "proxy", "https", "http", "login", "bind", "database", "db", "smtp", "sftp", "ftp",
	"ldap", "service", "svc", "admin", "api", "client",
}

// credentialIdentityName reports the identity half of a credential pair (the
// public half beside a password, as clientId is beside a client secret), read
// on the normalized name alone, so every spelling of one name is one answer:
// a name ending in "username" (a monitor's auth_username, authUsername,
// AUTH_USERNAME), a bare user or login, and a user or login made of
// qualifier words only (authUser, auth_user, AUTHUSER, basicauthuser,
// proxyuser, dblogin). Names that only mention users (on_call_users,
// user_count, user_id, last_login_at, acknowledged_by) stay visible.
func credentialIdentityName(n string) bool {
	if n == "user" || n == "login" || strings.HasSuffix(n, "username") {
		return true
	}
	for _, stem := range []string{"user", "login"} {
		if rest, ok := strings.CutSuffix(n, stem); ok && rest != "" && qualifierWords(rest) {
			return true
		}
	}
	return false
}

// qualifierWords reports whether s is one or more identityQualifiers written
// together.
func qualifierWords(s string) bool {
	if s == "" {
		return true
	}
	for _, q := range identityQualifiers {
		if rest, ok := strings.CutPrefix(s, q); ok && qualifierWords(rest) {
			return true
		}
	}
	return false
}

// NumberIsCredential reports whether a number under the member name is a
// credential: the name reads as one and not as a count, size or duration.
func NumberIsCredential(name string) bool {
	if !SecretFieldName(name) || countLikeName.MatchString(normalizedFieldName(name)) {
		return false
	}
	w := fieldWords(name)
	return !(len(w) > 1 && countLeadWords[w[0]])
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

// fieldWords splits a name into lowercase words at separators and case
// changes (NumericPassword -> numeric, password; maxSessions -> max, sessions).
func fieldWords(name string) []string {
	var words []string
	var cur []byte
	flush := func() {
		if len(cur) > 0 {
			words = append(words, strings.ToLower(string(cur)))
			cur = cur[:0]
		}
	}
	isUpper := func(c byte) bool { return 'A' <= c && c <= 'Z' }
	isLower := func(c byte) bool { return 'a' <= c && c <= 'z' }
	for i := 0; i < len(name); i++ {
		c := name[i]
		alnum := isUpper(c) || isLower(c) || ('0' <= c && c <= '9') || c >= 0x80
		if !alnum {
			flush()
			continue
		}
		if isUpper(c) && len(cur) > 0 && (isLower(cur[len(cur)-1]) || (i+1 < len(name) && isLower(name[i+1]) && isUpper(cur[len(cur)-1]))) {
			flush()
		}
		cur = append(cur, c)
	}
	flush()
	return words
}

// RedactedValue replaces a credential-bearing value.
const RedactedValue = "[redacted]"

// RedactSecretFields walks a decoded JSON value and masks every value under a
// credential name (containers included). Null and booleans are kept; a number
// is masked unless the name reads as a count, size or duration.
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
				case json.Number, float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
					if NumberIsCredential(k) {
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
