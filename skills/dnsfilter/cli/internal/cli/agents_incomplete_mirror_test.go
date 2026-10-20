// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// The local-mirror readers (agents stale, agents drift, lists where) must not
// print an all-clear or a clean empty result when a collection their answer
// joins was never synced or its last sync attempt did not finish.

package cli

import (
	"bytes"
	"encoding/json"
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
	if cmd := "dnsfilter-cli sync --resources " + strings.Join(want, ","); !strings.HasSuffix(env.Meta.Note, cmd) {
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
	missing := []string{"organizations", "networks", "user-agents"}

	out, _ := run("lists", "where", "example.com", "--db", db, "--json")
	var rows []listsWhereRow
	if err := json.Unmarshal(decodeIncomplete(t, "lists where --json", out, missing), &rows); err != nil || len(rows) != 1 || rows[0].PolicyID != "10" {
		t.Fatalf("lists where dropped the matching row: err=%v rows=%+v", err, rows)
	}

	agentOut, _ := run("lists", "where", "example.com", "--db", db, "--agent")
	decodeIncomplete(t, "lists where --agent", agentOut, missing)

	human, _ := run("lists", "where", "example.com", "--db", db, "--human-friendly")
	if !strings.Contains(human, "policy 10") || !strings.Contains(human, "Incomplete local mirror") || !strings.HasSuffix(human, "--resources organizations,networks,user-agents\n") {
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
		if !reflect.DeepEqual(v.IncompleteCollections, missing) || !strings.HasSuffix(v.Note, "dnsfilter-cli sync --resources organizations,networks") {
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
		map[string]string{"user-agents": "complete", "networks": "complete", "organizations": "complete", "policies": "complete"})

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
	got, err := incompleteCollections(t.Context(), ro, "organizations", "networks", "policies", "user-agents")
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
	got, err = incompleteCollections(t.Context(), ro, "organizations", "networks")
	if err != nil || !reflect.DeepEqual(got, []string{"organizations", "networks"}) {
		t.Fatalf("legacy store: incompleteCollections = %v, %v", got, err)
	}
}
