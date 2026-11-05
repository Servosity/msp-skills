// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json mcp-endpoint-write-confirm,
// retire-stdin-dry-run): an MCP write previews and sends nothing until confirm
// is the boolean true, on the built MCP server. Not a credential class: it
// moved here from the retired black-box package, which held the print's own
// credential layer tests too.

package credwire

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestMCPWriteSendsOnlyWithConfirmTrue(t *testing.T) {
	retire := func() map[string]any { return map[string]any{"reason": "decommissioned", "client_id": "cid-1042"} }
	tools := []struct {
		name, path string
		args       func() map[string]any
	}{
		{"backups_ingest", "/backup-radar/v3/backups", func() map[string]any { return map[string]any{"body_json": ingestBody} }},
		{"backups_retire", "/backup-radar/v3/backups/retire", retire},
	}
	for _, tool := range tools {
		t.Run(tool.name, func(t *testing.T) {
			e := newEnv(t, nil, "BACKUP_RADAR_API_KEY="+dryRunKey)
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
				if r := replies[i+1]; !strings.Contains(r, "confirmation_required") {
					t.Errorf("call %d (%v) returned no preview: %.800s", i+1, calls[i]["arguments"], r)
				}
			}
			if got := e.api.all(); len(got) != 0 {
				t.Fatalf("an unconfirmed MCP write reached the API: %+v\n%.2000s", got, out)
			}
			args := tool.args()
			args["confirm"] = true
			replies, out = e.mcp(t, map[string]any{"name": tool.name, "arguments": args})
			got := e.api.all()
			if len(got) != 1 || got[0].Method != http.MethodPost || got[0].Path != tool.path {
				t.Fatalf("confirm true: want one POST %s, got %+v\n%.2000s", tool.path, got, out)
			}
			if strings.Contains(got[0].Body, "confirm") {
				t.Errorf("the confirm argument reached the API: %s", got[0].Body)
			}
			if strings.Contains(replies[1], "confirmation_required") {
				t.Errorf("confirm true still previewed: %.800s", replies[1])
			}
		})
	}
}

// TestMCPServerDryRunIsTheVendorsPreview: the MCP retire tool names the vendor's
// own preview server_dry_run (sent as the body field dry_run), and it is still a
// write that needs confirm true; nothing on the tool is called dry_run.
func TestMCPServerDryRunIsTheVendorsPreview(t *testing.T) {
	e := newEnv(t, nil, "BACKUP_RADAR_API_KEY="+dryRunKey)
	args := map[string]any{"reason": "decommissioned", "client_id": "cid-1042", "server_dry_run": true}
	replies, out := e.mcp(t, map[string]any{"name": "backups_retire", "arguments": args})
	if !strings.Contains(replies[1], "confirmation_required") || len(e.api.all()) != 0 {
		t.Fatalf("server_dry_run without confirm must preview and send nothing: %.800s\nsent %+v", replies[1], e.api.all())
	}
	args["confirm"] = true
	_, out = e.mcp(t, map[string]any{"name": "backups_retire", "arguments": args})
	got := e.api.all()
	if len(got) != 1 {
		t.Fatalf("want one request, got %+v\n%.2000s", got, out)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(got[0].Body), &body); err != nil || body["dry_run"] != true {
		t.Fatalf("body %s: want dry_run true (err %v)", got[0].Body, err)
	}
	if _, ok := body["server_dry_run"]; ok {
		t.Fatalf("the public name reached the wire: %s", got[0].Body)
	}
}
