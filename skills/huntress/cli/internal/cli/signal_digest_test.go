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

// TestNovelSignalDigestHelpWires smoke-tests that the signal-digest command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelSignalDigestHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"signal-digest", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("signal-digest --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "signal-digest"} {
		if !strings.Contains(help, want) {
			t.Fatalf("signal-digest --help missing %q in output:\n%s", want, help)
		}
	}
}

func TestSignalDigestOnSeededStore(t *testing.T) {
	testenv.Isolate(t)
	db := filepath.Join(t.TempDir(), "data.db")
	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	at := func(h int) string { return time.Now().UTC().Add(-time.Duration(h) * time.Hour).Format(time.RFC3339) }
	for _, s := range []string{
		`{"id":1,"type":"Footholds","status":"reported","created_at":"` + at(5) + `","organization":{"id":7,"name":"Acme"}}`,
		`{"id":2,"type":"Footholds","status":"closed","created_at":"` + at(30) + `","organization":{"id":7,"name":"Acme"}}`,
		`{"id":3,"type":"Antivirus","status":"closed","created_at":"` + at(2) + `","organization":{"id":8,"name":"Beta"}}`,
		`{"id":4,"type":"Footholds","status":"reported","created_at":"` + at(24*30) + `","organization":{"id":7,"name":"Acme"}}`,
	} {
		if err := st.UpsertSignals(json.RawMessage(s)); err != nil {
			t.Fatal(err)
		}
	}
	_ = st.Close()

	rows := runNovelJSON(t, "signal-digest", "--db", db, "--since", "7d")
	if len(rows) != 2 {
		t.Fatalf("rows = %v, want 2 groups (the 30-day-old signal is outside 7d)", rows)
	}
	top := rows[0]
	if top["type"] != "Footholds" || fmt.Sprint(top["organization_id"]) != "7" || top["organization_name"] != "Acme" ||
		fmt.Sprint(top["signals"]) != "2" || fmt.Sprint(top["reported"]) != "1" || fmt.Sprint(top["closed"]) != "1" {
		t.Fatalf("top group = %v", top)
	}
	if fs, _ := top["first_seen"].(string); !strings.HasSuffix(fs, "Z") || fs > fmt.Sprint(top["last_seen"]) {
		t.Fatalf("first_seen/last_seen = %v/%v", top["first_seen"], top["last_seen"])
	}
	if rows := runNovelJSON(t, "signal-digest", "--db", db, "--since", "all", "--type", "footholds"); len(rows) != 1 || fmt.Sprint(rows[0]["signals"]) != "3" {
		t.Fatalf("--since all --type footholds = %v, want one group of 3", rows)
	}
	if rows := runNovelJSON(t, "signal-digest", "--db", db, "--since", "7d", "--org", "8"); len(rows) != 1 || rows[0]["type"] != "Antivirus" {
		t.Fatalf("--org 8 = %v", rows)
	}

	cmd := RootCmd()
	cmd.SetArgs([]string{"signal-digest", "--db", db, "--since", "lastweek"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err == nil {
		t.Fatal("--since lastweek was accepted; an unreadable window must be refused, not widened to all time")
	}
}
