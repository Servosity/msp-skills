// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (handfixes.json warranty-novels): one test per review finding
// class (holder, output sinks, MCP confirm, negative answers, pagination).

package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMCPWritesNeedConfirm(t *testing.T) {
	if inFreshProcess(t) {
		return
	}
	warrantyTestEnv(t, defaultFakeVendor())
	t.Setenv("WARRANTY_MCP_CHILD", "1")
	out, errOut, err := execWarranty(t, "", "lookup", "1ABC234", "--json")
	if err != nil {
		t.Fatalf("lookup: %v\n%s", err, errOut)
	}
	var run lookupRun
	decodeJSON(t, out, &run)
	if run.Saved || !strings.Contains(run.Note, "confirm") {
		t.Fatalf("an MCP lookup without confirm must preview: %+v", run)
	}
	if _, err := os.Stat(warrantyDBPath()); err == nil {
		t.Fatal("the preview wrote the mirror")
	}
	t.Setenv("WARRANTY_MCP_CHILD", "confirmed")
	out, _, err = execWarranty(t, "", "lookup", "1ABC234", "--confirm", "--json")
	decodeJSON(t, out, &run)
	if err != nil || !run.Saved {
		t.Fatalf("lookup --confirm over MCP must save: err=%v %+v", err, run)
	}
}

func TestNegativeVendorAnswerClearsCoverage(t *testing.T) {
	if inFreshProcess(t) {
		return
	}
	f := defaultFakeVendor()
	warrantyTestEnv(t, f)
	f.lenovo = func(r *http.Request) (int, string) {
		return 200, `{"Serial":"PF1ABCDE","Warranty":[{"ID":"BASE","Name":"Base","Start":"2024-12-01T00:00:00","End":"2026-12-01T00:00:00"}]}`
	}
	if _, errOut, err := execWarranty(t, "", "lookup", "PF1ABCDE", "--vendor", "lenovo", "--as-of", "2026-10-28", "--json"); err != nil {
		t.Fatalf("first lookup: %v\n%s", err, errOut)
	}
	f.lenovo = func(r *http.Request) (int, string) {
		return 200, `{"Serial":"PF1ABCDE","ErrorCode":100,"ErrorMessage":"Warranty data is not found"}`
	}
	if _, errOut, err := execWarranty(t, "", "lookup", "PF1ABCDE", "--vendor", "lenovo", "--as-of", "2026-10-28", "--json"); err != nil {
		t.Fatalf("second lookup: %v\n%s", err, errOut)
	}
	out, _, _ := execWarranty(t, "", "report", "--as-of", "2026-10-28", "--json")
	var rep reportView
	decodeJSON(t, out, &rep)
	if len(rep.Devices) != 0 || rep.Unverified != 1 {
		t.Fatalf("stale coverage still reported after ErrorCode 100: %+v", rep)
	}
	out, _, _ = execWarranty(t, "", "events", "list", "--json")
	if !strings.Contains(out, `"coverage.changed"`) {
		t.Fatalf("clearing coverage must raise coverage.changed: %s", out)
	}
	// A run that could not call the vendor keeps the last verdict.
	t.Setenv("WARRANTY_LENOVO_CLIENT_KEY", "")
	_, _, _ = execWarranty(t, "", "lookup", "PF1ABCDE", "--vendor", "lenovo", "--json")
	out, _, _ = execWarranty(t, "", "unresolved", "--json")
	if !strings.Contains(out, `"reason": "vendor_unconfigured"`) {
		t.Fatalf("unresolved should name the unconfigured vendor: %s", out)
	}
}

func TestAppleCoverageIsPaged(t *testing.T) {
	f := defaultFakeVendor()
	warrantyTestEnv(t, f)
	f.apple = func(r *http.Request) (int, string) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/appleCareCoverage") && r.URL.Query().Get("cursor") == "":
			return 200, `{"data":[{"id":"limited","attributes":{"status":"ACTIVE","description":"Limited Warranty","endDateTime":"2026-11-01T00:00:00Z"}}],"meta":{"paging":{"nextCursor":"N"}}}`
		case strings.HasSuffix(r.URL.Path, "/appleCareCoverage"):
			return 200, `{"data":[{"id":"abe","attributes":{"status":"ACTIVE","paymentType":"ABE_SUBSCRIPTION","description":"AppleCare+ for Business","endDateTime":null}}]}`
		}
		return 200, `{"data":{"id":"C02XG0FDH7JY","attributes":{"deviceModel":"MacBook Air"}}}`
	}
	out, errOut, err := execWarranty(t, "", "lookup", "C02XG0FDH7JY", "--vendor", "apple", "--as-of", "2026-10-28", "--no-save", "--json")
	if err != nil {
		t.Fatalf("lookup: %v\n%s", err, errOut)
	}
	var run lookupRun
	decodeJSON(t, out, &run)
	if len(run.Results) != 1 || !run.Results[0].CoverageOpen || run.Results[0].Status != "covered" || len(run.Results[0].Lines) != 2 {
		t.Fatalf("second coverage page ignored: %+v", run.Results)
	}
	if run.Calls["apple"] != 3 {
		t.Fatalf("apple calls = %d, want 3 (two coverage pages and the device)", run.Calls["apple"])
	}
	// An agent envelope reports live only when a vendor was called.
	t.Setenv("WARRANTY_APPLE_CLIENT_ID", "")
	out, _, _ = execWarranty(t, "", "lookup", "C02XG0FDH7JY", "--vendor", "apple", "--no-save", "--agent")
	var env struct {
		Meta struct {
			Source string `json:"source"`
		} `json:"meta"`
	}
	_ = json.Unmarshal([]byte(out), &env)
	if env.Meta.Source == "live" {
		t.Fatalf("meta.source is live with no vendor call: %s", out)
	}
}

func TestMCPPreviewWritesNoStore(t *testing.T) {
	home, _ := warrantyTestEnv(t, defaultFakeVendor())
	t.Setenv("WARRANTY_MCP_CHILD", "1")
	if _, errOut, err := execWarranty(t, "", "lookup", "ABCDEFGHIJ", "--json"); err != nil {
		t.Fatalf("lookup: %v\n%s", err, errOut)
	}
	_ = filepath.WalkDir(home, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".db") {
			t.Errorf("an MCP preview created %s", strings.TrimPrefix(p, home))
		}
		return nil
	})
}

func TestCoverageRecoveryAndVendorChangeRaiseEvents(t *testing.T) {
	if inFreshProcess(t) {
		return
	}
	f := defaultFakeVendor()
	warrantyTestEnv(t, f)
	good := `{"Serial":"PF1ABCDE","Warranty":[{"ID":"1EZ","Start":"2025-01-01T00:00:00","End":"2028-12-01T00:00:00"}]}`
	gone := `{"Serial":"PF1ABCDE","ErrorCode":100}`
	answers := []string{good, gone, good, gone, good}
	for i, a := range answers {
		body := a
		f.lenovo = func(r *http.Request) (int, string) { return 200, body }
		if _, errOut, err := execWarranty(t, "", "lookup", "PF1ABCDE", "--vendor", "lenovo", "--as-of", "2026-10-28", "--json"); err != nil {
			t.Fatalf("lookup %d: %v\n%s", i, err, errOut)
		}
	}
	out, _, _ := execWarranty(t, "", "events", "list", "--type", "coverage.changed", "--json")
	var evs struct {
		Events []map[string]any `json:"events"`
	}
	decodeJSON(t, out, &evs)
	if len(evs.Events) != 4 {
		t.Fatalf("want 4 coverage.changed events (lost, back, lost, back), got %d: %s", len(evs.Events), out)
	}
	// A declared vendor change drops the old vendor's coverage.
	t.Setenv("WARRANTY_APPLE_CLIENT_ID", "")
	if _, errOut, err := execWarranty(t, "", "lookup", "PF1ABCDE", "--vendor", "apple", "--json"); err != nil {
		t.Fatalf("vendor change lookup: %v\n%s", err, errOut)
	}
	out, _, _ = execWarranty(t, "", "report", "--as-of", "2026-10-28", "--within", "1500d", "--json")
	var rep reportView
	decodeJSON(t, out, &rep)
	if len(rep.Devices) != 0 || rep.Unverified != 1 {
		t.Fatalf("Lenovo coverage survived a change to Apple: %+v", rep)
	}
}

// Apple documents status INACTIVE with an end date for coverage that ended.
// Input: one INACTIVE AppleCare line that ended in 2023. The device is expired,
// in report and lifecycle, with its event; it does not read as no coverage data.
func TestInactiveCoverageThatEndedReadsAsExpired(t *testing.T) {
	if inFreshProcess(t) {
		return
	}
	f := defaultFakeVendor()
	warrantyTestEnv(t, f)
	f.apple = func(r *http.Request) (int, string) {
		return 200, `{"data":[{"type":"appleCareCoverage","id":"C02XG0FDH7JY","attributes":{"status":"INACTIVE","paymentType":"NONE","description":"Limited Warranty","startDateTime":"2022-02-02T00:00:00Z","endDateTime":"2023-02-02T00:00:00Z","isRenewable":false,"isCanceled":false}}],"meta":{"paging":{"limit":100}}}`
	}
	if _, errOut, err := execWarranty(t, "", "lookup", "C02XG0FDH7JY", "--vendor", "apple", "--customer", "Acme", "--as-of", "2026-10-28", "--json"); err != nil {
		t.Fatalf("lookup: %v\n%s", err, errOut)
	}
	out, _, err := execWarranty(t, "", "report", "--as-of", "2026-10-28", "--status", "expired", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var rep reportView
	decodeJSON(t, out, &rep)
	if len(rep.Devices) != 1 || rep.Devices[0].Status != "expired" || rep.Devices[0].CoverageEnd != "2023-02-02" || rep.Devices[0].LastCoverage == "" || rep.Unverified != 0 {
		t.Fatalf("an ended INACTIVE line must read as expired: %s", out)
	}
}

// Apple documents INACTIVE with an end date. Input: a line ending today, judged as of
// tomorrow, as of today and years before it ended. Only a date past the end reads as
// expired, in lookup, report and lifecycle, and an inactive line never reads as covered.
func TestInactiveCoverageIsJudgedOnTheAsOfDate(t *testing.T) {
	if inFreshProcess(t) {
		return
	}
	now := time.Now().UTC()
	end := now.Format("2006-01-02")
	tomorrow := now.AddDate(0, 0, 1).Format("2006-01-02")
	f := defaultFakeVendor()
	warrantyTestEnv(t, f)
	f.apple = func(r *http.Request) (int, string) {
		return 200, `{"data":[{"type":"appleCareCoverage","id":"C02XG0FDH7JY","attributes":{"status":"INACTIVE","paymentType":"NONE","description":"Limited Warranty","startDateTime":"2022-02-02T00:00:00Z","endDateTime":"` + end + `T00:00:00Z","isRenewable":false,"isCanceled":false}}],"meta":{"paging":{"limit":100}}}`
	}
	out, errOut, err := execWarranty(t, "", "lookup", "C02XG0FDH7JY", "--vendor", "apple", "--customer", "Acme", "--as-of", tomorrow, "--json")
	if err != nil {
		t.Fatalf("lookup: %v\n%s", err, errOut)
	}
	if !strings.Contains(out, `"status": "expired"`) && !strings.Contains(out, `"status":"expired"`) {
		t.Errorf("lookup as of the day after the end must read expired: %s", out)
	}
	for _, c := range []struct {
		asOf       string
		expired    int
		unverified int
	}{{tomorrow, 1, 0}, {end, 0, 1}, {"2021-01-01", 0, 1}} {
		out, _, err := execWarranty(t, "", "report", "--as-of", c.asOf, "--within", "90d", "--json")
		if err != nil {
			t.Fatal(err)
		}
		var rep reportView
		decodeJSON(t, out, &rep)
		if rep.Counts["expired"] != c.expired || rep.Unverified != c.unverified || rep.Counts["expiring"] != 0 {
			t.Errorf("report as of %s: want %d expired, %d unverified: %s", c.asOf, c.expired, c.unverified, out)
		}
		out, _, err = execWarranty(t, "", "lifecycle", "--as-of", c.asOf, "--json")
		if err != nil {
			t.Fatal(err)
		}
		if c.expired == 1 && !strings.Contains(out, `"expired": 1`) && !strings.Contains(out, `"expired":1`) {
			t.Errorf("lifecycle as of %s: want the device expired: %s", c.asOf, out)
		}
		if strings.Contains(out, `"covered": 1`) || strings.Contains(out, `"covered":1`) {
			t.Errorf("lifecycle as of %s: an inactive line read as covered: %s", c.asOf, out)
		}
	}
}

// Apple's AppleCareCoverageResponse allows an empty data array. Input: that answer,
// saved. report counts the device unverified, so unresolved must list it with a
// reason and never say every device has current coverage data.
func TestEmptyCoverageAnswerIsUnresolved(t *testing.T) {
	if inFreshProcess(t) {
		return
	}
	f := defaultFakeVendor()
	warrantyTestEnv(t, f)
	f.apple = func(r *http.Request) (int, string) {
		return 200, `{"data":[],"links":{"self":"https://api-business.apple.com/v1/orgDevices/C02XG0FDH7JY/appleCareCoverage"}}`
	}
	if _, errOut, err := execWarranty(t, "", "lookup", "C02XG0FDH7JY", "--vendor", "apple", "--customer", "Acme", "--json"); err != nil {
		t.Fatalf("lookup: %v\n%s", err, errOut)
	}
	out, _, err := execWarranty(t, "", "unresolved", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "C02XG0FDH7JY") {
		t.Errorf("unresolved left out a device report counts as unverified: %s", out)
	}
}

// A saving lookup with a future --as-of must not raise or burn today's alerts.
// Input: coverage that ends in 30 days, looked up as of 60 days from now. No
// expired event is raised for a date that has not come, and a plain lookup today
// raises exactly one expiring alert.
func TestFutureAsOfDoesNotBurnEvents(t *testing.T) {
	if inFreshProcess(t) {
		return
	}
	now := time.Now().UTC()
	end := now.AddDate(0, 0, 30).Format("2006-01-02")
	f := defaultFakeVendor()
	warrantyTestEnv(t, f)
	f.lenovo = func(r *http.Request) (int, string) {
		return 200, `{"Serial":"PF1ABCDE","Warranty":[{"ID":"1EZ","Start":"2025-01-01T00:00:00","End":"` + end + `T00:00:00"}]}`
	}
	for _, d := range []int{60} {
		asOf := now.AddDate(0, 0, d).Format("2006-01-02")
		if _, errOut, err := execWarranty(t, "", "lookup", "PF1ABCDE", "--vendor", "lenovo", "--as-of", asOf, "--json"); err != nil {
			t.Fatalf("lookup as of %s: %v\n%s", asOf, err, errOut)
		}
	}
	count := func(typ string) int {
		out, _, _ := execWarranty(t, "", "events", "list", "--type", typ, "--json")
		var evs struct {
			Events []map[string]any `json:"events"`
		}
		decodeJSON(t, out, &evs)
		return len(evs.Events)
	}
	if n := count("coverage.expired"); n != 0 {
		t.Fatalf("a future --as-of raised %d expired events", n)
	}
	if _, errOut, err := execWarranty(t, "", "lookup", "PF1ABCDE", "--vendor", "lenovo", "--json"); err != nil {
		t.Fatalf("lookup today: %v\n%s", err, errOut)
	}
	if n := count("coverage.expiring"); n != 1 || count("coverage.expired") != 0 {
		t.Fatalf("today's expiring alert: want 1, got %d", n)
	}
}

// A saving lookup with a past --as-of judges the device on a date that is not
// today, so it raises no dated alert. Input: coverage that ended 34 days ago,
// looked up as of 64 days ago within 90d. Neither expiring nor expired is raised
// by that lookup; a plain lookup today raises exactly one expired event.
func TestPastAsOfRaisesNoDatedEvents(t *testing.T) {
	if inFreshProcess(t) {
		return
	}
	now := time.Now().UTC()
	end := now.AddDate(0, 0, -34).Format("2006-01-02")
	f := defaultFakeVendor()
	warrantyTestEnv(t, f)
	f.lenovo = func(r *http.Request) (int, string) {
		return 200, `{"Serial":"PF1ABCDE","Warranty":[{"ID":"1EZ","Start":"2023-01-01T00:00:00","End":"` + end + `T00:00:00"}]}`
	}
	asOf := now.AddDate(0, 0, -64).Format("2006-01-02")
	if _, errOut, err := execWarranty(t, "", "lookup", "PF1ABCDE", "--vendor", "lenovo", "--as-of", asOf, "--within", "90d", "--json"); err != nil {
		t.Fatalf("lookup as of %s: %v\n%s", asOf, err, errOut)
	}
	count := func(typ string) int {
		out, _, _ := execWarranty(t, "", "events", "list", "--type", typ, "--json")
		var evs struct {
			Events []map[string]any `json:"events"`
		}
		decodeJSON(t, out, &evs)
		return len(evs.Events)
	}
	if n, m := count("coverage.expiring"), count("coverage.expired"); n != 0 || m != 0 {
		t.Fatalf("a past --as-of raised %d expiring and %d expired events", n, m)
	}
	if _, errOut, err := execWarranty(t, "", "lookup", "PF1ABCDE", "--vendor", "lenovo", "--json"); err != nil {
		t.Fatalf("lookup today: %v\n%s", err, errOut)
	}
	if n := count("coverage.expired"); n != 1 || count("coverage.expiring") != 0 {
		t.Fatalf("today's expired alert: want 1, got %d", n)
	}
}

// unresolved judges saved lines on today, as report does. Input: a device whose
// saved summary columns were judged before its inactive line ended (empty), while
// its lines show coverage that has since ended. report calls it expired, so
// unresolved must not call it no_coverage.
func TestUnresolvedJudgesSavedLinesToday(t *testing.T) {
	if inFreshProcess(t) {
		return
	}
	f := defaultFakeVendor()
	warrantyTestEnv(t, f)
	if _, errOut, err := execWarranty(t, "", "lookup", "PF1ABCDE", "--vendor", "lenovo", "--json"); err != nil {
		t.Fatalf("lookup: %v\n%s", err, errOut)
	}
	db, err := openWarrantyWrite(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`UPDATE warranty_devices SET coverage_end = '', coverage_open = 0 WHERE serial = 'PF1ABCDE'`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	out, _, err := execWarranty(t, "", "unresolved", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "no_coverage") {
		t.Errorf("unresolved trusted a summary judged on an earlier date: %s", out)
	}
}
