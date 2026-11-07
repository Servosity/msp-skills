// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// The additive BYOC numbers walk: e911 reads only the rows it wrote, and it
// never disturbs the unfiltered numbers coverage numbers inventory needs.

package cli

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"zoom-phone-pp-cli/internal/store"
)

func localByocFindings(e localE911Result) string {
	out := []string{}
	for _, f := range e.Findings {
		if f.Kind == "BYOC_CARRIER_UPDATE_REQUIRED" {
			out = append(out, f.EntityID)
		}
	}
	return strings.Join(out, ",")
}

func TestLocalByocWalkIsAdditive(t *testing.T) {
	localTestEnv(t)
	var mu sync.Mutex
	byoc := []string{
		`{"id":"pn-2","number":"+15125550102","source":"external","status":"available","carrier":{"code":1,"name":"Bandwidth"},"emergency_address_status":1}`,
		`{"id":"pn-3","number":"+15125550103","source":"external","status":"available","carrier":{"code":1,"name":"Bandwidth"},"emergency_address_status":1}`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch strings.TrimPrefix(r.URL.Path, "/v2") {
		case "/phone/users":
			_, _ = w.Write([]byte(`{"users":[` + localTestUsers[0] + `],"next_page_token":""}`))
		case "/phone/numbers":
			if r.URL.Query().Get("type") == "byoc" {
				mu.Lock()
				body := `{"phone_numbers":[` + strings.Join(byoc, ",") + `],"next_page_token":""}`
				mu.Unlock()
				_, _ = w.Write([]byte(body))
				return
			}
			_, _ = w.Write([]byte(`{"phone_numbers":[` +
				`{"id":"pn-1","number":"+14155550101","source":"internal","status":"available","assignee":{"id":"u-jane","name":"Jane Doe","type":"user"}},` +
				`{"id":"pn-2","number":"+15125550102","source":"external","status":"available"},` +
				`{"id":"pn-3","number":"+15125550103","source":"external","status":"available"}` +
				`],"next_page_token":""}`))
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
	e911 := func() localE911Result {
		stdout, _, err := localRun(t, newNovelE911AuditCmd, "--db", db)
		if err != nil {
			t.Fatalf("e911: %v", err)
		}
		var res localE911Result
		localDecode(t, stdout, &res)
		return res
	}
	byocArgs := []string{"--resources", "numbers", "--db", db, "--resource-param", "numbers:type=byoc", "--json"}

	// 1. Full unfiltered numbers walk: the inventory is assessed; e911 has
	// no BYOC walk yet and says how to get one (an additive, never --full walk).
	localRunSync(t, "--full", "--resources", "users,numbers", "--db", db, "--json")
	if inv := inventory(); !inv.Assessed || inv.Counts.Total != 3 {
		t.Fatalf("after full sync: assessed=%v total=%d reasons=%v", inv.Assessed, inv.Counts.Total, inv.MissingReasons)
	}
	skipped := strings.Join(e911().SkippedChecks, "\n")
	if !strings.Contains(skipped, "sync --resources numbers --resource-param numbers:type=byoc (additive") || strings.Contains(skipped, "sync --full --resources numbers --resource-param") {
		t.Fatalf("BYOC hint = %q", skipped)
	}

	// 2. Additive BYOC walk: the inventory keeps every number and stays
	// assessed; e911 flags both BYOC numbers.
	localRunSync(t, byocArgs...)
	if scope := localScopeRows(t, db); scope["numbers"] != "0|1|" {
		t.Fatalf("the byoc walk replaced the unfiltered numbers record: %v", scope)
	}
	if inv := inventory(); !inv.Assessed || inv.Counts.Total != 3 || len(inv.MissingLocal) != 0 {
		t.Fatalf("after the byoc walk: assessed=%v total=%d missing=%v reasons=%v", inv.Assessed, inv.Counts.Total, inv.MissingLocal, inv.MissingReasons)
	}
	if e := e911(); localByocFindings(e) != "pn-2,pn-3" || localContains(e.MissingLocal, "numbers") {
		t.Fatalf("after the byoc walk: BYOC=%q missing=%v", localByocFindings(e), e.MissingLocal)
	}

	// 3. pn-3 leaves BYOC: its row still carries the old status, but the new
	// walk did not return it, so it is not flagged.
	mu.Lock()
	byoc = byoc[:1]
	mu.Unlock()
	localRunSync(t, byocArgs...)
	if e := e911(); localByocFindings(e) != "pn-2" {
		t.Fatalf("a number that left BYOC is flagged: BYOC=%q", localByocFindings(e))
	}
	if inv := inventory(); !inv.Assessed || inv.Counts.Total != 3 {
		t.Fatalf("after the second byoc walk: assessed=%v total=%d", inv.Assessed, inv.Counts.Total)
	}

	// 4a. A later plain unfiltered numbers sync rewrites the rows without
	// BYOC fields: the BYOC walk is superseded and the check is skipped.
	localRunSync(t, "--resources", "numbers", "--db", db, "--json")
	e := e911()
	if localByocFindings(e) != "" || !strings.Contains(strings.Join(e.SkippedChecks, "\n"), "a later numbers sync replaced the BYOC walk's rows") {
		t.Fatalf("superseded walk: BYOC=%q skipped=%v", localByocFindings(e), e.SkippedChecks)
	}
	// 4b. An unfiltered --full walk prunes, which drops the BYOC walk too;
	// the inventory is assessed again and the check asks for a new walk.
	localRunSync(t, "--full", "--resources", "numbers", "--db", db, "--json")
	e = e911()
	if localByocFindings(e) != "" || !strings.Contains(strings.Join(e.SkippedChecks, "\n"), "no completed BYOC numbers walk") {
		t.Fatalf("after an unfiltered --full: BYOC=%q skipped=%v", localByocFindings(e), e.SkippedChecks)
	}
	if inv := inventory(); !inv.Assessed || inv.Counts.Total != 3 {
		t.Fatalf("after an unfiltered --full: assessed=%v total=%d", inv.Assessed, inv.Counts.Total)
	}

	// 5. The same walk with --full is still additive: press 4.34.2 never
	// prunes a filtered sync, so every number stays and the inventory keeps
	// its unfiltered coverage.
	localRunSync(t, append([]string{"--full"}, byocArgs...)...)
	inv := inventory()
	if !inv.Assessed || inv.Counts.Total != 3 || len(inv.MissingLocal) != 0 {
		t.Fatalf("after a --full byoc walk: assessed=%v total=%d reasons=%v", inv.Assessed, inv.Counts.Total, inv.MissingReasons)
	}
	if e := e911(); localByocFindings(e) != "pn-2" {
		t.Fatalf("after a --full byoc walk: BYOC=%q", localByocFindings(e))
	}
}

// A byoc walk re-points the unfiltered numbers record only when that record
// was valid for the checkpoint the walk started from.
func TestLocalByocWalkDoesNotRevalidateStaleRecord(t *testing.T) {
	localTestEnv(t)
	path := localTestStore(t, map[string][]string{"numbers": localTestNumbers, "users": localTestUsers})
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	// The numbers record describes an older checkpoint.
	if err := localWriteSyncScope(db, "numbers", 0, "", "2001-01-01T00:00:00Z", "then", true); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	localSimulateRun(t, localTestSyncCommand(t, "--resources", "numbers", "--resource-param", "numbers:type=byoc", "--db", path), path, "numbers")
	stdout, _, err := localRun(t, newNovelNumbersInventoryCmd, "--db", path)
	if err != nil {
		t.Fatalf("inventory: %v", err)
	}
	var inv localInventoryResult
	localDecode(t, stdout, &inv)
	if inv.Assessed || !localContains(inv.MissingLocal, "numbers") {
		t.Fatalf("a byoc walk revalidated a stale numbers record: assessed=%v missing=%v", inv.Assessed, inv.MissingLocal)
	}
}
