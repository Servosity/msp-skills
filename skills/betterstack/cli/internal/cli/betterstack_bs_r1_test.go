// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Codex review round 1 of the betterstack 4.34.2 reprint: the wrong-result
// classes on Better Stack's documented shapes (spec.yaml). Each test was seen
// failing on the round-1 code (seen/r1-<class>-seen-failing.log in the run dir).

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"betterstack-pp-cli/internal/cliutil/testenv"
	"betterstack-pp-cli/internal/store"
)

func bsR1Page(items []string, next string) string {
	n := "null"
	if next != "" {
		n = strconv.Quote(next)
	}
	return `{"data":[` + strings.Join(items, ",") + `],"pagination":{"first":null,"last":null,"prev":null,"next":` + n + `}}`
}

// bsR1Fake is a Better Stack Uptime stand-in whose monitors and incidents a
// test changes between syncs; GET /incidents honors the documented from
// filter (spec.yaml resources.incidents.endpoints.list.params[from]).
type bsR1Fake struct {
	mu        sync.Mutex
	monitors  []string
	incidents []struct{ started, body string }
}

func (f *bsR1Fake) serve(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/monitors"):
			fmt.Fprint(w, bsR1Page(f.monitors, ""))
		case strings.HasSuffix(r.URL.Path, "/incidents"):
			var items []string
			from := r.URL.Query().Get("from")
			for _, in := range f.incidents {
				if from == "" || in.started[:10] >= from[:10] {
					items = append(items, in.body)
				}
			}
			fmt.Fprint(w, bsR1Page(items, ""))
		default:
			fmt.Fprint(w, bsR1Page(nil, ""))
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("BETTERSTACK_BASE_URL", srv.URL+"/api/v2")
	t.Setenv("BETTERSTACK_API_TOKEN", "bsR1UnitToken-7e3c9a41")
	return srv
}

func bsR1Runner(db string) func(args ...string) (string, error) {
	return func(args ...string) (string, error) {
		cmd := RootCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(append(args, "--db", db, "--no-learn"))
		err := cmd.Execute()
		return out.String(), err
	}
}

func bsR1Incident(id string, started time.Time, resolved bool) struct{ started, body string } {
	res := "null"
	if resolved {
		res = strconv.Quote(started.Add(5 * time.Minute).UTC().Format(time.RFC3339))
	}
	s := started.UTC().Format(time.RFC3339)
	return struct{ started, body string }{s, `{"id":"` + id + `","type":"incident","attributes":{"name":"API down ` + id + `","cause":"Timeout","started_at":"` + s + `","acknowledged_at":null,"resolved_at":` + res + `}}`}
}

// Class time-filtered-walk-read-as-complete (Codex r1 [P1] filtered incident
// snapshot read as complete; a new instance of the pre-closed class
// incomplete-snapshot-read-as-complete): `sync --since 7d` sends GET
// /incidents from=<date>, and every later plain sync sent from=<last sync>,
// so an incident that started before the window and is still open was never
// stored, and one stored open was never seen resolved; triage reported them
// wrong. A walk that sends any filter the account's whole set does not pass
// is not a snapshot, and the plain sync walks every incident.
func TestBsR1TimeFilteredSyncIsNotASnapshot(t *testing.T) {
	testenv.Isolate(t)
	now := time.Now()
	f := &bsR1Fake{}
	f.incidents = append(f.incidents, bsR1Incident("7000001", now.AddDate(0, 0, -28), false), bsR1Incident("7000002", now.Add(-time.Hour), true))
	f.serve(t)
	run := bsR1Runner(filepath.Join(t.TempDir(), "data.db"))
	if out, err := run("sync", "--since", "7d"); err != nil {
		t.Fatalf("sync --since: %v\n%s", err, out)
	}
	out, err := run("triage", "--json", "--data-source", "local")
	if err == nil && !strings.Contains(out, "7000001") {
		t.Errorf("after `sync --since 7d`, triage answered without the incident open since before the window: %s", out)
	}
	if out, err := run("sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if out, err := run("triage", "--json", "--data-source", "local"); err != nil || !strings.Contains(out, "7000001") {
		t.Fatalf("after a plain sync, triage must list the open incident: err %v, out %s", err, out)
	}
	f.mu.Lock()
	f.incidents[0] = bsR1Incident("7000001", now.AddDate(0, 0, -28), true) // resolved in Better Stack
	f.mu.Unlock()
	if out, err := run("sync"); err != nil {
		t.Fatalf("second sync: %v\n%s", err, out)
	}
	out, err = run("triage", "--json", "--data-source", "local")
	if err == nil && strings.Contains(out, "7000001") {
		t.Errorf("after the incident was resolved and a plain sync ran, triage still lists it open: %s", out)
	}
	if err != nil {
		t.Errorf("a plain sync is the whole account; triage refused: %v\n%s", err, out)
	}
}

// Class deleted-rows-read-as-current (Codex r1 [P1]): no synced resource
// reconciled deletions (flatReconcileModes was empty), so a monitor deleted in
// Better Stack stayed in the store after every sync, --full included, and
// fleet reported it down. A sync that is the account's whole set removes what
// the account no longer has; one that does not reconcile is not a snapshot.
func TestBsR1DeletedRecordsLeaveTheStore(t *testing.T) {
	const monitor = `{"id":"2085431","type":"monitor","attributes":{"pronounceable_name":"API","url":"https://api.example.com","monitor_type":"status","status":"down","paused":false}}`
	for _, r := range defaultSyncResources() {
		if !resourceIsFlatReconcilable(resourceReconcileMode(r)) {
			t.Errorf("%s is synced but no sync removes what Better Stack deleted (flatReconcileModes)", r)
		}
	}
	for _, c := range []struct {
		name   string
		resync []string
		refuse bool // a sync that does not reconcile: the novels refuse rather than answer from rows it did not confirm
	}{
		{"sync --full", []string{"sync", "--full"}, false},
		{"plain sync", []string{"sync"}, false},
		{"sync --resources monitors", []string{"sync", "--resources", "monitors"}, false},
		{"sync --full --no-prune", []string{"sync", "--full", "--no-prune"}, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			testenv.Isolate(t)
			f := &bsR1Fake{monitors: []string{monitor}}
			f.serve(t)
			run := bsR1Runner(filepath.Join(t.TempDir(), "data.db"))
			if out, err := run("sync"); err != nil {
				t.Fatalf("sync: %v\n%s", err, out)
			}
			if out, err := run("fleet", "--json", "--data-source", "local"); err != nil || !strings.Contains(out, `"down": 1`) && !strings.Contains(out, `"down":1`) {
				t.Fatalf("fleet must report the down monitor first: err %v, out %s", err, out)
			}
			f.mu.Lock()
			f.monitors = nil // deleted in Better Stack
			f.mu.Unlock()
			if out, err := run(c.resync...); err != nil {
				t.Fatalf("%v: %v\n%s", c.resync, err, out)
			}
			out, err := run("fleet", "--json", "--data-source", "local")
			switch {
			case c.refuse && err == nil:
				t.Errorf("%s does not remove deleted records, but fleet answered from them: %s", c.name, out)
			case !c.refuse && (err != nil || strings.Contains(out, `"down": 1`) || strings.Contains(out, `"down":1`)):
				t.Errorf("after %s the deleted monitor must be gone: err %v, out %s", c.name, err, out)
			}
		})
	}
}

func bsR1Seed(t *testing.T, recs map[string][]string) string {
	t.Helper()
	db := filepath.Join(t.TempDir(), "data.db")
	s, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	for typ, items := range recs {
		for _, it := range items {
			var probe struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal([]byte(it), &probe); err != nil {
				t.Fatal(err)
			}
			if err := s.Upsert(typ, probe.ID, []byte(it)); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, r := range defaultSyncResources() {
		if err := s.SaveSyncState(r, "", len(recs[r])); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	return db
}

// Class heartbeat-incidents-omitted (Codex r1 [P1]): an incident's source was
// read as a monitor only (spec.yaml types.Incident.fields carries monitor_id
// and heartbeat_id; the API also links it under relationships), so a
// heartbeat's open incident counted nowhere: group-health reported
// open_incidents 0 for its heartbeat group, statuspage-audit skipped heartbeat
// entries, and down said there were no open incidents.
func TestBsR1HeartbeatIncidentsAreCounted(t *testing.T) {
	testenv.Isolate(t)
	started := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	db := bsR1Seed(t, map[string][]string{
		"heartbeat-groups": {`{"id":"6000001","type":"heartbeat_group","attributes":{"name":"Client A","paused":false}}`},
		"heartbeats": {
			`{"id":"7000001","type":"heartbeat","attributes":{"name":"backup job","status":"down","period":3600,"grace":300,"paused":false,"heartbeat_group_id":6000001}}`,
			`{"id":"7000002","type":"heartbeat","attributes":{"name":"report job","status":"up","period":3600,"grace":300,"paused":false,"heartbeat_group_id":6000001}}`,
		},
		"incidents": {
			`{"id":"8000001","type":"incident","attributes":{"name":"backup job","started_at":"` + started + `","acknowledged_at":null,"resolved_at":null,"heartbeat_id":7000001}}`,
			`{"id":"8000002","type":"incident","attributes":{"name":"backup job","started_at":"` + started + `","acknowledged_at":null,"resolved_at":null},"relationships":{"heartbeat":{"data":{"id":"7000002","type":"heartbeat"}}}}`,
		},
		"status-pages": {`{"id":"184620","type":"status_page","attributes":{"company_name":"Client A","subdomain":"clienta","aggregate_state":"operational"}}`},
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, bsR1Page([]string{`{"id":"1","type":"status_page_resource","attributes":{"public_name":"Backups","resource_type":"Heartbeat","resource_id":7000001}}`}, ""))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("BETTERSTACK_BASE_URL", srv.URL+"/api/v2")
	t.Setenv("BETTERSTACK_API_TOKEN", "bsR1UnitToken-hb51c2")
	run := bsR1Runner(db)

	out, err := run("group-health", "--json", "--data-source", "local")
	if err != nil {
		t.Fatalf("group-health: %v\n%s", err, out)
	}
	var doc any
	_ = json.Unmarshal([]byte(out), &doc)
	found := false
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if x["group_id"] == "6000001" && x["kind"] == "heartbeat-group" {
				found = true
				if n, _ := x["open_incidents"].(float64); n != 2 {
					t.Errorf("heartbeat group 6000001 has two open heartbeat incidents, group-health says %v", x["open_incidents"])
				}
			}
			for _, c := range x {
				walk(c)
			}
		case []any:
			for _, c := range x {
				walk(c)
			}
		}
	}
	walk(doc)
	if !found {
		t.Errorf("group-health has no row for heartbeat group 6000001: %s", out)
	}
	if out, _ := run("down", "--data-source", "local"); strings.Contains(out, "no open incidents") {
		t.Errorf("down says there are no open incidents while two heartbeat incidents are open: %s", out)
	}
	// Two heartbeats whose incidents carry one name are two sources.
	if out, err := run("flapping", "--json", "--data-source", "local"); err != nil || !strings.Contains(out, "heartbeat:7000001") || !strings.Contains(out, "heartbeat:7000002") {
		t.Errorf("flapping must key heartbeat incidents by heartbeat, not by name: err %v, out %s", err, out)
	}
	if out, err := run("statuspage-audit", "--json"); err != nil || !strings.Contains(out, `"drift"`) {
		t.Errorf("an operational page lists heartbeat 7000001 with an open incident; statuspage-audit found no drift: err %v, out %s", err, out)
	}
}

// Class incomplete-paging-read-as-complete (Codex r1 [P1]; a new instance of
// the pre-closed class export-first-page-only): statuspage-audit read one page
// of GET /status-pages/{id}/resources (per_page 250) and said "No status-page
// drift found ... (live check: ok)" for a page whose 251st resource is a
// paused monitor (spec.yaml resources.status-page-resources.endpoints.list;
// pagination.next on page 1). Every page is read until pagination.next is
// null.
func TestBsR1StatuspageAuditWalksEveryResourcePage(t *testing.T) {
	testenv.Isolate(t)
	var monitors []string
	for i := 0; i < 251; i++ {
		paused := "false"
		if i == 250 {
			paused = "true"
		}
		monitors = append(monitors, fmt.Sprintf(`{"id":"%d","type":"monitor","attributes":{"pronounceable_name":"m%d","status":"up","paused":%s}}`, 3000000+i, i, paused))
	}
	db := bsR1Seed(t, map[string][]string{
		"monitors":     monitors,
		"status-pages": {`{"id":"184620","type":"status_page","attributes":{"company_name":"Client B","subdomain":"clientb","aggregate_state":"operational"}}`},
	})
	var srvURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		page := r.URL.Query().Get("page")
		var items []string
		next := ""
		if page == "" || page == "1" {
			for i := 0; i < 250; i++ {
				items = append(items, fmt.Sprintf(`{"id":"%d","type":"status_page_resource","attributes":{"public_name":"m%d","resource_type":"Monitor","resource_id":%d}}`, 9000000+i, i, 3000000+i))
			}
			next = srvURL + "/api/v2/status-pages/184620/resources?page=2&per_page=250"
		} else if page == "2" {
			items = append(items, `{"id":"9000250","type":"status_page_resource","attributes":{"public_name":"m250","resource_type":"Monitor","resource_id":3000250}}`)
		}
		fmt.Fprint(w, bsR1Page(items, next))
	}))
	srvURL = srv.URL
	t.Cleanup(srv.Close)
	t.Setenv("BETTERSTACK_BASE_URL", srv.URL+"/api/v2")
	t.Setenv("BETTERSTACK_API_TOKEN", "bsR1UnitToken-pg2b7d")
	out, err := bsR1Runner(db)("statuspage-audit")
	if err != nil || !strings.Contains(out, "paused") || strings.Contains(out, "No status-page drift found") {
		t.Errorf("the page's 251st resource is a paused monitor on page two; statuspage-audit missed it: err %v\n%s", err, out)
	}
}
