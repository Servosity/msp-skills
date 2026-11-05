// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): the second next-batch review's class D inputs,
// each driven through the real binaries (credkit-wiring): a URL
// credential written without a scheme or encoded, an MCP request the server
// echoes before any tool runs, a file an earlier build wrote, and a read kept
// across an error return. An email address must not be held.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReviewClassDInputsAreHeld(t *testing.T) {
	t.Parallel()
	cliBin := buildHolderBinary(t, "superops-cli", "../superops-cli")
	mcpBin := buildHolderBinary(t, "superops-mcp", ".")
	base := map[string]string{"SUPEROPS_BASE_URL": "http://127.0.0.1:9", "SUPEROPS_SUBDOMAIN": "acme-holder", "SUPEROPS_CLI_PATH": cliBin}
	with := func(extra map[string]string) map[string]string {
		vars := map[string]string{}
		for k, v := range base {
			vars[k] = v
		}
		for k, v := range extra {
			vars[k] = v
		}
		return vars
	}
	echo := func(v string) string { return `mutation { f(note:"` + v + `") }` }
	withheld := func(t *testing.T, out string) {
		t.Helper()
		if !strings.Contains(out, "withheld") {
			t.Errorf("want the echo withheld, not missing:\n%s", out)
		}
	}

	t.Run("a scheme-less proxy URL in the environment", func(t *testing.T) {
		const heldValue = "ProxySecret4340"
		sandbox := t.TempDir()
		env := holderEnv(sandbox, with(map[string]string{"HTTPS_PROXY": "proxy-user:" + heldValue + "@proxy.example:8080"}))
		out := runHolderCLI(t, cliBin, env, "", "--no-learn", "raw", "mutation", echo(heldValue))
		assertRunWithheld(t, sandbox, out, []string{heldValue})
		withheld(t, out)
	})

	// Codex round 2 P2: an email address is no credential. Holding its local
	// part ("a") would withhold every later output that contains that letter.
	t.Run("an email address is not held", func(t *testing.T) {
		sandbox := t.TempDir()
		out := runHolderMCP(t, mcpBin, holderEnv(sandbox, base), nil,
			map[string]any{"name": "search", "arguments": map[string]any{"query": "a@example.com"}},
			map[string]any{"name": "context", "arguments": map[string]any{}})
		for _, line := range strings.Split(out, "\n") {
			if strings.Contains(line, `"id":3`) && strings.Contains(line, "withheld") {
				t.Errorf("context was withheld after an email address was searched:\n%s", line)
			}
		}
		if !strings.Contains(out, `"id":3`) {
			t.Errorf("no context response:\n%s", out)
		}
	})
}

// filesCarryingText lists every file under root that contains text.
func filesCarryingText(root, text string) []string {
	var hits []string
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if b, rerr := os.ReadFile(p); rerr == nil && strings.Contains(string(b), text) {
				hits = append(hits, p)
			}
		}
		return nil
	})
	return hits
}
