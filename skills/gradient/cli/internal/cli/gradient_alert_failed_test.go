// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gradient-pp-cli/internal/cliutil/testenv"
	"gradient-pp-cli/internal/ledger"
)

// alert-failure-is-terminal (Codex r1 class documented alert failure reported
// as timeout): the debug route's documented failure (RetrieveAlertResponseDto
// status "failed", statusCode, errors) ends the wait as failed, exits
// non-zero, and the ledger records it as failed with Gradient's reason.
func TestAlertSendReportsADocumentedFailure(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/vendor-api/alerting/acct-1042"):
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"messageId":"m1","alertId":"a1","correlationID":"c1"}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/vendor-api/alerting/debug/m1"):
			_, _ = w.Write([]byte(`{"messageId":"m1","status":"failed","statusCode":400,"errors":["PSA rejected ticket"]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(api.Close)
	testenv.Isolate(t)
	dir := t.TempDir()
	t.Setenv("GRADIENT_LEDGER_DIR", dir)
	t.Setenv("GRADIENT_BASE_URL", api.URL+"/api")
	t.Setenv("GRADIENT_TOKEN", "alertFailUnitToken-5c1e9a")
	flags := &rootFlags{asJSON: true, dataSource: "auto", timeout: 10 * time.Second}
	cmd := newNovelAlertSendCmd(flags)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--account", "acct-1042", "--title", "Backup", "--description", "Failed", "--wait", "--timeout-wait", "0s"})
	err := cmd.ExecuteContext(context.Background())
	if err == nil {
		t.Errorf("a documented alert failure must exit non-zero\n%s", out.String())
	}
	var view alertSendView
	if jerr := json.Unmarshal([]byte(out.String()[:strings.LastIndex(out.String(), "}")+1]), &view); jerr != nil {
		t.Fatalf("output is not JSON: %v\n%s", jerr, out.String())
	}
	if view.TicketStatus != "failed" || !strings.Contains(view.Note, "PSA rejected ticket") {
		t.Errorf("want ticket_status failed with Gradient's reason, got %q (%s)", view.TicketStatus, view.Note)
	}
	alerts, lerr := ledger.ReadAlerts(dir)
	if lerr != nil || len(alerts) != 1 {
		t.Fatalf("ledger: %v %v", alerts, lerr)
	}
	if alerts[0].TicketStatus != "failed" || !strings.Contains(alerts[0].Error, "PSA rejected ticket") {
		t.Errorf("ledger keeps %q (%q), want failed with the reason", alerts[0].TicketStatus, alerts[0].Error)
	}
}

// Any failure signal wins over the rest of the debug response; only a
// ticketId with no failure signal is a created ticket.
func TestReadAlertDebugAnyFailureSignalWins(t *testing.T) {
	for body, want := range map[string]string{
		`{"status":"failed"}`:                                    "failed",
		`{"status":" FAILED "}`:                                  "failed",
		`{"errors":["x"]}`:                                       "failed",
		`{"errors":"PSA down"}`:                                  "failed",
		`{"errors":{"psa":"down"}}`:                              "failed",
		`{"statusCode":500}`:                                     "failed",
		`{"statusCode":"404"}`:                                   "failed",
		`{"ticketId":"T-1","status":"success","errors":["x"]}`:   "failed",
		`{"ticketId":"T-1","statusCode":422}`:                    "failed",
		`{"ticketId":"T-1","status":"success"}`:                  "created",
		`{"ticketId":"T-1","status":"success","statusCode":201}`: "created",
		`{"ticketId":"T-1","errors":[],"statusCode":0}`:          "created",
		`{"status":"pending"}`:                                   "pending",
		`{"status":"success"}`:                                   "pending",
		`not json`:                                               "pending",
	} {
		if got := readAlertDebug([]byte(body)).State; got != want {
			t.Errorf("%s: state %q, want %q", body, got, want)
		}
	}
}

// alert trace --stuck lists every alert that is not a created ticket,
// failed ones included.
func TestAlertTraceStuckIncludesFailed(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GRADIENT_LEDGER_DIR", dir)
	now := time.Now().UTC()
	for _, rec := range []ledger.AlertRecord{
		{At: now, AccountID: "a1", MessageID: "m1", TicketStatus: "created", TicketID: "T-1"},
		{At: now, AccountID: "a2", MessageID: "m2", TicketStatus: "failed", Error: "PSA rejected ticket"},
	} {
		if err := ledger.AppendAlert(dir, rec); err != nil {
			t.Fatal(err)
		}
	}
	cmd := newNovelAlertTraceCmd(&rootFlags{asJSON: true, dataSource: "auto"})
	_ = cmd.Flags().Set("stuck", "true")
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	var view alertTraceView
	if err := json.Unmarshal(out.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.StuckCount != 1 || len(view.Alerts) != 1 || view.Alerts[0].MessageID != "m2" {
		t.Errorf("--stuck must list the failed alert: %+v", view)
	}
}
