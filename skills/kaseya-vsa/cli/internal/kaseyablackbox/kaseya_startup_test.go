// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package kaseyablackbox

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// Startup ordering: a credential inside a config file that fails to parse
// never reaches startup output, on the CLI or the MCP server.
func TestMalformedConfigCredentialNeverInStartupOutput(t *testing.T) {
	host := echoServer(t)
	shapes := []string{
		"token_secret = " + bbTokenSecret + "\n",
		bbTokenSecret + "\n",
		"token_secret = \"" + bbTokenSecret + "\"\ntoken_secret = \"" + bbTokenSecret + "\"\n",
		"[auth]\ntoken_secret = '" + bbTokenSecret + "\n",
		"x = 1\ntoken_secret = [\"" + bbTokenSecret + "\", ]]\n",
	}
	for i, body := range shapes {
		e := newEnv(t, host, false)
		cfg := filepath.Join(e.home, "kaseya-vsa.toml")
		if err := os.WriteFile(cfg, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		e.env = append(e.env, "KASEYA_VSA_CONFIG="+cfg)
		for _, args := range [][]string{{"doctor"}, {"auth", "status"}, {"devices", "list", "--json"}, {"--config", cfg, "doctor"}} {
			assertNoEcho(t, fmt.Sprintf("shape %d: output of %v", i, args), e.run(t, args...))
		}
		assertNoEcho(t, fmt.Sprintf("shape %d: MCP session", i), e.mcpCalls(t, []map[string]any{
			{"name": "kaseya-vsa_read", "arguments": map[string]any{"endpoint_id": "devices.list"}},
			{"name": "health", "arguments": map[string]any{}},
		}))
	}
}
