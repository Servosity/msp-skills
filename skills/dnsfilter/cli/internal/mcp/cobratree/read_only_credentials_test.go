// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "mcp-read-only-withholds-credentials" entry.

package cobratree

import (
	"testing"

	"dnsfilter-pp-cli/internal/cliutil"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
)

// TestReadOnlyMirrorSkipsCredentialCommands: a mirrored command that calls a
// credential path (here the uninstall PIN, annotated mcp:read-only like the
// generated endpoint command) is not registered under the read-only switch
// and is refused at call time if it was registered before the switch flipped.
func TestReadOnlyMirrorSkipsCredentialCommands(t *testing.T) {
	newRoot := func() (*cobra.Command, *cobra.Command) {
		root := &cobra.Command{Use: "dnsfilter-cli"}
		pin := &cobra.Command{
			Use:         "pin-lookup",
			RunE:        func(*cobra.Command, []string) error { return nil },
			Annotations: map[string]string{"pp:method": "GET", "pp:path": "/v1/user_agents/uninstall_pin", ReadOnlyAnnotation: "true"},
		}
		plain := &cobra.Command{
			Use:         "site-lookup",
			RunE:        func(*cobra.Command, []string) error { return nil },
			Annotations: map[string]string{"pp:method": "GET", "pp:path": "/v1/networks", ReadOnlyAnnotation: "true"},
		}
		root.AddCommand(pin, plain)
		return root, pin
	}
	fakeCLI := func() (string, error) { return "/nonexistent/dnsfilter-cli", nil }

	t.Setenv(cliutil.MCPReadOnlyEnvVar, "")
	root, pin := newRoot()
	s := server.NewMCPServer("t", "t")
	RegisterAll(s, root, fakeCLI)
	if s.GetTool("pin_lookup") == nil || s.GetTool("site_lookup") == nil {
		t.Fatalf("default mode should mirror both commands: %v", s.ListTools())
	}

	t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
	if msg := mirroredMCPReadOnlyRefusal(pin, nil); msg == "" {
		t.Fatal("call-time guard let a credential command run under read-only")
	}
	root, _ = newRoot()
	s = server.NewMCPServer("t", "t")
	RegisterAll(s, root, fakeCLI)
	if s.GetTool("pin_lookup") != nil {
		t.Fatal("read-only mode mirrored a credential command")
	}
	if s.GetTool("site_lookup") == nil {
		t.Fatal("read-only mode dropped a plain read")
	}
}

// TestReadOnlyMirrorRefusesCredentialResourceArgs: `tail api-keys` (and any
// mirrored call naming a credential resource) is refused under the read-only
// switch before the companion CLI runs; other resources still run.
func TestReadOnlyMirrorRefusesCredentialResourceArgs(t *testing.T) {
	tail := &cobra.Command{Use: "tail", Annotations: map[string]string{ReadOnlyAnnotation: "true"}}
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
	for _, args := range []map[string]any{
		{"resource": "api-keys"},
		{"positional-resource": "API_KEYS"},
		{"args": "api-keys"},
		{"resources": "networks,api-keys"},
		{"type": []any{"networks", "uninstall-pin"}},
	} {
		if msg := mirroredMCPReadOnlyRefusal(tail, args); msg == "" {
			t.Fatalf("read-only let %v through", args)
		}
	}
	if msg := mirroredMCPReadOnlyRefusal(tail, map[string]any{"resource": "networks", "follow": false}); msg != "" {
		t.Fatalf("read-only refused a plain resource: %s", msg)
	}
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "")
	if msg := mirroredMCPReadOnlyRefusal(tail, map[string]any{"resource": "api-keys"}); msg != "" {
		t.Fatalf("default mode refused: %s", msg)
	}
}
