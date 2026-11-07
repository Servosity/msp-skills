// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package liongardblackbox

// Codex review round 1 (liongard 4.34.2 reprint) class holder-incomplete,
// end to end: a SecureConfig credential that is a JSON number (Codex's input:
// liongard_execute launchpoints.launchpoint with SecureConfig
// {"password":12345678}) was printed by the MCP confirmation preview and the
// CLI --dry-run preview. Seen failing on the pre-fix code.

import (
	"strings"
	"testing"
)

func TestNumericSecureConfigValuesNeverPrint(t *testing.T) {
	const (
		named   = "48261937" // under a credential name (password)
		unnamed = "59372816" // under an inspector field name that does not read as one
	)
	secure := `{"password":` + named + `,"ServicePin":` + unnamed + `}`
	sv := echoServer(t)
	e := newEnv(t, sv, credEnvKey)
	var out strings.Builder
	out.WriteString(e.run(t, append(launchpointFlags("HQ lp"), "--secure-config", secure, "--dry-run")...))
	out.WriteString(e.mcpCalls(t, []map[string]any{
		{"name": "liongard_execute", "arguments": map[string]any{"endpoint_id": "launchpoints.launchpoint",
			"params": map[string]any{"Alias": "HQ lp", "SecureConfig": map[string]any{"password": 48261937, "ServicePin": 59372816}}}},
	}))
	got := out.String()
	// Every preview repeats the SecureConfig it was given, so the shared
	// credential kit withholds each whole: its notice shows they ran.
	if !strings.Contains(got, kitNotice) {
		t.Fatalf("the SecureConfig previews did not run:\n%.3000s", got)
	}
	for _, v := range []string{named, unnamed} {
		if strings.Contains(got, v) {
			t.Errorf("output repeats the numeric SecureConfig value %s", v)
		}
	}
}
