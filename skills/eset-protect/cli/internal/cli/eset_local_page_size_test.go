// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"eset-protect-pp-cli/internal/cliutil/testenv"
	"eset-protect-pp-cli/internal/store"
)

// TestLocalReadPageSizeIsNotATotal (handfixes.json:
// local-read-page-size-is-not-a-total): local reads treated ESET's pageSize
// as a total-result limit, so `devices list --data-source local --all
// --page-size 1` returned one device and `--page-size 0` (the server default
// page) returned none. A plain --page-size 1 is still one page.
func TestLocalReadPageSizeIsNotATotal(t *testing.T) {
	testenv.Isolate(t)
	setDefaultDBScopeIdentity("", "", "")
	t.Cleanup(func() { setDefaultDBScopeIdentity("", "", "") })
	db, err := store.OpenWithContext(context.Background(), defaultDBPath(esetCLIName))
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 3; i++ {
		raw, _ := json.Marshal(map[string]any{"uuid": fmt.Sprintf("00000000-0000-0000-0000-00000000000%d", i), "displayName": fmt.Sprintf("PC-%d", i)})
		if err := db.Upsert("devices", fmt.Sprintf("00000000-0000-0000-0000-00000000000%d", i), raw); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.SaveSyncState("devices", "", 3); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	count := func(args ...string) int {
		t.Helper()
		out, errOut, err := captureExecuteAll(t, append([]string{"devices", "list", "--data-source", "local", "--json"}, args...)...)
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, errOut)
		}
		var env struct {
			Results []json.RawMessage `json:"results"`
		}
		var rows []json.RawMessage
		if json.Unmarshal(out, &rows) != nil {
			if err := json.Unmarshal(out, &env); err != nil {
				t.Fatalf("%v: not JSON: %v\n%s", args, err, out)
			}
			rows = env.Results
		}
		return len(rows)
	}
	if n := count("--all", "--page-size", "1"); n != 3 {
		t.Fatalf("--all --page-size 1 returned %d of 3 mirrored devices", n)
	}
	if n := count("--page-size", "0"); n != 3 {
		t.Fatalf("--page-size 0 (server default page) returned %d of 3", n)
	}
	if n := count("--page-size", "1"); n != 1 {
		t.Fatalf("--page-size 1 without --all is one page, got %d", n)
	}
}

// TestLocalReadRefusesRecursiveGroupFilter (handfixes.json:
// local-group-filter-refused): ESET's deviceGroupUuid filter includes every
// subgroup, but the local reader applied it as exact equality on a field the
// mirrored rows do not carry, so a local (or outage-fallback) read returned
// an empty, successful result. It must refuse instead.
func TestLocalReadRefusesRecursiveGroupFilter(t *testing.T) {
	testenv.Isolate(t)
	setDefaultDBScopeIdentity("", "", "")
	t.Cleanup(func() { setDefaultDBScopeIdentity("", "", "") })
	db, err := store.OpenWithContext(context.Background(), defaultDBPath(esetCLIName))
	if err != nil {
		t.Fatal(err)
	}
	// The patch row names its device's DIRECT group (a Workstations subgroup);
	// the query names the customer group above it.
	if err := db.Upsert("device-patches", "p1", json.RawMessage(`{"deviceUuid":"00000000-0000-0000-0000-000000000001","deviceGroupUuid":"00000000-0000-0000-0000-0000000000c2"}`)); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveSyncState("device-patches", "", 1); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	out, errOut, err := captureExecuteAll(t, "device-patches", "--device-group-uuid", "00000000-0000-0000-0000-0000000000c1", "--data-source", "local", "--all", "--json")
	if err == nil {
		t.Fatalf("a recursive group filter was applied locally and returned: %s\n%s", out, errOut)
	}
}
