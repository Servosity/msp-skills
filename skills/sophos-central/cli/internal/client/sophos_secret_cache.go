// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: pinned by skills/sophos-central/handfixes.json under
// "secret-responses-never-persisted".
//
// Sophos Central returns live credentials in a few responses: an endpoint's
// tamper protection password (and its previous passwords) and a migration
// job's token. The generated response cache wrote every GET body to disk, so
// one `endpoints get-tamper-protection` left the password in the cache
// directory. A body that carries any such key is never cached. The rule
// keys on the response content, not on a list of paths, so a new endpoint
// that returns one of these keys is covered too.

package client

import "encoding/json"

// secretResponseKeys are JSON keys whose values are live credentials.
var secretResponseKeys = map[string]bool{
	"password":          true,
	"previousPasswords": true,
	"token":             true,
	"clientSecret":      true,
	"client_secret":     true,
	"access_token":      true,
}

// responseCarriesSecret reports whether a JSON body holds a secret key at any
// depth. A body that is not JSON is treated as not carrying one.
func responseCarriesSecret(data []byte) bool {
	var v any
	if json.Unmarshal(data, &v) != nil {
		return false
	}
	return valueCarriesSecret(v)
}

func valueCarriesSecret(v any) bool {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			if secretResponseKeys[k] || valueCarriesSecret(child) {
				return true
			}
		}
	case []any:
		for _, child := range t {
			if valueCarriesSecret(child) {
				return true
			}
		}
	}
	return false
}
