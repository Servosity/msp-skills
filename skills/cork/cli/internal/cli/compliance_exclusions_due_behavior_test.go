// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cork-pp-cli/internal/types"
)

const (
	exclClientA = "11111111-1111-1111-1111-111111111111"
	exclClientB = "22222222-2222-2222-2222-222222222222"
)

func exclusionsStub(t *testing.T, failB bool) *httptest.Server {
	t.Helper()
	day := func(d int) string { return time.Now().AddDate(0, 0, d).Format("2006-01-02") }
	send := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/clients", func(w http.ResponseWriter, r *http.Request) {
		send(w, map[string]any{"items": []any{
			map[string]any{"uuid": exclClientA, "name": "Acme Corp"},
			map[string]any{"uuid": exclClientB, "name": "Beta LLC"},
		}})
	})
	mux.HandleFunc("/compliance/client/"+exclClientA+"/notification-settings", func(w http.ResponseWriter, r *http.Request) {
		send(w, map[string]any{"items": []any{
			map[string]any{"event_description": "flagged-no-date", "needs_review": true},
			map[string]any{"event_description": "past-due", "review_on": day(-10), "reason": "vendor exception"},
			map[string]any{"event_description": "future", "review_on": day(30)},
		}})
	})
	mux.HandleFunc("/compliance/client/"+exclClientB+"/notification-settings", func(w http.ResponseWriter, r *http.Request) {
		if failB {
			http.Error(w, `{"detail":"boom"}`, http.StatusInternalServerError)
			return
		}
		send(w, map[string]any{"items": []any{}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func runExclusionsDue(t *testing.T, srv *httptest.Server) (string, error) {
	t.Helper()
	t.Setenv("CORK_BASE_URL", srv.URL)
	t.Setenv("CORK_API_KEY", "dummy")
	cmd := RootCmd()
	cmd.SetArgs([]string{"compliance", "exclusions-due", "--db", t.TempDir() + "/absent.db", "--json"})
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	err := cmd.Execute()
	return out.String(), err
}

// A partial sweep must report what is due, oldest first, and must say it was partial.
func TestExclusionsDueListsDueRowsAndFlagsPartialSweep(t *testing.T) {
	out, err := runExclusionsDue(t, exclusionsStub(t, true))
	if err != nil {
		t.Fatalf("one failed client must not sink the sweep: %v\n%s", err, out)
	}
	var v exclusionsDueView
	if err := json.Unmarshal([]byte(out[strings.Index(out, "{"):]), &v); err != nil {
		t.Fatalf("decode: %v\n%s", err, out)
	}
	if v.TotalDue != 2 || len(v.Items) != 2 {
		t.Fatalf("want 2 due (past-due + flagged), got %d: %+v", v.TotalDue, v.Items)
	}
	if v.Items[0].EventDescription != "past-due" || v.Items[1].EventDescription != "flagged-no-date" {
		t.Fatalf("want past-due first and the undated flagged row last, got %q then %q", v.Items[0].EventDescription, v.Items[1].EventDescription)
	}
	if v.Items[0].Client != "Acme Corp" || v.Items[0].DaysPastReview == nil || *v.Items[0].DaysPastReview < 9 {
		t.Fatalf("past-due row lost its client or days: %+v", v.Items[0])
	}
	if v.ClientsFailed != 1 || !v.ScanCapHit || v.FirstError == "" {
		t.Fatalf("a failed client must set clients_failed, scan_cap_hit and first_error: %+v", v)
	}
}

// Every client failing is a read failure, never "nothing is due".
func TestExclusionsDueRefusesAllFailedSweep(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/clients" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"items":[{"uuid":"` + exclClientA + `","name":"Acme"}]}`))
			return
		}
		http.Error(w, `{"detail":"boom"}`, http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	out, err := runExclusionsDue(t, srv)
	if err == nil {
		t.Fatalf("an all-failed sweep returned success:\n%s", out)
	}
}

func TestExclusionDueDecision(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		s    types.ComplianceEventNotificationSetting
		want bool
	}{
		{types.ComplianceEventNotificationSetting{ReviewOn: "2026-11-02"}, true},
		{types.ComplianceEventNotificationSetting{ReviewOn: "2026-11-01T00:00:00Z"}, true},
		{types.ComplianceEventNotificationSetting{ReviewOn: "2026-12-01"}, false},
		{types.ComplianceEventNotificationSetting{ReviewOn: "2026-12-01", NeedsReview: true}, true},
		{types.ComplianceEventNotificationSetting{}, false},
	} {
		if got, _ := exclusionDue(tc.s, now); got != tc.want {
			t.Errorf("exclusionDue(%+v) = %v, want %v", tc.s, got, tc.want)
		}
	}
}
