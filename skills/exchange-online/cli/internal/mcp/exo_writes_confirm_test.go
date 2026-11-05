// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"strings"
	"testing"
)

// Every Exchange endpoint is a POST, so the HTTP method says nothing about
// state: the cmdlet verb does. A Get-* cmdlet reads and runs through
// exchange-online_read with no confirm (it stays available under
// EXCHANGE_ONLINE_MCP_READ_ONLY); every other verb (Set, Add, Remove, Enable,
// Disable, Release, Delete, Clear) changes state, so exchange-online_execute
// must preview it until confirm and exchange-online_read must refuse it
// (spec: x-pp-mutation on every operation).
func TestEveryStateChangingCmdletNeedsConfirm(t *testing.T) {
	reads, writes := 0, 0
	for _, ep := range codeOrchEndpoints {
		_, cmdlet, ok := strings.Cut(ep.Path, "#")
		if !ok {
			t.Fatalf("%s %s: no #<Cmdlet> in the path", ep.ID, ep.Path)
		}
		if strings.HasPrefix(cmdlet, "Get-") {
			reads++
			if ep.NeedsConfirm {
				t.Errorf("%s (%s) only reads but needs confirm", ep.ID, cmdlet)
			}
			continue
		}
		writes++
		if !ep.NeedsConfirm {
			t.Errorf("%s (%s) changes state but runs without confirm", ep.ID, cmdlet)
		}
	}
	if reads != 39 || writes != 21 {
		t.Fatalf("found %d reads and %d writes, want 39 and 21; the endpoint table moved", reads, writes)
	}
}
