// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// spStub is a Service Provider (ACC-SP) with two subscribers. fail names the
// reads that answer 403: "b-endpoint", "b-firebox", "incidents", "names",
// "all-products", "unallocated".
func spStub(t *testing.T, fail map[string]bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		deny := func() { w.WriteHeader(http.StatusForbidden); _, _ = w.Write([]byte(`{"message":"no permission"}`)) }
		p := r.URL.Path
		switch {
		case p == "/oauth/token":
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
		case strings.HasSuffix(p, "/audiences"):
			body, _ := io.ReadAll(r.Body)
			var in struct {
				AccountID string `json:"accountId"`
			}
			_ = json.Unmarshal(body, &in)
			_, _ = w.Write([]byte(`{"audience":"aud-` + in.AccountID + `"}`))
		case strings.HasSuffix(p, "/ACC-SP/children"):
			if fail["names"] {
				deny()
				return
			}
			_, _ = w.Write([]byte(`{"items":[{"accountId":"ACC-A","name":"Acme"},{"accountId":"ACC-B","name":"Beta"}],"pageControls":{"totalItems":2}}`))
		case strings.Contains(p, "/firebox/management/v1/info/"):
			if fail["all-products"] || (fail["b-firebox"] && strings.Contains(p, "ACC-B")) {
				deny()
				return
			}
			if strings.Contains(p, "ACC-SP") {
				_, _ = w.Write([]byte(`{"data":[{"id":"FB-1","serial_number":"SER-A","state":"online"}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"count":1,"data":[{"id":"FB","state":"online"}]}`))
		case strings.HasSuffix(p, "/devicesprotectionstatus"):
			if fail["all-products"] || (fail["b-endpoint"] && strings.Contains(p, "ACC-B")) {
				deny()
				return
			}
			if strings.Contains(p, "ACC-A") {
				// ACC-A has an at-risk endpoint, so its score beats a zeroed ACC-B.
				_, _ = w.Write([]byte(`{"data":[{"host_name":"A1","accumulated_protection_status":2,"isolation_state":1}],"total_items":1}`))
				return
			}
			_, _ = w.Write([]byte(`{"data":[],"total_items":0}`))
		case strings.HasSuffix(p, "/incidents"):
			if fail["incidents"] {
				deny()
				return
			}
			_, _ = w.Write([]byte(`{"items":[{"id":"i1","account":"ACC-A","severity":"Critical","status":"new","timestamp":"2026-10-13T10:00:00Z"}],"count":1}`))
		case strings.Contains(p, "/assets/summary/firebox"):
			if fail["unallocated"] && r.URL.Query().Get("allocationStatus") == "unallocated" {
				deny()
				return
			}
			_, _ = w.Write([]byte(`{"data":[],"pageControls":{"totalItems":0}}`))
		default:
			_, _ = w.Write([]byte(`{"data":[],"items":[],"pageControls":{"totalItems":0}}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func spEnv(t *testing.T, srv *httptest.Server) {
	t.Helper()
	isolateAuthEnv(t)
	t.Setenv("WATCHGUARD_HOME", t.TempDir())
	t.Setenv("WATCHGUARD_BASE_URL", srv.URL)
	t.Setenv("WATCHGUARD_TOKEN_URL", srv.URL+"/oauth/token")
	t.Setenv("WATCHGUARD_CLIENT_ID", "AID")
	t.Setenv("WATCHGUARD_CLIENT_SECRET", "PW")
	t.Setenv("WATCHGUARD_API_KEY", "KEY")
	t.Setenv("WATCHGUARD_ACCOUNT_ID", "ACC-SP")
	t.Setenv("WATCHGUARD_NO_LEARN", "1")
}

func runJSON(t *testing.T, args ...string) (map[string]any, int) {
	t.Helper()
	cmd := RootCmd()
	cmd.SetArgs(append(args, "--rate-limit", "0"))
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	err := cmd.Execute()
	var doc map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &doc)
	if err == nil {
		return doc, 0
	}
	return doc, ExitCode(err)
}

func healthRowFor(doc map[string]any, id string) map[string]any {
	rows, _ := doc["accounts"].([]any)
	for _, r := range rows {
		if m, _ := r.(map[string]any); m["account_id"] == id {
			return m
		}
	}
	return nil
}

// hand-fix (handfixes.json fanout-partial-data-exit): accounts health never
// shows a failed read as 0, never ranks an incomplete row as healthier than a
// complete one, and exits 6 (5 when nothing about any account was read).
func TestAccountsHealthFailedReadsArePartial(t *testing.T) {
	spEnv(t, spStub(t, map[string]bool{"b-endpoint": true}))
	doc, code := runJSON(t, "accounts", "health", "--json", "--concurrency", "1")
	if code != 6 {
		t.Fatalf("one failed product read: exit %d, want 6", code)
	}
	b := healthRowFor(doc, "ACC-B")
	if b == nil || b["endpoints_total"] != nil || b["endpoints_at_risk"] != nil || b["complete"] != false {
		t.Fatalf("ACC-B's failed endpoint read is not null/flagged: %v", b)
	}
	if first := doc["accounts"].([]any)[0].(map[string]any); first["account_id"] != "ACC-B" {
		t.Fatalf("incomplete ACC-B ranked below a complete row: %v", doc["accounts"])
	}
	if _, code := runJSON(t, "accounts", "health", "--json", "--allow-partial-failure"); code != 0 {
		t.Fatalf("--allow-partial-failure: exit %d, want 0", code)
	}

	spEnv(t, spStub(t, map[string]bool{"incidents": true}))
	doc, code = runJSON(t, "accounts", "health", "--json")
	if a := healthRowFor(doc, "ACC-A"); code != 6 || a == nil || a["incidents_open"] != nil || a["complete"] != false {
		t.Fatalf("failed incident lookup: exit %d, ACC-A %v (want 6, incidents_open null)", code, a)
	}

	spEnv(t, spStub(t, map[string]bool{"all-products": true}))
	if _, code := runJSON(t, "accounts", "health", "--json"); code != 5 {
		t.Fatalf("no product read succeeded for any account: exit %d, want 5", code)
	}

	spEnv(t, spStub(t, nil))
	doc, code = runJSON(t, "accounts", "health", "--json")
	if a := healthRowFor(doc, "ACC-A"); code != 0 || a == nil || a["complete"] != true || a["endpoints_at_risk"] != float64(1) {
		t.Fatalf("all reads ok: exit %d, ACC-A %v", code, a)
	}
}

// The same rule in reconcile (a failed lookup) and queue (the account-name
// lookup): exit 6, --allow-partial-failure exits 0.
func TestReconcileAndQueueFailedLookupsArePartial(t *testing.T) {
	spEnv(t, spStub(t, map[string]bool{"unallocated": true}))
	if _, code := runJSON(t, "allocations", "reconcile", "--types", "endPoint", "--json"); code != 6 {
		t.Fatalf("reconcile with a failed lookup: exit %d, want 6", code)
	}
	if _, code := runJSON(t, "allocations", "reconcile", "--types", "endPoint", "--json", "--allow-partial-failure"); code != 0 {
		t.Fatalf("reconcile --allow-partial-failure: exit %d, want 0", code)
	}
	spEnv(t, spStub(t, map[string]bool{"names": true}))
	if _, code := runJSON(t, "threatsync-incidents", "queue", "--json"); code != 6 {
		t.Fatalf("queue with a failed name lookup: exit %d, want 6", code)
	}
}
