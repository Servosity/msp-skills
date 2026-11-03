// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package domotzblackbox

// Codex review round 1 (liongard 4.34.2 reprint, the shared credential layer)
// class holder-incomplete, end to end on domotz: a credential that is a
// JSON number under a credential name (the liongard input was
// {"password":12345678}) was printed by the MCP confirmation preview and the
// CLI --dry-run preview. Domotz's own shapes are a device credentials
// password that is a number (PUT .../credentials), a numeric SNMP community
// (PUT .../snmp-community), a team leader's numeric password (POST
// /area/{area_id}/team, the password nested under leader) and a numeric
// custom-driver credentials password (a nested credentials container); the
// import path posts a record as given, so a numeric password there is the
// same class. Under the credential kit a CLI preview that carries one is
// withheld whole (an --input body is held by cliutil.HoldBody, a --stdin body
// is held whole) and an MCP preview masks it (mcp-endpoint-write-confirm).
// The liongard-only half of that test (a number under an
// inspector field name inside a launchpoint's SecureConfig container) has no
// Domotz counterpart: no Domotz request or response carries a vendor-marked
// secure container.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNumericCredentialValuesNeverPrint(t *testing.T) {
	t.Parallel()
	const named = "48261937" // under a credential name (password, read, snmp_read_community)
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	imp := filepath.Join(e.home, "numeric.jsonl")
	if err := os.WriteFile(imp, []byte(`{"name":"HQ numeric","color":"red","password":`+named+`}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	out.WriteString(e.run(t, "import", "custom-tag", "--input", imp, "--dry-run"))
	out.WriteString(e.runIn(t, `{"username":"HQ numeric","password":`+named+`}`,
		"agent", "device", "set-credentials", bbAgentID, bbDeviceID, "--stdin", "--dry-run"))
	out.WriteString(e.runIn(t, `{"read":`+named+`,"write":`+named+`}`,
		"agent", "device", "set-snmp-community", bbAgentID, bbDeviceID, "--stdin", "--dry-run"))
	out.WriteString(e.runIn(t, `{"version":"V2","snmp_read_community":`+named+`}`,
		"agent", "device", "set-snmpauthentication", bbAgentID, bbDeviceID, "--stdin", "--dry-run"))
	out.WriteString(e.runIn(t, `{"name":"HQ numeric","leader":{"name":"lead","password":`+named+`,"details":{"display_name":"Lead"}}}`,
		"area", "team", "create", bbAreaID, "--stdin", "--dry-run"))
	out.WriteString(e.runIn(t, `{"sample_period":300,"credentials":{"username":"hq","password":`+named+`}}`,
		"custom-driver", "agent", "create-custom-driver-association", bbDriverID, bbAgentID, bbDeviceID, "--stdin", "--dry-run"))
	out.WriteString(e.mcpCalls(t, []map[string]any{
		execute("agent.device.set-credentials", onDevice(map[string]any{"username": "hq-admin", "password": 48261937})),
		execute("agent.device.set-snmp-community", onDevice(map[string]any{"read": 48261937})),
		execute("area.team.create", map[string]any{"area_id": bbAreaID, "name": "HQ numeric",
			"leader": map[string]any{"name": "lead", "password": 48261937, "details": map[string]any{"display_name": "Lead"}}}),
		execute("custom-driver.agent.create-custom-driver-association", map[string]any{"custom_driver_id": bbDriverID, "agent_id": bbAgentID,
			"device_id": bbDeviceID, "credentials": map[string]any{"username": "hq-driver", "password": 48261937}}),
	}))
	got := out.String()
	// Three MCP previews mask the number; a number under bare read is a count
	// to the mask (cliutil.NumberIsCredential), so that preview carries the
	// held value and the kit withholds it whole.
	if !strings.Contains(got, "HQ numeric") || !strings.Contains(got, "dry_run") || strings.Count(got, "confirmation_required") != 3 ||
		!strings.Contains(got, "[credkit: withheld from mcp") {
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
