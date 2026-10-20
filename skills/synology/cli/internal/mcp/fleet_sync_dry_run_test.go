// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED.

package mcp

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"synology-pp-cli/internal/cli"
)

// TestMCPFleetSyncDryRunNamesEachNAS: fleet_sync with dry-run answered only
// "would run fleet sync". It names each registered NAS it would contact and
// the credential env-var names it would read (never values), honours nas,
// and sends nothing.
func TestMCPFleetSyncDryRunNamesEachNAS(t *testing.T) {
	nas := newFakeDSM(t, "File Server")
	t.Setenv("SYNOLOGY_CLI_PATH", os.Args[0])
	t.Setenv(actAsCLIEnv, "1")
	s := sweepMCPServer(t, nil, "")
	t.Setenv("SYNOLOGY_ACME_ACCOUNT", "msp-admin")
	t.Setenv("SYNOLOGY_ACME_PASSWORD", "PWDRYRUN-MCP-77")
	for _, args := range [][]string{
		{"fleet", "add", "acme", "--url", nas.srv.URL, "--client", "Acme Corp", "--json"},
		{"fleet", "add", "lab", "--url", "https://192.0.2.20:5001", "--json"},
	} {
		root := cli.RootCmd()
		root.SetArgs(args)
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		if err := root.Execute(); err != nil {
			t.Fatalf("%v: %v: %s", args, err, out.String())
		}
	}
	text, isErr := callTool(t, s, "fleet_sync", map[string]any{"dry-run": true})
	if isErr || !strings.Contains(text, `"acme"`) || !strings.Contains(text, `"lab"`) || !strings.Contains(text, "SYNOLOGY_ACME_PASSWORD") ||
		!strings.Contains(text, "SYNOLOGY_LAB_PASSWORD") || !strings.Contains(text, "nothing was sent") {
		t.Fatalf("fleet_sync dry-run: isErr=%v %s", isErr, text)
	}
	if strings.Contains(text, "PWDRYRUN-MCP-77") {
		t.Fatalf("the plan printed a password: %s", text)
	}
	text, isErr = callTool(t, s, "fleet_sync", map[string]any{"dry-run": true, "nas": "lab"})
	if isErr || strings.Contains(text, `"acme"`) || !strings.Contains(text, `"lab"`) {
		t.Fatalf("fleet_sync dry-run nas=lab: isErr=%v %s", isErr, text)
	}
	if len(nas.Calls()) != 0 {
		t.Fatalf("a dry run reached the NAS: %+v", nas.Calls())
	}
}
