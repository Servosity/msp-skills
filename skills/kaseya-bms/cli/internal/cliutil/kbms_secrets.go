// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

// kaseya-bms handfix (store-never-keeps-secrets): the one rule for which
// field names carry credentials, shared by the local store's redaction, the
// config.toml filter and the credential-endpoint rule, so they can never
// disagree. Holding credentials for output is the shared credential kit's
// (internal/credkit, handfixes.json credkit-wiring).

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"kaseya-bms-pp-cli/internal/credkit"

	"bytes"
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
// deliberately over-matches ("token", "session"); the Kaseya BMS API names it
// matches are its real credential fields (the login form's Password, MFACode,
// ClientSecret, SamlKey, SamlResponse and ItcToken; TokenOutputDto's
// AccessToken and RefreshToken; the IT Glue and VSA access tokens;
// IntegrationSettingDto.PasswordOrToken; webhook Secret; RemoteControl's
// client pair; Pulseway session Token and the remote-session AuthorizationLink;
// the FullCalendar license key) plus the exemptions below. A PIN, passcode or
// verification code (often a number) is a credential too.
var secretFieldPattern = regexp.MustCompile(`(passw(or)?d|pass$|passhash|passphrase|secret|credential|token|apikey|privatekey|community|^key$|licensekey|^psk$|preshared|clientid|accesskey|authoriz|authenticat|^auth$|^xauth|cookie|session|bearer|jwt|signature|^sig$|assertion|saml|^otp$|^totp$|^mfa|^pin$|pincode|passcode|verificationcode)`)

// publicSecretLikeFields match the pattern but are data, not a credential
// (normalized names; kaseya-bms handfix credential-names-match-bms-data,
// derived from the vendor spec, TestNoBMSSpecNameReadsAsACredential). Held,
// each would withhold every output that repeats its value anywhere (every held
// value is matched at any length), so a count or a record id would blank whole
// lists. Exempt:
//   - accesstokenexpireon, refreshtokenexpireon, tokenexpiry: when a token
//     expires (timestamps; TokenOutputDto and the credential store).
//   - tokentype: always "Bearer".
//   - hassecret, useaccesstoken, isconsolesession: booleans
//     (WebhookConfigurationOutputDto, IntegrationSettingDto, Pulseway sessions).
//   - suggestedpasswords: a count (SuggestedResourceCountOutputDto).
//   - passwordid: the IT Glue password record id in a path, not the password.
//   - sessions: the remote-session list (RemoteSessionsOutputDto); the
//     credential members inside it (Token, AuthorizationLink) stay held by name.
//   - externalclientid, appclientidentifier: QuickBooks and Kaseya One record
//     identifiers. A plain clientid stays held (RemoteControl.ClientId is the
//     public half of a client pair).
//   - skiptoken: the Kaseya One lookup paging cursor.
var publicSecretLikeFields = map[string]bool{
	"accesstokenexpireon":  true,
	"refreshtokenexpireon": true,
	"tokenexpiry":          true,
	"tokentype":            true,
	"hassecret":            true,
	"useaccesstoken":       true,
	"isconsolesession":     true,
	"suggestedpasswords":   true,
	"passwordid":           true,
	"sessions":             true,
	"externalclientid":     true,
	"appclientidentifier":  true,
	"skiptoken":            true,
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
// can be a credential. A number is a credential too (a one-time code, a PIN:
// Codex r1 4.34.2 class holder-incomplete) unless its name reads as a measure
// (numericMeasureName: "total_tokens", "passwordMinLength", "sessionCount").
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
// "passwordMinLength", "tokenTTL", "token_expires_at", Kaseya BMS's
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

// CredentialPath reports a request path with a segment that reads as a
// credential (kaseya-bms handfixes credential-endpoint-result-held and
// credential-endpoint-request-held): POST /v2/security/authenticate and
// /v2/security/refreshtoken, GET /v2/integrations/itglue/passwords/{id} and
// /v2/rmm/{type}/remotesessions/{asset}. Such an endpoint's request and
// answer are credentials as a whole, not only under credential names.
func CredentialPath(p string) bool {
	for _, seg := range strings.Split(strings.SplitN(p, "?", 2)[0], "/") {
		if seg != "" && SecretFieldName(seg) {
			return true
		}
	}
	return false
}

// credentialEndpointIdentifiers are the request fields of a credential
// endpoint that identify, not authenticate, and may be shown (an allowlist:
// every other field of such a request is held): the login's user name and
// tenant, the grant type and the redirect URI.
var credentialEndpointIdentifiers = map[string]bool{"username": true, "tenant": true, "granttype": true, "redirecturi": true}

// CredentialEndpointIdentifier reports a credential-endpoint request field
// that may be shown.
func CredentialEndpointIdentifier(name string) bool {
	return credentialEndpointIdentifiers[normalizedFieldName(name)]
}

// HoldCredentialEndpointValues holds every value of a credential endpoint's
// request (fields by name, at any depth: strings and numbers as written) but
// the allowlisted identifiers, with the credential kit (kaseya-bms handfix
// credential-endpoint-request-held): the BMS login form carries credentials
// under names that are data elsewhere in the API ("Code", the OAuth
// authorization code, is an account code in eight other schemas), so a
// declared field name cannot hold them.
func HoldCredentialEndpointValues(fields map[string]any) {
	for k, v := range fields {
		if !CredentialEndpointIdentifier(k) {
			holdEveryScalar("field:"+k, v)
		}
	}
}

func holdEveryScalar(source string, v any) {
	switch t := v.(type) {
	case string:
		credkit.Hold(source, t)
	case json.Number:
		credkit.Hold(source, t.String())
	case float64:
		credkit.Hold(source, strconv.FormatFloat(t, 'f', -1, 64))
	case int:
		credkit.Hold(source, strconv.Itoa(t))
	case int64:
		credkit.Hold(source, strconv.FormatInt(t, 10))
	case map[string]any:
		for _, x := range t {
			holdEveryScalar(source, x)
		}
	case []any:
		for _, x := range t {
			holdEveryScalar(source, x)
		}
	}
}

// Held reports whether the credential kit would withhold s from a file: s
// carries a value this process holds, in any form the kit's own matcher
// finds (handfixes.json credkit-wiring). The connector keeps no matcher of
// its own.
func Held(s string) bool {
	if s == "" {
		return false
	}
	var buf bytes.Buffer
	_, err := credkit.NewWriter(&buf, credkit.SinkFile).Write([]byte(s))
	return err != nil || buf.String() != s
}
