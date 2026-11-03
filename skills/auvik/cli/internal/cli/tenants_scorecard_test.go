// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"auvik-pp-cli/internal/cliutil/testenv"
)

// TestNovelTenantsScorecardHelpWires smoke-tests that the tenants scorecard command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelTenantsScorecardHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"tenants", "scorecard", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("tenants scorecard --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "scorecard"} {
		if !strings.Contains(help, want) {
			t.Fatalf("tenants scorecard --help missing %q in output:\n%s", want, help)
		}
	}
}

func scorecardFixture(now time.Time) scorecardInputs {
	recent := now.Add(-2 * time.Hour).Format(time.RFC3339)
	old := now.Add(-90 * 24 * time.Hour).Format(time.RFC3339)
	return scorecardInputs{
		devices: []auvikRow{
			device("d1", "old-switch", "l3Switch", "Cisco 2960", "t1"),
			device("d2", "unbacked-fw", "firewall", "FG60F", "t1"),
			device("d3", "stale-router", "router", "ISR 4331", "t2"),
		},
		lifecycleV2: []auvikRow{
			specRow("d1", map[specField]any{fLifecycleV2LastSupportDate: "2024-03-01"}, nil),
			specRow("d3", map[specField]any{fLifecycleV2LastSupportDate: "2031-01-01"}, nil),
		},
		configs: []auvikRow{
			specRow("c1", map[specField]any{fConfigBackupTime: now.Add(-24 * time.Hour).Format(time.RFC3339), fConfigIsRunning: true},
				map[specRel]any{rConfigDevice: "d1"}),
			specRow("c3", map[specField]any{fConfigBackupTime: old, fConfigIsRunning: true},
				map[specRel]any{rConfigDevice: "d3"}),
		},
		alerts: []auvikRow{
			specRow("a1", map[specField]any{fAlertDetectedOn: recent, fAlertSeverity: "warning"}, map[specRel]any{rAlertEntity: "d3"}),
			specRow("a2", map[specField]any{fAlertDetectedOn: recent, fAlertSeverity: "critical"}, map[specRel]any{rAlertEntity: "d3"}),
			specRow("a3", map[specField]any{fAlertDetectedOn: old, fAlertSeverity: "info"}, map[specRel]any{rAlertEntity: "d1"}),
		},
	}
}

// Each column must equal what the single-purpose command reports for that
// client, folded per client, worst client first.
func TestBuildScorecard_FoldsEachBuilderPerClient(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	got := buildScorecardReport(scorecardFixture(now), tenantMap(), now, now.Add(-30*24*time.Hour), 30*24*time.Hour)
	if len(got.Rows) != 2 {
		t.Fatalf("want 2 clients, got %d: %+v", len(got.Rows), got.Rows)
	}
	acme, globex := got.Rows[0], got.Rows[1]
	if acme.Client != "acme" {
		t.Fatalf("acme has expired gear and must sort first, got %+v", got.Rows)
	}
	if acme.Devices != 2 || acme.EOLExpired != 1 || acme.BackupMissing != 1 || acme.BackupStale != 0 || acme.Alerts != 0 {
		t.Errorf("acme = %+v, want 2 devices, 1 expired, 1 missing backup, no alerts (a3 is outside the window)", acme)
	}
	if globex.Devices != 1 || globex.EOLExpired != 0 || globex.BackupStale != 1 || globex.Alerts != 2 {
		t.Errorf("globex = %+v, want 1 device, 1 stale backup, 2 alerts", globex)
	}
	if got.Totals.Devices != 3 || got.Totals.Alerts != 2 || got.Totals.BackupMissing != 1 {
		t.Errorf("totals = %+v", got.Totals)
	}
}

// A family that was never synced reads 0 because nothing was read. It must be
// named as unsynced and must not be counted as a finding: with no backups
// synced, configuration audit flags every device no_backup, which here would
// be a sync gap dressed up as a client problem.
func TestBuildScorecard_UnsyncedFamilyIsNotAFinding(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	in := scorecardFixture(now)
	in.configs, in.alerts = nil, nil
	got := buildScorecardReport(in, tenantMap(), now, now.Add(-30*24*time.Hour), 30*24*time.Hour)
	if got.Totals.BackupMissing != 0 {
		t.Errorf("backup_missing = %d with no backups synced, want 0", got.Totals.BackupMissing)
	}
	joined := strings.Join(got.Unsynced, "|")
	for _, want := range []string{"configuration backups", "alerts"} {
		if !strings.Contains(joined, want) {
			t.Errorf("unsynced = %q, want it to name %q", joined, want)
		}
	}
}

func TestBuildScorecard_NoDevicesFabricatesNothing(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	got := buildScorecardReport(scorecardInputs{}, tenantMap(), now, now, time.Hour)
	b, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"rows":[]`) || got.Note == "" {
		t.Fatalf("empty mirror must marshal rows as [] with a note, got %s", b)
	}
}
