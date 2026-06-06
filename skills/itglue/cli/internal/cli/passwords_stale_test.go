// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Seeded-store test for the passwords-stale novel feature.

package cli

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestNovelPasswordsStaleCommand(t *testing.T) {
	seedITGStore(t)
	out := runITGCmd(t, "passwords", "stale", "--days", "365", "--json")

	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}

	ids := map[string]bool{}
	for _, r := range rows {
		ids[fmt.Sprint(r["id"])] = true
	}
	if !ids["20"] {
		t.Errorf("stale should include password 20 (Firewall admin, 2020); got %s", out)
	}
	if ids["21"] {
		t.Errorf("stale should NOT include fresh password 21 (VPN); got %s", out)
	}

	// The audit must never surface a secret value.
	for _, r := range rows {
		if _, leaked := r["password"]; leaked {
			t.Errorf("stale audit leaked a password value: %#v", r)
		}
	}

	// Filtering to a different org yields none of Acme's passwords.
	out2 := runITGCmd(t, "passwords", "stale", "--days", "365", "--org", "999", "--json")
	var rows2 []map[string]any
	if err := json.Unmarshal([]byte(out2), &rows2); err != nil {
		t.Fatalf("unmarshal --org: %v\n%s", err, out2)
	}
	if len(rows2) != 0 {
		t.Errorf("--org 999 should match nothing; got %s", out2)
	}
}
