// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"keeper-pp-cli/internal/cliutil/testenv"
	"keeper-pp-cli/internal/store"
)

// TestNovelEventsListHelpWires smoke-tests that the events list command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelEventsListHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"events", "list", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("events list --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "list"} {
		if !strings.Contains(help, want) {
			t.Fatalf("events list --help missing %q in output:\n%s", want, help)
		}
	}
}

// A cursor past the newest stored event (a rebuilt store) gets a stderr note;
// stdout stays the same JSON. A cursor inside the range gets no note.
func TestEventsListCursorPastNewest(t *testing.T) {
	testenv.Isolate(t)
	db, err := store.Open(defaultDBPath("keeper-cli"))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"e1", "e2"} {
		if _, err := db.InsertVendorEvent(context.Background(), store.VendorEvent{EventID: id, Type: "user.twofa_disabled", Source: "sweep",
			OccurredAt: "2026-10-28T00:00:00Z", ResourceType: "user", ResourceID: "4421/ada@contoso.com", Client: "4421"}); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()

	run := func(since string) (string, string) {
		var out, errOut bytes.Buffer
		cmd := RootCmd()
		cmd.SetOut(&out)
		cmd.SetErr(&errOut)
		cmd.SetArgs([]string{"events", "list", "--since", since, "--json"})
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		return out.String(), errOut.String()
	}
	out, errOut := run("41")
	if !strings.Contains(errOut, "cursor 41 is past the newest event 2") || !strings.Contains(errOut, "--since 0") {
		t.Fatalf("stderr = %q", errOut)
	}
	if !strings.Contains(out, `"cursor": "41"`) || !strings.Contains(out, `"events": []`) {
		t.Fatalf("stdout = %q", out)
	}
	if _, errOut = run("1"); errOut != "" {
		t.Fatalf("in-range cursor got stderr %q", errOut)
	}
	if _, errOut = run("2"); errOut != "" {
		t.Fatalf("newest cursor got stderr %q", errOut)
	}
}
