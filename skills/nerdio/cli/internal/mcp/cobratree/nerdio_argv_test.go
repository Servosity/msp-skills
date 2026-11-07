// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cobratree

import (
	"context"
	"strings"
	"testing"

	"nerdio-pp-cli/internal/cliutil"
	"nerdio-pp-cli/internal/credkit"
)

// Class held-value-crosses-process-boundary (Codex r2): the child CLI never
// receives an argv that repeats a credential this MCP server holds (the
// credential kit refuses it, credkit.RunCommand) or a tenant secret it showed
// (handfixes.json tenant-secrets-shown-never-stored), whatever form the tool
// arguments took before the mirror formatted them (a JSON number the mirror
// prints as float64 included).
func TestChildArgvNeverRepeatsAHeldValue(t *testing.T) {
	const session = "10000000000000000000000000000000"
	argv := append([]string{"teach", "--query=find application"}, cliArgsFromMCP(map[string]any{"notes": float64(1e31)}, nil)...)
	if !strings.Contains(strings.Join(argv, " "), session) {
		t.Fatalf("test setup: the mirror's argv %v does not carry the value", argv)
	}

	hold := cliutil.Hold
	cliutil.Hold = func(credkit.Source, ...string) {}
	cliutil.ShowTenantSecrets([]byte(`{"password":"` + session + `"}`))
	cliutil.Hold = hold
	_, err := RunCLICommand(context.Background(), "/nonexistent/nerdio-cli", argv)
	if err == nil || !strings.Contains(err.Error(), "repeats a tenant secret") || strings.Contains(err.Error(), session) {
		t.Fatalf("a tenant secret in argv: want the refusal without the value, got %v", err)
	}

	const token = "ChildArgvHeldToken-5d1e9b"
	credkit.Hold("minted:test", token)
	_, err = RunCLICommand(context.Background(), "/nonexistent/nerdio-cli", []string{"teach", "--notes", "x" + token})
	if err == nil || !strings.Contains(err.Error(), "credkit: withheld") || strings.Contains(err.Error(), token) {
		t.Fatalf("a held credential in argv: want the kit's refusal without the value, got %v", err)
	}
}
