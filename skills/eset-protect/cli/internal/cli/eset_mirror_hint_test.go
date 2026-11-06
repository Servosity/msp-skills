// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored. The local-mirror views must say when they are empty because
// nothing was synced: MCP forwards only "hint:"/"warning:" stderr lines, so
// the JSON synced_hint is the only signal an agent is guaranteed to see.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"eset-protect-pp-cli/internal/cliutil/testenv"
	"eset-protect-pp-cli/internal/store"
)

func mirrorHintRun(t *testing.T, args ...string) (stdout, stderr string) {
	t.Helper()
	cmd := RootCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("%v: %v\nstdout: %s\nstderr: %s", args, err, out.String(), errOut.String())
	}
	return out.String(), errOut.String()
}

func mirrorHintOf(t *testing.T, out string) string {
	t.Helper()
	var v struct {
		SyncedHint *string `json:"synced_hint"`
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if v.SyncedHint == nil {
		return ""
	}
	return *v.SyncedHint
}

// mirrorHintEmptyDB creates a mirror file with the schema but no rows, and
// records a completed sync for the given resources.
func mirrorHintEmptyDB(t *testing.T, synced ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "data.db")
	db, err := store.OpenWithContext(context.Background(), path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	for _, r := range synced {
		if err := db.SaveSyncState(r, "", 0); err != nil {
			t.Fatalf("save sync state %s: %v", r, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

var mirrorHintCommands = [][]string{
	{"fleet", "stale"},
	{"fleet", "gaps"},
	{"triage"},
	{"detections", "spread"},
}

func TestMirrorViewsHintWhenMirrorIsMissing(t *testing.T) {
	testenv.Isolate(t)
	for _, c := range mirrorHintCommands {
		t.Run(strings.Join(c, " "), func(t *testing.T) {
			args := append(append([]string{}, c...), "--db", filepath.Join(t.TempDir(), "absent.db"), "--json")
			out, errOut := mirrorHintRun(t, args...)
			hint := mirrorHintOf(t, out)
			if !strings.Contains(hint, "no local mirror") || !strings.Contains(hint, esetCLIName+" sync --resources ") {
				t.Fatalf("synced_hint = %q, want the no-mirror sync hint", hint)
			}
			if !strings.HasPrefix(strings.TrimSpace(errOut), "hint: no local mirror") {
				t.Fatalf("stderr must start with \"hint:\" so MCP forwards it, got %q", errOut)
			}
		})
	}
}

func TestMirrorViewsHintWhenNothingWasSynced(t *testing.T) {
	testenv.Isolate(t)
	path := mirrorHintEmptyDB(t)
	for _, c := range mirrorHintCommands {
		t.Run(strings.Join(c, " "), func(t *testing.T) {
			args := append(append([]string{}, c...), "--db", path, "--json")
			out, _ := mirrorHintRun(t, args...)
			hint := mirrorHintOf(t, out)
			if !strings.Contains(hint, "no completed sync for") || !strings.Contains(hint, "devices") {
				t.Fatalf("synced_hint = %q, want a never-synced hint naming devices", hint)
			}
		})
	}
}

func TestMirrorViewsNoHintForASyncedEmptyTenant(t *testing.T) {
	testenv.Isolate(t)
	path := mirrorHintEmptyDB(t, "devices", "device-groups", "detections", "incidents")
	for _, c := range mirrorHintCommands {
		t.Run(strings.Join(c, " "), func(t *testing.T) {
			args := append(append([]string{}, c...), "--db", path, "--json")
			out, _ := mirrorHintRun(t, args...)
			if hint := mirrorHintOf(t, out); hint != "" {
				t.Fatalf("a synced empty tenant is a real zero; synced_hint = %q", hint)
			}
		})
	}
}

// mirrorHintMarkSync records a completed (or, with complete false, an
// interrupted) sync for resources in an existing mirror.
func mirrorHintMarkSync(t *testing.T, path string, complete bool, resources ...string) {
	t.Helper()
	db, err := store.OpenWithContext(context.Background(), path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = db.Close() }()
	for _, r := range resources {
		save := db.SaveSyncState
		if !complete {
			save = db.SaveSyncProgress
		}
		if err := save(r, "page-2", 1); err != nil {
			t.Fatalf("save sync state %s: %v", r, err)
		}
	}
}

func mirrorHintSeeded(t *testing.T) string {
	t.Helper()
	return fleetStaleTestSeedDB(t, map[string][]map[string]any{
		"device-groups": fleetStaleTestGroups(),
		"devices":       fleetStaleTestDevices(),
	})
}

func TestMirrorViewsNoHintWhenSyncCompleted(t *testing.T) {
	testenv.Isolate(t)
	path := mirrorHintSeeded(t)
	mirrorHintMarkSync(t, path, true, "devices", "device-groups")
	out, _ := mirrorHintRun(t, "fleet", "stale", "--db", path, "--json")
	if hint := mirrorHintOf(t, out); hint != "" {
		t.Fatalf("a completed sync must not carry a synced_hint, got %q", hint)
	}
}

// Rows from an interrupted first sync are not a trustworthy answer: the
// filtered result can be empty only because later pages were never fetched.
func TestMirrorViewsHintWhenSyncWasInterrupted(t *testing.T) {
	testenv.Isolate(t)
	path := mirrorHintSeeded(t)
	mirrorHintMarkSync(t, path, true, "device-groups")
	mirrorHintMarkSync(t, path, false, "devices")
	out, _ := mirrorHintRun(t, "fleet", "stale", "--db", path, "--json")
	if hint := mirrorHintOf(t, out); !strings.Contains(hint, "no completed sync for devices") {
		t.Fatalf("an interrupted sync must carry a synced_hint naming devices, got %q", hint)
	}
}

// sync --full / --latest-only reset state to complete=1 with a zero timestamp
// before fetching; a sync that dies there has not completed anything.
func TestMirrorViewsHintAfterAResetThatNeverFetched(t *testing.T) {
	testenv.Isolate(t)
	path := mirrorHintSeeded(t)
	mirrorHintMarkSync(t, path, true, "device-groups")
	db, err := store.OpenWithContext(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SaveSyncStateAt("devices", "", 0, time.Time{}); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	out, _ := mirrorHintRun(t, "fleet", "stale", "--db", path, "--json")
	if hint := mirrorHintOf(t, out); !strings.Contains(hint, "no completed sync for devices") {
		t.Fatalf("a reset-only sync state must carry a synced_hint naming devices, got %q", hint)
	}
}

// The human table must show the hint even when it has rows: an interrupted
// sync can leave a partial, non-empty answer.
func TestMirrorViewsHumanTableShowsHintWithRows(t *testing.T) {
	testenv.Isolate(t)
	path := mirrorHintSeeded(t)
	mirrorHintMarkSync(t, path, false, "devices", "device-groups")
	out, _ := mirrorHintRun(t, "fleet", "stale", "--db", path, "--human-friendly")
	if !strings.Contains(out, "CUSTOMER") {
		t.Fatalf("expected a non-empty table, got %q", out)
	}
	if !strings.Contains(out, "no completed sync for devices") {
		t.Fatalf("non-empty human table must print the synced hint, got %q", out)
	}
}
