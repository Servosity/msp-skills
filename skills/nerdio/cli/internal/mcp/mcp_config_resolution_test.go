// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"os"
	"path/filepath"
	"testing"
)

// Issue #270: `mcp/tools.go` hardcoded ~/.config/<tool>-cli/config.toml and
// ignored the operator's config location entirely, so a Claude Desktop install
// had no way to point the MCP server away from a plaintext token cache -- the
// half the reporter's `--config <null device>` workaround could never reach.
// newMCPClient now resolves the config through config.Load(""), the same
// resolver the CLI uses, which reads NERDIO_CONFIG and applies
// NERDIO_NO_CONFIG_WRITE.

func TestMCPClientHonoursTheConfigEnvVar(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("base_url = \"https://mcp-config-probe.invalid\"\n"), 0o600); err != nil {
		t.Fatalf("seeding config: %v", err)
	}
	t.Setenv("HOME", dir)
	t.Setenv("NERDIO_CONFIG", path)
	// Guard against a stray ambient override changing the assertion.
	os.Unsetenv("NERDIO_BASE_URL")

	c, err := newMCPClient()
	if err != nil {
		t.Fatalf("newMCPClient: %v", err)
	}
	if c.Config == nil {
		t.Fatal("newMCPClient returned a client with no config")
	}
	if c.Config.Path != path {
		t.Errorf("MCP server read %q, want the NERDIO_CONFIG path %q", c.Config.Path, path)
	}
	if c.Config.BaseURL != "https://mcp-config-probe.invalid" {
		t.Errorf("MCP server did not load the operator's config: base_url = %q", c.Config.BaseURL)
	}
}
