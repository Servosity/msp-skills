// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// The local-mirror readers (agents stale, agents drift, lists where) must not
// print an all-clear or a clean empty result when a collection their answer
// joins was never synced or its last sync attempt did not finish.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"dnsfilter-pp-cli/internal/cliutil/testenv"
	"dnsfilter-pp-cli/internal/store"
)

const (
	mirrorOrg        = `{"id":"1","attributes":{"id":1,"name":"Acme Dental","user_agents_auto_update":false}}`
	mirrorSite       = `{"id":"50","attributes":{"name":"HQ"},"relationships":{"organization":{"data":{"id":"1"}},"policy":{"data":{"id":"10"}}}}`
	mirrorPolicy     = `{"id":"10","attributes":{"name":"Std","is_global_policy":false,"whitelist_domains":["example.com"],"blacklist_domains":[]},"relationships":{"organization":{"data":{"id":"1"}}}}`
	mirrorFeedLatest = `{"user_agent_releases":[{"agent_type":"windows","architecture":"x86_64","agent_version":"3.2.0","white_label":false,"release_channels":[{"value":"stable"}]}]}`
)

// mirrorAgent is an installed roaming client on site 50, checked in now and
// protected, on agent version 3.2.0: healthy and current.
func mirrorAgent() string {
	return `{"id":"a1","attributes":{"hostname":"PC-1","agent_type":"windows","agent_version":"3.2.0","status":"active","agent_state":"protected","last_sync":"` + time.Now().UTC().Format(time.RFC3339) + `"},"relationships":{"network":{"data":{"id":"50"}}}}`
}

// seedMirror writes rows straight into a store at dbPath and records each
// collection's sync state: "complete" (a full walk), "partial" (an interrupted
// first walk), "cleared" (a full walk, then a --full cursor clear), or no
// entry at all (never synced).
func seedMirror(t *testing.T, dbPath string, rows map[string][]string, state map[string]string) {
	t.Helper()
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for rt, items := range rows {
		for _, it := range items {
			var m struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal([]byte(it), &m); err != nil {
				t.Fatal(err)
			}
			if err := db.Upsert(rt, m.ID, json.RawMessage(it)); err != nil {
				t.Fatal(err)
			}
		}
	}
	for rt, st := range state {
		var err error
		switch st {
		case "complete":
			err = db.SaveSyncStateAt(rt, "", len(rows[rt]), time.Now().UTC())
		case "partial":
			err = db.SaveSyncProgress(rt, "2", len(rows[rt]))
		case "cleared":
			if err = db.SaveSyncStateAt(rt, "", len(rows[rt]), time.Now().UTC()); err == nil {
				err = db.ClearSyncCursor(rt)
			}
		default:
			t.Fatalf("unknown sync state %q", st)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}

// mirrorRunner returns a command runner against a fixture API (drift reads the
// release feed live) that reports stdout and stderr separately.
func mirrorRunner(t *testing.T, f *fixtureAPI) func(args ...string) (string, string) {
	t.Helper()
	testenv.Isolate(t)
	srv := httptest.NewServer(f.handler(t))
	t.Cleanup(srv.Close)
	t.Setenv("DNSFILTER_BASE_URL", srv.URL)
	t.Setenv("DNSFILTER_API_KEY", "fixture-key")
	t.Cleanup(func() { humanFriendly = false })
	return func(args ...string) (string, string) {
		humanFriendly = false
		cmd := RootCmd()
		cmd.SetArgs(args)
		var out, errOut bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&errOut)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%v: %v (exit %d)\n%s%s", args, err, exitCode(err), out.String(), errOut.String())
		}
		return out.String(), errOut.String()
	}
}

type incompleteEnvelope struct {
	Meta *struct {
		IncompleteCollections []string `json:"incomplete_collections"`
		Note                  string   `json:"note"`
	} `json:"meta"`
	Results json.RawMessage `json:"results"`
}

// decodeIncomplete fails unless out is the incomplete-mirror envelope naming
// exactly want, with a note carrying the exact sync command.
func decodeIncomplete(t *testing.T, label, out string, want []string) json.RawMessage {
	t.Helper()
	var env incompleteEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil || env.Meta == nil {
		t.Fatalf("%s: JSON is not the incomplete-mirror envelope (err=%v); a clean-looking result over an incomplete mirror:\n%s", label, err, out)
	}
	if !reflect.DeepEqual(env.Meta.IncompleteCollections, want) {
		t.Fatalf("%s: incomplete_collections = %v, want %v", label, env.Meta.IncompleteCollections, want)
	}
	if cmd := "dnsfilter-cli sync --full --resources " + strings.Join(want, ","); !strings.HasSuffix(env.Meta.Note, cmd) {
		t.Fatalf("%s: note %q does not end with %q", label, env.Meta.Note, cmd)
	}
	return env.Results
}

// lists where with policies synced but sites, clients and orgs never walked
// returned rows with sites=0 roaming=0 and no hint: a false all-clear on impact.
func TestListsWhereIncompleteMirrorIsNotAnImpactAllClear(t *testing.T) {
	run := mirrorRunner(t, &fixtureAPI{})
	db := filepath.Join(t.TempDir(), "mirror.db")
	seedMirror(t, db, map[string][]string{"policies": {mirrorPolicy}}, map[string]string{"policies": "complete"})
	missing := []string{"organizations", "networks", "scheduled-policies", "user-agents"}

	out, _ := run("lists", "where", "example.com", "--db", db, "--json")
	var rows []listsWhereRow
	if err := json.Unmarshal(decodeIncomplete(t, "lists where --json", out, missing), &rows); err != nil || len(rows) != 1 || rows[0].PolicyID != "10" {
		t.Fatalf("lists where dropped the matching row: err=%v rows=%+v", err, rows)
	}

	agentOut, _ := run("lists", "where", "example.com", "--db", db, "--agent")
	decodeIncomplete(t, "lists where --agent", agentOut, missing)

	human, _ := run("lists", "where", "example.com", "--db", db, "--human-friendly")
	if !strings.Contains(human, "policy 10") || !strings.Contains(human, "Incomplete local mirror") || !strings.HasSuffix(human, "--resources organizations,networks,scheduled-policies,user-agents\n") {
		t.Fatalf("lists where human output must keep the row and name the unsynced collections:\n%s", human)
	}
}

// agents stale --org with sites and orgs never walked returned [] and blamed
// the filter; it must name the missing sync instead.
func TestStaleOrgOverUnwalkedSitesNamesTheSync(t *testing.T) {
	run := mirrorRunner(t, &fixtureAPI{})
	db := filepath.Join(t.TempDir(), "mirror.db")
	seedMirror(t, db, map[string][]string{"user-agents": {mirrorAgent()}}, map[string]string{"user-agents": "complete"})
	missing := []string{"organizations", "networks"}

	out, errOut := run("agents", "stale", "--org", "Acme", "--db", db, "--json")
	decodeIncomplete(t, "agents stale --org --json", out, missing)
	if strings.Contains(errOut, "matches --org") || !strings.Contains(errOut, "hint: Incomplete local mirror") {
		t.Fatalf("stale blamed --org instead of the missing sync:\n%s", errOut)
	}

	human, errOut := run("agents", "stale", "--org", "Acme", "--db", db, "--human-friendly")
	if strings.Contains(errOut, "matches --org") || !strings.HasSuffix(human, "--resources organizations,networks\n") {
		t.Fatalf("stale --org human output must name the missing sync:\nstdout:\n%s\nstderr:\n%s", human, errOut)
	}
}

// A first-ever interrupted user-agents walk, or a --full cursor clear that
// never re-walked, must not get the stdout all-clear computed on partial data.
func TestStalePartialClientWalkIsNotAnAllClear(t *testing.T) {
	for _, st := range []string{"partial", "cleared"} {
		t.Run(st, func(t *testing.T) {
			run := mirrorRunner(t, &fixtureAPI{})
			db := filepath.Join(t.TempDir(), "mirror.db")
			seedMirror(t, db,
				map[string][]string{"user-agents": {mirrorAgent()}, "networks": {mirrorSite}, "organizations": {mirrorOrg}},
				map[string]string{"user-agents": st, "networks": "complete", "organizations": "complete"})

			human, _ := run("agents", "stale", "--db", db, "--human-friendly")
			if strings.Contains(human, "No roaming clients silent") || !strings.HasSuffix(human, "--resources user-agents\n") {
				t.Fatalf("stale printed an all-clear over a %s client walk:\n%s", st, human)
			}
			out, _ := run("agents", "stale", "--db", db, "--json")
			decodeIncomplete(t, "agents stale --json", out, []string{"user-agents"})
		})
	}
}

// agents drift: --org over unwalked sites blamed the filter, and the
// all-clear printed over an incomplete join.
func TestDriftIncompleteMirrorIsNotAnAllClear(t *testing.T) {
	run := mirrorRunner(t, &fixtureAPI{releases: []string{mirrorFeedLatest}})
	db := filepath.Join(t.TempDir(), "mirror.db")
	seedMirror(t, db, map[string][]string{"user-agents": {mirrorAgent()}}, map[string]string{"user-agents": "complete"})
	missing := []string{"organizations", "networks"}

	for _, args := range [][]string{{"agents", "drift"}, {"agents", "drift", "--org", "Acme"}} {
		out, errOut := run(append(args, "--db", db, "--json")...)
		var v driftView
		if err := json.Unmarshal([]byte(out), &v); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		if !reflect.DeepEqual(v.IncompleteCollections, missing) || !strings.HasSuffix(v.Note, "dnsfilter-cli sync --full --resources organizations,networks") {
			t.Fatalf("%v: incomplete_collections=%v note=%q", args, v.IncompleteCollections, v.Note)
		}
		if !strings.Contains(errOut, "hint: Incomplete local mirror") {
			t.Fatalf("%v: no stderr hint:\n%s", args, errOut)
		}
		human, _ := run(append(args, "--db", db, "--human-friendly")...)
		if strings.Contains(human, "No client org has roaming clients behind") || strings.Contains(human, "widen --org") || !strings.Contains(human, "--resources organizations,networks\n") {
			t.Fatalf("%v: drift human output over an incomplete mirror:\n%s", args, human)
		}
	}
}

// With every joined collection complete the output is what it was before the
// completeness check: the bare JSON rows, no incomplete_collections key, and
// the exact human all-clear lines.
func TestNovelReadersCompleteMirrorOutputUnchanged(t *testing.T) {
	run := mirrorRunner(t, &fixtureAPI{releases: []string{mirrorFeedLatest}})
	db := filepath.Join(t.TempDir(), "mirror.db")
	seedMirror(t, db,
		map[string][]string{"user-agents": {mirrorAgent()}, "networks": {mirrorSite}, "organizations": {mirrorOrg}, "policies": {mirrorPolicy}},
		map[string]string{"user-agents": "complete", "networks": "complete", "organizations": "complete", "policies": "complete", "scheduled-policies": "complete"})

	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"agents", "stale", "--human-friendly"}, "No roaming clients silent for more than 7d or unprotected.\n"},
		{[]string{"agents", "stale", "--json"}, "[]\n"},
		{[]string{"agents", "stale", "--org", "Nope", "--json"}, "[]\n"},
		{[]string{"agents", "drift", "--human-friendly"}, "channel stable (default), latest windows 3.2.0, 1 roaming clients scanned\nNo client org has roaming clients behind the latest release.\n"},
		{[]string{"lists", "where", "nothing.test", "--human-friendly"}, "No synced policy lists nothing.test or a parent domain.\n"},
		{[]string{"lists", "where", "nothing.test", "--json"}, "[]\n"},
		{[]string{"lists", "where", "example.com", "--human-friendly"}, "allow example.com                  Acme Dental              policy 10 \"Std\" sites=1 roaming=1 global=false\n"},
	} {
		out, errOut := run(append(c.args, "--db", db)...)
		if out != c.want {
			t.Fatalf("%v over a complete mirror:\n got %q\nwant %q", c.args, out, c.want)
		}
		if strings.Contains(errOut, "Incomplete local mirror") {
			t.Fatalf("%v: incomplete hint over a complete mirror: %s", c.args, errOut)
		}
	}
	out, errOut := run("agents", "stale", "--org", "Nope", "--db", db, "--json")
	if out != "[]\n" || !strings.Contains(errOut, `matches --org "Nope"; nothing was checked`) {
		t.Fatalf("complete mirror, --org matching nothing must keep the nothing-checked hint:\n%s%s", out, errOut)
	}
	drift, _ := run("agents", "drift", "--db", db, "--json")
	if strings.Contains(drift, "incomplete_collections") {
		t.Fatalf("drift JSON over a complete mirror gained incomplete_collections:\n%s", drift)
	}
}

// incompleteCollections: complete rows pass; partial, cleared and never-synced
// rows fail; a read-only store that predates last_attempt_complete proves
// nothing and fails every collection instead of erroring.
func TestNovelReadersIncompleteCollectionsStates(t *testing.T) {
	testenv.Isolate(t)
	dbPath := filepath.Join(t.TempDir(), "mirror.db")
	seedMirror(t, dbPath, nil, map[string]string{"organizations": "complete", "networks": "partial", "policies": "cleared"})
	ro, err := store.OpenReadOnly(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	got, err := incompleteCollections(t.Context(), ro.DB(), "organizations", "networks", "policies", "user-agents")
	ro.Close()
	if err != nil || !reflect.DeepEqual(got, []string{"networks", "policies", "user-agents"}) {
		t.Fatalf("incompleteCollections = %v, %v", got, err)
	}

	rw, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rw.DB().Exec(`ALTER TABLE sync_state DROP COLUMN last_attempt_complete`); err != nil {
		t.Fatal(err)
	}
	rw.Close()
	ro, err = store.OpenReadOnly(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	got, err = incompleteCollections(t.Context(), ro.DB(), "organizations", "networks")
	if err != nil || !reflect.DeepEqual(got, []string{"organizations", "networks"}) {
		t.Fatalf("legacy store: incompleteCollections = %v, %v", got, err)
	}
}

// scheduleSlots builds a 672-slot policy_ids array: the first half of the
// week uses first, the second half second (JSON numbers, as the API sends).
func scheduleSlots(first, second int) string {
	slots := make([]string, 672)
	for i := range slots {
		slots[i] = fmt.Sprint(first)
		if i >= 336 {
			slots[i] = fmt.Sprint(second)
		}
	}
	return "[" + strings.Join(slots, ",") + "]"
}

// lists where counts a site or roaming client as governed by a policy when
// any slot of its scheduled policy (own or inherited from the site) uses it.
// Fully synced, a policy reached only through schedules reported sites=0
// roaming_clients=0 while governing those deployments.
func TestListsWhereCountsScheduleGovernedDeployments(t *testing.T) {
	run := mirrorRunner(t, &fixtureAPI{})
	db := filepath.Join(t.TempDir(), "mirror.db")
	now := time.Now().UTC().Format(time.RFC3339)
	client := func(id, rels string) string {
		return `{"id":"` + id + `","attributes":{"hostname":"PC-` + id + `","agent_type":"windows","agent_version":"3.2.0","status":"active","agent_state":"protected","last_sync":"` + now + `"},"relationships":{` + rels + `}}`
	}
	seedMirror(t, db, map[string][]string{
		"organizations": {mirrorOrg},
		"policies": {mirrorPolicy,
			`{"id":"11","attributes":{"name":"Annex","is_global_policy":false,"whitelist_domains":[],"blacklist_domains":["example.com"]},"relationships":{"organization":{"data":{"id":"1"}}}}`},
		"scheduled-policies": {
			`{"id":"7","attributes":{"name":"Office hours","timezone":"America/New_York","policy_ids":` + scheduleSlots(10, 11) + `}}`,
			`{"id":"8","attributes":{"name":"Always Std","timezone":"UTC","policy_ids":` + scheduleSlots(10, 10) + `}}`},
		"networks": {
			`{"id":"50","attributes":{"name":"HQ"},"relationships":{"organization":{"data":{"id":"1"}},"scheduled_policy":{"data":{"id":"7"}}}}`,
			`{"id":"51","attributes":{"name":"Annex"},"relationships":{"organization":{"data":{"id":"1"}},"policy":{"data":{"id":"11"}}}}`},
		"user-agents": {
			client("a1", `"network":{"data":{"id":"50"}}`),                                        // inherits site 50's schedule (10 and 11)
			client("a2", `"network":{"data":{"id":"51"}},"scheduled_policy":{"data":{"id":"8"}}`), // own schedule (10)
			client("a3", `"network":{"data":{"id":"51"}}`),                                        // inherits site 51's policy 11
		},
	}, map[string]string{"organizations": "complete", "policies": "complete", "scheduled-policies": "complete", "networks": "complete", "user-agents": "complete"})

	out, errOut := run("lists", "where", "example.com", "--db", db, "--json")
	var rows []listsWhereRow
	if err := json.Unmarshal([]byte(out), &rows); err != nil || len(rows) != 2 {
		t.Fatalf("lists where over a complete mirror: err=%v\n%s%s", err, out, errOut)
	}
	byPolicy := map[string]listsWhereRow{}
	for _, r := range rows {
		byPolicy[r.PolicyID] = r
	}
	// Policy 10 governs only through schedules: site 50 (schedule 7), a1
	// (site 50's schedule) and a2 (own schedule 8).
	if r := byPolicy["10"]; r.Sites != 1 || r.RoamingClients != 2 || r.ScheduledSites != 1 || r.ScheduledRoamingClients != 2 {
		t.Fatalf("policy 10 (schedule-only) counts = %+v, want sites=1 roaming=2 scheduled 1/2", r)
	}
	// Policy 11: site 51 directly plus site 50's schedule; a3 inherits 51, a1 site 50's schedule.
	if r := byPolicy["11"]; r.Sites != 2 || r.RoamingClients != 2 || r.ScheduledSites != 1 || r.ScheduledRoamingClients != 1 {
		t.Fatalf("policy 11 counts = %+v, want sites=2 roaming=2 scheduled 1/1", r)
	}
	if strings.Contains(errOut, "Incomplete local mirror") {
		t.Fatalf("complete mirror flagged incomplete: %s", errOut)
	}
	human, _ := run("lists", "where", "example.com", "--db", db, "--human-friendly")
	if !strings.Contains(human, `policy 10 "Std" sites=1 roaming=2 global=false (via a schedule slot: sites=1 roaming=2)`) {
		t.Fatalf("human output does not show schedule-governed impact:\n%s", human)
	}
}

// A deployment naming a schedule the mirror lacks, or one whose slots name no
// policy, has unknown policies: the counts are not an all-clear. With
// scheduled-policies completely synced, re-syncing cannot fix that, so it is
// reported as unresolved_schedules, never as an incomplete collection; only a
// truly incomplete scheduled-policies sync goes in incomplete_collections.
func TestListsWhereUnresolvedScheduleIsNotAllClear(t *testing.T) {
	run := mirrorRunner(t, &fixtureAPI{})
	db := filepath.Join(t.TempDir(), "mirror.db")
	all := map[string]string{"organizations": "complete", "policies": "complete", "scheduled-policies": "complete", "networks": "complete", "user-agents": "complete"}
	seedMirror(t, db, map[string][]string{
		"organizations":      {mirrorOrg},
		"policies":           {mirrorPolicy},
		"scheduled-policies": {`{"id":"6","attributes":{"name":"Empty","timezone":"UTC","policy_ids":` + scheduleSlots(0, 0) + `}}`},
		"networks": {
			`{"id":"50","attributes":{"name":"HQ"},"relationships":{"organization":{"data":{"id":"1"}},"scheduled_policy":{"data":{"id":"9"}}}}`,
			`{"id":"51","attributes":{"name":"Annex"},"relationships":{"organization":{"data":{"id":"1"}},"scheduled_policy":{"data":{"id":"6"}}}}`},
	}, all)

	out, errOut := run("lists", "where", "example.com", "--db", db, "--json")
	var env struct {
		Meta struct {
			IncompleteCollections []string `json:"incomplete_collections"`
			Note                  string   `json:"note"`
			UnresolvedSchedules   []string `json:"unresolved_schedules"`
		} `json:"meta"`
		Results []listsWhereRow `json:"results"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil || env.Meta.IncompleteCollections == nil {
		t.Fatalf("unresolved schedules must withhold the all-clear in the envelope (err=%v):\n%s", err, out)
	}
	if len(env.Meta.IncompleteCollections) != 0 || !reflect.DeepEqual(env.Meta.UnresolvedSchedules, []string{"6", "9"}) || len(env.Results) != 1 {
		t.Fatalf("complete mirror, dangling schedule 9 and empty schedule 6: meta=%+v results=%d", env.Meta, len(env.Results))
	}
	if !strings.HasPrefix(env.Meta.Note, "2 site or roaming-client assignment(s) use a scheduled policy that is not in the local mirror") || strings.Contains(env.Meta.Note, "sync") {
		t.Fatalf("note must say the schedules are unresolved, not ask for a sync that cannot fix it: %q", env.Meta.Note)
	}
	if !strings.Contains(errOut, "hint: 2 site or roaming-client assignment(s)") {
		t.Fatalf("no stderr hint: %s", errOut)
	}
	human, _ := run("lists", "where", "example.com", "--db", db, "--human-friendly")
	if !strings.Contains(human, "policy 10") || !strings.HasSuffix(human, "(scheduled policy ids 6, 9); their impact counts are unknown, so this is not an all-clear.\n") {
		t.Fatalf("human output must keep the row and say the schedules are unresolved:\n%s", human)
	}

	// scheduled-policies really incomplete: it is an incomplete collection
	// (remedy: sync --full) and the dangling ids are still reported.
	seedMirror(t, db, nil, map[string]string{"scheduled-policies": "partial"})
	out, _ = run("lists", "where", "example.com", "--db", db, "--json")
	env.Meta.UnresolvedSchedules = nil
	if err := json.Unmarshal(decodeIncomplete(t, "partial scheduled-policies", out, []string{"scheduled-policies"}), &env.Results); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil || !reflect.DeepEqual(env.Meta.UnresolvedSchedules, []string{"6", "9"}) {
		t.Fatalf("partial scheduled-policies: unresolved_schedules = %v (err %v)", env.Meta.UnresolvedSchedules, err)
	}
}

// The remedy for an incomplete collection is sync --full: a plain sync resumes
// an interrupted or page-capped walk from its cursor and stays incomplete.
func TestNovelReadersRemedyIsFullSync(t *testing.T) {
	run := mirrorRunner(t, &fixtureAPI{releases: []string{mirrorFeedLatest}})
	db := filepath.Join(t.TempDir(), "mirror.db")
	seedMirror(t, db, map[string][]string{"user-agents": {mirrorAgent()}}, map[string]string{"user-agents": "partial"})
	for _, args := range [][]string{{"agents", "stale"}, {"agents", "drift"}, {"lists", "where", "example.com"}} {
		human, _ := run(append(args, "--db", db, "--human-friendly")...)
		if !strings.Contains(human, "Run: dnsfilter-cli sync --full --resources ") || strings.Contains(human, "Run: dnsfilter-cli sync --resources ") {
			t.Fatalf("%v: remedy must be sync --full:\n%s", args, human)
		}
	}
	empty := filepath.Join(t.TempDir(), "absent.db")
	_, errOut := run("agents", "stale", "--db", empty, "--json")
	if !strings.Contains(errOut, "Run: dnsfilter-cli sync --full --resources organizations,networks,user-agents --db ") {
		t.Fatalf("no-mirror hint must give sync --full: %s", errOut)
	}
	if out, _ := run("agents", "drift", "--db", empty, "--json"); !strings.Contains(out, "run: dnsfilter-cli sync --full --resources organizations,networks,user-agents") {
		t.Fatalf("drift no-mirror note must give sync --full:\n%s", out)
	}
	// An interrupted first walk that stored no client yet.
	noRows := filepath.Join(t.TempDir(), "norows.db")
	seedMirror(t, noRows, nil, map[string]string{"user-agents": "partial"})
	if human, _ := run("agents", "stale", "--db", noRows, "--human-friendly"); !strings.Contains(human, "Run: dnsfilter-cli sync --full --resources organizations,networks,user-agents") {
		t.Fatalf("stale empty-mirror line must give sync --full:\n%s", human)
	}
}

// A scheduled assignment replaces the direct one, as why-blocked resolves it:
// a site or roaming client that keeps direct policy 10 but runs a schedule
// whose every slot is policy 20 is governed by 20 only. The site also lists
// 10 in its to-many policies relationship, which why-blocked ignores, and
// so does site 51 (direct policy 11, no schedule).
func TestListsWhereScheduleReplacesDirectPolicy(t *testing.T) {
	run := mirrorRunner(t, &fixtureAPI{})
	db := filepath.Join(t.TempDir(), "mirror.db")
	now := time.Now().UTC().Format(time.RFC3339)
	client := func(id, rels string) string {
		return `{"id":"` + id + `","attributes":{"hostname":"PC-` + id + `","agent_type":"windows","agent_version":"3.2.0","status":"active","agent_state":"protected","last_sync":"` + now + `"},"relationships":{` + rels + `}}`
	}
	seedMirror(t, db, map[string][]string{
		"organizations": {mirrorOrg},
		"policies": {mirrorPolicy,
			`{"id":"20","attributes":{"name":"Sched","is_global_policy":false,"whitelist_domains":["example.com"],"blacklist_domains":[]},"relationships":{"organization":{"data":{"id":"1"}}}}`},
		"scheduled-policies": {`{"id":"8","attributes":{"name":"Always 20","timezone":"UTC","policy_ids":` + scheduleSlots(20, 20) + `}}`},
		"networks": {`{"id":"50","attributes":{"name":"HQ"},"relationships":{"organization":{"data":{"id":"1"}},"policy":{"data":{"id":"10"}},"policies":{"data":[{"id":"10"}]},"scheduled_policy":{"data":{"id":"8"}}}}`,
			`{"id":"51","attributes":{"name":"Annex"},"relationships":{"organization":{"data":{"id":"1"}},"policy":{"data":{"id":"11"}},"policies":{"data":[{"id":"10"}]}}}`},
		"user-agents": {
			client("a1", `"network":{"data":{"id":"50"}},"policy":{"data":{"id":"10"}},"scheduled_policy":{"data":{"id":"8"}}`), // own direct 10, own schedule 8
			client("a2", `"network":{"data":{"id":"50"}}`), // inherits site 50: its schedule, not its retained direct 10
		},
	}, map[string]string{"organizations": "complete", "policies": "complete", "scheduled-policies": "complete", "networks": "complete", "user-agents": "complete"})

	out, errOut := run("lists", "where", "example.com", "--db", db, "--json")
	var rows []listsWhereRow
	if err := json.Unmarshal([]byte(out), &rows); err != nil || len(rows) != 2 {
		t.Fatalf("lists where: err=%v\n%s%s", err, out, errOut)
	}
	for _, r := range rows {
		switch r.PolicyID {
		case "10":
			if r.Sites != 0 || r.RoamingClients != 0 {
				t.Fatalf("policy 10 is overridden by schedule 8 everywhere, got %+v", r)
			}
		case "20":
			if r.Sites != 1 || r.RoamingClients != 2 || r.ScheduledSites != 1 || r.ScheduledRoamingClients != 2 {
				t.Fatalf("policy 20 (every slot of schedule 8) = %+v, want sites=1 roaming=2 scheduled 1/2", r)
			}
		}
	}
}

// A sync that commits new rows and marks the collection complete after a
// reader loaded its rows but before it read sync_state must not let the
// reader pair the old rows with the new completeness flag: rows and the flag
// come from one snapshot, so the reader still sees the collection incomplete.
func TestNovelReadersRowsAndCompletenessShareOneSnapshot(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339)
	client := func(id, version, state string) string {
		return `{"id":"` + id + `","attributes":{"hostname":"PC-` + id + `","agent_type":"windows","agent_version":"` + version + `","status":"active","agent_state":"` + state + `","last_sync":"` + now + `"},"relationships":{"network":{"data":{"id":"50"}}}}`
	}
	cases := []struct {
		name     string
		args     []string
		rows     map[string][]string
		state    map[string]string
		commit   map[string][]string // committed between the reader's two reads
		complete string              // collection the commit marks complete
		allClear string
	}{
		{
			name:   "agents stale",
			args:   []string{"agents", "stale"},
			rows:   map[string][]string{"user-agents": {client("a1", "3.2.0", "protected")}, "networks": {mirrorSite}, "organizations": {mirrorOrg}},
			state:  map[string]string{"user-agents": "partial", "networks": "complete", "organizations": "complete"},
			commit: map[string][]string{"user-agents": {client("a2", "3.2.0", "unprotected")}}, complete: "user-agents",
			allClear: "No roaming clients silent",
		},
		{
			name:   "agents drift",
			args:   []string{"agents", "drift"},
			rows:   map[string][]string{"user-agents": {client("a1", "3.2.0", "protected")}, "networks": {mirrorSite}, "organizations": {mirrorOrg}},
			state:  map[string]string{"user-agents": "partial", "networks": "complete", "organizations": "complete"},
			commit: map[string][]string{"user-agents": {client("a2", "3.1.0", "protected")}}, complete: "user-agents",
			allClear: "No client org has roaming clients behind",
		},
		{
			name:   "lists where",
			args:   []string{"lists", "where", "example.com"},
			rows:   map[string][]string{"policies": {mirrorPolicy}, "organizations": {mirrorOrg}},
			state:  map[string]string{"policies": "complete", "organizations": "complete", "networks": "partial", "user-agents": "complete", "scheduled-policies": "complete"},
			commit: map[string][]string{"networks": {mirrorSite}}, complete: "networks",
			allClear: "sites=0 roaming=0 global=false\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			run := mirrorRunner(t, &fixtureAPI{releases: []string{mirrorFeedLatest}})
			db := filepath.Join(t.TempDir(), "mirror.db")
			seedMirror(t, db, c.rows, c.state)
			fired := false
			mirrorRowsReadHook = func() {
				if fired {
					return
				}
				fired = true
				seedMirror(t, db, c.commit, map[string]string{c.complete: "complete"})
			}
			t.Cleanup(func() { mirrorRowsReadHook = nil })
			human, _ := run(append(c.args, "--db", db, "--human-friendly")...)
			if !fired {
				t.Fatal("the interleaved commit never ran")
			}
			if strings.HasSuffix(human, c.allClear) || strings.Contains(human, c.allClear+"\n") || !strings.Contains(human, "Incomplete local mirror: no complete sync of "+c.complete) {
				t.Fatalf("%s paired rows read before the commit with completeness read after it:\n%s", c.name, human)
			}
		})
	}
}
