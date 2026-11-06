// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json maxio-credential-names-exact and
// response-credentials-redacted): which Maxio names carry a credential, and the
// one place a credential in a Maxio response is taken out of the body. Holding
// and withholding are the shared credential kit's job (internal/credwire).

package cliutil

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"regexp"
	"strings"
)

// secretFieldPattern matches a credential name on its normalized form
// (normalizedFieldName). A card's full_number and cvv, a bank account number and
// IBAN, a gateway nonce or vault reference and the generic credential names all
// read as one. The Maxio spec names it matches are pinned by
// TestNoMaxioSpecNameReadsAsACredential.
var secretFieldPattern = regexp.MustCompile(`(passw(or)?d|pass$|passhash|passphrase|secret|credential|token|apikey|privatekey|community|^key$|fullnumber$|^cvv$|^cvc$|^bankaccountnumber$|^bankiban$|^iban$|paymentmethodnonce|nonce$|licensekey|(encryption|privacy|priv|signing|shared|master)key|^psk$|preshared|clientid|accesskeyid|authoriz|authenticat|^auth$|^xauth|cookie|session|bearer|jwt|signature|^sig$|assertion|saml|^otp$|^totp$|^mfa|secure|sensitive)`)

// publicSecretLikeFields match the pattern but are metadata, not a credential
// (normalized names).
var publicSecretLikeFields = map[string]bool{
	"passwordcomplexity": true,
	"authenticationalg":  true, // the donor layer's SNMP v3 enum of algorithm names (Undefined, SHA1, MD5)
	// maxio handfix (maxio-credential-names-exact): Maxio spec names that read
	// as credentials and are not. Holding one withholds its value (an event
	// type, an ACH authorizer's name, a dedupe string, a boolean) wherever it
	// appears in output, and drops it from the store.
	"key":                           true, // Event.key: the event type (payment_success, ...)
	"authorizerfirstname":           true, // ACH agreement: the authorizing person's name
	"authorizerlastname":            true, // ACH agreement: the authorizing person's name
	"requiressecuritytoken":         true, // ChargifyJs keys: a boolean setting
	"securecheckoutpolicyurl":       true, // a public policy page URL
	"storedcredentialtransactionid": true, // PSD2 reference to an earlier gateway transaction
	"uniquenesstoken":               true, // Events-Based Billing dedupe string the caller chooses
}

// countLikeName matches the credential-shaped names whose NUMBER is a count, a
// size or a duration, never a credential.
var countLikeName = regexp.MustCompile(`(count|counts|total|totals|tokens|sessions|limit|length|size|ttl|timeout|expiresin|expires|expiry|lifetime|remaining|used|seconds|minutes|hours|days|^read|^write)$`)

var countLeadWords = map[string]bool{"total": true, "num": true, "number": true, "max": true, "maximum": true, "min": true, "minimum": true}

// NumberIsCredential reports whether a number under the member name is a
// credential: the name reads as one (SecretFieldName) and not as a count,
// size or duration (countLikeName, or a leading count word).
func NumberIsCredential(name string) bool {
	if !SecretFieldName(name) || countLikeName.MatchString(normalizedFieldName(name)) {
		return false
	}
	w := fieldWords(name)
	return !(len(w) > 1 && countLeadWords[w[0]])
}

// fieldWords splits a name into lowercase words at separators and case
// changes (NumericPassword -> numeric, password; maxSessions -> max, sessions;
// HTTPToken -> http, token).
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

// SecretFieldName reports a property, flag, parameter or header name that
// looks like a credential, in any spelling.
func SecretFieldName(name string) bool {
	n := normalizedFieldName(name)
	return !publicSecretLikeFields[n] && (secretFieldPattern.MatchString(n) || credentialIdentityName(name, n))
}

// SecretFlagName reports a command-line flag whose value is a credential
// (maxio handfix maxio-credential-names-exact; black-box class
// flag-read-by-its-whole-path). A generated body flag is the body path of the
// field it fills joined by "-" (--payment-profile-cvv fills
// payment_profile.cvv, --chargify-uniqueness-token fills
// chargify.uniqueness_token), while the rule's anchored and allowlisted names
// are leaf names. Read whole, the joined name both missed credentials
// (paymentprofilecvv is not ^cvv$; a bank account number and IBAN the same)
// and held public billing data (chargifyuniquenesstoken matches token; an ACH
// authorizer's name, a secure checkout policy URL and a stored credential
// transaction id under their containers likewise). The leaf is a word-aligned
// tail of the name:
//   - a tail that is a public Maxio name (publicSecretLikeFields, the longest
//     such tail) is the leaf, and the flag is public unless the path before it
//     reads as a credential itself: a credential reading of the whole name
//     that starts in that path (--api-key, --credentials-key), or any run of
//     the path's words that is a credential name (a credential container: its
//     every member is one);
//   - otherwise the flag is a credential when its whole name or any
//     word-aligned tail is one (SecretFieldName), so every signal the whole
//     name had is kept.
func SecretFlagName(name string) bool {
	w := fieldWords(name)
	for k := 0; k < len(w); k++ {
		if !publicSecretLikeFields[strings.Join(w[k:], "")] {
			continue
		}
		prefix := strings.Join(w[:k], "")
		if loc := secretFieldPattern.FindStringIndex(prefix + strings.Join(w[k:], "")); loc != nil && loc[0] < len(prefix) {
			return true
		}
		for i := 0; i < k; i++ {
			for j := i + 1; j <= k; j++ {
				if SecretFieldName(strings.Join(w[i:j], "-")) {
					return true
				}
			}
		}
		return false
	}
	if SecretFieldName(name) {
		return true
	}
	for k := 1; k < len(w); k++ {
		if SecretFieldName(strings.Join(w[k:], "-")) {
			return true
		}
	}
	return false
}

// identityQualifierWords mark a user or login name as the identity half of a
// credential pair (auth_user, basic_auth_user, proxy_user, db_login).
var identityQualifierWords = map[string]bool{
	"auth": true, "basic": true, "proxy": true, "http": true, "https": true, "login": true, "bind": true,
	"db": true, "database": true, "smtp": true, "ftp": true, "sftp": true, "ldap": true, "service": true,
	"svc": true, "admin": true, "api": true, "client": true,
}

// credentialIdentityName reports the identity half of a credential pair (the
// public half beside a password, the way clientId is beside a client
// secret): any name ending in "username" (auth_username, proxyUsername,
// db_username), user name and userName, a bare user or login, and a user or
// login word qualified by an auth word (auth_user, basicAuthUser, auth_login).
// Codex r1 (maxio 4.34.2 reprint) class credential-pair-identity-unheld:
// a monitor's documented auth_username (beside auth_password) matched no rule,
// so `monitors get` printed it and sync stored it. Names that only mention
// users (on_call_users, user_count, user_id, last_login_at) stay visible.
func credentialIdentityName(name, normalized string) bool {
	if normalized == "user" || normalized == "login" || strings.HasSuffix(normalized, "username") {
		return true
	}
	w := fieldWords(name)
	for i, x := range w {
		if x == "username" {
			return true
		}
		if x != "user" && x != "login" {
			continue
		}
		if i+1 < len(w) && w[i+1] == "name" {
			return true
		}
		for _, y := range w {
			if y != x && identityQualifierWords[y] {
				return true
			}
		}
	}
	return false
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

// AuthHeaderHeldParts is what of a built auth header is a credential. Maxio's
// Basic password is the documented constant x (handfixes.json
// basic-constant-password-public): held, it would withhold any output with x as a
// whole word, so a Basic header gives its encoded form, its pair and its user,
// and never the scheme word form, which the kit would split into x again. Any
// other header comes back whole.
func AuthHeaderHeldParts(header string) []string {
	scheme, rest, ok := strings.Cut(strings.TrimSpace(header), " ")
	if !ok || !strings.EqualFold(scheme, "basic") {
		return []string{header}
	}
	rest = strings.TrimSpace(rest)
	dec, err := base64.StdEncoding.DecodeString(rest)
	if err != nil {
		return []string{header}
	}
	user, pass, found := strings.Cut(string(dec), ":")
	if !found || strings.TrimSpace(pass) != "x" {
		return []string{header}
	}
	return []string{rest, string(dec), user}
}

// RedactedValue replaces a credential value in a response body.
const RedactedValue = "[redacted]"

// RedactResponseCredentials replaces every value under a credential name in a
// JSON response body (handfixes.json response-credentials-redacted), so a
// subscription's stored-card vault reference never reaches a sink and the rest
// of the record stays readable. The kit withholds a whole unit that carries a
// held value; with these out of the body an ordinary list keeps its data.
// Null and booleans are kept; a number under a count-like name is kept. A body
// that is not JSON, or has no such value, comes back byte for byte.
func RedactResponseCredentials(body []byte) []byte {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || (trimmed[0] != '{' && trimmed[0] != '[') {
		return body
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil || dec.More() {
		return body
	}
	if !redactCredentialValues(v, map[string]bool{}) {
		return body
	}
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return body
	}
	return bytes.TrimRight(out.Bytes(), "\n")
}

func redactCredentialValues(v any, seen map[string]bool) (changed bool) {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			secret, ok := seen[k]
			if !ok {
				secret = SecretFieldName(k)
				seen[k] = secret
			}
			if secret {
				switch val.(type) {
				case nil, bool:
					continue
				case json.Number:
					if !NumberIsCredential(k) {
						continue
					}
				}
				if val != RedactedValue {
					t[k] = RedactedValue
					changed = true
				}
				continue
			}
			if redactCredentialValues(val, seen) {
				changed = true
			}
		}
	case []any:
		for _, val := range t {
			if redactCredentialValues(val, seen) {
				changed = true
			}
		}
	}
	return changed
}
