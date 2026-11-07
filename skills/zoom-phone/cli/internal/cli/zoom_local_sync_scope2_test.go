// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Codex round-4: sync-scope recording is driven by the run's own resource
// selection, and coverage needs a full reconciled walk.

package cli

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"
	"zoom-phone-pp-cli/internal/store"
)

func localTestSyncCommand(t *testing.T, args ...string) *cobra.Command {
	t.Helper()
	for _, c := range RootCmd().Commands() {
		if c.Name() == "sync" {
			if err := c.ParseFlags(args); err != nil {
				t.Fatal(err)
			}
			return c
		}
	}
	t.Fatal("no sync command")
	return nil
}

// ---- item 1: only the resources this run synced are recorded ----

func TestLocalSyncSelectionResolvesLikeSync(t *testing.T) {
	localTestEnv(t)
	users := localSyncSelection(localTestSyncCommand(t, "--resources", "users"))
	if !users["users"] || !users["phone_users_call_history"] || users["numbers"] || users["sites"] || users["phone_sites_outbound_calling_exception_rules"] {
		t.Fatalf("--resources users selection = %v", users)
	}
	numbers := localSyncSelection(localTestSyncCommand(t, "--resources", "numbers"))
	if len(numbers) != 1 || !numbers["numbers"] {
		t.Fatalf("--resources numbers selection = %v", numbers)
	}
	all := localSyncSelection(localTestSyncCommand(t))
	for _, want := range []string{"users", "numbers", "assigned", "unassigned", "emergency_addresses", "phone_users_call_history", "roles"} {
		if !all[want] {
			t.Fatalf("default selection lacks %s", want)
		}
	}
}

// The exact Codex sequence: a filtered users sync, then a numbers-only sync
// whose checkpoints land in the same second. The numbers run must not
// rewrite the users record.
func TestLocalNumbersOnlySyncKeepsUsersFilterMark(t *testing.T) {
	localTestEnv(t)
	path := localTestStoreScoped(t, map[string][]string{"users": localTestUsers, "numbers": localTestNumbers},
		map[string]string{"users": "site_id=site-A"})
	if scope := localScopeRows(t, path); scope["users"] != "1|1|site_id=site-A" {
		t.Fatalf("setup scope = %v", scope)
	}
	// Run 2 writes only numbers checkpoints, in the same second as the users
	// checkpoint from the fixture.
	run2 := localTestSyncCommand(t, "--full", "--resources", "numbers", "--db", path)
	localSimulateRun(t, run2, path, "numbers")
	scope := localScopeRows(t, path)
	if scope["users"] != "1|1|site_id=site-A" {
		t.Fatalf("a numbers-only run rewrote the users record: %v", scope)
	}
	if scope["numbers"] != "0|1|" {
		t.Fatalf("numbers record = %q", scope["numbers"])
	}
}

func TestLocalRealSyncSequenceKeepsUsersFilterMark(t *testing.T) {
	localTestEnv(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch strings.TrimPrefix(r.URL.Path, "/v2") {
		case "/phone/users":
			_, _ = w.Write([]byte(`{"users":[` + localTestUsers[0] + `],"next_page_token":""}`))
		case "/phone/numbers":
			_, _ = w.Write([]byte(`{"phone_numbers":[` + localTestNumbers[0] + `],"next_page_token":""}`))
		default:
			_, _ = w.Write([]byte(`{"next_page_token":""}`))
		}
	}))
	defer srv.Close()
	t.Setenv("ZOOM_PHONE_BASE_URL", srv.URL+"/v2")
	db := filepath.Join(t.TempDir(), "data.db")
	localRunSync(t, "--full", "--resources", "users", "--db", db, "--resource-param", "users:site_id=site-A", "--json")
	localRunSync(t, "--full", "--resources", "numbers", "--db", db, "--json")
	scope := localScopeRows(t, db)
	if scope["users"] != "1|0|site_id=site-A" || scope["numbers"] != "0|1|" {
		t.Fatalf("scope after filtered users then numbers-only = %v", scope)
	}
}

// ---- item 2: coverage needs a full reconciled walk ----

func TestLocalSyncWasFullWalk(t *testing.T) {
	localTestEnv(t)
	cases := []struct {
		args []string
		want bool
	}{
		{[]string{"--full"}, true},
		{[]string{}, false},
		{[]string{"--full", "--no-prune"}, false},
		{[]string{"--full", "--latest-only"}, false},
		{[]string{"--full", "--max-pages", "5"}, false},
		{[]string{"--full", "--since", "7d"}, false},
	}
	for _, tc := range cases {
		if got := localSyncWasFullWalk(localTestSyncCommand(t, tc.args...)); got != tc.want {
			t.Errorf("full walk for %v = %v, want %v", tc.args, got, tc.want)
		}
	}
	t.Setenv("PRINTING_PRESS_DOGFOOD", "1")
	if localSyncWasFullWalk(localTestSyncCommand(t, "--full")) {
		t.Errorf("dogfood caps pages at 1, so --full is not a full walk there")
	}
}

func TestLocalPlainSyncLeavesStaleRowsUnassessed(t *testing.T) {
	localTestEnv(t)
	var mu sync.Mutex
	numbers := []string{
		`{"id":"pn-1","number":"+14155550101","source":"internal","status":"available","assignee":{"id":"u-jane","name":"Jane Doe","type":"user"}}`,
		`{"id":"pn-2","number":"+14155550102","source":"internal","status":"available"}`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch strings.TrimPrefix(r.URL.Path, "/v2") {
		case "/phone/users":
			_, _ = w.Write([]byte(`{"users":[` + localTestUsers[0] + `],"next_page_token":""}`))
		case "/phone/numbers":
			mu.Lock()
			body := `{"phone_numbers":[` + strings.Join(numbers, ",") + `],"next_page_token":""}`
			mu.Unlock()
			_, _ = w.Write([]byte(body))
		default:
			_, _ = w.Write([]byte(`{"next_page_token":""}`))
		}
	}))
	defer srv.Close()
	t.Setenv("ZOOM_PHONE_BASE_URL", srv.URL+"/v2")
	db := filepath.Join(t.TempDir(), "data.db")
	inventory := func() localInventoryResult {
		stdout, _, err := localRun(t, newNovelNumbersInventoryCmd, "--db", db)
		if err != nil {
			t.Fatalf("inventory: %v", err)
		}
		var res localInventoryResult
		localDecode(t, stdout, &res)
		return res
	}

	localRunSync(t, "--full", "--resources", "users,numbers", "--db", db, "--json")
	if res := inventory(); !res.Assessed || res.Counts.Total != 2 {
		t.Fatalf("after a full sync: assessed=%v total=%d missing=%v reasons=%v", res.Assessed, res.Counts.Total, res.MissingLocal, res.MissingReasons)
	}

	// The API stops returning pn-2. A plain sync keeps the stale row.
	mu.Lock()
	numbers = numbers[:1]
	mu.Unlock()
	localRunSync(t, "--resources", "numbers", "--db", db, "--json")
	res := inventory()
	if res.Counts.Total != 2 {
		t.Fatalf("plain sync: total=%d; expected the removed number to linger (no prune without --full)", res.Counts.Total)
	}
	if res.Assessed || strings.Join(res.MissingLocal, ",") != "numbers" || !strings.Contains(res.MissingReasons["numbers"], "not a full reconciled walk") ||
		!strings.Contains(res.MissingReasons["numbers"], "sync --full --resources numbers") {
		t.Fatalf("plain sync: assessed=%v missing=%v reasons=%v", res.Assessed, res.MissingLocal, res.MissingReasons)
	}

	// A full sync prunes it and the inventory is assessed again.
	localRunSync(t, "--full", "--resources", "numbers", "--db", db, "--json")
	res = inventory()
	if !res.Assessed || res.Counts.Total != 1 || len(res.Unassigned) != 0 {
		t.Fatalf("after --full: assessed=%v total=%d unassigned=%v missing=%v", res.Assessed, res.Counts.Total, res.Unassigned, res.MissingLocal)
	}
}

// A store synced before scope recording existed (checkpoints, no record)
// does not cover the account, and says so.
func TestLocalLegacyStoreWithoutScopeRecordIsUnassessed(t *testing.T) {
	localTestEnv(t)
	path := filepath.Join(t.TempDir(), "data.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, rt := range []string{"numbers", "users"} {
		if err := db.SaveSyncState(rt, "", 0); err != nil {
			t.Fatal(err)
		}
	}
	_ = db.Close()
	stdout, _, err := localRun(t, newNovelNumbersInventoryCmd, "--db", path)
	if err != nil {
		t.Fatalf("inventory: %v", err)
	}
	var res localInventoryResult
	localDecode(t, stdout, &res)
	if res.Assessed || strings.Join(res.MissingLocal, ",") != "numbers,users" {
		t.Fatalf("legacy store: assessed=%v missing=%v", res.Assessed, res.MissingLocal)
	}
	for _, rt := range []string{"numbers", "users"} {
		if !strings.Contains(res.MissingReasons[rt], "no full reconciled sync is recorded") || !strings.Contains(res.MissingReasons[rt], "sync --full --resources "+rt) {
			t.Fatalf("legacy %s reason = %q", rt, res.MissingReasons[rt])
		}
	}
}
