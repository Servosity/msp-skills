// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"
)

func TestAgentsDossierFullProfile(t *testing.T) {
	agents := []map[string]any{
		agentFix("agent-1", "WIN-DC01", "SiteA", "24.1.2.6", map[string]any{
			"uuid":   "uuid-dc01",
			"osName": "Windows Server 2022",
		}),
	}
	threats := []map[string]any{
		threatFix("t1", "BadDll", "deadbeef", "WIN-DC01", "SiteA", map[string]any{
			"analystVerdict":   "malicious",
			"mitigationStatus": "active",
		}),
	}
	db := newSeededStore(t, agents, threats)
	seedResource(t, db, "activities", []map[string]any{
		{
			"id":                 "act-1",
			"agentId":            "agent-1",
			"primaryDescription": "Agent WIN-DC01 reported a threat",
			"createdAt":          time.Now().UTC().Format(time.RFC3339),
		},
	})

	out := runJSON(t, newNovelAgentsDossierCmd(jsonFlags()), "--db", db, "WIN-DC01")

	if m, _ := out["matches"].(float64); m != 1 {
		t.Fatalf("matches = %v, want 1", out["matches"])
	}
	agent := out["agent"].(map[string]any)
	if agent["uuid"] != "uuid-dc01" {
		t.Fatalf("agent uuid = %v, want uuid-dc01", agent["uuid"])
	}
	if agent["os_name"] != "Windows Server 2022" {
		t.Fatalf("agent os_name = %v, want Windows Server 2022", agent["os_name"])
	}
	threatsOut := out["threats"].([]any)
	if len(threatsOut) != 1 {
		t.Fatalf("threats len = %d, want 1", len(threatsOut))
	}
	if threatsOut[0].(map[string]any)["threat"] != "BadDll" {
		t.Fatalf("threat name = %v, want BadDll", threatsOut[0])
	}
	acts := out["recent_activities"].([]any)
	if len(acts) != 1 {
		t.Fatalf("recent_activities len = %d, want 1", len(acts))
	}
}

func TestAgentsDossierUnknownIsHonestEmpty(t *testing.T) {
	agents := []map[string]any{
		agentFix("agent-1", "WIN-DC01", "SiteA", "24.1.2.6", nil),
	}
	db := newSeededStore(t, agents, nil)
	out := runJSON(t, newNovelAgentsDossierCmd(jsonFlags()), "--db", db, "NO-SUCH-HOST")

	if c, _ := out["count"].(float64); c != 0 {
		t.Fatalf("expected honest-empty count 0, got %v", out)
	}
	if out["query"] != "NO-SUCH-HOST" {
		t.Fatalf("expected query echoed, got %v", out["query"])
	}
	if r, _ := out["reason"].(string); r == "" {
		t.Fatalf("expected a non-empty reason field, got %v", out)
	}
	// No fabricated agent object.
	if _, ok := out["agent"]; ok {
		t.Fatalf("honest-empty must not include an agent object: %v", out)
	}
}
