// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Codex review round 1 (knowbe4 4.34.2 reprint), classes fixed in this file's
// tests:
//   - credential-to-other-api-host: neither KnowBe4 key reaches the other
//     API's host in any part of a request (URL, header, body), in any form,
//     whole or split across fields, from any of its sources;
//   - non-text-sql-binding-unheld: a number the process holds as a credential
//     is never kept by the local store, whatever column type receives it;
//   - missing-input-read-as-zero: the QBR reads the account risk score from
//     the documented GET /v1/account (sync fetches it), and every QBR figure
//     whose input the store does not hold reads as unavailable (null), never 0.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"knowbe4-pp-cli/internal/cliutil/testenv"

	_ "modernc.org/sqlite"
)

type recordingAPI struct {
	mu   sync.Mutex
	reqs []string
}

func (r *recordingAPI) handler(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		b, _ := io.ReadAll(req.Body)
		r.mu.Lock()
		r.reqs = append(r.reqs, req.Method+" "+req.URL.String()+" "+fmt.Sprint(req.Header)+" "+string(b))
		r.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, body)
	}
}

func (r *recordingAPI) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.reqs)
}

func (r *recordingAPI) all() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Join(r.reqs, "\n")
}

func runKnowBe4In(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	cmd := RootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

// Class credential-to-other-api-host. The failing input from the review:
// KNOWBE4_API_KEY=ReportingSecret-7c92 KNOWBE4_USER_EVENT_API_KEY=EventSecret-8d31
// events create ... --description ReportingSecret-7c92 sent the Reporting key
// to the User Event host in the POST body. The connector's guard refuses the
// key whole, from the environment and from the credential store (auth
// set-token), and the reverse, the event key in a Reporting API query. A
// piece of the key or an encoding of it is refused by the credential kit's
// outbound guard: TestEventsRefuseAPieceOrEncodingOfTheReportingKey.
func TestNeitherKeyReachesTheOtherAPIsHost(t *testing.T) {
	const reportKey = "ReportingSecret-7c92f1ab3e"
	const storedKey = "StoredReportSecret-5e1d0c77"
	const eventKey = "EventSecret-8d31e6c40b"
	testenv.Isolate(t)
	events, report := &recordingAPI{}, &recordingAPI{}
	evSrv := httptest.NewServer(events.handler(`{"data":{"id":"513f46ac-c3d7-4682-ad0d-0c149c0728a2"},"meta":{}}`))
	t.Cleanup(evSrv.Close)
	rpSrv := httptest.NewServer(report.handler(`[]`))
	t.Cleanup(rpSrv.Close)
	t.Setenv("KNOWBE4_USER_EVENT_BASE_URL", evSrv.URL)
	t.Setenv("KNOWBE4_BASE_URL", rpSrv.URL+"/v1")
	t.Setenv("KNOWBE4_USER_EVENT_API_KEY", eventKey)
	t.Setenv("KNOWBE4_API_KEY", reportKey)

	create := func(extra ...string) []string {
		return append([]string{"events", "create", "--target-user", "alice@kb4-demo.com", "--event-type", "Training"}, extra...)
	}
	// The path works: a plain event reaches the events host, a plain list the Reporting host.
	if out, err := runKnowBe4In(t, "", create("--description", "plain text")...); err != nil || events.count() != 1 {
		t.Fatalf("control events create: err %v, %d requests\n%s", err, events.count(), out)
	}
	if out, err := runKnowBe4In(t, "", "users", "list", "--json"); err != nil || report.count() != 1 {
		t.Fatalf("control users list: err %v, %d requests\n%s", err, report.count(), out)
	}

	sent := events.count()
	for name, args := range map[string][]string{
		"raw":    create("--description", reportKey),
		"source": create("--source", "SOC "+reportKey),
	} {
		_, err := runKnowBe4In(t, "", args...)
		if err == nil {
			t.Errorf("%s: events create carrying the Reporting key succeeded", name)
		}
		if events.count() != sent {
			t.Errorf("%s: the request reached the User Event host:\n%s", name, events.all())
			sent = events.count()
		}
	}

	before := report.count()
	if _, err := runKnowBe4In(t, "", "users", "list", "--status", eventKey, "--json"); err == nil {
		t.Errorf("users list carrying the User Event key succeeded")
	}
	if report.count() != before || strings.Contains(report.all(), eventKey) {
		t.Errorf("the User Event key reached the Reporting host:\n%s", report.all())
	}

	// The Reporting key from the credential store, not the environment.
	t.Setenv("KNOWBE4_API_KEY", "")
	if out, err := runKnowBe4In(t, storedKey+"\n", "auth", "set-token"); err != nil {
		t.Fatalf("auth set-token: %v\n%s", err, out)
	}
	if _, err := runKnowBe4In(t, "", create("--description", storedKey)...); err == nil {
		t.Errorf("events create carrying the stored Reporting key succeeded")
	}
	if events.count() != sent {
		t.Errorf("the stored Reporting key reached the User Event host:\n%s", events.all())
	}
	for _, k := range []string{reportKey, storedKey} {
		if strings.Contains(events.all(), k) {
			t.Errorf("the User Event host received the Reporting key %s", k)
		}
	}
}

// Class missing-input-read-as-zero. The failing input from the review: a
// tenant whose documented GET /v1/account answers current_risk_score 65; after
// `sync` the QBR reported account_risk_score 0 (sync never fetched the
// account, and an empty table read as 0). Every QBR figure whose input the
// store does not hold reads as unavailable.
func TestQBRReadsTheDocumentedAccountRiskScore(t *testing.T) {
	testenv.Isolate(t)
	fake := newFakeKnowBe4(t)
	fake.bodies["/account"] = `{"name":"KB4-Demo","type":"paid","domains":["kb4-demo.com"],` +
		`"admins":[{"id":974278,"first_name":"Grace","last_name":"O'Malley","email":"grace.o@kb4-demo.com"}],` +
		`"subscription_level":"Diamond","subscription_end_date":"2027-03-16","number_of_seats":25,"current_risk_score":65.4}`
	db := filepath.Join(t.TempDir(), "data.db")
	if _, errOut, err := runKnowBe4(t, "sync", "--db", db, "--no-learn"); err != nil {
		t.Fatalf("sync: %v\n%s", err, errOut)
	}
	var rep map[string]any
	// The fixture's clicks are dated (2026-08-02, 2026-09-20); a window that
	// reaches both keeps the repeat clicker in it whatever today's date is
	// (the default 90d window lost the August click on 2026-11-01).
	out := mustRunKnowBe4(t, "qbr", "--db", db, "--json", "--no-learn", "--since", "100y")
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("qbr: %v\n%s", err, out)
	}
	if rep["account_risk_score"] != 65.4 {
		t.Errorf("account_risk_score = %v after a sync of GET /v1/account (current_risk_score 65.4), want 65.4\n%s", rep["account_risk_score"], out)
	}
	if rep["training_completion_pct"] != 100.0 || rep["repeat_clicker_count"] != 1.0 {
		t.Errorf("synced figures: training_completion_pct %v, repeat_clicker_count %v\n%s", rep["training_completion_pct"], rep["repeat_clicker_count"], out)
	}

	// A store that holds none of the inputs: nothing reads as a measured 0.
	empty := filepath.Join(t.TempDir(), "empty.db")
	mustRunKnowBe4(t, "sync", "--db", empty, "--no-learn", "--resources", "policies")
	out = mustRunKnowBe4(t, "qbr", "--db", empty, "--json", "--no-learn")
	rep = nil
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("qbr: %v\n%s", err, out)
	}
	for _, k := range []string{"account_risk_score", "training_completion_pct", "repeat_clicker_count", "untrained_clicker_count"} {
		if v, ok := rep[k]; !ok || v != nil {
			t.Errorf("%s = %v (present %v) on a store without its input, want null\n%s", k, v, ok, out)
		}
	}
	md := mustRunKnowBe4(t, "qbr", "--db", empty, "--format", "md", "--no-learn")
	if strings.Contains(md, "Account risk score: **0.0**") || strings.Contains(md, "Training completion: **0.0%**") {
		t.Errorf("the markdown QBR prints unavailable figures as 0:\n%s", md)
	}
}

// Class missing-input-read-as-zero, the whole class: every novel whose answer
// needs an input the store does not hold says so instead of answering "none"
// (an empty list) or 0. The failing input: a store after `sync --resources
// users` (no phishing tests, recipients or enrollments synced, one risk
// snapshot); repeat-clickers answered [] ("nobody clicked twice") and
// risk-leaderboard gave every user clicked_psts 0 and open_trainings 0.
func TestNoNovelReadsAMissingInputAsNone(t *testing.T) {
	testenv.Isolate(t)
	newFakeKnowBe4(t)
	db := filepath.Join(t.TempDir(), "users-only.db")
	mustRunKnowBe4(t, "sync", "--db", db, "--no-learn", "--resources", "users")
	for _, novel := range []string{"repeat-clickers", "untrained-clickers", "coverage-gaps", "report-rate",
		"phish-prone-trend", "risk-drift", "group-risk-contribution"} {
		out, errOut, err := runKnowBe4(t, novel, "--db", db, "--json", "--no-learn")
		if err == nil || !strings.Contains(err.Error()+errOut, "not in the local store") {
			t.Errorf("%s on a store without its input: err %v, want \"not in the local store\"\n%s%s", novel, err, out, errOut)
		}
	}
	out := mustRunKnowBe4(t, "risk-leaderboard", "--db", db, "--json", "--no-learn")
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil || len(rows) == 0 {
		t.Fatalf("risk-leaderboard: %v\n%s", err, out)
	}
	for _, r := range rows {
		for _, k := range []string{"clicked_psts", "reported_psts", "open_trainings"} {
			if v, ok := r[k]; !ok || v != nil {
				t.Errorf("risk-leaderboard %v %s = %v without its input synced, want null", r["email"], k, v)
			}
		}
	}

	// Control: after a full sync the same answers are figures.
	full := filepath.Join(t.TempDir(), "full.db")
	mustRunKnowBe4(t, "sync", "--db", full, "--no-learn")
	out = mustRunKnowBe4(t, "risk-leaderboard", "--db", full, "--json", "--no-learn")
	rows = nil
	if err := json.Unmarshal([]byte(out), &rows); err != nil || len(rows) == 0 || rows[0]["clicked_psts"] == nil || rows[0]["open_trainings"] == nil {
		t.Fatalf("risk-leaderboard after a full sync: %v\n%s", err, out)
	}
	mustRunKnowBe4(t, "repeat-clickers", "--db", full, "--json", "--no-learn")
	mustRunKnowBe4(t, "coverage-gaps", "--db", full, "--json", "--no-learn")
}
