// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/acronis/handfixes.json under
// the "credential-fields-never-persisted" entry. See docs/reprint-survival.md.

package client

import (
	"encoding/json"
	"strings"

	"acronis-pp-cli/internal/cliutil"
)

// responseNotCacheable reports whether a GET body must stay off the on-disk
// response cache: an identity-provider path (its answers are tokens whatever
// their shape), a path segment named like a credential, or a body that is not
// one JSON document or carries a non-empty secret field at any depth
// (cliutil.JSONCarriesSecret, the same predicate the local store applies).
func responseNotCacheable(path string, data json.RawMessage) bool {
	return credentialPath(path) || cliutil.JSONCarriesSecret(data)
}

// credentialPath reports whether a request path addresses the identity
// provider (/api/2/idp/...) or names a credential in one of its segments.
func credentialPath(path string) bool {
	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}
	for _, seg := range strings.Split(strings.ToLower(path), "/") {
		if seg == "idp" || cliutil.IsSecretFieldName(seg) {
			return true
		}
	}
	return false
}
