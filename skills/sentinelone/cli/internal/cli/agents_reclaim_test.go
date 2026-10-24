// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"sentinelone-pp-cli/internal/cliutil/testenv"
)

// TestNovelAgentsReclaimHelpWires smoke-tests that the agents reclaim command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelAgentsReclaimHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"agents", "reclaim", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("agents reclaim --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "reclaim"} {
		if !strings.Contains(help, want) {
			t.Fatalf("agents reclaim --help missing %q in output:\n%s", want, help)
		}
	}
}

func daysAgo(d int) string {
	return time.Now().Add(-time.Duration(d) * 24 * time.Hour).UTC().Format(time.RFC3339)
}

// TestAgentsReclaimListsOnlyProvablyIdleAgents: an MSP decommissions what this
// prints, so every listed agent must be inactive (isActive false) with a known
// check-in.
func TestAgentsReclaimListsOnlyProvablyIdleAgents(t *testing.T) {
	off := map[string]any{"isActive": false}
	with := func(base map[string]any, kv ...any) map[string]any {
		m := map[string]any{}
		for k, v := range base {
			m[k] = v
		}
		for i := 0; i < len(kv); i += 2 {
			m[kv[i].(string)] = kv[i+1]
		}
		return m
	}
	agents := []map[string]any{
		agentFix("1", "LIVE-01", "SiteA", "24.1", nil),                                           // active: never listed
		agentFix("2", "DEAD-01", "SiteA", "24.1", with(off, "lastActiveDate", daysAgo(60))),      // offline 60d
		agentFix("3", "NAP-01", "SiteA", "24.1", with(off, "lastActiveDate", daysAgo(5))),        // offline, under threshold
		agentFix("4", "OLDCONN", "SiteB", "24.1", map[string]any{"lastActiveDate": daysAgo(90)}), // active, stale date: not idle
		agentFix("5", "NODATE", "SiteB", "24.1", with(off, "lastActiveDate", "")),                // unknown: counted, not listed
		agentFix("6", "DEC-01", "SiteB", "24.1", with(off, "lastActiveDate", daysAgo(99), "isDecommissioned", true)),
		// listed for being offline 40d; the newer same-name record is context only
		agentFix("7", "WS-07", "SiteC", "24.1", with(off, "lastActiveDate", daysAgo(40))),
		agentFix("8", "ws-07", "SiteC", "24.1", map[string]any{"lastActiveDate": daysAgo(0)}),
		// a hostname twin offline under the threshold is never listed
		agentFix("10", "WS-11", "SiteC", "24.1", with(off, "lastActiveDate", daysAgo(3))),
		agentFix("11", "WS-11", "SiteC", "24.1", map[string]any{"lastActiveDate": daysAgo(0)}),
		// same name in another site is a different endpoint
		agentFix("9", "WS-07", "SiteD", "24.1", with(off, "lastActiveDate", daysAgo(2))),
	}
	db := newSeededStore(t, agents, nil)
	out := runJSON(t, newNovelAgentsReclaimCmd(jsonFlags()), "--db", db)

	listed := map[string]string{}
	newer := map[string]string{}
	for _, s := range out["sites"].([]any) {
		for _, a := range s.(map[string]any)["agents"].([]any) {
			m := a.(map[string]any)
			var rs []string
			for _, r := range m["reasons"].([]any) {
				rs = append(rs, r.(string))
			}
			listed[m["id"].(string)] = strings.Join(rs, ",")
			newer[m["id"].(string)], _ = m["newer_record_same_name"].(string)
		}
	}
	if len(listed) != 2 || listed["2"] != "offline 60d" || listed["7"] != "offline 40d" || newer["7"] != "8" {
		t.Fatalf("listed = %v newer = %v, want exactly 2 (offline 60d) and 7 (offline 40d, newer record 8)", listed, newer)
	}
	if got := out["reclaimable_total"].(float64); got != 2 {
		t.Fatalf("reclaimable_total = %v, want 2", got)
	}
	if got := out["unknown_last_seen"].(float64); got != 1 {
		t.Fatalf("unknown_last_seen = %v, want 1", got)
	}
	if got := out["decommissioned_skipped"].(float64); got != 1 {
		t.Fatalf("decommissioned_skipped = %v, want 1", got)
	}
}

func TestAgentsReclaimRefusesAZeroThreshold(t *testing.T) {
	db := newSeededStore(t, []map[string]any{agentFix("1", "A", "S", "24.1", nil)}, nil)
	cmd := newNovelAgentsReclaimCmd(jsonFlags())
	cmd.SetArgs([]string{"--db", db, "--days", "0"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "--days") {
		t.Fatalf("--days 0 must be refused (it would list every inactive agent), got %v", err)
	}
}

// TestReclaimListsOnlyAnExplicitlyInactiveAgent: whether a machine is still
// checking in is isActive (spec.yaml GET /agents "Include only active Agents"),
// not networkStatus, which is the "Disconnect from network" isolation state.
// Only isActive as the JSON boolean false is proof that no machine is checking
// in. A missing, null, string or numeric isActive is unknown: counted as
// unknown_status, never listed. The network status changes nothing in either
// direction: an isolated agent silent 60 days is listed, an isolated active
// one is not.
func TestReclaimListsOnlyAnExplicitlyInactiveAgent(t *testing.T) {
	stale := daysAgo(60)
	const absent = "(absent)"
	cases := []struct {
		name     string
		isActive any
		network  string
		listed   bool
		unknown  bool
	}{
		{"INACTIVE-CONN", false, "connected", true, false},
		{"INACTIVE-ISOLATED", false, "disconnected", true, false},
		{"INACTIVE-NONET", false, "", true, false},
		{"ACTIVE-CONN", true, "connected", false, false},
		{"ACTIVE-ISOLATED", true, "disconnected", false, false},
		{"MISSING-ISOLATED", absent, "disconnected", false, true},
		{"NULL", nil, "disconnected", false, true},
		{"STRING-FALSE", "false", "disconnected", false, true},
		{"NUMBER-ZERO", float64(0), "disconnected", false, true},
	}
	agents := []map[string]any{}
	want := map[string]bool{}
	wantUnknown := 0
	for i, c := range cases {
		a := agentFix(string(rune('a'+i)), c.name, "SiteA", "24.1",
			map[string]any{"networkStatus": c.network, "lastActiveDate": stale})
		if s, ok := c.isActive.(string); ok && s == absent {
			delete(a, "isActive")
		} else {
			a["isActive"] = c.isActive
		}
		agents = append(agents, a)
		if c.listed {
			want[c.name] = true
		}
		if c.unknown {
			wantUnknown++
		}
	}
	db := newSeededStore(t, agents, nil)
	out := runJSON(t, newNovelAgentsReclaimCmd(jsonFlags()), "--db", db, "--days", "30")
	got := map[string]bool{}
	sites, _ := out["sites"].([]any)
	for _, s := range sites {
		for _, a := range s.(map[string]any)["agents"].([]any) {
			got[a.(map[string]any)["computer_name"].(string)] = true
		}
	}
	total, _ := out["reclaimable_total"].(float64)
	if len(got) != len(want) || int(total) != len(want) {
		t.Fatalf("listed %v (reclaimable_total %v), want exactly %v", got, out["reclaimable_total"], want)
	}
	for n := range want {
		if !got[n] {
			t.Fatalf("listed %v: %s (isActive false, silent 60d) must be listed", got, n)
		}
	}
	if u, _ := out["unknown_status"].(float64); int(u) != wantUnknown {
		t.Fatalf("unknown_status = %v, want %d (missing, null, string and numeric isActive)", out["unknown_status"], wantUnknown)
	}
}
