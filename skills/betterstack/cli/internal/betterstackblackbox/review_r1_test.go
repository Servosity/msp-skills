// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package betterstackblackbox

// Class holder-incomplete, end to end: a credential that is a JSON number
// under a credential name was printed by the MCP confirmation preview and the
// CLI --dry-run preview. Better Stack's shapes: a monitor whose auth_password
// (the monitored site's basic-auth password) is a number, and an escalation
// policy whose incident_token is one. A CLI preview that carries one is
// withheld whole by the credential kit (an --input body is held by
// cliutil.HoldBody before it prints, a --stdin body is held whole); an MCP
// preview masks it (mcp-endpoint-write-confirm).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNumericCredentialValuesNeverPrint(t *testing.T) {
	const named = "48261937" // under a credential name (auth_password, incident_token)
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	imp := filepath.Join(e.home, "numeric.jsonl")
	if err := os.WriteFile(imp, []byte(`{"monitor_type":"status","pronounceable_name":"HQ numeric","auth_password":`+named+`}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	out.WriteString(e.run(t, "import", "monitors", "--input", imp, "--dry-run"))
	out.WriteString(e.runIn(t, `{"monitor_type":"status","pronounceable_name":"HQ numeric","auth_password":`+named+`}`,
		"monitors", "create", "--stdin", "--dry-run"))
	out.WriteString(e.runIn(t, `{"name":"HQ numeric","incident_token":`+named+`}`, "policies", "create", "--stdin", "--dry-run"))
	out.WriteString(e.mcpCalls(t, []map[string]any{
		{"name": "monitors_create", "arguments": map[string]any{"type": "status", "name": "HQ numeric", "auth_password": 48261937}},
		{"name": "policies_create", "arguments": map[string]any{"name": "HQ numeric", "incident_token": 48261937}},
	}))
	got := out.String()
	if !strings.Contains(got, "HQ numeric") || !strings.Contains(got, "dry_run") || strings.Count(got, "confirmation_required") != 2 {
		t.Fatalf("the previews did not run:\n%.3000s", got)
	}
	if strings.Contains(got, named) {
		t.Errorf("output repeats the numeric credential %s:\n%.3000s", named, got)
	}
	sv.mu.Lock()
	defer sv.mu.Unlock()
	if len(sv.methods) != 0 {
		t.Errorf("a preview sent %v", sv.methods)
	}
}
