// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json mcp-preview-never-mints,
// mcp-endpoint-write-confirm): an MCP write previews and sends nothing, the token
// mint included, until confirm is the boolean true, on the built MCP server.

package credwire

import (
	"net/http"
	"strings"
	"testing"
)

func TestMCPWriteSendsOnlyWithConfirmTrue(t *testing.T) {
	t.Parallel()
	exec := func(id string, params map[string]any, confirm any) map[string]any {
		args := map[string]any{"endpoint_id": id, "params": params}
		if confirm != nil {
			args["confirm"] = confirm
		}
		return map[string]any{"name": "datto-rmm_execute", "arguments": args}
	}
	update := map[string]any{"siteUid": "site-1042", "name": "HQ map"}
	e := newEnv(t, nil, keyPair...)
	var calls []map[string]any
	for _, confirm := range []any{nil, false, "true", 1, "yes"} {
		calls = append(calls, exec("site.update", update, confirm))
	}
	for id, params := range map[string]map[string]any{
		"site.create":                      {"name": "Preview Site"},
		"account.create-variable":          {"name": "previewVar", "value": "x"},
		"device.quickjob.create-quick-job": {"deviceUid": "dev-5150", "jobName": "Preview"},
		"alert.resolve.alert":              {"alertUid": "alert-7731"},
		"user.reset-api-keys":              {},
		"site.variable.delete-site":        {"siteUid": "site-1042", "variableId": "7"},
	} {
		calls = append(calls, exec(id, params, nil))
	}
	replies, out := e.mcp(t, calls...)
	for i := range calls {
		if r := replies[i+1]; !strings.Contains(r, "confirmation_required") {
			t.Errorf("call %d (%v) returned no preview: %.800s", i+1, calls[i]["arguments"], r)
		}
	}
	if got := e.api.all(); len(got) != 0 {
		t.Fatalf("an unconfirmed MCP write reached the API: %+v\n%.2000s", got, out)
	}
	if got := e.mints.all(); len(got) != 0 {
		t.Fatalf("an unconfirmed MCP write minted a token: %+v\n%.2000s", got, out)
	}
	replies, out = e.mcp(t, exec("site.update", update, true))
	got := e.api.all()
	if len(got) != 1 || got[0].Method != http.MethodPost || got[0].Path != "/api/v2/site/site-1042" {
		t.Fatalf("confirm true: want one POST, got %+v\n%.2000s", got, out)
	}
	if strings.Contains(got[0].Body, "confirm") {
		t.Errorf("the confirm argument reached the API: %s", got[0].Body)
	}
	if strings.Contains(replies[1], "confirmation_required") {
		t.Errorf("confirm true still previewed: %.800s", replies[1])
	}
}

// TestMCPMirroredWritesPreview: the mirrored import and fleet resolve-storm
// preview too.
func TestMCPMirroredWritesPreview(t *testing.T) {
	t.Parallel()
	e := newEnv(t, nil, keyPair...)
	replies, out := e.mcp(t,
		map[string]any{"name": "import", "arguments": map[string]any{"resource": "user"}},
		map[string]any{"name": "fleet_resolve_storm", "arguments": map[string]any{}},
	)
	for i := 1; i <= 2; i++ {
		if !strings.Contains(replies[i], "confirmation_required") && !strings.Contains(replies[i], "Preview only: nothing was sent") {
			t.Errorf("call %d returned no preview: %.800s", i, replies[i])
		}
	}
	for what, rec := range map[string]*recorder{"the API": e.api, "the token endpoint": e.mints} {
		if got := rec.all(); len(got) != 0 {
			t.Errorf("an unconfirmed mirrored write reached %s: %+v\n%.2000s", what, got, out)
		}
	}
}
