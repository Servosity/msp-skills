// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import "testing"

// Every Umbrella operation that is not a GET changes state, except the three
// documented read POSTs (domain categorization lookup, identities by ID, the
// SWG override list), so cisco-umbrella_execute must preview it until confirm
// and cisco-umbrella_read must refuse it (spec: x-pp-mutation on every non-GET
// operation, tools/spec_repair.py).
func TestEveryStateChangingEndpointNeedsConfirm(t *testing.T) {
	reads := map[string]bool{"investigate.post-domains-categorization": true, "reports.post-identities": true, "swg_device_settings.list": true}
	seen := 0
	for _, ep := range codeOrchEndpoints {
		if ep.Method == "GET" {
			continue
		}
		if reads[ep.ID] {
			seen++
			if ep.NeedsConfirm {
				t.Errorf("%s %s is a documented read but needs confirm", ep.Method, ep.ID)
			}
			continue
		}
		if !ep.NeedsConfirm {
			t.Errorf("%s %s (%s) changes state but runs without confirm", ep.Method, ep.ID, ep.Path)
		}
	}
	if seen != len(reads) {
		t.Fatalf("found %d of the %d documented read POSTs; the endpoint IDs moved", seen, len(reads))
	}
}
