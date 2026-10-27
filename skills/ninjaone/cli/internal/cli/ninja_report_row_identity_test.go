// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"ninjaone-pp-cli/internal/cliutil/testenv"
	"ninjaone-pp-cli/internal/store"
)

// Class report-row-identity, end to end through `sync` against a fake NinjaOne that pages a
// /v2/queries/* report the way the public API does (an object cursor, pageSize rows a page):
//
//   - a report whose spec.yaml row schema documents no per-item identity keys each row on its
//     device and its ordinal among that device's rows in the walk, so two identical rows stay
//     two rows and an upgraded version takes its predecessor's place;
//   - every complete, unfiltered walk from page one, ordinary sync included, replaces the
//     report's stored rows (for a position-keyed report, --no-prune included), and an
//     incomplete walk deletes nothing (of a position-keyed report, it stores nothing).

type reportFake struct {
	mu       sync.Mutex
	path     string
	rows     []string
	failPage int
	failCode int
	failType string
	failBody string
	requests []string
}

func newReportFake(path string, rows ...string) *reportFake {
	return &reportFake{path: path, rows: rows}
}

// answer replaces the report NinjaOne returns from now on.
func (f *reportFake) answer(rows ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows = rows
	f.failPage = 0
	f.requests = nil
}

// failOn answers 1-based page with body instead of report rows.
func (f *reportFake) failOn(page, code int, contentType, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failPage, f.failCode, f.failType, f.failBody = page, code, contentType, body
}

func (f *reportFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != f.path {
		http.NotFound(w, r)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r.URL.RawQuery)
	offset := 0
	if c := r.URL.Query().Get("cursor"); c != "" {
		offset, _ = strconv.Atoi(strings.TrimPrefix(c, "off-"))
	}
	size := pageSizeOf(r)
	if f.failPage > 0 && offset/size+1 == f.failPage {
		w.Header().Set("Content-Type", f.failType)
		w.WriteHeader(f.failCode)
		fmt.Fprint(w, f.failBody)
		return
	}
	end := offset + size
	if end > len(f.rows) {
		end = len(f.rows)
	}
	var page []string
	if offset < end {
		page = f.rows[offset:end]
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"cursor":{"name":"off-%d","offset":%d,"count":%d},"results":[%s]}`, offset+len(page), offset, len(page), strings.Join(page, ","))
}

func (f *reportFake) firstRequest() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.requests) == 0 {
		return ""
	}
	return f.requests[0]
}

// runReportCmd runs one ninjaone-cli command against srvURL and returns stdout and stderr; a
// failed sync is returned, not fatal, so a walk that errors on a page can be asserted on.
func runReportCmd(t *testing.T, srvURL string, args ...string) (string, string, error) {
	t.Helper()
	cfg := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfg, []byte("access_token = \"fixture-token\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NINJAONE_CONFIG", cfg)
	t.Setenv("NINJAONE_BASE_URL", srvURL)
	t.Setenv("NINJAONE_CLIENT_ID", "")
	t.Setenv("NINJAONE_CLIENT_SECRET", "")
	root := newRootCmd(&rootFlags{})
	var out, errBuf bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errBuf)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), errBuf.String(), err
}

func runReportSync(t *testing.T, srvURL, dbPath string, args ...string) string {
	t.Helper()
	out, errOut, err := runReportCmd(t, srvURL, append([]string{"sync", "--db", dbPath, "--json"}, args...)...)
	if err != nil {
		t.Fatalf("sync %v failed: %v\n%s%s", args, err, out, errOut)
	}
	return out + errOut
}

func storedReportRows(t *testing.T, dbPath, resource string) []map[string]any {
	t.Helper()
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := loadRows(db, resource)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func rowValues(rows []map[string]any, field string) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, nvStr(r, field))
	}
	sort.Strings(out)
	return out
}

// Codex r3 [P1] #1, the input verbatim: two DeviceProcessor rows of device 7, both named
// "Intel Xeon" (a dual-socket server), are two processors.
func TestIdenticalProcessorsOfOneDeviceStayTwoRows(t *testing.T) {
	xeons := []string{
		`{"deviceId":7,"name":"Intel Xeon","clockSpeed":2400}`,
		`{"deviceId":7,"name":"Intel Xeon","clockSpeed":2500}`,
	}
	codexBody := `{"cursor":{"name":"C","offset":0,"count":2},"results":[` + strings.Join(xeons, ",") + `]}`
	for _, tc := range []struct {
		name     string
		args     []string
		verbatim bool
	}{
		{"codex-r3-full", []string{"--full"}, true},
		{"ordinary", nil, true},
		// The ordinal counts across the page boundaries of one walk.
		{"one-row-per-page", []string{"--resource-param", "queries-processors:pageSize=1"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testenv.Isolate(t)
			var handler http.Handler = newReportFake("/v2/queries/processors", xeons...)
			if tc.verbatim {
				handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path != "/v2/queries/processors" {
						http.NotFound(w, r)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, codexBody)
				})
			}
			srv := httptest.NewServer(handler)
			defer srv.Close()
			dbPath := filepath.Join(t.TempDir(), "data.db")
			runReportSync(t, srv.URL, dbPath, append([]string{"--resources", "queries-processors"}, tc.args...)...)
			rows := storedReportRows(t, dbPath, "queries-processors")
			if got := rowValues(rows, "clockSpeed"); strings.Join(got, ",") != "2400,2500" {
				t.Fatalf("queries-processors holds clock speeds %v, want both processors [2400 2500]: %v", got, rows)
			}
		})
	}
}

func acme(device int, version string) string {
	return fmt.Sprintf(`{"deviceId":%d,"name":"Acme Agent","version":%q}`, device, version)
}

func softwareAuditOf(t *testing.T, dbPath, name string) []softwareTitleOut {
	t.Helper()
	out, errOut, err := runReportCmd(t, "http://127.0.0.1:1", "software-audit", "--name", name, "--json", "--db", dbPath)
	if err != nil {
		t.Fatalf("software-audit: %v\n%s", err, errOut)
	}
	var titles []softwareTitleOut
	if err := json.Unmarshal([]byte(out), &titles); err != nil {
		var env struct {
			Results []softwareTitleOut `json:"results"`
		}
		if err2 := json.Unmarshal([]byte(out), &env); err2 != nil {
			t.Fatalf("software-audit output is not JSON: %v\n%s", err, out)
		}
		titles = env.Results
	}
	return titles
}

// Codex r3 [P1] #2: after a full sync stores Acme Agent version 1, an ORDINARY sync whose
// response returns only version 2 leaves one install of version 2, as NinjaOne reports.
// --no-prune does not apply to a position-keyed report: its rows have no identity to keep, so
// a whole walk replaces it under --no-prune too and says so.
func TestOrdinarySyncReplacesAnUpgradedVersion(t *testing.T) {
	aardvark := `{"deviceId":7,"name":"Aardvark Tool","version":"9"}`
	for _, tc := range []struct {
		name          string
		before, after []string
		args          []string
		installs      int
		versions      string
		rows          int
	}{
		{"codex-r3-input", []string{acme(7, "1")}, []string{acme(7, "2")}, nil, 1, "2", 1},
		{"codex-r3-input-no-prune", []string{acme(7, "1")}, []string{acme(7, "2")}, []string{"--no-prune"}, 1, "2", 1},
		// Aardvark was uninstalled while Acme Agent was upgraded: version 2 is now device 7's
		// first row and version 1 sits at a position the walk no longer reaches.
		{"upgrade-and-uninstall", []string{aardvark, acme(7, "1")}, []string{acme(7, "2")}, nil, 1, "2", 1},
		{"upgrade-and-uninstall-no-prune", []string{aardvark, acme(7, "1")}, []string{acme(7, "2")}, []string{"--no-prune"}, 1, "2", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testenv.Isolate(t)
			f := newReportFake("/v2/queries/software", tc.before...)
			srv := httptest.NewServer(f)
			defer srv.Close()
			dbPath := filepath.Join(t.TempDir(), "data.db")
			runReportSync(t, srv.URL, dbPath, "--resources", "queries-software", "--full")
			f.answer(tc.after...)
			out := runReportSync(t, srv.URL, dbPath, append([]string{"--resources", "queries-software"}, tc.args...)...)
			titles := softwareAuditOf(t, dbPath, "Acme Agent")
			if len(titles) != 1 || titles[0].Installs != tc.installs || strings.Join(titles[0].Versions, ",") != tc.versions {
				t.Fatalf("software-audit --name \"Acme Agent\" = %+v, want %d install(s) of versions %s\n%s", titles, tc.installs, tc.versions, out)
			}
			if rows := storedReportRows(t, dbPath, "queries-software"); len(rows) != tc.rows {
				t.Fatalf("queries-software holds %d rows, want %d: %v", len(rows), tc.rows, rows)
			}
			if !strings.Contains(out, `"event":"reconcile","resource":"queries-software","scope":"snapshot"`) {
				t.Fatalf("sync (args %v) did not replace the report:\n%s", tc.args, out)
			}
			noPrune, notice := tc.args != nil, strings.Contains(out, `"reason":"no_prune_not_applicable"`)
			if notice != noPrune {
				t.Fatalf("sync (args %v): the --no-prune notice present=%v, want %v:\n%s", tc.args, notice, noPrune, out)
			}
		})
	}
}

func appRows(device, from, to int) []string {
	var out []string
	for i := from; i <= to; i++ {
		out = append(out, fmt.Sprintf(`{"deviceId":%d,"name":"App %d","version":"1"}`, device+i, i))
	}
	return out
}

func patchRows(from, to int) []string {
	var out []string
	for i := from; i <= to; i++ {
		out = append(out, fmt.Sprintf(`{"deviceId":%d,"id":"patch-%d","name":"KB%d","status":"PENDING"}`, i, i, i))
	}
	return out
}

func hasRow(rows []map[string]any, field, value string) bool {
	for _, r := range rows {
		if nvStr(r, field) == value {
			return true
		}
	}
	return false
}

// An incomplete walk proves nothing about rows it did not reach: it never deletes. Each case
// stores a 150-row report (two pages), then NinjaOne stops returning the first row and the
// next walk breaks off. The first row must survive; a later complete walk removes it.
func TestIncompleteReportWalkDeletesNothing(t *testing.T) {
	for _, tc := range []struct {
		name, resource, path, field, stale string
		before, after                      []string
		args                               []string
		failPage, failCode                 int
		failType, failBody                 string
		reason                             string // reconcile_skipped reason; "" = the walk fails first
	}{
		{name: "max-pages-cap", resource: "queries-software", path: "/v2/queries/software", field: "name", stale: "App 1",
			before: appRows(0, 1, 150), after: appRows(0, 2, 151), args: []string{"--max-pages", "1"}, reason: "max_pages_cap"},
		{name: "non-json-page", resource: "queries-software", path: "/v2/queries/software", field: "name", stale: "App 1",
			before: appRows(0, 1, 150), after: appRows(0, 2, 151), failPage: 2, failCode: 200, failType: "text/plain", failBody: "upstream maintenance", reason: "non_json_200_body"},
		{name: "non-report-page", resource: "queries-software", path: "/v2/queries/software", field: "name", stale: "App 1",
			before: appRows(0, 1, 150), after: appRows(0, 2, 151), failPage: 2, failCode: 200, failType: "application/json", failBody: `{"message":"busy"}`, reason: "non_report_page"},
		{name: "html-page", resource: "queries-software", path: "/v2/queries/software", field: "name", stale: "App 1",
			before: appRows(0, 1, 150), after: appRows(0, 2, 151), failPage: 2, failCode: 200, failType: "text/html", failBody: "<html>maintenance</html>"},
		{name: "server-error-page", resource: "queries-software", path: "/v2/queries/software", field: "name", stale: "App 1",
			before: appRows(0, 1, 150), after: appRows(0, 2, 151), failPage: 2, failCode: 500, failType: "application/json", failBody: `{"error":"boom"}`},
		{name: "processors-max-pages-cap", resource: "queries-processors", path: "/v2/queries/processors", field: "name", stale: "App 1",
			before: appRows(0, 1, 150), after: appRows(0, 2, 151), args: []string{"--max-pages", "1"}, reason: "max_pages_cap"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testenv.Isolate(t)
			f := newReportFake(tc.path, tc.before...)
			srv := httptest.NewServer(f)
			defer srv.Close()
			dbPath := filepath.Join(t.TempDir(), "data.db")
			runReportSync(t, srv.URL, dbPath, "--resources", tc.resource, "--full")
			if n := len(storedReportRows(t, dbPath, tc.resource)); n != len(tc.before) {
				t.Fatalf("first sync stored %d rows, want %d", n, len(tc.before))
			}
			f.answer(tc.after...)
			if tc.failPage > 0 {
				f.failOn(tc.failPage, tc.failCode, tc.failType, tc.failBody)
			}
			stdout, stderr, err := runReportCmd(t, srv.URL, append([]string{"sync", "--db", dbPath, "--json", "--resources", tc.resource}, tc.args...)...)
			out := stdout + stderr
			if tc.reason == "" && err == nil {
				t.Fatalf("sync with a failing page succeeded:\n%s", out)
			}
			if strings.Contains(out, `"event":"reconcile","resource":"`+tc.resource+`"`) {
				t.Fatalf("an incomplete walk replaced %s:\n%s", tc.resource, out)
			}
			if tc.reason != "" && !strings.Contains(out, `"scope":"snapshot","reason":"`+tc.reason+`"`) {
				t.Fatalf("want reconcile_skipped reason %s:\n%s", tc.reason, out)
			}
			if rows := storedReportRows(t, dbPath, tc.resource); !hasRow(rows, tc.field, tc.stale) {
				t.Fatalf("an incomplete walk deleted %q (%d rows left)", tc.stale, len(rows))
			}
			f.answer(tc.after...)
			runReportSync(t, srv.URL, dbPath, "--resources", tc.resource)
			rows := storedReportRows(t, dbPath, tc.resource)
			if hasRow(rows, tc.field, tc.stale) || len(rows) != len(tc.after) {
				t.Fatalf("a complete ordinary walk left %d rows (stale %q present=%v), want exactly the %d rows NinjaOne returns", len(rows), tc.stale, hasRow(rows, tc.field, tc.stale), len(tc.after))
			}
		})
	}
}

// A walk resumed from a saved cursor did not see the pages before it: it never replaces.
// /v2/queries/os-patches rows key on the documented DeviceOSPatch.id, so they may resume.
func TestResumedReportWalkDeletesNothing(t *testing.T) {
	testenv.Isolate(t)
	f := newReportFake("/v2/queries/os-patches", patchRows(1, 150)...)
	srv := httptest.NewServer(f)
	defer srv.Close()
	dbPath := filepath.Join(t.TempDir(), "data.db")
	runReportSync(t, srv.URL, dbPath, "--resources", "queries-os-patches", "--full")
	f.answer(patchRows(2, 151)...)
	runReportSync(t, srv.URL, dbPath, "--resources", "queries-os-patches", "--max-pages", "1")
	f.answer(patchRows(2, 151)...)
	out := runReportSync(t, srv.URL, dbPath, "--resources", "queries-os-patches")
	if !strings.Contains(f.firstRequest(), "cursor=off-100") {
		t.Fatalf("the walk after a capped one did not resume from the saved cursor: %q", f.firstRequest())
	}
	if !strings.Contains(out, `"scope":"snapshot","reason":"resumed_walk"`) {
		t.Fatalf("want reconcile_skipped reason resumed_walk:\n%s", out)
	}
	rows := storedReportRows(t, dbPath, "queries-os-patches")
	if len(rows) != 151 || !hasRow(rows, "id", "patch-1") || !hasRow(rows, "id", "patch-2") {
		t.Fatalf("a resumed walk deleted rows it did not walk: %d rows left", len(rows))
	}
}

// A report keyed by row ordinal never resumes mid-report: the resumed pages would number a
// device's rows from the resume point and overwrite that device's first rows.
func TestOrdinalReportWalkStartsAtPageOne(t *testing.T) {
	testenv.Isolate(t)
	var rows []string
	for i := 0; i < 150; i++ {
		rows = append(rows, fmt.Sprintf(`{"deviceId":7,"name":"App %03d","version":"1"}`, i))
	}
	f := newReportFake("/v2/queries/software", rows...)
	srv := httptest.NewServer(f)
	defer srv.Close()
	dbPath := filepath.Join(t.TempDir(), "data.db")
	runReportSync(t, srv.URL, dbPath, "--resources", "queries-software", "--full")
	// A capped walk of an ordinal report stores none of its rows (its warning makes the run's
	// only resource unsuccessful), but it may still leave a resume cursor behind.
	if stdout, stderr, _ := runReportCmd(t, srv.URL, "sync", "--db", dbPath, "--json", "--resources", "queries-software", "--max-pages", "1"); !strings.Contains(stdout+stderr, `"reason":"report_not_stored"`) {
		t.Fatalf("a capped ordinal walk did not say its rows were not stored:\n%s%s", stdout, stderr)
	}
	f.answer(rows...)
	runReportSync(t, srv.URL, dbPath, "--resources", "queries-software")
	if q := f.firstRequest(); strings.Contains(q, "cursor=") {
		t.Fatalf("an ordinal report resumed from a saved cursor: %q", q)
	}
	got := rowValues(storedReportRows(t, dbPath, "queries-software"), "name")
	if len(got) != 150 {
		t.Fatalf("device 7 holds %d rows, want its 150 installs", len(got))
	}
	for i, name := range got {
		if want := fmt.Sprintf("App %03d", i); name != want {
			t.Fatalf("device 7 row %d is %q, want %q (rows overwritten by a resumed walk)", i, name, want)
		}
	}
}

// The volume readers read what sync stores under ordinal keys: three volumes of one device,
// two of them with the same name, are three volumes, joined to the device and organization.
func TestVolumeReadersReadSyncedOrdinalRows(t *testing.T) {
	testenv.Isolate(t)
	f := newReportFake("/v2/queries/volumes",
		`{"deviceId":7,"name":"","driveLetter":"C:","capacity":107374182400,"freeSpace":3221225472,"bitLockerStatus":{"protectionStatus":"PROTECTED"}}`,
		`{"deviceId":7,"name":"","driveLetter":"D:","capacity":107374182400,"freeSpace":53687091200,"bitLockerStatus":{"protectionStatus":"UNPROTECTED","conversionStatus":"FULLY_DECRYPTED"}}`,
		`{"deviceId":7,"name":"E:","driveLetter":"E:","capacity":0,"freeSpace":0}`)
	srv := httptest.NewServer(f)
	defer srv.Close()
	dbPath := filepath.Join(t.TempDir(), "data.db")
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.UpsertBatch(rtOrgs, []json.RawMessage{json.RawMessage(`{"id":2,"name":"Acme"}`)}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.UpsertBatch(rtDevices, []json.RawMessage{json.RawMessage(`{"id":7,"systemName":"FRONTDESK","organizationId":2,"nodeClass":"WINDOWS_WORKSTATION"}`)}); err != nil {
		t.Fatal(err)
	}
	db.Close()
	runReportSync(t, srv.URL, dbPath, "--resources", "queries-volumes")
	if n := len(storedReportRows(t, dbPath, "queries-volumes")); n != 3 {
		t.Fatalf("queries-volumes holds %d rows, want 3", n)
	}
	read := func(args ...string) []map[string]any {
		t.Helper()
		out, errOut, err := runReportCmd(t, srv.URL, append(args, "--db", dbPath, "--json")...)
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, errOut)
		}
		var rows []map[string]any
		if json.Unmarshal([]byte(out), &rows) != nil {
			var env struct {
				Results []map[string]any `json:"results"`
			}
			if err := json.Unmarshal([]byte(out), &env); err != nil {
				t.Fatalf("%v output is not JSON rows: %v\n%s", args, err, out)
			}
			rows = env.Results
		}
		return rows
	}
	if low := read("low-disk"); len(low) != 1 || low[0]["deviceName"] != "FRONTDESK" || low[0]["org"] != "Acme" {
		t.Fatalf("low-disk = %v, want only FRONTDESK's nearly full volume in Acme", low)
	}
	if gaps := read("bitlocker-gaps"); len(gaps) != 1 || gaps[0]["protectionStatus"] != "UNPROTECTED" {
		t.Fatalf("bitlocker-gaps = %v, want only FRONTDESK's UNPROTECTED volume", gaps)
	}
}
