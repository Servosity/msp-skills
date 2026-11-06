// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json mcp-endpoint-write-confirm,
// mcp-mirror-write-confirm, mcp-import-hidden): an MCP write previews and
// sends nothing until confirm is the boolean true, on the built MCP server;
// import is not an MCP tool at all. Not a credential class: it moved here from
// the retired black-box package, which held the print's own credential layer
// tests too.

package credwire

import (
	"encoding/json"
	"strings"
	"testing"
)

// previewed reports whether reply is a write tool's preview: the marker, in a
// result that is not an error (a usage error is not a preview).
func previewed(reply, marker string) bool {
	var msg struct {
		Result *struct {
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	if json.Unmarshal([]byte(reply), &msg) != nil || msg.Result == nil || msg.Result.IsError {
		return false
	}
	return strings.Contains(reply, marker)
}

func TestMCPWriteSendsOnlyWithConfirmTrue(t *testing.T) {
	base := "/api/v1/tenants/" + fxTenant
	protect := func() map[string]any {
		return map[string]any{"tenant_id": fxTenant, "resource_id": fxResource, "policy_id": fxPolicy}
	}
	tools := []struct {
		name, marker string
		args         func() map[string]any
		want         []string // exactly the requests confirm true sends, in order
	}{
		{"orgs_create", "confirmation_required", func() map[string]any {
			return map[string]any{"name": "HQ-MCPOrg", "admin_email": "admin@corp.example"}
		}, []string{"POST /api/v1/orgs"}},
		{"tenants_archives_delete", "confirmation_required", func() map[string]any {
			return map[string]any{"tenant_id": fxTenant, "id": fxArchive, "reason": "HQ retire"}
		}, []string{"DELETE " + base + "/archives/" + fxArchive}},
		{"tenants_jobs_tasks-trigger", "confirmation_required", func() map[string]any {
			return map[string]any{"tenant_id": fxTenant, "job_id": fxJob}
		}, []string{"PUT " + base + "/jobs/" + fxJob + "/trigger"}},
		{"tenants_resources_protections-protect", "confirmation_required", protect,
			[]string{"POST " + base + "/resources/" + fxResource + "/protect"}},
		{"tenants_resources_protections-unprotect", "confirmation_required", protect,
			[]string{"DELETE " + base + "/resources/" + fxResource + "/protect"}},
		{"offboard", "Preview only: nothing was sent.", func() map[string]any {
			return map[string]any{"resource-id": fxResource, "tenant": fxTenant, "poll-interval": "10ms", "reason": "HQ departure"}
		}, []string{
			"GET " + base + "/protections",
			"PUT " + base + "/jobs/" + fxJob + "/trigger",
			"GET " + base + "/tasks/" + fxTask,
			"GET " + base + "/archives",
			"DELETE " + base + "/resources/" + fxResource + "/protect",
		}},
	}
	for _, tool := range tools {
		t.Run(tool.name, func(t *testing.T) {
			e := newEnv(t, afiFake, "AFI_API_KEY="+dryRunKey)
			var calls []map[string]any
			for _, confirm := range []any{nil, false, "true", 1, "yes"} {
				args := tool.args()
				if confirm != nil {
					args["confirm"] = confirm
				}
				calls = append(calls, map[string]any{"name": tool.name, "arguments": args})
			}
			replies, out := e.mcp(t, calls...)
			for i := range calls {
				if r := replies[i+1]; !previewed(r, tool.marker) {
					t.Errorf("call %d (%v) returned no preview: %.800s", i+1, calls[i]["arguments"], r)
				}
			}
			if got := e.api.all(); len(got) != 0 {
				t.Fatalf("an unconfirmed MCP write reached the API: %+v\n%.2000s", got, out)
			}
			args := tool.args()
			args["confirm"] = true
			replies, out = e.mcp(t, map[string]any{"name": tool.name, "arguments": args})
			var got []string
			for _, s := range e.api.all() {
				got = append(got, s.Method+" "+s.Path)
				if strings.Contains(s.Body, "confirm") {
					t.Errorf("the confirm argument reached the API: %s", s.Body)
				}
				if s.Header.Get("Authorization") != dryRunKey {
					t.Errorf("%s %s went without the key as Authorization", s.Method, s.Path)
				}
			}
			if strings.Join(got, "|") != strings.Join(tool.want, "|") {
				t.Fatalf("confirm true: want exactly %v, got %v\n%.2000s", tool.want, got, out)
			}
			if previewed(replies[1], tool.marker) {
				t.Errorf("confirm true still previewed: %.800s", replies[1])
			}
		})
	}
}

// TestMCPImportIsHidden: import's required --input names a host file, which
// the mirror refuses, so it is no MCP tool; a call by name, confirmed or not,
// reaches nothing.
func TestMCPImportIsHidden(t *testing.T) {
	e := newEnv(t, afiFake, "AFI_API_KEY="+dryRunKey)
	replies, out := e.mcp(t,
		map[string]any{"name": "import", "arguments": map[string]any{"resource": "orgs"}},
		map[string]any{"name": "import", "arguments": map[string]any{"resource": "orgs", "confirm": true}},
	)
	for i := 1; i <= 2; i++ {
		if previewed(replies[i], "Preview only") || previewed(replies[i], "confirmation_required") {
			t.Errorf("a hidden import answered call %d with a preview: %.800s", i, replies[i])
		}
	}
	if got := e.api.all(); len(got) != 0 {
		t.Fatalf("a hidden import sent %+v\n%.2000s", got, out)
	}
}
