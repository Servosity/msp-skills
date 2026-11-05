// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Codex review round 1 of the crowdstrike 4.34.2 reprint (static review of the
// hand-written layer), the fleet classes:
//   - token-for-another-tenant: a credential header the configuration names
//     ([headers] Authorization in a trusted config.toml) was set on every
//     request after the client's own bearer, so it overrode the member_cid
//     token of each child CID and fleet sync stored the parent tenant's hosts
//     under the child CID. The token binding owns Authorization: no configured
//     header carries a credential, and a child config carries none.
//   - fleet-refresh-completeness: what fleet sync keeps must be what it read.
//     A complete read of one kind for one CID replaces that kind's rows (a
//     remediated vulnerability that left Spotlight's status:'open' list stayed
//     open in the store forever), and a posture snapshot is recorded only for
//     a CID whose hosts, alerts and vulnerabilities were all read completely
//     in the same sync (a sync of policies alone recorded zero critical
//     vulnerabilities, and fleet trend called the tenant "better").

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"crowdstrike-pp-cli/internal/config"
)

// A trusted config file (0600) whose [headers] carries a parent tenant's
// bearer: fleet sync for one child CID reads that child with its member_cid
// token alone and stores only the child's hosts.
func TestR1ConfiguredAuthorizationNeverReachesAChildRead(t *testing.T) {
	f := newFlightControl(t, false)
	home := fleetEnv(t, f)
	const static = "pinParentStaticToken-3a1f9c"
	f.mu.Lock()
	f.minted[static] = "parent" // the parent tenant honors it
	f.mu.Unlock()
	cfgPath := filepath.Join(home, "parent.toml")
	if err := os.WriteFile(cfgPath, []byte("[headers]\nAuthorization = \"Bearer "+static+"\"\nX-Trace-Label = \"fleet-review\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(t.TempDir(), "fleet.db")
	out, errb, err := runFleet(t, "fleet", "sync", "--config", cfgPath, "--member-cid", pinChild, "--kinds", "hosts", "--db", db, "--json")
	if err != nil {
		t.Fatalf("fleet sync: %v\n%s%s", err, out, errb)
	}
	_, reqs, minted := f.state()
	var hostReads int
	for _, r := range reqs {
		fields := strings.Fields(r)
		tok := fields[len(fields)-1]
		hostReads++
		if minted[tok] != pinChild {
			t.Errorf("%s %s carried the token of tenant %q (%q), want only the member_cid token of %s", fields[0], fields[1], minted[tok], tok, pinChild)
		}
	}
	if hostReads == 0 {
		t.Fatalf("fleet sync read no hosts; the test did not run:\n%s%s", out, errb)
	}
	ents, err := loadFleetEntities(t.Context(), db, kindHost)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if e.CID != pinChild || e.ID != "dev-of-Child" {
			t.Errorf("stored host %+v: want only the child tenant's host under %s", e, pinChild)
		}
	}
}

// The child config itself carries no credential header, whatever the parent
// config holds, and keeps every other header.
func TestR1ChildConfigCarriesNoCredentialHeader(t *testing.T) {
	parent := &config.Config{Headers: map[string]string{
		"Authorization": "Bearer parent-a", "proxy-authorization": "Basic cGFyZW50", "X-Api-Key": "parent-key",
		"Cookie": "sid=parent", "X-Trace-Label": "kept",
	}}
	child := scopedChildConfig(parent, "child-token")
	for k := range child.Headers {
		if k != "X-Trace-Label" {
			t.Errorf("child config keeps credential header %q", k)
		}
	}
	if child.Headers["X-Trace-Label"] != "kept" {
		t.Errorf("child config lost a non-credential header: %v", child.Headers)
	}
	if len(parent.Headers) != 5 {
		t.Errorf("the parent config's headers changed: %v", parent.Headers)
	}
	if got := child.AuthHeader(); got != "Bearer child-token" {
		t.Errorf("child Authorization = %q, want the member_cid token", got)
	}
}

// refreshFalcon is one Falcon child tenant whose lists the test changes
// between syncs: body[path] answers that path, fail[path] answers HTTP 403
// (a read that fails without the client's retries).
type refreshFalcon struct {
	srv  *httptest.Server
	mu   sync.Mutex
	body map[string]string
	fail map[string]bool
}

func newRefreshFalcon(t *testing.T) *refreshFalcon {
	t.Helper()
	f := &refreshFalcon{body: map[string]string{}, fail: map[string]bool{}}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/oauth2/token" {
			_, _ = io.ReadAll(r.Body)
			_, _ = io.WriteString(w, `{"access_token":"refreshChildToken-7c2e","token_type":"bearer","expires_in":1799}`)
			return
		}
		f.mu.Lock()
		body, ok := f.body[r.URL.Path]
		fail := f.fail[r.URL.Path]
		f.mu.Unlock()
		if fail {
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"errors":[{"code":403,"message":"access denied, authorization failed"}]}`)
			return
		}
		if !ok {
			body = falconRecords()
		}
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *refreshFalcon) set(path, body string, fail bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.body[path], f.fail[path] = body, fail
}

// falconRecords is a complete Falcon list of the given records.
func falconRecords(records ...string) string {
	return fmt.Sprintf(`{"meta":{"pagination":{"offset":0,"limit":500,"total":%d}},"resources":[%s],"errors":[]}`, len(records), strings.Join(records, ","))
}

const (
	refreshChild    = "c0ffee00child00cid0000000000refr"
	refreshVulnPath = "/spotlight/combined/vulnerabilities/v1"
	refreshCritVuln = `{"id":"vul-remediated-1","status":"open","cve":{"id":"CVE-2026-1111","severity":"CRITICAL"}}`
)

func refreshEnv(t *testing.T, f *refreshFalcon) string {
	t.Helper()
	ff := &flightControl{srv: f.srv}
	home := fleetEnv(t, ff)
	return filepath.Join(home, "refresh.db")
}

func refreshSync(t *testing.T, db, kinds string) string {
	t.Helper()
	out, errb, err := runFleet(t, "fleet", "sync", "--member-cid", refreshChild, "--kinds", kinds, "--db", db, "--json")
	if err != nil {
		t.Fatalf("fleet sync --kinds %s: %v\n%s%s", kinds, err, out, errb)
	}
	return out
}

// A complete read replaces what the store held for that CID and kind: the
// vulnerability Spotlight no longer lists as open is gone from fleet vulns and
// from the scorecard. A read that failed keeps the previous rows (and warns).
func TestR1CompleteRefreshReplacesTheKindItRead(t *testing.T) {
	f := newRefreshFalcon(t)
	db := refreshEnv(t, f)
	f.set(refreshVulnPath, falconRecords(refreshCritVuln), false)
	refreshSync(t, db, "vulns")
	if out, _, err := runFleet(t, "fleet", "vulns", "--db", db, "--json"); err != nil || !strings.Contains(out, "vul-remediated-1") {
		t.Fatalf("control: the first sync must store the open vulnerability (err %v):\n%s", err, out)
	}
	// The vulnerability list fails: nothing was read, the row stays.
	f.set(refreshVulnPath, "", true)
	if out := refreshSync(t, db, "vulns"); !strings.Contains(out, "sync failed") {
		t.Fatalf("control: a failed read must warn:\n%s", out)
	}
	if out, _, _ := runFleet(t, "fleet", "vulns", "--db", db, "--json"); !strings.Contains(out, "vul-remediated-1") {
		t.Fatalf("control: a failed read must keep the stored rows:\n%s", out)
	}
	// Remediated: the open list is complete and empty.
	f.set(refreshVulnPath, falconRecords(), false)
	refreshSync(t, db, "vulns")
	if out, _, _ := runFleet(t, "fleet", "vulns", "--db", db, "--json"); strings.Contains(out, "vul-remediated-1") {
		t.Errorf("fleet vulns still lists the remediated vulnerability after a complete refresh:\n%s", out)
	}
	out, _, err := runFleet(t, "fleet", "scorecard", "--db", db, "--json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, `"critical_vulns": 1`) || strings.Contains(out, `"critical_vulns":1`) {
		t.Errorf("fleet scorecard still counts the remediated critical vulnerability:\n%s", out)
	}
}

type trendRow struct {
	CID                string `json:"cid"`
	CriticalVulns      int    `json:"critical_vulns"`
	CriticalVulnsDelta int    `json:"critical_vulns_delta"`
	Direction          string `json:"direction"`
}

func refreshTrend(t *testing.T, db string) []trendRow {
	t.Helper()
	out, errb, err := runFleet(t, "fleet", "trend", "--db", db, "--json")
	if err != nil {
		t.Fatalf("fleet trend: %v\n%s%s", err, out, errb)
	}
	var view struct {
		Tenants []trendRow `json:"tenants"`
	}
	if jerr := json.Unmarshal([]byte(out), &view); jerr != nil {
		var env struct {
			Results struct {
				Tenants []trendRow `json:"tenants"`
			} `json:"results"`
		}
		if json.Unmarshal([]byte(out), &env) != nil {
			t.Fatalf("fleet trend output does not decode: %v\n%s", jerr, out)
		}
		view.Tenants = env.Results.Tenants
	}
	return view.Tenants
}

// A sync that did not read hosts, alerts and vulnerabilities completely
// records no posture: a vulnerabilities-only sync followed by a policies-only
// sync never reads as "critical vulnerabilities went from 1 to 0", and a sync
// whose alert read failed adds no generation either. A complete sync records
// one (control).
func TestR1PartialRefreshRecordsNoPosture(t *testing.T) {
	f := newRefreshFalcon(t)
	db := refreshEnv(t, f)
	f.set(refreshVulnPath, falconRecords(refreshCritVuln), false)
	refreshSync(t, db, "vulns")
	f.set("/policy/combined/prevention/v1", falconRecords(`{"id":"pol-1","name":"Workstations","platform_name":"Windows","enabled":true}`), false)
	refreshSync(t, db, "policies")
	for _, row := range refreshTrend(t, db) {
		if row.Direction == "better" || row.CriticalVulnsDelta < 0 || (row.CID == refreshChild && row.CriticalVulns == 0) {
			t.Errorf("partial syncs were recorded as posture: %+v", row)
		}
	}
	// Control: a complete posture read records a baseline with the vulnerability.
	refreshSync(t, db, "hosts,alerts,vulns")
	rows := refreshTrend(t, db)
	if len(rows) != 1 || rows[0].CID != refreshChild || rows[0].CriticalVulns != 1 || rows[0].Direction != "baseline" {
		t.Fatalf("control: one complete sync must record one baseline with 1 critical vulnerability, got %+v", rows)
	}
	// The alert read fails: no second generation, so no delta is invented.
	f.set("/alerts/combined/alerts/v1", "", true)
	f.set(refreshVulnPath, falconRecords(), false)
	refreshSync(t, db, "hosts,alerts,vulns")
	rows = refreshTrend(t, db)
	if len(rows) != 1 || rows[0].Direction != "baseline" {
		t.Errorf("a sync whose alert read failed was recorded as posture: %+v", rows)
	}
}

// Any failure signal wins: a page answered 200 with a populated Falcon errors
// list is not a complete read, so it replaces nothing and records no posture.
func TestR1PageWithFalconErrorsReplacesNothing(t *testing.T) {
	f := newRefreshFalcon(t)
	db := refreshEnv(t, f)
	f.set(refreshVulnPath, falconRecords(refreshCritVuln), false)
	refreshSync(t, db, "hosts,alerts,vulns")
	f.set(refreshVulnPath, `{"meta":{"pagination":{"offset":0,"limit":500,"total":0}},"resources":[],"errors":[{"code":500,"message":"spotlight backend unavailable"}]}`, false)
	out := refreshSync(t, db, "hosts,alerts,vulns")
	if !strings.Contains(out, "spotlight backend unavailable") {
		t.Errorf("the Falcon error must be reported:\n%s", out)
	}
	if got, _, _ := runFleet(t, "fleet", "vulns", "--db", db, "--json"); !strings.Contains(got, "vul-remediated-1") {
		t.Errorf("a page carrying Falcon errors replaced the stored vulnerabilities:\n%s", got)
	}
	if rows := refreshTrend(t, db); len(rows) != 1 || rows[0].Direction != "baseline" {
		t.Errorf("a read carrying Falcon errors was recorded as posture: %+v", rows)
	}
}

// A fabric refresh upserts (Codex r2 class fleet-replace-only-a-proven-scope
// superseded round 1's fabric replacement): the fabric rows sit under "self",
// the label of whichever parent authenticated, so replacing them would delete
// another parent's roster in a shared --db. A listed child is refreshed; a
// child Flight Control no longer lists keeps its row (the 0.1.10 behavior;
// .review/followups.md).
func TestR1FabricRefreshUpsertsTheChildList(t *testing.T) {
	f := newRefreshFalcon(t)
	db := refreshEnv(t, f)
	f.set("/mssp/queries/children/v1", `{"meta":{"pagination":{"offset":0,"limit":500,"total":2}},"resources":["cidkeep0000000000000000000000001","cidgone0000000000000000000000002"],"errors":[]}`, false)
	f.set("/mssp/entities/children/GET/v2", falconRecords(`{"child_cid":"cidkeep0000000000000000000000001","name":"Kept Tenant"}`), false)
	if out, errb, err := runFleet(t, "fleet", "sync", "--kinds", "fabric", "--db", db, "--json"); err != nil {
		t.Fatalf("fleet sync fabric: %v\n%s%s", err, out, errb)
	}
	out, _, err := runFleet(t, "fleet", "tenants", "--db", db, "--json")
	if err != nil || !strings.Contains(out, "cidkeep0000000000000000000000001") || !strings.Contains(out, "cidgone0000000000000000000000002") {
		t.Fatalf("control: both listed children must be tenants (the second as a bare row), err %v:\n%s", err, out)
	}
	f.set("/mssp/queries/children/v1", `{"meta":{"pagination":{"offset":0,"limit":500,"total":1}},"resources":["cidkeep0000000000000000000000001"],"errors":[]}`, false)
	f.set("/mssp/entities/children/GET/v2", falconRecords(`{"child_cid":"cidkeep0000000000000000000000001","name":"Kept Tenant Renamed"}`), false)
	if out, errb, err := runFleet(t, "fleet", "sync", "--kinds", "fabric", "--db", db, "--json"); err != nil {
		t.Fatalf("fleet sync fabric: %v\n%s%s", err, out, errb)
	}
	out, _, _ = runFleet(t, "fleet", "tenants", "--db", db, "--json")
	if !strings.Contains(out, "Kept Tenant Renamed") {
		t.Errorf("fleet tenants did not refresh the listed child:\n%s", out)
	}
	if !strings.Contains(out, "cidgone0000000000000000000000002") {
		t.Errorf("a fabric refresh deleted a row it cannot prove belongs to this parent:\n%s", out)
	}
}

// A walk the page cap ends (a list with no documented total that still had
// full pages) is not complete either: it replaces nothing.
func TestR1WalkEndedByThePageCapIsIncomplete(t *testing.T) {
	page := func(i int) json.RawMessage {
		ids := make([]string, fleetSyncPageLimit)
		for j := range ids {
			ids[j] = fmt.Sprintf("%q", fmt.Sprintf("id-%d-%d", i, j))
		}
		return json.RawMessage(`{"resources":[` + strings.Join(ids, ",") + `]}`)
	}
	n := 0
	got, err := fleetWalk(func(offset int, after string) (json.RawMessage, error) {
		n++
		return page(n), nil
	}, extractStringResources, func(s string) string { return s })
	if len(got) != fleetSyncMaxPages*fleetSyncPageLimit {
		t.Fatalf("control: the walk must read every page up to the cap, got %d records", len(got))
	}
	var short *fleetShortWalk
	if !errors.As(err, &short) {
		t.Fatalf("a walk ended by the page cap read as complete (err %v)", err)
	}
}
