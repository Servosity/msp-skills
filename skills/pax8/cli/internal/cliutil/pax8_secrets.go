// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

// pax8 handfix (store-never-keeps-secrets): the one rule for which
// field names carry credentials, shared by output redaction and the local
// store, so the two can never disagree.

import (
	"encoding/base64"
	"encoding/json"
	"github.com/pelletier/go-toml/v2"
	"net/url"
	"os"
	"regexp"
	"strings"
)

// secretFieldPattern matches field names that carry credentials, read on the
// normalized name (normalizedFieldName: lowercase, letters and digits only, so
// clientSecret, client_secret, client-secret and CLIENT-SECRET are one name):
// OAuth access and refresh tokens, client secrets, passwords, and the generic
// credential names (private keys, API keys, pass phrases, communities), and
// the standard auth carriers (Authorization and Proxy-Authorization, the
// authenticate challenges, cookies, sessions, bearer and JWT values,
// signatures, assertions, one-time codes). It guards the config,
// request params, MCP arguments and any body that echoes a credential. The
// public half of a credential pair (clientId, accessKeyId) counts too. It
// deliberately over-matches ("token", "apikey", "session"); no Pax8 Partner
// API property or parameter name matches it except provisioning "key"
// (TestNoPax8SpecNameReadsAsACredential). Codex r1 (4.34.2 reprint) class
// holder-incomplete: "authorization" was not a credential name, so a response
// credential under it was neither held nor redacted.
var secretFieldPattern = regexp.MustCompile(`(passw(or)?d|pass$|passhash|passphrase|secret|credential|token|apikey|privatekey|community|^key$|licensekey|^psk$|preshared|clientid|accesskeyid|authoriz|authenticat|^auth$|^xauth|cookie|session|bearer|jwt|signature|^sig$|assertion|saml|^otp$|^totp$|^mfa)`)

// publicSecretLikeFields match the pattern but are metadata, not a credential
// (normalized names). A Pax8 client ID is half of the client-credentials pair
// and never appears in Partner API data, so clientId stays a credential name.
var publicSecretLikeFields = map[string]bool{
	"passwordcomplexity": true,
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

// isMaskValue reports a value that already hides a credential: a "****"
// mask, RedactedValue, the funnel's notices, and placeholder words
// such as the learn journal's "redacted" argv class.
func isMaskValue(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" || strings.Trim(v, "*•") == "" || FunnelWithheld(v) {
		return true
	}
	switch strings.ToLower(strings.Trim(v, "[]<>()")) {
	case "redacted", "masked", "hidden", "withheld":
		return true
	}
	return false
}

// HoldCredentialValues is the one walker that registers credentials found in
// structured data with the holder (pax8 handfix: output-chokepoint): every
// string, of any length, under a credential-named key at any depth (all
// strings under a credential-named object or array), except masks. A string
// that is itself a JSON object or array is read too. Request bodies and
// params, responses, MCP tool arguments and output members all use it.
func HoldCredentialValues(v any) {
	holdCredentialValues(v, false)
}

func holdCredentialValues(v any, secret bool) {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			holdCredentialValues(child, secret || SecretFieldName(k))
		}
	case []any:
		for _, child := range t {
			holdCredentialValues(child, secret)
		}
	case string:
		if secret && !isMaskValue(t) {
			FunnelHold(t)
			holdCredentialParts(t)
		}
		// A value shaped like a credential is one under any name (Codex r1
		// class holder-incomplete): an auth-scheme value ("Bearer x",
		// "Basic x", ...) and a JWT.
		if !secret {
			holdCredentialShapedValue(t)
		}
		if trimmed := strings.TrimSpace(t); len(trimmed) > 1 && (trimmed[0] == '{' || trimmed[0] == '[') {
			var inner any
			if json.Unmarshal([]byte(trimmed), &inner) == nil {
				holdCredentialValues(inner, secret)
			}
		}
	}
}

// authSchemes are the Authorization schemes whose value is a credential.
var authSchemes = []string{"bearer ", "basic ", "token ", "digest ", "negotiate ", "ntlm ", "apikey ", "sso-key "}

// holdCredentialParts holds the pieces a credential-named value carries
// besides itself: the credential after an auth scheme word, and each value of
// a cookie jar or parameter list ("sid=x; theme=y", "a=x&b=y").
func holdCredentialParts(v string) {
	t := strings.TrimSpace(v)
	for _, scheme := range authSchemes {
		if len(t) > len(scheme) && strings.EqualFold(t[:len(scheme)], scheme) {
			if rest := strings.TrimSpace(t[len(scheme):]); !isMaskValue(rest) {
				FunnelHold(rest)
			}
		}
	}
	if !strings.Contains(t, "=") {
		return
	}
	for _, pair := range strings.FieldsFunc(t, func(r rune) bool { return r == ';' || r == '&' || r == ',' }) {
		if _, val, ok := strings.Cut(strings.TrimSpace(pair), "="); ok {
			if val = strings.Trim(strings.TrimSpace(val), `"`); len(val) >= shortMinLen && !isMaskValue(val) {
				FunnelHold(val)
			}
		}
	}
}

// holdCredentialShapedValue holds a value that is a credential by its shape
// whatever it is called: "<scheme> <token68>" for every auth scheme
// (authSchemes: Bearer, Token, ApiKey, ...; a single token of at least
// shortMinLen*2 characters holding a digit, or of 20 or more), "Basic
// <base64 of user:password>", and a JWT. The shapes are narrow so ordinary
// text ("Basic Mobility", "Bearer of news", "token expired") is never held.
func holdCredentialShapedValue(v string) {
	t := strings.TrimSpace(v)
	for _, scheme := range authSchemes {
		if scheme == "basic " || len(t) <= len(scheme) || !strings.EqualFold(t[:len(scheme)], scheme) {
			continue
		}
		rest := strings.TrimSpace(t[len(scheme):])
		if token68.MatchString(rest) && len(rest) >= 2*shortMinLen && (len(rest) >= 20 || strings.ContainsAny(rest, "0123456789")) && !isMaskValue(rest) {
			FunnelHold(t, rest)
		}
	}
	if len(t) > 6 && strings.EqualFold(t[:6], "basic ") {
		rest := strings.TrimSpace(t[6:])
		if dec, err := base64.StdEncoding.DecodeString(rest); err == nil && token68.MatchString(rest) {
			if user, pw, ok := strings.Cut(string(dec), ":"); ok && user != "" && pw != "" {
				FunnelHold(t, rest, string(dec), pw)
			}
		}
	}
	if LooksLikeJWT(t) {
		FunnelHold(t)
	}
}

// token68 is the RFC 7235 token68 character set (a bearer token's).
var token68 = regexp.MustCompile(`^[A-Za-z0-9\-._~+/]+=*$`)

// HoldCredentialFields decodes a JSON body and holds its credential values.
func HoldCredentialFields(body []byte) {
	if len(body) == 0 {
		return
	}
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.UseNumber()
	var v any
	if dec.Decode(&v) == nil {
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

// HoldEnvCredentials holds, before anything can print (pax8 handfix:
// output-chokepoint), every credential the environment carries for this
// connector: the value of every PAX8_* variable whose name reads as a
// credential (PAX8_CLIENT_ID, PAX8_CLIENT_SECRET, the MCP HTTP tokens, and
// any credential-named variable a later print adds), and the credentials a
// PAX8_*_URL value carries (its userinfo password and credential-named query
// values). A URL itself is not a credential (PAX8_TOKEN_URL names "token").
func HoldEnvCredentials() {
	for _, kv := range os.Environ() {
		name, _, ok := strings.Cut(kv, "=")
		if !ok || !strings.HasPrefix(name, "PAX8_") {
			continue
		}
		v := EnvOverride(name)
		switch {
		case strings.HasSuffix(name, "_URL"):
			HoldURLCredentials(v)
		case SecretFieldName(strings.TrimPrefix(name, "PAX8_")):
			HoldKeys(v)
		}
	}
}

// HoldURLCredentials holds the credentials a URL carries: the userinfo
// password and the value of every credential-named query parameter, as
// written and percent-decoded. The raw text is read first, so a URL that does
// not parse is held too (Codex r2 class output-before-chokepoint, fixed after
// the stop rule: an unparseable base_url held nothing and was printed).
func HoldURLCredentials(raw string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return
	}
	u, err := url.Parse(raw)
	// The password as written: the userinfo of the authority, after its first
	// ':'. For a URL that does not parse, the userinfo runs to the last '@'
	// before any '?' or '#' (a raw '/' in the password).
	if _, after, ok := strings.Cut(raw, "://"); ok {
		authority := after
		stop := "/?#"
		if err != nil {
			stop = "?#"
		}
		if i := strings.IndexAny(authority, stop); i >= 0 {
			authority = authority[:i]
		}
		if at := strings.LastIndex(authority, "@"); at >= 0 {
			if _, pw, ok := strings.Cut(authority[:at], ":"); ok && pw != "" {
				FunnelHold(pw)
				if dec, derr := url.PathUnescape(pw); derr == nil {
					FunnelHold(dec)
				}
			}
		}
	}
	query := ""
	if _, q, ok := strings.Cut(raw, "?"); ok {
		query, _, _ = strings.Cut(q, "#")
	}
	for _, pair := range strings.FieldsFunc(query, func(r rune) bool { return r == '&' || r == ';' }) {
		k, v, _ := strings.Cut(pair, "=")
		if dk, derr := url.QueryUnescape(k); derr == nil {
			k = dk
		}
		if !SecretFieldName(k) || v == "" {
			continue
		}
		if !isMaskValue(v) {
			FunnelHold(v)
		}
		if dv, derr := url.QueryUnescape(v); derr == nil && !isMaskValue(dv) {
			FunnelHold(dv)
		}
	}
	if err == nil && u.User != nil {
		if pw, ok := u.User.Password(); ok {
			FunnelHold(pw)
		}
	}
}

// HoldTOMLCredentials holds what a configuration or credential file carries
// the moment its bytes are read, before anything decides whether to use or
// refuse the file (pax8 handfix output-chokepoint; Codex r2 class
// output-before-chokepoint, fixed after the stop rule: a refused file's
// values were never held). Every value under a credential-named key at any
// depth, and the credentials of every URL the text holds, read from the raw
// text so a value TOML or url.Parse rejects is held too.
func HoldTOMLCredentials(data []byte) {
	text := string(data)
	for _, line := range strings.Split(text, "\n") {
		for rest := line; ; {
			i := strings.Index(rest, "://")
			if i < 0 {
				break
			}
			start := strings.LastIndexAny(rest[:i], " \t\"'=[,") + 1
			end := strings.IndexAny(rest[i:], " \t\"'],")
			if end < 0 {
				end = len(rest) - i
			}
			HoldURLCredentials(rest[start : i+end])
			rest = rest[i+end:]
		}
		if k, v, ok := strings.Cut(line, "="); ok && SecretFieldName(strings.Trim(strings.TrimSpace(k), "\"'")) {
			if v = strings.Trim(strings.TrimSpace(v), "\"'"); v != "" && !isMaskValue(v) {
				FunnelHold(v)
			}
		}
	}
	var doc map[string]any
	if toml.Unmarshal(data, &doc) == nil {
		HoldCredentialValues(doc)
	}
}

// HoldKeys holds each value as given and without a "Bearer " or "Basic "
// scheme prefix (the Basic payload is base64 of user:password, so the
// password inside it is not aligned for the funnel's base64 view).
func HoldKeys(keys ...string) {
	for _, k := range keys {
		k = strings.TrimSpace(k)
		for _, scheme := range []string{"bearer ", "basic "} {
			if len(k) > len(scheme) && strings.EqualFold(k[:len(scheme)], scheme) {
				FunnelHold(strings.TrimSpace(k[len(scheme):]))
			}
		}
		FunnelHold(k)
	}
}

// URLWithoutCredentials returns raw with every credential it carries taken
// out (pax8 handfix: config-file-holds-no-credential): the userinfo, each
// query parameter whose name reads as a credential or whose value (as written
// or decoded) repeats a held value, and a fragment that repeats one. changed
// reports a removal. A URL that does not parse and carries userinfo or a held
// value comes back "" (the caller must not keep any of it).
func URLWithoutCredentials(raw string) (clean string, changed bool) {
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
		if strings.Contains(authority, "@") || HeldEchoes(s) {
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
			if SecretFieldName(dk) || (v != "" && (HeldEchoes(v) || HeldEchoes(dv))) {
				changed = true
				continue
			}
			kept = append(kept, pair)
		}
		u.RawQuery = strings.Join(kept, "&")
	}
	if u.Fragment != "" && HeldEchoes(u.Fragment) {
		u.Fragment, u.RawFragment = "", ""
		changed = true
	}
	if !changed {
		return raw, false
	}
	return u.String(), true
}
