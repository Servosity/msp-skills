// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"servosity-msp-pp-cli/internal/cliutil/testenv"
	"servosity-msp-pp-cli/internal/store"
)

// isolateNovelStore gives a test its own home and resets the process-wide
// store scope. The scope is derived from the credential and kept globally, so
// a test that set a token would otherwise leave seeding and the command under
// test on different database files.
func isolateNovelStore(t *testing.T) {
	t.Helper()
	testenv.Isolate(t)
	setDefaultDBScopeIdentity("", "", "")
	t.Cleanup(func() { setDefaultDBScopeIdentity("", "", "") })
}

// seedNovelStore writes one company with a classic, a restic and a DR backup
// into the store the commands open, plus freshness rows: the restic backup
// last succeeded 30 days ago, the DR backup today, and an orphan row names a
// restic backup that sync has since removed. The DR backup goes through the
// generic resources table, which is where press 4.34.0 stores dr-backups.
func seedNovelStore(t *testing.T) {
	t.Helper()
	db, err := store.Open(defaultDBPath("servosity-cli"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	company := `"https://api.servosity.com/api/v1/companies/4421/"`
	for _, step := range []struct {
		name string
		err  error
	}{
		{"company", db.UpsertCompanies(json.RawMessage(`{"id":4421,"name":"Acme"}`))},
		{"classic", db.UpsertBackups(json.RawMessage(`{"id":10,"company":{"id":4421,"name":"Acme"},"display_name":"CLASSIC-1","retention":"1YEAR","guarantee_eligible":false,"state":"active"}`))},
		{"restic", db.UpsertResticBackups(json.RawMessage(`{"id":20,"company":` + company + `,"device_name":"SRV-R","display_name":"","retention":"7YEAR","guarantee_eligible":true,"state":"active"}`))},
		{"dr", db.Upsert("dr-backups", "30", json.RawMessage(`{"id":30,"company":`+company+`,"device_name":"SRV-DR","display_name":"","retention":"4YEAR","guarantee_eligible":true,"state":"active"}`))},
	} {
		if step.err != nil {
			t.Fatalf("seed %s: %v", step.name, step.err)
		}
	}
	ctx := context.Background()
	if err := ensureLastSuccessTable(ctx, db); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for _, row := range [][]any{
		{"restic", "20", 999, "SRV-R", now.Add(-30 * 24 * time.Hour).Format(time.RFC3339)}, // stale ownership: inventory says 4421
		{"dr", "30", 4421, "SRV-DR", now.Format(time.RFC3339)},
		{"restic", "99", 4421, "RETIRED", now.Add(-90 * 24 * time.Hour).Format(time.RFC3339)},
	} {
		if _, err := db.DB().ExecContext(ctx, `INSERT INTO pp_last_success (engine, backup_id, company_id, hostname, last_success, checked_at) VALUES (?, ?, ?, ?, ?, ?)`,
			append(row, now.Format(time.RFC3339))...); err != nil {
			t.Fatalf("seed freshness: %v", err)
		}
	}
}

func runNovel(t *testing.T, args ...string) string {
	t.Helper()
	cmd := RootCmd()
	cmd.SetArgs(args)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("%v: %v\nstderr: %s", args, err, errOut.String())
	}
	return out.String()
}

// TestNovelCommandsReadTheCurrentStore runs the local novels against a real
// store. It caught DR backups read from a dr_backups table press 4.34.0 no
// longer creates, and pins the freshness fixes: health is per backup, retired
// backups drop out, and the email filter's default engine matches everything.
func TestNovelCommandsReadTheCurrentStore(t *testing.T) {
	isolateNovelStore(t)
	seedNovelStore(t)

	var facts []backupFact
	if err := json.Unmarshal([]byte(runNovel(t, "backup-facts", "--json")), &facts); err != nil {
		t.Fatalf("backup-facts json: %v", err)
	}
	health := map[string]string{}
	for _, f := range facts {
		health[f.Engine+"/"+f.Hostname] = f.Health
	}
	want := map[string]string{"classic/CLASSIC-1": "unknown", "restic/SRV-R": "stale", "dr/SRV-DR": "ok"}
	for k, v := range want {
		if health[k] != v {
			t.Errorf("backup-facts health[%s] = %q, want %q (all: %v)", k, health[k], v, health)
		}
	}

	var findings []retentionFinding
	if err := json.Unmarshal([]byte(runNovel(t, "retention-audit", "--min", "7YEAR", "--json")), &findings); err != nil {
		t.Fatalf("retention-audit json: %v", err)
	}
	got := map[string]int{}
	for _, f := range findings {
		got[f.Engine+"/"+f.Hostname] = len(f.Reasons)
	}
	if got["classic/CLASSIC-1"] != 2 || got["dr/SRV-DR"] != 1 || len(got) != 2 {
		t.Errorf("retention-audit findings = %v, want classic/CLASSIC-1:2 and dr/SRV-DR:1", got)
	}

	stale := runNovel(t, "stale-backups", "--days", "7", "--json")
	if !strings.Contains(stale, "SRV-R") || strings.Contains(stale, "RETIRED") || strings.Contains(stale, "SRV-DR") {
		t.Errorf("stale-backups = %s; want SRV-R only (no retired backup, no fresh DR)", stale)
	}
	if !strings.Contains(stale, `"company_id": 4421`) || strings.Contains(stale, "999") {
		t.Errorf("stale-backups = %s; ownership must come from the current inventory (4421), not the freshness row (999)", stale)
	}

	drafts := runNovel(t, "email-draft", "--days", "7", "--json")
	if !strings.Contains(drafts, "SRV-R") {
		t.Errorf("email-draft with the default --engine all produced no draft for the stale backup: %s", drafts)
	}

	var att attentionResult
	if err := json.Unmarshal([]byte(runNovel(t, "attention", "--json")), &att); err != nil {
		t.Fatalf("attention json: %v", err)
	}
	if att.Totals["stale"] != 1 || att.Totals["freshness_unknown"] != 0 {
		t.Errorf("attention totals = %v, want stale 1 (retired backup excluded) and freshness_unknown 0", att.Totals)
	}

	report := runNovel(t, "qbr", "4421", "--format", "md")
	if !strings.Contains(report, "SRV-R") || !strings.Contains(report, "SRV-DR") {
		t.Errorf("qbr coverage lost a host (empty display_name must fall through to device_name):\n%s", report)
	}
}
