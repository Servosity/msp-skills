// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/synology/handfixes.json under
// the "export-writes-through-cmd-out", "deliver-after-profile" and
// "deliver-on-failure" entries.

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readDelivered(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

// TestDeliverCapturesEveryCommandsOutput: export, agent-context and version
// wrote to the process stdout and help printed through root's writer, so
// --deliver file:... produced an empty file for each.
func TestDeliverCapturesEveryCommandsOutput(t *testing.T) {
	sweepIsolate(t)
	dir := t.TempDir()
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"export", []string{"export", "group", "--dry-run"}, `"dry_run"`},
		{"agent-context", []string{"agent-context"}, `"schema_version"`},
		{"version", []string{"version"}, "synology-cli "},
		{"help", []string{"help", "abb"}, "Usage:"},
	} {
		target := filepath.Join(dir, tc.name+".out")
		_, _, code := sweepExecute(t, append(tc.args, "--deliver", "file:"+target)...)
		if code != 0 {
			t.Fatalf("%s exited %d", tc.name, code)
		}
		if got := readDelivered(t, target); !strings.Contains(got, tc.want) {
			t.Fatalf("%s --deliver wrote %q, want it to contain %q", tc.name, got, tc.want)
		}
	}
	if _, _, code := sweepExecute(t, "completion", "bash", "--deliver", "file:"+filepath.Join(dir, "c.out")); code != 2 {
		t.Fatalf("completion --deliver exited %d, want a usage refusal (2)", code)
	}
}

// TestDeliverOnFailure: a command that prints and then fails (auth status
// with no login, exit 4; fleet status and the abb reports exit 6 with
// findings) still delivers what it printed, instead of leaving the sink with
// the previous run's report.
func TestDeliverOnFailure(t *testing.T) {
	sweepIsolate(t)
	target := filepath.Join(t.TempDir(), "status.json")
	_, _, code := sweepExecute(t, "auth", "status", "--json", "--deliver", "file:"+target)
	if code != 4 {
		t.Fatalf("auth status without a login exited %d, want 4", code)
	}
	if got := readDelivered(t, target); !strings.Contains(got, `"authenticated": false`) {
		t.Fatalf("the failed command delivered %q", got)
	}
}

// TestDeliverAfterProfile: the deliver sink was wired before --profile was
// applied, so a --deliver the profile restores was ignored; and profile save
// performed the --deliver it was only meant to record.
func TestDeliverAfterProfile(t *testing.T) {
	sweepIsolate(t)
	target := filepath.Join(t.TempDir(), "nightly.out")
	if _, _, code := sweepExecute(t, "profile", "save", "nightly", "--deliver", "file:"+target); code != 0 {
		t.Fatalf("profile save exited %d", code)
	}
	if got := readDelivered(t, target); got != "" {
		t.Fatalf("profile save delivered its own confirmation: %q", got)
	}
	if _, _, code := sweepExecute(t, "version", "--profile", "nightly"); code != 0 {
		t.Fatalf("version --profile exited %d", code)
	}
	if got := readDelivered(t, target); !strings.Contains(got, "synology-cli ") {
		t.Fatalf("the profile's --deliver was not honoured: %q", got)
	}
}
