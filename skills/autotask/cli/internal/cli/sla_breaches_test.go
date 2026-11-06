// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"autotask-pp-cli/internal/cliutil/testenv"
	"autotask-pp-cli/internal/store"
)

// TestNovelSlaBreachesHelpWires smoke-tests that the sla-breaches command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelSlaBreachesHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"sla-breaches", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("sla-breaches --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "sla-breaches"} {
		if !strings.Contains(help, want) {
			t.Fatalf("sla-breaches --help missing %q in output:\n%s", want, help)
		}
	}
}

// A ticket overdue for first response is a breach even when its due date is
// ahead; a met deadline is never pending (TicketModel fields, spec.json).
func TestSlaBreachesReadsEveryDocumentedDeadline(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	past, future := "2026-10-28T12:00:00Z", "2026-11-05T12:00:00Z"
	cases := []struct {
		ticket map[string]any
		want   string
	}{
		{map[string]any{"dueDateTime": future, "firstResponseDueDateTime": past}, "first-response"},
		{map[string]any{"dueDateTime": future, "firstResponseDueDateTime": past, "firstResponseDateTime": past}, ""},
		{map[string]any{"dueDateTime": future, "resolutionPlanDueDateTime": past}, "resolution-plan"},
		{map[string]any{"dueDateTime": future, "resolvedDueDateTime": past}, "resolution"},
		{map[string]any{"dueDateTime": future, "resolvedDueDateTime": past, "resolvedDateTime": past}, ""},
		{map[string]any{"dueDateTime": past}, "due-date"},
		{map[string]any{"dueDateTime": "2026-10-20T12:00:00Z", "firstResponseDueDateTime": past}, "due-date"},
		{map[string]any{"dueDateTime": future}, ""},
	}
	for i, c := range cases {
		got, _, ok := mostOverdueDeadline(c.ticket, now)
		if got != c.want || ok != (c.want != "") {
			t.Errorf("case %d %v: breach %q (ok %v), want %q", i, c.ticket, got, ok, c.want)
		}
	}
}

// Through the command: an open ticket whose due date is next week but whose
// first response was due yesterday and never made is listed.
func TestSlaBreachesListsAnOverdueFirstResponse(t *testing.T) {
	testenv.Isolate(t)
	db := filepath.Join(t.TempDir(), "data.db")
	s, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	tk := `{"id":7001,"title":"Server down","status":1,"dueDateTime":"` + now.Add(7*24*time.Hour).Format(time.RFC3339) +
		`","firstResponseDueDateTime":"` + now.Add(-24*time.Hour).Format(time.RFC3339) + `"}`
	if err := s.Upsert("tickets", "7001", []byte(tk)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().Exec(`INSERT OR REPLACE INTO sync_state (resource_type, last_attempt_complete, last_synced_at) VALUES ('tickets', 1, ?)`, now.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	recordUnfilteredWalks(t, s)
	_ = s.Close()
	cmd := RootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"sla-breaches", "--json", "--db", db})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("sla-breaches: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "Server down") {
		t.Fatalf("a ticket overdue for first response is not listed:\n%s", out.String())
	}
}
