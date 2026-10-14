// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"threatdown-pp-cli/internal/client"
	"threatdown-pp-cli/internal/cliutil/testenv"
	"threatdown-pp-cli/internal/fleet"
)

func TestSelectPending(t *testing.T) {
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	iso := func(h int) string { return now.Add(-time.Duration(h) * time.Hour).Format(time.RFC3339) }
	jobs := []jobRow{
		{ID: "timedout", AccountID: "a1", Command: "command.threat.scan", MachineName: "pc1", MachineID: "m1", Status: 16, IssuedAt: iso(30), Data: json.RawMessage(`"{\"scan_settings\":{\"type\":\"ThreatScan\",\"remove\":true}}"`)},
		{ID: "fresh-sent", AccountID: "a1", Command: "command.threat.scan", MachineName: "pc2", Status: 2, IssuedAt: iso(0)},
		{ID: "old-sent", AccountID: "a1", Command: "command.edr.isolation", MachineName: "pc3", MachineID: "m3", Status: 2, IssuedAt: iso(5)},
		{ID: "done", AccountID: "a1", Command: "command.threat.scan", MachineName: "pc4", Status: 32, IssuedAt: iso(3)},
		{ID: "ancient", AccountID: "a1", Command: "command.threat.scan", MachineName: "pc5", Status: 128, IssuedAt: iso(24 * 30)},
	}
	sites := map[string]fleet.Site{"a1": {Name: "Acme"}}
	rows := selectPending(jobs, sites, now, now.Add(-7*24*time.Hour), time.Hour, "")
	got := []string{}
	for _, r := range rows {
		got = append(got, r.JobID)
	}
	if strings.Join(got, ",") != "old-sent,timedout" {
		t.Fatalf("pending = %v (completed, too-fresh and out-of-window jobs must be excluded)", got)
	}
	// Re-issue targets the job's own machine id and keeps its scan settings.
	if rows[1].Reissue != `echo '[{"endpoint_id":"m1","account_id":"a1"}]' | threatdown-cli respond scan --stdin --scan-type ThreatScan --remove` || !rows[1].Failed || rows[1].Status != "TIMED_OUT" {
		t.Fatalf("timed-out row = %+v", rows[1])
	}
	// An isolation whose settings are unreadable gets a note, never a guess.
	if rows[0].Reissue != "" || rows[0].ReissueNote == "" {
		t.Fatalf("isolate without data = %+v", rows[0])
	}
	if only := selectPending(jobs, sites, now, now.Add(-7*24*time.Hour), time.Hour, "isolate"); len(only) != 1 || only[0].JobID != "old-sent" {
		t.Fatalf("--command isolate filter = %+v", only)
	}
	groups := groupPending(rows)
	if len(groups) != 2 || groups[0].Waiting+groups[1].Waiting != 1 || groups[0].Failed+groups[1].Failed != 1 {
		t.Fatalf("groups = %+v", groups)
	}
}

func TestSeatFlags(t *testing.T) {
	r := seatRow{WorkstationAllocated: 2, WorkstationDeployed: 4, WorkstationActive: 3, ServerAllocated: 3, ServerDeployed: 1, ServerActive: 1}
	got := strings.Join(seatFlags(r), " ")
	want := "workstation-under-allocated:2-unlicensed workstation-stale:1 server-over-allocated:2-unused"
	if got != want {
		t.Fatalf("flags = %q want %q", got, want)
	}
	if f := seatFlags(seatRow{WorkstationAllocated: 1, WorkstationDeployed: 1, WorkstationActive: 1}); len(f) != 0 {
		t.Fatalf("a balanced site needs no flags, got %v", f)
	}
}

func TestRespondJobData(t *testing.T) {
	d, err := respondJobData("isolate", "network", "", false)
	if err != nil || d["network"] != true || d["process"] != false || d["desktop"] != false {
		t.Fatalf("isolate network-only = %v %v", d, err)
	}
	if _, err := respondJobData("isolate", "network,bogus", "", false); err == nil {
		t.Fatal("unknown isolation mode must error")
	}
	if _, err := respondJobData("isolate", ",", "", false); err == nil {
		t.Fatal("empty isolation mode must error")
	}
	d, _ = respondJobData("scan", "", "HyperScan", true)
	ss := d["scan_settings"].(map[string]any)
	if ss["type"] != "HyperScan" || ss["remove"] != true {
		t.Fatalf("scan data = %v", d)
	}
	if _, err := respondJobData("scan", "", "CustomScan", false); err == nil {
		t.Fatal("CustomScan needs paths and is not offered")
	}
	if d, _ := respondJobData("reboot", "", "", false); d != nil {
		t.Fatalf("reboot sends no data, got %v", d)
	}
}

func TestReadEndpointRefs(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{`[{"endpoint_id":"m1","account_id":"a1"},{"id":"m2","account_id":"a2"},{"id":"m3"}]`, 2},
		{`{"endpoints":[{"endpoint_id":"m1","account_id":"a1"}],"count":1}`, 1},
		{`{"queue":[{"endpoint_id":"m1","account_id":"a1"}]}`, 1},
	}
	for _, tc := range cases {
		refs, err := readEndpointRefs(strings.NewReader(tc.in))
		if err != nil || len(refs) != tc.want {
			t.Fatalf("%s: refs=%v err=%v", tc.in, refs, err)
		}
	}
	for _, bad := range []string{`nope`, `[]`, `{"endpoints":[{"id":"m1"}]}`} {
		if _, err := readEndpointRefs(strings.NewReader(bad)); err == nil {
			t.Fatalf("%s should be rejected", bad)
		}
	}
}

func TestMatchSite(t *testing.T) {
	sites := []fleet.Site{{ID: "s1", AccountID: "a1", Name: "Acme Corp"}, {ID: "s2", AccountID: "a2", Name: "Acme Labs"}, {ID: "s3", AccountID: "a3", Name: "Globex"}}
	if s, _, ok := matchSite(sites, "globex"); !ok || s.ID != "s3" {
		t.Fatal("unique substring should match")
	}
	if s, _, ok := matchSite(sites, "ACME CORP"); !ok || s.ID != "s1" {
		t.Fatal("exact name should win over an ambiguous substring")
	}
	if s, _, ok := matchSite(sites, "a2"); !ok || s.ID != "s2" {
		t.Fatal("account id should match")
	}
	if _, cands, ok := matchSite(sites, "acme"); ok || len(cands) != 2 {
		t.Fatalf("ambiguous match should list both candidates, got %v", cands)
	}
	if _, cands, ok := matchSite(sites, "initech"); ok || len(cands) != 0 {
		t.Fatal("no match should report nothing")
	}
}

// fakeOneView serves just enough of OneView for the respond write path and
// records every jobs/bulk body it receives.
func fakeOneView(t *testing.T) (*httptest.Server, *[]map[string]any) {
	t.Helper()
	var mu sync.Mutex
	var bulk []map[string]any
	endpoint := func(id, acct, host string, isolated bool) map[string]any {
		return map[string]any{"display_name": host,
			"agent":   map[string]any{"machine_id": id, "account_id": acct, "host_name": host},
			"machine": map[string]any{"id": id, "account_id": acct, "isolated": isolated, "online": true, "last_day_seen": time.Now().UTC().Format(time.RFC3339)}}
	}
	all := []map[string]any{endpoint("m1", "a1", "LAPTOP-042", false), endpoint("m2", "a1", "LAPTOP-0420", false), endpoint("m3", "a1", "SRV-01", true),
		endpoint("m4", "a1", "DESKTOP-01", false), endpoint("m5", "a2", "DESKTOP-01", false), endpoint("m6", "a1", "REJECTME", false)}
	for i := 0; i < 150; i++ {
		all = append(all, endpoint(fmt.Sprintf("bulk-%03d", i), "a3", fmt.Sprintf("BULK-%03d", i), false))
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/oneview/oauth2/token" {
			_, _ = io.WriteString(w, `{"access_token":"tok","expires_in":3600,"token_type":"Bearer","scope":"read write execute"}`)
			return
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"statusCode":401,"error":"Unauthorized","message":""}`)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch r.URL.Path {
		case "/oneview/v1/sites":
			_, _ = io.WriteString(w, `{"sites":[{"id":"s1","account_id":"a1","company_name":"Acme"},{"id":"s2","account_id":"a2","company_name":"Globex"},{"id":"s3","account_id":"a3","company_name":"Bulkco"},{"id":"s4","account_id":"a4","company_name":"Brokenco"}]}`)
		case "/oneview/v1/endpoints":
			q, _ := body["name_multi_search"].(string)
			accts := map[string]bool{}
			for _, a := range body["account_ids"].([]any) {
				accts[a.(string)] = true
			}
			out := []any{}
			for _, e := range all {
				host := e["display_name"].(string)
				if !accts[e["machine"].(map[string]any)["account_id"].(string)] {
					continue
				}
				if q == "" || strings.Contains(strings.ToLower(host), strings.ToLower(q)) {
					out = append(out, e)
				}
			}
			if accts["a4"] && q == "" {
				// One readable endpoint plus one with no account id.
				out = append(out, endpoint("m9", "a4", "BROKEN-01", false), map[string]any{"machine": map[string]any{"id": "m10"}})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"endpoints": out, "total_count": len(out), "next_cursor": ""})
		case "/oneview/v1/jobs/bulk":
			mu.Lock()
			bulk = append(bulk, body)
			mu.Unlock()
			w.WriteHeader(http.StatusCreated)
			eps := body["endpoints"].([]any)
			if len(eps) >= 100 {
				_, _ = io.WriteString(w, `{"jobs":[],"errors":[],"total_count":0,"correlation_id":"too-big"}`)
				return
			}
			jobs := []map[string]string{}
			errs := []map[string]string{}
			for _, e := range eps {
				id := e.(map[string]any)["id"].(string)
				if id == "m6" {
					errs = append(errs, map[string]string{"id": id, "account_id": "a1"})
					continue
				}
				jobs = append(jobs, map[string]string{"machine_id": id, "job_id": "job-" + id})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"jobs": jobs, "errors": errs, "total_count": len(jobs), "correlation_id": fmt.Sprintf("c%d", len(*&bulk))})
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"statusCode":404}`)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &bulk
}

func runRoot(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := RootCmd()
	cmd.SetArgs(args)
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	err := cmd.Execute()
	return out.String(), err
}

func TestRespondIsolatePreviewThenConfirm(t *testing.T) {
	testenv.Isolate(t)
	srv, bulk := fakeOneView(t)
	t.Setenv("THREATDOWN_BASE_URL", srv.URL+"/oneview")
	t.Setenv("THREATDOWN_TOKEN_URL", srv.URL+"/oneview/oauth2/token")
	t.Setenv("THREATDOWN_CLIENT_ID", "cid")
	t.Setenv("THREATDOWN_CLIENT_SECRET", "secret")

	// Preview: exact host match only (LAPTOP-0420 must not be targeted), the
	// isolated server is skipped, and nothing is sent.
	out, err := runRoot(t, "respond", "isolate", "--host", "laptop-042,SRV-01", "--json")
	if err != nil {
		t.Fatalf("preview error: %v", err)
	}
	var view respondView
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("preview not JSON: %v\n%s", err, out)
	}
	if view.Issued || view.WouldIssue != 1 || view.Skipped != 1 || len(view.Targets) != 2 {
		t.Fatalf("preview view = %+v", view)
	}
	for _, tg := range view.Targets {
		if tg.HostName == "LAPTOP-0420" {
			t.Fatal("prefix match must not become a target")
		}
	}
	if len(*bulk) != 0 {
		t.Fatalf("preview sent %d jobs", len(*bulk))
	}

	// Confirm: one jobs/bulk with the documented isolation body.
	out, err = runRoot(t, "respond", "isolate", "--host", "LAPTOP-042", "--mode", "network", "--confirm", "--json")
	if err != nil {
		t.Fatalf("confirm error: %v\n%s", err, out)
	}
	if len(*bulk) != 1 {
		t.Fatalf("want 1 bulk call, got %d", len(*bulk))
	}
	b := (*bulk)[0]
	data, _ := b["data"].(map[string]any)
	eps, _ := b["endpoints"].([]any)
	if b["command"] != "command.edr.isolation" || data["network"] != true || data["process"] != false || len(eps) != 1 {
		t.Fatalf("bulk body = %v", b)
	}
	if !strings.Contains(out, `"job_id": "job-m1"`) || !strings.Contains(out, `"issued": true`) {
		t.Fatalf("confirm output missing job id:\n%s", out)
	}

	// Blast-radius guard: two exact targets with --max-targets 1 refuses.
	if _, err := runRoot(t, "respond", "reboot", "--host", "LAPTOP-042,LAPTOP-0420", "--max-targets", "1", "--confirm", "--json"); err == nil {
		t.Fatal("max-targets guard should refuse")
	}
	if len(*bulk) != 1 {
		t.Fatal("refused run must not send a job")
	}
}

func TestRespondSafetyGuards(t *testing.T) {
	testenv.Isolate(t)
	srv, bulk := fakeOneView(t)
	t.Setenv("THREATDOWN_BASE_URL", srv.URL+"/oneview")
	t.Setenv("THREATDOWN_TOKEN_URL", srv.URL+"/oneview/oauth2/token")
	t.Setenv("THREATDOWN_CLIENT_ID", "cid")
	t.Setenv("THREATDOWN_CLIENT_SECRET", "secret")

	// A host name present in two sites is refused without --site.
	if _, err := runRoot(t, "respond", "scan", "--host", "DESKTOP-01", "--confirm", "--json"); err == nil || !strings.Contains(err.Error(), "matches 2 endpoints") {
		t.Fatalf("ambiguous host should be refused, got %v", err)
	}
	// --site narrows it to one endpoint.
	out, err := runRoot(t, "respond", "scan", "--host", "DESKTOP-01", "--site", "Globex", "--json")
	if err != nil || !strings.Contains(out, `"would_issue": 1`) || !strings.Contains(out, `"endpoint_id": "m5"`) {
		t.Fatalf("site-scoped preview = %v\n%s", err, out)
	}
	// Stdin refs outside --site scope are not targeted.
	refs := `[{"endpoint_id":"m5","account_id":"a2"},{"endpoint_id":"m4","account_id":"a1"}]`
	cmd := RootCmd()
	cmd.SetArgs([]string{"respond", "scan", "--stdin", "--site", "Acme", "--json"})
	cmd.SetIn(strings.NewReader(refs))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	r, w, _ := os.Pipe()
	_, _ = w.WriteString(refs)
	_ = w.Close()
	oldStdin := os.Stdin
	os.Stdin = r
	err = cmd.Execute()
	os.Stdin = oldStdin
	if err != nil || !strings.Contains(buf.String(), `"would_issue": 1`) || !strings.Contains(buf.String(), "account not in --site scope") {
		t.Fatalf("stdin scope = %v\n%s", err, buf.String())
	}
	if len(*bulk) != 0 {
		t.Fatalf("previews sent %d jobs", len(*bulk))
	}
	// A stdin ref whose account listing holds an unreadable record is refused
	// before anything is sent, even though the requested endpoint is readable.
	cmd = RootCmd()
	cmd.SetArgs([]string{"respond", "scan", "--stdin", "--confirm", "--json"})
	buf.Reset()
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	r, w, _ = os.Pipe()
	go func() { _, _ = w.WriteString(`[{"endpoint_id":"m9","account_id":"a4"}]`); _ = w.Close() }()
	os.Stdin = r
	err = cmd.Execute()
	os.Stdin = oldStdin
	if err == nil || !strings.Contains(err.Error(), "unreadable") || len(*bulk) != 0 {
		t.Fatalf("unreadable account listing should refuse stdin targeting, got %v (%d jobs sent)", err, len(*bulk))
	}
	// A job OneView rejects is reported and exits non-zero.
	out, err = runRoot(t, "respond", "scan", "--host", "REJECTME", "--confirm", "--json")
	if err == nil || !strings.Contains(out, `"action": "rejected"`) || !strings.Contains(out, `"issued": false`) {
		t.Fatalf("rejected job = %v\n%s", err, out)
	}
}

func TestRespondBatchesBulkJobs(t *testing.T) {
	testenv.Isolate(t)
	srv, bulk := fakeOneView(t)
	t.Setenv("THREATDOWN_BASE_URL", srv.URL+"/oneview")
	t.Setenv("THREATDOWN_TOKEN_URL", srv.URL+"/oneview/oauth2/token")
	t.Setenv("THREATDOWN_CLIENT_ID", "cid")
	t.Setenv("THREATDOWN_CLIENT_SECRET", "secret")
	refs := make([]string, 0, 150)
	for i := 0; i < 150; i++ {
		refs = append(refs, fmt.Sprintf(`{"endpoint_id":"bulk-%03d","account_id":"a3"}`, i))
	}
	r, w, _ := os.Pipe()
	go func() { // a pipe holds only a few KB until it is read
		_, _ = w.WriteString("[" + strings.Join(refs, ",") + "]")
		_ = w.Close()
	}()
	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()
	out, err := runRoot(t, "respond", "update", "--stdin", "--max-targets", "200", "--confirm", "--json")
	if err != nil {
		t.Fatalf("batched issue error: %v\n%s", err, out)
	}
	if len(*bulk) != 2 {
		t.Fatalf("150 targets should go out in 2 batches, got %d", len(*bulk))
	}
	for _, b := range *bulk {
		if n := len(b["endpoints"].([]any)); n >= 100 {
			t.Fatalf("batch of %d endpoints exceeds the vendor's 99 limit", n)
		}
	}
	if !strings.Contains(out, `"issued_count": 150`) {
		t.Fatalf("all 150 jobs should be reconciled:\n%s", out[:400])
	}
}

func TestSearchPagesRejectsMissingCollection(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/oneview/oauth2/token" {
			_, _ = io.WriteString(w, `{"access_token":"tok","expires_in":3600,"token_type":"Bearer"}`)
			return
		}
		_, _ = io.WriteString(w, `{"dry_run":true}`)
	}))
	defer srv.Close()
	t.Setenv("THREATDOWN_BASE_URL", srv.URL+"/oneview")
	t.Setenv("THREATDOWN_TOKEN_URL", srv.URL+"/oneview/oauth2/token")
	t.Setenv("THREATDOWN_CLIENT_ID", "cid")
	t.Setenv("THREATDOWN_CLIENT_SECRET", "secret")
	flags := &rootFlags{timeout: 10 * time.Second}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := searchPages(context.Background(), c, "/v1/endpoints", map[string]any{"account_ids": []string{"a1"}}, "endpoints", 5); err == nil {
		t.Fatal("a response without the endpoints collection must be an error, not an empty fleet")
	}
}

func TestStopFallback(t *testing.T) {
	cases := []struct {
		err  error
		stop bool
	}{
		{authErr(errors.New("401")), true},
		{notFoundErr(errors.New("no site")), true},
		{usageErr(errors.New("bad flag")), true},
		{apiErr(errors.New("500")), false},
		{rateLimitErr(errors.New("429")), false},
		{errors.New("dial tcp: refused"), false},
		// Raw client errors are classified first: auth never falls back.
		{errors.New("GET /v1/sites returned HTTP 403: forbidden"), true},
		{errors.New("POST /oneview/oauth2/token returned HTTP 401: invalid_client"), true},
		{fmt.Errorf("token: %w", client.ErrPlaceholderCredential), true},
		{errors.New("POST /v1/endpoints returned HTTP 503: unavailable"), false},
	}
	for _, tc := range cases {
		if got := stopFallback(tc.err); got != tc.stop {
			t.Errorf("stopFallback(%v) = %v, want %v", tc.err, got, tc.stop)
		}
	}
}

func TestScopeSitesFilterWithNoSitesIsNotFound(t *testing.T) {
	if got, err := scopeSites(nil, ""); err != nil || len(got) != 0 {
		t.Fatalf("no filter on an empty inventory is fine: %v %v", got, err)
	}
	_, err := scopeSites(nil, "Acme")
	var ce *cliError
	if !errors.As(err, &ce) || ce.code != 3 {
		t.Fatalf("a --site filter with nothing to match must exit 3, got %v", err)
	}
	if got, err := scopeSites([]fleet.Site{{ID: "s1", Name: "Acme Corp"}}, "acme"); err != nil || len(got) != 1 {
		t.Fatalf("a matching filter keeps the site: %v %v", got, err)
	}
}

func TestValidRecords(t *testing.T) {
	raws := []json.RawMessage{json.RawMessage(`{"id":"d1","account_id":"a1"}`), json.RawMessage(`null`), json.RawMessage(`{"id":"d2"}`), json.RawMessage(`[]`), json.RawMessage(`{"id":"d3","account_id":"a1","status":[]}`)}
	if kept, bad := validDetections(raws); len(kept) != 1 || bad != 4 {
		t.Fatalf("detections kept %d bad %d, want 1 and 4", len(kept), bad)
	}
	jobs := []json.RawMessage{json.RawMessage(`{"id":"j1","account_id":"a1","status":32}`), json.RawMessage(`{"id":"j2","account_id":"a1","status":"done"}`)}
	if kept, bad := validJobs(jobs); len(kept) != 1 || bad != 1 {
		t.Fatalf("jobs kept %d bad %d, want 1 and 1", len(kept), bad)
	}
}

func TestReissueFlags(t *testing.T) {
	cases := []struct {
		verb, data, want string
		ok               bool
	}{
		{"isolate", `"{\"network\":true,\"process\":false,\"desktop\":false}"`, " --mode network", true},
		{"isolate", `{"network":true,"desktop":true}`, " --mode network,desktop", true},
		{"isolate", ``, "", false},
		{"isolate", `"{}"`, "", false},
		{"scan", `"{\"scan_settings\":{\"type\":\"HyperScan\",\"remove\":false}}"`, " --scan-type HyperScan", true},
		{"scan", `"garbage"`, "", false},
		{"reboot", ``, "", true},
		{"reboot", `"{}"`, "", true},
		{"reboot", `"{\"delay_in_seconds\":300,\"message\":\"bye\"}"`, "", false},
	}
	for _, tc := range cases {
		got, ok := reissueFlags(tc.verb, json.RawMessage(tc.data))
		if got != tc.want || ok != tc.ok {
			t.Errorf("reissueFlags(%s, %s) = %q, %v; want %q, %v", tc.verb, tc.data, got, ok, tc.want, tc.ok)
		}
	}
}

func TestCountByAccountRejectsMalformedAnswers(t *testing.T) {
	testenv.Isolate(t)
	body := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/oneview/oauth2/token" {
			_, _ = io.WriteString(w, `{"access_token":"tok","expires_in":3600,"token_type":"Bearer"}`)
			return
		}
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()
	t.Setenv("THREATDOWN_BASE_URL", srv.URL+"/oneview")
	t.Setenv("THREATDOWN_TOKEN_URL", srv.URL+"/oneview/oauth2/token")
	t.Setenv("THREATDOWN_CLIENT_ID", "cid")
	t.Setenv("THREATDOWN_CLIENT_SECRET", "secret")
	flags := &rootFlags{timeout: 10 * time.Second}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{`{}`, `null`, `{"results":null,"next_cursor":""}`, `{"results":[]}`} {
		body = bad
		if _, err := countByAccount(context.Background(), c, []string{"a1"}, map[string]any{}); err == nil {
			t.Fatalf("%s must be a fetch failure, not zero counts", bad)
		}
	}
	body = `{"results":[{"value":"a1","total":4}],"next_cursor":""}`
	got, err := countByAccount(context.Background(), c, []string{"a1"}, map[string]any{})
	if err != nil || got["a1"] != 4 {
		t.Fatalf("well-formed answer = %v, %v", got, err)
	}
}

func TestFleetHealthStopsOnCountAuthError(t *testing.T) {
	testenv.Isolate(t)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/oneview/oauth2/token":
			_, _ = io.WriteString(w, `{"access_token":"tok","expires_in":3600,"token_type":"Bearer"}`)
		case "/oneview/v1/sites":
			_, _ = io.WriteString(w, `{"sites":[{"id":"s1","account_id":"a1","company_name":"Acme"}]}`)
		default:
			calls++
			if calls > 1 {
				w.WriteHeader(http.StatusForbidden)
				_, _ = io.WriteString(w, `{"statusCode":403}`)
				return
			}
			_, _ = io.WriteString(w, `{"results":[{"value":"a1","total":3}],"next_cursor":""}`)
		}
	}))
	defer srv.Close()
	t.Setenv("THREATDOWN_BASE_URL", srv.URL+"/oneview")
	t.Setenv("THREATDOWN_TOKEN_URL", srv.URL+"/oneview/oauth2/token")
	t.Setenv("THREATDOWN_CLIENT_ID", "cid")
	t.Setenv("THREATDOWN_CLIENT_SECRET", "secret")
	_, err := fleetHealthLive(context.Background(), &rootFlags{timeout: 10 * time.Second}, "", 24, 7, time.Now())
	if err == nil || !stopFallback(err) {
		t.Fatalf("a 403 after a successful count must end the report as an auth error, got %v", err)
	}
}
