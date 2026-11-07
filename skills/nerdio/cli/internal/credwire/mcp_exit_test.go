// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

import (
	"strings"
	"testing"
)

// mcp-exit-delivers-stderr (handfixes.json): stderr is a pipe a goroutine
// drains, and the MCP server used to leave through os.Exit (flag.Parse's own
// included) before the pipe delivered: -h printed no usage and a startup
// refusal (HTTP mode without NERDIO_MCP_HTTP_TOKEN) exited with no reason.
// Every exit now goes through credkit.Exit, which delivers first; flags parse
// with ContinueOnError, so -h exits 0 and a bad flag 2 after the text arrives.
func TestMCPExitDeliversItsMessage(t *testing.T) {
	_, mcpBin := binaries(t)
	home := t.TempDir()
	env := sandboxEnv(home, "NERDIO_MCP_HTTP_TOKEN=")
	if out, code := runCode(t, mcpBin, env, "", "-h"); code != 0 || !strings.Contains(out, "-transport") {
		t.Errorf("-h: exit %d, want 0 and the usage text; printed %q", code, out)
	}
	if out, code := runCode(t, mcpBin, env, "", "--transport", "http", "--addr", "127.0.0.1:0"); code == 0 || !strings.Contains(out, "NERDIO_MCP_HTTP_TOKEN must be set") {
		t.Errorf("http with no token: exit %d, want non-zero and the reason; printed %q", code, out)
	}
	token := "NERDIO_MCP_HTTP_TOKEN=Zq8vR2mK7xL4pN9wB3cT6yH1jF5sD0gAe7Wu"
	if out, code := runCode(t, mcpBin, append(env, token), "", "--transport", "http", "--addr", "0.0.0.0:0"); code == 0 || !strings.Contains(out, "MCP server error") {
		t.Errorf("non-loopback bind without TLS: exit %d, want non-zero and the reason; printed %q", code, out)
	}
	if out, code := runCode(t, mcpBin, env, "", "--no-such-flag"); code != 2 || !strings.Contains(out, "no-such-flag") {
		t.Errorf("unknown flag: exit %d, want 2 and the flag named; printed %q", code, out)
	}
}
