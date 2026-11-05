// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (NOT generated): regression guards for the classes left open
// after the 4.34.0 reprint review, round 2.

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"avanan-pp-cli/internal/avananmirror"
	"avanan-pp-cli/internal/store"
)

// mixedTaskResponse is SaasEntityActionResponse with one target whose task is
// null (Google Mail, live-verified: ledger task-poll-needs-query-string-scope)
// and one with a real task.
const mixedTaskResponse = `{"responseEnvelope":{"responseCode":0,"responseText":"OK"},"responseData":[` +
	`{"entityId":"G","taskId":null},{"entityId":"O","taskId":"T-1"}]}`

func wantPending(t *testing.T, what string, err error, out string) {
	t.Helper()
	var ce *cliError
	if err == nil || !errors.As(err, &ce) || ce.code != ExitJobPending {
		t.Errorf("%s returned %v, want exit %d (an unconfirmed target is not success)\n%s", what, err, ExitJobPending, out)
	}
}

// TestActionWaitNeverSucceedsWithAnUntaskedTarget: the parser dropped the
// null-task record, so --wait polled only T-1 and exited 0 while target G was
// never observed.
func TestActionWaitNeverSucceedsWithAnUntaskedTarget(t *testing.T) {
	s := &reviewServer{handlers: map[string]func(http.ResponseWriter, *http.Request){
		"POST /v1.0/action/entity": jsonReply(mixedTaskResponse),
		"GET /v1.0/task/T-1":       jsonReply(taskReply("completed")),
	}}
	srv := httptest.NewServer(s)
	defer srv.Close()
	out, err := runAgainst(t, srv, "action", "post-entity",
		"--request-data-entity-action-name", "quarantine",
		"--request-data-entity-ids", "G,O",
		"--request-data-entity-type", "office365_emails_email",
		"--request-data-scope", "farm1:tenant-a",
		"--wait", "--wait-timeout", "10s", "--json")
	wantPending(t, "action --wait with a null-task target", err, out)
	if s.count("GET /v1.0/task/T-1") == 0 {
		t.Errorf("--wait never polled the real task\n%s", out)
	}
}

// TestRemediateWaitNeverSucceedsWithAnUnconfirmedTarget: an unmapped target,
// an all-null response the entity never confirmed, and a task that never went
// terminal each set completed=false and still exited 0.
func TestRemediateWaitNeverSucceedsWithAnUnconfirmedTarget(t *testing.T) {
	for _, tc := range []struct {
		name, action, task string
		entities           []string
	}{
		{"mixed null and real task", mixedTaskResponse, taskReply("completed"), []string{"G", "O"}},
		{"every task null", `{"responseEnvelope":{"responseCode":0},"responseData":[{"entityId":"G","taskId":null}]}`, "", []string{"G"}},
		{"task never terminal", `{"responseEnvelope":{"responseCode":0},"responseData":[{"entityId":"O","taskId":"T-1"}]}`, taskReply("inprogress"), []string{"O"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := map[string]func(http.ResponseWriter, *http.Request){"POST /v1.0/action/entity": jsonReply(tc.action)}
			if tc.task != "" {
				h["GET /v1.0/task/T-1"] = jsonReply(tc.task)
			}
			srv := httptest.NewServer(&reviewServer{handlers: h})
			defer srv.Close()
			args := []string{"remediate", "quarantine", "--scope", "farm1:tenant-a", "--entity-type", "office365_emails_email",
				"--wait", "--wait-timeout", "3s", "--yes", "--json"}
			for _, e := range tc.entities {
				args = append(args, "--entity", e)
			}
			out, err := runAgainst(t, srv, args...)
			wantPending(t, "remediate --wait", err, out)
		})
	}
}

// legacyExceptionRow is a row the shipped (4.30.2) mirror wrote: no partition,
// no scope, no selectors.
const legacyExceptionRow = `{"sub_system":"anti-phishing-whitelist","engine":"whitelist","id":"x1","match_string":"bad.example","list_side":"allow","created_by":"","comment":"","raw":{}}`

// TestLegacyExceptionRowsNeverAnswer: a row from the shipped mirror survived a
// complete empty listing (ApExceptionListResponse responseData:[]) and still
// answered `exceptions find` for an exception the API no longer lists.
func TestLegacyExceptionRowsNeverAnswer(t *testing.T) {
	// Until it is reconciled, an unattributable row must not answer.
	dbPath := seedMirror(t, map[string]map[string]string{
		avananmirror.ResourceExceptions: {"anti-phishing-whitelist/x1": legacyExceptionRow},
	})
	out, err := runLocal(t, "exceptions", "find", "bad.example", "--db", dbPath, "--json")
	if err == nil || !strings.Contains(err.Error(), "mirror --resources exceptions") {
		t.Errorf("find answered from a legacy row: %v\n%s", err, out)
	}

	// A run that reads every partition it requested replaces them.
	for _, scopes := range [][]string{nil, {"farm:a"}} {
		dbPath := seedMirror(t, map[string]map[string]string{
			avananmirror.ResourceExceptions: {"anti-phishing-whitelist/x1": legacyExceptionRow},
		})
		db, err := store.Open(dbPath)
		if err != nil {
			t.Fatal(err)
		}
		if _, problems := avananmirror.Exceptions(context.Background(), fakeExceptionPoster{}, db, avananmirror.Options{Scopes: scopes}); len(problems) != 0 {
			t.Fatalf("ingest problems: %v", problems)
		}
		db.Close()
		out, err := runLocal(t, "exceptions", "find", "bad.example", "--db", dbPath, "--json")
		var ce *cliError
		if err == nil || !errors.As(err, &ce) || ce.code != 3 {
			t.Errorf("scopes=%v: after a complete empty listing, find bad.example = %v, want no match (exit 3)\n%s", scopes, err, out)
		}
	}
}

// TestPredicateFieldsAreNotCollapsedByValue: an ApException with
// senderEmail and recipient both alice@example.com (spec
// components.schemas ApException) was shown unconditional, and audit merged it
// with the plain senderEmail rule as one predicate.
func TestPredicateFieldsAreNotCollapsedByValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mirror.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	p := fakeExceptionPoster{byPath: map[string]string{
		"/v1.0/exceptions/whitelist": `{"responseEnvelope":{"responseCode":200},"responseData":[` +
			`{"entityId":"x3","senderEmail":"alice@example.com","recipient":"alice@example.com"}]}`,
		"/v1.0/exceptions/blacklist": `{"responseEnvelope":{"responseCode":200},"responseData":[` +
			`{"entityId":"y1","senderEmail":"alice@example.com"}]}`,
	}}
	if _, problems := avananmirror.Exceptions(context.Background(), p, db, avananmirror.Options{Scopes: []string{"farm:a"}}); len(problems) != 0 {
		t.Fatalf("ingest problems: %v", problems)
	}
	if err := db.SaveSyncState(avananmirror.ResourceExceptions, "", 2); err != nil {
		t.Fatal(err)
	}
	db.Close()

	out, err := runLocal(t, "exceptions", "find", "alice@example.com", "--db", path, "--json")
	if err != nil {
		t.Fatalf("find: %v\n%s", err, out)
	}
	var found struct {
		Hits []exceptionHit `json:"hits"`
	}
	if err := json.Unmarshal([]byte(out), &found); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	for _, h := range found.Hits {
		if h.ID == "x3" && !h.Conditional {
			t.Errorf("a senderEmail + recipient rule was shown unconditional:\n%s", out)
		}
	}

	out, err = runLocal(t, "exceptions", "audit", "--skip-unmatched", "--db", path, "--json")
	if err != nil {
		t.Fatalf("audit: %v\n%s", err, out)
	}
	var audit struct {
		Conflicts []json.RawMessage `json:"conflicts"`
	}
	if err := json.Unmarshal([]byte(out), &audit); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	if len(audit.Conflicts) != 0 {
		t.Errorf("audit merged a two-field predicate with a one-field rule into a conflict:\n%s", out)
	}
}
