// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (NOT generated): tactical-rmm handfix alerts-sync-reads-alert-list.

package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// The real sync against a fake Tactical RMM: GET /alerts/templates/ answers two
// alert templates (what the generated "alerts" partition walks), PATCH /alerts/
// answers the alert list in AlertSerializer shape (amidaware/tacticalrmm
// alerts/views.py GetAddAlerts.patch). One alert is active, one resolved, one
// snoozed: the reports must count exactly one active alert, by severity too.
func TestAlertReportsReadTheAlertList(t *testing.T) {
	novelTestEnv(t)
	now := time.Now().UTC().Format(time.RFC3339)
	patched := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/alerts/templates/":
			_, _ = w.Write([]byte(`[{"id":1,"name":"Default","is_active":true},{"id":2,"name":"Servers","is_active":true}]`))
		case r.Method == http.MethodPatch && r.URL.Path == "/alerts/":
			patched++
			_, _ = w.Write([]byte(`[
 {"id":11,"severity":"error","resolved":false,"snoozed":false,"alert_type":"availability","client":"Acme","hostname":"ws1","alert_time":"` + now + `"},
 {"id":12,"severity":"warning","resolved":true,"snoozed":false,"alert_type":"check","client":"Acme","hostname":"ws2","alert_time":"` + now + `"},
 {"id":13,"severity":"warning","resolved":false,"snoozed":true,"alert_type":"check","client":"Acme","hostname":"ws3","alert_time":"` + now + `"}]`))
		case r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`[]`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer srv.Close()
	t.Setenv("TACTICAL_RMM_BASE_URL", srv.URL)
	t.Setenv("TRMM_API_KEY", "test-key")

	if _, stderr, err := runNovel(t, "sync", "--resources", "agents,alerts"); err != nil {
		t.Fatalf("sync: %v\n%s", err, stderr)
	}
	if patched != 1 {
		t.Fatalf("sync fetched the alert list %d times, want 1 (PATCH /alerts/)", patched)
	}

	out, stderr, err := runNovel(t, "fleet", "health")
	if err != nil {
		t.Fatalf("fleet health: %v\n%s", err, stderr)
	}
	var fh struct {
		Alerts struct {
			Active     int            `json:"active"`
			BySeverity map[string]int `json:"by_severity"`
		} `json:"alerts"`
	}
	assertJSON(t, out, &fh)
	if fh.Alerts.Active != 1 || fh.Alerts.BySeverity["error"] != 1 || fh.Alerts.BySeverity["warning"] != 0 {
		t.Errorf("fleet health alerts = %+v, want active 1 = error 1, warning 0 (resolved and snoozed are not active)", fh.Alerts)
	}

	out, stderr, err = runNovel(t, "alerts", "digest", "--since", "24h", "--by", "severity")
	if err != nil {
		t.Fatalf("alerts digest: %v\n%s", err, stderr)
	}
	var digest []struct {
		Group  string `json:"group"`
		Total  int    `json:"total"`
		Active int    `json:"active"`
	}
	assertJSON(t, out, &digest)
	got := map[string][2]int{}
	for _, d := range digest {
		got[d.Group] = [2]int{d.Total, d.Active}
	}
	if got["error"] != [2]int{1, 1} || got["warning"] != [2]int{2, 0} || len(got) != 2 {
		t.Errorf("alerts digest = %v, want error 1/1 and warning 2/0", got)
	}

	out, _, err = runNovel(t, "since", "24h")
	if err != nil {
		t.Fatalf("since: %v", err)
	}
	var since struct {
		NewAlerts []map[string]any `json:"new_alerts"`
	}
	assertJSON(t, out, &since)
	if len(since.NewAlerts) != 3 {
		b, _ := json.Marshal(since.NewAlerts)
		t.Errorf("since new_alerts = %s, want the 3 alerts raised in the window", b)
	}
}
