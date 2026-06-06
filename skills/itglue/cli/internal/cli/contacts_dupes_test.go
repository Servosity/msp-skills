// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Seeded-store test for the contacts-dupes novel feature.

package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNovelContactsDupesCommand(t *testing.T) {
	seedITGStore(t)
	out := runITGCmd(t, "contacts", "dupes", "--json")

	var groups []dupeGroup
	if err := json.Unmarshal([]byte(out), &groups); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if len(groups) == 0 {
		t.Fatalf("expected a duplicate group; got %s", out)
	}

	found := false
	for _, g := range groups {
		ids := map[string]bool{}
		for _, c := range g.Contacts {
			ids[c.ID] = true
		}
		if g.Count == 2 && ids["10"] && ids["11"] {
			found = true
			// 10 and 11 share both a normalized name ("jane doe") and email.
			if !strings.Contains(g.MatchType, "email") || !strings.Contains(g.MatchType, "name") {
				t.Errorf("match_type = %q, want email+name", g.MatchType)
			}
		}
	}
	if !found {
		t.Errorf("expected duplicate group {10,11}; got %s", out)
	}
}
