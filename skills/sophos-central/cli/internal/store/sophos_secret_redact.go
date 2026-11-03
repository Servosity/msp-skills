// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: pinned by skills/sophos-central/handfixes.json under
// "secret-responses-never-persisted".
//
// `sync` mirrored GET /endpoint/v1/migrations rows verbatim, so each
// migration job's token (the credential the other tenant uses to accept the
// endpoints) sat in the resources table, its FTS index and the typed
// migrations.token column. Every store write passes through
// upsertGenericResourceTx, so secret keys are dropped there, at any depth;
// the typed migrations writer drops its token column too.

package store

import "encoding/json"

// storedSecretKeys are JSON keys whose values are live credentials. Mirrors
// secretResponseKeys in internal/client.
var storedSecretKeys = map[string]bool{
	"password":          true,
	"previousPasswords": true,
	"token":             true,
	"clientSecret":      true,
	"client_secret":     true,
	"access_token":      true,
}

// redactStoredSecrets returns data without any secret key. Data that is not
// a JSON object or array is returned unchanged.
func redactStoredSecrets(data json.RawMessage) json.RawMessage {
	var v any
	if json.Unmarshal(data, &v) != nil {
		return data
	}
	if !stripSecretKeys(v) {
		return data
	}
	out, err := json.Marshal(v)
	if err != nil {
		return data
	}
	return out
}

// stripSecretKeys deletes secret keys in place and reports whether any were
// removed.
func stripSecretKeys(v any) bool {
	removed := false
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			if storedSecretKeys[k] {
				delete(t, k)
				removed = true
				continue
			}
			if stripSecretKeys(child) {
				removed = true
			}
		}
	case []any:
		for _, child := range t {
			if stripSecretKeys(child) {
				removed = true
			}
		}
	}
	return removed
}

// RedactSecrets is redactStoredSecrets for callers outside the store: the
// bulk export and tail commands print API bodies that can carry a migration
// token, and an MCP caller reaches both.
func RedactSecrets(data json.RawMessage) json.RawMessage {
	return redactStoredSecrets(data)
}
