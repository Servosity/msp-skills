// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written behavior tests for threats mailbox and threats sender.

package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mimecast-pp-cli/internal/cliutil/testenv"
)

// threatsFake serves the five sources. failing names paths that answer with
// a Mimecast fail[] envelope; nextToken makes the URL log claim more pages.
func threatsFake(t *testing.T, failing map[string]bool, nextToken bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		if r.URL.Path == "/oauth/token" {
			_, _ = io.WriteString(w, `{"access_token":"tok-123","token_type":"Bearer","expires_in":1800}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if failing[r.URL.Path] {
			_, _ = io.WriteString(w, `{"meta":{"status":200},"data":[],"fail":[{"errors":[{"code":"err_xdk_product_unavailable","message":"Product not enabled","retryable":false}]}]}`)
			return
		}
		switch r.URL.Path {
		case "/api/ttp/url/get-logs":
			meta := `{"status":200}`
			if nextToken {
				meta = `{"status":200,"pagination":{"pageSize":100,"next":"NEXT"}}`
			}
			_, _ = io.WriteString(w, `{"meta":`+meta+`,"data":[{"clickLogs":[
				{"date":"2026-10-14T10:00:00+0000","userEmailAddress":"alice@example.com","fromUserEmailAddress":"ceo@mail.phish.example","url":"https://phish.example/login","action":"allow","scanResult":"malicious","creationMethod":"User Click","userOverride":"Continue","messageId":"<m1@phish>","subject":"Pay now"},
				{"date":"2026-10-14T09:00:00+0000","userEmailAddress":"bob@example.com","fromUserEmailAddress":"ceo@mail.phish.example","url":"https://phish.example/login","action":"block","scanResult":"malicious","creationMethod":"Entry Scan","messageId":"<m2@phish>"},
				{"date":"2026-10-14T08:00:00+0000","userEmailAddress":"carol@example.com","fromUserEmailAddress":"news@notphish.example","url":"https://notphish.example/","action":"allow","scanResult":"clean","creationMethod":"User Click"}
			]}],"fail":[]}`)
		case "/api/ttp/attachment/get-logs":
			_, _ = io.WriteString(w, `{"meta":{"status":200},"data":[{"attachmentLogs":[
				{"date":"2026-10-13T12:00:00+0000","recipientAddress":"bob@example.com","senderAddress":"ceo@phish.example","fileName":"invoice.html","result":"malicious","actionTriggered":"block"},
				{"date":"2026-10-13T11:00:00+0000","recipientAddress":"partner@vendor.example","senderAddress":"alice@example.com","fileName":"report.pdf","result":"safe","actionTriggered":"none"}
			]}],"fail":[]}`)
		case "/api/ttp/impersonation/get-logs":
			_, _ = io.WriteString(w, `{"meta":{"status":200},"data":[{"impersonationLogs":[
				{"eventTime":"2026-10-14T07:00:00+0000","recipientAddress":"Alice@Example.com","senderAddress":"ceo@phish.example","action":"hold","taggedMalicious":true,"definition":"Default Impersonation"}
			],"resultCount":1}],"fail":[]}`)
		case "/api/gateway/get-rejections":
			_, _ = io.WriteString(w, `{"meta":{"status":200},"data":[{"rejections":[
				{"created":"2026-10-14T06:00:00+0000","fromAddress":"ceo@phish.example","toAddress":"dave@example.com","description":"Rejected by policy","spamScore":60}
			]}],"fail":[]}`)
		case "/api/gateway/find-held-messages":
			_, _ = io.WriteString(w, `{"meta":{"status":200},"data":[{"heldEmails":[
				{"id":"h1","subject":"Pay now","heldReason":"Impersonation Protection","fromEnv":{"emailAddress":"ceo@phish.example"},"to":[{"emailAddress":"alice@example.com"},{"emailAddress":"erin@example.com"}],"heldSince":"2026-10-14T11:00:00+0000"}
			]}],"fail":[]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"fail":[{"code":"endpoint_evaluation_failed_1","message":"Endpoint cannot be found"}]}`)
		}
	}))
}

func TestThreatsMailboxTimeline(t *testing.T) {
	testenv.Isolate(t)
	srv := threatsFake(t, nil, false)
	defer srv.Close()
	out, err := runMimecast(t, srv, "threats", "mailbox", "alice@example.com", "--since", "7d")
	if err != nil {
		t.Fatalf("threats mailbox failed: %v\n%s", err, out)
	}
	var v threatsMailboxView
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	// alice: one click (through a warning), one outbound attachment, one
	// impersonation hit (mixed-case recipient), one hold. Not carol or dave.
	if v.TotalEvents != 4 || v.ByKind["url-click"] != 1 || v.ByKind["attachment"] != 1 || v.ByKind["impersonation"] != 1 || v.ByKind["held"] != 1 {
		t.Fatalf("unexpected events: total=%d by_kind=%v\n%s", v.TotalEvents, v.ByKind, out)
	}
	if v.Events[0].Kind != "held" || v.Events[1].Kind != "url-click" || !v.Events[1].ClickedThroughWarning {
		t.Fatalf("events must be newest first with the click-through flagged: %+v", v.Events)
	}
	for _, e := range v.Events {
		if e.Kind == "attachment" && e.Direction != "outbound" {
			t.Fatalf("alice's own attachment must read outbound: %+v", e)
		}
	}
	if !v.Complete || v.FetchFailures != 0 || len(v.Sources) != 5 {
		t.Fatalf("a full scan must be complete over 5 sources: %+v", v.threatsReport)
	}
}

func TestThreatsSenderBlastRadius(t *testing.T) {
	testenv.Isolate(t)
	srv := threatsFake(t, nil, false)
	defer srv.Close()
	out, err := runMimecast(t, srv, "threats", "sender", "@phish.example", "--since", "3d")
	if err != nil {
		t.Fatalf("threats sender failed: %v\n%s", err, out)
	}
	var v threatsSenderView
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	// phish.example (and mail.phish.example) reached alice, bob, dave and erin;
	// notphish.example's carol is not part of it.
	got := map[string]threatRecipient{}
	for _, r := range v.Recipients {
		got[r.Recipient] = r
	}
	if v.TotalRecipients != 4 || got["carol@example.com"].Recipient != "" {
		t.Fatalf("want alice, bob, dave, erin; got %+v", v.Recipients)
	}
	if v.Recipients[0].Recipient != "alice@example.com" || !v.Recipients[0].Clicked || !v.Recipients[0].ClickedThroughWarning {
		t.Fatalf("the clicker must sort first: %+v", v.Recipients[0])
	}
	if got["bob@example.com"].Clicked {
		t.Fatal("an entry scan is not a click")
	}
	if got["erin@example.com"].CurrentlyHeld != 1 || got["erin@example.com"].HeldIDs[0] != "h1" || v.CurrentlyHeld != 2 || v.Clicked != 1 {
		t.Fatalf("held counts wrong: %+v / %d held, %d clicked", got["erin@example.com"], v.CurrentlyHeld, v.Clicked)
	}
}

func TestThreatsPartialFailureIsReportedNotClean(t *testing.T) {
	testenv.Isolate(t)
	srv := threatsFake(t, map[string]bool{"/api/ttp/attachment/get-logs": true}, true)
	defer srv.Close()
	out, err := runMimecast(t, srv, "threats", "mailbox", "bob@example.com", "--max-scan-pages", "1")
	if err != nil {
		t.Fatalf("a partial failure still answers: %v\n%s", err, out)
	}
	var v threatsMailboxView
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if v.Complete || v.FetchFailures != 1 || !strings.Contains(v.FirstError, "Product not enabled") {
		t.Fatalf("a failed source must be reported: %+v", v.threatsReport)
	}
	if !strings.Contains(strings.Join(v.IncompleteSources, ","), threatSrcAttachment) || !strings.Contains(strings.Join(v.IncompleteSources, ","), threatSrcURL) {
		t.Fatalf("the failed and the truncated source must be incomplete: %v", v.IncompleteSources)
	}
	// --agent compaction keeps the reconciliation fields.
	out, err = runMimecast(t, srv, "threats", "mailbox", "bob@example.com", "--agent")
	if err != nil || !strings.Contains(out, `"fetch_failures"`) || !strings.Contains(out, `"complete"`) {
		t.Fatalf("--agent dropped the failure report: %v\n%s", err, out)
	}
}

func TestThreatsAllSourcesFailedExitsNonZero(t *testing.T) {
	testenv.Isolate(t)
	srv := threatsFake(t, map[string]bool{
		"/api/ttp/url/get-logs": true, "/api/ttp/attachment/get-logs": true, "/api/ttp/impersonation/get-logs": true,
		"/api/gateway/get-rejections": true, "/api/gateway/find-held-messages": true,
	}, false)
	defer srv.Close()
	if out, err := runMimecast(t, srv, "threats", "sender", "ceo@phish.example"); err == nil {
		t.Fatalf("every source failed: must not read as zero results\n%s", out)
	}
}

func TestThreatTargetValidation(t *testing.T) {
	for in, want := range map[string]string{"@Phish.Example": "phish.example", "a@b.example": "a@b.example", " b.example ": "b.example"} {
		if got, err := threatTarget([]string{in}); err != nil || got != want {
			t.Errorf("threatTarget(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "@", "a@", "a@@b", "a b@c", "x,y"} {
		if _, err := threatTarget([]string{bad}); err == nil {
			t.Errorf("threatTarget(%q) should fail", bad)
		}
	}
}
