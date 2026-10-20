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
