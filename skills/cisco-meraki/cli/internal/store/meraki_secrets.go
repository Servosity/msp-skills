// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/cisco-meraki/handfixes.json
// under "meraki-mirror-redacts-secrets".

package store

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

// RedactedSecret replaces a credential value before it reaches the local
// mirror. SSID updates, RADIUS/IPsec/webhook settings and a few reads echo
// PSKs and shared secrets; search, analytics and MCP sql (Allow tier) read the
// mirror, so the value must never be stored.
const RedactedSecret = "[redacted]"

// merakiSecretKeys are Dashboard API field names (lowercased) whose string
// value is a credential. Keys ending in password/passphrase/secret/psk are
// redacted by suffix as well, which covers authPassword, radiusSecret and
// friends.
var merakiSecretKeys = map[string]bool{
	"apikey":               true,
	"claimkey":             true,
	"communitystring":      true,
	"contenttoken":         true,
	"hashedtoken":          true,
	"md5authenticationkey": true,
	"parsedtoken":          true,
	"token":                true,
	"v2communitystring":    true,
	"v3authpass":           true,
	"v3privpass":           true,
	"vppservicetoken":      true,
}

var merakiSecretSuffixes = []string{"password", "passphrase", "secret", "psk"}

// merakiLicenseSiblings mark an object whose "key" is a license key, which the
// mirror keeps on purpose (license audits); any other string "key" (Umbrella,
// Secure Access and Dashboard API keys) is redacted. Numeric keys (the SSID
// GRE key) are not strings and are kept.
var merakiLicenseSiblings = []string{"mode", "editions", "counts", "licenseType"}

func merakiSecretKey(name string, obj map[string]any) bool {
	lower := strings.ToLower(name)
	if merakiSecretKeys[lower] {
		return true
	}
	for _, suffix := range merakiSecretSuffixes {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	if lower == "key" {
		for _, sibling := range merakiLicenseSiblings {
			if _, ok := obj[sibling]; ok {
				return false
			}
		}
		return true
	}
	return false
}

func redactMerakiValue(v any) (any, bool) {
	switch tv := v.(type) {
	case map[string]any:
		changed := false
		label, _ := tv["label"].(string)
		for k, child := range tv {
			// A secret-named key always forces a re-encode, even when its
			// decoded value is empty or not a string: the decoder keeps only
			// the LAST duplicate key, so the original bytes could still hold
			// an earlier secret ({"psk":"x","psk":""}).
			if merakiSecretKey(k, tv) || ((k == "oldValue" || k == "newValue") && merakiSecretLabel(label)) {
				if s, ok := child.(string); ok && s != "" {
					tv[k] = RedactedSecret
				}
				changed = true
				continue
			}
			if s, ok := child.(string); ok && s != "" && s != RedactedSecret {
				// Change-log rows carry an API request body as a JSON string.
				if next := RedactMerakiJSONString(s); next != s {
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
// RedactedSecret. JSON that carries no secret and repeats no key is returned
// unchanged byte for byte; text that is not valid JSON is scrubbed textually.
func RedactMerakiSecrets(data json.RawMessage) json.RawMessage {
	if !json.Valid(data) {
		return json.RawMessage(merakiScrubText(string(data)))
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
				if s, ok := child.(string); ok && s != "" && s != RedactedSecret && merakiSecretKey(k, tv) && strings.Contains(id, s) {
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
	l := strings.ToLower(label)
	for _, w := range []string{"psk", "pre-shared", "preshared", "passphrase", "secret", "community", "api key", "authentication key", "shared key"} {
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

var merakiASCIIEscRe = regexp.MustCompile(`\\u[0-9a-fA-F]{4}`)

// merakiSecretPairRe matches a "secret-key": "value" pair in text that looks
// like JSON but does not parse (a truncated change-log body); the value may
// run to the end of the text.
var merakiSecretPairRe = regexp.MustCompile(`(?i)("(?:[a-z0-9_]*(?:password|passphrase|secret|psk)|apikey|claimkey|communitystring|v2communitystring|v3authpass|v3privpass|md5authenticationkey|token|contenttoken|parsedtoken|vppservicetoken|hashedtoken|key)"\s*:\s*")((?:[^"\\]|\\.|\\$)*)("|$)`)

// merakiScrubText scrubs secret-named pairs from text that looks like JSON
// but does not parse (a truncated change-log body), after decoding \uXXXX
// escapes so "\u0070sk" cannot hide a key name. Text with no such pair is
// returned unchanged.
func merakiScrubText(s string) string {
	u := merakiASCIIEscRe.ReplaceAllStringFunc(s, func(m string) string {
		n, err := strconv.ParseUint(m[2:], 16, 32)
		if err != nil || n < 0x20 || n >= 0x7f || n == '"' || n == '\\' {
			return m
		}
		return string(rune(n))
	})
	if r := merakiSecretPairRe.ReplaceAllString(u, "${1}"+RedactedSecret+"${3}"); r != u {
		return r
	}
	return s
}

// merakiHasDuplicateKey reports whether any object in valid JSON data repeats
// a key.
func merakiHasDuplicateKey(data []byte) bool {
	type frame struct {
		keys      map[string]bool // nil for an array
		expectKey bool
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	var stack []*frame
	for {
		tok, err := dec.Token()
		if err != nil {
			return false
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

// RedactMerakiJSONString redacts credentials inside a string that holds JSON
// (a change-log oldValue/newValue). Any other string is returned unchanged.
func RedactMerakiJSONString(s string) string {
	t := strings.TrimSpace(s)
	if t == "" || (t[0] != '{' && t[0] != '[') {
		return s
	}
	if !json.Valid([]byte(t)) {
		return merakiScrubText(s)
	}
	if out := RedactMerakiSecrets(json.RawMessage(t)); string(out) != t {
		return string(out)
	}
	return s
}

// RedactMerakiChangeValue is the change-log output twin of the mirror
// redaction: fleet changes and fleet blame print oldValue/newValue live.
func RedactMerakiChangeValue(label, v string) string {
	if v != "" && merakiSecretLabel(label) {
		return RedactedSecret
	}
	return RedactMerakiJSONString(v)
}
