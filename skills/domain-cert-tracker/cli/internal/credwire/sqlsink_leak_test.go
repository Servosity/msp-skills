// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated): Codex publish pass 2 round 2, sqlite-sink.
// The reviewer's own input, driven through the real binaries.

package credwire

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"domain-cert-tracker-pp-cli/internal/credkit"
)

// With an existing store and auth_header = "lowercasecredential123", MCP
// sql {"query":"SELECT 'lowercasecredential123' AS sample"} must not repeat
// the credential. Since synccpu (owner GO 2026-11-09) the store does not read
// statement text, so the statement runs and the MCP server checks the answer.
// The answer is one JSON document whose sample leaf carries the credential,
// so the kit redacts that leaf to "[redacted]" and releases the rest
// (credential leak contract section 4, amended 2026-11-25) instead of
// withholding the result whole: no notice, no tool error.
//
// The reviewer's credential shares the piece "credential" with every kit
// notice; the redaction marker shares no piece with it. The second run holds
// a credential that shares no piece with any notice.
func TestKitMCPSQLStatementWithheldAtSQLite(t *testing.T) {
	cli, mcp := binaries(t)
	for _, secret := range []string{"lowercasecredential123", "zq7Wsinkvalu3Kx"} {
		s := newSandbox(t)
		cfg := filepath.Join(s.home, "config", "config.toml")
		if err := os.MkdirAll(filepath.Dir(cfg), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(cfg, []byte("auth_header = \""+secret+"\"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		res := run(t, cli, s.env(), "clients", "set", "Acme", "--domain", "example.com", "--yes", "--json")
		if res.code != 0 {
			t.Fatalf("could not create the store (exit %d): stdout=%q stderr=%q", res.code, res.stdout, res.stderr)
		}
		if _, err := os.Stat(filepath.Join(s.home, "data", "data.db")); err != nil {
			t.Fatalf("no store after clients set: %v", err)
		}

		frames, stderr := mcpCall(t, mcp, s.env(), "sql", map[string]any{"query": "SELECT '" + secret + "' AS sample"})
		requireAbsent(t, "MCP sql frames", frames, secret)
		requireAbsent(t, "MCP stderr", stderr, secret)
		// synccpu: the store no longer reads statement text, so the statement
		// runs and the MCP server redacts the leaf that carries the credential.
		if !strings.Contains(frames, credkit.Redacted) {
			t.Errorf("%s: the answer's leaf was not redacted at the MCP sink; frames:\n%s", secret, frames)
		}
		if strings.Contains(frames, credkit.Notice(credkit.SinkMCP)) || strings.Contains(frames, `"isError":true`) {
			t.Errorf("%s: the answer was withheld whole, not redacted; frames:\n%s", secret, frames)
		}
		// A redacted answer may keep no piece of the credential either
		// (contract section 2(b)): "zq7Wsink[redacted]" carries no whole value
		// and still leaks "zq7Wsink".
		requireNoPiece(t, "MCP sql frames", frames, secret)
		requireNoPiece(t, "MCP stderr", stderr, secret)

		// An ordinary statement still runs.
		frames, _ = mcpCall(t, mcp, s.env(), "sql", map[string]any{"query": "SELECT 'an ordinary value' AS sample"})
		if !strings.Contains(frames, "an ordinary value") {
			t.Errorf("%s: an ordinary statement did not run; frames:\n%s", secret, frames)
		}
	}
}

// requireNoPiece fails when text carries any 8-character window of secret
// (credential leak contract section 2(b): a piece of 8 or more characters
// counts anywhere, so one surviving half is a leak on its own), or secret
// whole when it is shorter than 8. It is the check for every MCP output that
// may be redacted.
func requireNoPiece(t *testing.T, where, text, secret string) {
	t.Helper()
	if len(secret) < 8 {
		requireAbsent(t, where, text, secret)
		return
	}
	for i := 0; i+8 <= len(secret); i++ {
		if w := secret[i : i+8]; strings.Contains(text, w) {
			t.Errorf("%s carries %q, an 8-character piece of the credential %q:\n%s", where, w, secret, text)
			return
		}
	}
}
