// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated). The earlier credential layer's false-withheld
// classes, re-tested under the shared credential kit: GET /workflows/integrations
// returns a secrets_schema keyed by the secrets it describes ("Authentication
// Headers"), and the schema definitions must print; agent-context must print its
// auth descriptor. Driven through the built CLI.

package credwire

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// workflowIntegrationsPage is spec.json's WorkflowIntegrationExample
// secrets_schema as documented, in one cursor page.
const workflowIntegrationsPage = `{"integrations":[{"id":"PWFI001","type":"workflow_integration","name":"HQ-Record Web API",` +
	`"secrets_schema":{"$schema":"https://json-schema.org/draft/2020-12/schema","$id":"https://api.pagerduty.com/workflows/integrations/http-api/schemas/secrets.json",` +
	`"title":"HQ-Record secrets schema","description":"The secrets of a workflow integration connection to Web API","type":"object",` +
	`"properties":{"Authentication Headers":{"type":"text","description":"Enter the headers to include in Web API calls made with this connection, one per line.",` +
	`"is_hidden":false,"advanced":false,"metadata":"{\"format\":\"textarea\"}","default_value":null,"restrictions":null,"display_order":1,"format":"TEXTAREA"}},` +
	`"required":["Authentication Headers"]}}],"limit":25,"next_cursor":null}`

func TestWorkflowIntegrationSecretsSchemaPrints(t *testing.T) {
	srv, ca, seen := kitServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/workflows/integrations" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, workflowIntegrationsPage)
	})
	out := runKitCLI(t, srv, ca, "workflows", "list-integrations", "--json")
	if *seen == 0 {
		t.Fatalf("the CLI never sent the key to the fake, so nothing was tested:\n%s", out)
	}
	for _, want := range []string{"Authentication Headers", "TEXTAREA", "HQ-Record secrets schema"} {
		if !strings.Contains(out, want) {
			t.Errorf("the secrets_schema definition was withheld (no %q):\n%.3000s", want, out)
		}
	}
	if strings.Contains(out, "[credkit: withheld") {
		t.Errorf("a unit was withheld:\n%.3000s", out)
	}
}

// TestAgentContextPrintsWithAKey: the command an agent reads first prints whole
// with a key held, in its default and --pretty forms, its auth descriptor
// (which names the variable and never carries a value) included. Over this
// document the encoder's HTML escapes alone spent the kit's budget, so the whole
// output was withheld (handfixes.json agent-context-prints-with-a-key).
func TestAgentContextPrintsWithAKey(t *testing.T) {
	srv, ca, _ := kitServer(t, func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	for _, args := range [][]string{{"agent-context"}, {"agent-context", "--pretty"}} {
		out := runKitCLI(t, srv, ca, args...)
		if strings.Contains(out, "[credkit: withheld") {
			t.Fatalf("%v was withheld:\n%.1000s", args, out)
		}
		var doc struct {
			Auth struct {
				Mode    string `json:"mode"`
				EnvVars []struct {
					Name string `json:"name"`
				} `json:"env_vars"`
			} `json:"auth"`
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &doc); err != nil {
			t.Fatalf("%v did not print one document: %v\n%.1000s", args, err, out)
		}
		if doc.Auth.Mode != "api_key" || len(doc.Auth.EnvVars) == 0 || doc.Auth.EnvVars[0].Name != "PAGERDUTY_API_KEY" {
			t.Errorf("%v printed no auth descriptor: %+v", args, doc.Auth)
		}
	}
}
