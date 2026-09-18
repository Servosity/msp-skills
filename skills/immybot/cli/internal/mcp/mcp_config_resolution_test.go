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
// The MCP server now resolves the config through config.Load(""), the same
// resolver the CLI uses, which reads IMMYBOT_CONFIG and applies
// IMMYBOT_NO_CONFIG_WRITE.

func TestMCPClientHonoursTheConfigEnvVar(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("base_url = \"https://mcp-config-probe.invalid\"\n"), 0o600); err != nil {
		t.Fatalf("seeding config: %v", err)
	}
	t.Setenv("HOME", dir)
	t.Setenv("IMMYBOT_CONFIG", path)
	// Guard against a stray ambient override changing the assertion.
	os.Unsetenv("IMMYBOT_BASE_URL")
	os.Unsetenv("IMMYBOT_SUBDOMAIN")

	cfg0, err := newMCPConfig()
	if err != nil {
		t.Fatalf("resolving the MCP config: %v", err)
	}
	cfg := cfg0
	if cfg == nil {
		t.Fatal("the MCP server resolved no config")
	}
	if cfg.Path != path {
		t.Errorf("MCP server read %q, want the IMMYBOT_CONFIG path %q", cfg.Path, path)
	}
	if cfg.BaseURL != "https://mcp-config-probe.invalid" {
		t.Errorf("MCP server did not load the operator's config: base_url = %q", cfg.BaseURL)
	}
}
