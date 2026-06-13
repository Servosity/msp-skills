package config

import (
	"path/filepath"
	"testing"
)

// nonexistentPath returns a path under a fresh temp dir that does not exist, so
// Load() runs its env-var resolution without reading any real on-disk config.
func nonexistentPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "config.toml")
}

// TestClientIDHeaderInjected pins the hand-wired clientId header. ConnectWise
// Automate v2020.11+ requires a registered integration GUID on the `clientId`
// header of every request; it is per-MSP, so it is read from
// CONNECTWISE_AUTOMATE_CLIENT_ID and injected into cfg.Headers (which the
// generated client applies to every outbound request).
func TestClientIDHeaderInjected(t *testing.T) {
	t.Setenv("CONNECTWISE_AUTOMATE_CLIENT_ID", "11111111-2222-3333-4444-555555555555")

	cfg, err := Load(nonexistentPath(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := cfg.Headers["clientId"]
	if got != "11111111-2222-3333-4444-555555555555" {
		t.Fatalf("clientId header = %q, want the GUID from CONNECTWISE_AUTOMATE_CLIENT_ID", got)
	}
}

// TestClientIDHeaderAbsentWhenUnset confirms older servers (pre-2020.11) that
// don't require clientId aren't forced to send an empty header.
func TestClientIDHeaderAbsentWhenUnset(t *testing.T) {
	t.Setenv("CONNECTWISE_AUTOMATE_CLIENT_ID", "")

	cfg, err := Load(nonexistentPath(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := cfg.Headers["clientId"]; ok {
		t.Fatalf("clientId header should be absent when CONNECTWISE_AUTOMATE_CLIENT_ID is unset, got %q", cfg.Headers["clientId"])
	}
}

// TestServerTemplateVarResolves confirms the per-server base URL resolves from
// CONNECTWISE_AUTOMATE_SERVER and is normalized (scheme/trailing slash dropped).
func TestServerTemplateVarResolves(t *testing.T) {
	t.Setenv("CONNECTWISE_AUTOMATE_SERVER", "https://acme.hostedrmm.com/")

	cfg, err := Load(nonexistentPath(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.TemplateVars["server"]; got != "acme.hostedrmm.com" {
		t.Fatalf("server template var = %q, want normalized host acme.hostedrmm.com", got)
	}
}

// TestServerTemplateVarDefault confirms the placeholder default applies when the
// env var is unset, so doctor still has a real-shaped URL to probe.
func TestServerTemplateVarDefault(t *testing.T) {
	t.Setenv("CONNECTWISE_AUTOMATE_SERVER", "")

	cfg, err := Load(nonexistentPath(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.TemplateVars["server"]; got != "YOUR_SERVER.hostedrmm.com" {
		t.Fatalf("server template var default = %q, want YOUR_SERVER.hostedrmm.com", got)
	}
}
