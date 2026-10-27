// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"sort"
	"testing"
	"time"
)

// livenessAgents is the reviewer's failing input plus agents whose liveness is
// unknown. SentinelOne's networkStatus is the state of the "Disconnect from
// network" isolation action (spec.yaml POST /agents/actions/disconnect
// isolates an endpoint, and the agent keeps talking to the console), so it is
// set on every agent here to the value that would mislead a reader of it.
func livenessAgents() []map[string]any {
	noIsActive := func(a map[string]any) map[string]any {
		delete(a, "isActive")
		return a
	}
	return []map[string]any{
		// Sets the windows target version; live and not isolated.
		agentFix("100", "TARGET-NEW", "SiteA", "24.1.2.6", map[string]any{
			"osType": "windows", "networkStatus": "connected", "isActive": true}),
		// Powered off for 90 days. Never isolated, so networkStatus still says
		// "connected". This is the seat an MSP pays for and should reclaim.
		agentFix("101", "LAPTOP-OFF-90D", "SiteA", "23.4.1.1", map[string]any{
			"osType": "windows", "networkStatus": "connected", "isActive": false, "lastActiveDate": daysAgo(90)}),
		// Isolated right now, still checking in: live, not a seat to reclaim.
		agentFix("102", "SRV-ISOLATED-NOW", "SiteA", "23.4.1.1", map[string]any{
			"osType": "windows", "networkStatus": "disconnected", "isActive": true, "lastActiveDate": daysAgo(0)}),
		// No isActive at all: liveness unknown. One reads "connected", one
		// reads "disconnected"; neither may be counted online or listed.
		noIsActive(agentFix("103", "UNKNOWN-CONN", "SiteA", "23.4.1.1", map[string]any{
			"osType": "windows", "networkStatus": "connected", "lastActiveDate": daysAgo(60)})),
		noIsActive(agentFix("104", "UNKNOWN-ISO", "SiteA", "23.4.1.1", map[string]any{
			"osType": "windows", "networkStatus": "disconnected", "lastActiveDate": daysAgo(60)})),
	}
}

func namesIn(list any) []string {
	var out []string
	items, _ := list.([]any)
	for _, it := range items {
		if m, ok := it.(map[string]any); ok {
			if n, ok := m["computer_name"].(string); ok {
				out = append(out, n)
			}
		}
	}
	sort.Strings(out)
	return out
}

func sameNames(got []string, want ...string) bool {
	sort.Strings(want)
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// TestLivenessComesFromIsActiveNotIsolation: whether an agent is online is
// isActive (GET /agents "Include only active Agents"; threats agentIsActive
// "Agents currently connected to the Management Console"), never networkStatus.
// Unknown liveness is never online and never listed for action.
func TestLivenessComesFromIsActiveNotIsolation(t *testing.T) {
	db := newSeededStore(t, livenessAgents(), nil)

	t.Run("agents reclaim", func(t *testing.T) {
		out := runJSON(t, newNovelAgentsReclaimCmd(jsonFlags()), "--db", db, "--days", "30")
		var listed []string
		sites, _ := out["sites"].([]any)
		for _, s := range sites {
			listed = append(listed, namesIn(s.(map[string]any)["agents"])...)
		}
		sort.Strings(listed)
		if !sameNames(listed, "LAPTOP-OFF-90D") {
			t.Fatalf("reclaim listed %v, want only LAPTOP-OFF-90D (inactive 90d; the isolated live server and the unknown-status agents are never listed); output %v", listed, out)
		}
		if got, _ := out["unknown_status"].(float64); got != 2 {
			t.Fatalf("unknown_status = %v, want 2 (UNKNOWN-CONN, UNKNOWN-ISO counted, not listed); output %v", out["unknown_status"], out)
		}
	})

	t.Run("fleet-health summary", func(t *testing.T) {
		out := runJSON(t, newNovelFleetHealthSummaryCmd(jsonFlags()), "--db", db)
		fleet, _ := out["fleet"].(map[string]any)
		num := func(k string) float64 { v, _ := fleet[k].(float64); return v }
		if num("online") != 2 || num("offline") != 1 || num("unknown_status") != 2 {
			t.Fatalf("online/offline/unknown_status = %v/%v/%v, want 2/1/2 (online: TARGET-NEW, SRV-ISOLATED-NOW; offline: LAPTOP-OFF-90D); fleet %v",
				fleet["online"], fleet["offline"], fleet["unknown_status"], fleet)
		}
	})

	t.Run("fleet-health", func(t *testing.T) {
		out := runJSON(t, newNovelFleetHealthCmd(jsonFlags()), "--db", db)
		num := func(k string) float64 { v, _ := out[k].(float64); return v }
		if num("online") != 2 || num("offline") != 1 || num("unknown_status") != 2 {
			t.Fatalf("online/offline/unknown_status = %v/%v/%v, want 2/1/2; output %v", out["online"], out["offline"], out["unknown_status"], out)
		}
	})

	t.Run("posture", func(t *testing.T) {
		out := runJSON(t, newNovelPostureCmd(jsonFlags()), "--db", db)
		sites, _ := out["sites"].([]any)
		if len(sites) != 1 {
			t.Fatalf("want one site, got %v", out["sites"])
		}
		// Healthy needs online: TARGET-NEW and SRV-ISOLATED-NOW, 2 of 5.
		if hp, _ := sites[0].(map[string]any)["health_pct"].(float64); hp != 40 {
			t.Fatalf("health_pct = %v, want 40 (2 of 5 online; unknown status is not online)", hp)
		}
	})

	t.Run("agents dossier", func(t *testing.T) {
		for name, want := range map[string]string{"LAPTOP-OFF-90D": "offline", "SRV-ISOLATED-NOW": "online", "UNKNOWN-ISO": "unknown"} {
			out := runJSON(t, newNovelAgentsDossierCmd(jsonFlags()), "--db", db, name)
			agent, _ := out["agent"].(map[string]any)
			if agent["liveness"] != want {
				t.Fatalf("dossier %s liveness = %v, want %q", name, agent["liveness"], want)
			}
		}
	})

	t.Run("versions plan", func(t *testing.T) {
		out := runJSON(t, newNovelVersionsPlanCmd(jsonFlags()), "--db", db)
		var reachable, offline, unknown []string
		groups, _ := out["groups"].([]any)
		for _, g := range groups {
			m := g.(map[string]any)
			reachable = append(reachable, namesIn(m["reachable"])...)
			offline = append(offline, namesIn(m["offline"])...)
			unknown = append(unknown, namesIn(m["unknown_status"])...)
		}
		sort.Strings(reachable)
		sort.Strings(offline)
		sort.Strings(unknown)
		if !sameNames(reachable, "SRV-ISOLATED-NOW") || !sameNames(offline, "LAPTOP-OFF-90D") || !sameNames(unknown, "UNKNOWN-CONN", "UNKNOWN-ISO") {
			t.Fatalf("reachable %v offline %v unknown_status %v, want [SRV-ISOLATED-NOW] [LAPTOP-OFF-90D] [UNKNOWN-CONN UNKNOWN-ISO]", reachable, offline, unknown)
		}
		num := func(k string) float64 { v, _ := out[k].(float64); return v }
		if num("reachable_total") != 1 || num("offline_total") != 1 || num("unknown_status_total") != 2 || num("behind_total") != 4 {
			t.Fatalf("reachable/offline/unknown_status/behind totals = %v/%v/%v/%v, want 1/1/2/4",
				out["reachable_total"], out["offline_total"], out["unknown_status_total"], out["behind_total"])
		}
	})
}

// TestWhatchangedOfflineIsActiveNotIsolation: "went offline" is isActive
// flipping true -> false. An isolation (networkStatus connected ->
// disconnected) is its own change, and an agent whose baseline liveness is
// unknown never "went offline".
func TestWhatchangedOfflineIsActiveNotIsolation(t *testing.T) {
	db := newSeededStore(t, nil, nil)
	base := time.Now().Add(-10 * 24 * time.Hour)
	latest := time.Now().Add(-1 * time.Hour)
	noIsActive := func(a map[string]any) map[string]any {
		delete(a, "isActive")
		return a
	}
	seedSnapshot(t, db, "agents", base, []map[string]any{
		agentFix("a1", "WENT-DARK", "SiteA", "24.1.2.6", map[string]any{"isActive": true, "networkStatus": "connected"}),
		agentFix("a2", "GOT-ISOLATED", "SiteA", "24.1.2.6", map[string]any{"isActive": true, "networkStatus": "connected"}),
		noIsActive(agentFix("a3", "WAS-UNKNOWN", "SiteA", "24.1.2.6", map[string]any{"networkStatus": "connected"})),
	})
	seedSnapshot(t, db, "agents", latest, []map[string]any{
		agentFix("a1", "WENT-DARK", "SiteA", "24.1.2.6", map[string]any{"isActive": false, "networkStatus": "connected"}),
		agentFix("a2", "GOT-ISOLATED", "SiteA", "24.1.2.6", map[string]any{"isActive": true, "networkStatus": "disconnected"}),
		agentFix("a3", "WAS-UNKNOWN", "SiteA", "24.1.2.6", map[string]any{"isActive": false, "networkStatus": "disconnected"}),
	})
	out := runJSON(t, newNovelWhatchangedCmd(jsonFlags()), "--db", db, "--since", "30d")
	offline, _ := out["agents_offline"].([]any)
	if len(offline) != 1 {
		t.Fatalf("agents_offline = %v, want only WENT-DARK", out["agents_offline"])
	}
	off := offline[0].(map[string]any)
	if off["endpoint"] != "WENT-DARK" || off["detail"] != "active -> inactive" {
		t.Fatalf("agents_offline[0] = %v, want WENT-DARK with detail %q", off, "active -> inactive")
	}
	isolated, _ := out["agents_isolated"].([]any)
	var isoNames []string
	for _, it := range isolated {
		isoNames = append(isoNames, it.(map[string]any)["endpoint"].(string))
	}
	sort.Strings(isoNames)
	if !sameNames(isoNames, "GOT-ISOLATED", "WAS-UNKNOWN") {
		t.Fatalf("agents_isolated = %v, want GOT-ISOLATED and WAS-UNKNOWN (network connected -> disconnected)", isoNames)
	}
}
