// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored. Kept in its own file so `generate --force` preserves it
// (handfixes.json credkit-wiring): which caller-supplied mask values are
// credentials.

package client

import (
	"strings"
)

// callerCredentials is extra less ids, the config's identifiers (the Entra
// client id and tenant id, which the token mint passes to the masker beside
// the secret), read under the token-state lock. The config's own credentials
// are held from its declaration whatever their value, so a credential equal
// to an identifier stays held.
func callerCredentials(extra, ids []string) []string {
	out := make([]string, 0, len(extra))
next:
	for _, v := range extra {
		if v = strings.TrimSpace(v); v == "" {
			continue
		}
		for _, id := range ids {
			if v == strings.TrimSpace(id) {
				continue next
			}
		}
		out = append(out, v)
	}
	return out
}
