// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"datto-saas-pp-cli/internal/cliutil"
	"datto-saas-pp-cli/internal/cliutil/testenv"
	"datto-saas-pp-cli/internal/store"
)

func seedRows(t *testing.T, domains, seats, apps []string) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "data.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, d := range domains {
		if err := db.UpsertDomains(json.RawMessage(d)); err != nil {
			t.Fatal(err)
		}
	}
	for _, s := range seats {
		if err := db.UpsertSeats(json.RawMessage(s)); err != nil {
			t.Fatal(err)
		}
	}
	for _, a := range apps {
		if err := db.UpsertApplications(json.RawMessage(a)); err != nil {
			t.Fatal(err)
		}
	}
	markSynced(t, db)
	return dbPath
}

func TestBulkPlanRejectsWhitespaceSelector(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("DATTO_SAAS_NO_AUTO_REFRESH", "1")
	db := seedSaasStore(t)
	if out, err := runSaasCmd(t, "seats", "bulk-plan", "--db", db, "--action", "Unlicense", "--customer", "  "); err == nil {
		t.Fatalf("whitespace-only --customer must not build a fleet-wide plan: %s", out)
	}
}

func TestExitInventoryNotReadyWithoutBackupEvidence(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("DATTO_SAAS_NO_AUTO_REFRESH", "1")
	db := seedRows(t,
		[]string{`{"saasCustomerId":5,"saasCustomerName":"Bare","domain":"bare.example","externalSubscriptionId":"Classic:Office365:9"}`},
		[]string{`{"parent_id":"5","mainId":"a@bare.example","seatType":"User","seatState":"Active","billable":true,"remoteId":"r1"}`},
		[]string{`{"parent_id":"5","customerId":5,"suites":[]}`})
	out, err := runSaasCmd(t, "exit-inventory", "5", "--data-source", "local", "--db", db)
	if err != nil {
		t.Fatal(err)
	}
	var inv exitInventory
	if err := json.Unmarshal([]byte(out), &inv); err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	for _, b := range inv.Blockers {
		kinds[b.Kind] = true
	}
	if inv.ReadyToCancel || !kinds["backup_stats_missing"] || !kinds["no_backup_history"] {
		t.Fatalf("missing evidence must block cancellation: %+v", inv.Blockers)
	}
}

func TestExitInventoryAuthErrorOnDetailedStatsFails(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("DATTO_SAAS_NO_AUTO_REFRESH", "1")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/saas/domains":
			_, _ = w.Write([]byte(`[{"saasCustomerId":5,"saasCustomerName":"Bare","domain":"bare.example"}]`))
		case strings.HasSuffix(r.URL.Path, "/seats"):
			_, _ = w.Write([]byte(`[]`))
		case strings.HasSuffix(r.URL.Path, "/applications"):
			_, _ = w.Write([]byte(`{"items":[]}`))
		default:
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"unauthorized"}`))
		}
	}))
	defer srv.Close()
	t.Setenv("DATTO_SAAS_BASE_URL", srv.URL)
	t.Setenv("DATTO_SAAS_PUBLIC_KEY", "pub")
	t.Setenv("DATTO_SAAS_SECRET_KEY", "sec")
	out, err := runSaasCmd(t, "exit-inventory", "5", "--data-source", "live")
	if err == nil {
		t.Fatalf("a 401 on detailedBackupStats must fail the command, got %s", out)
	}
	if !strings.Contains(err.Error(), "401") {
		t.Fatalf("error should name the 401: %v", err)
	}
}

func TestPartialCountsAreNotTreatedAsComplete(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("DATTO_SAAS_NO_AUTO_REFRESH", "1")
	db := seedRows(t, []string{
		`{"saasCustomerId":7,"saasCustomerName":"Split","domain":"a.example","seatsUsed":5,"backupStats":{"activeServicesCount":100}}`,
		`{"saasCustomerId":7,"saasCustomerName":"Split","domain":"b.example","backupStats":{"activeServicesWithRecentBackupCount":100}}`,
	}, []string{`{"parent_id":"7","mainId":"u@a.example","seatType":"User","seatState":"Active","billable":true,"remoteId":"r"}`}, nil)

	out, err := runSaasCmd(t, "seats", "reconcile", "--db", db)
	if err != nil {
		t.Fatal(err)
	}
	rows := decodeRows(t, out)
	if !strings.Contains(out, "seats_used_incomplete") || strings.Contains(out, "seats_used_mismatch") || rows[0]["delta_billable_minus_seats_used"] != nil {
		t.Fatalf("partial seatsUsed must be flagged without deltas: %s", out)
	}

	out, err = runSaasCmd(t, "backup-health", "--db", db)
	if err != nil {
		t.Fatal(err)
	}
	if rows := decodeRows(t, out); rows[0]["coverage_pct"] != nil {
		t.Fatalf("mismatched partial counts must not manufacture coverage: %s", out)
	}
	out, err = runSaasCmd(t, "backup-health", "--db", db, "--below", "50")
	if err != nil || len(decodeRows(t, out)) != 1 {
		t.Fatalf("--below must keep unknown-coverage clients: %s %v", out, err)
	}
}

func TestFlexIDKeepsLargeIntegersExact(t *testing.T) {
	var d saasDomain
	if err := json.Unmarshal([]byte(`{"saasCustomerId":9007199254740993}`), &d); err != nil {
		t.Fatal(err)
	}
	if d.CustomerID != "9007199254740993" {
		t.Fatalf("id = %s", d.CustomerID)
	}
}

func TestExitInventoryBlocksUnusableAppHistory(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("DATTO_SAAS_NO_AUTO_REFRESH", "1")
	good := `{"timeWindow":"1","startTime":1,"activeServiceCount":2,"activeServiceWithBackupCount":2,"activeServiceWithPerfectBackupCount":2}`
	db := seedRows(t,
		[]string{`{"saasCustomerId":5,"saasCustomerName":"Mixed","domain":"m.example","externalSubscriptionId":"Classic:Office365:9","backupStats":{"activeServicesCount":2,"activeServicesWithRecentBackupCount":2}}`},
		[]string{`{"parent_id":"5","mainId":"a@m.example","seatType":"User","seatState":"Active","billable":true,"remoteId":"r1"}`},
		[]string{`{"parent_id":"5","customerId":5,"suites":[{"suiteType":"S","appTypes":[{"appType":"Good","backupHistory":[` + good + `]},{"appType":"Blank","backupHistory":[{}]},{"appType":"Empty","backupHistory":[]}]}]}`})
	out, err := runSaasCmd(t, "exit-inventory", "5", "--data-source", "local", "--db", db)
	if err != nil {
		t.Fatal(err)
	}
	var inv exitInventory
	_ = json.Unmarshal([]byte(out), &inv)
	var unusable *invBlocker
	for i := range inv.Blockers {
		if inv.Blockers[i].Kind == "apps_without_usable_history" {
			unusable = &inv.Blockers[i]
		}
	}
	if inv.ReadyToCancel || unusable == nil || unusable.Count != 2 {
		t.Fatalf("blank and empty histories must block: ready=%v blockers=%+v", inv.ReadyToCancel, inv.Blockers)
	}
}

func TestExitInventoryAutoNeverFallsBackOnHTTPError(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("DATTO_SAAS_NO_AUTO_REFRESH", "1")
	db := seedSaasStore(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"connection refused by policy"}`))
	}))
	defer srv.Close()
	t.Setenv("DATTO_SAAS_BASE_URL", srv.URL)
	t.Setenv("DATTO_SAAS_PUBLIC_KEY", "pub")
	t.Setenv("DATTO_SAAS_SECRET_KEY", "sec")
	if out, err := runSaasCmd(t, "exit-inventory", "123456", "--db", db); err == nil {
		t.Fatalf("a 403 must not fall back to the local store in auto mode: %s", out)
	}
}

func TestExitInventoryHalfConfiguredKeyPairErrors(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("DATTO_SAAS_NO_AUTO_REFRESH", "1")
	t.Setenv("DATTO_SAAS_PUBLIC_KEY", "pub")
	t.Setenv("DATTO_SAAS_SECRET_KEY", "")
	db := seedSaasStore(t)
	if out, err := runSaasCmd(t, "exit-inventory", "123456", "--db", db); err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("a half-configured key pair must error, got out=%s err=%v", out, err)
	}
}

func TestBulkPlanCommandRoundTripsIDsExactly(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("DATTO_SAAS_NO_AUTO_REFRESH", "1")
	db := seedRows(t,
		[]string{`{"saasCustomerId":8,"saasCustomerName":"Comma","domain":"c.example","productType":"Office365","externalSubscriptionId":"Classic:Office365:8"}`},
		[]string{`{"parent_id":"8","mainId":"x@c.example","seatType":"User","seatState":"Unprotected","remoteId":"a,b"}`,
			`{"parent_id":"8","mainId":"y@c.example","seatType":"User","seatState":"Unprotected","remoteId":"it's"}`}, nil)
	out, err := runSaasCmd(t, "seats", "bulk-plan", "--db", db, "--action", "License", "--customer", "8")
	if err != nil {
		t.Fatal(err)
	}
	var plan bulkPlan
	if err := json.Unmarshal([]byte(out), &plan); err != nil || len(plan.Batches) != 1 {
		t.Fatalf("plan = %s (%v)", out, err)
	}
	b := plan.Batches[0]
	quoted := b.Command[strings.Index(b.Command, "--ids ")+len("--ids "):]
	arg := quoted
	if strings.HasPrefix(quoted, "'") { // undo shellQuote
		arg = strings.ReplaceAll(strings.TrimSuffix(strings.TrimPrefix(quoted, "'"), "'"), `'\''`, "'")
	}
	got, err := cliutil.ParseStringList(arg)
	if err != nil || len(got) != len(b.IDs) || got[0] != b.IDs[0] || got[1] != b.IDs[1] {
		t.Fatalf("printed --ids %q parses to %q, want %q (%v)", quoted, got, b.IDs, err)
	}
}

func TestIncompleteCountsNeverLookComplete(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("DATTO_SAAS_NO_AUTO_REFRESH", "1")
	db := seedRows(t,
		[]string{`{"saasCustomerId":9,"saasCustomerName":"NoCounts","domain":"n.example","externalSubscriptionId":"S:9","backupStats":{"activeServicesCount":100,"activeServicesWithRecentBackupCount":100}}`},
		[]string{`{"parent_id":"9","mainId":"u@n.example","seatType":"User","seatState":"Active","billable":true,"remoteId":"r"}`},
		[]string{`{"parent_id":"9","customerId":9,"suites":[{"appTypes":[` +
			`{"appType":"A","backupHistory":[{"startTime":1,"activeServiceCount":1,"activeServiceWithBackupCount":1,"activeServiceWithPerfectBackupCount":1}]},` +
			`{"appType":"B","backupHistory":[{"startTime":1,"activeServiceCount":99,"activeServiceWithBackupCount":99}]}]}]}`})
	out, err := runSaasCmd(t, "seats", "reconcile", "--db", db, "--only-flagged")
	if err != nil || !strings.Contains(out, "seats_used_incomplete") || !strings.Contains(out, `"seats_used": null`) {
		t.Fatalf("no seatsUsed at all must be flagged with null seats_used: %s %v", out, err)
	}
	out, err = runSaasCmd(t, "backup-health", "--db", db)
	if err != nil {
		t.Fatal(err)
	}
	if rows := decodeRows(t, out); rows[0]["perfect_backup_pct"] != nil || !strings.Contains(out, "app_counts_incomplete") {
		t.Fatalf("a missing perfect count must not yield a perfect-backup rate: %s", out)
	}
}

func TestBulkChangeDryRunUsesCanonicalValidation(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("DATTO_SAAS_NO_AUTO_REFRESH", "1")
	for _, ids := range []string{`[""]`, `[" "]`} {
		if _, err := runSaasCmd(t, "seats", "bulk-change", "1", "S:1", "--seat-type", "User", "--action-type", "License", "--ids", ids, "--dry-run"); err == nil {
			t.Fatalf("dry-run accepted ids %q that a real send refuses", ids)
		}
	}
	if _, err := runSaasCmd(t, "seats", "bulk-change", "1", "S:1", "--seat-type", "User", "--action-type", "License", "--ids", `["a,b"]`, "--dry-run"); err != nil {
		t.Fatalf("an id containing a comma must pass as a JSON array: %v", err)
	}
}

func TestBulkPlanBlocksIncompleteSubscriptionMapping(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("DATTO_SAAS_NO_AUTO_REFRESH", "1")
	db := seedRows(t, []string{
		`{"saasCustomerId":11,"saasCustomerName":"Half","domain":"a.example","productType":"Office365","externalSubscriptionId":"Classic:Office365:11"}`,
		`{"saasCustomerId":11,"saasCustomerName":"Half","domain":"b.example","productType":"Office365"}`,
	}, []string{`{"parent_id":"11","mainId":"u@b.example","seatType":"User","seatState":"Unprotected","remoteId":"r"}`}, nil)
	out, err := runSaasCmd(t, "seats", "bulk-plan", "--db", db, "--action", "License", "--customer", "11")
	if err != nil {
		t.Fatal(err)
	}
	var plan bulkPlan
	_ = json.Unmarshal([]byte(out), &plan)
	if len(plan.Batches) != 0 || len(plan.Blocked) != 1 || !strings.HasPrefix(plan.Blocked[0].Reason, "incomplete_subscription_mapping") {
		t.Fatalf("an unmapped domain must block auto subscription choice: %s", out)
	}
}

func TestUndatedWindowsAndZeroCounts(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("DATTO_SAAS_NO_AUTO_REFRESH", "1")
	db := seedRows(t,
		[]string{`{"saasCustomerId":12,"saasCustomerName":"Dates","domain":"d.example","externalSubscriptionId":"S:12","backupStats":{"activeServicesCount":1,"activeServicesWithRecentBackupCount":1}}`},
		[]string{`{"parent_id":"12","mainId":"u@d.example","seatType":"User","seatState":"Active","billable":true,"remoteId":"r"}`},
		[]string{`{"parent_id":"12","customerId":12,"suites":[{"appTypes":[` +
			`{"appType":"Undated","backupHistory":[{"startTime":1,"activeServiceCount":1,"activeServiceWithBackupCount":1,"activeServiceWithPerfectBackupCount":1},{"activeServiceCount":1}]},` +
			`{"appType":"Zero","backupHistory":[{"startTime":5,"activeServiceCount":0,"activeServiceWithBackupCount":0,"activeServiceWithPerfectBackupCount":0}]}]}]}`})
	out, err := runSaasCmd(t, "exit-inventory", "12", "--data-source", "local", "--db", db)
	if err != nil {
		t.Fatal(err)
	}
	var inv exitInventory
	_ = json.Unmarshal([]byte(out), &inv)
	var items []string
	for _, b := range inv.Blockers {
		if b.Kind == "apps_without_usable_history" {
			items = b.Items
		}
	}
	if inv.ReadyToCancel || len(items) != 1 || items[0] != "Undated" {
		t.Fatalf("undated history must block and zero counts must not: ready=%v items=%v blockers=%+v", inv.ReadyToCancel, items, inv.Blockers)
	}
}

func TestExitInventoryFallbackIsTypedNotTextual(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("DATTO_SAAS_NO_AUTO_REFRESH", "1")
	t.Setenv("DATTO_SAAS_PUBLIC_KEY", "pub")
	t.Setenv("DATTO_SAAS_SECRET_KEY", "sec")
	db := seedSaasStore(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"saasCustomerId":1,"saasCustomerName":"Other","domain":"o.example"}]`))
	}))
	t.Setenv("DATTO_SAAS_BASE_URL", srv.URL)
	// The customer error text mentions "connection refused" but is not a transport failure.
	customer := "connection refused" // a customer name argument, not an expected error
	out, err := runSaasCmd(t, "exit-inventory", customer, "--db", db)
	if err == nil {
		t.Fatalf("a customer-resolution error must not fall back: %s", out)
	}
	srv.Close()
	// A real transport failure (closed server) does fall back to the store.
	out, err = runSaasCmd(t, "exit-inventory", "123456", "--db", db)
	if err != nil || !strings.Contains(out, `"source": "local"`) {
		t.Fatalf("an unreachable API should fall back to local: out=%s err=%v", out, err)
	}
}

func TestBulkChangeStdinPreviewIsCanonical(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("DATTO_SAAS_NO_AUTO_REFRESH", "1")
	f := filepath.Join(t.TempDir(), "body.json")
	if err := os.WriteFile(f, []byte(`{"seat_type":"User","action_type":"License","ids":"a,b"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	in, err := os.Open(f)
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdin
	os.Stdin = in
	t.Cleanup(func() { os.Stdin = orig; _ = in.Close() })
	// The generated dry-run writes the request preview to the process stderr.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origErr := os.Stderr
	os.Stderr = w
	cmd := RootCmd()
	cmd.SetArgs([]string{"seats", "bulk-change", "1", "S:1", "--stdin", "--dry-run"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	runErr := cmd.Execute()
	os.Stderr = origErr
	_ = w.Close()
	preview, _ := io.ReadAll(r)
	if runErr != nil {
		t.Fatal(runErr)
	}
	if strings.Contains(string(preview), `"a,b"`) || !strings.Contains(string(preview), `"a"`) {
		t.Fatalf("dry-run must preview the canonical array body, got:\n%s", preview)
	}
}

func TestExitInventoryStalledAuthBodyDoesNotFallBack(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("DATTO_SAAS_NO_AUTO_REFRESH", "1")
	t.Setenv("DATTO_SAAS_PUBLIC_KEY", "pub")
	t.Setenv("DATTO_SAAS_SECRET_KEY", "sec")
	db := seedSaasStore(t)
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "64")
		w.WriteHeader(http.StatusUnauthorized)
		w.(http.Flusher).Flush()
		select { // stall the body past the client timeout
		case <-release:
		case <-time.After(5 * time.Second):
		}
	}))
	defer srv.Close()
	defer close(release)
	t.Setenv("DATTO_SAAS_BASE_URL", srv.URL)
	if out, err := runSaasCmd(t, "exit-inventory", "123456", "--db", db, "--timeout", "1s"); err == nil {
		t.Fatalf("a 401 with a stalled body must not fall back to the store: %s", out)
	}
}

func TestSingleUndatedWindowAndUnknownAggregates(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("DATTO_SAAS_NO_AUTO_REFRESH", "1")
	db := seedRows(t,
		[]string{`{"saasCustomerId":13,"saasCustomerName":"One","domain":"one.example","externalSubscriptionId":"S:13","seatsUsed":1,"backupStats":{"activeServicesCount":1,"activeServicesWithRecentBackupCount":1}}`},
		[]string{`{"parent_id":"13","mainId":"u@one.example","seatType":"User","seatState":"Active","remoteId":"r"}`},
		[]string{
			`{"parent_id":"13","customerId":13,"usedBytes":100,"suites":[{"appTypes":[{"appType":"Solo","backupHistory":[{"activeServiceCount":1,"activeServiceWithBackupCount":1,"activeServiceWithPerfectBackupCount":1}]}]}]}`,
		})
	out, err := runSaasCmd(t, "exit-inventory", "13", "--data-source", "local", "--db", db)
	if err != nil {
		t.Fatal(err)
	}
	var inv exitInventory
	_ = json.Unmarshal([]byte(out), &inv)
	blocked := false
	for _, b := range inv.Blockers {
		blocked = blocked || b.Kind == "apps_without_usable_history"
	}
	if inv.ReadyToCancel || !blocked {
		t.Fatalf("a single undated window must block: %+v", inv.Blockers)
	}
	if inv.Storage.UsedBytes == nil || *inv.Storage.UsedBytes != 100 || inv.Storage.UsedBytesStatus != "complete" {
		t.Fatalf("complete storage: %+v", inv.Storage)
	}

	out, err = runSaasCmd(t, "seats", "reconcile", "--db", db)
	if err != nil {
		t.Fatal(err)
	}
	rows := decodeRows(t, out)
	if rows[0]["delta_billable_minus_seats_used"] != nil || strings.Contains(out, "seats_used_mismatch") || rows[0]["delta_active_minus_seats_used"] != 0.0 {
		t.Fatalf("unknown billable must suppress the billable delta only: %s", out)
	}
}

func TestPartialUsedBytesIsNotATotal(t *testing.T) {
	inv := buildExitInventory(saasCustomer{ID: "1"}, nil, []saasAppReport{
		{UsedBytes: flexNum{v: 100, ok: true}}, {},
	}, nil, "included", "local")
	if inv.Storage.UsedBytes != nil || inv.Storage.UsedBytesPartial == nil || *inv.Storage.UsedBytesPartial != 100 ||
		!strings.HasPrefix(inv.Storage.UsedBytesStatus, "incomplete") {
		t.Fatalf("partial usedBytes must not be a total: %+v", inv.Storage)
	}
}

func TestBulkChangeRefusesRedirect(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("DATTO_SAAS_NO_AUTO_REFRESH", "1")
	t.Setenv("DATTO_SAAS_PUBLIC_KEY", "pub")
	t.Setenv("DATTO_SAAS_SECRET_KEY", "sec")
	var puts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			puts++
		}
		if r.URL.Path == "/saas/1/S:1/bulkSeatChange" {
			http.Redirect(w, r, "/saas/2/S:2/bulkSeatChange", http.StatusTemporaryRedirect)
			return
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	t.Setenv("DATTO_SAAS_BASE_URL", srv.URL)
	if _, err := runSaasCmd(t, "seats", "bulk-change", "1", "S:1", "--seat-type", "User", "--action-type", "License", "--ids", "r1"); err == nil {
		t.Fatal("a redirected bulkSeatChange must fail")
	}
	if puts != 1 {
		t.Fatalf("the PUT was sent %d times; a redirect must never resend it", puts)
	}
}

func TestExitInventoryPaginatedReportBlocks(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("DATTO_SAAS_NO_AUTO_REFRESH", "1")
	t.Setenv("DATTO_SAAS_PUBLIC_KEY", "pub")
	t.Setenv("DATTO_SAAS_SECRET_KEY", "sec")
	good := `{"timeWindow":"1","startTime":1,"activeServiceCount":1,"activeServiceWithBackupCount":1,"activeServiceWithPerfectBackupCount":1}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/saas/domains":
			_, _ = w.Write([]byte(`[{"saasCustomerId":5,"saasCustomerName":"P","domain":"p.example","externalSubscriptionId":"S:5","backupStats":{"activeServicesCount":1,"activeServicesWithRecentBackupCount":1}}]`))
		case strings.HasSuffix(r.URL.Path, "/seats"):
			_, _ = w.Write([]byte(`[{"mainId":"u@p.example","seatType":"User","seatState":"Active","billable":true,"remoteId":"r"}]`))
		case strings.HasSuffix(r.URL.Path, "/applications"):
			_, _ = w.Write([]byte(`{"pagination":{"page":1,"perPage":1,"totalPages":2,"count":2},"items":[{"customerId":5,"usedBytes":10,"suites":[{"appTypes":[{"appType":"A","backupHistory":[` + good + `]}]}]}]}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()
	t.Setenv("DATTO_SAAS_BASE_URL", srv.URL)
	out, err := runSaasCmd(t, "exit-inventory", "5", "--data-source", "live")
	if err != nil {
		t.Fatal(err)
	}
	var inv exitInventory
	_ = json.Unmarshal([]byte(out), &inv)
	paged := false
	for _, b := range inv.Blockers {
		paged = paged || b.Kind == "application_report_paginated"
	}
	if inv.ReadyToCancel || !paged || inv.Storage.UsedBytes != nil {
		t.Fatalf("an unread second page must block and leave storage incomplete: %+v", inv)
	}
}

func TestEmptyReportAndNegativeCountsAreMissingEvidence(t *testing.T) {
	good := `{"timeWindow":"1","startTime":1,"activeServiceCount":1,"activeServiceWithBackupCount":1,"activeServiceWithPerfectBackupCount":1}`
	var healthy, empty saasAppReport
	if err := json.Unmarshal([]byte(`{"customerId":5,"usedBytes":1,"suites":[{"appTypes":[{"appType":"A","backupHistory":[`+good+`]}]}]}`), &healthy); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"customerId":5,"usedBytes":1,"suites":[]}`), &empty); err != nil {
		t.Fatal(err)
	}
	var d saasDomain
	_ = json.Unmarshal([]byte(`{"saasCustomerId":5,"domain":"n.example","externalSubscriptionId":"S:5","backupStats":{"activeServicesCount":-1,"activeServicesWithRecentBackupCount":-1}}`), &d)
	cust := groupCustomers([]saasDomain{d})[0]
	seat := saasSeat{MainID: "u", SeatType: "User", SeatState: "Active", RemoteID: "r", BillableJS: json.RawMessage(`true`)}
	inv := buildExitInventory(cust, []saasSeat{seat}, []saasAppReport{healthy, empty}, nil, "included", "local")
	kinds := map[string]bool{}
	for _, b := range inv.Blockers {
		kinds[b.Kind] = true
	}
	if inv.ReadyToCancel || !kinds["no_backup_history"] || !kinds["backup_stats_missing"] {
		t.Fatalf("an empty report and negative counts must block: %+v", inv.Blockers)
	}
	var nan flexNum
	if err := json.Unmarshal([]byte(`"NaN"`), &nan); err != nil || nan.ok {
		t.Fatalf("NaN must not decode as a count: %+v", nan)
	}
}

func TestExitInventoryPartialMirrorBlocks(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("DATTO_SAAS_NO_AUTO_REFRESH", "1")
	dbPath := seedSaasStore(t)
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SaveSyncProgress("seats", "", 1); err != nil { // an interrupted seats sync
		t.Fatal(err)
	}
	_ = db.Close()
	out, err := runSaasCmd(t, "exit-inventory", "123456", "--data-source", "local", "--db", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	var inv exitInventory
	_ = json.Unmarshal([]byte(out), &inv)
	var partial *invBlocker
	for i := range inv.Blockers {
		if inv.Blockers[i].Kind == "local_mirror_incomplete" {
			partial = &inv.Blockers[i]
		}
	}
	if inv.ReadyToCancel || partial == nil || len(partial.Items) != 1 || partial.Items[0] != "seats" || inv.Storage.UsedBytes != nil {
		t.Fatalf("an interrupted sync must block and demote storage: %+v", inv)
	}
}

func TestBulkChangeNeverReplays(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("DATTO_SAAS_NO_AUTO_REFRESH", "1")
	t.Setenv("DATTO_SAAS_PUBLIC_KEY", "pub")
	t.Setenv("DATTO_SAAS_SECRET_KEY", "sec")
	var puts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			puts++
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	t.Setenv("DATTO_SAAS_BASE_URL", srv.URL)
	args := []string{"seats", "bulk-change", "1", "S:1", "--seat-type", "User", "--action-type", "License", "--ids", "r1"}
	if _, err := runSaasCmd(t, args...); err == nil || puts != 1 {
		t.Fatalf("a 500 must fail without a replay: puts=%d err=%v", puts, err)
	}
	// A configured idempotency header would make the client replay; it is refused.
	cfg := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfg, []byte("[headers]\n\"Idempotency-Key\" = \"k1\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATTO_SAAS_CONFIG", cfg)
	puts = 0
	if _, err := runSaasCmd(t, args...); err == nil || puts != 0 {
		t.Fatalf("an Idempotency-Key header must refuse the send: puts=%d err=%v", puts, err)
	}
}

func TestBackupHealthAllEmptyReportsIsIncomplete(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("DATTO_SAAS_NO_AUTO_REFRESH", "1")
	db := seedRows(t,
		[]string{`{"saasCustomerId":14,"saasCustomerName":"Empty","domain":"e.example","backupStats":{"activeServicesCount":10,"activeServicesWithRecentBackupCount":10}}`},
		nil, []string{`{"parent_id":"14","customerId":14,"suites":[]}`})
	out, err := runSaasCmd(t, "backup-health", "--db", db, "--below", "95")
	if err != nil {
		t.Fatal(err)
	}
	if rows := decodeRows(t, out); len(rows) != 1 || !strings.Contains(out, "app_counts_incomplete") {
		t.Fatalf("all-empty reports must be flagged and kept under --below: %s", out)
	}
}

func interruptSync(t *testing.T, dbPath string, resources ...string) {
	t.Helper()
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, r := range resources {
		if err := db.SaveSyncProgress(r, "", 1); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLocalCommandsHonorIncompleteMirror(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("DATTO_SAAS_NO_AUTO_REFRESH", "1")
	db := seedSaasStore(t)
	interruptSync(t, db, "domains", "applications")

	out, err := runSaasCmd(t, "seats", "bulk-plan", "--db", db, "--action", "License", "--customer", "fabrikam.com")
	if err != nil {
		t.Fatal(err)
	}
	var plan bulkPlan
	_ = json.Unmarshal([]byte(out), &plan)
	if len(plan.Batches) != 0 || len(plan.MirrorIncomplete) == 0 {
		t.Fatalf("no batches may be planned from an incomplete mirror: %s", out)
	}

	out, err = runSaasCmd(t, "seats", "reconcile", "--db", db)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "seats_used_mismatch") || !strings.Contains(out, "local_mirror_incomplete") || strings.Contains(out, `"delta_billable_minus_seats_used": -`) {
		t.Fatalf("reconcile must claim no mismatch from an incomplete mirror: %s", out)
	}

	out, err = runSaasCmd(t, "backup-health", "--db", db, "--below", "1")
	if err != nil {
		t.Fatal(err)
	}
	rows := decodeRows(t, out)
	if len(rows) != 2 {
		t.Fatalf("incomplete clients must stay under --below: %s", out)
	}
	for _, r := range rows {
		if r["coverage_pct"] != nil || r["perfect_backup_pct"] != nil {
			t.Fatalf("metrics from an incomplete mirror must be null: %v", r)
		}
	}
}

func TestBulkSeatGuardDropsGetBody(t *testing.T) {
	next := &getBodyRT{}
	guard := bulkSeatGuard{next: next}
	req, _ := http.NewRequest(http.MethodPut, "https://api.datto.com/v1/saas/1/S:1/bulkSeatChange",
		strings.NewReader(`{"seat_type":"User","action_type":"License","ids":["a"]}`))
	if req.GetBody == nil {
		t.Fatal("precondition: NewRequest sets GetBody for a strings.Reader body")
	}
	if _, err := guard.RoundTrip(req); err != nil {
		t.Fatal(err)
	}
	if !next.called || next.hadGetBody {
		t.Fatalf("the forwarded PUT must not carry GetBody (HTTP/2 could replay it): %+v", next)
	}
}

type getBodyRT struct{ called, hadGetBody bool }

func (g *getBodyRT) RoundTrip(req *http.Request) (*http.Response, error) {
	g.called, g.hadGetBody = true, req.GetBody != nil
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("[]")), Header: http.Header{}}, nil
}

func TestLocalReadsAreOneSnapshot(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("DATTO_SAAS_NO_AUTO_REFRESH", "1")
	db := seedSaasStore(t)
	done := make(chan error, 1)
	snapshotHook = func() {
		go func() {
			w, err := store.Open(db)
			if err != nil {
				done <- err
				return
			}
			defer w.Close()
			// A concurrent sync adds a second subscription for Fabrikam.
			done <- w.UpsertDomains(json.RawMessage(`{"saasCustomerId":222333,"saasCustomerName":"Fabrikam Inc","domain":"fabrikam.org","productType":"GoogleApps","externalSubscriptionId":"Classic:GoogleApps:999"}`))
		}()
		time.Sleep(300 * time.Millisecond) // let the write reach the lock while the snapshot is open
	}
	t.Cleanup(func() { snapshotHook = nil })
	out, err := runSaasCmd(t, "seats", "bulk-plan", "--db", db, "--action", "License", "--customer", "222333")
	if err != nil {
		t.Fatal(err)
	}
	if werr := <-done; werr != nil {
		t.Fatalf("concurrent write failed: %v", werr)
	}
	var plan bulkPlan
	_ = json.Unmarshal([]byte(out), &plan)
	// One snapshot: either the old single subscription (a batch) or, had the
	// write landed first, a block. Never old domains mixed with new seats; here
	// the write must still be waiting, so the plan reflects the old snapshot.
	if len(plan.Batches) != 1 || plan.Batches[0].Subscription != "Classic:GoogleApps:777888" || len(plan.Blocked) != 0 {
		t.Fatalf("plan must come from one consistent snapshot: %s", out)
	}
}

func runSaasCmdStderr(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := RootCmd()
	cmd.SetArgs(append(args, "--json"))
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

func TestIncompleteWarningIndependentOfStaleHint(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("DATTO_SAAS_NO_AUTO_REFRESH", "1")
	dbPath := seedSaasStore(t)
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SaveSyncStateAt("domains", "", 3, time.Now().Add(-48*time.Hour)); err != nil { // stale
		t.Fatal(err)
	}
	if err := db.SaveSyncProgress("seats", "", 1); err != nil { // interrupted
		t.Fatal(err)
	}
	_ = db.Close()
	_, stderr, err := runSaasCmdStderr(t, "seats", "roster", "--db", dbPath, "--max-age", "1h")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr, "the last sync of seats did not finish") {
		t.Fatalf("the incomplete-mirror warning must print alongside a stale hint; stderr:\n%s", stderr)
	}
}

func TestBackupHealthPartialAppsSuppressesAppMetrics(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("DATTO_SAAS_NO_AUTO_REFRESH", "1")
	db := seedSaasStore(t)
	interruptSync(t, db, "applications")
	out, err := runSaasCmd(t, "backup-health", "--db", db, "--customer", "123456")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range decodeRows(t, out) {
		if r["coverage_pct"] != nil || r["mirror_incomplete"] != true || r["counts_complete"] == true {
			t.Fatalf("per-app rows from an incomplete applications sync must be marked and nulled: %v", r)
		}
	}
	out, err = runSaasCmd(t, "backup-health", "--db", db)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range decodeRows(t, out) {
		if r["worst_app_coverage_pct"] != nil || r["short_window_streak"] != 0.0 {
			t.Fatalf("fleet app-derived metrics must be suppressed: %v", r)
		}
	}
}

func TestExitInventoryRedirectLoopDoesNotFallBack(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("DATTO_SAAS_NO_AUTO_REFRESH", "1")
	t.Setenv("DATTO_SAAS_PUBLIC_KEY", "pub")
	t.Setenv("DATTO_SAAS_SECRET_KEY", "sec")
	db := seedSaasStore(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, r.URL.Path, http.StatusFound) // redirect loop
	}))
	defer srv.Close()
	t.Setenv("DATTO_SAAS_BASE_URL", srv.URL)
	if out, err := runSaasCmd(t, "exit-inventory", "123456", "--db", db); err == nil {
		t.Fatalf("a redirect-policy failure is not an unreachable API and must not fall back: %s", out)
	}
}

func TestBulkChangePreviewRefusesIdempotencyHeaders(t *testing.T) {
	for _, name := range []string{"Idempotency-Key", "X-Idempotency-Key"} {
		t.Run(name, func(t *testing.T) {
			testenv.Isolate(t)
			t.Setenv("DATTO_SAAS_NO_AUTO_REFRESH", "1")
			cfg := filepath.Join(t.TempDir(), "config.toml")
			if err := os.WriteFile(cfg, []byte("[headers]\n\""+name+"\" = \"k1\"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("DATTO_SAAS_CONFIG", cfg)
			if _, err := runSaasCmd(t, "seats", "bulk-change", "1", "S:1", "--seat-type", "User", "--action-type", "License", "--ids", "r1", "--dry-run"); err == nil {
				t.Fatalf("--dry-run must refuse a configured %s header like the real send", name)
			}
		})
	}
}
