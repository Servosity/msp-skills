// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"testing"
)

func TestFailuresAttribution(t *testing.T) {
	newFixture(t)
	out, stderr, err := runCLI(t, "failures", "--since", "24h", "--json")
	if err != nil {
		t.Fatalf("failures: %v\n%s", err, stderr)
	}
	rep := decodeJSON[failuresReport](t, out)
	got := map[string]failureRow{}
	for _, r := range rep.Rows {
		got[r.Type+":"+r.Target] = r
	}
	if r := got["vzdump:108"]; r.Result != "failed" || r.GuestName != "erp" || !strings.Contains(r.Reason, "got timeout") {
		t.Errorf("108 = %+v", r)
	}
	if r := got["vzdump:104"]; r.Result != "incomplete" || r.GuestName != "files" {
		t.Errorf("104 (started, never finished, no error) = %+v", r)
	}
	if r := got["gc:main"]; r.Product != "pbs" || !strings.Contains(r.Reason, "not enough space") || len(r.LogTail) == 0 {
		t.Errorf("gc = %+v", r)
	}
	if r := got["verify:main:v1"]; r.Product != "pbs" || r.Result != "failed" {
		t.Errorf("verify = %+v", r)
	}
	// Negative: guests that finished are never reported.
	for _, k := range []string{"vzdump:100", "vzdump:105"} {
		if _, ok := got[k]; ok {
			t.Errorf("%s finished OK but was reported", k)
		}
	}
	if len(rep.Rows) != 4 || rep.ScannedTasks == 0 || rep.LogsRead == 0 {
		t.Errorf("rows=%d scanned=%d logs=%d", len(rep.Rows), rep.ScannedTasks, rep.LogsRead)
	}
	out, _, err = runCLI(t, "failures", "--type", "vzdump", "--json")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range decodeJSON[failuresReport](t, out).Rows {
		if r.Type != "vzdump" {
			t.Errorf("--type vzdump returned %s", r.Type)
		}
	}
	if _, _, err := runCLI(t, "failures", "--type", "backups", "--json"); ExitCode(err) != 2 {
		t.Errorf("unknown --type must exit 2, got %v", err)
	}
	// Scan cap: with no logs allowed, no vzdump attribution and an honest count.
	out, _, err = runCLI(t, "failures", "--type", "vzdump", "--max-logs", "0", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if rep := decodeJSON[failuresReport](t, out); rep.LogsRead != 0 || !rep.LogCapHit {
		t.Errorf("--max-logs 0: logs=%d cap=%v", rep.LogsRead, rep.LogCapHit)
	}
}

func TestParseVzdumpLog(t *testing.T) {
	res := parseVzdumpLog([]string{
		"INFO: Starting Backup of VM 100 (qemu)", "INFO: Finished Backup of VM 100 (00:00:42)",
		"INFO: Starting Backup of VM 101 (lxc)", "ERROR: Backup of VM 101 failed - unable to open file",
		"INFO: Starting Backup of VM 102 (qemu)",
	})
	want := map[int64]string{100: "ok", 101: "failed", 102: "incomplete"}
	if len(res) != 3 {
		t.Fatalf("got %d results", len(res))
	}
	for _, r := range res {
		if r.Result != want[r.VMID] {
			t.Errorf("vm %d = %s, want %s", r.VMID, r.Result, want[r.VMID])
		}
	}
	if len(parseVzdumpLog([]string{"INFO: nothing to do"})) != 0 {
		t.Error("log without guests must yield no results")
	}
}

func TestFailuresLogCapStillReportsFailedTasks(t *testing.T) {
	newFixture(t)
	out, _, err := runCLI(t, "failures", "--type", "vzdump", "--max-logs", "0", "--json")
	if err != nil {
		t.Fatal(err)
	}
	rep := decodeJSON[failuresReport](t, out)
	if !rep.LogCapHit || len(rep.Rows) != 1 || rep.Rows[0].Result != "failed" || rep.Rows[0].Reason == "" {
		t.Fatalf("the failed vzdump task must still be reported when its log is past the cap: %+v", rep)
	}
}

func TestAgentModeKeepsSparseAnswerFields(t *testing.T) {
	newFixture(t)
	out, _, err := runCLI(t, "posture", "--agent")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"age_hours"`, `"last_attempt_status"`, `"last_verified_ok_at"`, `"verdict"`} {
		if !strings.Contains(out, want) {
			t.Errorf("posture --agent dropped %s", want)
		}
	}
	out, _, _ = runCLI(t, "backup", "coverage", "--include-disks", "--agent")
	if !strings.Contains(out, `"disk"`) {
		t.Error("backup coverage --agent dropped disk")
	}
	out, _, _ = runCLI(t, "failures", "--agent")
	if !strings.Contains(out, `"guest_name"`) {
		t.Error("failures --agent dropped guest_name")
	}
	out, _, _ = runCLI(t, "verify-health", "--agent")
	if !strings.Contains(out, `"newest_verified_ok_at"`) {
		t.Error("verify-health --agent dropped newest_verified_ok_at")
	}
}
