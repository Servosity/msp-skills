// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import "testing"

// Every PRTG operation that is not a GET changes state, except the documented
// lookup POST /users/api-keys/find, so prtg_execute must preview it until
// confirm and prtg_read must refuse it (spec: x-pp-mutation on every non-GET
// operation, tools/derive_spec.py step 7). A filter-only body is not a read:
// POST /users/pause pauses every matching account.
func TestEveryStateChangingEndpointNeedsConfirm(t *testing.T) {
	reads := map[string]bool{"users.find-apikey": true}
	for _, ep := range codeOrchEndpoints {
		if ep.Method == "GET" {
			continue
		}
		if reads[ep.ID] {
			if ep.NeedsConfirm {
				t.Errorf("%s %s is a documented read but needs confirm", ep.Method, ep.ID)
			}
			continue
		}
		if !ep.NeedsConfirm {
			t.Errorf("%s %s (%s) changes state but runs without confirm", ep.Method, ep.ID, ep.Path)
		}
	}
}
