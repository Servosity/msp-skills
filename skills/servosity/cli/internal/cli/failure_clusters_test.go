// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"strings"
	"testing"

	"servosity-msp-pp-cli/internal/cliutil/testenv"
)

// TestNovelFailureClustersHelpWires smoke-tests that the failure-clusters command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelFailureClustersHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"failure-clusters", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("failure-clusters --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "failure-clusters"} {
		if !strings.Contains(help, want) {
			t.Fatalf("failure-clusters --help missing %q in output:\n%s", want, help)
		}
	}
}

func TestNormalizeFailureTextGroupsOneRootCause(t *testing.T) {
	a := normalizeFailureText(`2026-10-03T02:00:11Z Fatal: unable to open C:\Users\ops\AppData\x.lock: VSS snapshot 3f2a9c1e timed out after 600s (id 0c9b0f51-2d4e-4f8a-9f1c-1a2b3c4d5e6f)`)
	b := normalizeFailureText(`2026-10-09T23:59:01Z Fatal: unable to open D:\data\y.lock: VSS snapshot 9abc12de timed out after 900s (id 11111111-2222-3333-4444-555555555555)`)
	if a != b {
		t.Fatalf("one root cause produced two signatures:\n%s\n%s", a, b)
	}
	if c := normalizeFailureText("repository is locked by another process"); c == a {
		t.Fatal("two different root causes collapsed into one signature")
	}
	if got := normalizeFailureText("   "); got != "(no error text)" {
		t.Fatalf("empty text signature = %q", got)
	}
}

func TestClusterFailuresRanksByHostsThenClients(t *testing.T) {
	events := []failureEvent{
		{engine: "restic", host: "srv1", text: "VSS timed out after 600s", company: 1},
		{engine: "restic", host: "srv1", text: "VSS timed out after 900s", company: 1},
		{engine: "restic", host: "srv2", text: "VSS timed out after 30s", company: 2},
		{engine: "restic", host: "srv3", text: "disk full", company: 3},
		{engine: "dr", host: "srv3", text: "VSS timed out after 30s", company: 3},
	}
	got := clusterFailures(events)
	if len(got) != 3 {
		t.Fatalf("clusters = %d, want 3 (engine is part of the key)", len(got))
	}
	if got[0].Engine != "restic" || got[0].Hosts != 2 || got[0].Clients != 2 || got[0].Failures != 3 {
		t.Fatalf("top cluster = %+v, want restic VSS cluster with 2 hosts, 2 clients, 3 failures", *got[0])
	}
}

func TestFailureItemsAcceptsEveryEnvelope(t *testing.T) {
	for name, raw := range map[string]string{
		"array":  `[{"errors":"x"}]`,
		"page":   `{"count":1,"results":[{"errors":"x"}]}`,
		"single": `{"errors":"x"}`,
	} {
		if items := failureItems([]byte(raw)); len(items) != 1 || stringField(items[0], "errors") != "x" {
			t.Errorf("%s: failureItems = %v", name, items)
		}
	}
}

func TestFailureHostReadsEachEngineShape(t *testing.T) {
	restic := map[string]any{"resticbackupset": map[string]any{"resticbackup": map[string]any{"id": "r1", "device_name": "SRV-1"}}}
	dr := map[string]any{"drbackup": map[string]any{"id": "d1"}}
	if got := failureHost(restic, "restic"); got != "SRV-1" {
		t.Errorf("restic host = %q", got)
	}
	if got := failureHost(dr, "dr"); got != "d1" {
		t.Errorf("dr host = %q", got)
	}
}
