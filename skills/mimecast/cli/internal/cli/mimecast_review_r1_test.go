// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written regression tests for the Codex review of the 4.34.2 reprint.

package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"mimecast-pp-cli/internal/cliutil/testenv"
)

// agentRows finds every object in an --agent answer that has field kind,
// at any depth, so the test does not depend on the envelope shape.
func agentRows(v any, out *[]map[string]any) {
	switch t := v.(type) {
	case map[string]any:
		if _, ok := t["kind"]; ok {
			if _, ok := t["source"]; ok {
				*out = append(*out, t)
			}
		}
		for _, x := range t {
			agentRows(x, out)
		}
	case []any:
		for _, x := range t {
			agentRows(x, out)
		}
	}
}

// Codex r1 class sparse-warning-signal-loss: --agent compaction must not drop
// a field a novel view emits on only some rows (userOverride "Continue" is
// documented in spec.yaml clickLogs[].userOverride).
func TestAgentCompactionKeepsSparseNovelFields(t *testing.T) {
	testenv.Isolate(t)
	srv := threatsFake(t, nil, false)
	defer srv.Close()
	out, err := runMimecast(t, srv, "threats", "mailbox", "alice@example.com", "--since", "7d", "--agent")
	if err != nil {
		t.Fatalf("threats mailbox --agent failed: %v\n%s", err, out)
	}
	var v any
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	var rows []map[string]any
	agentRows(v, &rows)
	var click map[string]any
	for _, r := range rows {
		if r["kind"] == "url-click" {
			click = r
		}
	}
	if click == nil {
		t.Fatalf("no url-click event in the --agent answer:\n%s", out)
	}
	if click["clicked_through_warning"] != true {
		t.Fatalf("--agent dropped clicked_through_warning from the click that continued past a warning: %v\n%s", click, out)
	}
	var held map[string]any
	for _, r := range rows {
		if r["kind"] == "held" {
			held = r
		}
	}
	if held == nil || held["held_id"] != "h1" {
		t.Fatalf("--agent dropped held_id from the held event: %v\n%s", held, out)
	}
	if !strings.Contains(out, "url-protect") {
		t.Fatalf("sources missing:\n%s", out)
	}
}
