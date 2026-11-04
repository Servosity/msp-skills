// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"autotask-pp-cli/internal/cliutil/testenv"
	"autotask-pp-cli/internal/store"
)

// fakeAutotask answers POST /<Entity>/query the way Autotask does: the body's
// filter conditions (gte/eq on numeric fields) select the records, MaxRecords
// caps the page, and the envelope is {"items":[...],"pageDetails":{...}}. It
// checks the three auth headers on every call.
type fakeAutotask struct {
	mu       sync.Mutex
	entities map[string][]map[string]any // "tickets" -> records, keyed by the path segment lowercased
	bodies   []string
}

func (f *fakeAutotask) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("ApiIntegrationCode") != "e2eCode-5b7d9f1a" || r.Header.Get("UserName") != "e2e-user@example.com" || r.Header.Get("Secret") != "e2eSecret-3c5e7a9b" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	seg := strings.Split(strings.TrimPrefix(r.URL.Path, "/atservicesrest/V1.0/"), "/")
	if r.Method != http.MethodPost || len(seg) != 2 || seg[1] != "query" {
		_, _ = w.Write([]byte(`{"items":[],"pageDetails":{"count":0,"requestCount":500}}`))
		return
	}
	raw, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	f.bodies = append(f.bodies, seg[0]+" "+string(raw))
	recs := f.entities[strings.ToLower(seg[0])]
	f.mu.Unlock()
	var body struct {
		Filter     []map[string]any `json:"filter"`
		MaxRecords int              `json:"MaxRecords"`
	}
	_ = json.Unmarshal(raw, &body)
	if body.MaxRecords <= 0 || body.MaxRecords > 500 {
		body.MaxRecords = 500
	}
	out := []map[string]any{} // Autotask answers an empty page with "items":[], never null
	for _, rec := range recs {
		if matchesAll(rec, body.Filter) {
			out = append(out, rec)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i]["id"].(float64) < out[j]["id"].(float64) })
	if len(out) > body.MaxRecords {
		out = out[:body.MaxRecords]
	}
	b, _ := json.Marshal(map[string]any{"items": out, "pageDetails": map[string]any{"count": len(out), "requestCount": body.MaxRecords, "prevPageUrl": nil, "nextPageUrl": nil}})
	_, _ = w.Write(b)
}

func matchesAll(rec map[string]any, conds []map[string]any) bool {
	for _, c := range conds {
		field, _ := c["field"].(string)
		want, _ := c["value"].(float64)
		got, _ := rec[field].(float64)
		switch c["op"] {
		case "gte":
			if got < want {
				return false
			}
		case "gt":
			if got <= want {
				return false
			}
		case "eq":
			if got != want {
				return false
			}
		}
	}
	return true
}

func decodeRecords(t *testing.T, items ...string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, it := range items {
		var m map[string]any
		if err := json.Unmarshal([]byte(it), &m); err != nil {
			t.Fatal(err)
		}
		out = append(out, m)
	}
	return out
}

func e2eEnv(t *testing.T) (*fakeAutotask, func(args ...string) (string, error), string) {
	t.Helper()
	testenv.Isolate(t)
	created := time.Now().Add(-48 * time.Hour).UTC().Format("2006-01-02T15:04:05Z")
	f := &fakeAutotask{entities: map[string][]map[string]any{
		"tickets": decodeRecords(t,
			`{"id":101,"title":"VPN down","status":1,"priority":1,"companyID":10,"createDate":"`+created+`"}`,
			`{"id":102,"title":"Printer jam","status":1,"priority":3,"companyID":10,"createDate":"`+created+`"}`,
			`{"id":103,"title":"Closed issue","status":5,"priority":2,"companyID":10,"createDate":"`+created+`"}`),
		"companies":          decodeRecords(t, `{"id":10,"companyName":"Contoso"}`),
		"contacts":           decodeRecords(t, `{"id":201,"companyID":10,"firstName":"Ada"}`, `{"id":202,"companyID":10,"firstName":"Bo"}`),
		"contracts":          decodeRecords(t, `{"id":301,"companyID":10,"contractName":"Managed"}`),
		"configurationitems": decodeRecords(t, `{"id":401,"companyID":10,"referenceTitle":"FW-01"}`),
		"opportunities":      decodeRecords(t, `{"id":501,"companyID":10,"title":"Renewal"}`),
		"resources":          decodeRecords(t, `{"id":29682885,"firstName":"Ada","lastName":"Tech","userName":"ada.tech"}`, `{"id":29682886,"firstName":"Bo","lastName":"Desk","userName":"bo.desk"}`),
	}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	t.Setenv("AUTOTASK_BASE_URL", srv.URL+"/atservicesrest/V1.0")
	t.Setenv("AUTOTASK_API_INTEGRATION_CODE", "e2eCode-5b7d9f1a")
	t.Setenv("AUTOTASK_PSA_USER_NAME", "e2e-user@example.com")
	t.Setenv("AUTOTASK_PSA_SECRET", "e2eSecret-3c5e7a9b")
	db := filepath.Join(t.TempDir(), "data.db")
	return f, func(args ...string) (string, error) {
		t.Helper()
		cmd := RootCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(append(args, "--db", db))
		done := make(chan error, 1)
		go func() { done <- cmd.Execute() }()
		select {
		case err := <-done:
			if err != nil {
				out.WriteString(err.Error())
			}
			return out.String(), err
		case <-time.After(90 * time.Second):
			t.Fatalf("%s timed out", strings.Join(args, " "))
			return "", nil
		}
	}, db
}

// The novels read what `sync` really stores: the POST /query id walk keeps
// every record, the novels answer from the synced resource names, and the
// three auth headers reach the API on every call.
func TestSyncThenNovelsKeepEveryRecord(t *testing.T) {
	_, run, _ := e2eEnv(t)
	if out, err := run("sync", "--resources", "tickets,companies,contacts,contracts,configuration-items,opportunities"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	out, err := run("triage", "--json")
	if err != nil {
		t.Fatalf("triage: %v\n%s", err, out)
	}
	for _, want := range []string{"VPN down", "Printer jam"} {
		if !strings.Contains(out, want) {
			t.Errorf("triage after sync is missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Closed issue") {
		t.Errorf("triage lists a completed ticket:\n%s", out)
	}
	out, err = run("company-360", "10", "--json")
	if err != nil {
		t.Fatalf("company-360: %v\n%s", err, out)
	}
	var c360 struct {
		CompanyName string         `json:"companyName"`
		Summary     map[string]int `json:"summary"`
	}
	if err := json.Unmarshal([]byte(out[strings.Index(out, "{"):]), &c360); err != nil {
		t.Fatalf("company-360 output: %v\n%s", err, out)
	}
	want := map[string]int{"tickets": 3, "openTickets": 2, "contacts": 2, "contracts": 1, "configurationItems": 1, "opportunities": 1}
	for k, n := range want {
		if c360.Summary[k] != n {
			t.Errorf("company-360 summary %s = %d, want %d (%+v)", k, c360.Summary[k], n, c360.Summary)
		}
	}
	if c360.CompanyName != "Contoso" {
		t.Errorf("company-360 companyName = %q", c360.CompanyName)
	}
}

// incomplete-snapshot-read-as-complete: a filtered walk, a walk that did not
// finish, and a resource never synced while others were are refused by every
// novel, and a later unfiltered sync clears the refusal.
func TestNovelsRefuseAPartialStore(t *testing.T) {
	_, run, _ := e2eEnv(t)
	if out, err := run("sync", "--resources", "tickets", "--param", `filter=[{"op":"eq","field":"status","value":5}]`); err != nil {
		t.Fatalf("filtered sync: %v\n%s", err, out)
	}
	out, err := run("triage", "--json")
	if err == nil || !strings.Contains(out, "sent filters (filter)") {
		t.Fatalf("triage after a filtered sync: want the filtered-walk refusal, got err %v:\n%s", err, out)
	}
	if out, err := run("sync", "--resources", "tickets"); err != nil {
		t.Fatalf("unfiltered sync: %v\n%s", err, out)
	}
	if out, err := run("triage", "--json"); err != nil || !strings.Contains(out, "VPN down") {
		t.Fatalf("triage after the unfiltered sync: err %v:\n%s", err, out)
	}
	// account-brief also reads opportunities, contracts and contract blocks,
	// none of them synced: refused, never read as "none".
	out, err = run("account-brief", "10", "--json")
	if err == nil || !strings.Contains(out, "was never synced") {
		t.Fatalf("account-brief with unsynced opportunities: want the never-synced refusal, got err %v:\n%s", err, out)
	}
	// company-360 reads contacts, contracts, config items and opportunities.
	out, err = run("company-360", "10", "--json")
	if err == nil || !strings.Contains(out, "was never synced") {
		t.Fatalf("company-360 with unsynced contacts: want the never-synced refusal, got err %v:\n%s", err, out)
	}
}

// psa-resources-table-shadow-rename, restated for 4.34.2: the Autotask entity
// named Resources shares its name with the generic store table. 4.22 emitted
// a typed `resources` table beside it and every sync insert crashed; 4.34.2
// emits none, so the rows land in the generic store. Sync it through the real
// CLI and read the rows back.
func TestSyncStoresTheResourcesEntity(t *testing.T) {
	_, run, db := e2eEnv(t)
	if out, err := run("sync", "--resources", "resources"); err != nil {
		t.Fatalf("sync resources: %v\n%s", err, out)
	}
	s, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	var n int
	if err := s.DB().QueryRow(`SELECT COUNT(*) FROM resources WHERE resource_type = 'resources'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("stored %d Resources rows, want 2", n)
	}
}

// A sync whose last attempt did not complete leaves the store refused.
func TestNovelsRefuseAnUnfinishedSync(t *testing.T) {
	testenv.Isolate(t)
	db := filepath.Join(t.TempDir(), "data.db")
	s, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Upsert("tickets", "101", []byte(`{"id":101,"title":"VPN down","status":1}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().Exec(`INSERT OR REPLACE INTO sync_state (resource_type, last_attempt_complete, last_synced_at) VALUES ('tickets', 0, NULL)`); err != nil {
		t.Fatal(err)
	}
	if err := requireFinishedSync(s, "tickets"); err == nil || !strings.Contains(err.Error(), "did not finish") {
		t.Fatalf("unfinished sync: want the refusal, got %v", err)
	}
	if _, err := s.DB().Exec(`UPDATE sync_state SET last_attempt_complete = 1, last_synced_at = '2026-10-28T00:00:00Z' WHERE resource_type = 'tickets'`); err != nil {
		t.Fatal(err)
	}
	if err := requireFinishedSync(s, "tickets"); err == nil || !strings.Contains(err.Error(), "record no filters") {
		t.Fatalf("a checkpoint no sync of this build wrote: want the refusal, got %v", err)
	}
	// This build's sync creates the filtered-walk record table (markFilteredWalks).
	if _, err := s.DB().Exec(`CREATE TABLE ` + syncFilteredTable + ` (resource_type TEXT PRIMARY KEY, params TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if err := requireFinishedSync(s, "tickets"); err != nil {
		t.Fatalf("finished sync refused: %v", err)
	}
	if err := requireFinishedSync(s, "contacts"); err == nil || !strings.Contains(err.Error(), "never synced") {
		t.Fatalf("contacts never synced beside tickets: want the refusal, got %v", err)
	}
	_ = s.Close()
}

// Codex r1 class incomplete-snapshot-read-as-complete (resumed cursor): a
// filtered walk stopped by --max-pages leaves its id cursor in sync_state, and
// the press keeps filters out of the checkpoint scope, so the next unfiltered
// sync resumed after the filtered walk's last id and finished "complete"
// without the open ticket below it. Neither walk may resume the other's cursor.
func TestUnfilteredSyncDoesNotResumeAFilteredWalk(t *testing.T) {
	f, run, _ := e2eEnv(t)
	recs := decodeRecords(t, `{"id":1,"title":"Open below the cursor","status":1,"priority":1,"companyID":10}`)
	for id := 2; id <= 201; id++ { // two pages of completed tickets (the walk reads 100 at a time)
		recs = append(recs, decodeRecords(t, fmt.Sprintf(`{"id":%d,"title":"Done %d","status":5,"priority":2,"companyID":10}`, id, id))...)
	}
	f.mu.Lock()
	f.entities["tickets"] = recs
	f.mu.Unlock()
	if out, err := run("sync", "--resources", "tickets", "--param", `filter=[{"op":"eq","field":"status","value":5}]`, "--max-pages", "1"); err != nil {
		t.Fatalf("filtered sync: %v\n%s", err, out)
	}
	if out, err := run("sync", "--resources", "tickets"); err != nil {
		t.Fatalf("unfiltered sync: %v\n%s", err, out)
	}
	out, err := run("triage", "--json")
	if err != nil || !strings.Contains(out, "Open below the cursor") {
		t.Fatalf("triage after the unfiltered sync is missing ticket 1 (err %v):\n%s", err, out)
	}
}

// Codex r2, same class: a store whose filtered walk was recorded by a build
// that kept no cursor mark (the round-1 build) still has the filtered walk's
// cursor; the next unfiltered walk must not resume it either. The filtered
// record itself, not only the mark, makes the first unfiltered walk start over.
func TestUnfilteredSyncDoesNotResumeARecordedFilteredWalk(t *testing.T) {
	f, run, db := e2eEnv(t)
	recs := decodeRecords(t, `{"id":1,"title":"Open below the cursor","status":1,"priority":1,"companyID":10}`)
	for id := 2; id <= 201; id++ {
		recs = append(recs, decodeRecords(t, fmt.Sprintf(`{"id":%d,"title":"Done %d","status":5,"priority":2,"companyID":10}`, id, id))...)
	}
	f.mu.Lock()
	f.entities["tickets"] = recs
	f.mu.Unlock()
	if out, err := run("sync", "--resources", "tickets", "--param", `filter=[{"op":"eq","field":"status","value":5}]`, "--max-pages", "1"); err != nil {
		t.Fatalf("filtered sync: %v\n%s", err, out)
	}
	s, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`DROP TABLE IF EXISTS autotask_sync_unfiltered_resume`} {
		if _, err := s.DB().Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()
	if out, err := run("sync", "--resources", "tickets"); err != nil {
		t.Fatalf("unfiltered sync: %v\n%s", err, out)
	}
	out, err := run("triage", "--json")
	if err != nil || !strings.Contains(out, "Open below the cursor") {
		t.Fatalf("triage after the unfiltered sync is missing ticket 1 (err %v):\n%s", err, out)
	}
}

// The other side of the same rule: after its first walk starts over, an
// unfiltered sync run in chunks (--max-pages) resumes its own cursor and
// finishes, so the filtered record clears and the novels answer.
func TestChunkedUnfilteredSyncResumesAfterAFilteredWalk(t *testing.T) {
	f, run, _ := e2eEnv(t)
	recs := decodeRecords(t, `{"id":1,"title":"Open below the cursor","status":1,"priority":1,"companyID":10}`)
	for id := 2; id <= 201; id++ {
		recs = append(recs, decodeRecords(t, fmt.Sprintf(`{"id":%d,"title":"Done %d","status":5,"priority":2,"companyID":10}`, id, id))...)
	}
	f.mu.Lock()
	f.entities["tickets"] = recs
	f.mu.Unlock()
	if out, err := run("sync", "--resources", "tickets", "--param", `filter=[{"op":"eq","field":"status","value":5}]`, "--max-pages", "1"); err != nil {
		t.Fatalf("filtered sync: %v\n%s", err, out)
	}
	for i := 0; i < 3; i++ { // 201 tickets, 100 a page
		if out, err := run("sync", "--resources", "tickets", "--max-pages", "1"); err != nil {
			t.Fatalf("unfiltered chunk %d: %v\n%s", i, err, out)
		}
	}
	out, err := run("triage", "--json")
	if err != nil || !strings.Contains(out, "Open below the cursor") {
		t.Fatalf("triage after three chunks (err %v):\n%s", err, out)
	}
}

// Same class, older store: a build without the filtered-walk record (the
// published 4.24 build) left a filtered walk's checkpoint complete with no
// record. The novels refuse such a store until an unfiltered sync, which
// starts over, has walked the resource.
func TestStoreFromABuildWithoutTheFilterRecordIsNotTrusted(t *testing.T) {
	f, run, db := e2eEnv(t)
	recs := decodeRecords(t, `{"id":1,"title":"Open below the cursor","status":1,"priority":1,"companyID":10}`)
	for id := 2; id <= 201; id++ {
		recs = append(recs, decodeRecords(t, fmt.Sprintf(`{"id":%d,"title":"Done %d","status":5,"priority":2,"companyID":10}`, id, id))...)
	}
	f.mu.Lock()
	f.entities["tickets"] = recs
	f.mu.Unlock()
	if out, err := run("sync", "--resources", "tickets", "--param", `filter=[{"op":"eq","field":"status","value":5}]`); err != nil {
		t.Fatalf("filtered sync: %v\n%s", err, out)
	}
	s, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`DROP TABLE autotask_sync_filtered`, `DROP TABLE autotask_sync_unfiltered_resume`} {
		if _, err := s.DB().Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()
	if out, err := run("triage", "--json"); err == nil {
		t.Fatalf("triage trusted a store an older build filled with a filtered walk:\n%s", out)
	}
	if out, err := run("sync", "--resources", "tickets"); err != nil {
		t.Fatalf("unfiltered sync: %v\n%s", err, out)
	}
	out, err := run("triage", "--json")
	if err != nil || !strings.Contains(out, "Open below the cursor") {
		t.Fatalf("triage after the unfiltered sync is missing ticket 1 (err %v):\n%s", err, out)
	}
}

// recordUnfilteredWalks creates the filtered-walk record table the way this
// build's sync does, so a test that writes sync_state directly stands for an
// unfiltered sync of this build, not a checkpoint an older build left.
func recordUnfilteredWalks(t *testing.T, s *store.Store) {
	t.Helper()
	if _, err := s.DB().Exec(`CREATE TABLE IF NOT EXISTS ` + syncFilteredTable + ` (resource_type TEXT PRIMARY KEY, params TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
}
