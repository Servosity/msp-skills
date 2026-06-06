// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Seeded-store test for the org-show novel feature (reframed from organizations brief).

package cli

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestNovelOrgShowCommand(t *testing.T) {
	seedITGStore(t)
	out := runITGCmd(t, "org", "show", "1", "--json")

	var brief map[string]any
	if err := json.Unmarshal([]byte(out), &brief); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}

	counts, ok := brief["counts"].(map[string]any)
	if !ok {
		t.Fatalf("brief missing counts: %s", out)
	}
	want := map[string]string{"contacts": "2", "passwords": "2", "configurations": "1", "documents": "0"}
	for k, v := range want {
		if got := fmt.Sprint(counts[k]); got != v {
			t.Errorf("counts[%s] = %s, want %s", k, got, v)
		}
	}

	org, ok := brief["organization"].(map[string]any)
	if !ok {
		t.Fatalf("brief missing organization header: %s", out)
	}
	if org["name"] != "Acme" {
		t.Errorf("organization.name = %v, want Acme", org["name"])
	}
	if org["found"] != true {
		t.Errorf("organization.found = %v, want true", org["found"])
	}

	// The configurations section carries the projected configuration.
	configs, ok := brief["configurations"].([]any)
	if !ok || len(configs) != 1 {
		t.Fatalf("expected 1 configuration in brief; got %v", brief["configurations"])
	}
}

func TestNovelOrgShowUnknownOrg(t *testing.T) {
	seedITGStore(t)
	out := runITGCmd(t, "org", "show", "999", "--json")

	var brief map[string]any
	if err := json.Unmarshal([]byte(out), &brief); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	org, ok := brief["organization"].(map[string]any)
	if !ok {
		t.Fatalf("brief missing organization header: %s", out)
	}
	if org["found"] != false {
		t.Errorf("organization.found = %v, want false for unknown org", org["found"])
	}
}
