// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/cisco-meraki/handfixes.json
// under "meraki-mirror-redacts-secrets".

package store

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// RedactedSecret replaces a credential value before it reaches the local
// mirror. SSID updates, RADIUS/IPsec/webhook settings and a few reads echo
// PSKs and shared secrets; search, analytics and MCP sql (Allow tier) read the
// mirror, so the value must never be stored.
const RedactedSecret = "[redacted]"

// merakiSecretKeys are Dashboard API field names (lowercased, with "_", "-"
// and spaces removed) whose string value is a credential. Keys ending in
// password/passphrase/secret/psk/token/key are redacted by suffix as well,
// which covers authPassword, radiusSecret, access_token, routing_key and
// friends.
var merakiSecretKeys = map[string]bool{
	"apikey":               true,
	"bodyfile":             true, // base64 webhook body template (can carry a token)
	"claimkey":             true,
	"communitystring":      true,
	"contenttoken":         true,
	"hashedtoken":          true,
	"callback":             true, // an action batch or live-tool callback: receiver URL + HTTP server
	"httpserverid":         true, // a webhook HTTP server ID is the base64 of its URL
	"httpserverids":        true,
	"headersfile":          true, // base64 webhook headers (Authorization)
	"md5authenticationkey": true,
	"parsedtoken":          true,
	"token":                true,
	"v2communitystring":    true,
	"v3authpass":           true,
	"v3privpass":           true,
	"vppservicetoken":      true,
}

var merakiSecretSuffixes = []string{"password", "passphrase", "secret", "psk", "token", "key"}

// merakiLicenseSiblings mark an object whose "key" is a license key, which the
// mirror keeps on purpose (license audits); any other string "key" (Umbrella,
// Secure Access and Dashboard API keys) is redacted. Numeric keys (the SSID
// GRE key) are not strings and are kept.
var merakiLicenseSiblings = []string{"mode", "editions", "counts", "licenseType"}

var merakiKeyNameNormalizer = strings.NewReplacer("-", "", "_", "", " ", "")

func merakiSecretKey(name string, obj map[string]any) bool {
	if strings.HasPrefix(name, "_pp_") {
		return false // the store's own synthesized fields (_pp_map_key)
	}
	lower := merakiKeyNameNormalizer.Replace(strings.ToLower(name))
	if merakiSecretKeys[lower] {
		return true
	}
	if lower == "licensekey" {
		return false // license keys are kept on purpose
	}
	if lower != "key" {
		for _, suffix := range merakiSecretSuffixes {
			if strings.HasSuffix(lower, suffix) {
				return true
			}
		}
		return false
	}
	for _, sibling := range merakiLicenseSiblings {
		if _, ok := obj[sibling]; ok {
			return false
		}
	}
	return true
}

// merakiSecretField reports whether obj[k] holds a credential: a secret-named
// key, a change-log old/new value under a secret label, or the value of a
// name/value pair whose name is a credential (a webhook template header
// {"name":"Authorization","template":"Basic ..."}).
func merakiSecretField(k string, obj map[string]any) bool {
	if merakiSecretKey(k, obj) {
		return true
	}
	switch k {
	case "oldValue", "newValue":
		label, _ := obj["label"].(string)
		v, _ := obj[k].(string)
		return merakiSecretChange(label, v)
	case "value", "template":
		name, _ := obj["name"].(string)
		return merakiSecretName(name)
	}
	return false
}

// merakiSecretName reports a credential header or parameter name
// ("Authorization", "X-Api-Key", "x-functions-key", "Cookie",
// "client_secret"). It fails closed: any name ending in "key" counts.
func merakiSecretName(name string) bool {
	l := strings.NewReplacer("-", "", "_", "", " ", "").Replace(strings.ToLower(name))
	for _, w := range []string{"auth", "cookie", "apikey", "token", "secret", "password", "passphrase", "psk", "community", "credential", "signature"} {
		if strings.Contains(l, w) {
			return true
		}
	}
	return strings.HasSuffix(l, "key") || merakiSecretKeys[l]
}

// redactAllStrings replaces every non-empty string in v with RedactedSecret.
func redactAllStrings(v any) (any, bool) {
	switch tv := v.(type) {
	case string:
		if tv != "" && tv != RedactedSecret {
			return RedactedSecret, true
		}
	case map[string]any:
		changed := false
		for k, child := range tv {
			if next, ok := redactAllStrings(child); ok {
				tv[k] = next
				changed = true
			}
		}
		return tv, changed
	case []any:
		changed := false
		for i, child := range tv {
			if next, ok := redactAllStrings(child); ok {
				tv[i] = next
				changed = true
			}
		}
		return tv, changed
	}
	return v, false
}

func redactMerakiValue(v any) (any, bool) {
	switch tv := v.(type) {
	case map[string]any:
		changed := false
		for k, child := range tv {
			// A secret field always forces a re-encode, even when its
			// decoded value is empty or not a string: the decoder keeps only
			// the LAST duplicate key, so the original bytes could still hold
			// an earlier secret ({"psk":"x","psk":""}). A container under a
			// secret field (md5AuthenticationKey.passphrase,
			// parsedToken.hashedToken) has every string redacted: fail closed.
			if merakiSecretField(k, tv) {
				tv[k], _ = redactAllStrings(child)
				changed = true
				continue
			}
			if s, ok := child.(string); ok && s != "" && s != RedactedSecret {
				// Change-log rows carry an API request body as a JSON string.
				next := redactMerakiString(s)
				if merakiRequestLogKeys[strings.ToLower(k)] {
					next = ScrubMerakiURLSecrets(next)
				}
				if next != s {
					tv[k] = next
					changed = true
					continue
				}
			}
			if next, ok := redactMerakiValue(child); ok {
				tv[k] = next
				changed = true
			}
		}
		return tv, changed
	case []any:
		changed := false
		for i, child := range tv {
			if s, ok := child.(string); ok {
				if next := redactMerakiString(s); next != s {
					tv[i] = next
					changed = true
				}
				continue
			}
			if next, ok := redactMerakiValue(child); ok {
				tv[i] = next
				changed = true
			}
		}
		return tv, changed
	}
	return v, false
}

// RedactMerakiSecrets returns data with every credential value replaced by
// RedactedSecret. JSON with no secret field and no repeated key is returned
// unchanged byte for byte; JSON with a secret field is re-encoded (keys
// sorted) even when nothing under it was a string. Data that is not valid
// JSON is withheld whole (sync stores items of a parsed array, so this is a
// fail-closed backstop, not a path real rows take).
func RedactMerakiSecrets(data json.RawMessage) json.RawMessage {
	if len(bytes.TrimSpace(data)) == 0 {
		return data
	}
	if !json.Valid(data) {
		return json.RawMessage(strconv.Quote(RedactedCutOff))
	}
	// UseNumber keeps large numeric IDs exact across the re-encode.
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if dec.Decode(&v) != nil {
		return data
	}
	redacted, changed := redactMerakiValue(v)
	// The decoder keeps only the LAST duplicate key, so a repeated key (at
	// any depth) can hide an earlier secret from the walk above; re-encoding
	// the decoded value drops those earlier bytes.
	if !changed && !merakiHasDuplicateKey(data) {
		return data
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if enc.Encode(redacted) != nil {
		return data
	}
	return bytes.TrimRight(buf.Bytes(), "\n")
}

// merakiIDCarriesSecret reports whether id contains any credential value in
// obj, so the storage-ID resolvers never key a row on a secret.
func merakiIDCarriesSecret(id string, obj map[string]any) bool {
	var walk func(v any) bool
	walk = func(v any) bool {
		switch tv := v.(type) {
		case map[string]any:
			for k, child := range tv {
				// Change-log values are never part of an ID, and a short one
				// ("0", "1") is a substring of nearly every composite.
				if k == "oldValue" || k == "newValue" {
					continue
				}
				if s, ok := child.(string); ok && s != "" && s != RedactedSecret && merakiSecretField(k, tv) && strings.Contains(id, s) {
					return true
				}
				if walk(child) {
					return true
				}
			}
		case []any:
			for _, child := range tv {
				if walk(child) {
					return true
				}
			}
		}
		return false
	}
	return walk(obj)
}

// merakiSecretLabel reports a change-log label that names a credential
// ("Pre-shared key (PSK)", "RADIUS secret 1", "SNMP community"). It fails
// closed: any strong credential word counts. A label whose only credential
// word is "password" is skipped when it is about a password policy
// ("Password expiration", "Minimum password length").
func merakiSecretLabel(label string) bool {
	l := merakiNormalized(label) // "web%68ooks" is "webhooks"
	// "webhook": payload-template bodies and headers are free-form templates
	// (a PagerDuty routing_key, a form-encoded token), and an HTTP server's
	// URL and ID are credentials, so webhook change values are withheld whole.
	for _, w := range []string{"psk", "pre-shared", "preshared", "passphrase", "secret", "community", "api key", "authentication key", "shared key", "webhook"} {
		if strings.Contains(l, w) {
			return true
		}
	}
	i := strings.LastIndex(l, "password")
	if i < 0 {
		return false
	}
	// Only a policy word AFTER the password names a policy setting
	// ("Password expiration"); "Access policy: RADIUS password" is a secret.
	for _, policy := range []string{"expiration", "expiry", "length", "complexity", "history", "rotation", "strength", "lockout", "requirement"} {
		if strings.Contains(l[i:], policy) {
			return false
		}
	}
	return true
}

// merakiHasDuplicateKey reports whether any object in valid JSON data repeats
// a key.
func merakiHasDuplicateKey(data []byte) bool {
	type frame struct {
		keys      map[string]bool // nil for an array
		expectKey bool
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber() // a float64-overflowing number (1e400) must not end the walk
	var stack []*frame
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return false
		}
		if err != nil {
			return true // fail closed: re-encode
		}
		if d, ok := tok.(json.Delim); ok && (d == '}' || d == ']') {
			stack = stack[:len(stack)-1]
			continue
		}
		if n := len(stack); n > 0 && stack[n-1].keys != nil {
			top := stack[n-1]
			if top.expectKey {
				k, _ := tok.(string)
				if top.keys[k] {
					return true
				}
				top.keys[k] = true
				top.expectKey = false
				continue
			}
			top.expectKey = true
		}
		switch tok {
		case json.Delim('{'):
			stack = append(stack, &frame{keys: map[string]bool{}, expectKey: true})
		case json.Delim('['):
			stack = append(stack, &frame{})
		}
	}
}

// RedactedCutOff replaces a JSON-looking string that does not parse, such as
// a change-log request body Meraki cut off. Its pairs cannot be told apart
// reliably (escaped nested bodies, a key cut mid-name), so the whole value is
// withheld: fail closed. The raw change log (Credential tier) still has it.
const RedactedCutOff = "[redacted: cut-off JSON]"

// RedactMerakiJSONString redacts credentials inside a string that holds JSON
// (a change-log oldValue/newValue). A JSON-looking string that does not parse
// becomes RedactedCutOff. Any other string is returned unchanged.
func RedactMerakiJSONString(s string) string {
	t := strings.TrimSpace(s)
	if t == "" || (t[0] != '{' && t[0] != '[') {
		return s
	}
	if !json.Valid([]byte(t)) {
		// A value that holds a string is cut-off JSON unless it opens with a
		// bracketed tag: `[Cedar] "Clinic"` is a name, `[null,{"psk":` is not.
		if strings.Contains(t, `"`) && !merakiBracketTag(t) {
			return RedactedCutOff
		}
		return s
	}
	if out := RedactMerakiSecrets(json.RawMessage(t)); string(out) != t {
		return string(out)
	}
	return s
}

// RedactMerakiChangeValue is the change-log output twin of the mirror
// redaction: fleet changes and fleet blame print oldValue/newValue live.
func RedactMerakiChangeValue(label, v string) string {
	if v != "" && merakiSecretChange(label, v) {
		return RedactedSecret
	}
	return redactMerakiString(v)
}

// merakiSecretChange reports a change-log value that is withheld whole: one
// under a credential label, or one that touches a webhook (an action batch
// whose action targets /webhooks/ carries receiver URLs and free-form
// payload templates, whatever the label says).
func merakiSecretChange(label, v string) bool {
	return merakiSecretLabel(label) || strings.Contains(merakiNormalized(v), "webhook")
}

// merakiBracketTag reports a value that opens with a bracketed name tag
// ("[Default] Guest"): a "]" that closes before any JSON punctuation.
func merakiBracketTag(t string) bool {
	end := strings.IndexByte(t, ']')
	return t[0] == '[' && end > 1 && !strings.ContainsAny(t[1:end], `[{",:`)
}

// merakiLoose matches lit with any character percent-encoded ("web%68ooks").
// Use under (?i).
func merakiLoose(lit string) string {
	var b strings.Builder
	for _, c := range []byte(lit) {
		b.WriteString("(?:" + regexp.QuoteMeta(string(c)) + "|%" + strconv.FormatInt(int64(c), 16) + ")")
	}
	return b.String()
}

// merakiHTTPServerIDPattern matches a webhook HTTP server ID in a path: the ID
// is the base64 of the receiver URL ("/" and "+" included, percent-encoded in
// a path; a Slack, Teams or PagerDuty URL is a credential), and change-log
// labels and API request paths carry it. An already scrubbed "[redacted:...]"
// is outside the class and never matches.
var merakiHTTPServerIDPattern = regexp.MustCompile(`(?i)(` + merakiLoose("/webhooks/httpServers/") + `)([A-Za-z0-9+/=_%.~-]+)`)

// merakiURLParamPattern matches the start of a url= query parameter, its name
// possibly percent-encoded: the webhook logs filter puts the receiver URL in
// API request query strings.
var merakiURLParamPattern = regexp.MustCompile(`(?i)(?:^|[?&"])` + merakiLoose("url="))

// merakiPercentDecodePattern is one %XX escape.
var merakiPercentDecodePattern = regexp.MustCompile(`%[0-9A-Fa-f]{2}`)

// merakiPercentDecoded decodes %XX escapes until none are left (double
// encoding included); a malformed "%" is kept as is.
func merakiPercentDecoded(s string) string {
	for i := 0; i < 4 && strings.Contains(s, "%"); i++ {
		next := merakiPercentDecodePattern.ReplaceAllStringFunc(s, func(m string) string {
			c, _ := strconv.ParseUint(m[1:], 16, 8)
			return string([]byte{byte(c)})
		})
		if next == s {
			break
		}
		s = next
	}
	return s
}

// merakiJSONEscapePattern is one \uXXXX escape.
var merakiJSONEscapePattern = regexp.MustCompile(`\\u[0-9A-Fa-f]{4}`)

// merakiNormalized lowercases s with %XX and \uXXXX escapes decoded (nested
// or mixed: "web%68\u006foks") and backslashes dropped, for the keyword checks
// that withhold a webhook value. It fails closed: it only ever adds matches.
func merakiNormalized(s string) string {
	for i := 0; i < 4; i++ {
		next := merakiPercentDecoded(s)
		next = merakiJSONEscapePattern.ReplaceAllStringFunc(next, func(m string) string {
			c, _ := strconv.ParseUint(m[2:], 16, 32)
			return string(rune(c))
		})
		next = strings.ReplaceAll(next, `\`, "")
		if next == s {
			break
		}
		s = next
	}
	return strings.ToLower(s)
}

// ScrubMerakiURLSecrets replaces webhook HTTP server IDs and url= query values
// in s with "[redacted:<hash>]". The 128-bit hash keeps two receivers (and two
// row IDs built from them) distinct without revealing either; an already
// scrubbed value is left alone. fleet changes and fleet blame apply it to
// labels, row identities to each part, and the mirror to API request paths
// and query strings (scrubMerakiHTTPServerIDs covers every other string).
//
// A url= value runs to the end of the string: a receiver URL can be sent
// unencoded, partly encoded or double encoded, so no later "&" is certainly
// the outer query's. The webhook logs parameters after it (perPage,
// startingAfter) are withheld with it on purpose.
func ScrubMerakiURLSecrets(s string) string {
	s = scrubMerakiHTTPServerIDs(s)
	loc := merakiURLParamPattern.FindStringIndex(s)
	if loc == nil {
		return s
	}
	rest := s[loc[1]:]
	end := strings.IndexAny(rest, "\"\t\n\f\r ")
	if end < 0 {
		end = len(rest)
	}
	return s[:loc[1]] + merakiRedactedHash(rest[:end]) + ScrubMerakiURLSecrets(rest[end:])
}

// scrubMerakiHTTPServerIDs replaces webhook HTTP server IDs in a path.
func scrubMerakiHTTPServerIDs(s string) string {
	return merakiHTTPServerIDPattern.ReplaceAllStringFunc(s, func(m string) string {
		parts := merakiHTTPServerIDPattern.FindStringSubmatch(m)
		return parts[1] + merakiRedactedHash(parts[2])
	})
}

// merakiRequestLogKeys are the API request log fields that carry the webhook
// logs url= filter; a url= anywhere else (a splash page redirect) is kept.
var merakiRequestLogKeys = map[string]bool{"querystring": true, "path": true}

// merakiScrubbedPattern is a value this scrub already produced.
var merakiScrubbedPattern = regexp.MustCompile(`^\[redacted(:[0-9a-f]{32})?\]$`)

// merakiRedactedHash is "[redacted:<128-bit sha256 prefix>]" for v, or v when
// it is empty or already scrubbed.
func merakiRedactedHash(v string) string {
	if v == "" || merakiScrubbedPattern.MatchString(v) {
		return v
	}
	sum := sha256.Sum256([]byte(v))
	return "[redacted:" + hex.EncodeToString(sum[:16]) + "]"
}

func redactMerakiString(s string) string {
	if t := strings.TrimSpace(s); t != "" && (t[0] == '{' || t[0] == '[') && json.Valid([]byte(t)) {
		return RedactMerakiJSONString(s) // its string leaves were scrubbed decoded
	}
	return scrubMerakiHTTPServerIDs(RedactMerakiJSONString(s))
}

// RedactMerakiObject redacts credentials in m in place (fleet push prints the
// current settings it diffs against). Under a webhook path every string is
// redacted: an HTTP server's plain "url" is its credential.
func RedactMerakiObject(m map[string]any, path string) {
	// Decoded first: "web%68ooks" reaches the same endpoint.
	// A malformed escape fails closed.
	if _, err := url.PathUnescape(path); err != nil || strings.Contains(merakiNormalized(path), "webhook") {
		redactAllStrings(m)
		return
	}
	redactMerakiValue(m)
}
