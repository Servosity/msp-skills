// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

// TestSyncPruneRegionScope guards hand-fix sync-prune-region-scope: the
// configured region alone does not make a sync tenant-scoped (press 4.34.2
// skipped every --full prune on ESET), but a path-context override or any
// other template or path line still does.
func TestSyncPruneRegionScope(t *testing.T) {
	for key, want := range map[string]bool{
		"":                                       false,
		"template:region=eu":                     false,
		"path:region=us":                         true,
		"template:region=eu\npath:policyUuid=p1": true,
		"template:tenant=x":                      true,
	} {
		if got := syncScopeIsTenant(key); got != want {
			t.Errorf("syncScopeIsTenant(%q) = %v, want %v", key, got, want)
		}
	}
}
