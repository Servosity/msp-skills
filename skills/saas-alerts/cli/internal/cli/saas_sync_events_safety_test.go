// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/saas-alerts/handfixes.json
// under the "events-sync-restart-and-filtered" entry.

package cli

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"saas-alerts-pp-cli/internal/cliutil/testenv"
	"saas-alerts-pp-cli/internal/store"
)

func saasEventsSyncServer(t *testing.T, n int, froms *[]string, mu *sync.Mutex) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/reportApi/api/v1/reports/events" {
			w.Write([]byte(`[]`))
			return
		}
		mu.Lock()
		*froms = append(*froms, r.URL.Query().Get("from")+"|"+r.URL.Query().Get("customerId"))
		mu.Unlock()
		var rows []string
		for i := 0; i < n; i++ {
			rows = append(rows, fmt.Sprintf(`{"_id":"ev%d","eventId":"E%d","time":%q,"jointType":"login.failure","alertStatus":"low","customer":{"id":"C1","name":"Acme"},"user":{"id":"u1","name":"a@acme.example"},"product":{"id":"p","name":"Microsoft"}}`,
				i, i, time.Now().UTC().Add(-time.Duration(i)*time.Minute).Format(time.RFC3339)))
		}
		w.Write([]byte("[" + strings.Join(rows, ",") + "]"))
	}))
}

func saasRunSync(t *testing.T, args ...string) string {
	t.Helper()
	cmd := RootCmd()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs(append([]string{"--no-learn", "--rate-limit", "0", "sync"}, args...))
	_ = cmd.Execute()
	return out.String() + errb.String()
}

// TestSaasEventsSyncRestartsFromTop: a saved events offset from an unfinished
// sync is never resumed; new events arrive at the top of a timeSort=desc list.
func TestSaasEventsSyncRestartsFromTop(t *testing.T) {
	testenv.Isolate(t)
	var froms []string
	var mu sync.Mutex
	srv := saasEventsSyncServer(t, 3, &froms, &mu)
	defer srv.Close()
	t.Setenv("SAAS_ALERTS_BASE_URL", srv.URL)
	t.Setenv("SAAS_ALERTS_API_KEY", "test-partner-key")
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	t.Setenv("PRINTING_PRESS_DOGFOOD", "")
	dbPath := filepath.Join(t.TempDir(), "data.db")
	saasRunSync(t, "--resources", "events", "--db", dbPath, "--json") // stores rows, so the generated empty-store reset does not apply
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SaveSyncProgress("events", "200", 200); err != nil {
		t.Fatal(err)
	}
	db.Close()
	mu.Lock()
	froms = nil
	mu.Unlock()
	saasRunSync(t, "--resources", "events", "--db", dbPath, "--json")
	if len(froms) == 0 || strings.HasPrefix(froms[0], "200|") {
		t.Fatalf("events sync resumed the stale offset: requests %v", froms)
	}
}

// TestSaasFilteredEventsSyncIsNotComplete: --param narrows the sync, so it
// must not advance the watermark or record coverage.
func TestSaasFilteredEventsSyncIsNotComplete(t *testing.T) {
	testenv.Isolate(t)
	var froms []string
	var mu sync.Mutex
	srv := saasEventsSyncServer(t, 3, &froms, &mu)
	defer srv.Close()
	t.Setenv("SAAS_ALERTS_BASE_URL", srv.URL)
	t.Setenv("SAAS_ALERTS_API_KEY", "test-partner-key")
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	t.Setenv("PRINTING_PRESS_DOGFOOD", "")
	dbPath := filepath.Join(t.TempDir(), "data.db")
	out := saasRunSync(t, "--resources", "events", "--since", "7d", "--param", "customerId=C1", "--db", dbPath, "--json")
	if !strings.Contains(out, "filtered_sync_not_complete") || !strings.Contains(out, `"warned":1`) {
		t.Fatalf("filtered sync not reported as warned:\n%s", out)
	}
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if n, _ := db.Count("events"); n != 3 {
		t.Fatalf("stored %d events, want 3", n)
	}
	var complete int
	var last *string
	if err := db.DB().QueryRow(`SELECT last_attempt_complete, last_synced_at FROM sync_state WHERE resource_type = 'events'`).Scan(&complete, &last); err != nil {
		t.Fatal(err)
	}
	if complete != 0 || last != nil {
		t.Fatalf("filtered sync recorded complete=%d last_synced_at=%v", complete, last)
	}
	if _, ok, _ := db.EventsCoverage(); ok {
		t.Fatal("filtered sync recorded an events coverage floor")
	}
}

// TestSaasFilteredFullSyncNeverPrunes: a --full rule-triggers sync narrowed to
// one organization must not delete other organizations' stored triggers.
func TestSaasFilteredFullSyncNeverPrunes(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/respondApi/api/v1/auth":
			w.Write([]byte(`{"accessToken":"t"}`))
		case "/respondApi/api/v1/rule-triggers":
			if len(r.URL.Query()["organizationIds[]"]) > 0 {
				w.Write([]byte(`{"ruleTriggers":[{"id":"RT_A","status":"PENDING","triggeredAt":1686299747000,"organization":{"id":"ORG_A"}}],"pagination":{"hasNextPage":false,"cursor":null}}`))
				return
			}
			w.Write([]byte(`{"ruleTriggers":[{"id":"RT_A","status":"PENDING","triggeredAt":1686299747000,"organization":{"id":"ORG_A"}},{"id":"RT_B","status":"PENDING","triggeredAt":1686299747001,"organization":{"id":"ORG_B"}}],"pagination":{"hasNextPage":false,"cursor":null}}`))
		default:
			w.Write([]byte(`[]`))
		}
	}))
	defer srv.Close()
	t.Setenv("SAAS_ALERTS_BASE_URL", srv.URL)
	t.Setenv("SAAS_ALERTS_API_KEY", "test-partner-key")
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	t.Setenv("PRINTING_PRESS_DOGFOOD", "")
	dbPath := filepath.Join(t.TempDir(), "data.db")
	saasRunSync(t, "--resources", "rule-triggers", "--full", "--db", dbPath, "--json")
	out := saasRunSync(t, "--resources", "rule-triggers", "--full", "--resource-param", "rule-triggers:organizationIds[]=ORG_A", "--db", dbPath, "--json")
	if !strings.Contains(out, "filtered_sync_not_complete") {
		t.Fatalf("no filtered-sync warning:\n%s", out)
	}
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if n, _ := db.Count("rule-triggers"); n != 2 {
		t.Fatalf("filtered --full sync left %d triggers, want 2 (ORG_B pruned)", n)
	}
}

// TestSaasIncrementalEventsSyncRereadsLateAllowance: an explicit incremental
// events sync starts saasEventsLateAllowance before the previous watermark, so
// an event pulled after the last sync but timestamped before it is fetched.
func TestSaasIncrementalEventsSyncRereadsLateAllowance(t *testing.T) {
	testenv.Isolate(t)
	var mu sync.Mutex
	var starts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/reportApi/api/v1/reports/events" {
			mu.Lock()
			starts = append(starts, r.URL.Query().Get("start"))
			mu.Unlock()
			w.Write([]byte(fmt.Sprintf(`[{"_id":"ev1","time":%q,"jointType":"login.failure","customer":{"id":"C1"},"user":{"id":"u1"},"product":{"id":"p","name":"Microsoft"}}]`, time.Now().UTC().Format(time.RFC3339))))
			return
		}
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	t.Setenv("SAAS_ALERTS_BASE_URL", srv.URL)
	t.Setenv("SAAS_ALERTS_API_KEY", "test-partner-key")
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	t.Setenv("PRINTING_PRESS_DOGFOOD", "")
	dbPath := filepath.Join(t.TempDir(), "data.db")
	saasRunSync(t, "--resources", "events", "--db", dbPath, "--json")
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, watermark, _, err := db.GetSyncState("events")
	db.Close()
	if err != nil || watermark.IsZero() {
		t.Fatalf("no watermark after the first sync: %v", err)
	}
	mu.Lock()
	starts = nil
	mu.Unlock()
	saasRunSync(t, "--resources", "events", "--db", dbPath, "--json")
	if len(starts) == 0 {
		t.Fatal("second sync sent no events request")
	}
	got, err := time.Parse(time.RFC3339, starts[0])
	if err != nil {
		t.Fatalf("start %q: %v", starts[0], err)
	}
	if want := watermark.Add(-saasEventsLateAllowance); !got.Equal(want.Truncate(time.Second)) {
		t.Fatalf("incremental start = %v, want %v (watermark %v minus the late allowance)", got, want, watermark)
	}
}

func saasUsersServer(t *testing.T, users map[string]*string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/reportApi/api/v1/customers":
			w.Write([]byte(`[{"id":"C1","name":"Acme"},{"id":"C2","name":"Birch"}]`))
		case "/reportApi/api/v1/reports/users":
			if body, ok := users[r.URL.Query().Get("customerId")]; ok {
				w.Write([]byte(*body))
				return
			}
			w.Write([]byte(`[]`))
		default:
			w.Write([]byte(`[]`))
		}
	}))
}

func saasUserParents(t *testing.T, dbPath string) map[string]string {
	t.Helper()
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.DB().Query(`SELECT json_extract(data, '$.id'), json_extract(data, '$.parent_id') FROM resources WHERE resource_type = 'users'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, parent string
		if err := rows.Scan(&id, &parent); err != nil {
			t.Fatal(err)
		}
		out[id+"@"+parent] = parent
	}
	return out
}

// TestSaasUsersFullSyncPrunesDeletedUsers: the per-customer users prune
// matches the injected parent_id, so a user removed upstream is deleted.
func TestSaasUsersFullSyncPrunesDeletedUsers(t *testing.T) {
	testenv.Isolate(t)
	c1 := `[{"id":"U1","email":"a@acme.example"},{"id":"U2","email":"b@acme.example"}]`
	srv := saasUsersServer(t, map[string]*string{"C1": &c1})
	defer srv.Close()
	t.Setenv("SAAS_ALERTS_BASE_URL", srv.URL)
	t.Setenv("SAAS_ALERTS_API_KEY", "test-partner-key")
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	t.Setenv("PRINTING_PRESS_DOGFOOD", "")
	dbPath := filepath.Join(t.TempDir(), "data.db")
	saasRunSync(t, "--resources", "customers", "--full", "--db", dbPath, "--json")
	if got := saasUserParents(t, dbPath); len(got) != 2 {
		t.Fatalf("first sync stored %v, want U1 and U2", got)
	}
	c1 = `[{"id":"U1","email":"a@acme.example"}]`
	saasRunSync(t, "--resources", "customers", "--full", "--db", dbPath, "--json")
	if got := saasUserParents(t, dbPath); len(got) != 1 {
		t.Fatalf("full sync kept a deleted user: %v", got)
	}
}

// TestSaasParentKeyOverrideRefused: --resource-param users:customerId=C1 must
// not store C1's users under C2.
func TestSaasParentKeyOverrideRefused(t *testing.T) {
	testenv.Isolate(t)
	c1 := `[{"id":"U1","email":"a@acme.example"}]`
	srv := saasUsersServer(t, map[string]*string{"C1": &c1})
	defer srv.Close()
	t.Setenv("SAAS_ALERTS_BASE_URL", srv.URL)
	t.Setenv("SAAS_ALERTS_API_KEY", "test-partner-key")
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	t.Setenv("PRINTING_PRESS_DOGFOOD", "")
	dbPath := filepath.Join(t.TempDir(), "data.db")
	out := saasRunSync(t, "--resources", "customers", "--resource-param", "users:customerId=C1", "--db", dbPath, "--json")
	for k, parent := range saasUserParents(t, dbPath) {
		if parent != "C1" {
			t.Fatalf("stored %s under %s from an overridden customerId:\n%s", k, parent, out)
		}
	}
	if !strings.Contains(out, "refusing to sync users") {
		t.Fatalf("no refusal reported:\n%s", out)
	}
}

// TestSaasFilteredSyncDropsItsCursor: a filtered, page-capped walk must not
// leave a cursor that a later unfiltered sync would resume.
func TestSaasFilteredSyncDropsItsCursor(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/respondApi/api/v1/auth":
			w.Write([]byte(`{"accessToken":"t"}`))
		case "/respondApi/api/v1/rule-triggers":
			w.Write([]byte(`{"ruleTriggers":[{"id":"RT_A","status":"PENDING","triggeredAt":1686299747000}],"pagination":{"hasNextPage":true,"cursor":[1686299747000,"RT_A"]}}`))
		default:
			w.Write([]byte(`[]`))
		}
	}))
	defer srv.Close()
	t.Setenv("SAAS_ALERTS_BASE_URL", srv.URL)
	t.Setenv("SAAS_ALERTS_API_KEY", "test-partner-key")
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	t.Setenv("PRINTING_PRESS_DOGFOOD", "")
	dbPath := filepath.Join(t.TempDir(), "data.db")
	saasRunSync(t, "--resources", "rule-triggers", "--max-pages", "1", "--resource-param", "rule-triggers:organizationIds[]=ORG_A", "--db", dbPath, "--json")
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if cur := db.GetSyncCursor("rule-triggers"); cur != "" {
		t.Fatalf("filtered sync saved cursor %q", cur)
	}
}

// TestSaasLateAllowanceAnchorsToExplicitSync: an automatic refresh that moves
// the watermark must not move the next explicit sync's late-event look-back.
func TestSaasLateAllowanceAnchorsToExplicitSync(t *testing.T) {
	testenv.Isolate(t)
	var mu sync.Mutex
	var starts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/reportApi/api/v1/reports/events" {
			mu.Lock()
			starts = append(starts, r.URL.Query().Get("start"))
			mu.Unlock()
			w.Write([]byte(fmt.Sprintf(`[{"_id":"ev1","time":%q,"jointType":"login.failure","customer":{"id":"C1"},"user":{"id":"u1"},"product":{"id":"p","name":"Microsoft"}}]`, time.Now().UTC().Format(time.RFC3339))))
			return
		}
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	t.Setenv("SAAS_ALERTS_BASE_URL", srv.URL)
	t.Setenv("SAAS_ALERTS_API_KEY", "test-partner-key")
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	t.Setenv("PRINTING_PRESS_DOGFOOD", "")
	dbPath := filepath.Join(t.TempDir(), "data.db")
	saasRunSync(t, "--resources", "events", "--db", dbPath, "--json")
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	explicit, ok := db.EventsExplicitWatermark()
	if !ok {
		t.Fatal("explicit sync recorded no explicit watermark")
	}
	// An automatic refresh later moved the watermark 10 hours forward.
	if err := db.SaveSyncStateAt("events", "", 1, explicit.Add(10*time.Hour)); err != nil {
		t.Fatal(err)
	}
	db.Close()
	mu.Lock()
	starts = nil
	mu.Unlock()
	saasRunSync(t, "--resources", "events", "--db", dbPath, "--json")
	got, err := time.Parse(time.RFC3339, starts[0])
	if err != nil {
		t.Fatal(err)
	}
	if want := explicit.Add(-saasEventsLateAllowance); !got.Equal(want) {
		t.Fatalf("explicit sync started at %v, want %v (last explicit watermark minus the allowance)", got, want)
	}
}

// TestSaasInterruptedFilteredWalkSavesNoCursor: a filtered rule-triggers walk
// that fails on page 2 must not leave the page-1 cursor for a plain sync.
func TestSaasInterruptedFilteredWalkSavesNoCursor(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/respondApi/api/v1/auth":
			w.Write([]byte(`{"accessToken":"t"}`))
		case "/respondApi/api/v1/rule-triggers":
			if len(r.URL.Query()["startAfter"]) > 0 {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"message":"boom"}`))
				return
			}
			w.Write([]byte(`{"ruleTriggers":[{"id":"RT_A","status":"PENDING","triggeredAt":1686299747000}],"pagination":{"hasNextPage":true,"cursor":[1686299747000,"RT_A"]}}`))
		default:
			w.Write([]byte(`[]`))
		}
	}))
	defer srv.Close()
	t.Setenv("SAAS_ALERTS_BASE_URL", srv.URL)
	t.Setenv("SAAS_ALERTS_API_KEY", "test-partner-key")
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	t.Setenv("PRINTING_PRESS_DOGFOOD", "")
	dbPath := filepath.Join(t.TempDir(), "data.db")
	saasRunSync(t, "--resources", "rule-triggers", "--resource-param", "rule-triggers:organizationIds[]=ORG_A", "--db", dbPath, "--json")
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if cur := db.GetSyncCursor("rule-triggers"); cur != "" {
		t.Fatalf("interrupted filtered walk saved cursor %q", cur)
	}
}

// TestSaasScopedResourceNeverWholeTablePruned: a --full sync of a resource that
// requires organizationIds must not delete rows stored for other organizations.
func TestSaasScopedResourceNeverWholeTablePruned(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/account-mailbox-rules") || strings.Contains(r.URL.Path, "mailbox") {
			org := r.URL.Query().Get("organizationIds")
			w.Write([]byte(fmt.Sprintf(`[{"updatedAt":"2026-10-01T00:00:00Z","organizationId":%q,"partnerId":"P1","userName":"u@%s.example","ruleName":"r"}]`, org, strings.ToLower(org))))
			return
		}
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	t.Setenv("SAAS_ALERTS_BASE_URL", srv.URL)
	t.Setenv("SAAS_ALERTS_API_KEY", "test-partner-key")
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	t.Setenv("PRINTING_PRESS_DOGFOOD", "")
	dbPath := filepath.Join(t.TempDir(), "data.db")
	saasRunSync(t, "--resources", "reports-account-mailbox-rules", "--resource-param", "reports-account-mailbox-rules:organizationIds=ORG_A", "--db", dbPath, "--json")
	out := saasRunSync(t, "--resources", "reports-account-mailbox-rules", "--full", "--resource-param", "reports-account-mailbox-rules:organizationIds=ORG_B", "--db", dbPath, "--json")
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if n, _ := db.Count("reports-account-mailbox-rules"); n != 2 {
		t.Fatalf("--full of ORG_B left %d mailbox-rule rows, want 2 (ORG_A pruned):\n%s", n, out)
	}
}

// TestSaasCappedFilteredSyncIsWarned: a filtered walk stopped by --max-pages
// is still reported as a filtered (warned) sync.
func TestSaasCappedFilteredSyncIsWarned(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/respondApi/api/v1/auth":
			w.Write([]byte(`{"accessToken":"t"}`))
		case "/respondApi/api/v1/rule-triggers":
			w.Write([]byte(`{"ruleTriggers":[{"id":"RT_A","status":"PENDING","triggeredAt":1686299747000}],"pagination":{"hasNextPage":true,"cursor":[1686299747000,"RT_A"]}}`))
		default:
			w.Write([]byte(`[]`))
		}
	}))
	defer srv.Close()
	t.Setenv("SAAS_ALERTS_BASE_URL", srv.URL)
	t.Setenv("SAAS_ALERTS_API_KEY", "test-partner-key")
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	t.Setenv("PRINTING_PRESS_DOGFOOD", "")
	out := saasRunSync(t, "--resources", "rule-triggers", "--max-pages", "1", "--resource-param", "rule-triggers:organizationIds[]=ORG_A", "--db", filepath.Join(t.TempDir(), "data.db"), "--json")
	if !strings.Contains(out, "filtered_sync_not_complete") || !strings.Contains(out, `"warned":1`) {
		t.Fatalf("capped filtered sync not reported as warned:\n%s", out)
	}
}
