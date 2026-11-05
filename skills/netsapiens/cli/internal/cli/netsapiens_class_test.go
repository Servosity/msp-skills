// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Class tests for the 4.34.2 reprint's second fix pass (after Codex round 2).
// Each names its class and failed on the round-2 tree.

package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"netsapiens-pp-cli/internal/cliutil/testenv"
	"netsapiens-pp-cli/internal/store"
)

// storeGate is what the sync command and the MCP server do to an existing
// store before they use it.
func storeGate(path string) error {
	s, err := store.Open(path)
	if err != nil {
		return err
	}
	return s.Close()
}

// baseline-identity-corruption: preparing a store for use must never rewrite
// the CLI's own rows. The baseline's identity is the documented Domain.domain
// (spec.json GET /domains), so an unchanged tenant reads as zero changes.
func TestClassBaselineIdentitySurvivesTheStoreGate(t *testing.T) {
	testenv.Isolate(t)
	path := filepath.Join(t.TempDir(), "data.db")
	seedCovered(t, path, "domains", `{"domain":"acme.example"}`)
	mustRunFleet(t, "fleet", "changes", "--rebaseline", "--db", path, "--json")
	if err := storeGate(path); err != nil {
		t.Fatal(err)
	}
	var rep changesReport
	if err := json.Unmarshal([]byte(mustRunFleet(t, "fleet", "changes", "--db", path, "--json")), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.Added != 0 || rep.Removed != 0 || rep.Changed != 0 {
		t.Fatalf("nothing changed upstream, yet fleet changes reports %d added, %d removed, %d changed: %+v", rep.Added, rep.Removed, rep.Changed, rep.Changes)
	}
}

// incomplete-audit-output: --select must never strip an incomplete audit of
// its completeness, whatever made it incomplete (here: stale coverage).
func TestClassSelectNeverDropsIncompleteness(t *testing.T) {
	testenv.Isolate(t)
	path := filepath.Join(t.TempDir(), "data.db")
	seedCovered(t, path, "domains", `{"domain":"acme.example"}`)
	seedCovered(t, path, "users", `{"domain":"acme.example","user":"100"}`)
	seedCovered(t, path, "devices", `{"domain":"acme.example","device":"100a","user":"100","device-sip-registration-state":"unregistered"}`)
	for _, rt := range rtPhones {
		seedCovered(t, path, rt)
	}
	out, _, err := runFleet(t, "fleet", "offline", "--db", path, "--max-age", "1ns", "--select", "issues", "--json")
	if err == nil && !strings.Contains(out, `"complete"`) {
		t.Fatalf("a stale audit printed without its completeness (it reads as a clean answer):\n%s", out)
	}
}

// mirror-freshness: every dependency a judgment uses is judged for age. Users
// synced two days ago cannot prove that the documented route target
// (Phonenumber.dial-rule-translation-destination-user, User.user) is gone.
func TestClassDidsJudgeEveryDependencyForAge(t *testing.T) {
	testenv.Isolate(t)
	path := filepath.Join(t.TempDir(), "data.db")
	seedCovered(t, path, "phonenumbers", `{"domain":"acme.example","phonenumber":"16025550100","dial-rule-application":"to-user","dial-rule-translation-destination-user":"100","dial-rule-translation-destination-host":"acme.example"}`)
	seedCovered(t, path, "domains", `{"domain":"acme.example"}`)
	old := time.Now().UTC().Add(-48 * time.Hour)
	seedSyncedRows(t, path, "users", old, `{"domain":"acme.example","user":"101"}`)
	seedSyncState(t, path, "users", true, old)
	seedProvenance(t, path, "users", old.Add(-time.Minute), true)
	rep := decodeObject(t, mustRunFleet(t, "fleet", "dids", "--db", path, "--json"))
	if rep["complete"] == true {
		t.Fatalf("users are 48h old (older than --max-age), yet the route judgment reads as complete: %v", rep)
	}
	if !strings.Contains(stringsOf(rep["stale_resources"]), "users") {
		t.Fatalf("the stale dependency must be named: %v", rep)
	}
}

// mirror-reconciliation: a read outside sync (write-through of a cached list)
// must not make a row the last full walk did not return look returned by it.
func TestClassWriteThroughDoesNotRejoinAWalk(t *testing.T) {
	testenv.Isolate(t)
	path := defaultDBPath("netsapiens-cli")
	before := time.Now().UTC().Add(-time.Hour)
	seedDomainRows(t, path, before, "acme.example", "beta.example")
	seedSyncState(t, path, "domains", true, before)
	seedProvenance(t, path, "domains", before.Add(-time.Minute), true)
	mustRunFleet(t, "fleet", "changes", "--rebaseline", "--json")
	// A full walk that no longer returns beta: acme is re-observed, beta is not.
	seedDomainRows(t, path, time.Now().UTC(), "acme.example")
	seedSyncState(t, path, "domains", true, time.Now().UTC())
	seedProvenance(t, path, "domains", time.Now().UTC().Add(-time.Minute), true)
	// Later, a list served from the 5-minute HTTP cache writes beta through.
	// gamma was never returned by a walk at all (pruned by --full, or cached).
	writeThroughCache(context.Background(), "domains", json.RawMessage(`[{"domain":"acme.example"},{"domain":"beta.example"},{"domain":"gamma.example"}]`))
	var rep changesReport
	if err := json.Unmarshal([]byte(mustRunFleet(t, "fleet", "changes", "--json")), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.Removed != 1 {
		t.Fatalf("the last full walk did not return beta.example; a cached read must not hide its removal: %+v", rep)
	}
	if rep.Added != 0 {
		t.Fatalf("no walk returned gamma.example; a cached read must not report it added: %+v", rep)
	}
}

// fanout-scope: a complete fan-out enumerates the live domain list, never the
// 5-minute HTTP cache (spec.json GET /domains documents both as ordinary rows).
func TestClassQueueFanoutEnumeratesLiveDomains(t *testing.T) {
	testenv.Isolate(t)
	var second atomic.Bool
	var betaChecked atomic.Bool
	fakeNetsapiens(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/domains"):
			if second.Load() {
				writeJSON(w, `[{"domain":"acme.example"},{"domain":"beta.example"}]`)
				return
			}
			writeJSON(w, `[{"domain":"acme.example"}]`)
		case strings.Contains(r.URL.Path, "/domains/beta.example/"):
			betaChecked.Store(true)
			writeJSON(w, `[]`)
		default:
			writeJSON(w, `[]`)
		}
	})
	_, _, _ = runFleet(t, "fleet", "queues", "--max-domains", "0", "--json")
	second.Store(true)
	_, _, _ = runFleet(t, "fleet", "queues", "--max-domains", "0", "--json")
	if !betaChecked.Load() {
		t.Fatal("beta.example was added upstream, but the second fan-out never checked it (the domain list came from the HTTP cache)")
	}
}

// identifier-disambiguation: a namespace is searched only when the identifier
// fits its syntax. AC:60:25:55:01:42 is a MAC (Device.device-provisioning-mac-address),
// not the phone number 6025550142 (Phonenumber.phonenumber).
func TestClassMACIsNotSearchedAsAPhoneNumber(t *testing.T) {
	testenv.Isolate(t)
	path := filepath.Join(t.TempDir(), "data.db")
	seedCovered(t, path, "devices", `{"domain":"acme.example","device":"100a","user":"100","device-provisioning-mac-address":"ac6025550142"}`)
	seedCovered(t, path, "phonenumbers", `{"domain":"beta.example","phonenumber":"16025550142","dial-rule-application":"to-user","dial-rule-translation-destination-user":"200","dial-rule-translation-destination-host":"beta.example"}`)
	seedCovered(t, path, "users", `{"domain":"acme.example","user":"100"}`, `{"domain":"beta.example","user":"200"}`)
	seedCovered(t, path, "smsnumbers")
	for _, rt := range rtPhones {
		seedCovered(t, path, rt)
	}
	out := mustRunFleet(t, "trace", "AC:60:25:55:01:42", "--db", path, "--json")
	if strings.Contains(out, "beta.example") {
		t.Fatalf("the MAC resolved to an unrelated tenant's number:\n%s", out)
	}
}

// seedDomainRows stores domains under their documented identity (Domain.domain),
// as a sync or a write-through does.
func seedDomainRows(t *testing.T, path string, at time.Time, names ...string) {
	t.Helper()
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, n := range names {
		if _, err := db.DB().Exec(`INSERT OR REPLACE INTO resources (id, resource_type, data, synced_at, updated_at) VALUES (?, 'domains', ?, ?, ?)`,
			n, `{"domain":"`+n+`"}`, at.Format(time.RFC3339), at.Format(time.RFC3339)); err != nil {
			t.Fatal(err)
		}
	}
}

func stringsOf(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
