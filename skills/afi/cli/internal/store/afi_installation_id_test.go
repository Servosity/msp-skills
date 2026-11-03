// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import "testing"

// Both documented installation shapes get a stable identity; anything else
// keeps the generated extraction.
func TestInstallationsHaveAnIdentity(t *testing.T) {
	for obj, want := range map[[2]string]string{
		{"org1", ""}:     "org1:",
		{"", "ten1"}:     ":ten1",
		{"org1", "ten1"}: "org1:ten1",
		{"", ""}:         "",
	} {
		got := ExtractResourceID("applications", map[string]any{"org_id": obj[0], "tenant_id": obj[1]})
		if got != want {
			t.Errorf("installation %v: id %q, want %q", obj, got, want)
		}
	}
	if got := ExtractResourceID("tenants", map[string]any{"id": "t9", "org_id": "org1"}); got != "t9" {
		t.Errorf("tenants id = %q, want t9", got)
	}
}
