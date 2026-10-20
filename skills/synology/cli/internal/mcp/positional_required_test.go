// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED. Pins HAND-FIX(required-positionals) on the MCP side.

package mcp

import (
	"os"
	"sort"
	"strings"
	"testing"
)

// TestMirroredPositionalsAreRequired is the review's mt repro: abb_triage
// and abb_device need their positional (the task, the host name) but their
// schemas did not require it, so a call without it ran the command bare and
// returned its help text with isError false (abb_result_detail refused). For
// every Cobra-mirror tool with a positional: a positional its command needs
// ("<name>" in Use) is in the schema's required list and a call without it
// is refused before the CLI runs; a tool whose positionals are all optional
// never answers a call without them with the command's help as a success.
func TestMirroredPositionalsAreRequired(t *testing.T) {
	s := sweepMCPServer(t, nil, "")
	t.Setenv("SYNOLOGY_CLI_PATH", os.Args[0])
	t.Setenv(actAsCLIEnv, "1")
	tools := s.ListTools()
	var names []string
	for name := range tools {
		names = append(names, name)
	}
	sort.Strings(names)
	requiredSeen := map[string]bool{}
	for _, name := range names {
		schema := tools[name].Tool.InputSchema
		req := map[string]bool{}
		for _, r := range schema.Required {
			req[r] = true
		}
		var required, optional []string
		for prop, raw := range schema.Properties {
			p, _ := raw.(map[string]any)
			desc, _ := p["description"].(string)
			display, ok := strings.CutPrefix(desc, "Positional argument ")
			if !ok {
				continue
			}
			if strings.HasPrefix(display, "<") {
				required = append(required, prop)
				if !req[prop] {
					t.Errorf("%s: positional %s (%s) is required by its command but not by the schema (required %v)", name, prop, display, schema.Required)
				}
			} else {
				optional = append(optional, prop)
			}
		}
		if len(required) == 0 && len(optional) == 0 {
			continue
		}
		text, isErr := callViaServer(t, s, name, map[string]any{})
		switch {
		case len(required) > 0:
			for _, r := range required {
				requiredSeen[name+"."+r] = true
			}
			if !isErr || !strings.Contains(text, "requires") || !strings.Contains(text, "nothing was sent") {
				t.Errorf("%s {} : isErr=%v text=%.200s, want the required-argument refusal", name, isErr, text)
			}
		case !isErr && strings.Contains(text, "Usage:"):
			t.Errorf("%s {} : answered with the command's help as a success (isError false); a positional the command needs must be declared <...>:\n%.300s", name, text)
		}
	}
	for _, want := range []string{"abb_triage.task", "abb_device.host-name"} {
		if !requiredSeen[want] {
			t.Errorf("%s is not a required positional (seen: %v)", want, requiredSeen)
		}
	}
}
