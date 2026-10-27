// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"ninjaone-pp-cli/internal/cliutil/testenv"
	"ninjaone-pp-cli/internal/store"
)

// Class report-row-identity, the class "an ordinal report's stored rows are written or kept by
// a walk that is not the whole report" (Codex r1, widened by the 2026-10-26 adversarial
// review): position-keyed rows are written only by a walk that also replaces the whole
// report, in one transaction. One classifier (ninjaReportWalkParams) decides which user
// parameters a report walk may send and still be the whole report; a row-narrowing walk of an
// ordinal report is refused before any request; a walk with only row-preserving parameters
// replaces the report; any other walk (capped, failed) writes nothing; --no-prune does not
// apply to these reports; and a complete `workflow archive` walk replaces the report too.

func userParamsOf(flat, global map[string]string, perResource map[string]map[string]string) *syncUserParams {
	if flat == nil {
		flat = map[string]string{}
	}
	if global == nil {
		global = map[string]string{}
	}
	if perResource == nil {
		perResource = map[string]map[string]string{}
	}
	return &syncUserParams{flatGlobal: flat, trueGlobal: global, perResource: perResource}
}

func joinParams(ps []ninjaReportParam) string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.String())
	}
	return strings.Join(out, "; ")
}

// The allowlist is what spec.yaml documents as paging or projection for the endpoint; every
// other parameter, and any it does not document, narrows the walk.
func TestReportWalkParamsClassifier(t *testing.T) {
	sw := func(kv ...string) map[string]map[string]string {
		m := map[string]string{}
		for i := 0; i < len(kv); i += 2 {
			m[kv[i]] = kv[i+1]
		}
		return map[string]map[string]string{"queries-software": m}
	}
	for _, tc := range []struct {
		name, resource        string
		dependent             bool
		params                *syncUserParams
		preserving, narrowing string
	}{
		{"page-size-is-paging", "queries-software", false, userParamsOf(nil, nil, sw("pageSize", "1")), "--resource-param queries-software:pageSize=1", ""},
		{"page-size-from-param", "queries-processors", false, userParamsOf(map[string]string{"pageSize": "7"}, nil, nil), "--param pageSize=7", ""},
		{"page-size-from-global-param", "queries-windows-services", false, userParamsOf(nil, map[string]string{"pageSize": "5000"}, nil), "--global-param pageSize=5000", ""},
		{"page-size-zero", "queries-software", false, userParamsOf(nil, nil, sw("pageSize", "0")), "", "--resource-param queries-software:pageSize=0"},
		{"page-size-negative", "queries-software", false, userParamsOf(nil, nil, sw("pageSize", "-5")), "", "--resource-param queries-software:pageSize=-5"},
		{"page-size-not-a-number", "queries-software", false, userParamsOf(nil, nil, sw("pageSize", "all")), "", "--resource-param queries-software:pageSize=all"},
		{"include-is-volumes-projection", "queries-volumes", false, userParamsOf(nil, nil, map[string]map[string]string{"queries-volumes": {"include": "bl"}}), "--resource-param queries-volumes:include=bl", ""},
		{"include-undocumented-on-software", "queries-software", false, userParamsOf(nil, nil, sw("include", "bl")), "", "--resource-param queries-software:include=bl"},
		{"cursor-positions-the-walk", "queries-software", false, userParamsOf(nil, nil, sw("cursor", "off-100")), "", "--resource-param queries-software:cursor=off-100"},
		{"documented-filter", "queries-software", false, userParamsOf(nil, nil, sw("installedAfter", "2026-01-01")), "", "--resource-param queries-software:installedAfter=2026-01-01"},
		{"device-filter", "queries-software", false, userParamsOf(map[string]string{"df": "org=1"}, nil, nil), "", "--param df=org=1"},
		{"timestamp-filter", "queries-processors", false, userParamsOf(nil, map[string]string{"ts": "2026-01-01"}, nil), "", "--global-param ts=2026-01-01"},
		{"unknown-key", "queries-software", false, userParamsOf(nil, nil, sw("foo", "bar")), "", "--resource-param queries-software:foo=bar"},
		{"key-case-matters", "queries-software", false, userParamsOf(nil, nil, sw("pagesize", "1")), "", "--resource-param queries-software:pagesize=1"},
		{"mixed", "queries-software", false, userParamsOf(nil, nil, sw("pageSize", "1", "installedAfter", "2026-01-01")), "--resource-param queries-software:pageSize=1", "--resource-param queries-software:installedAfter=2026-01-01"},
		// The value on the request is the one applyTo leaves: --resource-param over --global-param over --param.
		{"resource-param-value-wins", "queries-software", false, userParamsOf(map[string]string{"pageSize": "all"}, nil, sw("pageSize", "5")), "--resource-param queries-software:pageSize=5", ""},
		{"global-param-value-wins", "queries-software", false, userParamsOf(map[string]string{"pageSize": "5"}, map[string]string{"pageSize": "all"}, nil), "", "--global-param pageSize=all"},
		{"another-resources-param-is-not-sent", "queries-software", false, userParamsOf(nil, nil, map[string]map[string]string{"queries-volumes": {"df": "org=1"}}), "", ""},
		{"dependent-walk-is-not-sent-param", "queries-software", true, userParamsOf(map[string]string{"df": "org=1"}, nil, nil), "", ""},
		{"not-a-report", "devices", false, userParamsOf(nil, nil, map[string]map[string]string{"devices": {"pageSize": "50"}}), "", "--resource-param devices:pageSize=50"},
		{"no-params", "queries-software", false, nil, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			preserving, narrowing := ninjaReportWalkParams(tc.resource, tc.dependent, tc.params)
			if got := joinParams(preserving); got != tc.preserving {
				t.Errorf("row-preserving = %q, want %q", got, tc.preserving)
			}
			if got := joinParams(narrowing); got != tc.narrowing {
				t.Errorf("row-narrowing = %q, want %q", got, tc.narrowing)
			}
		})
	}
}

// The replace decision judges a report walk by its row-narrowing parameters only; every other
// partition keeps every skip it had, and every other signal still skips.
func TestReportPruneSkipJudgesOnlyRowNarrowingParams(t *testing.T) {
	pageSize := func(resource string) *syncUserParams {
		return userParamsOf(nil, nil, map[string]map[string]string{resource: {"pageSize": "1"}})
	}
	for _, tc := range []struct {
		name, resource string
		dependent      bool
		since          string
		params         *syncUserParams
		want           string
	}{
		{"ordinal-report-page-size", "queries-software", false, "", pageSize("queries-software"), ""},
		{"ordinal-report-page-size-param", "queries-software", false, "", userParamsOf(map[string]string{"pageSize": "1"}, nil, nil), ""},
		{"ordinal-report-page-size-global", "queries-software", false, "", userParamsOf(nil, map[string]string{"pageSize": "1"}, nil), ""},
		{"documented-id-report-page-size", "queries-os-patches", false, "", pageSize("queries-os-patches"), ""},
		{"volumes-include", "queries-volumes", false, "", userParamsOf(nil, nil, map[string]map[string]string{"queries-volumes": {"include": "bl"}}), ""},
		{"report-filter", "queries-os-patches", false, "", userParamsOf(nil, nil, map[string]map[string]string{"queries-os-patches": {"pageSize": "1", "status": "FAILED"}}), syncPruneSkipResourceParam},
		{"report-flat-filter", "queries-os-patches", false, "", userParamsOf(map[string]string{"df": "org=1"}, nil, nil), syncPruneSkipParamFilter},
		{"report-global-filter", "queries-os-patches", false, "", userParamsOf(nil, map[string]string{"df": "org=1"}, nil), syncPruneSkipGlobalParam},
		{"report-since-window", "queries-software", false, "2026-01-01T00:00:00Z", pageSize("queries-software"), syncPruneSkipSinceWindow},
		// Not report snapshot partitions: unchanged.
		{"flat-global-queries-path", "queries-backup-usage", false, "", pageSize("queries-backup-usage"), syncPruneSkipResourceParam},
		{"flat-global", "organization", false, "", pageSize("organization"), syncPruneSkipResourceParam},
		{"list", "devices", false, "", userParamsOf(map[string]string{"pageSize": "1"}, nil, nil), syncPruneSkipParamFilter},
		{"dependent", "organization_devices", true, "", pageSize("organization_devices"), syncPruneSkipResourceParam},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := syncPruneSkipReason(tc.resource, tc.dependent, tc.since, "", tc.params, resourceReconcileMode(tc.resource), ""); got != tc.want {
				t.Fatalf("syncPruneSkipReason(%s) = %q, want %q", tc.resource, got, tc.want)
			}
		})
	}
}

// Every ordinal report names a live read that exists, and only ordinal reports are refused.
func TestNarrowedWalkRefusalCoversEveryOrdinalReport(t *testing.T) {
	root := newRootCmd(&rootFlags{})
	ordinal := 0
	for _, resource := range knownSyncResourceNames() {
		df := userParamsOf(map[string]string{"df": "org=1"}, nil, nil)
		err := ninjaNarrowedOrdinalWalkError(resource, "", "", df)
		if !store.ReportRowsAreOrdinal(resource) {
			if err != nil {
				t.Errorf("%s is not keyed by ordinal but a filtered walk of it was refused: %v", resource, err)
			}
			continue
		}
		ordinal++
		if err == nil || !strings.Contains(err.Error(), "--param df=org=1") {
			t.Errorf("%s: a filtered walk was not refused naming the parameter: %v", resource, err)
		}
		live := ninjaOrdinalReportLiveCommands[resource]
		if cmd, _, ferr := root.Find(strings.Fields(live)); live == "" || ferr != nil || cmd == root {
			t.Errorf("%s names live read %q, which is not a command", resource, live)
		} else if err != nil && !strings.Contains(err.Error(), "`ninjaone-cli "+live+"`") {
			t.Errorf("%s refusal does not name its live read: %v", resource, err)
		}
		if ninjaNarrowedOrdinalWalkError(resource, "", "", userParamsOf(nil, map[string]string{"pageSize": "10"}, nil)) != nil {
			t.Errorf("%s: a pageSize-only walk was refused", resource)
		}
		if ninjaNarrowedOrdinalWalkError(resource, "updatedAfter", "2026-01-01T00:00:00Z", nil) == nil {
			t.Errorf("%s: a walk with a since window was not refused", resource)
		}
	}
	if ordinal != 7 {
		t.Fatalf("found %d ordinal reports among the sync resources, want 7", ordinal)
	}
}

// filteringReport is a fake /v2/queries/* report that pages like NinjaOne and honours the
// documented row filters: installedAfter on DeviceApplication.installDate, and any other
// query key that names a row field (state, name, productState, productName) by equality.
type filteringReport struct {
	mu       sync.Mutex
	path     string
	rows     []string
	requests []string
}

func newFilteringReport(path string, rows ...string) *filteringReport {
	return &filteringReport{path: path, rows: rows}
}

func (f *filteringReport) answer(rows ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows = rows
	f.requests = nil
}

func (f *filteringReport) sent() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.requests...)
}

func (f *filteringReport) keep(row string, q map[string][]string) bool {
	var fields map[string]any
	if json.Unmarshal([]byte(row), &fields) != nil {
		return false
	}
	for key, vals := range q {
		switch key {
		case "cursor", "pageSize", "include":
			continue
		case "installedAfter":
			if d, _ := fields["installDate"].(string); d < vals[0] {
				return false
			}
		default:
			if v, ok := fields[key]; ok && fmt.Sprint(v) != vals[0] {
				return false
			}
		}
	}
	return true
}

func (f *filteringReport) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != f.path {
		http.NotFound(w, r)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r.URL.RawQuery)
	var rows []string
	for _, row := range f.rows {
		if f.keep(row, r.URL.Query()) {
			rows = append(rows, row)
		}
	}
	offset := 0
	if c := r.URL.Query().Get("cursor"); c != "" {
		offset, _ = strconv.Atoi(strings.TrimPrefix(c, "off-"))
	}
	end := offset + pageSizeOf(r)
	if end > len(rows) {
		end = len(rows)
	}
	var page []string
	if offset < end {
		page = rows[offset:end]
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"cursor":{"name":"off-%d","offset":%d,"count":%d},"results":[%s]}`, offset+len(page), offset, len(page), strings.Join(page, ","))
}

// partitionContent is every stored row of resource, id and data, sorted.
func partitionContent(t *testing.T, dbPath, resource string) string {
	t.Helper()
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.DB().Query(`SELECT id, data FROM resources WHERE resource_type = ?`, resource)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id, data string
		if err := rows.Scan(&id, &data); err != nil {
			t.Fatal(err)
		}
		out = append(out, strconv.Quote(id)+" "+data)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	sort.Strings(out)
	return strings.Join(out, "\n")
}

const (
	r1OldTool   = `{"deviceId":7,"name":"OldTool","version":"3.1","installDate":"2025-04-02T00:00:00Z"}`
	r1AcmeAgent = `{"deviceId":7,"name":"Acme Agent","version":"5.0","installDate":"2026-02-10T00:00:00Z"}`
)

// Codex r1 [P1] #1, the input: device 7 has OldTool (installed 2025) and Acme Agent (installed
// 2026); a sync filtered with the documented installedAfter returns only Acme Agent, which
// would take OldTool's ordinal while the stored Acme Agent row stays. The filtered sync is
// refused: it exits non-zero naming the parameter, sends no request, and the stored rows are
// unchanged, so software-audit still reports one Acme Agent install.
func TestFilteredOrdinalReportSyncIsRefused(t *testing.T) {
	for _, tc := range []struct {
		name, named string
		args        []string
	}{
		{"codex-r1-input", "--resource-param queries-software:installedAfter=2026-01-01", []string{"--resource-param", "queries-software:installedAfter=2026-01-01"}},
		{"full", "--resource-param queries-software:installedAfter=2026-01-01", []string{"--full", "--resource-param", "queries-software:installedAfter=2026-01-01"}},
		{"param", "--param installedAfter=2026-01-01", []string{"--param", "installedAfter=2026-01-01"}},
		{"global-param", "--global-param installedAfter=2026-01-01", []string{"--global-param", "installedAfter=2026-01-01"}},
		{"beside-page-size", "--resource-param queries-software:installedAfter=2026-01-01", []string{"--resource-param", "queries-software:pageSize=1", "--resource-param", "queries-software:installedAfter=2026-01-01"}},
		{"cursor", "--resource-param queries-software:cursor=off-1", []string{"--resource-param", "queries-software:cursor=off-1"}},
		{"unknown-param", "--resource-param queries-software:vendor=acme", []string{"--resource-param", "queries-software:vendor=acme"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testenv.Isolate(t)
			f := newFilteringReport("/v2/queries/software", r1OldTool, r1AcmeAgent)
			srv := httptest.NewServer(f)
			defer srv.Close()
			dbPath := filepath.Join(t.TempDir(), "data.db")
			runReportSync(t, srv.URL, dbPath, "--resources", "queries-software", "--full")
			before := partitionContent(t, dbPath, "queries-software")
			if n := len(storedReportRows(t, dbPath, "queries-software")); n != 2 {
				t.Fatalf("first sync stored %d rows, want 2", n)
			}
			f.answer(r1OldTool, r1AcmeAgent)
			stdout, stderr, err := runReportCmd(t, srv.URL, append([]string{"sync", "--db", dbPath, "--json", "--resources", "queries-software"}, tc.args...)...)
			out := stdout + stderr
			if err == nil {
				t.Fatalf("a filtered sync of an ordinal report exited 0:\n%s", out)
			}
			if !strings.Contains(out, tc.named) || !strings.Contains(out, "syncs only whole") || !strings.Contains(out, "queries get-software") {
				t.Fatalf("the refusal does not name %q, say the report syncs only whole, and name the live read:\n%s", tc.named, out)
			}
			if sent := f.sent(); len(sent) != 0 {
				t.Fatalf("a refused sync sent %d request(s): %v", len(sent), sent)
			}
			if after := partitionContent(t, dbPath, "queries-software"); after != before {
				t.Fatalf("a refused sync changed the stored report:\nbefore:\n%s\nafter:\n%s", before, after)
			}
			if titles := softwareAuditOf(t, dbPath, "Acme Agent"); len(titles) != 1 || titles[0].Installs != 1 {
				t.Fatalf("software-audit --name \"Acme Agent\" = %+v, want one install", titles)
			}
		})
	}
}

// The adversarial review's filter input, and every other documented row filter of an ordinal
// report: Windows services `state` / `name`, antivirus status `productState` / `productName`.
// state=STOPPED stored [Spooler, Spooler, EDR] over [Backup Agent, Spooler, EDR]; it is now
// refused before any request and the stored rows are byte-for-byte unchanged.
func TestDocumentedFiltersOfOrdinalReportsAreRefused(t *testing.T) {
	services := []string{
		`{"deviceId":7,"name":"Backup Agent","displayName":"Backup Agent","state":"RUNNING"}`,
		`{"deviceId":7,"name":"Spooler","displayName":"Print Spooler","state":"STOPPED"}`,
		`{"deviceId":7,"name":"EDR","displayName":"EDR Sensor","state":"RUNNING"}`,
	}
	antivirus := []string{
		`{"deviceId":7,"productName":"Defender","productState":"ON","definitionStatus":"UP_TO_DATE"}`,
		`{"deviceId":7,"productName":"Acme AV","productState":"OFF","definitionStatus":"OUT_OF_DATE"}`,
	}
	for _, tc := range []struct {
		name, resource, path, param string
		rows                        []string
	}{
		{"adversarial-windows-services-state", "queries-windows-services", "/v2/queries/windows-services", "state=STOPPED", services},
		{"windows-services-name", "queries-windows-services", "/v2/queries/windows-services", "name=Spooler", services},
		{"antivirus-product-state", "queries-antivirus-status", "/v2/queries/antivirus-status", "productState=OFF", antivirus},
		{"antivirus-product-name", "queries-antivirus-status", "/v2/queries/antivirus-status", "productName=Acme AV", antivirus},
		{"volumes-timestamp", "queries-volumes", "/v2/queries/volumes", "ts=1700000000", []string{`{"deviceId":7,"name":"C:","capacity":100,"freeSpace":40}`}},
		{"disks-device-filter", "queries", "/v2/queries/disks", "df=org=1", []string{`{"deviceId":7,"name":"Disk 0","size":100}`}},
		{"processors-timestamp", "queries-processors", "/v2/queries/processors", "ts=1700000000", []string{`{"deviceId":7,"name":"Intel Xeon"}`}},
		{"logged-on-users-device-filter", "queries-logged-on-users", "/v2/queries/logged-on-users", "df=class=WINDOWS_SERVER", []string{`{"deviceId":7,"userName":"alice"}`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testenv.Isolate(t)
			f := newFilteringReport(tc.path, tc.rows...)
			srv := httptest.NewServer(f)
			defer srv.Close()
			dbPath := filepath.Join(t.TempDir(), "data.db")
			runReportSync(t, srv.URL, dbPath, "--resources", tc.resource, "--full")
			before := partitionContent(t, dbPath, tc.resource)
			if n := len(storedReportRows(t, dbPath, tc.resource)); n != len(tc.rows) {
				t.Fatalf("first sync stored %d rows, want %d", n, len(tc.rows))
			}
			f.answer(tc.rows...)
			flag := tc.resource + ":" + tc.param
			stdout, stderr, err := runReportCmd(t, srv.URL, "sync", "--db", dbPath, "--json", "--resources", tc.resource, "--resource-param", flag)
			out := stdout + stderr
			if err == nil || !strings.Contains(out, "--resource-param "+flag) || !strings.Contains(out, "syncs only whole") {
				t.Fatalf("a %s sync was not refused naming the parameter (err %v):\n%s", flag, err, out)
			}
			if sent := f.sent(); len(sent) != 0 {
				t.Fatalf("a refused sync sent %d request(s): %v", len(sent), sent)
			}
			if after := partitionContent(t, dbPath, tc.resource); after != before {
				t.Fatalf("a refused sync changed the stored report:\nbefore:\n%s\nafter:\n%s", before, after)
			}
		})
	}
}

// Codex r1 [P1] #2, the input: two software rows of device 7 are stored, then SoftwareReport
// returns only the second, synced with only the documented paging parameter pageSize=1. The
// walk is complete and unfiltered, so it replaces the report: the stale ordinal-1 row goes
// and software-audit reports the one install NinjaOne reports.
func TestPageSizeOnlyOrdinalWalkReplacesTheReport(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"codex-r1-input", []string{"--resource-param", "queries-software:pageSize=1"}},
		{"full", []string{"--full", "--resource-param", "queries-software:pageSize=1"}},
		{"param", []string{"--param", "pageSize=1"}},
		{"global-param", []string{"--global-param", "pageSize=1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testenv.Isolate(t)
			f := newFilteringReport("/v2/queries/software", r1OldTool, r1AcmeAgent)
			srv := httptest.NewServer(f)
			defer srv.Close()
			dbPath := filepath.Join(t.TempDir(), "data.db")
			runReportSync(t, srv.URL, dbPath, "--resources", "queries-software", "--full")
			f.answer(r1AcmeAgent)
			out := runReportSync(t, srv.URL, dbPath, append([]string{"--resources", "queries-software"}, tc.args...)...)
			sent := f.sent()
			if len(sent) < 2 {
				t.Fatalf("a pageSize=1 walk of a one-row report sent %d request(s), want it to page to the end: %v", len(sent), sent)
			}
			for _, q := range sent {
				if !strings.Contains(q, "pageSize=1") {
					t.Fatalf("request %q did not carry pageSize=1", q)
				}
			}
			if !strings.Contains(out, `"event":"reconcile","resource":"queries-software","scope":"snapshot"`) {
				t.Fatalf("a complete walk with only pageSize did not replace the report:\n%s", out)
			}
			rows := storedReportRows(t, dbPath, "queries-software")
			if len(rows) != 1 || nvStr(rows[0], "name") != "Acme Agent" {
				t.Fatalf("queries-software holds %v, want only device 7's Acme Agent", rows)
			}
			if titles := softwareAuditOf(t, dbPath, "Acme Agent"); len(titles) != 1 || titles[0].Installs != 1 {
				t.Fatalf("software-audit --name \"Acme Agent\" = %+v, want one install\n%s", titles, out)
			}
		})
	}
}

func deviceApps(device, from, to int) []string {
	var out []string
	for i := from; i <= to; i++ {
		out = append(out, fmt.Sprintf(`{"deviceId":%d,"name":"App %03d","version":"1"}`, device, i))
	}
	return out
}

// A walk that does not reach the end of an ordinal report writes nothing. The adversarial
// input: device 7 holds App 000..App 149 (two pages of 100); NinjaOne then lists "New Title"
// first (151 rows). That answer's first page is New Title, App 000..App 098: written on its own
// it would move App 098 onto App 099's position and App 099 would vanish from the store. Each
// walk below breaks off, and the stored rows stay byte-for-byte as they were.
func TestIncompleteOrdinalWalkWritesNothing(t *testing.T) {
	newFirst := append([]string{`{"deviceId":7,"name":"New Title","version":"1"}`}, deviceApps(7, 0, 149)...)
	for _, tc := range []struct {
		name               string
		args               []string
		failPage, failCode int
		failType, failBody string
		reason             string // the reconcile_skipped reason; "" = the walk fails with an error first
	}{
		{name: "adversarial-max-pages-cap", args: []string{"--max-pages", "1"}, reason: "max_pages_cap"},
		{name: "adversarial-page-2-server-error", failPage: 2, failCode: 500, failType: "application/json", failBody: `{"error":"boom"}`},
		{name: "max-pages-cap-no-prune", args: []string{"--max-pages", "1", "--no-prune"}, reason: "max_pages_cap"},
		{name: "latest-only", args: []string{"--latest-only"}, reason: "max_pages_cap"},
		{name: "page-2-not-found", failPage: 2, failCode: 404, failType: "application/json", failBody: `{"error":"gone"}`},
		{name: "page-2-non-json", failPage: 2, failCode: 200, failType: "text/plain", failBody: "upstream maintenance", reason: "non_json_200_body"},
		{name: "page-2-not-report-rows", failPage: 2, failCode: 200, failType: "application/json", failBody: `{"message":"busy"}`, reason: "non_report_page"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testenv.Isolate(t)
			f := newReportFake("/v2/queries/software", deviceApps(7, 0, 149)...)
			srv := httptest.NewServer(f)
			defer srv.Close()
			dir := t.TempDir()
			dbPath := filepath.Join(dir, "data.db")
			runReportSync(t, srv.URL, dbPath, "--resources", "queries-software")
			before := partitionContent(t, dbPath, "queries-software")
			if n := len(storedReportRows(t, dbPath, "queries-software")); n != 150 {
				t.Fatalf("first sync stored %d rows, want 150", n)
			}
			f.answer(newFirst...)
			if tc.failPage > 0 {
				f.failOn(tc.failPage, tc.failCode, tc.failType, tc.failBody)
			}
			stdout, stderr, err := runReportCmd(t, srv.URL, append([]string{"sync", "--db", dbPath, "--json", "--resources", "queries-software"}, tc.args...)...)
			out := stdout + stderr
			if tc.reason == "" && err == nil {
				t.Fatalf("sync with a failing page succeeded:\n%s", out)
			}
			if tc.reason != "" && (!strings.Contains(out, `"scope":"snapshot","reason":"`+tc.reason+`"`) || !strings.Contains(out, `"reason":"report_not_stored"`)) {
				t.Fatalf("want reconcile_skipped reason %s and report_not_stored:\n%s", tc.reason, out)
			}
			if after := partitionContent(t, dbPath, "queries-software"); after != before {
				t.Fatalf("an incomplete walk changed the stored report (%d rows now, App 099 installs %+v):\n%s", len(storedReportRows(t, dbPath, "queries-software")), softwareAuditOf(t, dbPath, "App 099"), out)
			}
			if titles := softwareAuditOf(t, dbPath, "App 099"); len(titles) != 1 || titles[0].Installs != 1 {
				t.Fatalf("software-audit --name \"App 099\" = %+v, want one install", titles)
			}
			if staged, _ := filepath.Glob(filepath.Join(dir, ninjaReportSpoolPattern)); len(staged) != 0 {
				t.Fatalf("the walk left its staging file behind: %v", staged)
			}
			f.answer(newFirst...)
			runReportSync(t, srv.URL, dbPath, "--resources", "queries-software")
			if got := rowValues(storedReportRows(t, dbPath, "queries-software"), "name"); len(got) != 151 || got[0] != "App 000" || got[149] != "App 149" || got[150] != "New Title" {
				t.Fatalf("a complete walk stored %d rows, want App 000..App 149 and New Title", len(got))
			}
		})
	}
}

// The adversarial --no-prune input: device 7 reports Alpha, Beta, Gamma; Beta is uninstalled;
// `sync --no-prune` stored Alpha, Gamma, Gamma (Beta gone, Gamma twice). Position-keyed rows
// have no identity to keep, so --no-prune does not apply to them: the whole walk replaces the
// report, the run says so, and the store holds Alpha and Gamma once each. A report whose rows
// carry their own id keeps --no-prune: a patch the report stopped returning stays.
func TestNoPruneDoesNotApplyToOrdinalReports(t *testing.T) {
	app := func(name string) string { return fmt.Sprintf(`{"deviceId":7,"name":%q,"version":"1"}`, name) }
	testenv.Isolate(t)
	f := newReportFake("/v2/queries/software", app("Alpha"), app("Beta"), app("Gamma"))
	srv := httptest.NewServer(f)
	defer srv.Close()
	dbPath := filepath.Join(t.TempDir(), "data.db")
	runReportSync(t, srv.URL, dbPath, "--resources", "queries-software", "--no-prune")
	f.answer(app("Alpha"), app("Gamma"))
	out := runReportSync(t, srv.URL, dbPath, "--resources", "queries-software", "--no-prune")
	if got := strings.Join(rowValues(storedReportRows(t, dbPath, "queries-software"), "name"), ","); got != "Alpha,Gamma" {
		t.Fatalf("after Beta was uninstalled, `sync --no-prune` stored %s, want Alpha,Gamma\n%s", got, out)
	}
	for _, name := range []string{"Alpha", "Gamma"} {
		if titles := softwareAuditOf(t, dbPath, name); len(titles) != 1 || titles[0].Installs != 1 {
			t.Fatalf("software-audit --name %s = %+v, want one install", name, titles)
		}
	}
	if !strings.Contains(out, `"reason":"no_prune_not_applicable"`) || !strings.Contains(out, "--no-prune does not apply to queries-software") {
		t.Fatalf("`sync --no-prune` did not say --no-prune does not apply to a position-keyed report:\n%s", out)
	}

	patches := newReportFake("/v2/queries/os-patches", patchRows(1, 2)...)
	psrv := httptest.NewServer(patches)
	defer psrv.Close()
	runReportSync(t, psrv.URL, dbPath, "--resources", "queries-os-patches")
	patches.answer(patchRows(2, 2)...)
	pout := runReportSync(t, psrv.URL, dbPath, "--resources", "queries-os-patches", "--no-prune")
	if rows := storedReportRows(t, dbPath, "queries-os-patches"); len(rows) != 2 || !hasRow(rows, "id", "patch-1") {
		t.Fatalf("--no-prune on a report with documented ids did not keep patch-1: %v", rows)
	}
	if strings.Contains(pout, "no_prune_not_applicable") {
		t.Fatalf("--no-prune was reported as not applying to a report with documented ids:\n%s", pout)
	}
}

// `workflow archive` walks every report complete and unfiltered, so an ordinal report it walks
// replaces the stored one: an upgraded Acme Agent is one install, not two. Archive never
// prunes otherwise, but it was never given --no-prune, so it does not report one.
func TestArchiveReplacesAnOrdinalReport(t *testing.T) {
	testenv.Isolate(t)
	f := newReportFake("/v2/queries/software", r1OldTool, acme(7, "1"))
	srv := httptest.NewServer(f)
	defer srv.Close()
	dbPath := filepath.Join(t.TempDir(), "data.db")
	runReportSync(t, srv.URL, dbPath, "--resources", "queries-software", "--full")
	f.answer(acme(7, "2"))
	stdout, stderr, err := runReportCmd(t, srv.URL, "workflow", "archive", "--db", dbPath, "--timeout", "0")
	if err != nil {
		t.Fatalf("workflow archive: %v\n%s%s", err, stdout, stderr)
	}
	if titles := softwareAuditOf(t, dbPath, "Acme Agent"); len(titles) != 1 || titles[0].Installs != 1 || strings.Join(titles[0].Versions, ",") != "2" {
		t.Fatalf("after archive, software-audit --name \"Acme Agent\" = %+v, want one install of version 2\n%s%s", titles, stdout, stderr)
	}
	if rows := storedReportRows(t, dbPath, "queries-software"); len(rows) != 1 {
		t.Fatalf("after archive, queries-software holds %d rows, want 1: %v", len(rows), rows)
	}
	out := stdout + stderr
	if !strings.Contains(out, `"event":"reconcile","resource":"queries-software","scope":"snapshot"`) || strings.Contains(out, "no_prune_not_applicable") {
		t.Fatalf("archive did not replace queries-software as an ordinary walk (it was never given --no-prune):\n%s", out)
	}
}
