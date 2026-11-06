// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package kbmsblackbox

// Codex review r1 input as given (class holder incomplete): the OAuth
// authorization code of the BMS login form prints in no preview.

import (
	"strings"
	"testing"
)

func TestAuthorizationCodeNeverPrints(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, credStored)
	const code = "X9b2K7m4Q8p3N6r5"
	out := e.run(t, "security", "authenticate", "--grant-type", "authorization_code", "--code", code, "--dry-run")
	if !strings.Contains(out, "dry") {
		t.Fatalf("no preview printed; the case did not run:\n%.1500s", out)
	}
	if strings.Contains(out, code) {
		t.Fatalf("the authorization code prints in the preview:\n%.1500s", out)
	}
	if methods, mints, _, _ := sv.counts(); len(methods) != 0 || mints != 0 {
		t.Fatalf("--dry-run sent %v (%d logins)", methods, mints)
	}
}

// Codex review r1 P2 (one line, fixed): a read-only POST through
// kaseya-bms_read sends, so its client mints on a clean login install like a
// GET does (only an execute preview builds a client that never mints).
func TestMCPReadOnlyPostMints(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, credLogin)
	out := e.mcpCalls(t, []map[string]any{{"name": "kaseya-bms_read", "arguments": map[string]any{
		"endpoint_id": "servicedesk.search-tickets", "params": map[string]any{}}}})
	methods, mints, minted, bad := sv.counts()
	if mints != 1 || minted != 1 {
		t.Fatalf("want one accepted login for the read-only POST, got %d (%d accepted, refused %v):\n%.1500s", mints, minted, bad, out)
	}
	var sent bool
	for _, m := range methods {
		sent = sent || strings.HasPrefix(m, "POST /v2/servicedesk/tickets/search")
	}
	if n, _ := sv.sawHeader("Bearer " + bbMintAccess); !sent || n == 0 {
		t.Fatalf("the read-only POST was not sent with the minted token: %v\n%.1500s", methods, out)
	}
	assertNoEcho(t, "MCP session", out)
}

// Codex review r2 input as given (class holder incomplete, the MCP preview
// path): a kaseya-bms_execute preview of the login with an authorization code
// returns no part of the code.
func TestMCPLoginPreviewNeverReturnsTheAuthorizationCode(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, credStored)
	const code = "X9b2K7m4Q8p3N6r5"
	out := e.mcpCalls(t, []map[string]any{{"name": "kaseya-bms_execute", "arguments": map[string]any{
		"endpoint_id": "security.authenticate", "params": map[string]any{"GrantType": "authorization_code", "Code": code}}}})
	if !strings.Contains(out, `"id":1`) {
		t.Fatalf("the MCP call was not answered:\n%.1500s", out)
	}
	if strings.Contains(out, code) {
		t.Fatalf("the MCP preview returns the authorization code:\n%.1500s", out)
	}
	if methods, mints, _, _ := sv.counts(); len(methods) != 0 || mints != 0 {
		t.Fatalf("the preview sent %v (%d logins)", methods, mints)
	}
}
