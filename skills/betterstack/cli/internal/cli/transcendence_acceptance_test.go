// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Behavioral acceptance tests for the 6 transcendence commands. Seeds a local
// SQLite mirror with realistic JSON:API fixtures (the {id,type,attributes,…}
// envelope Better Stack returns) and asserts each command's --json output.
// This is the primary correctness evidence: live dogfood is skipped when no
// API token is available.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"betterstack-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

func elem(id, typ string, attrs map[string]any, rels map[string]any) json.RawMessage {
	m := map[string]any{"id": id, "type": typ, "attributes": attrs}
	if rels != nil {
		m["relationships"] = rels
	}
	b, _ := json.Marshal(m)
	return b
}

func monitorRel(monitorID string) map[string]any {
	return map[string]any{"monitor": map[string]any{"data": map[string]any{"id": monitorID, "type": "monitor"}}}
}

func seedAcceptanceStore(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "data.db")
	s, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	now := time.Now().UTC()
	ts := func(d time.Duration) string { return now.Add(d).Format(time.RFC3339) }

	// Monitors: m1 covered, m2 unprotected+down, m3 paused+unprotected.
	must(t, s.Upsert("monitors", "1", elem("1", "monitor", map[string]any{
		"pronounceable_name": "API", "url": "https://api.example.com", "monitor_type": "status",
		"status": "up", "paused": false, "policy_id": "10", "email": true,
	}, nil)))
	must(t, s.Upsert("monitors", "2", elem("2", "monitor", map[string]any{
		"pronounceable_name": "Web", "url": "https://web.example.com", "monitor_type": "keyword",
		"status": "down", "paused": false, "policy_id": "", "email": false, "sms": false, "call": false, "push": false,
	}, nil)))
	must(t, s.Upsert("monitors", "3", elem("3", "monitor", map[string]any{
		"pronounceable_name": "Staging", "url": "https://staging.example.com", "monitor_type": "status",
		"status": "up", "paused": true, "policy_id": "", "email": false,
	}, nil)))

	// Heartbeats: h1 tight grace, h2 paused+zero grace, h3 healthy.
	must(t, s.Upsert("heartbeats", "1", elem("1", "heartbeat", map[string]any{
		"name": "cron-nightly", "period": 60, "grace": 30, "paused": false, "status": "up",
	}, nil)))
	must(t, s.Upsert("heartbeats", "2", elem("2", "heartbeat", map[string]any{
		"name": "backup-job", "period": 3600, "grace": 0, "paused": true, "status": "paused",
	}, nil)))
	must(t, s.Upsert("heartbeats", "3", elem("3", "heartbeat", map[string]any{
		"name": "healthy-task", "period": 60, "grace": 120, "paused": false, "status": "up",
	}, nil)))

	// Incidents: i1,i2 resolved (source mon 2); i3 old (outside window); i4 open (source mon 1).
	must(t, s.Upsert("incidents", "1", elem("1", "incident", map[string]any{
		"name": "Web down", "url": "https://web.example.com",
		"started_at": ts(-48 * time.Hour), "acknowledged_at": ts(-48*time.Hour + 5*time.Minute),
		"resolved_at": ts(-48*time.Hour + 30*time.Minute), "status": "Resolved",
	}, monitorRel("2"))))
	must(t, s.Upsert("incidents", "2", elem("2", "incident", map[string]any{
		"name": "Web slow", "url": "https://web.example.com",
		"started_at": ts(-24 * time.Hour), "acknowledged_at": ts(-24*time.Hour + 10*time.Minute),
		"resolved_at": ts(-24*time.Hour + 60*time.Minute), "status": "Resolved",
	}, monitorRel("2"))))
	must(t, s.Upsert("incidents", "3", elem("3", "incident", map[string]any{
		"name": "Ancient", "started_at": ts(-100 * 24 * time.Hour),
		"resolved_at": ts(-100*24*time.Hour + 10*time.Minute), "status": "Resolved",
	}, monitorRel("2"))))
	must(t, s.Upsert("incidents", "4", elem("4", "incident", map[string]any{
		"name": "API blip", "url": "https://api.example.com",
		"started_at": ts(-72 * time.Hour), "status": "Started",
	}, monitorRel("1"))))

	// On-call: oc1 covered, oc2 empty.
	must(t, s.Upsert("on-calls", "1", elem("1", "on_call", map[string]any{
		"name": "Primary", "default_calendar": true,
		"on_call_users": []any{map[string]any{"id": "u1", "email": "a@example.com"}},
	}, nil)))
	must(t, s.Upsert("on-calls", "2", elem("2", "on_call", map[string]any{
		"name": "Secondary", "default_calendar": false, "on_call_users": []any{},
	}, nil)))

	if err := s.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}
	return dbPath
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
}

// runJSON runs a built command with --json semantics and decodes stdout into v.
func runJSON(t *testing.T, build func(*rootFlags) *cobra.Command, args []string, v any) {
	t.Helper()
	flags := &rootFlags{asJSON: true}
	cmd := build(flags)
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute %v: %v (stderr: %s)", args, err, errb.String())
	}
	if v != nil {
		if err := json.Unmarshal(out.Bytes(), v); err != nil {
			t.Fatalf("decode output %q: %v", out.String(), err)
		}
	}
}

func TestFleetCommand(t *testing.T) {
	db := seedAcceptanceStore(t)
	var rep fleetReport
	runJSON(t, newNovelFleetCmd, []string{"--db", db}, &rep)
	if rep.Monitors.Total != 3 || rep.Monitors.Up != 1 || rep.Monitors.Down != 1 || rep.Monitors.Paused != 1 {
		t.Errorf("monitors = %+v, want total3 up1 down1 paused1", rep.Monitors)
	}
	if rep.Heartbeats.Total != 3 || rep.Heartbeats.Paused != 1 {
		t.Errorf("heartbeats = %+v, want total3 paused1", rep.Heartbeats)
	}
	if rep.Incidents.Total != 4 || rep.Incidents.Open != 1 {
		t.Errorf("incidents = %+v, want total4 open1", rep.Incidents)
	}
	if rep.OnCall.Calendars != 2 || rep.OnCall.Covered != 1 {
		t.Errorf("on_call = %+v, want calendars2 covered1", rep.OnCall)
	}
}

func TestCoverageCommand(t *testing.T) {
	db := seedAcceptanceStore(t)
	var gaps []coverageGap
	runJSON(t, newNovelCoverageCmd, []string{"--db", db}, &gaps)
	if len(gaps) != 1 {
		t.Fatalf("default coverage gaps = %d, want 1 (only m2; m3 paused excluded)", len(gaps))
	}
	if gaps[0].ID != "2" || len(gaps[0].Why) != 2 {
		t.Errorf("gap = %+v, want id 2 with 2 reasons", gaps[0])
	}

	var gapsAll []coverageGap
	runJSON(t, newNovelCoverageCmd, []string{"--db", db, "--include-paused"}, &gapsAll)
	if len(gapsAll) != 2 {
		t.Errorf("coverage --include-paused gaps = %d, want 2", len(gapsAll))
	}
}

func TestMttrCommand(t *testing.T) {
	db := seedAcceptanceStore(t)
	var rep mttrReport
	runJSON(t, newNovelMttrCmd, []string{"--db", db, "--days", "30"}, &rep)
	if rep.IncidentsInWin != 3 {
		t.Errorf("incidents_in_window = %d, want 3 (i3 outside 30d)", rep.IncidentsInWin)
	}
	if rep.AcknowledgedN != 2 || rep.ResolvedN != 2 {
		t.Errorf("acked=%d resolved=%d, want 2/2", rep.AcknowledgedN, rep.ResolvedN)
	}
	if rep.MeanMTTASeconds != 450 {
		t.Errorf("mean MTTA = %v, want 450", rep.MeanMTTASeconds)
	}
	if rep.MeanMTTRSeconds != 2700 {
		t.Errorf("mean MTTR = %v, want 2700", rep.MeanMTTRSeconds)
	}
}

func TestFlappingCommand(t *testing.T) {
	db := seedAcceptanceStore(t)
	var entries []flapEntry
	runJSON(t, newNovelFlappingCmd, []string{"--db", db, "--days", "30"}, &entries)
	if len(entries) != 2 {
		t.Fatalf("flapping sources = %d, want 2", len(entries))
	}
	if entries[0].Source != "2" || entries[0].Incidents != 2 {
		t.Errorf("top flapper = %+v, want source 2 with 2 incidents", entries[0])
	}
}

func TestOncallGapsCommand(t *testing.T) {
	db := seedAcceptanceStore(t)
	var gaps []onCallGap
	runJSON(t, newNovelOncallGapsCmd, []string{"--db", db}, &gaps)
	if len(gaps) != 1 || gaps[0].ID != "2" {
		t.Errorf("oncall gaps = %+v, want exactly calendar 2", gaps)
	}
}

func TestHeartbeatRiskCommand(t *testing.T) {
	db := seedAcceptanceStore(t)
	var risks []heartbeatRisk
	runJSON(t, newNovelHeartbeatRiskCmd, []string{"--db", db}, &risks)
	if len(risks) != 2 {
		t.Fatalf("heartbeat risks = %d, want 2 (h3 healthy excluded)", len(risks))
	}
	if risks[0].ID != "2" || risks[0].Score < 7 {
		t.Errorf("top risk = %+v, want heartbeat 2 with score >= 7", risks[0])
	}
}

// Empty-store safety: every transcendence command must exit 0 on an empty mirror.
func TestTranscendenceEmptyStore(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "empty.db")
	s, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	s.Close()
	for _, build := range []func(*rootFlags) *cobra.Command{
		newNovelFleetCmd, newNovelCoverageCmd, newNovelMttrCmd,
		newNovelFlappingCmd, newNovelOncallGapsCmd, newNovelHeartbeatRiskCmd,
	} {
		flags := &rootFlags{asJSON: true}
		cmd := build(flags)
		var out, errb bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&errb)
		cmd.SetArgs([]string{"--db", dbPath})
		if err := cmd.Execute(); err != nil {
			t.Errorf("%s on empty store errored: %v", cmd.Use, err)
		}
	}
}
