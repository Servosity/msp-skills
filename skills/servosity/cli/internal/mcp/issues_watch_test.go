// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): the MCP surface of `issues watch` and
// `events list`. Pinned by this connector's handfixes.json under the
// "issues-watch-events" entry.

package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/server"
)

// TestMCPEventsListReadOnlyAndWatchHidden: events_list is a read-only,
// non-destructive tool with no store-path parameter, and the long-running
// watcher is not a tool at all.
func TestMCPEventsListReadOnlyAndWatchHidden(t *testing.T) {
	s := server.NewMCPServer("servosity-msp", "test")
	RegisterTools(s)
	tools := s.ListTools()
	ev, ok := tools["events_list"]
	if !ok {
		t.Fatal("events_list is not registered")
	}
	a := ev.Tool.Annotations
	if a.ReadOnlyHint == nil || !*a.ReadOnlyHint || a.DestructiveHint == nil || *a.DestructiveHint {
		t.Errorf("events_list annotations: %+v", a)
	}
	schema, _ := json.Marshal(ev.Tool.InputSchema)
	for _, want := range []string{`"since"`, `"type"`, `"limit"`} {
		if !strings.Contains(string(schema), want) {
			t.Errorf("events_list schema lacks %s: %s", want, schema)
		}
	}
	if strings.Contains(string(schema), `"db"`) {
		t.Errorf("events_list takes a store path: %s", schema)
	}
	for name := range tools {
		if strings.Contains(name, "watch") && strings.HasPrefix(name, "issues") {
			t.Errorf("the watcher is an MCP tool: %s", name)
		}
	}
}
