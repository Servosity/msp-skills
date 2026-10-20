// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"strings"
	"testing"

	"backup-radar-pp-cli/internal/cliutil/testenv"
	"backup-radar-pp-cli/internal/radar"
)

// TestNovelStaleHelpWires smoke-tests that the stale command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelStaleHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"stale", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("stale --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "stale"} {
		if !strings.Contains(help, want) {
			t.Fatalf("stale --help missing %q in output:\n%s", want, help)
		}
	}
}

// TestGroupStaleByClientKeepsSameNamedClientsApart: two ScalePad clients both
// named Acme must stay two groups (Codex / adversarial round 4, 2026-10-19).
func TestGroupStaleByClientKeepsSameNamedClientsApart(t *testing.T) {
	rows := []radar.StaleRow{
		{Client: "Acme", ClientID: "c1", Device: "SRV-01"},
		{Client: "Acme", ClientID: "c2", Device: "SRV-01"},
		{Client: "Acme", ClientID: "c1", Device: "SRV-02"},
		{Client: "Acme", Device: "LEGACY"}, // v2-only: no id, groups by name alone
	}
	got := groupStaleByClient(rows)
	if len(got) != 3 || got[0].ClientID != "c1" || got[0].Count != 2 || got[1].ClientID != "c2" || got[1].Count != 1 || got[2].ClientID != "" || got[2].Count != 1 {
		t.Fatalf("by_client merged same-named clients: %+v", got)
	}
}

// TestImportPayloadWrapsObjectsForArrayEndpoints: POST /backups takes a root
// array, so an object row is wrapped and an array row is posted as given.
func TestImportPayloadWrapsObjectsForArrayEndpoints(t *testing.T) {
	got, err := importPayload("backups", []byte(`{"client_name":"Acme","device_name":"SRV-01"}`))
	if err != nil {
		t.Fatal(err)
	}
	arr, ok := got.([]any)
	if !ok || len(arr) != 1 || arr[0].(map[string]any)["client_name"] != "Acme" {
		t.Fatalf("object row for an array endpoint must be wrapped: %#v", got)
	}
	got, err = importPayload("backups", []byte(`[{"client_name":"Acme"},{"client_name":"Beta"}]`))
	if err != nil || len(got.([]any)) != 2 {
		t.Fatalf("array row must be posted as given: %#v %v", got, err)
	}
	if _, err := importPayload("backups", []byte(`"just a string"`)); err == nil {
		t.Fatal("a scalar row must be refused")
	}
	got, err = importPayload("other", []byte(`{"a":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.(map[string]any); !ok {
		t.Fatalf("object endpoints keep the bare object: %#v", got)
	}
}
