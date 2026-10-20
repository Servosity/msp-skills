// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"eset-protect-pp-cli/internal/cliutil/testenv"
	"eset-protect-pp-cli/internal/store"
)

// TestLocalReadPageSizeIsNotATotal (handfixes.json:
// local-read-page-size-is-not-a-total): local reads treated ESET's pageSize
// as a total-result limit, so `devices list --data-source local --all
// --page-size 1` returned one device and `--page-size 0` (the server default
// page of 50) returned none. A plain --page-size 1 is still one page.
func TestLocalReadPageSizeIsNotATotal(t *testing.T) {
	testenv.Isolate(t)
	setDefaultDBScopeIdentity("", "", "")
	t.Cleanup(func() { setDefaultDBScopeIdentity("", "", "") })
	db, err := store.OpenWithContext(context.Background(), defaultDBPath(esetCLIName))
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 60; i++ {
		id := fmt.Sprintf("00000000-0000-0000-0000-%012d", i)
		raw, _ := json.Marshal(map[string]any{"uuid": id, "displayName": fmt.Sprintf("PC-%d", i)})
		if err := db.Upsert("devices", id, raw); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.SaveSyncState("devices", "", 60); err != nil {
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
	if n := count("--all", "--page-size", "1"); n != 60 {
		t.Fatalf("--all --page-size 1 returned %d of 60 mirrored devices", n)
	}
	if n := count("--page-size", "0"); n != 50 {
		t.Fatalf("--page-size 0 (ESET's default page of 50) returned %d of 60", n)
	}
	if n := count("--page-size", "1"); n != 1 {
		t.Fatalf("--page-size 1 without --all is one page, got %d", n)
	}
	// local-read-page-size-is-not-a-total: no --page-size is ESET's default
	// page, as a live read returns.
	if n := count(); n != 50 {
		t.Fatalf("no --page-size returned %d of 60, want the default page of 50", n)
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

// TestLocalFilterProtoDefaults (handfixes.json: local-filter-proto-defaults):
// protobuf JSON omits a field holding its default, so with one muted device
// and one device with no isMuted field, `devices list --is-muted=false
// --data-source local --all` returned nothing; and an *_UNSPECIFIED enum
// filter (ESET's "no filter") matched only rows that literally said so.
func TestLocalFilterProtoDefaults(t *testing.T) {
	testenv.Isolate(t)
	setDefaultDBScopeIdentity("", "", "")
	t.Cleanup(func() { setDefaultDBScopeIdentity("", "", "") })
	db, err := store.OpenWithContext(context.Background(), defaultDBPath(esetCLIName))
	if err != nil {
		t.Fatal(err)
	}
	for id, body := range map[string]string{
		"00000000-0000-0000-0000-00000000000a": `{"uuid":"00000000-0000-0000-0000-00000000000a","displayName":"MUTED","isMuted":true,"functionalityStatus":"DEVICE_FUNCTIONALITY_STATUS_OK"}`,
		"00000000-0000-0000-0000-00000000000b": `{"uuid":"00000000-0000-0000-0000-00000000000b","displayName":"DEFAULTED","functionalityStatus":"DEVICE_FUNCTIONALITY_STATUS_OK"}`,
	} {
		if err := db.Upsert("devices", id, json.RawMessage(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.SaveSyncState("devices", "", 2); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	names := func(args ...string) string {
		t.Helper()
		out, errOut, err := captureExecuteAll(t, append([]string{"devices", "list", "--data-source", "local", "--all", "--json"}, args...)...)
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, errOut)
		}
		return string(out)
	}
	if got := names("--is-muted=false"); !strings.Contains(got, "DEFAULTED") || strings.Contains(got, "MUTED\"") {
		t.Fatalf("--is-muted=false must return the device whose isMuted is omitted, not the muted one: %s", got)
	}
	if got := names("--functionality-status", "DEVICE_FUNCTIONALITY_STATUS_UNSPECIFIED"); !strings.Contains(got, "DEFAULTED") || !strings.Contains(got, "MUTED") {
		t.Fatalf("an UNSPECIFIED filter is no filter: %s", got)
	}
}

// TestLocalEmptyAfterCompleteSync (handfixes.json: local-empty-after-complete-sync):
// a resource whose last sync completed with zero rows read as "Run sync
// first" instead of an empty list.
func TestLocalEmptyAfterCompleteSync(t *testing.T) {
	testenv.Isolate(t)
	setDefaultDBScopeIdentity("", "", "")
	t.Cleanup(func() { setDefaultDBScopeIdentity("", "", "") })
	db, err := store.OpenWithContext(context.Background(), defaultDBPath(esetCLIName))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SaveSyncState("detections", "", 0); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	out, errOut, err := captureExecuteAll(t, "detections", "list", "--data-source", "local", "--all", "--json")
	if err != nil {
		t.Fatalf("an empty, completely synced resource failed: %v\n%s%s", err, out, errOut)
	}
}
