// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package knowbe4blackbox

// Credential-rule classes the fleet Codex reviews found, each proven on the built
// binaries:
//   - output-before-chokepoint: the configuration and the credential store
//     print warnings while they load, so the chokepoint is installed first
//     and delivers nothing until every startup source is held (CLI and MCP);

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"knowbe4-pp-cli/internal/credkit"
)

func withoutEnv(env []string, key string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if !strings.HasPrefix(kv, key+"=") {
			out = append(out, kv)
		}
	}
	return out
}

// Class output-before-chokepoint: a config file other users can change names
// another host, so loading it prints a refusal that repeats its base_url. The
// URL's path repeats the environment's two keys (KNOWBE4_API_KEY and
// KNOWBE4_USER_EVENT_API_KEY) and the MCP HTTP token, and its userinfo
// carries a password nothing else holds.
func TestNothingPrintsBeforeTheChokepoint(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	cfg := filepath.Join(e.home, "loose.toml")
	base := "https://ro:" + bbStored + "@other.example.com/" + bbReportHeld + "/" + bbEventHeld + "/" + bbHTTPHeld
	if err := os.WriteFile(cfg, []byte(fmt.Sprintf("base_url = %q\n", base)), 0o600); err != nil {
		t.Fatal(err)
	}
	makeLoose(t, cfg)
	e.env = append(withoutEnv(e.env, "KNOWBE4_BASE_URL"), "KNOWBE4_CONFIG="+cfg)
	cli := e.run(t, "version")
	mcp := e.mcpCalls(t, nil)
	for where, out := range map[string]string{"CLI startup": cli, "MCP startup": mcp} {
		// The MCP server no longer loads the config before its first tool
		// call, so its startup may print nothing at all; the CLI's must
		// print the refusal (or the kit's notice in its place).
		if where == "CLI startup" && !strings.Contains(out, "refused") && !strings.Contains(out, "not applied") && !strings.Contains(out, credkit.Notice(credkit.SinkStderr)) {
			t.Fatalf("%s printed no refusal for the loose config; the test did not run:\n%.1500s", where, out)
		}
		assertNoEcho(t, where, out)
	}
}
