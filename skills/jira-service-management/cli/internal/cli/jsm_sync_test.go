// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestJSMSkipDependent(t *testing.T) {
	none := map[string]bool{}
	req := map[string]bool{"request": true}
	named := map[string]bool{"request": true, "comment": true, "sla": true}
	cases := []struct {
		name, parent string
		allow        map[string]bool
		skip         bool
	}{
		{"comment", "request", none, true},
		{"comment", "request", req, true},
		{"comment", "request", named, false},
		{"comment_attachment", "comment", named, true},
		{"sla", "request", named, true},
		{"approval", "request", none, true},
		{"user", "organization", none, false},
		{"queue", "servicedesk", req, false},
	}
	for _, c := range cases {
		if got, _ := jsmSkipDependent(c.name, c.parent, c.allow); got != c.skip {
			t.Errorf("jsmSkipDependent(%s, %s, %v) = %v, want %v", c.name, c.parent, c.allow, got, c.skip)
		}
	}
	if _, res := jsmSkipDependent("sla", "request", named); len(res) != 1 || res[0].Err == nil {
		t.Fatalf("named unsupported dependent must return an error result: %+v", res)
	}
	if _, res := jsmSkipDependent("sla", "request", req); len(res) != 0 {
		t.Fatalf("unnamed dependent must be skipped silently: %+v", res)
	}
}
