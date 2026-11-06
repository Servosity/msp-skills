// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Class incomplete-snapshot-read-as-complete (Codex r2, projected snapshot):
// a sync narrowed by a query parameter (the documented fields[] projection,
// conditions[] filters, --param, --global-param, a since window) finished,
// so sync_state read it as complete, and a store-backed novel then answered
// from the rows and fields the narrowing left behind. These tests drive the
// real `sync` command against a local TLS API that serves Liongard's
// documented list shapes (spec.yaml, components/schemas/*).

import (
	"bytes"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"liongard-pp-cli/internal/cliutil"
	"liongard-pp-cli/internal/cliutil/testenv"
	"liongard-pp-cli/internal/store"
)

const snapshotTestKey = "snapshotUnitLiongardKey-4c9e2a7b1d"

// markFullSnapshot records each resource as a finished full sync, through
// the state sequence the sync walk runs: bind the scope, mark the attempt
// started, finish with the full-snapshot verdict.
func markFullSnapshot(t *testing.T, db *store.Store, resources ...string) {
	t.Helper()
	for _, rt := range resources {
		if _, _, _, err := db.BindSyncScope(rt, ""); err != nil {
			t.Fatalf("bind %s: %v", rt, err)
		}
		if err := db.SaveSyncProgressInScope(rt, "", "", 0); err != nil {
			t.Fatalf("start %s: %v", rt, err)
		}
		if err := db.SaveSyncSnapshotAtInScope(rt, "", "", 0, time.Now(), true); err != nil {
			t.Fatalf("finish %s: %v", rt, err)
		}
	}
}

// snapshotAPI is a local Liongard v1 API. routes maps a list path (after
// /api/v1) to a handler that renders the body for the request's query.
type snapshotAPI struct {
	mu       sync.Mutex
	routes   map[string]func(r *http.Request) string
	requests []string
}

func (a *snapshotAPI) set(path string, h func(r *http.Request) string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.routes[path] = h
}

func (a *snapshotAPI) seen() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.requests...)
}

// startSnapshotAPI starts the API over TLS (the client refuses to send a
// credential over plain http), trusts its certificate through
// LIONGARD_CA_CERT, and points a sandboxed CLI at it.
func startSnapshotAPI(t *testing.T) *snapshotAPI {
	t.Helper()
	api := &snapshotAPI{routes: map[string]func(r *http.Request) string{}}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-ROAR-API-KEY") != snapshotTestKey {
			http.Error(w, `{"message":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/api/v1")
		api.mu.Lock()
		api.requests = append(api.requests, path+"?"+r.URL.RawQuery)
		h := api.routes[path]
		api.mu.Unlock()
		if h == nil {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		// A generous advertised budget: the client paces an unknown server
		// at 2 requests a second until its X-Ratelimit headers name one, and
		// these tests run dozens of multi-resource syncs. Nothing here
		// asserts pacing.
		w.Header().Set("X-Ratelimit-Remaining", "1000000")
		w.Header().Set("X-Ratelimit-Reset", "60")
		_, _ = w.Write([]byte(h(r)))
	}))
	t.Cleanup(srv.Close)
	testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	caPath := filepath.Join(t.TempDir(), "api-ca.pem")
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	if err := os.WriteFile(caPath, pemBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LIONGARD_CA_CERT", caPath)
	t.Setenv("LIONGARD_BASE_URL", srv.URL+"/api/v1")
	t.Setenv("LIONGARD_API_KEY", snapshotTestKey)
	t.Setenv("LIONGARD_ACCESS_KEY_ID", "")
	t.Setenv("LIONGARD_ACCESS_KEY_SECRET", "")
	t.Setenv("LIONGARD_ENDPOINTS_SESSION_TOKEN_AUTH", "")
	return api
}

// runSnapshotCLI runs one CLI invocation in-process and returns its stdout.
func runSnapshotCLI(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := RootCmd()
	cmd.SetArgs(append(append([]string(nil), args...), "--no-learn"))
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

func mustRunSnapshotCLI(t *testing.T, args ...string) string {
	t.Helper()
	out, errOut, err := runSnapshotCLI(t, args...)
	if err != nil {
		t.Fatalf("liongard-cli %s: %v\nstdout: %s\nstderr: %s", strings.Join(args, " "), err, out, errOut)
	}
	return out
}

func jsonArray(t *testing.T, rows []map[string]any) string {
	t.Helper()
	b, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// documentedAgent is one agent in the components/schemas/Agents shape.
func documentedAgent(id int, envID int, heartbeat time.Time) map[string]any {
	return map[string]any{
		"ID":               id,
		"Environment":      map[string]any{"ID": envID, "Name": fmt.Sprintf("Env %d", envID)},
		"UID":              fmt.Sprintf("uid-%d", id),
		"Name":             fmt.Sprintf("Agent %d", id),
		"Description":      "Main Agent",
		"Status":           1,
		"Hostname":         fmt.Sprintf("HOST-%d", id),
		"Platform":         "windows",
		"Suspended":        false,
		"Managed":          false,
		"LastHeartbeat":    heartbeat.UTC().Format("2006-01-02T15:04:05.000Z"),
		"CreatedOn":        "2021-09-10T19:21:03.195Z",
		"UpdatedOn":        nil,
		"UpdatesCheckedOn": nil,
		"Type":             "customer-on-prem",
		"SAAS":             false,
	}
}

// documentedEnvironment is one environment in the components/schemas/Environments shape.
func documentedEnvironment(id int) map[string]any {
	return map[string]any{
		"ID":                id,
		"ServiceProviderID": 7,
		"Name":              fmt.Sprintf("Env %d", id),
		"ShortName":         fmt.Sprintf("E%d", id),
		"Description":       "",
		"Status":            "Active",
		"Visible":           true,
		"CreatedOn":         "2021/09/10 19:21:03",
		"UpdatedOn":         nil,
		"Tier":              "Core",
	}
}

// projectFields applies the documented fields[] projection to a row.
func projectFields(row map[string]any, fields []string) map[string]any {
	out := map[string]any{}
	for _, f := range fields {
		if v, ok := row[f]; ok {
			out[f] = v
		}
	}
	return out
}

// projectingList renders rows, applying a fields[] projection when the
// request carries one (the documented /agents GET parameters.fields[]).
func projectingList(t *testing.T, rows []map[string]any) func(r *http.Request) string {
	return func(r *http.Request) string {
		fields := r.URL.Query()["fields[]"]
		if len(fields) == 0 {
			return jsonArray(t, rows)
		}
		projected := make([]map[string]any, 0, len(rows))
		for _, row := range rows {
			projected = append(projected, projectFields(row, fields))
		}
		return jsonArray(t, projected)
	}
}

// The failing input from codex-r2.md: on a fresh store, sync agents with the
// documented fields[] projection (the API answers [{"ID":1511}]) and the
// environments in full, then sweep for offline agents. Agent 1511's
// LastHeartbeat is days old, so a zero count is a wrong answer: the sweep
// must refuse and name the remedy. After the remedy sync, it answers.
func TestSnapshotProjectedSyncRefusesAgentsOffline(t *testing.T) {
	api := startSnapshotAPI(t)
	agents := []map[string]any{documentedAgent(1511, 42, time.Now().Add(-72*time.Hour))}
	api.set("/agents", projectingList(t, agents))
	api.set("/environments", func(r *http.Request) string {
		return jsonArray(t, []map[string]any{documentedEnvironment(42)})
	})

	mustRunSnapshotCLI(t, "sync", "--resources", "agents,environments", "--resource-param", "agents:fields[]=ID", "--json")
	sawProjection := false
	for _, req := range api.seen() {
		if strings.HasPrefix(req, "/agents?") && strings.Contains(req, "fields") {
			sawProjection = true
		}
	}
	if !sawProjection {
		t.Fatalf("the projected agents request was never made: %v", api.seen())
	}

	out, errOut, err := runSnapshotCLI(t, "agents", "offline", "--agent")
	if err == nil {
		t.Fatalf("agents offline answered from a fields[]-projected agents sync (agent 1511 last heartbeat 3 days ago):\n%s", out)
	}
	msg := err.Error() + errOut
	for _, want := range []string{"agents", "sync --full --resources agents"} {
		if !strings.Contains(msg, want) {
			t.Errorf("refusal does not name %q: %v", want, msg)
		}
	}

	// The remedy: an unnarrowed sync of the resource makes the sweep answer.
	mustRunSnapshotCLI(t, "sync", "--full", "--resources", "agents", "--json")
	out = mustRunSnapshotCLI(t, "agents", "offline", "--agent")
	var got struct {
		Count  int `json:"count"`
		Agents []struct {
			AgentID string `json:"agent_id"`
		} `json:"agents"`
	}
	decodeNovelResult(t, out, &got)
	if got.Count != 1 || len(got.Agents) != 1 || got.Agents[0].AgentID != "1511" {
		t.Fatalf("after a full sync, agent 1511 (heartbeat 3 days old) is not offline: %s", out)
	}
}

// decodeNovelResult decodes a novel's --agent output: the result object,
// under "results" when the output carries the local-source envelope.
func decodeNovelResult(t *testing.T, out string, dst any) {
	t.Helper()
	var env struct {
		Results json.RawMessage `json:"results"`
	}
	body := []byte(out)
	if err := json.Unmarshal(body, &env); err == nil && len(env.Results) > 0 {
		body = env.Results
	}
	if err := json.Unmarshal(body, dst); err != nil {
		t.Fatalf("decoding command output: %v\n%s", err, out)
	}
}

// documentedEstate is a small Liongard estate in the documented list shapes
// (spec.yaml components/schemas), keyed by list path, with nMetrics metric
// definitions and nInspectors inspector types (at least 14 of each).
func documentedEstate(nMetrics, nInspectors int) map[string][]map[string]any {
	now := time.Now().UTC()
	slash := func(d time.Duration) string { return now.Add(-d).Format("2006/01/02 15:04:05") }
	iso := func(d time.Duration) string { return now.Add(-d).Format("2006-01-02T15:04:05.000Z") }
	envRef := func(id int) map[string]any { return map[string]any{"ID": id, "Name": fmt.Sprintf("Env %d", id)} }
	inspector := func(id int) map[string]any {
		return map[string]any{
			"ID": id, "Name": fmt.Sprintf("inspector-%03d", id), "Alias": fmt.Sprintf("Inspector %03d", id),
			"Published": true, "Author": "Liongard, Inc.", "PublishedStatus": "production",
			"InspectorCategory": "appservices", "EnhancedBilling": false,
			"DefaultFrequency": map[string]any{"Type": "days", "Interval": 1},
			"CreatedOn":        "2021/09/10 19:21:03", "UpdatedOn": "2021/09/10 19:21:03", "DiagnosticsEnabled": false,
		}
	}
	lpRef := func(id int) map[string]any { return map[string]any{"ID": id, "Alias": fmt.Sprintf("LP %d", id)} }
	launchpoint := func(id, env, insp int, lastScheduled any, totalScheduled int) map[string]any {
		return map[string]any{
			"ID": id, "Environment": envRef(env), "EnvironmentID": env, "Inspector": inspector(insp),
			"Alias": fmt.Sprintf("LP %d", id), "TotalScheduledRuns": totalScheduled, "TotalAdHocRuns": 0,
			"NextScheduledFor": slash(-time.Hour), "LastScheduledRun": lastScheduled, "LastAdHocRun": nil,
			"Enabled": true, "Visible": true, "Status": "Active", "CreatedOn": "2021/09/10 19:21:03", "UpdatedOn": nil,
		}
	}
	system := func(id, env, insp int, lp any) map[string]any {
		return map[string]any{
			"ID": id, "Environment": envRef(env), "Launchpoint": lp, "Inspector": inspector(insp),
			"UniqueCompositeKey": "", "Name": fmt.Sprintf("System %d", id), "Description": "", "Tags": nil,
			"UserDefined": false, "Status": 1, "Visible": true, "LastVerifyTime": slash(time.Hour),
			"CreatedOn": "2021/09/10 19:21:03", "CreatedBy": "Alex Weber", "UpdatedOn": nil, "UpdatedBy": "Alex Weber",
		}
	}
	timeline := func(id, lp, sys, env int, status string, finished time.Duration) map[string]any {
		return map[string]any{
			"ID": id, "Environment": envRef(env), "Launchpoint": lpRef(lp),
			"System": map[string]any{"ID": sys, "Name": fmt.Sprintf("System %d", sys)}, "SystemDetail": map[string]any{"ID": id},
			"Status": status, "AdHoc": false, "ChangeDetections": 0, "Alerts": 0, "Latest": true, "ExitCode": 0, "ExitMessage": "",
			"CreatedOn": slash(finished + 5*time.Minute), "UpdatedOn": nil, "ScheduledAt": slash(finished + 6*time.Minute),
			"ProcessingAt": slash(finished + 4*time.Minute), "RunningAt": slash(finished + 3*time.Minute),
			"FinishedAt": slash(finished), "PayloadSizeBytes": 202285, "Expired": false,
		}
	}
	metric := func(id, insp int) map[string]any {
		return map[string]any{
			"ID": id, "Name": fmt.Sprintf("Metric %03d: Count", id), "UUIDS": fmt.Sprintf("uuid-%03d", id),
			"UCK": fmt.Sprintf("Metric %03d: Count", id), "Description": "", "CreatedOn": "2021-09-10T19:21:03.195Z",
			"UpdatedOn": nil, "ServiceProvider": nil, "Environment": nil, "Inspector": inspector(insp),
			"MetricDisplay": true, "ChangesEnabled": true,
			"MetricVersions": []any{map[string]any{"ID": 100 + id, "InspectorVersionID": 65, "Query": "length(Computers)"}},
			"Rules":          []any{}, "HasActiveAlertRule": false, "BelongsToChangeRule": false,
		}
	}

	estate := map[string][]map[string]any{
		"/environments": {documentedEnvironment(42), documentedEnvironment(43), documentedEnvironment(44)},
		"/agents":       {documentedAgent(1511, 42, now.Add(-72*time.Hour)), documentedAgent(1512, 43, now.Add(-2*time.Minute))},
		"/launchpoints": {
			launchpoint(1203, 42, 13, slash(time.Hour), 12),
			launchpoint(1204, 43, 13, slash(30*24*time.Hour), 40),
			launchpoint(1205, 42, 14, nil, 0),
		},
		"/systems": {
			system(3454, 42, 13, lpRef(1203)),
			system(3455, 43, 13, lpRef(1204)),
			system(3456, 42, 14, nil),
		},
		"/timeline": {
			timeline(1294715, 1203, 3454, 42, "Completed", time.Hour),
			timeline(1294716, 1204, 3455, 43, "Failed", 2*24*time.Hour),
		},
		"/detections": {{
			"ID": 501, "TimelineID": 1294715, "Environment": envRef(42), "Inspector": inspector(13),
			"System": map[string]any{"ID": 3454, "Name": "System 3454"}, "Name": "Admin added",
			"Description": "", "CreatedOn": iso(3 * time.Hour),
		}},
		"/access-keys": {},
		"/tasks":       {},
		"/users":       {},
	}
	for i := 1; i <= nInspectors; i++ {
		estate["/inspectors"] = append(estate["/inspectors"], inspector(i))
	}
	for i := 1; i <= nMetrics; i++ {
		insp := 14
		if i%2 == 1 {
			insp = 13
		}
		estate["/metrics"] = append(estate["/metrics"], metric(i, insp))
	}
	return estate
}

// startDocumentedEstate serves documentedEstate, honoring the documented
// fields[] projection on every list.
func startDocumentedEstate(t *testing.T, nMetrics, nInspectors int) *snapshotAPI {
	t.Helper()
	api := startSnapshotAPI(t)
	for path, rows := range documentedEstate(nMetrics, nInspectors) {
		api.set(path, projectingList(t, rows))
	}
	return api
}

func storeRowCount(t *testing.T, resource string) int {
	t.Helper()
	db, err := store.OpenWithContext(t.Context(), defaultDBPath("liongard-cli"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	n, err := db.Count(resource)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func storeSnapshot(t *testing.T, resource string) store.SyncSnapshot {
	t.Helper()
	db, err := store.OpenWithContext(t.Context(), defaultDBPath("liongard-cli"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	snap, err := db.SyncSnapshotState(resource)
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

// r2-metrics-inspectors-syncable: metrics and inspectors were absent from
// the sync tables, so `sync --resources metrics,inspectors` failed at once
// and metrics pivot / breach and inspectors coverage could only refuse.
// Liongard documents no paging on /metrics or /inspectors (nor on any v1
// list), so they are walked like agents and environments: one request is
// the whole list.
func TestSnapshotMetricsAndInspectorsSyncAndAnswer(t *testing.T) {
	// Long enough to cross the generated 100-item page limit and the
	// store's 200-row default list cap.
	api := startDocumentedEstate(t, 250, 210)

	mustRunSnapshotCLI(t, "sync", "--resources", "metrics,inspectors", "--json")
	for resource, want := range map[string]int{"metrics": 250, "inspectors": 210} {
		if got := storeRowCount(t, resource); got != want {
			t.Errorf("%s: stored %d rows, the API served %d", resource, got, want)
		}
		if snap := storeSnapshot(t, resource); !snap.Complete || !snap.Full {
			t.Errorf("%s: the sync is not recorded as a finished full snapshot: %+v", resource, snap)
		}
	}
	for _, path := range []string{"/metrics", "/inspectors"} {
		n := 0
		for _, req := range api.seen() {
			if strings.HasPrefix(req, path+"?") {
				n++
				if q := strings.TrimPrefix(req, path+"?"); q != "" {
					t.Errorf("%s was requested with query %q; the walk sends none", path, q)
				}
			}
		}
		if n != 1 {
			t.Errorf("%s requested %d times, want 1 (unpaged list)", path, n)
		}
	}

	// The default sync carries both: the three novels need them.
	for _, r := range []string{"metrics", "inspectors"} {
		if !slicesContains(defaultSyncResources(), r) {
			t.Errorf("%s is not in the default sync set", r)
		}
		if !slicesContains(knownSyncResourceNames(), r) {
			t.Errorf("%s is not a known sync resource (--resource-param rejects it)", r)
		}
	}
	mustRunSnapshotCLI(t, "sync", "--resources", "systems,environments,launchpoints", "--json")

	var pivot struct {
		Matched int `json:"matched_definitions"`
		Rows    []struct {
			SystemID string `json:"system_id"`
			HasValue bool   `json:"has_value"`
		} `json:"rows"`
	}
	decodeNovelResult(t, mustRunSnapshotCLI(t, "metrics", "pivot", "Metric 007:", "--agent"), &pivot)
	gotSystems := []string{}
	for _, r := range pivot.Rows {
		gotSystems = append(gotSystems, r.SystemID)
		if r.HasValue {
			t.Errorf("metric definitions carry no value, but row %+v has one", r)
		}
	}
	if pivot.Matched != 1 || strings.Join(gotSystems, ",") != "3454,3455" {
		t.Errorf("metrics pivot Metric 007 (inspector 13): want 1 definition over systems 3454,3455, got %d over %v", pivot.Matched, gotSystems)
	}

	// The definitions carry no evaluated values (Liongard serves those from
	// GET /metrics/bulk), so a threshold scan cannot be answered from the
	// store: it refuses instead of reporting zero breaches.
	out, errOut, err := runSnapshotCLI(t, "metrics", "breach", "Metric 007:", "--op", "gt", "--value", "5", "--agent")
	if err == nil {
		t.Errorf("metrics breach reported a result with no evaluated values in the store: %s", out)
	} else if msg := err.Error() + errOut; !strings.Contains(msg, "metrics evaluation") || !strings.Contains(msg, "2 of the 2 covered system") {
		t.Errorf("metrics breach refusal does not explain the missing values: %v", msg)
	}

	var cov struct {
		Environments int `json:"environments"`
		Count        int `json:"count"`
		Inspectors   []struct {
			InspectorID string `json:"inspector_id"`
			Covered     int    `json:"environments_covered"`
			Missing     int    `json:"environments_missing"`
		} `json:"inspectors"`
	}
	decodeNovelResult(t, mustRunSnapshotCLI(t, "inspectors", "coverage", "--agent"), &cov)
	if cov.Environments != 3 || cov.Count != 210 {
		t.Errorf("inspectors coverage: want 3 environments and 210 inspectors, got %d and %d", cov.Environments, cov.Count)
	}
	for _, r := range cov.Inspectors {
		switch r.InspectorID {
		case "13":
			if r.Covered != 2 || r.Missing != 1 {
				t.Errorf("inspector 13 (launchpoints in envs 42, 43): %+v", r)
			}
		case "14":
			if r.Covered != 1 || r.Missing != 2 {
				t.Errorf("inspector 14 (launchpoint in env 42): %+v", r)
			}
		}
	}
}

// storeBackedNovels is every command that answers from the local store
// (each reads it through loadObjs), with the resources it reads.
var storeBackedNovels = []struct {
	args      []string
	resources []string
	refuses   bool // refuses even on a full snapshot (metrics breach: no values)
}{
	{[]string{"agents", "offline"}, []string{"agents", "environments"}, false},
	{[]string{"coverage"}, []string{"systems", "environments"}, false},
	{[]string{"detections", "failures"}, []string{"timeline", "launchpoints", "environments"}, false},
	{[]string{"drift"}, []string{"detections", "environments"}, false},
	{[]string{"inspectors", "coverage"}, []string{"inspectors", "launchpoints", "environments"}, false},
	{[]string{"environments", "overview", "42"}, []string{"environments", "systems", "launchpoints", "agents", "timeline", "detections"}, false},
	{[]string{"health"}, []string{"launchpoints", "timeline", "environments", "agents", "systems"}, false},
	{[]string{"launchpoints", "run-stale"}, []string{"launchpoints", "timeline", "environments"}, false},
	{[]string{"launchpoints", "stale"}, []string{"launchpoints", "timeline", "environments"}, false},
	{[]string{"metrics", "pivot", "Metric 007:"}, []string{"metrics", "systems", "environments"}, false},
	{[]string{"metrics", "breach", "Metric 007:", "--value", "5"}, []string{"metrics", "systems", "environments"}, true},
	{[]string{"systems", "history", "3454"}, []string{"timeline", "detections", "environments"}, false},
}

// Every store-backed novel refuses a resource whose last finished sync was
// narrowed by any user query parameter (--resource-param, --param,
// --global-param), names the resource and the remedy, and answers again
// after the remedy sync.
func TestSnapshotEveryStoreBackedNovelRefusesANarrowedResource(t *testing.T) {
	narrowings := [][]string{
		{"--resource-param", "%s:fields[]=ID"},
		{"--param", "conditions[]={\"path\":\"ID\",\"op\":\"eq\",\"value\":1}"},
		{"--global-param", "fields[]=ID"},
	}
	for _, novel := range storeBackedNovels {
		novel := novel
		t.Run(strings.Join(novel.args, "_"), func(t *testing.T) {
			startDocumentedEstate(t, 20, 20)
			mustRunSnapshotCLI(t, "sync", "--resources", strings.Join(novel.resources, ","), "--json")
			run := func() (string, error) {
				out, errOut, err := runSnapshotCLI(t, append(append([]string(nil), novel.args...), "--agent")...)
				if err != nil {
					return out, fmt.Errorf("%v %s", err, errOut)
				}
				return out, nil
			}
			if out, err := run(); (err != nil) != novel.refuses {
				t.Fatalf("on a full snapshot: refused=%v (want %v): %v %s", err != nil, novel.refuses, err, out)
			}
			for i, resource := range novel.resources {
				narrow := narrowings[i%len(narrowings)]
				flag, value := narrow[0], narrow[1]
				if strings.Contains(value, "%s") {
					value = fmt.Sprintf(value, resource)
				}
				mustRunSnapshotCLI(t, "sync", "--resources", resource, flag, value, "--json")
				out, err := run()
				if err == nil {
					t.Fatalf("answered after a %s-narrowed sync of %s: %s", flag, resource, out)
				}
				for _, want := range []string{"copy of " + resource, "narrowed", "sync --full --resources " + resource} {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("after narrowing %s: refusal lacks %q: %v", resource, want, err)
					}
				}
				mustRunSnapshotCLI(t, "sync", "--full", "--resources", resource, "--json")
				if out, err := run(); (err != nil) != novel.refuses {
					t.Fatalf("after the remedy sync of %s: refused=%v (want %v): %v %s", resource, err != nil, novel.refuses, err, out)
				}
			}
		})
	}
}

// run-stale --confirm sends inspection runs: on a narrowed launchpoints or
// timeline copy it refuses before any request.
func TestSnapshotRunStaleConfirmSendsNothingOnANarrowedCopy(t *testing.T) {
	for _, resource := range []string{"launchpoints", "timeline"} {
		t.Run(resource, func(t *testing.T) {
			api := startDocumentedEstate(t, 20, 20)
			api.set("/launchpoints/1204/run", func(r *http.Request) string { return `{}` })
			api.set("/launchpoints/1205/run", func(r *http.Request) string { return `{}` })
			mustRunSnapshotCLI(t, "sync", "--resources", "launchpoints,timeline,environments", "--json")
			mustRunSnapshotCLI(t, "sync", "--resources", resource, "--resource-param", resource+":fields[]=ID", "--json")
			before := len(api.seen())
			if out, _, err := runSnapshotCLI(t, "launchpoints", "run-stale", "--older-than", "7d", "--confirm", "--agent"); err == nil {
				t.Fatalf("run-stale --confirm ran on a narrowed %s copy: %s", resource, out)
			}
			if after := api.seen()[before:]; len(after) != 0 {
				t.Fatalf("run-stale sent %v after refusing", after)
			}
		})
	}
}

// A row missing a needed field refuses even when sync_state says the copy is
// full: here a narrowed live read (agents get --fields ID) cached a new agent
// with no LastHeartbeat or Environment beside the synced ones.
func TestSnapshotLiveReadProjectedRowRefuses(t *testing.T) {
	api := startDocumentedEstate(t, 20, 20)
	mustRunSnapshotCLI(t, "sync", "--resources", "agents,environments", "--json")
	estate := documentedEstate(20, 20)
	agents := append(estate["/agents"], documentedAgent(1513, 44, time.Now().Add(-48*time.Hour)))
	api.set("/agents", projectingList(t, agents))
	mustRunSnapshotCLI(t, "agents", "get", "--fields", "ID", "--json")

	out, errOut, err := runSnapshotCLI(t, "agents", "offline", "--agent")
	if err == nil {
		t.Fatalf("agents offline answered with a projected row cached: %s", out)
	}
	msg := err.Error() + errOut
	for _, want := range []string{"LastHeartbeat (1 of 3 rows)", "Environment (1 of 3 rows)", "sync --full --resources agents"} {
		if !strings.Contains(msg, want) {
			t.Errorf("refusal lacks %q: %v", want, msg)
		}
	}
	// The remedy sync fetches agent 1513 in full and the sweep answers.
	mustRunSnapshotCLI(t, "sync", "--full", "--resources", "agents", "--json")
	var got struct {
		Count int `json:"count"`
	}
	decodeNovelResult(t, mustRunSnapshotCLI(t, "agents", "offline", "--agent"), &got)
	if got.Count != 2 {
		t.Fatalf("after the remedy sync: want agents 1511 and 1513 offline, got %d", got.Count)
	}
}

// Each signal alone makes a walk not full, and a walk with none is full.
func TestSnapshotWalkVerdictSignals(t *testing.T) {
	parse := func(param, resourceParam, globalParam []string) *syncUserParams {
		p, err := parseSyncUserParams(param, resourceParam, globalParam)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	none := parse(nil, nil, nil)
	cases := []struct {
		name            string
		params          *syncUserParams
		complete        bool
		cursor, since   string
		priorFull, want bool
	}{
		{"unnarrowed finished walk", none, true, "", "", false, true},
		{"nil params", nil, true, "", "", false, true},
		{"walk did not finish", none, false, "", "", true, false},
		{"--resource-param on this resource", parse(nil, []string{"agents:fields[]=ID"}, nil), true, "", "", true, false},
		{"--resource-param on another resource", parse(nil, []string{"systems:fields[]=ID"}, nil), true, "", "", false, true},
		{"--param", parse([]string{"conditions[]=x"}, nil, nil), true, "", "", true, false},
		{"--global-param", parse(nil, nil, []string{"fields[]=ID"}), true, "", "", true, false},
		{"resumed cursor", none, true, "2", "", true, false},
		{"since window over a partial copy", none, true, "", "2026-11-01T00:00:00Z", false, false},
		{"since window over a full snapshot", none, true, "", "2026-11-01T00:00:00Z", true, true},
	}
	for _, c := range cases {
		if got := syncWalkIsFullSnapshot("agents", c.params, c.complete, c.cursor, c.since, c.priorFull); got != c.want {
			t.Errorf("%s: full = %v, want %v", c.name, got, c.want)
		}
	}
}

// Every resource a store-backed novel reads has its needed fields listed;
// a documented row passes; dropping any field group refuses and names it; a
// documented null is data; a row that is not an object refuses; a metric
// evaluation the live-read cache stored beside the definitions is skipped.
func TestSnapshotRowsMissingANeededFieldRefuse(t *testing.T) {
	read := map[string]bool{}
	for _, n := range storeBackedNovels {
		for _, r := range n.resources {
			read[r] = true
		}
	}
	for r := range read {
		if len(novelFieldGroups[r]) == 0 {
			t.Errorf("%s is read by a store-backed novel but lists no needed fields", r)
		}
	}

	estate := documentedEstate(14, 14)
	for resource, groups := range novelFieldGroups {
		path := "/" + resource
		rows := estate[path]
		if len(rows) == 0 {
			t.Fatalf("no documented %s rows in the test estate", resource)
		}
		db := r1Store(t, nil)
		markFullSnapshot(t, db, resource)
		putRow := func(row map[string]any) {
			raw, _ := json.Marshal(row)
			if err := db.Upsert(resource, fmt.Sprint(row["ID"]), raw); err != nil {
				t.Fatal(err)
			}
		}
		for _, row := range rows {
			putRow(row)
		}
		if got, err := loadObjs(db, resource); err != nil || len(got) != len(rows) {
			t.Fatalf("%s: documented rows refused: %d rows, %v", resource, len(got), err)
		}
		for _, g := range groups {
			db := r1Store(t, nil)
			markFullSnapshot(t, db, resource)
			for i, row := range rows {
				if i == 0 {
					trimmed := map[string]any{}
					for k, v := range row {
						if !slicesContains(g, k) {
							trimmed[k] = v
						}
					}
					trimmed["ID"] = row["ID"]
					if g[0] == "ID" {
						delete(trimmed, "ID")
						trimmed["Ref"] = "x" // keep a storable identity-free row
					}
					raw, _ := json.Marshal(trimmed)
					if err := db.Upsert(resource, "row-0", raw); err != nil {
						t.Fatal(err)
					}
					continue
				}
				raw, _ := json.Marshal(row)
				if err := db.Upsert(resource, fmt.Sprint(row["ID"]), raw); err != nil {
					t.Fatal(err)
				}
			}
			_, err := loadObjs(db, resource)
			if err == nil || !strings.Contains(err.Error(), g[0]+" (1 of ") {
				t.Errorf("%s without %v: want a refusal naming %s, got %v", resource, g, g[0], err)
			}
		}
	}

	// A documented null is data, not a missing field.
	db := r1Store(t, map[string]map[string]map[string]any{"agents": {"9": {"ID": 9, "Environment": nil, "LastHeartbeat": nil}}})
	markFullSnapshot(t, db, rtAgents)
	if _, err := loadObjs(db, rtAgents); err != nil {
		t.Errorf("null documented fields were refused: %v", err)
	}

	// A stored row that is not an object refuses.
	if _, err := db.DB().Exec(`INSERT INTO resources (id, resource_type, data) VALUES ('arr', 'agents', '[1,2]')`); err != nil {
		t.Fatal(err)
	}
	if _, err := loadObjs(db, rtAgents); err == nil || !strings.Contains(err.Error(), "does not decode") {
		t.Errorf("a non-object row was read: %v", err)
	}

	// A metric evaluation row is skipped; a definition missing a field is not.
	db = r1Store(t, map[string]map[string]map[string]any{"metrics": {
		"1":    {"ID": 1, "Name": "Metric 001: Count", "Inspector": map[string]any{"ID": 13}, "Environment": nil},
		"eval": {"TimelineID": 1332180, "Metric": map[string]any{"ID": 1, "Name": "Metric 001: Count", "err": nil, "value": 7}, "_pp_map_key": "1"},
	}})
	markFullSnapshot(t, db, rtMetrics)
	if got, err := loadObjs(db, rtMetrics); err != nil || len(got) != 1 {
		t.Errorf("metrics with a cached evaluation row: want the 1 definition, got %d rows, %v", len(got), err)
	}
}
