// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package autotaskblackbox

// Codex review round 1 (liongard 4.34.2 reprint, the shared credential layer)
// class holder-incomplete, end to end on autotask: a credential that is a JSON
// number under a credential name (the liongard input was
// {"password":12345678}) was printed by the MCP confirmation preview and the
// CLI --dry-run preview. Autotask's own shapes: a client portal user's
// password, a webhook's secretKey and an integration vendor's secret, each a
// number here.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNumericCredentialValuesNeverPrint(t *testing.T) {
	t.Parallel()
	const named = "48261937" // under a credential name (password, secretKey, secret)
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	imp := filepath.Join(e.home, "numeric.jsonl")
	if err := os.WriteFile(imp, []byte(`{"title":"HQ numeric","companyID":77,"password":`+named+`}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	out.WriteString(e.run(t, "import", "tickets", "--input", imp, "--dry-run"))
	out.WriteString(e.runIn(t, `{"referenceTitle":"HQ numeric","secretKey":`+named+`,"password":`+named+`,"secret":`+named+`}`,
		e.api("configuration-items", "create-entity", "--stdin", "--dry-run")...))
	out.WriteString(e.mcpCalls(t, []map[string]any{
		{"name": "autotask_execute", "arguments": map[string]any{"endpoint_id": "configuration-items.create-entity",
			"params": map[string]any{"referenceTitle": "HQ numeric", "secretKey": 48261937, "Secret": 48261937}}},
		{"name": "autotask_execute", "arguments": map[string]any{"endpoint_id": "tickets.create-entity",
			"params": map[string]any{"title": "HQ numeric", "password": 48261937, "ApiIntegrationCode": 48261937}}},
	}))
	got := out.String()
	// The --stdin body is held whole (StdinFlags) and the MCP arguments by
	// name, so a preview that repeats them is withheld whole: the kit's notice.
	if !strings.Contains(got, "HQ numeric") || !strings.Contains(got, "dry run") ||
		(!strings.Contains(got, "confirmation_required") && !strings.Contains(got, "credkit: withheld")) {
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
