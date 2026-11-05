// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"testing"

	"prtg-pp-cli/internal/cliutil"
)

// prtg handfix (state-changing-get-confirms): a GET the spec documents as a
// state change (GET /session renews the session) previews until confirm, is
// refused under PRTG_MCP_READ_ONLY, and is confirm-gated whichever endpoint
// id resolves to it.
func TestStateChangingGETNeedsConfirm(t *testing.T) {
	found := false
	for i := range codeOrchEndpoints {
		ep := &codeOrchEndpoints[i]
		if ep.Method != "GET" || !cliutil.StateChangingGET(ep.Path) {
			continue
		}
		found = true
		if !ep.NeedsConfirm || !ep.Mutating {
			t.Errorf("GET %s (%s) changes state but runs without confirm", ep.Path, ep.ID)
		}
		if !codeOrchResolvedNeedsConfirm(ep, "GET", ep.Path, nil) {
			t.Errorf("GET %s resolves without confirm", ep.Path)
		}
		t.Setenv("PRTG_MCP_READ_ONLY", "1")
		if !codeOrchRefuseForReadOnlySwitch(ep, "GET", false) {
			t.Errorf("GET %s runs under PRTG_MCP_READ_ONLY", ep.Path)
		}
	}
	if !found {
		t.Fatal("no endpoint matched StateChangingGET; the list and the spec disagree")
	}
	plain := &codeOrchEndpoint{ID: "x", Method: "GET", Path: "/sensors"}
	if !codeOrchResolvedNeedsConfirm(plain, "GET", "/session", nil) {
		t.Error("GET /session reached through another endpoint id resolves without confirm")
	}
}
