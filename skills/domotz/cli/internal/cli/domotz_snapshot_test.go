// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// The store-backed novels answer for the whole account, so the store must be
// the account's whole, current set of Collectors and devices (classes
// deleted-rows-read-as-current and incomplete-snapshot-read-as-complete, the
// shared layer's betterstack and atera Codex r1 classes, pre-closed here on
// Domotz's documented shapes: DELETE /agent/{agent_id}, a Collector's device
// list answering an error, `sync --resources` naming other resources).

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"domotz-pp-cli/internal/cliutil/testenv"
)

func runDomotzErr(args ...string) (string, error) {
	cmd := RootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

// A device removed from a Collector and a Collector removed from the account
// leave the store on the next plain sync; before, only `sync --full` pruned,
// so fleet offline kept reporting a device the MSP had already removed.
func TestDeletedDevicesAndCollectorsLeaveTheStore(t *testing.T) {
	testenv.Isolate(t)
	fake := newFakeDomotz(t, map[string]string{
		"100": `[{"id":1,"display_name":"NAS-Acme","status":"DOWN","importance":"VITAL"},{"id":2,"display_name":"Old-Printer","status":"DOWN","importance":"FLOATING"}]`,
		"200": `[{"id":3,"display_name":"FW-Gone","status":"DOWN","importance":"VITAL"}]`,
	})
	db := filepath.Join(t.TempDir(), "data.db")
	runDomotz(t, "sync", "--db", db, "--no-learn")
	if out := runDomotz(t, "fleet", "offline", "--db", db, "--json", "--no-learn"); !strings.Contains(out, "Old-Printer") || !strings.Contains(out, "FW-Gone") {
		t.Fatalf("the first sync did not store the devices:\n%s", out)
	}
	fake.change(func(devices map[string]string, _ map[string]bool) {
		devices["100"] = `[{"id":1,"display_name":"NAS-Acme","status":"DOWN","importance":"VITAL"}]`
		delete(devices, "200")
	})
	runDomotz(t, "sync", "--db", db, "--no-learn")
	out := runDomotz(t, "fleet", "offline", "--db", db, "--json", "--no-learn")
	if strings.Contains(out, "Old-Printer") || strings.Contains(out, "FW-Gone") {
		t.Errorf("a removed device or Collector is still reported after a plain sync:\n%s", out)
	}
	if !strings.Contains(out, "NAS-Acme") {
		t.Errorf("the device still in the account is gone:\n%s", out)
	}
	if out := runDomotz(t, "fleet", "health", "--db", db, "--json", "--no-learn"); !strings.Contains(out, `"agents_total": 1`) {
		t.Errorf("fleet health still counts the removed Collector:\n%s", out)
	}
}

// A sync whose device walk did not finish (one Collector's device list
// answered an error) is a partial store: the device novels refuse it instead
// of reporting the Collectors they could read as the whole fleet.
func TestPartialDeviceSyncIsRefused(t *testing.T) {
	testenv.Isolate(t)
	fake := newFakeDomotz(t, map[string]string{
		"100": `[{"id":1,"display_name":"NAS-Acme","status":"ONLINE"}]`,
		"200": `[{"id":3,"display_name":"FW-Beta","status":"DOWN","importance":"VITAL"}]`,
	})
	fake.change(func(_ map[string]string, fail map[string]bool) { fail["200"] = true })
	db := filepath.Join(t.TempDir(), "data.db")
	_, _ = runDomotzErr("sync", "--db", db, "--no-learn")
	for _, args := range [][]string{{"fleet", "offline"}, {"fleet", "health"}, {"fleet", "inventory"}, {"fleet", "unmonitored"}} {
		out, err := runDomotzErr(append(args, "--db", db, "--json", "--no-learn")...)
		if err == nil {
			t.Errorf("%v answered from a partial device sync:\n%s", args, out)
		}
	}
}

// A store where the devices were never synced (only other resources were) is
// not an empty fleet: the device novels refuse it.
func TestNeverSyncedDevicesAreRefused(t *testing.T) {
	testenv.Isolate(t)
	newFakeDomotz(t, map[string]string{"100": `[{"id":1,"display_name":"NAS-Acme","status":"DOWN"}]`})
	db := filepath.Join(t.TempDir(), "data.db")
	runDomotz(t, "sync", "--db", db, "--no-learn", "--resources", "alert-profile")
	for _, args := range [][]string{{"fleet", "offline"}, {"fleet", "health"}, {"fleet", "breakdown"}, {"fleet", "stale"}} {
		out, err := runDomotzErr(append(args, "--db", db, "--json", "--no-learn")...)
		if err == nil {
			t.Errorf("%v reported an unsynced fleet as empty:\n%s", args, out)
		}
	}
}
