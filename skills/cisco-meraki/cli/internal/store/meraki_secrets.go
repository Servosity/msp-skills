// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/cisco-meraki/handfixes.json
// under "meraki-mirror-redacts-secrets".

package store

import (
	"bytes"
	"encoding/json"
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
		for k, child := range tv {
			if s, ok := child.(string); ok && s != "" && s != RedactedSecret && merakiSecretKey(k, tv) {
				tv[k] = RedactedSecret
				changed = true
				continue
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
