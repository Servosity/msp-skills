// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

// vsphere handfix (store-never-keeps-secrets, config-save-no-credential-outside-store):
// the one rule for which field names carry credentials, shared by the local
// store, the saved config and the credential kit's startup holds, so they can
// never disagree. Which values are held, and every sink, is the kit's.

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
)

// secretFieldPattern matches field names that carry credentials, read on the
// normalized name (normalizedFieldName: lowercase, letters and digits only, so
// clientSecret, client_secret, client-secret and CLIENT-SECRET are one name):
// OAuth access and refresh tokens, client secrets, passwords, and the generic
// credential names (private keys, API keys, pass phrases, communities). No
// vSphere property the CLI reads matches it; it guards the config,
// request params, MCP arguments and any body that echoes a credential. The
// public half of a credential pair (clientId, accessKeyId) counts too. It
// deliberately over-matches ("token", "apikey").
var secretFieldPattern = regexp.MustCompile(`(passw(or)?d|pass$|passhash|passphrase|secret|credential|token|apikey|privatekey|community|^key$|licensekey|^psk$|preshared|clientid|accesskeyid)`)

// publicSecretLikeFields match the pattern but are metadata, not a credential
// (normalized names).
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

// credentialNameWords are the parts of a header, parameter or key name that
// mark a credential SecretFieldName does not name (normalized names):
// Authorization and Proxy-Authorization, Cookie, session ids, request
// signatures, bearer and JWT values, SAML and CAS tickets, CSRF tokens.
var credentialNameWords = []string{"auth", "cookie", "session", "signature", "bearer", "jwt", "apikey", "saml", "ticket", "csrf", "xsrf"}

// credentialNameExact are short names that carry a credential only whole.
var credentialNameExact = map[string]bool{"sig": true, "sid": true, "pw": true, "pwd": true, "otp": true}

// CredentialName reports a header, query-parameter or field name that carries
// a credential, in any spelling: every SecretFieldName, and the names in
// credentialNameWords and credentialNameExact. It deliberately over-matches
// (X-Author is a credential name here).
func CredentialName(name string) bool {
	if SecretFieldName(name) {
		return true
	}
	n := normalizedFieldName(name)
	if credentialNameExact[n] {
		return true
	}
	for _, w := range credentialNameWords {
		if strings.Contains(n, w) {
			return true
		}
	}
	return false
}

// StripURLCredentials returns raw without the credentials a URL can carry:
// its userinfo, every query parameter whose name is a CredentialName, and a
// fragment that carries one. A URL that does not parse keeps only its scheme and what follows
// its last "@" up to any "?" or "#" (the userinfo may hold any delimiter, and
// the query and fragment are dropped: their parameters cannot be read safely). Text without "://" is returned
// unchanged; the credential kit judges the result where it is written.
func StripURLCredentials(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || !strings.Contains(trimmed, "://") {
		return raw
	}
	u, err := url.Parse(trimmed)
	if err != nil || u.Opaque != "" {
		scheme, rest, _ := strings.Cut(trimmed, "://")
		if at := strings.LastIndex(rest, "@"); at >= 0 {
			rest = rest[at+1:]
		}
		if j := strings.IndexAny(rest, "?#"); j >= 0 {
			rest = rest[:j]
		}
		return scheme + "://" + rest
	}
	u.User = nil
	u.RawQuery = keepNonCredentialParams(u.RawQuery)
	u.ForceQuery = false
	if frag := u.EscapedFragment(); frag != "" && (paramsCarryCredential(frag) || URLCarriesCredential(frag)) {
		u.Fragment, u.RawFragment = "", ""
	}
	return u.String()
}

// keepNonCredentialParams drops every name=value pair (or bare name) whose
// name, as written or unescaped, is a CredentialName, keeping the rest as
// written and in order.
func keepNonCredentialParams(raw string) string {
	if raw == "" {
		return ""
	}
	var kept []string
	for _, pair := range strings.FieldsFunc(raw, func(r rune) bool { return r == '&' || r == ';' }) {
		k, _, _ := strings.Cut(pair, "=")
		dk, err := url.QueryUnescape(k)
		if err != nil {
			dk = k
		}
		if CredentialName(k) || CredentialName(dk) {
			continue
		}
		kept = append(kept, pair)
	}
	return strings.Join(kept, "&")
}

// paramsCarryCredential reports a name=value pair (split on ?, #, & and ;)
// whose name, as written or query-unescaped, is a CredentialName.
func paramsCarryCredential(s string) bool {
	for _, pair := range strings.FieldsFunc(s, func(r rune) bool { return r == '&' || r == ';' || r == '?' || r == '#' }) {
		k, _, ok := strings.Cut(pair, "=")
		if !ok {
			continue
		}
		if CredentialName(k) {
			return true
		}
		if dk, err := url.QueryUnescape(k); err == nil && CredentialName(dk) {
			return true
		}
	}
	return false
}

// urlViews is text as written and as up to three rounds of query- and
// path-unescaping show it, so a nested or double-encoded URL is read too.
func urlViews(text string) []string {
	views := []string{text}
	seen := map[string]bool{text: true}
	for i := 0; i < len(views) && len(views) < 16; i++ {
		for _, dec := range []func(string) (string, error){url.QueryUnescape, url.PathUnescape} {
			if d, err := dec(views[i]); err == nil && !seen[d] {
				seen[d] = true
				views = append(views, d)
			}
		}
	}
	return views
}

// urlSchemeSep is how a URL's scheme separator is written as text: plain, and
// with its slashes JSON-escaped.
var urlSchemeSep = []string{"://", `:\/\/`}

// URLCarriesCredential reports whether any URL in text (as written, or
// percent-decoded, so a URL nested in a parameter counts) carries a credential
// of its own: userinfo (an "@" before the first "/", so a raw "?" or "#" in a
// password is covered, or after a ":" before the query, so a raw "/" is), or a
// parameter, in a query or a fragment, whose name is a CredentialName. Fail
// closed: an "@" in a query with no "/" before it, or after a port in a path,
// counts too.
func URLCarriesCredential(text string) bool {
	for _, view := range urlViews(text) {
		for _, sep := range urlSchemeSep {
			rest := view
			for {
				i := strings.Index(rest, sep)
				if i < 0 {
					break
				}
				rest = rest[i+len(sep):]
				end := strings.IndexFunc(rest, func(r rune) bool {
					return r == ' ' || r == '\t' || r == '\n' || r == '\r' || strings.ContainsRune("\"'`<>", r)
				})
				if end < 0 {
					end = len(rest)
				}
				target := rest[:end]
				// The authority as a parser that stops only at "/" reads it, so
				// a raw "?" or "#" inside a password is covered.
				head := target
				if j := strings.IndexByte(head, '/'); j >= 0 {
					head = head[:j]
				}
				if strings.Contains(head, "@") {
					return true
				}
				hierarchy := target
				if j := strings.IndexAny(hierarchy, "?#"); j >= 0 {
					hierarchy = hierarchy[:j]
				}
				if at := strings.LastIndex(hierarchy, "@"); at >= 0 && strings.Contains(hierarchy[:at], ":") {
					return true
				}
				if j := strings.IndexAny(target, "?#"); j >= 0 && paramsCarryCredential(target[j+1:]) {
					return true
				}
			}
		}
	}
	return false
}
