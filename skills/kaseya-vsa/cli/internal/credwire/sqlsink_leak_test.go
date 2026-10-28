// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated): Codex publish pass 2 round 2, sqlite-sink.
// The reviewer's own input, driven through the real binaries.

package credwire

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kaseya-vsa-pp-cli/internal/credkit"
)

// With an existing store and auth_header = "lowercasecredential123", MCP
// sql {"query":"SELECT 'lowercasecredential123' AS sample"} must be refused
// at the SQLite sink, before SQLite runs it: the statement text is a sink of
// its own (contract sections 2 and 5), not only the rows it would return.
//
// The reviewer's credential shares the piece "credential" with every kit
// notice, so the kit drops the notice from the reply too and the reply
// cannot say which sink refused. The second run holds a credential that
// shares no piece with any notice: there the reply names the sink, SQLite
// when the statement is refused before it runs, the MCP server when it ran
// and only its row was withheld.
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
		res := run(t, cli, s.env(), "search", "anything")
		if res.code != 0 {
			t.Fatalf("could not create the store (exit %d): stdout=%q stderr=%q", res.code, res.stdout, res.stderr)
		}
		if dbs, _ := filepath.Glob(filepath.Join(s.home, "data", "data*.db")); len(dbs) == 0 {
			t.Fatal("no store after search")
		}

		frames, stderr := mcpCall(t, mcp, s.env(), "sql", map[string]any{"query": "SELECT '" + secret + "' AS sample"})
		requireAbsent(t, "MCP sql frames", frames, secret)
		requireAbsent(t, "MCP stderr", stderr, secret)
		if !strings.Contains(frames, `"isError":true`) {
			t.Errorf("%s: the sql tool did not fail; frames:\n%s", secret, frames)
		}
		if !strings.Contains(strings.ToLower(secret), "credential") && !strings.Contains(frames, credkit.Notice(credkit.SinkSQLite)) {
			t.Errorf("%s: the statement was not refused at the SQLite sink (it ran, and only its output was withheld); frames:\n%s", secret, frames)
		}

		// An ordinary statement still runs.
		frames, _ = mcpCall(t, mcp, s.env(), "sql", map[string]any{"query": "SELECT 'an ordinary value' AS sample"})
		if !strings.Contains(frames, "an ordinary value") {
			t.Errorf("%s: an ordinary statement did not run; frames:\n%s", secret, frames)
		}
	}
}
