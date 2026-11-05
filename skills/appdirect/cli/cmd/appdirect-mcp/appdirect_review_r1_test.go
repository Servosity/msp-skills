// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package main

import (
	"bytes"
	"strings"
	"testing"

	"appdirect-pp-cli/internal/cliutil"
)

// Codex r1 (appdirect reprint on 4.34.2) class credential split across protocol
// messages: two unknown-tool errors, each naming half of the held token, were
// each filtered alone and printed the token whole. The message that completes
// a held value with what the server already sent is replaced by the withheld
// error for its request.
func TestTokenSplitAcrossProtocolMessagesIsWithheld(t *testing.T) {
	const tok = "r4Zq9mB2v7Kx6nT8p3Ds5wC1"
	cliutil.FunnelHold(tok)
	var out bytes.Buffer
	w := &withheldMessageWriter{dst: &out}
	for _, msg := range []string{
		`{"jsonrpc":"2.0","id":1,"error":{"code":-32602,"message":"tool 'r4Zq9mB2v7Kx' not found: tool not found"}}`,
		`{"jsonrpc":"2.0","id":2,"error":{"code":-32602,"message":"tool '6nT8p3Ds5wC1' not found: tool not found"}}`,
	} {
		if _, err := w.Write([]byte(msg + "\n")); err != nil {
			t.Fatal(err)
		}
	}
	got := out.String()
	if strings.Contains(got, "r4Zq9mB2v7Kx") && strings.Contains(got, "6nT8p3Ds5wC1") {
		t.Fatalf("both halves of the held token reached the client:\n%s", got)
	}
	if !strings.Contains(got, `"id":2`) {
		t.Fatalf("the withheld message must still answer request 2:\n%s", got)
	}
}
