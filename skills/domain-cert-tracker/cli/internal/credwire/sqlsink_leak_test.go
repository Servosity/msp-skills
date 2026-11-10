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
// sql {"query":"SELECT 'lowercasecredential123' AS sample"} must be refused
// whole. Since synccpu (owner GO 2026-11-09) the store does not read statement
// text, so the statement runs and the MCP server withholds the answer.
//
// The reviewer's credential shares the piece "credential" with every kit
// notice, so the kit drops the notice from the reply too and the reply
// cannot say which sink refused. The second run holds a credential that
// shares no piece with any notice: there the reply names the MCP sink.
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
		if !strings.Contains(frames, `"isError":true`) {
			t.Errorf("%s: the sql tool did not fail; frames:\n%s", secret, frames)
		}
		if !strings.Contains(strings.ToLower(secret), "credential") && !strings.Contains(frames, credkit.Notice(credkit.SinkMCP)) {
			// synccpu: the store no longer reads statement text, so the statement
			// runs and the MCP server withholds its answer whole
			t.Errorf("%s: the answer was not withheld at the MCP sink; frames:\n%s", secret, frames)
		}

		// An ordinary statement still runs.
		frames, _ = mcpCall(t, mcp, s.env(), "sql", map[string]any{"query": "SELECT 'an ordinary value' AS sample"})
		if !strings.Contains(frames, "an ordinary value") {
			t.Errorf("%s: an ordinary statement did not run; frames:\n%s", secret, frames)
		}
	}
}
