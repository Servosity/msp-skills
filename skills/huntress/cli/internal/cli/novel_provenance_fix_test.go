// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): regression tests for the two wrong-result
// classes Codex round 1 found on the 4.34.0 reprint's fix B (fix C):
//
//  1. A sync snapshot was treated as authoritative for absence without proof
//     that it enumerated the row's scope: a filtered walk, a reseller
//     partition walk the single sync_state row cannot describe, or a store
//     whose sync predates the record of what each walk enumerated.
//  2. A literal-text SQL prefilter narrowed rows before the semantic
//     indicator matcher, so an equivalent spelling the matcher accepts never
//     reached it.
//
// Each test drives the concrete input that produced a wrong answer. None of
// them names a symbol the fix introduced, so each one compiles and fails
// against the code before the fix.

package cli

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"huntress-pp-cli/internal/cliutil/testenv"
	"huntress-pp-cli/internal/store"
)

// shiftStoreTimes moves one time column of a table back by d, row by row.
// A table or column the store does not have is skipped (the walk record
// exists only once the fix is in).
func shiftStoreTimes(t *testing.T, db *sql.DB, table, column, layout string, d time.Duration) {
	t.Helper()
	rows, err := db.Query(`SELECT rowid, CAST(` + column + ` AS TEXT) FROM ` + table)
	if err != nil {
		if strings.Contains(err.Error(), "no such") {
			return
		}
		t.Fatalf("reading %s.%s: %v", table, column, err)
	}
	type stamp struct {
		rowid int64
		at    time.Time
	}
	var stamps []stamp
	for rows.Next() {
		var id int64
		var v sql.NullString
		if err := rows.Scan(&id, &v); err != nil {
			t.Fatal(err)
		}
		if at, ok := parseStoreTime(v.String); ok && v.Valid && at.Year() > 1 {
			stamps = append(stamps, stamp{id, at})
		}
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	for _, s := range stamps {
		if _, err := db.Exec(`UPDATE `+table+` SET `+column+` = ? WHERE rowid = ?`, s.at.Add(-d).Format(layout), s.rowid); err != nil {
			t.Fatalf("shifting %s.%s: %v", table, column, err)
		}
	}
}

// ageStore makes everything a store records about its past syncs d older,
// as if the next sync ran d later: the row sync times, the sync_state
// watermarks and the walk records.
func ageStore(t *testing.T, path string, d time.Duration) {
	t.Helper()
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	shiftStoreTimes(t, st.DB(), "resources", "synced_at", time.RFC3339, d)
	shiftStoreTimes(t, st.DB(), "sync_state", "last_synced_at", time.RFC3339, d)
	for _, col := range []string{"last_started_at", "last_ended_at", "full_started_at", "full_ended_at", "attempt_started_at"} {
		shiftStoreTimes(t, st.DB(), "walk_coverage", col, time.RFC3339Nano, d)
	}
}

// forgetWalkRecords turns a store into one written by a build that did not
// record what each walk enumerated (before the fix there is nothing to
// forget).
func forgetWalkRecords(t *testing.T, path string) {
	t.Helper()
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.DB().Exec(`DELETE FROM walk_coverage`); err != nil && !strings.Contains(err.Error(), "no such table") {
		t.Fatal(err)
	}
}

func runSync(t *testing.T, args ...string) {
	t.Helper()
	out, errOut, err := runCLI(t, append([]string{"sync"}, args...)...)
	if err != nil {
		t.Fatalf("sync %v: %v\nstdout: %s\nstderr: %s", args, err, out, errOut)
	}
}

func useServer(t *testing.T, h http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	t.Setenv("HUNTRESS_BASE_URL", srv.URL)
	t.Setenv("HUNTRESS_API_KEY", "hk_test_key_0001")
	t.Setenv("HUNTRESS_API_SECRET", "test-secret-value-0001")
}

// incidentServer answers /v1/incident_reports with a critical incident (1)
// and a low one (2) in organization 7, honoring a severity filter the way
// the API does. Every remediations list is empty.
func incidentServer(t *testing.T) {
	t.Helper()
	all := []map[string]any{
		{"id": 1, "organization_id": 7, "status": "sent", "severity": "critical", "sent_at": hoursAgo(3)},
		{"id": 2, "organization_id": 7, "status": "sent", "severity": "low", "sent_at": hoursAgo(2)},
	}
	useServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/v1/incident_reports":
			var out []map[string]any
			for _, inc := range all {
				if sev := r.URL.Query().Get("severity"); sev == "" || sev == inc["severity"] {
					out = append(out, inc)
				}
			}
			b, _ := json.Marshal(map[string]any{"incident_reports": out, "pagination": map[string]any{}})
			_, _ = w.Write(b)
		case strings.HasSuffix(r.URL.Path, "/remediations"):
			_, _ = w.Write([]byte(`{"remediations":[],"pagination":{}}`))
		default:
			http.NotFound(w, r)
		}
	})
}

// syncThenFilteredResync syncs both incidents, lets ago pass, then runs the
// review's filtered sync, which returns only the low incident.
func syncThenFilteredResync(t *testing.T, ago time.Duration) string {
	t.Helper()
	testenv.Isolate(t)
	incidentServer(t)
	db := filepath.Join(t.TempDir(), "data.db")
	runSync(t, "--db", db, "--resources", "incident-reports")
	ageStore(t, db, ago)
	runSync(t, "--db", db, "--resources", "incident-reports", "--resource-param", "incident-reports:severity=low")
	return db
}

// ---- class 1: absence needs a walk that provably enumerated the scope ----------

// Member A, the review's input: a sync filtered to severity=low completed and
// refreshed incident 2. It never asked for critical incidents, so it cannot
// show incident 1 was removed: the critical stays listed and counted, and a
// warning says the later sync was filtered.
func TestFilteredSyncDoesNotRemoveRowsItsFilterExcluded(t *testing.T) {
	db := syncThenFilteredResync(t, 10*time.Minute)
	rows := runNovelJSON(t, "fleet-incidents", "--severity", "critical", "--db", db)
	if len(rows) != 1 || fmt.Sprint(rows[0]["id"]) != "1" {
		t.Fatalf("fleet-incidents --severity critical after a severity=low sync = %v, want critical incident 1 (the filtered sync never asked for it)", rows)
	}
	sum, errOut := runNovelObject(t, "fleet-summary", "--db", db)
	if fmt.Sprint(sum["open_criticals"]) != "1" {
		t.Fatalf("fleet-summary open_criticals after a severity=low sync = %v, want 1", sum["open_criticals"])
	}
	if !strings.Contains(errOut, "which was filtered") || !strings.Contains(errOut, "incident-reports") {
		t.Fatalf("a row a filtered sync did not return was counted with no warning naming the filtered sync; stderr %q", errOut)
	}
}

// The review's second trigger: an ordinary --param filter on a flat walk is
// no more an enumeration than a --resource-param one.
func TestParamFilteredSyncDoesNotRemoveRows(t *testing.T) {
	testenv.Isolate(t)
	incidentServer(t)
	db := filepath.Join(t.TempDir(), "data.db")
	runSync(t, "--db", db, "--resources", "incident-reports")
	ageStore(t, db, 10*time.Minute)
	runSync(t, "--db", db, "--resources", "incident-reports", "--param", "severity=low")
	sum, errOut := runNovelObject(t, "fleet-summary", "--db", db)
	if fmt.Sprint(sum["open_criticals"]) != "1" || !strings.Contains(errOut, "which was filtered") {
		t.Fatalf("fleet-summary after a --param severity=low sync = open_criticals %v, stderr %q; want 1 with a filtered-sync warning", sum["open_criticals"], errOut)
	}
}

// pagedAgentsServer serves /v1/agents: 15 agents of organization 70 in pages
// of 10 (the sync page size). Page 2 answers HTTP 400 until failPage2 is
// cleared, so the first sync stops after page 1 with its cursor saved.
func pagedAgentsServer(t *testing.T, failPage2 *atomic.Bool) {
	t.Helper()
	useServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/v1/agents" {
			http.NotFound(w, r)
			return
		}
		from, to, next := 1, 10, "p2"
		if r.URL.Query().Get("page_token") == "p2" {
			if failPage2.Load() {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"temporary failure"}`))
				return
			}
			from, to, next = 11, 15, ""
		}
		var agents []map[string]any
		for id := from; id <= to; id++ {
			agents = append(agents, map[string]any{"id": id, "organization_id": 70, "last_callback_at": hoursAgo(1)})
		}
		pagination := map[string]any{}
		if next != "" {
			pagination["next_page_token"] = next
		}
		b, _ := json.Marshal(map[string]any{"agents": agents, "pagination": pagination})
		_, _ = w.Write(b)
	})
}

// The coordinator's input (adversarial review): page 2 of /v1/agents failed
// on the first sync; the second sync resumed from the saved page cursor,
// stored agents 11 to 15 and completed. Agents 1 to 10 were read by the same
// logical walk, so they are neither left out nor flagged; and a walk that
// did not start from the first page is never proof that a row is gone.
func TestResumedSyncIsNotProofOfAbsence(t *testing.T) {
	testenv.Isolate(t)
	var failPage2 atomic.Bool
	failPage2.Store(true)
	pagedAgentsServer(t, &failPage2)
	db := filepath.Join(t.TempDir(), "data.db")
	if out, errOut, err := runCLI(t, "sync", "--db", db, "--resources", "agents"); err == nil {
		t.Fatalf("the first sync succeeded although page 2 failed: %s %s", out, errOut)
	}
	ageStore(t, db, 10*time.Minute)
	failPage2.Store(false)
	runSync(t, "--db", db, "--resources", "agents")
	sum, errOut := runNovelObject(t, "fleet-summary", "--db", db)
	if fmt.Sprint(sum["agents"]) != "15" {
		t.Fatalf("fleet-summary agents after a resumed sync = %v, want 15 (the resumed walk did not reread page 1); stderr %q", sum["agents"], errOut)
	}
	if strings.Contains(errOut, "left out") || strings.Contains(errOut, "were not returned") {
		t.Fatalf("a resumed sync's own first page was reported as removed or unverified; stderr %q", errOut)
	}
}

// The adversarial review's P2 (a): after a sync of only the flat agents list
// on a reseller store, the sub-account agents (accounts_agents) are not in
// any organization the flat list holds, so that walk could not have
// returned them and must not flag them.
func TestFlatOnlyResyncDoesNotFlagSubAccountAgents(t *testing.T) {
	testenv.Isolate(t)
	db := seedTimed(t, []timedRow{
		{"accounts", `{"id":7,"name":"Child"}`, 10 * time.Minute},
		{"accounts_agents", `{"id":1,"organization_id":70,"account_id":7,"parent_id":"7"}`, 10 * time.Minute},
		{"agents", `{"id":5,"organization_id":10,"account_id":1}`, 30 * time.Second},
	}, map[string]syncMark{
		"accounts":        {ago: 10 * time.Minute, complete: true},
		"accounts_agents": {ago: 10 * time.Minute, complete: true},
		"agents":          {ago: time.Minute, complete: true},
	})
	sum, errOut := runNovelObject(t, "fleet-summary", "--db", db)
	if fmt.Sprint(sum["agents"]) != "2" || strings.Contains(errOut, "not returned by the later complete sync of agents") {
		t.Fatalf("fleet-summary agents = %v with stderr %q; want 2 and no warning (the flat list holds no row of organization 70)", sum["agents"], errOut)
	}
}

// The adversarial review's P2 (c): an organization the store holds nothing
// about has unknown counts, not zeros.
func TestOrgScorecardWithholdsCountsForAnOrgNotInTheStore(t *testing.T) {
	testenv.Isolate(t)
	db := seedStore(t, map[string][]string{
		"organizations": {`{"id":7,"name":"Acme"}`},
		"agents":        {`{"id":1,"organization_id":7}`},
	})
	card, errOut := runNovelObject(t, "org-scorecard", "--org", "999", "--db", db)
	if card["agent_count"] != nil || card["total_incidents"] != nil || card["organization_found"] != false || !strings.Contains(errOut, "not in the local store") {
		t.Fatalf("org-scorecard for an unknown organization = %v (stderr %q), want counts withheld (null), organization_found false and a warning", card, errOut)
	}
	card, _ = runNovelObject(t, "org-scorecard", "--org", "7", "--db", db)
	if fmt.Sprint(card["agent_count"]) != "1" || card["organization_found"] != true {
		t.Fatalf("org-scorecard for organization 7 = %v, want agent_count 1", card)
	}
}

// The same class on freshness: a filtered sync advances the sync time, so
// the --max-age check read the store as fresh although the rows outside the
// filter were last refreshed by an unfiltered sync 40 minutes ago.
func TestFilteredSyncDoesNotMakeTheStoreLookFresh(t *testing.T) {
	db := syncThenFilteredResync(t, 40*time.Minute)
	_, errOut, err := runCLI(t, "fleet-incidents", "--db", db, "--json")
	if err != nil {
		t.Fatalf("fleet-incidents: %v (stderr %q)", err, errOut)
	}
	if !strings.Contains(errOut, "last unfiltered complete sync of incident-reports") {
		t.Fatalf("a filtered sync made a 40-minute-old store pass the 30-minute --max-age check silently; stderr %q", errOut)
	}
}

// A store written before walks were recorded cannot show that its later
// complete sync was unfiltered, so a row that sync did not return is kept
// and flagged, never left out.
func TestLegacyFlatSyncCannotProveARowWasRemoved(t *testing.T) {
	testenv.Isolate(t)
	db := seedTimed(t, []timedRow{
		{"organizations", `{"id":70,"name":"Acme"}`, 30 * time.Second},
		{"agents", `{"id":1,"organization_id":70}`, 10 * time.Minute},
		{"agents", `{"id":2,"organization_id":70}`, 30 * time.Second},
	}, map[string]syncMark{
		"organizations": {ago: time.Minute, complete: true},
		"agents":        {ago: time.Minute, complete: true},
	})
	forgetWalkRecords(t, db)
	sum, errOut := runNovelObject(t, "fleet-summary", "--db", db)
	if fmt.Sprint(sum["agents"]) != "2" {
		t.Fatalf("fleet-summary agents = %v, want 2 (a sync whose filters were not recorded cannot remove agent 1)", sum["agents"])
	}
	if !strings.Contains(errOut, "does not record what that sync enumerated") {
		t.Fatalf("an agent a later unrecorded sync did not return was counted with no warning; stderr %q", errOut)
	}
}

// resellerAgentsServer serves account 7's agents: agents 1 and 2 until
// dropOne is set, then agent 2 only. A platform filter is honored.
func resellerAgentsServer(t *testing.T, dropOne *atomic.Bool) {
	t.Helper()
	useServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/v1/accounts/7/agents" {
			http.NotFound(w, r)
			return
		}
		agents := []map[string]any{
			{"id": 1, "organization_id": 70, "account_id": 7, "platform": "darwin", "last_callback_at": hoursAgo(1)},
			{"id": 2, "organization_id": 70, "account_id": 7, "platform": "windows", "last_callback_at": hoursAgo(1)},
		}
		if dropOne.Load() {
			agents = agents[1:]
		}
		var out []map[string]any
		for _, a := range agents {
			if p := r.URL.Query().Get("platform"); p == "" || p == a["platform"] {
				out = append(out, a)
			}
		}
		b, _ := json.Marshal(map[string]any{"agents": out, "pagination": map[string]any{}})
		_, _ = w.Write(b)
	})
}

// seedAccount stores the synced account list (account 7) the reseller
// fan-out walks.
func seedAccount(t *testing.T) string {
	t.Helper()
	db := filepath.Join(t.TempDir(), "data.db")
	st, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, _, err := st.UpsertBatch("accounts", []json.RawMessage{json.RawMessage(`{"id":7,"name":"Acme","status":"active"}`)}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveSyncState("accounts", "", 1); err != nil {
		t.Fatal(err)
	}
	return db
}

// Member B, the review's input: agent 1 was deleted upstream and an
// ordinary (not --full) accounts_agents sync then completed. That walk
// enumerated account 7 in full after agent 1's copy was written and did not
// return it, so agent 1 is left out and the warning says so; before the fix
// it was counted silently.
func TestResellerResyncLeavesOutAnAgentItsAccountWalkNoLongerReturns(t *testing.T) {
	testenv.Isolate(t)
	var dropOne atomic.Bool
	resellerAgentsServer(t, &dropOne)
	db := seedAccount(t)
	runSync(t, "--db", db, "--resources", "accounts_agents")
	ageStore(t, db, 10*time.Minute)
	dropOne.Store(true)
	runSync(t, "--db", db, "--resources", "accounts_agents")
	sum, errOut := runNovelObject(t, "fleet-summary", "--db", db)
	if fmt.Sprint(sum["agents"]) != "1" {
		t.Fatalf("fleet-summary agents = %v, want 1 (a later complete walk of account 7 did not return agent 1); stderr %q", sum["agents"], errOut)
	}
	if !strings.Contains(errOut, "left out 1 row(s) that a later complete sync of accounts_agents did not return") {
		t.Fatalf("agent 1 was left out with no warning naming the accounts_agents walk; stderr %q", errOut)
	}
}

// A filtered partition walk cannot show the agents it did not return were
// removed: they stay counted, and the warning names the filtered walk.
func TestFilteredResellerResyncKeepsAndFlagsTheAgentsItSkipped(t *testing.T) {
	testenv.Isolate(t)
	var dropOne atomic.Bool
	resellerAgentsServer(t, &dropOne)
	db := seedAccount(t)
	runSync(t, "--db", db, "--resources", "accounts_agents")
	ageStore(t, db, 10*time.Minute)
	runSync(t, "--db", db, "--resources", "accounts_agents", "--resource-param", "accounts_agents:platform=windows")
	sum, errOut := runNovelObject(t, "fleet-summary", "--db", db)
	if fmt.Sprint(sum["agents"]) != "2" {
		t.Fatalf("fleet-summary agents = %v, want 2 (a platform=windows walk cannot remove the darwin agent)", sum["agents"])
	}
	if !strings.Contains(errOut, "not returned by the later sync of accounts_agents, which was filtered") {
		t.Fatalf("an agent a filtered partition walk did not return was counted with no warning; stderr %q", errOut)
	}
}

// A partition copy older than its alias's later complete sync, in a store
// that does not record which accounts that sync walked: the single
// sync_state row cannot say whether the walk returned it, so it is kept and
// flagged (before the fix, holding a copy skipped the check entirely).
func TestLegacyPartitionCopyOlderThanALaterSyncIsFlagged(t *testing.T) {
	testenv.Isolate(t)
	db := seedTimed(t, []timedRow{
		{"accounts", `{"id":7,"name":"Acme"}`, 30 * time.Second},
		{"accounts_agents", `{"id":1,"organization_id":70,"account_id":7,"parent_id":"7"}`, 10 * time.Minute},
		{"accounts_agents", `{"id":2,"organization_id":70,"account_id":7,"parent_id":"7"}`, 30 * time.Second},
	}, map[string]syncMark{
		"accounts":        {ago: time.Minute, complete: true},
		"accounts_agents": {ago: time.Minute, complete: true},
	})
	forgetWalkRecords(t, db)
	sum, errOut := runNovelObject(t, "fleet-summary", "--db", db)
	if fmt.Sprint(sum["agents"]) != "2" {
		t.Fatalf("fleet-summary agents = %v, want 2", sum["agents"])
	}
	if !strings.Contains(errOut, "1 row(s) are older than the later complete sync of accounts_agents") {
		t.Fatalf("a partition copy older than a later complete accounts_agents sync was counted with no warning; stderr %q", errOut)
	}
}

// ---- class 2: the SQL prefilter admits every spelling the matcher accepts --------

func TestBlastRadiusPrefilterAdmitsEverySpellingTheMatcherAccepts(t *testing.T) {
	testenv.Isolate(t)
	inc := func(id int, field, text string, types string) string {
		return fmt.Sprintf(`{"id":%d,"organization_id":7,"status":"sent","sent_at":"%s",%q:%q,"indicator_types":%s}`, id, hoursAgo(float64(id)), field, text, types)
	}
	db := seedStore(t, map[string][]string{
		"accounts_incident_reports": {
			inc(1, "body", "Beacon to [::ffff:cb00:7107]", `[]`),
			inc(2, "summary", "Beacon to 0:0:0:0:0:FFFF:CB00:7107 now", `[]`),
			inc(3, "summary", "Beacon to 203.0.113.70", `[]`),
			inc(4, "summary", "unrelated", `["ÄNOMALY"]`),
		},
		"accounts_agents": {
			`{"id":21,"organization_id":7,"hostname":"Kiosk-01"}`,
			`{"id":22,"organization_id":7,"hostname":"kiosk-011"}`,
		},
		"accounts_external_ports": {
			`{"id":31,"ip_address":"::ffff:203.0.113.7","port":443}`,
		},
	})
	cases := []struct {
		name, indicator          string
		incidents, agents, ports string
	}{
		{"an IPv4 indicator against IPv4-mapped IPv6 spellings", "203.0.113.7", "[1 2]", "[]", "[31]"},
		{"an IPv4-mapped indicator against the same spellings", "::ffff:cb00:7107", "[1 2]", "[]", "[31]"},
		{"a text indicator against a Unicode case variant", "kiosk-01", "[]", "[21 22]", "[]"},
		{"an indicator type in another case", "änomaly", "[4]", "[]", "[]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runNovelObject(t, "blast-radius", "--indicator", c.indicator, "--db", db)
			inc, _ := out["incident_reports"].([]any)
			ag, _ := out["agents"].([]any)
			po, _ := out["external_ports"].([]any)
			if fmt.Sprint(idsOf(inc)) != c.incidents || fmt.Sprint(idsOf(ag)) != c.agents || fmt.Sprint(idsOf(po)) != c.ports {
				t.Fatalf("blast-radius %q = incidents %v agents %v ports %v, want %s %s %s", c.indicator, idsOf(inc), idsOf(ag), idsOf(po), c.incidents, c.agents, c.ports)
			}
		})
	}
}

// The adversarial review's P2 (b): a part of an address is never the
// address. "113.22" inside 203.0.113.22 is a fragment in incident text and
// in an agent's external_ip alike, and both are counted as fragments.
func TestBlastRadiusLabelsAPartialAddressAFragmentOnEveryRowType(t *testing.T) {
	testenv.Isolate(t)
	db := seedStore(t, map[string][]string{
		"accounts_incident_reports": {fmt.Sprintf(`{"id":1,"organization_id":7,"status":"sent","sent_at":"%s","summary":"Beacon to 203.0.113.22"}`, hoursAgo(1))},
		"accounts_agents":           {`{"id":21,"organization_id":7,"hostname":"ws1","external_ip":"203.0.113.22"}`},
	})
	out, errOut := runNovelObject(t, "blast-radius", "--indicator", "113.22", "--db", db)
	inc, _ := out["incident_reports"].([]any)
	ag, _ := out["agents"].([]any)
	if len(inc) != 1 || len(ag) != 1 {
		t.Fatalf("blast-radius 113.22 = incidents %v agents %v, want one of each (the same partial address on both row types)", inc, ag)
	}
	for _, r := range append(inc, ag...) {
		if m := r.(map[string]any); m["match"] != matchFragment {
			t.Fatalf("blast-radius 113.22 labelled row %v match %v, want fragment", m["id"], m["match"])
		}
	}
	if fmt.Sprint(out["fragment_matches"]) != "2" || !strings.Contains(errOut, "inside a longer") {
		t.Fatalf("fragment_matches = %v, stderr %q; want 2 with a warning", out["fragment_matches"], errOut)
	}
}

// Freshness under a resume: the resumed sync advances the sync time, but the
// first page was read when the interrupted walk started, 40 minutes ago.
func TestResumedSyncDoesNotMakeTheStoreLookFresh(t *testing.T) {
	testenv.Isolate(t)
	var failPage2 atomic.Bool
	failPage2.Store(true)
	pagedAgentsServer(t, &failPage2)
	db := filepath.Join(t.TempDir(), "data.db")
	_, _, _ = runCLI(t, "sync", "--db", db, "--resources", "agents")
	ageStore(t, db, 40*time.Minute)
	failPage2.Store(false)
	runSync(t, "--db", db, "--resources", "agents")
	_, errOut := runNovelObject(t, "fleet-summary", "--db", db)
	if !strings.Contains(errOut, "resumed a walk an interrupted sync started") {
		t.Fatalf("a resumed sync made 40-minute-old rows pass the 30-minute --max-age check silently; stderr %q", errOut)
	}
}

// Guard: a reseller store synced twice with nothing changed upstream (flat
// agents of the reseller's own organization, account 7's agents through the
// fan-out) raises no reconciliation warning and counts every agent once.
func TestSteadyStateResyncRaisesNoReconciliationWarning(t *testing.T) {
	testenv.Isolate(t)
	useServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/v1/agents":
			_, _ = w.Write([]byte(`{"agents":[{"id":5,"organization_id":10,"account_id":1,"last_callback_at":"` + hoursAgo(1) + `"}],"pagination":{}}`))
		case r.URL.Path == "/v1/accounts":
			_, _ = w.Write([]byte(`{"accounts":[{"id":7,"name":"Child","status":"active"}],"pagination":{}}`))
		case r.URL.Path == "/v1/accounts/7/agents":
			_, _ = w.Write([]byte(`{"agents":[{"id":1,"organization_id":70,"account_id":7,"last_callback_at":"` + hoursAgo(1) + `"},{"id":2,"organization_id":70,"account_id":7,"last_callback_at":"` + hoursAgo(1) + `"}],"pagination":{}}`))
		case strings.HasPrefix(r.URL.Path, "/v1/accounts/7/"):
			key := strings.TrimPrefix(r.URL.Path, "/v1/accounts/7/")
			b, _ := json.Marshal(map[string]any{key: []any{}, "pagination": map[string]any{}})
			_, _ = w.Write(b)
		default:
			http.NotFound(w, r)
		}
	})
	db := filepath.Join(t.TempDir(), "data.db")
	runSync(t, "--db", db, "--resources", "agents,accounts")
	ageStore(t, db, 10*time.Minute)
	runSync(t, "--db", db, "--resources", "agents,accounts")
	sum, errOut := runNovelObject(t, "fleet-summary", "--db", db)
	if fmt.Sprint(sum["agents"]) != "3" || strings.Contains(errOut, "row(s)") {
		t.Fatalf("fleet-summary after an unchanged resync = agents %v, stderr %q; want 3 and no reconciliation warning", sum["agents"], errOut)
	}
}
