// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import (
	"bytes"
	"encoding/json"
	"strings"
)

// hand-fix (handfixes.json secret-fields-never-persisted): WatchGuard Cloud
// GET responses carry credentials inside otherwise ordinary rows. Firebox
// Management returns BOVPN pre-shared keys (psk, psk_encrypted), certificate
// private keys and their passwords (pvt_key, password), Firebox user and
// WebBlocker override passwords, PPPoE / Dynamic DNS passwords, Wi-Fi
// passphrases and DHCP auth tokens; the activation API returns product
// activation keys. The local store drops these keys at any depth and the
// response cache refuses bodies that contain them. The list is a superset:
// generic credential names are included so a new endpoint is covered by name.
var secretFieldKeys = map[string]bool{
	"psk":            true,
	"psk_encrypted":  true,
	"pvt_key":        true,
	"private_key":    true,
	"privatekey":     true,
	"password":       true,
	"passphrase":     true,
	"secret":         true,
	"client_secret":  true,
	"shared_secret":  true,
	"token":          true,
	"auth_token":     true,
	"access_token":   true,
	"refresh_token":  true,
	"api_key":        true,
	"apikey":         true,
	"activationkey":  true,
	"activation_key": true,
}

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
func IsSecretFieldKey(key string) bool { return secretFieldKeys[strings.ToLower(key)] }

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
			if IsSecretFieldKey(k) {
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
