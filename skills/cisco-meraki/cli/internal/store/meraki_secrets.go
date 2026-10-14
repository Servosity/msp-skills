// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/cisco-meraki/handfixes.json
// under "meraki-mirror-redacts-secrets".

package store

import (
	"bytes"
	"encoding/json"
	"regexp"
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
			if s, ok := child.(string); ok && s != "" && s != RedactedSecret {
				if merakiSecretKey(k, tv) {
					tv[k] = RedactedSecret
					changed = true
					continue
				}
				// Change-log rows carry the request body as a JSON string
				// (oldValue/newValue of an API PUT) or a secret-labelled value.
				if (k == "oldValue" || k == "newValue") && merakiSecretLabel(label) {
					tv[k] = RedactedSecret
					changed = true
					continue
				}
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
// RedactedSecret. Input that is not JSON, or carries no secret, is returned
// unchanged byte for byte.
func RedactMerakiSecrets(data json.RawMessage) json.RawMessage {
	// UseNumber keeps large numeric IDs exact across the re-encode.
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if dec.Decode(&v) != nil {
		return data
	}
	redacted, changed := redactMerakiValue(v)
	if !changed {
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
	if !strings.Contains(l, "password") {
		return false
	}
	for _, policy := range []string{"expiration", "expiry", "policy", "length", "complexity", "history", "rotation", "strength", "lockout", "requirement"} {
		if strings.Contains(l, policy) {
			return false
		}
	}
	return true
}

// merakiSecretPairRe matches a "secret-key": "value" pair in text that looks
// like JSON but does not parse (a truncated change-log body); the value may
// run to the end of the text.
var merakiSecretPairRe = regexp.MustCompile(`(?i)("(?:[a-z0-9_]*(?:password|passphrase|secret|psk)|apikey|claimkey|communitystring|v2communitystring|v3authpass|v3privpass|md5authenticationkey|token|contenttoken|parsedtoken|vppservicetoken|hashedtoken|key)"\s*:\s*")((?:[^"\\]|\\.|\\$)*)("|$)`)

// RedactMerakiJSONString redacts credentials inside a string that holds JSON
// (a change-log oldValue/newValue). Any other string is returned unchanged.
func RedactMerakiJSONString(s string) string {
	t := strings.TrimSpace(s)
	if t == "" || (t[0] != '{' && t[0] != '[') {
		return s
	}
	if !json.Valid([]byte(t)) {
		// Truncated or broken JSON: scrub secret-named pairs textually.
		return merakiSecretPairRe.ReplaceAllString(s, "${1}"+RedactedSecret+"${3}")
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
