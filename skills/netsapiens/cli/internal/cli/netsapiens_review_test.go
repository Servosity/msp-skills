// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Regression tests for the press 4.34.2 reprint's Codex review (round 1). Each
// test names the finding class it pins; each failed on the tree the review read.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"netsapiens-pp-cli/internal/cliutil/testenv"
	"netsapiens-pp-cli/internal/store"
)

// runFleet runs the root command and returns stdout, stderr and the error.
func runFleet(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := RootCmd()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errb.String(), err
}

func mustRunFleet(t *testing.T, args ...string) string {
	t.Helper()
	out, errs, err := runFleet(t, args...)
	if err != nil {
		t.Fatalf("%v: %v\nstdout=%s\nstderr=%s", args, err, out, errs)
	}
	return out
}

func decodeObject(t *testing.T, out string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("stdout is not a JSON object (an audit result must carry its completeness): %v\n%s", err, out)
	}
	return m
}

// seedSyncedRows writes rows of one resource type with an explicit synced_at.
func seedSyncedRows(t *testing.T, path, rt string, syncedAt time.Time, docs ...string) {
	t.Helper()
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for i, d := range docs {
		if _, err := db.DB().Exec(`INSERT OR REPLACE INTO resources (id, resource_type, data, synced_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
			fmt.Sprintf("%s-%d", rt, i), rt, d, syncedAt.UTC().Format(time.RFC3339), syncedAt.UTC().Format(time.RFC3339)); err != nil {
			t.Fatal(err)
		}
	}
}

// seedSyncState writes the sync_state row a sync leaves for rt.
func seedSyncState(t *testing.T, path, rt string, complete bool, at time.Time) {
	t.Helper()
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	c := 0
	if complete {
		c = 1
	}
	if _, err := db.DB().Exec(`INSERT OR REPLACE INTO sync_state (resource_type, last_synced_at, total_count, last_attempt_complete) VALUES (?, ?, 0, ?)`,
		rt, at.UTC().Format(time.RFC3339), c); err != nil {
		t.Fatal(err)
	}
}

// seedProvenance records what the sync command records after a walk of rt
// that started at runStarted (authoritative = unfiltered and uncapped).
func seedProvenance(t *testing.T, path, rt string, runStarted time.Time, authoritative bool) {
	t.Helper()
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.DB().Exec(fleetProvenanceDDLForTest); err != nil {
		t.Fatal(err)
	}
	var state string
	_ = db.DB().QueryRow(`SELECT COALESCE(CAST(last_synced_at AS TEXT), '') FROM sync_state WHERE resource_type = ?`, rt).Scan(&state)
	a := 0
	if authoritative {
		a = 1
	}
	if _, err := db.DB().Exec(`INSERT OR REPLACE INTO fleet_sync_provenance (resource_type, run_started, state_synced_at, authoritative, detail) VALUES (?, ?, ?, ?, '')`,
		rt, runStarted.UTC().Format(time.RFC3339), state, a); err != nil {
		t.Fatal(err)
	}
}

// The provenance table as the sync command creates it (kept literal here so the
// tests describe the stored contract, not the implementation's constant).
const fleetProvenanceDDLForTest = `CREATE TABLE IF NOT EXISTS fleet_sync_provenance (
	resource_type TEXT PRIMARY KEY,
	run_started TEXT NOT NULL,
	state_synced_at TEXT NOT NULL,
	authoritative INTEGER NOT NULL,
	detail TEXT NOT NULL DEFAULT ''
)`

// seedCovered leaves rt exactly as a completed, unfiltered sync that started a
// minute ago leaves it.
func seedCovered(t *testing.T, path, rt string, docs ...string) {
	t.Helper()
	now := time.Now().UTC()
	seedSyncedRows(t, path, rt, now, docs...)
	seedSyncState(t, path, rt, true, now)
	seedProvenance(t, path, rt, now.Add(-time.Minute), true)
}

func fakeNetsapiens(t *testing.T, h http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	t.Setenv("NETSAPIENS_BASE_URL", srv.URL)
	if os.Getenv("NETSAPIENS_API_KEY") == "" {
		t.Setenv("NETSAPIENS_API_KEY", "test-key")
	}
	t.Setenv("NETSAPIENS_RETRY_MAX", "0")
}

func writeJSON(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(body))
}

func storeFileBytes(t *testing.T, path string) []byte {
	t.Helper()
	var all []byte
	for _, p := range []string{path, path + "-wal", path + "-shm"} {
		b, err := os.ReadFile(p)
		if err == nil {
			all = append(all, b...)
		}
	}
	if len(all) == 0 {
		t.Fatalf("no store at %s", path)
	}
	return all
}

// credential-persistence: a vendor-returned secret (the documented billing
// smtp_pwd, a device's SIP registration password, a voicemail PIN) must never
// be written to the local store, where the MCP sql/search tools can read it.
func TestReviewStoreNeverPersistsVendorSecrets(t *testing.T) {
	testenv.Isolate(t)
	const smtp, sip, pin, prov = "MailSecret987654", "SipSecret55443322", "Pin8675309Secret", "ProvSecret24681357"
	fakeNetsapiens(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/domains"):
			writeJSON(w, `[{"domain":"acme.example","email-send-smtp-password":"`+smtp+`"}]`)
		case strings.HasSuffix(r.URL.Path, "/billing"):
			writeJSON(w, `{"domain":"acme.example","count-users-total":3,"smtp_pwd":"`+smtp+`"}`)
		case strings.HasSuffix(r.URL.Path, "/devices"):
			writeJSON(w, `[{"domain":"acme.example","device":"100a","user":"100","device-sip-registration-password":"`+sip+`","device-provisioning-password":"`+prov+`"}]`)
		case strings.HasSuffix(r.URL.Path, "/users"):
			writeJSON(w, `[{"domain":"acme.example","user":"100","voicemail-login-pin":"`+pin+`","password":{"nested":"`+pin+`"}}]`)
		default:
			writeJSON(w, `[]`)
		}
	})
	path := filepath.Join(t.TempDir(), "data.db")
	mustRunFleet(t, "sync", "--resources", "domains,billing,devices,users", "--db", path)
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	_ = db.DB().QueryRow(`SELECT COUNT(*) FROM resources WHERE resource_type IN ('billing','devices','users','domains')`).Scan(&n)
	db.Close()
	if n != 4 {
		t.Fatalf("want 4 synced rows, got %d", n)
	}
	raw := storeFileBytes(t, path)
	for _, s := range []string{smtp, sip, pin, prov} {
		if bytes.Contains(raw, []byte(s)) {
			t.Errorf("secret %q persisted in the local store", s)
		}
	}
}

// credential-output: an error built from a response (an HTML page on a JSON
// endpoint, here echoing the API key) must reach no output unmasked.
func TestReviewResponseDerivedErrorsAreMasked(t *testing.T) {
	testenv.Isolate(t)
	const key = "ReviewSecret987654"
	t.Setenv("NETSAPIENS_API_KEY", key)
	fakeNetsapiens(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><title>` + key + `</title></head><body>proxy</body></html>`))
	})
	for _, args := range [][]string{
		{"fleet", "missed", "--domain", "acme.example", "--no-cache", "--json"},
		{"fleet", "queues", "--domain", "acme.example", "--no-cache", "--json"},
		{"domains", "list", "--no-cache", "--json", "--data-source", "live"},
	} {
		out, errs, err := runFleet(t, args...)
		all := out + errs
		if err != nil {
			all += err.Error()
		}
		if strings.Contains(all, key) {
			t.Errorf("%v leaked the API key:\n%s", args, all)
		}
	}
}

// incomplete-audit-output: an audit whose required source did not complete a
// sync must say so on stdout in every format and never print an all-clear.
func TestReviewIncompleteAuditsNeverReadClean(t *testing.T) {
	testenv.Isolate(t)
	path := filepath.Join(t.TempDir(), "data.db")
	now := time.Now().UTC()
	seedCovered(t, path, "domains", `{"domain":"acme.example","caller-id-number-emergency":"16025550100"}`)
	seedCovered(t, path, "users", `{"domain":"acme.example","user":"100","caller-id-number-emergency":"16025550100"}`)
	seedCovered(t, path, "phones", `{"domain":"acme.example","device-provisioning-mac-address":"0004f2aabbcc","device-provisioning-last-retrieved-datetime":"`+now.Format(time.RFC3339)+`"}`)
	seedCovered(t, path, "domains_addresses")
	seedCovered(t, path, "billing", `{"domain":"acme.example","count-users-total":1,"limits-max-users":5}`)
	// devices: the latest sync failed part-way, after storing an unregistered phone
	seedSyncedRows(t, path, "devices", now, `{"domain":"acme.example","device":"100a","user":"100","device-sip-registration-state":"unregistered"}`)
	seedSyncState(t, path, "devices", false, now)
	// phonenumbers: never completed
	seedSyncedRows(t, path, "phonenumbers", now, `{"domain":"acme.example","phonenumber":"16025550100","dial-rule-application":"to-user","dial-rule-translation-destination-user":"999","dial-rule-translation-destination-host":"acme.example"}`)
	seedSyncState(t, path, "phonenumbers", false, now)

	cases := []struct {
		args     []string
		allClear string
	}{
		{[]string{"fleet", "offline"}, "No offline"},
		{[]string{"fleet", "dids"}, "No matching phone numbers"},
		{[]string{"fleet", "e911"}, "No E911 gaps"},
	}
	for _, c := range cases {
		human := mustRunFleet(t, append(append([]string{}, c.args...), "--db", path, "--human-friendly")...)
		if strings.Contains(human, c.allClear) || !strings.Contains(strings.ToLower(human), "incomplete") {
			t.Errorf("%v: human output must state incompleteness, not %q:\n%s", c.args, c.allClear, human)
		}
		m := decodeObject(t, mustRunFleet(t, append(append([]string{}, c.args...), "--db", path, "--json")...))
		if m["complete"] != false {
			t.Errorf("%v --json: complete must be false: %v", c.args, m)
		}
		if _, _, err := runFleet(t, append(append([]string{}, c.args...), "--db", path, "--csv")...); err == nil {
			t.Errorf("%v --csv: an incomplete audit cannot carry its completeness in CSV and must refuse", c.args)
		}
	}
	// no mirror at all is the most incomplete state
	absent := filepath.Join(t.TempDir(), "absent.db")
	for _, sub := range []string{"seats", "offline", "dids", "e911"} {
		m := decodeObject(t, mustRunFleet(t, "fleet", sub, "--db", absent, "--json"))
		if m["complete"] != false {
			t.Errorf("fleet %s with no mirror: complete must be false: %v", sub, m)
		}
	}
}

// mirror-coverage: a filtered walk (here --resource-param users:site=HQ)
// completes its checkpoint but does not cover the table, so no negative
// answer may rest on it.
func TestReviewFilteredSyncIsNotCoverage(t *testing.T) {
	testenv.Isolate(t)
	fakeNetsapiens(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/domains"):
			writeJSON(w, `[{"domain":"acme.example"}]`)
		case strings.HasSuffix(r.URL.Path, "/users"):
			if r.URL.Query().Get("site") == "HQ" {
				writeJSON(w, `[{"domain":"acme.example","user":"100","site":"HQ"}]`)
				return
			}
			writeJSON(w, `[{"domain":"acme.example","user":"100","site":"HQ"},{"domain":"acme.example","user":"204","site":"Branch"}]`)
		default:
			writeJSON(w, `[]`)
		}
	})
	path := filepath.Join(t.TempDir(), "data.db")
	mustRunFleet(t, "sync", "--resources", "domains,users", "--resource-param", "users:site=HQ", "--db", path)
	m := decodeObject(t, mustRunFleet(t, "trace", "204", "--domain", "acme.example", "--db", path, "--json"))
	if m["complete"] != false {
		t.Fatalf("a site-filtered users sync must not back a complete negative: %v", m)
	}
	// an unfiltered sync covers it again
	mustRunFleet(t, "sync", "--resources", "domains,users", "--db", path)
	m = decodeObject(t, mustRunFleet(t, "trace", "204", "--domain", "acme.example", "--db", path, "--json"))
	if ms, _ := m["matches"].([]any); len(ms) != 1 {
		t.Fatalf("after an unfiltered sync user 204 must be found: %v", m)
	}
}

// coverage-lineage: a dependent walk only covers the parents that were stored
// when it ran; it is coverage only after a complete parent walk.
func TestReviewDependentCoverageNeedsCompleteParents(t *testing.T) {
	testenv.Isolate(t)
	path := filepath.Join(t.TempDir(), "data.db")
	now := time.Now().UTC()
	// domains: cut short (1,000 of 1,001 stored), so users walked only those
	seedSyncedRows(t, path, "domains", now, `{"domain":"acme.example"}`)
	seedSyncState(t, path, "domains", false, now)
	seedCovered(t, path, "users", `{"domain":"acme.example","user":"100"}`)
	m := decodeObject(t, mustRunFleet(t, "trace", "204", "--domain", "d1000.example", "--db", path, "--json"))
	if m["complete"] != false {
		t.Fatalf("users walked an incomplete domain list; the negative must be unverified: %v", m)
	}
	// domains re-walked after users: users no longer covers every stored domain
	seedCovered(t, path, "domains", `{"domain":"acme.example"}`, `{"domain":"d1000.example"}`)
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = db.DB().Exec(`UPDATE fleet_sync_provenance SET run_started = ? WHERE resource_type = 'users'`, now.Add(-time.Hour).Format(time.RFC3339))
	db.Close()
	m = decodeObject(t, mustRunFleet(t, "trace", "204", "--domain", "d1000.example", "--db", path, "--json"))
	if m["complete"] != false {
		t.Fatalf("users predates the latest domains walk; the negative must be unverified: %v", m)
	}
}

// mirror-freshness: every table a lookup reads is judged for age, not only
// the first one.
func TestReviewEveryDependencyIsJudgedForAge(t *testing.T) {
	testenv.Isolate(t)
	path := filepath.Join(t.TempDir(), "data.db")
	old := time.Now().UTC().Add(-48 * time.Hour)
	for _, rt := range []string{"domains", "users", "phonenumbers", "smsnumbers", "callqueues", "autoattendants", "phones"} {
		seedCovered(t, path, rt)
	}
	seedCovered(t, path, "users", `{"domain":"acme.example","user":"100"}`)
	seedSyncedRows(t, path, "devices", old, `{"domain":"acme.example","device":"100a","user":"100","device-sip-registration-state":"registered"}`)
	seedSyncState(t, path, "devices", true, old)
	seedProvenance(t, path, "devices", old.Add(-time.Minute), true)
	out, errs, err := runFleet(t, "trace", "100", "--domain", "acme.example", "--db", path, "--json")
	if err != nil {
		t.Fatal(err)
	}
	m := decodeObject(t, out)
	if m["complete"] != false || !strings.Contains(fmt.Sprint(m["stale_resources"]), "devices") {
		t.Fatalf("a 2-day-old devices table must make the trace incomplete and be named: %v", m)
	}
	if !strings.Contains(errs, "devices") {
		t.Fatalf("no staleness hint names devices: %s", errs)
	}
}

// mirror-reconciliation: a completed walk replaces membership; a row the API
// stopped returning is gone even without --full.
func TestReviewCompletedWalkDropsRowsTheAPINoLongerReturns(t *testing.T) {
	testenv.Isolate(t)
	users := `[{"domain":"acme.example","user":"100"}]`
	fakeNetsapiens(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/domains"):
			writeJSON(w, `[{"domain":"acme.example"}]`)
		case strings.HasSuffix(r.URL.Path, "/users"):
			writeJSON(w, users)
		default:
			writeJSON(w, `[]`)
		}
	})
	path := filepath.Join(t.TempDir(), "data.db")
	mustRunFleet(t, "sync", "--resources", "domains,users", "--db", path)
	mustRunFleet(t, "fleet", "changes", "--rebaseline", "--db", path, "--json")
	// No wait: a walk starts on a later whole second than any earlier write.
	users = `[]`
	mustRunFleet(t, "sync", "--resources", "domains,users", "--db", path)
	var rep changesReport
	if err := json.Unmarshal([]byte(mustRunFleet(t, "fleet", "changes", "--db", path, "--json")), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.Removed != 1 {
		t.Fatalf("user 100 was deleted upstream; want 1 removal, got %+v", rep)
	}
}

// mirror-value-merge: a list walk is the current state; a field the API now
// returns empty must not keep its old value.
func TestReviewSyncReplacesClearedFields(t *testing.T) {
	testenv.Isolate(t)
	ecid := "16025550100"
	fakeNetsapiens(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/domains") {
			writeJSON(w, `[{"domain":"acme.example","caller-id-number-emergency":"`+ecid+`"}]`)
			return
		}
		writeJSON(w, `[]`)
	})
	path := filepath.Join(t.TempDir(), "data.db")
	mustRunFleet(t, "sync", "--resources", "domains", "--db", path)
	mustRunFleet(t, "fleet", "changes", "--rebaseline", "--db", path, "--json")
	ecid = ""
	mustRunFleet(t, "sync", "--resources", "domains", "--full", "--db", path)
	var rep changesReport
	if err := json.Unmarshal([]byte(mustRunFleet(t, "fleet", "changes", "--db", path, "--json")), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.Changed != 1 || len(rep.Changes) != 1 || rep.Changes[0].Fields["emergency_caller_id"].To != "" {
		t.Fatalf("the cleared emergency caller ID must be reported: %+v", rep)
	}
}

// fanout-scope: the queue audit enumerates the live domain list; the mirror
// may order it but never narrows it.
func TestReviewQueueFanoutUsesLiveDomains(t *testing.T) {
	testenv.Isolate(t)
	path := filepath.Join(t.TempDir(), "data.db")
	seedCovered(t, path, "domains", `{"domain":"acme.example"}`)
	seedCovered(t, path, "callqueues", `{"domain":"acme.example","callqueue":"600"}`)
	fakeNetsapiens(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/domains"):
			writeJSON(w, `[{"domain":"acme.example"},{"domain":"beta.example"}]`)
		case r.URL.Path == "/domains/beta.example/statistics/callqueues/per-queue":
			writeJSON(w, `{"700":{"name":"Support","calls-abandoned-rate":12}}`)
		case strings.HasSuffix(r.URL.Path, "/per-queue"):
			writeJSON(w, `{}`)
		default:
			writeJSON(w, `[]`)
		}
	})
	var rep queueReport
	if err := json.Unmarshal([]byte(mustRunFleet(t, "fleet", "queues", "--max-domains", "0", "--db", path, "--json")), &rep); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, q := range rep.Queues {
		if q.Domain == "beta.example" && q.Callqueue == "700" {
			found = true
		}
	}
	if !found || rep.DomainsAvailable != 2 {
		t.Fatalf("beta.example gained a queue after the last sync and must be checked: %+v", rep)
	}
}

// identifier-disambiguation: an identifier valid in several namespaces is
// searched in every one (000011223344 is a documented all-digit MAC).
func TestReviewTraceSearchesEveryCompatibleNamespace(t *testing.T) {
	testenv.Isolate(t)
	path := filepath.Join(t.TempDir(), "data.db")
	for _, rt := range []string{"domains", "phonenumbers", "smsnumbers", "callqueues", "autoattendants", "phones"} {
		seedCovered(t, path, rt)
	}
	seedCovered(t, path, "users", `{"domain":"acme.example","user":"100"}`)
	seedCovered(t, path, "devices", `{"domain":"acme.example","device":"100a","user":"100","device-provisioning-mac-address":"000011223344"}`)
	m := decodeObject(t, mustRunFleet(t, "trace", "000011223344", "--db", path, "--json"))
	ms, _ := m["matches"].([]any)
	if len(ms) == 0 || !strings.Contains(fmt.Sprint(ms), "mac") {
		t.Fatalf("the all-digit MAC must be matched as a MAC: %v", m)
	}
}

// baseline-domain-scope: --domain keeps a change that touches the domain on
// either side (a DID moved away from acme is acme's change too).
func TestReviewChangesDomainFilterMatchesEitherSide(t *testing.T) {
	testenv.Isolate(t)
	path := filepath.Join(t.TempDir(), "data.db")
	seedCovered(t, path, "phonenumbers", `{"domain":"acme.example","phonenumber":"16025550100"}`)
	mustRunFleet(t, "fleet", "changes", "--rebaseline", "--db", path, "--json")
	seedCovered(t, path, "phonenumbers", `{"domain":"beta.example","phonenumber":"16025550100"}`)
	var rep changesReport
	if err := json.Unmarshal([]byte(mustRunFleet(t, "fleet", "changes", "--domain", "acme.example", "--db", path, "--json")), &rep); err != nil {
		t.Fatal(err)
	}
	if len(rep.Changes) != 1 || rep.Changed != 1 {
		t.Fatalf("acme lost DID 16025550100 to beta; --domain acme.example must report it: %+v", rep)
	}
}

// mirror-reconciliation (resumed walk): a flat walk cut short keeps a resume
// cursor and the next run finishes it; the rows the first run stored are part
// of that walk and must not be dropped as "not returned".
func TestReviewResumedWalkKeepsItsEarlierPages(t *testing.T) {
	testenv.Isolate(t)
	var page1 strings.Builder
	page1.WriteString("[")
	for i := 0; i < 1000; i++ {
		if i > 0 {
			page1.WriteString(",")
		}
		fmt.Fprintf(&page1, `{"domain":"acme.example","phonenumber":"1602555%04d","dial-rule-application":"available"}`, i)
	}
	page1.WriteString("]")
	fakeNetsapiens(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/phonenumbers") {
			writeJSON(w, `[]`)
			return
		}
		switch r.URL.Query().Get("start") {
		case "", "0":
			writeJSON(w, page1.String())
		case "1000":
			writeJSON(w, `[{"domain":"acme.example","phonenumber":"16025559999","dial-rule-application":"available"}]`)
		default:
			writeJSON(w, `[]`)
		}
	})
	numbers := func(path string) (bool, int) {
		m := decodeObject(t, mustRunFleet(t, "fleet", "dids", "--db", path, "--json"))
		ns, _ := m["numbers"].([]any)
		return m["complete"] == true, len(ns)
	}
	path := filepath.Join(t.TempDir(), "data.db")
	mustRunFleet(t, "sync", "--resources", "phonenumbers", "--max-pages", "1", "--db", path)
	if complete, _ := numbers(path); complete {
		t.Fatal("a walk stopped at --max-pages must not be complete")
	}
	time.Sleep(1100 * time.Millisecond) // the resuming run starts in a later second
	mustRunFleet(t, "sync", "--resources", "phonenumbers", "--db", path)
	if complete, n := numbers(path); !complete || n != 1001 {
		t.Fatalf("the resumed walk covers both runs' pages: complete=%v numbers=%d, want true/1001", complete, n)
	}

	// The same resume when the walk's start was not recorded cannot be coverage.
	path2 := filepath.Join(t.TempDir(), "data.db")
	mustRunFleet(t, "sync", "--resources", "phonenumbers", "--max-pages", "1", "--db", path2)
	db, err := store.Open(path2)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = db.DB().Exec(`DELETE FROM fleet_sync_walks`)
	db.Close()
	mustRunFleet(t, "sync", "--resources", "phonenumbers", "--db", path2)
	if complete, _ := numbers(path2); complete {
		t.Fatal("a resumed walk whose start is unknown must not be complete")
	}
}

// false-RED guard: an ordinary unfiltered sync of every audit source is
// coverage; each audit must then report complete:true.
func TestReviewFullSyncIsCoverage(t *testing.T) {
	testenv.Isolate(t)
	fakeNetsapiens(t, func(w http.ResponseWriter, r *http.Request) {
		switch p := r.URL.Path; {
		case strings.HasSuffix(p, "/domains"):
			writeJSON(w, `[{"domain":"acme.example","caller-id-number-emergency":"16025550100"}]`)
		case strings.HasSuffix(p, "/billing"):
			writeJSON(w, `{"domain":"acme.example","count-users-total":1,"limits-max-users":5}`)
		case strings.HasSuffix(p, "/users"):
			writeJSON(w, `[{"domain":"acme.example","user":"100"}]`)
		case strings.HasSuffix(p, "/devices"):
			writeJSON(w, `[{"domain":"acme.example","device":"100a","user":"100","device-sip-registration-state":"registered"}]`)
		case strings.HasSuffix(p, "/addresses"):
			writeJSON(w, `[{"domain":"acme.example","emergency-address-id":"a1"}]`)
		case strings.HasSuffix(p, "/phonenumbers"):
			writeJSON(w, `[{"domain":"acme.example","phonenumber":"16025550100","dial-rule-application":"available"}]`)
		case strings.HasSuffix(p, "/phones"):
			writeJSON(w, `[{"domain":"acme.example","device-provisioning-mac-address":"0004f2aabbcc"}]`)
		default:
			writeJSON(w, `[]`)
		}
	})
	path := filepath.Join(t.TempDir(), "data.db")
	mustRunFleet(t, "sync", "--resources", "domains,users,devices,billing,domains_addresses,phonenumbers,phones", "--db", path)
	for _, sub := range []string{"seats", "offline", "dids", "e911"} {
		m := decodeObject(t, mustRunFleet(t, "fleet", sub, "--db", path, "--json"))
		if m["complete"] != true {
			t.Errorf("fleet %s after a full unfiltered sync must be complete: %v", sub, m)
		}
	}
	m := decodeObject(t, mustRunFleet(t, "trace", "100", "--domain", "acme.example", "--db", path, "--json"))
	if m["complete"] != true {
		t.Errorf("trace after a full unfiltered sync must be complete: %v", m)
	}
}

// credential-persistence (legacy store): a store a build without store
// redaction wrote is never migrated. Sync and every read refuse it and say how
// to start over, so no plaintext and no half-scrubbed store is ever used.
func TestReviewLegacyStoreIsRefused(t *testing.T) {
	testenv.Isolate(t)
	fakeNetsapiens(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/domains"):
			writeJSON(w, `[{"domain":"acme.example"}]`)
		default:
			writeJSON(w, `[]`)
		}
	})
	path := filepath.Join(t.TempDir(), "data.db")
	mustRunFleet(t, "sync", "--resources", "domains", "--db", path)
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	blob := `{"domain":"acme.example","email-send-smtp-password":"LegacySmtp24680"}`
	if _, err := db.DB().Exec(`INSERT OR REPLACE INTO resources (id, resource_type, data) VALUES ('legacy', 'domains', ?)`, blob); err != nil {
		t.Fatal(err)
	}
	_, _ = db.DB().Exec(`DELETE FROM netsapiens_store_meta`)
	_ = db.Close()
	if _, _, err := runFleet(t, "sync", "--resources", "domains", "--db", path); err == nil || !strings.Contains(err.Error(), "local-data purge") {
		t.Fatalf("sync must refuse a legacy store and name the way out, got %v", err)
	}
	if ro, err := store.OpenReadOnly(path); err == nil {
		_ = ro.Close()
		t.Fatal("a read-only open (the MCP sql and search tools) must refuse a legacy store")
	}
}

// mirror-coverage (concurrent runs): the sync_state log cannot attribute a
// write to one of two overlapping sync runs, so neither credits coverage; a
// run left open by a process that no longer exists does not block coverage.
func TestReviewConcurrentSyncRunsCreditNothing(t *testing.T) {
	testenv.Isolate(t)
	fakeNetsapiens(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/phonenumbers") {
			writeJSON(w, `[{"domain":"acme.example","phonenumber":"16025550100","dial-rule-application":"available"}]`)
			return
		}
		writeJSON(w, `[]`)
	})
	path := filepath.Join(t.TempDir(), "data.db")
	complete := func() bool {
		return decodeObject(t, mustRunFleet(t, "fleet", "dids", "--db", path, "--json"))["complete"] == true
	}
	openRun := func(pid int) {
		db, err := store.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		if _, err := db.DB().Exec(`DELETE FROM fleet_sync_runs`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.DB().Exec(`INSERT INTO fleet_sync_runs (pid, started) VALUES (?, ?)`, pid, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			t.Fatal(err)
		}
	}
	mustRunFleet(t, "sync", "--resources", "phonenumbers", "--db", path)
	if !complete() {
		t.Fatal("a lone unfiltered sync is coverage")
	}
	openRun(os.Getpid()) // another sync of this store is running
	mustRunFleet(t, "sync", "--resources", "phonenumbers", "--db", path)
	if complete() {
		t.Fatal("a sync that overlapped another run must not be credited as coverage")
	}
	openRun(1 << 30) // left open by a process that no longer exists
	mustRunFleet(t, "sync", "--resources", "phonenumbers", "--db", path)
	if !complete() {
		t.Fatal("a dead run must not block coverage")
	}
}

// A scoped report must not move every client's baseline.
func TestClassScopedRebaselineIsRefused(t *testing.T) {
	testenv.Isolate(t)
	body := `[{"domain":"a.example","reseller":"r","description":"Old"},{"domain":"b.example","reseller":"r","description":"Old"}]`
	fakeNetsapiens(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/domains") {
			writeJSON(w, body)
			return
		}
		writeJSON(w, `[]`)
	})
	path := filepath.Join(t.TempDir(), "data.db")
	mustRunFleet(t, "sync", "--resources", "domains", "--db", path)
	mustRunFleet(t, "fleet", "changes", "--rebaseline", "--db", path, "--json")
	body = `[{"domain":"a.example","reseller":"r","description":"New"},{"domain":"b.example","reseller":"r","description":"New"}]`
	mustRunFleet(t, "sync", "--resources", "domains", "--db", path)
	_, _, _ = runFleet(t, "fleet", "changes", "--domain", "a.example", "--rebaseline", "--db", path, "--json")
	var rep changesReport
	if err := json.Unmarshal([]byte(mustRunFleet(t, "fleet", "changes", "--db", path, "--json")), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.Changed != 2 {
		t.Fatalf("b.example's change was erased by a report scoped to a.example; want 2 changes, got %+v", rep)
	}
}

// export walks the documented start offset, and a resource with no offset never
// repeats a page or exits clean on a full one.
func TestClassExportWalksEveryRow(t *testing.T) {
	serve := func(total int, offsets bool, reqs *int) func(http.ResponseWriter, *http.Request) {
		return func(w http.ResponseWriter, r *http.Request) {
			*reqs++
			start, _ := strconv.Atoi(r.URL.Query().Get("start"))
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			if limit == 0 {
				limit = 100
			}
			if !offsets {
				start = 0
			}
			rows := []string{}
			for i := start; i < total && i < start+limit; i++ {
				rows = append(rows, fmt.Sprintf(`{"domain":"d%d.example","device-provisioning-mac-address":"m%d"}`, i, i))
			}
			writeJSON(w, "["+strings.Join(rows, ",")+"]")
		}
	}
	lines := func(t *testing.T, file string) map[string]bool {
		t.Helper()
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		seen := map[string]bool{}
		for _, l := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			if seen[l] {
				t.Fatalf("a duplicate row: %s", l)
			}
			seen[l] = true
		}
		return seen
	}
	for name, args := range map[string][]string{"limit": {"--limit", "150"}, "every row": nil} {
		t.Run("domains "+name, func(t *testing.T) {
			testenv.Isolate(t)
			reqs := 0
			fakeNetsapiens(t, serve(150, true, &reqs))
			out := filepath.Join(t.TempDir(), "x.jsonl")
			mustRunFleet(t, append([]string{"export", "domains", "--format", "jsonl", "--no-cache", "-o", out}, args...)...)
			if got := lines(t, out); len(got) != 150 {
				t.Fatalf("want 150 distinct rows, got %d", len(got))
			}
		})
	}
	t.Run("phones has one page", func(t *testing.T) {
		testenv.Isolate(t)
		reqs := 0
		fakeNetsapiens(t, serve(40, false, &reqs))
		out := filepath.Join(t.TempDir(), "x.jsonl")
		mustRunFleet(t, "export", "phones", "--format", "jsonl", "--no-cache", "-o", out)
		if got := lines(t, out); len(got) != 40 || reqs != 1 {
			t.Fatalf("want 40 rows in 1 request, got %d in %d", len(got), reqs)
		}
	})
	t.Run("phones list --all asks once", func(t *testing.T) {
		testenv.Isolate(t)
		reqs := 0
		fakeNetsapiens(t, serve(40, false, &reqs))
		mustRunFleet(t, "phones", "list", "--all", "--json", "--no-cache", "--data-source", "live")
		if reqs != 1 {
			t.Fatalf("the same page was asked for %d times", reqs)
		}
	})
	t.Run("phones past the limit is not a clean exit", func(t *testing.T) {
		testenv.Isolate(t)
		reqs := 0
		fakeNetsapiens(t, serve(1500, false, &reqs))
		out := filepath.Join(t.TempDir(), "x.jsonl")
		_, _, err := runFleet(t, "export", "phones", "--format", "jsonl", "--no-cache", "-o", out)
		if err == nil || !strings.Contains(err.Error(), "incomplete") {
			t.Fatalf("want an incomplete error, got %v", err)
		}
		if reqs != 1 {
			t.Fatalf("the same page was asked for %d times", reqs)
		}
	})
}
