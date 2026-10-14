// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: secret-bearing ConnectSecure resources never reach the local store.

package store

import "connectsecure-pp-cli/internal/cliutil"

// NeverStoreResource reports whether rows of resourceType must not be
// persisted. The recorded store.go guards (entry points for honest sync
// counts, row writers for every other path) call it; the list itself lives
// in cliutil so the response-cache guard reads the same source.
func NeverStoreResource(resourceType string) bool {
	return cliutil.ConnectsecureSecretResource(resourceType)
}
