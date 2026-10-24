// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"huntress-pp-cli/internal/cliutil/testenv"
	"huntress-pp-cli/internal/store"
)

// TestNovelEscalationQueueHelpWires smoke-tests that the escalation-queue command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelEscalationQueueHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"escalation-queue", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("escalation-queue --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "escalation-queue"} {
		if !strings.Contains(help, want) {
			t.Fatalf("escalation-queue --help missing %q in output:\n%s", want, help)
		}
	}
}

func runNovelJSON(t *testing.T, args ...string) []map[string]any {
	t.Helper()
	cmd := RootCmd()
	cmd.SetArgs(append(args, "--json"))
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("%v: %v\nstderr: %s", args, err, errb.String())
	}
	var rows []map[string]any
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
		t.Fatalf("%v: output is not a JSON array: %v\n%s", args, err, out.String())
	}
	return rows
}

func TestEscalationQueueOnSeededStore(t *testing.T) {
	testenv.Isolate(t)
	db := filepath.Join(t.TempDir(), "data.db")
	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().Add(-72 * time.Hour).Format(time.RFC3339)
	recent := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	for _, e := range []string{
		`{"id":1,"severity":"low","status":"open","subject":"old low","created_at":"` + old + `","resolved_at":null,"organizations":[{"id":7,"name":"Acme"}]}`,
		`{"id":2,"severity":"critical","status":"sent","subject":"new crit","created_at":"` + recent + `","resolved_at":null,"organizations":[{"id":8,"name":"Beta"}]}`,
		`{"id":3,"severity":"critical","status":"open","subject":"old crit","created_at":"` + old + `","resolved_at":null,"organizations":[{"id":7,"name":"Acme"}]}`,
		`{"id":4,"severity":"critical","status":"resolved","subject":"done","created_at":"` + old + `","resolved_at":"` + recent + `","organizations":[]}`,
		`{"id":5,"severity":"high","status":"open","subject":"resolved_at only","created_at":"` + old + `","resolved_at":"` + recent + `","organizations":[]}`,
	} {
		if err := st.UpsertEscalations(json.RawMessage(e)); err != nil {
			t.Fatal(err)
		}
	}
	_ = st.Close()

	rows := runNovelJSON(t, "escalation-queue", "--db", db)
	var ids []string
	for _, r := range rows {
		ids = append(ids, fmt.Sprint(r["id"]))
	}
	// critical first, oldest first within a severity; resolved by status OR resolved_at drops out.
	if got := strings.Join(ids, ","); got != "3,2,1" {
		t.Fatalf("order = %s, want 3,2,1 (rows %v)", got, rows)
	}
	if h, _ := rows[0]["hours_open"].(float64); h < 71 || h > 73 {
		t.Fatalf("hours_open = %v, want about 72", rows[0]["hours_open"])
	}
	if orgs, _ := rows[0]["organizations"].([]any); len(orgs) != 1 {
		t.Fatalf("organizations not decoded: %#v", rows[0]["organizations"])
	}
	if rows := runNovelJSON(t, "escalation-queue", "--db", db, "--org", "7", "--severity", "critical"); len(rows) != 1 || fmt.Sprint(rows[0]["id"]) != "3" {
		t.Fatalf("--org 7 --severity critical = %v, want only id 3", rows)
	}

	cmd := RootCmd()
	cmd.SetArgs([]string{"escalation-queue", "--db", db, "--severity", "urgent"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err == nil {
		t.Fatal("--severity urgent was accepted; an unknown severity must be refused, not match nothing")
	}
}
