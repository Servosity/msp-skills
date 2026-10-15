// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import (
	"bytes"
	"encoding/json"
	"strings"
)

// hand-fix (handfixes.json secret-fields-never-persisted, secret-field-list-unified):
// WatchGuard Cloud bodies carry credentials inside otherwise ordinary rows.
// Firebox Management returns and accepts BOVPN pre-shared keys (psk,
// psk_encrypted), certificate private keys, PKCS#12 bundles and their
// passwords (pvt_key, pfx, password), Firebox user and WebBlocker override
// passwords, PPPoE / Dynamic DNS passwords, Wi-Fi passphrases and DHCP auth
// tokens; the activation API carries product activation and license keys
// (activationKey, initialServiceLicenseKey, parentLicenseKey); AuthPoint takes
// passwords, one-time passwords and Forgot Token activation / verification
// codes. This is the ONE list: the local store, the response cache, tail, row
// identity, --dry-run previews and error-body echo withholding all use it.
//
// secretFieldExact names match the whole lower-cased key, secretFieldParts
// match anywhere in it (adminPassword, psk_encrypted) and secretFieldSuffixes
// match its end (auth_token, accessToken, but not a total_tokens count).
var secretFieldExact = map[string]bool{
	"otp": true,
	"p12": true,
}

var secretFieldParts = []string{
	"password", "passphrase", "secret",
	"psk", "pvt_key", "private_key", "privatekey", "privkey", "pfx", "pkcs12",
	"apikey", "api_key",
	"activationkey", "activation_key", "licensekey", "license_key",
	"activationcode", "activation_code", "verificationcode", "verification_code",
}

var secretFieldSuffixes = []string{"token"}

// IsSecretBearingPath reports whether a GET path returns credentials in
// fields whose names are not secret-shaped: the Endpoint Security installers
// response is a download link for an installer that enrolls devices into the
// account, so it is never cached or stored.
func IsSecretBearingPath(path string) bool {
	p := strings.TrimRight(path, "/")
	if i := strings.IndexByte(p, '?'); i >= 0 {
		p = strings.TrimRight(p[:i], "/")
	}
	return strings.HasSuffix(p, "/installers")
}

// IsSecretFieldKey reports whether a JSON key names a credential value.
// Matching ignores case so psk, PSK and activationKey are all caught.
func IsSecretFieldKey(key string) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	if secretFieldExact[k] {
		return true
	}
	for _, part := range secretFieldParts {
		if strings.Contains(k, part) {
			return true
		}
	}
	for _, suffix := range secretFieldSuffixes {
		if strings.HasSuffix(k, suffix) {
			return true
		}
	}
	return false
}

// SecretFieldValue reports whether a value under a secret-named key can
// carry a secret: booleans and nulls (psk_set: true, isInForgotToken: false)
// cannot, so they stay readable and never mark a body as secret-bearing.
func SecretFieldValue(v any) bool {
	switch v.(type) {
	case bool, nil:
		return false
	}
	return true
}

// RedactSecretFields returns raw with every secret-named key removed at any
// depth, and whether anything was removed. Unchanged input is returned as-is;
// numbers keep their exact text (UseNumber). Invalid JSON is returned as-is.
func RedactSecretFields(raw json.RawMessage) (json.RawMessage, bool) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return raw, false
	}
	if !dropSecretKeys(v) {
		return raw, false
	}
	out, err := json.Marshal(v)
	if err != nil {
		return raw, false
	}
	return out, true
}

// ContainsSecretField reports whether raw holds a secret-named key at any depth.
func ContainsSecretField(raw json.RawMessage) bool {
	_, removed := RedactSecretFields(raw)
	return removed
}

func dropSecretKeys(v any) bool {
	removed := false
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			if IsSecretFieldKey(k) && SecretFieldValue(child) {
				delete(t, k)
				removed = true
				continue
			}
			if dropSecretKeys(child) {
				removed = true
			}
		}
	case []any:
		for _, child := range t {
			if dropSecretKeys(child) {
				removed = true
			}
		}
	}
	return removed
}
