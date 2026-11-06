// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// knowbe4 handfix sync-populates-insights-tables: the store-backed novels read
// what a real `sync` writes. In the 4.24 library and the published tree no
// code called insights.SyncExtras, so pst_recipients and risk_snapshots stayed
// empty after every sync and the recipient-based novels answered "nobody"
// (no repeat clickers, no untrained clickers, no reporters). This test syncs
// from a fake KnowBe4 Reporting API with the documented shapes (bare JSON
// arrays, page/per_page paging; developer.knowbe4.com Reporting API examples)
// and runs each novel on the result.

import (
	"bytes"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"knowbe4-pp-cli/internal/cliutil/testenv"

	_ "modernc.org/sqlite"
)

// fakeKnowBe4 serves the documented Reporting API list shapes on page 1 (every
// later page answers []), answers 500 for the paths in fail, records the paths
// it was asked.
type fakeKnowBe4 struct {
	mu     sync.Mutex
	bodies map[string]string
	fail   map[string]bool
	seen   []string
}

func newFakeKnowBe4(t *testing.T) *fakeKnowBe4 {
	f := &fakeKnowBe4{bodies: map[string]string{}, fail: map[string]bool{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		p := strings.TrimPrefix(r.URL.Path, "/v1")
		f.mu.Lock()
		defer f.mu.Unlock()
		f.seen = append(f.seen, p)
		if f.fail[p] {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"message":"internal error"}`)
			return
		}
		if pg := r.URL.Query().Get("page"); pg != "" && pg != "1" {
			fmt.Fprint(w, `[]`)
			return
		}
		if b, ok := f.bodies[p]; ok {
			fmt.Fprint(w, b)
			return
		}
		fmt.Fprint(w, `[]`)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("KNOWBE4_BASE_URL", srv.URL+"/v1")
	t.Setenv("KNOWBE4_API_KEY", "syncNovelsUnitKey-3f8a2c71")
	f.bodies["/users"] = `[` +
		`{"id":667542,"first_name":"William","last_name":"Marcoux","email":"wmarcoux@kb4-demo.com","phish_prone_percentage":14.2,"groups":[3264],"current_risk_score":61.5,"joined_on":"2019-04-02T15:02:38.000Z","status":"active","department":"Sales"},` +
		`{"id":796742,"first_name":"Sarah","last_name":"Thomas","email":"s_thomas@kb4-demo.com","phish_prone_percentage":2.1,"groups":[3264],"current_risk_score":12.0,"joined_on":"2019-04-02T15:02:38.000Z","status":"active","department":"Sales"},` +
		`{"id":801003,"first_name":"Nora","last_name":"Lane","email":"nlane@kb4-demo.com","phish_prone_percentage":0,"groups":[3265],"current_risk_score":30.0,"joined_on":"2026-10-01T09:00:00.000Z","status":"active","department":"Ops"}]`
	f.bodies["/groups"] = `[{"id":3264,"name":"Sales","group_type":"console_group","member_count":2,"current_risk_score":45.6,"status":"active"},` +
		`{"id":3265,"name":"Ops","group_type":"console_group","member_count":1,"current_risk_score":30.0,"status":"active"}]`
	f.bodies["/phishing/security_tests"] = `[` +
		`{"campaign_id":3423,"pst_id":16142,"status":"Closed","name":"Q2 Test","groups":[{"group_id":3264,"name":"Sales"}],"phish_prone_percentage":0.5,"started_at":"2026-08-02T15:02:38.000Z","duration":1},` +
		`{"campaign_id":3423,"pst_id":16143,"status":"Closed","name":"Q3 Test","groups":[{"group_id":3264,"name":"Sales"}],"phish_prone_percentage":0.25,"started_at":"2026-09-20T15:02:38.000Z","duration":1}]`
	recipient := func(rid, pst, uid int, email, clicked, reported string) string {
		return fmt.Sprintf(`{"recipient_id":%d,"pst_id":%d,"user":{"id":%d,"first_name":"x","last_name":"y","email":%q},"scheduled_at":"2026-09-01T10:00:00.000Z","delivered_at":"2026-09-01T10:01:00.000Z","opened_at":null,"clicked_at":%s,"replied_at":null,"attachment_opened_at":null,"macro_enabled_at":null,"data_entered_at":null,"reported_at":%s,"bounced_at":null,"ip_location":"St.Petersburg, FL","browser":"Chrome","os":"MacOSX"}`,
			rid, pst, uid, email, clicked, reported)
	}
	f.bodies["/phishing/security_tests/16142/recipients"] = "[" +
		recipient(3077742, 16142, 667542, "wmarcoux@kb4-demo.com", `"2026-08-02T16:00:00.000Z"`, "null") + "," +
		recipient(3077743, 16142, 796742, "s_thomas@kb4-demo.com", "null", `"2026-08-02T16:05:00.000Z"`) + "]"
	f.bodies["/phishing/security_tests/16143/recipients"] = "[" +
		recipient(3077842, 16143, 667542, "wmarcoux@kb4-demo.com", `"2026-09-20T16:00:00.000Z"`, "null") + "," +
		recipient(3077843, 16143, 796742, "s_thomas@kb4-demo.com", "null", `"2026-09-20T16:02:00.000Z"`) + "]"
	f.bodies["/training/enrollments"] = `[{"enrollment_id":1425526,"content_type":"Training Module","module_name":"2026 Security Awareness","user":{"id":796742,"first_name":"Sarah","last_name":"Thomas","email":"s_thomas@kb4-demo.com"},"campaign_name":"Annual Training","enrollment_date":"2026-07-02T15:02:38.000Z","start_date":"2026-07-03T15:02:38.000Z","completion_date":"2026-07-04T15:02:38.000Z","status":"Passed","time_spent":2340,"policy_acknowledged":false}]`
	f.bodies["/training/campaigns"] = `[{"campaign_id":4261,"name":"Annual Training","groups":[{"group_id":3264,"name":"Sales"}],"status":"Completed","start_date":"2026-07-01T00:00:00.000Z","end_date":"2026-07-31T00:00:00.000Z"}]`
	f.bodies["/phishing/campaigns"] = `[{"campaign_id":3423,"name":"Quarterly PST","groups":[{"group_id":3264,"name":"Sales"}],"last_phish_prone_percentage":0.25,"last_run":"2026-09-20T15:02:38.000Z","status":"Closed","hidden":false,"frequency":"Quarterly"}]`
	return f
}

func (f *fakeKnowBe4) paths() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.seen...)
}

func runKnowBe4(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := RootCmd()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errb.String(), err
}

func mustRunKnowBe4(t *testing.T, args ...string) string {
	t.Helper()
	out, errOut, err := runKnowBe4(t, args...)
	if err != nil {
		t.Fatalf("%v: %v\n%s%s", args, err, out, errOut)
	}
	return out
}

func countRows(t *testing.T, db, table string) int {
	t.Helper()
	h, err := sql.Open("sqlite", db)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	var n int
	if err := h.QueryRow(`SELECT COUNT(*) FROM "` + table + `"`).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func TestNovelsReadWhatSyncWrites(t *testing.T) {
	testenv.Isolate(t)
	fake := newFakeKnowBe4(t)
	db := filepath.Join(t.TempDir(), "data.db")
	_, errOut, err := runKnowBe4(t, "sync", "--db", db, "--no-learn")
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, errOut)
	}
	if !strings.Contains(errOut, "insights: 4 recipient rows from 2 phishing tests") {
		t.Errorf("sync did not report the insights walk:\n%s", errOut)
	}
	walked := 0
	for _, p := range fake.paths() {
		if strings.HasSuffix(p, "/recipients") {
			walked++
		}
	}
	if walked != 2 {
		t.Errorf("sync walked %d recipient lists, want 2: %v", walked, fake.paths())
	}
	if n := countRows(t, db, "pst_recipients"); n != 4 {
		t.Errorf("pst_recipients has %d rows after sync, want 4", n)
	}
	if n := countRows(t, db, "risk_snapshots"); n < 5 {
		t.Errorf("risk_snapshots has %d rows after sync, want one per user and group (5)", n)
	}

	for _, c := range []struct {
		args []string
		want []string
		not  []string
	}{
		{[]string{"repeat-clickers"}, []string{"wmarcoux@kb4-demo.com"}, []string{"s_thomas@kb4-demo.com"}},
		{[]string{"untrained-clickers"}, []string{"wmarcoux@kb4-demo.com"}, []string{"s_thomas@kb4-demo.com"}},
		{[]string{"report-rate"}, []string{"wmarcoux@kb4-demo.com", "s_thomas@kb4-demo.com"}, nil},
		{[]string{"risk-leaderboard"}, []string{"wmarcoux@kb4-demo.com"}, nil},
		{[]string{"phish-prone-trend", "--group", "Sales"}, []string{"16142", "16143"}, nil},
		{[]string{"coverage-gaps"}, []string{"nlane@kb4-demo.com"}, nil},
		{[]string{"qbr"}, []string{"wmarcoux@kb4-demo.com"}, nil},
		{[]string{"freshness"}, []string{"pst_recipients", "risk_snapshots"}, nil},
	} {
		out := mustRunKnowBe4(t, append(c.args, "--db", db, "--json", "--no-learn")...)
		for _, w := range c.want {
			if !strings.Contains(out, w) {
				t.Errorf("%v after a real sync: missing %q in\n%s", c.args, w, out)
			}
		}
		for _, w := range c.not {
			if strings.Contains(out, w) {
				t.Errorf("%v after a real sync: unexpected %q in\n%s", c.args, w, out)
			}
		}
	}
}

// A walk where every phishing test's recipient fetch failed is an error, never
// "no clickers"; a partial walk says how many tests failed.
func TestSyncFailsWhenEveryRecipientFetchFails(t *testing.T) {
	testenv.Isolate(t)
	fake := newFakeKnowBe4(t)
	fake.fail["/phishing/security_tests/16142/recipients"] = true
	db := filepath.Join(t.TempDir(), "data.db")
	_, errOut, err := runKnowBe4(t, "sync", "--db", db, "--no-learn")
	if err != nil {
		t.Fatalf("partial walk: %v\n%s", err, errOut)
	}
	if !strings.Contains(errOut, "1 tests failed") {
		t.Errorf("a partial recipient walk did not say so:\n%s", errOut)
	}
	fake.mu.Lock()
	fake.fail["/phishing/security_tests/16143/recipients"] = true
	fake.mu.Unlock()
	_, errOut, err = runKnowBe4(t, "sync", "--db", filepath.Join(t.TempDir(), "data.db"), "--no-learn")
	if err == nil || !strings.Contains(err.Error(), "all 2 phishing-test recipient fetches failed") {
		t.Fatalf("every recipient fetch failed: want an error, got %v\n%s", err, errOut)
	}
}

// A sync limited to resources the insights walk does not read sends no
// recipient request; --dry-run sends none either.
func TestSyncWalksRecipientsOnlyWhenItsInputsSync(t *testing.T) {
	testenv.Isolate(t)
	fake := newFakeKnowBe4(t)
	db := filepath.Join(t.TempDir(), "data.db")
	mustRunKnowBe4(t, "sync", "--db", db, "--no-learn", "--resources", "policies")
	mustRunKnowBe4(t, "sync", "--db", db, "--no-learn", "--dry-run")
	for _, p := range fake.paths() {
		if strings.HasSuffix(p, "/recipients") {
			t.Fatalf("a policies-only or --dry-run sync walked recipients: %v", fake.paths())
		}
	}
}
