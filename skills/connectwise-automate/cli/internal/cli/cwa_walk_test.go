// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// The fleet reports read only what the last finished, unfiltered sync walk of
// each resource returned. These tests drive the real sync command against a
// loopback fake Automate server (review class "local mirror read as
// complete"): a resumed walk keeps its earlier pages, a live read after the
// walk is not current, and a filtered walk is never taken for the whole fleet.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"connectwise-automate-pp-cli/internal/cliutil/testenv"
	"connectwise-automate-pp-cli/internal/store"
)

// ageMirror backdates every stored row, standing in for the minutes between
// two operator runs (a later walk then starts well after these rows landed).
func ageMirror(t *testing.T) {
	t.Helper()
	s, err := store.Open(defaultDBPath("connectwise-automate-cli"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.DB().Exec(`UPDATE resources SET synced_at = ?`, time.Now().UTC().Add(-10*time.Minute).Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
}

// cwaFake serves /Computers, /Clients and /Alerts as page-numbered lists
// (page, pagesize) and /Alerts/{id} as a live read. A condition of
// Status='Online' filters computers the way the Automate API does.
type cwaFake struct {
	mu        sync.Mutex
	computers []map[string]any
	clients   []map[string]any
	alerts    []map[string]any
	alertByID map[string]map[string]any
}

func (f *cwaFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	path := r.URL.Path[strings.Index(r.URL.Path, "/cwa/api/v1")+len("/cwa/api/v1"):]
	if strings.HasPrefix(path, "/Alerts/") {
		a, ok := f.alertByID[strings.TrimPrefix(path, "/Alerts/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(a)
		return
	}
	var list []map[string]any
	switch path {
	case "/Computers":
		for _, c := range f.computers {
			if r.URL.Query().Get("condition") == "Status='Online'" && c["Status"] != "Online" {
				continue
			}
			list = append(list, c)
		}
	case "/Clients":
		list = f.clients
	case "/Alerts":
		list = f.alerts
	default:
		_, _ = w.Write([]byte(`[]`))
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("pagesize"))
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 100
	}
	lo, hi := (page-1)*size, page*size
	if lo > len(list) {
		lo = len(list)
	}
	if hi > len(list) {
		hi = len(list)
	}
	out := list[lo:hi]
	if out == nil {
		out = []map[string]any{}
	}
	_ = json.NewEncoder(w).Encode(out)
}

func startCWAFake(t *testing.T, f *cwaFake) {
	t.Helper()
	testenv.Isolate(t)
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	t.Setenv("CONNECTWISE_AUTOMATE_BASE_URL", "http://{server}/cwa/api/v1")
	t.Setenv("CONNECTWISE_AUTOMATE_SERVER", strings.TrimPrefix(srv.URL, "http://"))
	t.Setenv("CONNECTWISE_AUTOMATE_TOKEN", "test-secret")
	t.Setenv("CONNECTWISE_AUTOMATE_CLIENT_ID", "00000000-0000-0000-0000-000000000001")
}

func cwaRoot(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := RootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func mustCWA(t *testing.T, args ...string) string {
	t.Helper()
	out, err := cwaRoot(t, args...)
	if err != nil {
		t.Fatalf("%v: %v\n%s", args, err, out)
	}
	return out
}

func cwaComputer(id, client int, status string) map[string]any {
	return map[string]any{"Id": id, "ComputerName": fmt.Sprintf("PC-%d", id), "ClientId": client, "ClientName": "Acme", "Status": status}
}

// A walk capped by --max-pages and resumed by a plain sync covered every
// page: the report counts the first page too.
func TestWalkResumedAfterCapKeepsEveryPage(t *testing.T) {
	f := &cwaFake{clients: []map[string]any{{"Id": 1, "Name": "Acme"}}}
	for i := 1; i <= 100; i++ {
		f.computers = append(f.computers, cwaComputer(i, 1, "Online"))
	}
	f.computers = append(f.computers, cwaComputer(101, 1, "Offline"))
	startCWAFake(t, f)
	mustCWA(t, "sync", "--resources", "clients,alerts", "--json")
	mustCWA(t, "sync", "--resources", "computers", "--max-pages", "1", "--json")
	ageMirror(t)
	mustCWA(t, "sync", "--resources", "computers", "--json")
	out := nospace(mustCWA(t, "fleet-health", "--json"))
	if !strings.Contains(out, `"computers":101`) || !strings.Contains(out, `"offline":1`) {
		t.Fatalf("a resumed walk lost its first page: %s", out)
	}
}

// A live read after a finished walk writes its record through to the store;
// the walk never returned it, so the reports do not list it.
func TestLiveReadAfterWalkIsNotReportedAsOpen(t *testing.T) {
	f := &cwaFake{
		clients:   []map[string]any{{"Id": 1, "Name": "Acme"}},
		computers: []map[string]any{cwaComputer(1, 1, "Online")},
		alertByID: map[string]map[string]any{"7": {"Id": 7, "ComputerId": 1, "ClientName": "Acme", "Status": "Closed", "Priority": 4, "Message": "resolved disk alert"}},
	}
	startCWAFake(t, f)
	mustCWA(t, "sync", "--resources", "clients,computers,alerts", "--json")
	mustCWA(t, "alerts", "get", "7", "--no-cache", "--json")
	for _, report := range []string{"alert-triage", "fleet-health", "client-rollup"} {
		out := mustCWA(t, report, "--json")
		if strings.Contains(out, "resolved disk alert") || strings.Contains(nospace(out), `"open_alerts":1`) {
			t.Fatalf("%s reported a live read's write-through as an open alert: %s", report, out)
		}
	}
}

// A finished walk filtered by --resource-param is not the whole fleet: the
// offline computer the earlier unfiltered walk returned still counts.
func TestFilteredWalkIsNotTheWholeFleet(t *testing.T) {
	f := &cwaFake{
		clients:   []map[string]any{{"Id": 1, "Name": "Acme"}},
		computers: []map[string]any{cwaComputer(1, 1, "Online"), cwaComputer(2, 1, "Offline")},
	}
	startCWAFake(t, f)
	mustCWA(t, "sync", "--resources", "clients,computers,alerts", "--json")
	ageMirror(t)
	mustCWA(t, "sync", "--resources", "computers", "--resource-param", "computers:condition=Status='Online'", "--json")
	out := nospace(mustCWA(t, "fleet-health", "--json"))
	if !strings.Contains(out, `"computers":2`) || !strings.Contains(out, `"offline":1`) {
		t.Fatalf("a filtered walk was read as the whole fleet: %s", out)
	}
}

// Only filtered walks ever ran: no walk proves what the fleet is, so the
// report refuses instead of reading the filtered rows as everything.
func TestOnlyFilteredWalksRefuse(t *testing.T) {
	f := &cwaFake{
		clients:   []map[string]any{{"Id": 1, "Name": "Acme"}},
		computers: []map[string]any{cwaComputer(1, 1, "Online"), cwaComputer(2, 1, "Offline")},
	}
	startCWAFake(t, f)
	mustCWA(t, "sync", "--resources", "clients,alerts", "--json")
	mustCWA(t, "sync", "--resources", "computers", "--resource-param", "computers:condition=Status='Online'", "--json")
	if out, err := cwaRoot(t, "fleet-health", "--json"); err == nil || !strings.Contains(err.Error(), "computers") {
		t.Fatalf("fleet-health over filtered-only computers: err=%v out=%s", err, out)
	}
}

// A computer the API stopped returning (retired) is not counted after the
// next finished walk, although a plain sync never prunes its row.
func TestWalkDropsRecordsTheAPIStoppedReturning(t *testing.T) {
	f := &cwaFake{
		clients:   []map[string]any{{"Id": 1, "Name": "Acme"}},
		computers: []map[string]any{cwaComputer(1, 1, "Online"), cwaComputer(2, 1, "Offline")},
	}
	startCWAFake(t, f)
	mustCWA(t, "sync", "--resources", "clients,computers,alerts", "--json")
	f.mu.Lock()
	f.computers = f.computers[:1]
	f.mu.Unlock()
	ageMirror(t)
	mustCWA(t, "sync", "--resources", "computers", "--json")
	out := nospace(mustCWA(t, "fleet-health", "--json"))
	if !strings.Contains(out, `"computers":1`) || !strings.Contains(out, `"offline":0`) {
		t.Fatalf("a retired computer still counted: %s", out)
	}
}
