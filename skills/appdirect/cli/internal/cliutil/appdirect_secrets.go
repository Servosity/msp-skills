// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

// appdirect handfix (store-never-keeps-secrets): the one rule for which
// field names carry credentials, shared by output redaction and the local
// store, so the two can never disagree.

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"math/big"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
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
// deliberately over-matches ("token", "apikey", "session"); the AppDirect API
// names it matches are its real credential fields (password, client_secret,
// secret_key, mfa_code, the token pair) plus the exemptions below
// (TestNoAppDirectSpecNameReadsAsACredential). Codex r1 (4.34.2 reprint) class
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
// structured data with the holder (appdirect handfix: output-chokepoint): every
// string, of any length, under a credential-named key at any depth (all
// strings under a credential-named object or array), except masks. A string
// that is itself a JSON object or array is read too. Request bodies and
// params, responses, MCP tool arguments and output members all use it.
func HoldCredentialValues(v any) {
	holdCredentialValues(v, false, false)
}

// holdCredentialValues: secret marks a value under a credential name (at any
// depth); measure marks one whose nearest name reads as a measure, so a number
// under it is a count, not a credential (Codex r1 4.34.2 class
// holder-incomplete: a numeric one-time code or PIN was neither held nor
// redacted).
func holdCredentialValues(v any, secret, measure bool) {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			measure := numericMeasureName(k)
			if _, list := child.([]any); list {
				measure = numericItemMeasureName(k)
			}
			holdCredentialValues(child, secret || SecretFieldName(k), measure)
		}
	case []any:
		for _, child := range t {
			holdCredentialValues(child, secret, measure)
		}
	case json.Number, float64, float32, int, int64, int32, uint64, uint32:
		if secret && !measure {
			holdNumber(t)
		}
	case string:
		if secret && !isMaskValue(t) {
			FunnelHold(t)
			holdCredentialParts(t)
		}
		// A credential a URL or a parameter list carries is one under any
		// name (Codex r1 4.34.2 class holder-incomplete): "?access_token=x",
		// "token=x&expires_in=3600".
		holdEmbeddedCredentials(t)
		// A value shaped like a credential is one under any name (Codex r1
		// class holder-incomplete): an auth-scheme value ("Bearer x",
		// "Basic x", ...) and a JWT. A credential-named value is read for
		// its shape too (Codex r1 4.34.2 appdirect class holder incomplete for
		// decoded authentication parts).
		holdCredentialShapedValue(t)
		if trimmed := strings.TrimSpace(t); len(trimmed) > 1 && (trimmed[0] == '{' || trimmed[0] == '[') {
			dec := json.NewDecoder(strings.NewReader(trimmed))
			dec.UseNumber()
			var inner any
			if dec.Decode(&inner) == nil {
				holdCredentialValues(inner, secret, measure)
			}
		}
	}
}

// authSchemes are the Authorization schemes whose value is a credential.
var authSchemes = []string{"bearer ", "basic ", "token ", "digest ", "negotiate ", "ntlm ", "apikey ", "sso-key "}

// holdCredentialParts holds the pieces a credential-named value carries
// besides itself: the credential after an auth scheme word, and each value of
// a cookie jar or parameter list ("sid=x; theme=y", "a=x&b=y"), at any length
// (Codex r1 4.34.2 class holder-incomplete: a value under shortMinLen was left
// out, so {"cookie":"sid=pQ7","name":"pQ7"} printed it; every held value is
// matched anywhere at any length).
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
			if val = strings.Trim(strings.TrimSpace(val), `"`); val != "" && !isMaskValue(val) {
				FunnelHold(val)
				if dec, err := url.QueryUnescape(val); err == nil && dec != val && !isMaskValue(dec) {
					FunnelHold(dec)
				}
			}
		}
	}
}

// credentialParts is every part a held credential carries besides itself, at
// any length (appdirect handfix derived-credential-parts-held; Codex r1 4.34.2
// class holder incomplete for decoded authentication parts): the value after
// an auth scheme word, and for the value and that part, the "id:secret" text a
// base64 pair decodes to and both of its halves. The funnel's needles keep
// these only from heldMatchFloor characters up, so a short Basic password
// ("Basic dTphYmNkZWY=" carries "abcdef") printed elsewhere was not withheld;
// held here, a part below heldMatchFloor is matched by shortEchoes. Every held
// value from every source passes holdEncodedCores, which holds these, so one
// rule covers the environment, the configuration and store, flags, MCP
// arguments, requests and responses. Masks are never held.
func credentialParts(v string) []string {
	t := strings.TrimSpace(v)
	cands := []string{t}
	var out []string
	keep := func(p string) {
		if p = strings.TrimSpace(p); p != "" && !isMaskValue(p) {
			out = append(out, p)
		}
	}
	if rest := funnelSchemeless(t); rest != t {
		keep(rest)
		cands = append(cands, rest)
	}
	for _, c := range cands {
		if pair, ok := funnelBase64Pair(c); ok {
			keep(pair)
			if id, secret, ok := strings.Cut(pair, ":"); ok {
				// The id is the public half (a user name): below shortMinLen it
				// is not held, since every held value is matched anywhere at
				// any length (shortEchoes) and a one-letter user name would
				// withhold nearly every line. The secret half is held at any
				// length.
				if len(strings.TrimSpace(id)) >= shortMinLen {
					keep(id)
				}
				keep(secret)
			}
		}
	}
	return out
}

// holdCredentialShapedValue holds a value that is a credential by its shape
// whatever it is called: "Bearer <token68>" (a token of at least
// shortMinLen*2 characters holding a digit, or of 20 or more), "Basic
// <base64 of user:password>", and a JWT. The shapes are narrow so ordinary
// text ("Basic Mobility", "Bearer of news") is never held.
func holdCredentialShapedValue(v string) {
	t := strings.TrimSpace(v)
	if len(t) > 7 && strings.EqualFold(t[:7], "bearer ") {
		rest := strings.TrimSpace(t[7:])
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
// A body that is not JSON (AppDirect documents XML responses; a token
// endpoint or proxy can answer form-encoded or plain text) is read as text:
// every credential its markup, parameter lists, "Name: value" lines, URLs,
// auth-scheme values and JWTs carry is held (Codex r1 4.34.2 class
// holder-incomplete).
func HoldCredentialFields(body []byte) {
	if len(body) == 0 {
		return
	}
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.UseNumber()
	var v any
	if dec.Decode(&v) == nil {
		HoldCredentialValues(v)
		return
	}
	holdEmbeddedCredentials(string(body))
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

// credentialEnvVars are the variables the configuration applies as
// credentials (config.Load): the AppDirect client pair, the only credential
// the environment carries (there is no static-token variable). Each is held
// whatever its name reads as: "CLIENT_ID" does not read as a credential name,
// so APPDIRECT_CLIENT_ID would be held only when Load applied it, and a route
// Load refuses (a config other users can change names another host) printed
// it in the refusal that repeats that config's base_url.
// TestEveryConfigEnvVariableIsClassified keeps this list in step with
// config.go.
var credentialEnvVars = []string{"APPDIRECT_CLIENT_ID", "APPDIRECT_CLIENT_SECRET"}

// publicEnvVars read as credential names but carry none: APPDIRECT_LEARN_SESSION
// is the learn journal's session grouping key, which the journal stores (held,
// every journal entry would repeat a held value and none would be written).
// A value equal to a real credential is still caught: the journal writes no
// entry that repeats a held value (appdirect handfix output-chokepoint).
var publicEnvVars = map[string]bool{"APPDIRECT_LEARN_SESSION": true}

// HoldEnvCredentials holds, before anything can print (appdirect handfix:
// output-chokepoint), every credential the environment carries for this
// connector: the configuration's credential variables (credentialEnvVars),
// the value of every APPDIRECT_* variable whose name reads as a credential (the
// MCP HTTP tokens, and any credential-named variable a later print adds), and
// the credentials a APPDIRECT_*_URL value carries (its userinfo password and
// credential-named query values). A URL itself is not a credential.
func HoldEnvCredentials() {
	for _, name := range credentialEnvVars {
		HoldKeys(EnvOverride(name))
	}
	for _, kv := range os.Environ() {
		name, _, ok := strings.Cut(kv, "=")
		if !ok || !strings.HasPrefix(name, "APPDIRECT_") {
			continue
		}
		v := EnvOverride(name)
		switch {
		case strings.HasSuffix(name, "_URL"):
			HoldURLCredentials(v)
		case publicEnvVars[name]:
		case SecretFieldName(strings.TrimPrefix(name, "APPDIRECT_")):
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
// refuse the file (appdirect handfix output-chokepoint; Codex r2 class
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
// out (appdirect handfix: config-file-holds-no-credential): the userinfo, each
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

// numericItemMeasureName is numericMeasureName for the items of an array: a
// plural name is the list's own name ("otpCodes": [123456]), so only the
// measure and identifier words count.
func numericItemMeasureName(name string) bool {
	for _, w := range nameWords(name) {
		if measureWords[w] {
			return true
		}
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

// holdNumber holds a numeric credential as written and as its canonical
// decimal (an integer without exponent or fraction, a fraction without
// trailing zeros), with and without its sign. The output chokepoint matches a
// held number in every way a number is written (numericEchoes).
func holdNumber(v any) {
	var text string
	switch n := v.(type) {
	case json.Number:
		text = n.String()
	case float64:
		text = strconv.FormatFloat(n, 'f', -1, 64)
	case float32:
		text = strconv.FormatFloat(float64(n), 'f', -1, 32)
	default:
		text = fmt.Sprint(n)
	}
	forms := []string{text}
	if r, ok := new(big.Rat).SetString(text); ok {
		forms = append(forms, canonicalNumber(r))
		forms = append(forms, canonicalNumber(new(big.Rat).Abs(r)))
	}
	for _, f := range forms {
		if f = strings.TrimSpace(f); f != "" {
			FunnelHold(f)
		}
	}
}

// canonicalNumber is r in plain decimal: an integer without a fraction, a
// fraction with no trailing zeros.
func canonicalNumber(r *big.Rat) string {
	if r.IsInt() {
		return r.Num().String()
	}
	return strings.TrimRight(strings.TrimRight(r.FloatString(32), "0"), ".")
}

// embeddedURL finds the URLs a text carries; embeddedParam the name=value
// pairs of a query string, cookie or parameter list; embeddedHeader a
// "Name: value" line; embeddedScheme an auth-scheme value anywhere.
var (
	embeddedParam  = regexp.MustCompile(`([A-Za-z0-9_.\-\[\]%]+)=("[^"]*"|[^&;,\s"'<>]*)`)
	embeddedHeader = regexp.MustCompile(`(?m)^\s*([A-Za-z0-9_\-]+)\s*:\s*(.+?)\s*$`)
	embeddedScheme = regexp.MustCompile(`(?i)\b(bearer|basic|token|apikey|sso-key)\s+([A-Za-z0-9\-._~+/:]+=*)`)
	embeddedJWT    = regexp.MustCompile(`[A-Za-z0-9_\-]{8,}\.[A-Za-z0-9_\-]{8,}\.[A-Za-z0-9_\-]*`)
	markupElement  = regexp.MustCompile(`<([A-Za-z_][\w:.\-]*)(?:\s[^<>]*)?>\s*(?:<!\[CDATA\[([\s\S]*?)\]\]>|([^<]*))\s*</([A-Za-z_][\w:.\-]*)\s*>`)
	markupAttr     = regexp.MustCompile(`([A-Za-z_][\w:.\-]*)\s*=\s*(?:"([^"]*)"|'([^']*)')`)
)

// holdMarkupCredentials holds the text of every element and the value of
// every attribute whose name (without a namespace prefix) reads as a
// credential, CDATA and entity-escaped values included.
func holdMarkupCredentials(s string) {
	local := func(name string) string {
		if i := strings.LastIndex(name, ":"); i >= 0 {
			return name[i+1:]
		}
		return name
	}
	hold := func(v string) {
		for _, f := range []string{v, html.UnescapeString(v)} {
			if f = strings.TrimSpace(f); f != "" && !isMaskValue(f) {
				FunnelHold(f)
				holdCredentialParts(f)
			}
		}
	}
	for _, m := range markupElement.FindAllStringSubmatch(s, -1) {
		if m[1] == m[4] && SecretFieldName(local(m[1])) {
			hold(m[2] + m[3])
		}
	}
	for _, m := range markupAttr.FindAllStringSubmatch(s, -1) {
		if SecretFieldName(local(m[1])) {
			hold(m[2] + m[3])
		}
	}
}

// holdEmbeddedCredentials holds the credentials a string carries under any
// name (Codex r1 4.34.2 class holder-incomplete): the userinfo password and
// credential-named query values of every URL in it, the value of every
// credential-named parameter of a query string, cookie or parameter list and
// of every credential-named "Name: value" line, every auth-scheme value and
// JWT in it, and the credential members of JSON that starts inside it.
func holdEmbeddedCredentials(s string) {
	if len(s) < 3 {
		return
	}
	if strings.Contains(s, "<") {
		holdMarkupCredentials(s)
	}
	if strings.Contains(s, "://") || strings.Contains(s, `:\/\/`) {
		u := strings.ReplaceAll(s, `\/`, "/")
		for rest := u; ; {
			i := strings.Index(rest, "://")
			if i < 0 {
				break
			}
			start := strings.LastIndexAny(rest[:i], " \t\r\n\"'=<>([{,;|`") + 1
			end := strings.IndexAny(rest[i:], " \t\r\n\"'<>)]},|`")
			if end < 0 {
				end = len(rest) - i
			}
			HoldURLCredentials(rest[start : i+end])
			rest = rest[i+end:]
		}
	}
	if strings.Contains(s, "=") {
		for _, m := range embeddedParam.FindAllStringSubmatch(s, -1) {
			k := m[1]
			if dk, err := url.QueryUnescape(k); err == nil {
				k = dk
			}
			if !SecretFieldName(k) {
				continue
			}
			v := strings.Trim(m[2], `"`)
			if v != "" && !isMaskValue(v) {
				FunnelHold(v)
				holdCredentialParts(v)
			}
			if dv, err := url.QueryUnescape(v); err == nil && dv != v && dv != "" && !isMaskValue(dv) {
				FunnelHold(dv)
			}
		}
	}
	if strings.Contains(s, ":") {
		for _, m := range embeddedHeader.FindAllStringSubmatch(s, -1) {
			// One token, or a credential shape: "Session: 3 hours" is prose.
			if SecretFieldName(m[1]) && !isMaskValue(m[2]) && (!strings.ContainsAny(m[2], " \t") || CredentialShaped(m[2])) {
				FunnelHold(m[2])
				holdCredentialParts(m[2])
				holdCredentialShapedValue(m[2])
			}
		}
	}
	for _, m := range embeddedScheme.FindAllString(s, -1) {
		holdCredentialShapedValue(m)
	}
	for _, m := range embeddedJWT.FindAllString(s, -1) {
		if LooksLikeJWT(m) {
			FunnelHold(m)
		}
	}
	for _, open := range []string{"{", "["} {
		if i := strings.Index(s, open); i > 0 {
			dec := json.NewDecoder(strings.NewReader(s[i:]))
			dec.UseNumber()
			var inner any
			if dec.Decode(&inner) == nil {
				holdCredentialValues(inner, false, false)
			}
		}
	}
}

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
