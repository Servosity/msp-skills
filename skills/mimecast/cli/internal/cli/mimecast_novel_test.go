// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written behavior tests for the Mimecast workflow commands.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"mimecast-pp-cli/internal/cliutil/testenv"
)

func TestClassifyHold(t *testing.T) {
	cases := []struct {
		reason, policy, verdict string
		score                   int
		want                    string
	}{
		{"High-Confidence Impersonation Protection", "", "", -1, riskNeverRelease},
		{"Attachment Sandbox", "", "", -1, riskNeverRelease},
		{"Default Spam Scanning Definition", "", "", 40, riskLikelyJunk},
		{"Spam", "", "", 5, riskReview},
		{"Graymail", "", "spam", -1, riskLikelyJunk},
		{"Content Examination", "Finance DLP", "", -1, riskPolicyReview},
		{"Something new", "", "", -1, riskReview},
	}
	for _, c := range cases {
		if got, _ := classifyHold(c.reason, c.policy, c.verdict, c.score); got != c.want {
			t.Errorf("classifyHold(%q,%q,%q,%d) = %s, want %s", c.reason, c.policy, c.verdict, c.score, got, c.want)
		}
	}
	if q := shellQuote(`a'b|c@x`); q != `'a'"'"'b|c@x'` {
		t.Errorf("shellQuote = %s", q)
	}
}

func TestParseSinceAndAddrMatches(t *testing.T) {
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	for in, want := range map[string]time.Time{
		"24h":                      now.Add(-24 * time.Hour),
		"7d":                       now.AddDate(0, 0, -7),
		"2w":                       now.AddDate(0, 0, -14),
		"2026-10-01T00:00:00+0000": time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
	} {
		got, err := parseSince(in, now)
		if err != nil || !got.Equal(want) {
			t.Errorf("parseSince(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	if _, err := parseSince("7x", now); err == nil {
		t.Error("parseSince(7x) should fail")
	}
	if !addrMatches("Spam@Bad.Example", "spam@bad.example") || !addrMatches("a@mail.bad.example", "bad.example") ||
		addrMatches("a@notbad.example", "bad.example") || addrMatches("other@bad.example", "spam@bad.example") {
		t.Error("addrMatches mismatch")
	}
	if mcTime(now) != "2026-10-14T12:00:00+0000" {
		t.Errorf("mcTime = %s", mcTime(now))
	}
}

func TestBuildTriageGroupsAndNewOnly(t *testing.T) {
	score := 40
	held := []heldMessage{
		{ID: "a", HeldReason: "Impersonation Protection", Subject: "Wire transfer"},
		{ID: "b", HeldReason: "Default Spam Scanning Definition", SpamScore: &score},
		{ID: "c", HeldReason: "Default Spam Scanning Definition", SpamScore: &score},
	}
	v := buildTriage(held, "reason", map[string]bool{"b": true}, false)
	if len(v.Groups) != 2 || v.Groups[0].RiskClass != riskNeverRelease || v.Groups[1].Count != 2 || v.Groups[1].NewCount != 1 {
		t.Fatalf("unexpected groups: %+v", v.Groups)
	}
	v = buildTriage(held, "reason", map[string]bool{"a": true, "b": true, "c": true}, true)
	if len(v.Groups) != 0 {
		t.Fatalf("newOnly with everything seen should be empty, got %+v", v.Groups)
	}
	ids := make([]string, 30)
	for i := range ids {
		ids[i] = string(rune('A' + i))
	}
	if b := batches(ids, 25); len(b) != 2 || len(b[0]) != 25 || len(b[1]) != 5 {
		t.Fatalf("batches wrong: %d", len(b))
	}
}

func TestSuggestSenders(t *testing.T) {
	rows := []cachedRelease{}
	for i := 0; i < 3; i++ {
		rows = append(rows,
			cachedRelease{Status: "released", Sender: "news@vendor.example", Operator: "admin@example.com", Recipients: []string{"alice@example.com"}},
			cachedRelease{Status: "rejected", Sender: "spam@bad.example", Operator: "admin@example.com", Recipients: []string{"bob@example.com"}},
			cachedRelease{Status: "rejected", Sender: "expired@x.example", Operator: "", Recipients: []string{"bob@example.com"}}, // housekeeping
		)
	}
	rows = append(rows, cachedRelease{Status: "rejected", Sender: "mixed@vendor.example", Operator: "a@example.com", Recipients: []string{"c@example.com"}},
		cachedRelease{Status: "released", Sender: "mixed@vendor.example", Operator: "a@example.com", Recipients: []string{"c@example.com"}})
	permit, block := suggestSenders(rows, 3)
	if len(permit) != 1 || permit[0].Sender != "news@vendor.example" || len(block) != 1 || block[0].Sender != "spam@bad.example" {
		t.Fatalf("permit=%+v block=%+v", permit, block)
	}
	if p, b := suggestSenders(rows, 4); len(p)+len(b) != 0 {
		t.Fatalf("min-holds 4 should suggest nothing, got %v %v", p, b)
	}
	risky := []cachedRelease{}
	for i := 0; i < 3; i++ {
		risky = append(risky, cachedRelease{Status: "released", Sender: "ceo@evil.example", Operator: "a@example.com", Recipients: []string{"alice@example.com"}, Risky: i == 0})
	}
	if p, _ := suggestSenders(risky, 3); len(p) != 0 {
		t.Fatalf("a pair with a risky release must never be a permit candidate: %+v", p)
	}
}

func TestJudgeMailflow(t *testing.T) {
	now := time.Now()
	v := mailflowView{Queues: []queueReading{{Name: "inbound_queue", Size: 10}}, LastInbound: mcTime(now.Add(-5 * time.Minute))}
	judgeMailflow(&v, 200, 30*time.Minute, now)
	if v.Verdict != "OK" {
		t.Fatalf("want OK, got %s %v", v.Verdict, v.Reasons)
	}
	v = mailflowView{Queues: []queueReading{{Name: "delivery", Size: 150}}}
	judgeMailflow(&v, 200, 30*time.Minute, now)
	if v.Verdict != "WARN" {
		t.Fatalf("want WARN, got %s", v.Verdict)
	}
	v = mailflowView{Queues: []queueReading{{Name: "delivery", Size: 500}}, StuckMessages: 2}
	judgeMailflow(&v, 200, 30*time.Minute, now)
	if v.Verdict != "FAIL" || len(v.Reasons) != 2 {
		t.Fatalf("want FAIL with 2 reasons, got %s %v", v.Verdict, v.Reasons)
	}
	groups, total := groupStuck([]processingMessage{{ID: "1", Attempts: 5, RemoteIP: "10.0.0.1", Info: "421"}, {ID: "2", Attempts: 1}, {ID: "3", Attempts: 4, RemoteIP: "10.0.0.1", Info: "421"}}, 3)
	if total != 2 || len(groups) != 1 || groups[0].MaxAttempts != 5 {
		t.Fatalf("groupStuck = %+v %d", groups, total)
	}
}

func TestClickMatchesAndRewrite(t *testing.T) {
	cases := []struct {
		clicked, target string
		byHost, want    bool
	}{
		{"https://phish.example/login?x=1", "https://phish.example/login", false, true},
		{"https://phish.example/login/step2", "https://phish.example/login", false, true},
		{"https://phish.example/other", "https://phish.example/login", false, false},
		{"https://phish.example/loginx", "https://phish.example/login", false, false},
		{"https://phish.example/Login?token=ABC", "https://phish.example/login?token=abc", false, false},
		{"https://phish.example.evil/x", "phish.example", false, false},
		{"https://phish.example.evil/x", "phish.example", true, false},
		{"http://www.phish.example/other", "phish.example", true, true},
		{"https://login.phish.example/x", "phish.example", true, true},
		{"https://PHISH.example/a", "https://phish.example/a", false, true},
	}
	for _, c := range cases {
		if got := clickMatches(c.clicked, c.target, c.byHost); got != c.want {
			t.Errorf("clickMatches(%q, %q, host=%v) = %v, want %v", c.clicked, c.target, c.byHost, got, c.want)
		}
	}
	if !isMimecastRewrite("https://protect-us.mimecast.com/s/AbC") || isMimecastRewrite("https://mimecast.com/s/x") {
		t.Fatal("isMimecastRewrite mismatch")
	}
}

func TestReleaseRiskFlagsAndLedgerMatch(t *testing.T) {
	f := false
	l := releaseLog{Status: "released", HeldReason: "Impersonation Protection", FromEnv: mcAddr{EmailAddress: "ceo@evil.example"}, Subject: "Pay now",
		Released: "2026-10-14T10:00:00+0000", SpamProcessingDetail: &spamDetail{DMARC: &spamCheck{Allow: &f, Info: "fail"}}}
	flags := releaseRiskFlags(l)
	if len(flags) != 2 {
		t.Fatalf("want impersonation + dmarc flags, got %v", flags)
	}
	ledger := []ledgerRow{{At: "2026-10-14T10:05:00Z", Action: "release", Outcome: "released", Ticket: "T-9", Sender: "CEO@evil.example", Subject: "Pay now"}}
	if got := attributeTickets([]releaseLog{l}, ledger); got[0] != "T-9" {
		t.Fatalf("attributeTickets = %v", got)
	}
	// Two releases competing for one ledger row: neither gets the ticket,
	// even when the other release is benign.
	benign := l
	benign.HeldReason, benign.SpamProcessingDetail = "Spam", nil
	if got := attributeTickets([]releaseLog{l, benign}, ledger); len(got) != 0 {
		t.Fatalf("a contested ledger row must not be attributed, got %v", got)
	}
	ledger[0].At = "2026-10-14T12:00:00Z"
	if got := attributeTickets([]releaseLog{l}, ledger); len(got) != 0 {
		t.Fatalf("ledger row 2h away must not match, got %v", got)
	}
	v := buildReleaseAudit([]releaseLog{l, {Status: "rejected", Operator: mcAddr{EmailAddress: "admin@example.com"}}}, nil, "")
	if len(v.Risky) != 1 || !v.Risky[0].NoTicket || len(v.Operators) != 2 {
		t.Fatalf("audit = %+v", v)
	}
}

func TestMimecastPartialFailure(t *testing.T) {
	var top map[string]any
	_ = json.Unmarshal([]byte(`{"data":[{"id":"a","release":true}],"fail":[{"errors":[{"code":"err_x","message":"bad id"}],"key":{"id":"b"}}]}`), &top)
	r := mimecastPartialFailure(top)
	if r == nil || !strings.Contains(r.Message, "1 item(s) failed, 1 succeeded") || !strings.Contains(r.Message, "err_x") {
		t.Fatalf("partial failure report = %+v", r)
	}
	_ = json.Unmarshal([]byte(`{"data":[{"id":"a"}],"fail":[]}`), &top)
	if mimecastPartialFailure(top) != nil {
		t.Fatal("empty fail[] must not report")
	}
}

// fakeMimecast serves the endpoints the workflow commands call.
type fakeMimecast struct {
	mu       sync.Mutex
	bodies   map[string][]string
	heldHits int
}

func (f *fakeMimecast) handler(t *testing.T) http.Handler {
	f.bodies = map[string][]string{}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.bodies[r.URL.Path] = append(f.bodies[r.URL.Path], string(body))
		f.mu.Unlock()
		if r.URL.Path == "/oauth/token" {
			_, _ = io.WriteString(w, `{"access_token":"tok-123","token_type":"Bearer","expires_in":1800}`)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok-123" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"fail":[{"code":"invalid_access_token","message":"Access token is invalid"}]}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/gateway/find-held-messages":
			f.mu.Lock()
			f.heldHits++
			f.mu.Unlock()
			// Always return the same next token: the paginator must stop on the repeat.
			_, _ = io.WriteString(w, `{"meta":{"status":200,"pagination":{"pageSize":100,"next":"T1"}},"data":[{"heldEmails":[
				{"id":"imp1","subject":"Wire transfer","heldReason":"Impersonation Protection","fromEnv":{"emailAddress":"ceo@evil.example"},"to":[{"emailAddress":"alice@example.com"}],"heldSince":"2026-10-14T09:00:00+0000"},
				{"id":"spam1","subject":"Cheap pills","heldReason":"Default Spam Scanning Definition","spamScore":45,"fromEnv":{"emailAddress":"spam@bad.example"},"to":[{"emailAddress":"bob@example.com"}]},
				{"id":"pol1","subject":"Invoice 42","heldReason":"Attachment Type Policy","policyInfo":"Block executables","fromEnv":{"emailAddress":"billing@vendor.example"},"to":[{"emailAddress":"alice@example.com"}],"heldSince":"2026-10-14T08:00:00+0000"}
			]}],"fail":[]}`)
		case "/api/gateway/hold-release":
			_, _ = io.WriteString(w, `{"meta":{"status":200},"data":[{"id":"pol1","release":true}],"fail":[]}`)
		case "/api/message-finder/search":
			_, _ = io.WriteString(w, `{"meta":{"status":200},"data":[{"trackedEmails":[{"id":"trk1","subject":"Invoice 42","fromEnv":{"emailAddress":"billing@vendor.example"},"to":[{"emailAddress":"alice@example.com"}],"status":"held","received":"2026-10-14T08:00:00+0000"}]}],"fail":[]}`)
		case "/api/message-finder/get-message-info":
			_, _ = io.WriteString(w, `{"meta":{"status":200},"data":[{"id":"trk1","status":"held","spamInfo":{"spamScore":3,"spamProcessingDetail":{"spf":{"allow":false,"info":"softfail"},"dkim":{"allow":true,"info":"pass"}}}}],"fail":[]}`)
		case "/api/gateway/get-held-release-logs", "/api/gateway/get-rejections":
			_, _ = io.WriteString(w, `{"meta":{"status":200},"data":[],"fail":[]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"fail":[{"code":"endpoint_evaluation_failed_1","message":"Endpoint cannot be found"}]}`)
		}
	})
}

func runMimecast(t *testing.T, srv *httptest.Server, args ...string) (string, error) {
	t.Helper()
	t.Setenv("MIMECAST_CLIENT_ID", "cid")
	return runMimecast2(t, srv, args...)
}

// runMimecast2 runs the CLI without resetting MIMECAST_CLIENT_ID.
func runMimecast2(t *testing.T, srv *httptest.Server, args ...string) (string, error) {
	t.Helper()
	t.Setenv("MIMECAST_BASE_URL", srv.URL)
	t.Setenv("MIMECAST_TOKEN_URL", srv.URL+"/oauth/token")
	t.Setenv("MIMECAST_CLIENT_SECRET", "csecret")
	cmd := RootCmd()
	cmd.SetArgs(append(args, "--json", "--no-cache"))
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	err := cmd.Execute()
	if err != nil {
		t.Logf("stderr: %s", errb.String())
	}
	return out.String(), err
}

func TestWhyEndToEnd(t *testing.T) {
	testenv.Isolate(t)
	fake := &fakeMimecast{}
	srv := httptest.NewServer(fake.handler(t))
	defer srv.Close()
	t.Setenv("MIMECAST_ACCOUNT_CODE", "CUST42")
	out, err := runMimecast(t, srv, "why", "--from", "billing@vendor.example", "--to", "alice@example.com", "--since", "3d")
	if err != nil {
		t.Fatalf("why failed: %v\n%s", err, out)
	}
	var v whyView
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("why output not JSON: %v\n%s", err, out)
	}
	if v.Stage != "held" || len(v.Held) != 1 || v.Held[0].ID != "pol1" || v.Held[0].RiskClass != riskPolicyReview {
		t.Fatalf("want held pol1 policy-review, got stage=%s held=%+v", v.Stage, v.Held)
	}
	if !strings.Contains(v.Summary, "spf") {
		t.Fatalf("summary should surface the failed SPF check: %s", v.Summary)
	}
	// Negative: the impersonation hold from another sender must not leak in.
	for _, h := range v.Held {
		if h.ID == "imp1" {
			t.Fatal("why matched a held message from a different sender")
		}
	}
	// Delegate access: every envelope request carries meta.accountCode.
	for _, b := range fake.bodies["/api/message-finder/search"] {
		if !strings.Contains(b, `"accountCode":"CUST42"`) {
			t.Fatalf("accountCode missing from request: %s", b)
		}
	}
}

func TestReleaseSafeRefusesNeverReleaseAndWritesLedger(t *testing.T) {
	testenv.Isolate(t)
	fake := &fakeMimecast{}
	srv := httptest.NewServer(fake.handler(t))
	defer srv.Close()
	// Without --override both are refused: imp1 is never-release, pol1 is a
	// policy hold. Nothing is sent and the refusal exits 1.
	out, err := runMimecast(t, srv, "held", "release-safe", "imp1", "pol1", "--ticket", "T-1")
	if code := ExitCode(err); code != 1 || len(fake.bodies["/api/gateway/hold-release"]) != 0 {
		t.Fatalf("refusals must exit 1 and send nothing, got %d (%v)\n%s", code, err, out)
	}
	out, err = runMimecast(t, srv, "held", "release-safe", "pol1", "--ticket", "T-1", "--override", "reviewed the attachment with the requester")
	if err != nil {
		t.Fatalf("overridden policy release: %v\n%s", err, out)
	}
	var v releaseSafeView
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	got := map[string]string{}
	for _, r := range v.Results {
		got[r.ID] = r.Outcome
	}
	if got["pol1"] != "released" || v.Released != 1 || v.Results[0].Override == "" {
		t.Fatalf("outcomes = %v (%+v)", got, v)
	}
	for _, b := range fake.bodies["/api/gateway/hold-release"] {
		if strings.Contains(b, "imp1") {
			t.Fatalf("never-release ID was sent to hold-release: %s", b)
		}
	}
	if fake.heldHits != 2 {
		t.Fatalf("release-safe should stop paging once every requested ID is found (1 call per run), made %d", fake.heldHits)
	}
	out, err = runMimecast(t, srv, "held", "release-audit", "--since", "7d")
	if err != nil {
		t.Fatalf("release-audit: %v\n%s", err, out)
	}
}

func TestTriageNewOnlySecondRunIsEmpty(t *testing.T) {
	testenv.Isolate(t)
	fake := &fakeMimecast{}
	srv := httptest.NewServer(fake.handler(t))
	defer srv.Close()
	out, err := runMimecast(t, srv, "held", "triage", "--since", "24h", "--new")
	if err != nil {
		t.Fatalf("triage: %v\n%s", err, out)
	}
	var v triageView
	if err := json.Unmarshal([]byte(out), &v); err != nil || len(v.Groups) != 3 || v.Groups[0].RiskClass != riskNeverRelease {
		t.Fatalf("first triage run: %v %+v", err, v)
	}
	out, err = runMimecast(t, srv, "held", "triage", "--since", "24h", "--new")
	if err != nil {
		t.Fatalf("triage 2: %v", err)
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil || len(v.Groups) != 0 || v.Note == "" {
		t.Fatalf("second --new run should be empty with a note: %v %+v", err, v)
	}
}

func TestGeneratedEndpointFailEnvelopeExitsNonZero(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			_, _ = io.WriteString(w, `{"access_token":"tok-123","token_type":"Bearer","expires_in":1800}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"meta":{"status":200},"data":[],"fail":[{"errors":[{"code":"err_hold_not_found","message":"Held message not found","retryable":false}],"key":{"id":"nope"}}]}`)
	}))
	defer srv.Close()
	out, err := runMimecast(t, srv, "held", "release", "--data", `[{"id":"nope"}]`)
	if err == nil {
		t.Fatalf("HTTP 200 + fail[] must exit non-zero; got success: %s", out)
	}
	if !strings.Contains(err.Error(), "err_hold_not_found") {
		t.Fatalf("error should name the Mimecast failure code: %v", err)
	}
	if code := ExitCode(err); code != 5 {
		t.Fatalf("exit code = %d, want 5", code)
	}
}

func TestRemainingWorkflowsAgainstFake(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/oauth/token":
			_, _ = io.WriteString(w, `{"access_token":"tok-123","token_type":"Bearer","expires_in":1800}`)
		case "/api/email/get-email-queues":
			_, _ = io.WriteString(w, `{"meta":{"status":200},"data":[{"inboundEmailQueue":[{"count":5,"date":"2026-10-14T10:00:00+0000"},{"count":450,"date":"2026-10-14T11:00:00+0000"}],"outboundEmailQueue":[{"count":2,"date":"2026-10-14T11:00:00+0000"}]}],"fail":[]}`)
		case "/email/cloud-gateway/v1/statistics":
			_, _ = io.WriteString(w, `{"processingQueueSize":3,"deliveryQueueSize":1,"lastInboundDateTime":"`+mcTime(time.Now())+`"}`)
		case "/api/gateway/find-processing-messages":
			_, _ = io.WriteString(w, `{"meta":{"status":200},"data":[{"messages":[{"id":"p1","attempts":6,"info":"421 try later","remoteIp":"203.0.113.9"}]}],"fail":[]}`)
		case "/api/gateway/find-held-messages":
			_, _ = io.WriteString(w, `{"meta":{"status":200},"data":[{"heldEmails":[
				{"id":"s1","fromEnv":{"emailAddress":"spam@bad.example"},"to":[{"emailAddress":"bob@example.com"}],"heldReason":"Spam"},
				{"id":"s2","fromEnv":{"emailAddress":"spam@bad.example"},"to":[{"emailAddress":"carol@example.com"}],"heldReason":"Spam"},
				{"id":"x1","fromEnv":{"emailAddress":"ok@notbad.example"},"to":[{"emailAddress":"bob@example.com"}],"heldReason":"Spam"}]}],"fail":[]}`)
		case "/api/gateway/get-held-release-logs":
			rows := []string{}
			for i := 0; i < 3; i++ {
				rows = append(rows, `{"id":"r`+string(rune('a'+i))+`","status":"released","released":"`+mcTime(time.Now().Add(-time.Duration(i+1)*time.Hour))+`","fromEnv":{"emailAddress":"news@vendor.example"},"to":[{"emailAddress":"alice@example.com"}],"operator":{"emailAddress":"admin@example.com"},"heldReason":"Spam","subject":"Weekly news"}`)
			}
			rows = append(rows, `{"id":"rz","status":"released","released":"`+mcTime(time.Now())+`","fromEnv":{"emailAddress":"ceo@evil.example"},"to":[{"emailAddress":"alice@example.com"}],"operator":{"emailAddress":"admin@example.com"},"heldReason":"Impersonation Protection","subject":"Pay now"}`)
			_, _ = io.WriteString(w, `{"meta":{"status":200},"data":[{"heldReleaseLogs":[`+strings.Join(rows, ",")+`]}],"fail":[]}`)
		case "/api/ttp/url/decode-url":
			_, _ = io.WriteString(w, `{"meta":{"status":200},"data":[{"url":"https://phish.example/login","success":true}],"fail":[]}`)
		case "/api/ttp/url/get-logs":
			_, _ = io.WriteString(w, `{"meta":{"status":200},"data":[{"clickLogs":[
				{"date":"2026-10-14T10:00:00+0000","userEmailAddress":"bob@example.com","url":"https://phish.example/login?u=1","action":"warn","scanResult":"malicious","userOverride":"Continue"},
				{"date":"2026-10-14T10:05:00+0000","userEmailAddress":"carol@example.com","url":"https://good.example/","action":"allow","scanResult":"clean"}]}],"fail":[]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	out, err := runMimecast(t, srv, "mailflow", "--max-queue", "200")
	var mf mailflowView
	if err == nil || json.Unmarshal([]byte(out), &mf) != nil || mf.Verdict != "FAIL" || mf.StuckMessages != 1 {
		t.Fatalf("mailflow should FAIL on the 450 inbound backlog with 1 stuck message: err=%v %s", err, out)
	}
	if code := ExitCode(err); code != 1 {
		t.Fatalf("mailflow FAIL exit = %d, want 1", code)
	}

	out, err = runMimecast(t, srv, "held", "purge-sender", "spam@bad.example", "--block", "--plan")
	var pv purgeView
	if err != nil || json.Unmarshal([]byte(out), &pv) != nil || pv.Matched != 2 || len(pv.BlockPairs) != 2 || len(pv.Rejected) != 0 {
		t.Fatalf("purge-sender --plan: err=%v %s", err, out)
	}

	out, err = runMimecast(t, srv, "held", "release-audit", "--since", "7d")
	var av releaseAuditView
	if err != nil || json.Unmarshal([]byte(out), &av) != nil || len(av.Risky) != 1 || !av.Risky[0].NoTicket {
		t.Fatalf("release-audit should flag the impersonation release without a ticket: err=%v %s", err, out)
	}

	out, err = runMimecast(t, srv, "senders", "suggest", "--since", "7d", "--min-holds", "3")
	var sv sendersSuggestView
	if err != nil || json.Unmarshal([]byte(out), &sv) != nil || len(sv.PermitCandidates) != 1 || sv.PermitCandidates[0].Sender != "news@vendor.example" {
		t.Fatalf("senders suggest: err=%v %s", err, out)
	}

	out, err = runMimecast(t, srv, "ttp", "who-clicked", "https://protect-us.mimecast.com/s/AbCdEfGh", "--since", "7d")
	var wv whoClickedView
	if err != nil || json.Unmarshal([]byte(out), &wv) != nil || len(wv.Clicks) != 1 || wv.Clicks[0].User != "bob@example.com" || wv.ClickedThru != 1 {
		t.Fatalf("who-clicked should match only bob's click after decoding: err=%v %s", err, out)
	}
}

func TestCodexRound1Regressions(t *testing.T) {
	testenv.Isolate(t)
	var mu sync.Mutex
	rejectReply := `{"meta":{"status":200},"data":[{"id":"s1","reject":true},{"id":"s2","reject":false}],"fail":[]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/oauth/token":
			_, _ = io.WriteString(w, `{"access_token":"tok-123","token_type":"Bearer","expires_in":1800}`)
		case "/api/gateway/find-held-messages":
			_, _ = io.WriteString(w, `{"meta":{"status":200},"data":[{"heldEmails":[
				{"id":"s1","fromEnv":{"emailAddress":"spam@bad.example"},"to":[{"emailAddress":"bob@example.com"}],"heldReason":"Spam"},
				{"id":"s2","fromEnv":{"emailAddress":"legit@good.example"},"fromHdr":{"emailAddress":"spam@bad.example"},"to":[{"emailAddress":"carol@example.com"}],"heldReason":"Spam"},
				{"id":"other","fromEnv":{"emailAddress":"billing@vendor.example"},"to":[{"emailAddress":"alice@example.com"}],"heldReason":"Attachment Type Policy"}]}],"fail":[]}`)
		case "/api/gateway/hold-reject":
			_, _ = io.WriteString(w, rejectReply)
		case "/api/managedsender/permit-or-block-sender":
			_, _ = io.WriteString(w, `{"meta":{"status":200},"data":[{"id":"m1","sender":"spam@bad.example","to":"bob@example.com","type":"Block"},{"id":"m2","sender":"spam@bad.example","to":"carol@example.com","type":"Block"}],"fail":[]}`)
		case "/api/message-finder/get-message-info":
			_, _ = io.WriteString(w, `{"meta":{"status":200},"data":[{"id":"msgX","status":"delivery","deliveredMessage":{"x":{"messageInfo":{"fromEnvelope":"billing@vendor.example"}}}}],"fail":[]}`)
		case "/api/gateway/get-held-release-logs", "/api/gateway/get-rejections":
			_, _ = io.WriteString(w, `{"meta":{"status":200},"data":[],"fail":[]}`)
		case "/api/ttp/url/get-logs":
			// data wrapper present but the function failed: must not read as "no clicks".
			_, _ = io.WriteString(w, `{"meta":{"status":200},"data":[{"clickLogs":[]}],"fail":[{"errors":[{"code":"err_validation","message":"bad window"}]}]}`)
		case "/api/email/get-email-queues":
			_, _ = io.WriteString(w, `{"meta":{"status":200},"data":[{"inboundEmailQueue":[{"count":1,"date":"2026-10-14T11:00:00+0000"}]}],"fail":[]}`)
		case "/api/gateway/find-processing-messages":
			_, _ = io.WriteString(w, `{"meta":{"status":200},"data":[{"messages":[]}],"fail":[]}`)
		case "/email/cloud-gateway/v1/statistics":
			t.Error("statistics must not be read for a delegated account")
			_, _ = io.WriteString(w, `{}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	// purge-sender: s2 only matched on the header, so its block pair uses the
	// header identity; s2 came back reject:false, so it is a failure.
	if out, err := runMimecast(t, srv, "held", "purge-sender", "spam@bad.example", "--block"); err != nil || !strings.Contains(out, `"plan": true`) {
		t.Fatalf("without --yes purge-sender must only plan: %v %s", err, out)
	}
	if _, err := runMimecast(t, srv, "held", "purge-sender", "com", "--yes"); ExitCode(err) != 2 {
		t.Fatalf("a bare TLD must be a usage error, got %v", err)
	}
	out, err := runMimecast(t, srv, "held", "purge-sender", "spam@bad.example", "--block", "--yes")
	var pv purgeView
	if json.Unmarshal([]byte(out), &pv) != nil {
		t.Fatalf("purge-sender output: %s", out)
	}
	for _, p := range pv.BlockPairs {
		if p.Sender == "legit@good.example" {
			t.Fatalf("blocked an unrelated envelope sender: %+v", pv.BlockPairs)
		}
	}
	if len(pv.Rejected) != 1 || pv.Rejected[0] != "s1" || len(pv.RejectFailures) != 1 || ExitCode(err) != 5 {
		t.Fatalf("reject:false must be a failure: %+v err=%v", pv, err)
	}

	// why <id>: another held message from the same sender must not decide the stage.
	out, err = runMimecast(t, srv, "why", "msgX")
	var wv whyView
	if err != nil || json.Unmarshal([]byte(out), &wv) != nil {
		t.Fatalf("why msgX: %v %s", err, out)
	}
	if wv.Stage != "delivery" || len(wv.Held) != 0 || wv.RelatedSenderHistory == nil || len(wv.RelatedSenderHistory.Held) != 1 {
		t.Fatalf("ID query must stay ID-anchored: stage=%s held=%v related=%+v", wv.Stage, wv.Held, wv.RelatedSenderHistory)
	}

	// A read whose response carries fail[] is an error, not an empty answer.
	if _, err = runMimecast(t, srv, "ttp", "who-clicked", "https://phish.example/login"); ExitCode(err) != 5 {
		t.Fatalf("fail[] on a read must exit 5, got %v", err)
	}

	// mailflow for a delegated account skips the unscoped statistics read.
	t.Setenv("MIMECAST_ACCOUNT_CODE", "CUST42")
	out, err = runMimecast(t, srv, "mailflow")
	var mv mailflowView
	if err != nil || json.Unmarshal([]byte(out), &mv) != nil || len(mv.NotMeasured) < 1 || mv.Verdict != "WARN" {
		t.Fatalf("delegated mailflow must WARN about the unmeasured statistics: %v %s", err, out)
	}
}

func TestTriageNewWithLimitMarksOnlyShown(t *testing.T) {
	testenv.Isolate(t)
	fake := &fakeMimecast{}
	srv := httptest.NewServer(fake.handler(t))
	defer srv.Close()
	if _, err := runMimecast(t, srv, "held", "triage", "--new", "--limit", "1"); err != nil {
		t.Fatal(err)
	}
	out, err := runMimecast(t, srv, "held", "triage", "--new")
	var v triageView
	if err != nil || json.Unmarshal([]byte(out), &v) != nil || len(v.Groups) != 2 {
		t.Fatalf("groups cut by --limit must stay new: %v %+v", err, v.Groups)
	}
}

func TestReadAllRepeatTokenIsTruncated(t *testing.T) {
	testenv.Isolate(t)
	fake := &fakeMimecast{}
	srv := httptest.NewServer(fake.handler(t))
	defer srv.Close()
	out, err := runMimecast(t, srv, "held", "triage", "--since", "24h")
	var v triageView
	if err != nil || json.Unmarshal([]byte(out), &v) != nil || !v.Truncated || v.Note == "" {
		t.Fatalf("a repeated page token must mark the scan truncated: %v %+v", err, v)
	}
}

func TestReleaseBatchRetriesAllFailedBatchOnce(t *testing.T) {
	testenv.Isolate(t)
	var mu sync.Mutex
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/oauth/token":
			_, _ = io.WriteString(w, `{"access_token":"tok-123","token_type":"Bearer","expires_in":1800}`)
		case "/api/gateway/find-held-messages":
			_, _ = io.WriteString(w, `{"meta":{"status":200},"data":[{"heldEmails":[{"id":"sp1","heldReason":"Spam","fromEnv":{"emailAddress":"a@b.example"}}]}],"fail":[]}`)
		case "/api/gateway/hold-release":
			mu.Lock()
			calls++
			n := calls
			mu.Unlock()
			if n == 1 {
				_, _ = io.WriteString(w, `{"meta":{"status":200},"data":[],"fail":[{"errors":[{"code":"err_busy","message":"try again","retryable":true}],"key":{"id":"sp1"}}]}`)
				return
			}
			_, _ = io.WriteString(w, `{"meta":{"status":200},"data":[{"id":"sp1","release":true}],"fail":[]}`)
		}
	}))
	defer srv.Close()
	out, err := runMimecast(t, srv, "held", "release-safe", "sp1", "--ticket", "T-2")
	if err != nil || calls != 2 || !strings.Contains(out, `"released": 1`) {
		t.Fatalf("all-failed batch should be retried once and succeed: calls=%d err=%v %s", calls, err, out)
	}
}

func TestCodexRound2Regressions(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body, _ := io.ReadAll(r.Body)
		switch r.URL.Path {
		case "/oauth/token":
			_, _ = io.WriteString(w, `{"access_token":"tok-123","token_type":"Bearer","expires_in":1800}`)
		case "/api/gateway/find-held-messages":
			if strings.Contains(string(body), `"fieldName":"from"`) || strings.Contains(string(body), "window-fail") {
				// generated read with a data wrapper next to fail[]
				_, _ = io.WriteString(w, `{"meta":{"status":200},"data":[{"heldEmails":[]}],"fail":[{"errors":[{"code":"err_window","message":"bad window"}]}]}`)
				return
			}
			_, _ = io.WriteString(w, `{"meta":{"status":200},"data":[{"heldEmails":[]}],"fail":[]}`)
		case "/api/message-finder/search":
			_, _ = io.WriteString(w, `{"meta":{"status":200},"data":[{"trackedEmails":[]}],"fail":[]}`)
		case "/api/gateway/get-held-release-logs":
			_, _ = io.WriteString(w, `{"meta":{"status":200},"data":[{"heldReleaseLogs":[]}],"fail":[]}`)
		case "/api/gateway/get-rejections":
			if !strings.Contains(string(body), `"fieldName":"to"`) {
				t.Errorf("recipient-only why must search rejections by recipient: %s", body)
			}
			_, _ = io.WriteString(w, `{"meta":{"status":200},"data":[{"rejections":[{"created":"2026-10-14T09:00:00+0000","fromAddress":"x@spam.example","toAddress":"alice@example.com","description":"Spam","spamScore":40,"ipAddress":"203.0.113.7"}]}],"fail":[]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	// Generated read: a fail[] next to a data wrapper must not exit 0.
	if _, err := runMimecast(t, srv, "held", "find", "--data", `[{"admin":true,"start":"window-fail"}]`); ExitCode(err) != 5 {
		t.Fatalf("generated read with fail[] must exit 5, got %v", err)
	}

	// Recipient-only why finds the gateway rejection.
	out, err := runMimecast(t, srv, "why", "--to", "alice@example.com", "--since", "3d")
	var wv whyView
	if err != nil || json.Unmarshal([]byte(out), &wv) != nil || wv.Stage != "rejected-at-gateway" {
		t.Fatalf("recipient-only why: %v %s", err, out)
	}

	// An explicit --db belongs to one tenant and account.
	db := t.TempDir() + "/shared.db"
	if _, err := runMimecast(t, srv, "held", "triage", "--new", "--db", db); err != nil {
		t.Fatalf("first triage: %v", err)
	}
	t.Setenv("MIMECAST_CLIENT_ID", "another-tenant-app")
	if _, err := runMimecast2(t, srv, "held", "triage", "--new", "--db", db); ExitCode(err) != 10 {
		t.Fatalf("reusing a --db for another tenant must be a config error (10), got %v", err)
	}
	t.Setenv("MIMECAST_CLIENT_ID", "cid")
	t.Setenv("MIMECAST_ACCOUNT_CODE", "OTHER")
	if _, err := runMimecast(t, srv, "held", "triage", "--new", "--db", db); ExitCode(err) != 10 {
		t.Fatalf("reusing a --db for another account must be a config error (10), got %v", err)
	}
}

func TestUnownedPopulatedDatabaseIsRefused(t *testing.T) {
	testenv.Isolate(t)
	path := t.TempDir() + "/legacy.db"
	db, err := openMimecastStore(context.Background(), path, "", "app:a account:")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`INSERT INTO mimecast_held_seen (id, first_seen, last_seen) VALUES ('x','t','t')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`DELETE FROM mimecast_meta`); err != nil { // simulate a database written before ownership existed
		t.Fatal(err)
	}
	_ = db.Close()
	if _, err := openMimecastStore(context.Background(), path, "CUST1", "app:a account:CUST1"); ExitCode(err) != 10 {
		t.Fatalf("a populated database with no owner must be refused, got %v", err)
	}
	empty := t.TempDir() + "/empty.db"
	db, err = openMimecastStore(context.Background(), empty, "CUST1", "app:a account:CUST1")
	if err != nil {
		t.Fatalf("an empty database is stamped for the first account: %v", err)
	}
	_ = db.Close()
}

func TestWhySubjectKeepsRejectionsAsContext(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/oauth/token":
			_, _ = io.WriteString(w, `{"access_token":"tok-123","token_type":"Bearer","expires_in":1800}`)
		case "/api/message-finder/search":
			_, _ = io.WriteString(w, `{"meta":{"status":200},"data":[{"trackedEmails":[]}],"fail":[]}`)
		case "/api/gateway/find-held-messages":
			_, _ = io.WriteString(w, `{"meta":{"status":200},"data":[{"heldEmails":[]}],"fail":[]}`)
		case "/api/gateway/get-held-release-logs":
			_, _ = io.WriteString(w, `{"meta":{"status":200},"data":[{"heldReleaseLogs":[]}],"fail":[]}`)
		case "/api/gateway/get-rejections":
			_, _ = io.WriteString(w, `{"meta":{"status":200},"data":[{"rejections":[{"created":"2026-10-14T09:00:00+0000","fromAddress":"news@vendor.example","toAddress":"alice@example.com","description":"Spam"}]}],"fail":[]}`)
		}
	}))
	defer srv.Close()
	out, err := runMimecast(t, srv, "why", "--from", "news@vendor.example", "--subject", "Invoice")
	var v whyView
	if err != nil || json.Unmarshal([]byte(out), &v) != nil || v.Stage != "not-found" || v.RelatedSenderHistory == nil || len(v.RelatedSenderHistory.Rejections) != 1 {
		t.Fatalf("a subjectless rejection must not decide a --subject query: %v %s", err, out)
	}
}

func TestWriteFailureKeepsExitClass(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/oauth/token":
			_, _ = io.WriteString(w, `{"access_token":"tok-123","token_type":"Bearer","expires_in":1800}`)
		case "/api/gateway/find-held-messages":
			_, _ = io.WriteString(w, `{"meta":{"status":200},"data":[{"heldEmails":[{"id":"sp1","heldReason":"Spam","fromEnv":{"emailAddress":"a@b.example"}}]}],"fail":[]}`)
		case "/api/gateway/hold-release":
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"fail":[{"code":"rate_limit","message":"temporarily busy"}]}`)
		}
	}))
	defer srv.Close()
	if _, err := runMimecast(t, srv, "held", "release-safe", "sp1", "--ticket", "T-3"); ExitCode(err) != 7 {
		t.Fatalf("a rate-limited release must exit 7, got %d (%v)", ExitCode(err), err)
	}
}

func TestAccountCodeGuardsRESTWrites(t *testing.T) {
	testenv.Isolate(t)
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/oauth/token" {
			_, _ = io.WriteString(w, `{"access_token":"tok-123","token_type":"Bearer","expires_in":1800}`)
			return
		}
		hit = true
		_, _ = io.WriteString(w, `{}`)
	}))
	defer srv.Close()
	t.Setenv("MIMECAST_ACCOUNT_CODE", "CUST42")
	_, err := runMimecast(t, srv, "gateway", "set-protection-mode", "--mode", "gateway")
	if err == nil || hit || !strings.Contains(err.Error(), "MIMECAST_ACCOUNT_CODE") {
		t.Fatalf("a REST write with an account code set must be refused before sending: hit=%v err=%v", hit, err)
	}
}

func TestPageNoteWording(t *testing.T) {
	if n := (mcPage{Pages: 2, Truncated: true, Loop: true}).note("raise --max-scan-pages"); !strings.Contains(n, "repeated a page token after 2 pages") || strings.Contains(n, "max-scan-pages") {
		t.Fatalf("loop note = %q", n)
	}
	if n := (mcPage{Pages: 10, Truncated: true}).note("raise --max-scan-pages"); n != "scan stopped at 10 pages; raise --max-scan-pages" {
		t.Fatalf("cap note = %q", n)
	}
	if n := (mcPage{Pages: 3}).note("x"); n != "" {
		t.Fatalf("complete scan note = %q", n)
	}
	if got := (mcFailure{}).text(); got == "" {
		t.Fatal("an empty fail entry must still produce text")
	}
}
