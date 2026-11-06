// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Codex review round 2 (knowbe4 4.34.2 reprint), classes fixed in this file's
// tests:
//   - missing-input-read-as-zero: a collection-wide absence claim (untrained,
//     never clicked, no enrollment, a zero count) is made only when that
//     collection's own sync completed, never because some row of it is in the
//     store (insights.CollectionSynced).
//   - stale-source-preferred: a value the store holds in two places is read
//     from the one read from the API more recently (insights.newer).
//   - persistence-gate-skipped-under-agentcookie: config.toml is judged
//     whole, every field it would persist and every header value together.
//   - credential-to-other-api-host (instance 1): a value is exempt from the
//     Reporting host's foreign set only as the credential the Reporting
//     client actually sends, never as a value an environment variable
//     overrode.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"knowbe4-pp-cli/internal/cliutil/testenv"
)

// Class missing-input-read-as-zero. The failing input from the review: on a
// fresh store, `training-enrollments get 1` (one cached enrollment, Bob's),
// `sync --resources users,phishing-tests`, then `untrained-clickers`. Alice
// clicked a synced phishing test and has an enrollment that was never fetched,
// documented "status":"Passed"; the cached row made the enrollments read as
// held and Alice was listed as untrained. Every novel that judges against the
// enrollments says they are not synced instead, and so does freshness.
func TestR2OneCachedRowIsNotASyncedCollection(t *testing.T) {
	testenv.Isolate(t)
	fake := newFakeKnowBe4(t)
	// Sarah (the review's Bob) passed enrollment 1425526; William (the
	// review's Alice) clicked both tests and passed enrollment 1425527, which
	// only the list returns.
	fake.bodies["/training/enrollments/1425526"] = `{"enrollment_id":1425526,"content_type":"Training Module","module_name":"2026 Security Awareness","user":{"id":796742,"first_name":"Sarah","last_name":"Thomas","email":"s_thomas@kb4-demo.com"},"campaign_name":"Annual Training","status":"Passed"}`
	fake.bodies["/training/enrollments"] = `[` +
		`{"enrollment_id":1425526,"content_type":"Training Module","module_name":"2026 Security Awareness","user":{"id":796742,"first_name":"Sarah","last_name":"Thomas","email":"s_thomas@kb4-demo.com"},"campaign_name":"Annual Training","status":"Passed"},` +
		`{"enrollment_id":1425527,"content_type":"Training Module","module_name":"2026 Security Awareness","user":{"id":667542,"first_name":"William","last_name":"Marcoux","email":"wmarcoux@kb4-demo.com"},"campaign_name":"Annual Training","status":"Passed"}]`

	mustRunKnowBe4(t, "training-enrollments", "get", "1425526", "--json", "--no-learn")
	mustRunKnowBe4(t, "sync", "--no-learn", "--resources", "users,phishing-tests")

	for _, novel := range [][]string{
		{"untrained-clickers"},
		{"coverage-gaps", "--type", "training"},
	} {
		out, errOut, err := runKnowBe4(t, append(novel, "--json", "--no-learn")...)
		if strings.Contains(out, "wmarcoux@kb4-demo.com") {
			t.Errorf("%v lists William, whose enrollment was never fetched:\n%s", novel, out)
		}
		if err == nil || !strings.Contains(err.Error()+errOut, "not in the local store") {
			t.Errorf("%v with one cached enrollment: err %v, want \"not in the local store\"\n%s%s", novel, err, out, errOut)
		}
	}
	var rows []map[string]any
	out := mustRunKnowBe4(t, "risk-leaderboard", "--json", "--no-learn")
	if err := json.Unmarshal([]byte(out), &rows); err != nil || len(rows) == 0 {
		t.Fatalf("risk-leaderboard: %v\n%s", err, out)
	}
	for _, r := range rows {
		if v, ok := r["open_trainings"]; !ok || v != nil {
			t.Errorf("risk-leaderboard %v open_trainings = %v with one cached enrollment, want null", r["email"], v)
		}
	}
	var rep map[string]any
	out = mustRunKnowBe4(t, "qbr", "--json", "--no-learn")
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("qbr: %v\n%s", err, out)
	}
	for _, k := range []string{"training_completion_pct", "untrained_clicker_count"} {
		if v, ok := rep[k]; !ok || v != nil {
			t.Errorf("qbr %s = %v with one cached enrollment, want null\n%s", k, v, out)
		}
	}
	var fresh struct {
		Commands []struct {
			Command string `json:"command"`
			Ready   bool   `json:"ready"`
		} `json:"commands"`
	}
	out = mustRunKnowBe4(t, "freshness", "--json", "--no-learn")
	if err := json.Unmarshal([]byte(out), &fresh); err != nil {
		t.Fatalf("freshness: %v\n%s", err, out)
	}
	for _, c := range fresh.Commands {
		if c.Command == "untrained-clickers" && c.Ready {
			t.Errorf("freshness says untrained-clickers is ready with one cached enrollment:\n%s", out)
		}
	}

	// One cached user is not the users collection either.
	testenv.Isolate(t)
	fake = newFakeKnowBe4(t)
	fake.bodies["/users/667542"] = `{"id":667542,"first_name":"William","last_name":"Marcoux","email":"wmarcoux@kb4-demo.com","current_risk_score":61.5,"status":"active"}`
	mustRunKnowBe4(t, "users", "get", "667542", "--json", "--no-learn")
	for _, novel := range []string{"risk-leaderboard", "coverage-gaps"} {
		out, errOut, err := runKnowBe4(t, novel, "--json", "--no-learn")
		if err == nil || !strings.Contains(err.Error()+errOut, "not in the local store") {
			t.Errorf("%s with one cached user: err %v, want \"not in the local store\"\n%s%s", novel, err, out, errOut)
		}
	}

	// A recipient walk where one phishing test failed is not the recipients
	// collection: who never clicked is unknown.
	testenv.Isolate(t)
	fake = newFakeKnowBe4(t)
	fake.fail["/phishing/security_tests/16143/recipients"] = true
	mustRunKnowBe4(t, "sync", "--no-learn")
	for _, novel := range []string{"repeat-clickers", "untrained-clickers", "report-rate"} {
		out, errOut, err := runKnowBe4(t, novel, "--json", "--no-learn")
		if err == nil || !strings.Contains(err.Error()+errOut, "not in the local store") {
			t.Errorf("%s after a partial recipient walk: err %v, want \"not in the local store\"\n%s%s", novel, err, out, errOut)
		}
	}

	// Control: after a complete sync every one of them answers.
	testenv.Isolate(t)
	newFakeKnowBe4(t)
	mustRunKnowBe4(t, "sync", "--no-learn")
	for _, novel := range []string{"untrained-clickers", "coverage-gaps", "risk-leaderboard", "repeat-clickers", "report-rate"} {
		mustRunKnowBe4(t, novel, "--json", "--no-learn")
	}
}

// Class missing-input-read-as-zero, the completion record itself: `workflow
// archive --full` wrote a completed sync record (sync_state
// last_attempt_complete = 1, zero rows) for every resource before walking any
// of them, so an archive that stopped at its first resource left users and
// training enrollments recorded as synced and empty, and coverage-gaps
// answered "no gaps". Clearing the cursor for a full walk leaves completion
// unset; only a walk that finished sets it.
func TestR2FullArchiveRecordsNoSyncItDidNotFinish(t *testing.T) {
	testenv.Isolate(t)
	// The first resource the archive walks (groups) answers after the
	// archive's timeout, which stops the archive there.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/groups") {
			select {
			case <-r.Context().Done():
			case <-time.After(5 * time.Second):
			}
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[]`)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("KNOWBE4_BASE_URL", srv.URL+"/v1")
	t.Setenv("KNOWBE4_API_KEY", "archiveUnitKey-2b7d9e41")
	out, errOut, err := runKnowBe4(t, "workflow", "archive", "--full", "--timeout", "1s", "--no-learn")
	if err == nil {
		t.Fatalf("an archive stopped by its timeout at its first resource succeeded:\n%s%s", out, errOut)
	}
	for _, novel := range [][]string{{"coverage-gaps", "--type", "training"}, {"risk-leaderboard"}} {
		out, errOut, err := runKnowBe4(t, append(novel, "--json", "--no-learn")...)
		if err == nil || !strings.Contains(err.Error()+errOut, "not in the local store") {
			t.Errorf("%v after an archive that never walked users: err %v, want \"not in the local store\"\n%s%s", novel, err, out, errOut)
		}
	}
}

// Class stale-source-preferred. The failing input from the review: `sync`
// while GET /account answers current_risk_score 65; the score changes to 90;
// `account info --json` (default auto mode) stores 90; `qbr --json` still
// reported 65 from the older sync snapshot. The newer reading wins whichever
// source holds it, and a copy the sync takes of an older reading is not a
// newer one.
func TestR2QBRReadsTheNewerAccountReading(t *testing.T) {
	testenv.Isolate(t)
	fake := newFakeKnowBe4(t)
	account := func(score string) {
		fake.mu.Lock()
		fake.bodies["/account"] = `{"name":"KB4-Demo","type":"paid","subscription_level":"Diamond","number_of_seats":25,"current_risk_score":` + score + `}`
		fake.mu.Unlock()
	}
	qbrScore := func(when string) any {
		t.Helper()
		var rep map[string]any
		out := mustRunKnowBe4(t, "qbr", "--json", "--no-learn")
		if err := json.Unmarshal([]byte(out), &rep); err != nil {
			t.Fatalf("qbr %s: %v\n%s", when, err, out)
		}
		return rep["account_risk_score"]
	}
	// Each step reads the API at least a second after the one before, so the
	// recorded times (whole seconds in the store) order them.
	step := func() { time.Sleep(1100 * time.Millisecond) }

	account("65")
	mustRunKnowBe4(t, "sync", "--no-learn")
	step()
	account("90")
	mustRunKnowBe4(t, "account", "info", "--json", "--no-learn")
	if got := qbrScore("after account info"); got != 90.0 {
		t.Errorf("account_risk_score = %v after `account info` read 90 (the sync read 65 earlier), want 90", got)
	}

	// The reverse: a later sync reads 70, newer than the account info read.
	step()
	account("70")
	mustRunKnowBe4(t, "sync", "--no-learn")
	if got := qbrScore("after a later sync"); got != 70.0 {
		t.Errorf("account_risk_score = %v after a sync read 70 (account info read 90 earlier), want 70", got)
	}

	// A sync whose own GET /account fails copies the stored account row
	// (90, read before the sync of 70) into its snapshot: that copy is the
	// older reading, not a newer one, so the QBR stays at 70.
	step()
	fake.mu.Lock()
	fake.fail["/account"] = true
	fake.mu.Unlock()
	mustRunKnowBe4(t, "sync", "--no-learn")
	if got := qbrScore("after a sync that could not read the account"); got != 70.0 {
		t.Errorf("account_risk_score = %v after a sync re-copied the older account info reading, want 70 (the newest reading)", got)
	}
}

// Class persistence-gate-skipped-under-agentcookie, the review's command: a
// marked 0600 config.toml with [headers] X-Tail="ujmikolp0987654321";
// `printf '%s\n' 'ManagedToken-new' | KNOWBE4_API_KEY=<the 36-character test value>
// KNOWBE4_REGION=qazwsxedcrfvtgbyhn knowbe4-cli --config <it> auth set-token`
// wrote the key's prefix into template_vars.region beside its suffix in
// headers.X-Tail.
func TestR2AuthSetTokenUnderAgentcookieWritesNoSplitKey(t *testing.T) {
	const prefix, suffix = "qazwsxedcrfvtgbyhn", "ujmikolp0987654321"
	testenv.Isolate(t)
	dir := filepath.Join(t.TempDir(), "kb4")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("[headers]\nX-Tail = \""+suffix+"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".agentcookie-managed"), []byte("managed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KNOWBE4_API_KEY", prefix+suffix)
	t.Setenv("KNOWBE4_REGION", prefix)
	out, err := runKnowBe4In(t, "ManagedToken-new\n", "--config", path, "auth", "set-token")
	got, _ := os.ReadFile(path)
	if strings.Contains(string(got), prefix) && strings.Contains(string(got), suffix) {
		t.Errorf("auth set-token (err %v) wrote the API key split across two fields of config.toml:\n%s\n%s", err, got, out)
	}
}

// Class credential-to-other-api-host, instance 1. The failing input from the
// review: a 0600 config file holding api_key="EventSecret-8d31" and
// `KNOWBE4_API_KEY=ReportingSecret-7c92 KNOWBE4_USER_EVENT_API_KEY=EventSecret-8d31
// knowbe4-cli --config <it> users list --status EventSecret-8d31`: the
// overridden file value (the environment key is the one sent) counted as a
// Reporting credential, which removed the Event key from the foreign set, and
// the Event key reached the Reporting host in the query.
func TestR2EventKeyIsForeignUnlessItIsTheSentReportingKey(t *testing.T) {
	const reportKey = "ReportingSecret-7c92f1ab"
	const eventKey = "EventSecret-8d31e6c4"
	testenv.Isolate(t)
	report := &recordingAPI{}
	rpSrv := httptest.NewServer(report.handler(`[]`))
	t.Cleanup(rpSrv.Close)
	t.Setenv("KNOWBE4_BASE_URL", rpSrv.URL+"/v1")
	t.Setenv("KNOWBE4_API_KEY", reportKey)
	t.Setenv("KNOWBE4_USER_EVENT_API_KEY", eventKey)
	path := filepath.Join(t.TempDir(), "kb4.toml")
	if err := os.WriteFile(path, []byte("api_key = \""+eventKey+"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Control: a plain list reaches the Reporting host.
	if out, err := runKnowBe4In(t, "", "--config", path, "users", "list", "--json"); err != nil || report.count() != 1 {
		t.Fatalf("control users list: err %v, %d requests\n%s", err, report.count(), out)
	}
	if strings.Contains(report.all(), eventKey) {
		t.Fatalf("the control request carried the User Event key:\n%s", report.all())
	}
	before := report.count()
	_, err := runKnowBe4In(t, "", "--config", path, "users", "list", "--status", eventKey, "--json")
	if err == nil {
		t.Errorf("users list carrying the User Event key succeeded")
	}
	if report.count() != before || strings.Contains(report.all(), eventKey) {
		t.Errorf("the User Event key reached the Reporting host (an overridden config value exempted it):\n%s", report.all())
	}

	// The exemption that stays: an operator who set both keys to one value.
	// The Reporting client sends that value as its own key, so it is not
	// foreign to the Reporting host.
	t.Setenv("KNOWBE4_USER_EVENT_API_KEY", reportKey)
	before = report.count()
	if out, err := runKnowBe4In(t, "", "--config", path, "users", "list", "--json"); err != nil || report.count() != before+1 {
		t.Errorf("both keys one value: users list refused (err %v, %d requests)\n%s", err, report.count()-before, out)
	}
}
