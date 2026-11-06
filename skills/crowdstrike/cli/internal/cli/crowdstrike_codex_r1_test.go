// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Codex review round 1 classes (the appdirect 4.34.2 reprint, ported), re-
// targeted at the CrowdStrike fleet novels, the only commands here that read a
// store partition of their own (the CID-keyed fleet_entities table that
// `fleet sync` fills) or declare a data source:
//   - sync-read-partition-mismatch: a novel reads only the rows of the kind it
//     reports (a stale-host list never lists an alert, an alert queue never a
//     vulnerability).
//   - local-source-contract-unenforced: the fleet rollups declare
//     `pp:data-source local` and refuse --data-source live; fleet sync
//     declares live and refuses --data-source local.
//   - incomplete-paging-read-as-complete: whatever ends a fleet sync walk (a
//     server that caps the page size below fleet sync's, or answers the same
//     page whatever offset it is sent), a walk whose documented paging
//     metadata (meta.pagination.total) says more records exist never reads as
//     complete.
//   - credential-persisted-in-files, typed tables: the fleet_entities table
//     keeps no credential-named value (the generic store strips them in
//     store/crowdstrike_secrets.go; fleet_entities was written by its own SQL).
// Not carried (no CrowdStrike counterpart): appdirect's company show user join
// (a field UserAccountV2 does not have; spec.json declares no response
// schemas to join against) and the per-path partition function of its
// generated endpoint commands (appdirect_partitions.go was not ported; see the
// helper report).

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"crowdstrike-pp-cli/internal/cliutil/testenv"
)

// seedFleet writes ents into the fleet store at dbPath.
func seedFleet(t *testing.T, dbPath string, ents ...fleetEntity) {
	t.Helper()
	st, err := openFleetStore(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := upsertFleetEntities(context.Background(), st.DB(), ents); err != nil {
		t.Fatal(err)
	}
}

// runFleet runs the CLI with args and returns stdout, stderr and the error.
func runFleet(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := newRootCmd(&rootFlags{})
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errb.String(), err
}

// Each fleet view reads the rows of its own kind: one row of every kind, all
// old, open and critical, each named for its kind; a view printing the name
// of another kind read another partition.
func TestR1FleetViewsReadOnlyTheirKind(t *testing.T) {
	testenv.Isolate(t)
	db := filepath.Join(t.TempDir(), "fleet.db")
	old := time.Now().Add(-90 * 24 * time.Hour)
	seedFleet(t, db,
		fleetEntity{CID: "cid-a", Kind: kindHost, ID: "dev-1", Name: "KindHostRow", Status: "normal", LastSeen: old},
		fleetEntity{CID: "cid-a", Kind: kindAlert, ID: "ldt:1", Name: "KindAlertRow", Severity: "critical", Status: "new", LastSeen: old},
		fleetEntity{CID: "cid-a", Kind: kindVuln, ID: "vul-1", Name: "KindVulnRow", Severity: "critical", Status: "open", LastSeen: old},
		fleetEntity{CID: "cid-a", Kind: kindPolicy, ID: "pol-1", Name: "Windows:KindPolicyRow", Status: "true", LastSeen: old},
	)
	all := []string{"KindHostRow", "KindAlertRow", "KindVulnRow", "KindPolicyRow"}
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"fleet", "stale", "--days", "1"}, "KindHostRow"},
		{[]string{"fleet", "alerts"}, "KindAlertRow"},
		{[]string{"fleet", "vulns"}, "KindVulnRow"},
	} {
		out, errb, err := runFleet(t, append(c.args, "--db", db, "--json")...)
		if err != nil || !strings.Contains(out, c.want) {
			t.Fatalf("%s: want %s listed (err %v):\n%s%s", strings.Join(c.args, " "), c.want, err, out, errb)
		}
		for _, other := range all {
			if other != c.want && strings.Contains(out, other) {
				t.Errorf("%s printed %s: it read another kind's rows:\n%s", strings.Join(c.args, " "), other, out)
			}
		}
	}
}

// Codex r1 P2 local-source-contract-unenforced, on the fleet novels: every
// rollup reads only the local store and refuses --data-source live (it would
// otherwise answer as if live from SQLite); fleet sync reads only the API and
// refuses --data-source local.
func TestR1FleetCommandsKeepTheirDataSource(t *testing.T) {
	testenv.Isolate(t)
	db := filepath.Join(t.TempDir(), "fleet.db")
	seedFleet(t, db, fleetEntity{CID: "cid-a", Kind: kindHost, ID: "dev-1", Name: "SourceRow", LastSeen: time.Now()})
	for _, args := range [][]string{
		{"fleet", "scorecard"}, {"fleet", "vulns"}, {"fleet", "stale"}, {"fleet", "policy-drift"}, {"fleet", "alerts"},
		{"fleet", "search", "SourceRow"}, {"fleet", "tenants"}, {"fleet", "remediate"}, {"fleet", "trend"},
	} {
		if out, _, err := runFleet(t, append(args, "--db", db, "--data-source", "live", "--json")...); err == nil {
			t.Errorf("%s --data-source live ran on the local store: %q", strings.Join(args, " "), out)
		}
		if _, errb, err := runFleet(t, append(args, "--db", db, "--data-source", "local", "--json")...); err != nil {
			t.Errorf("%s --data-source local (control): %v\n%s", strings.Join(args, " "), err, errb)
		}
	}
	t.Setenv("FALCON_CLIENT_ID", "sourceClientId-1")
	t.Setenv("FALCON_CLIENT_SECRET", "sourceClientSecret-2")
	t.Setenv("CROWDSTRIKE_BASE_URL", "http://127.0.0.1:9")
	if out, _, err := runFleet(t, "fleet", "sync", "--cids", "cid-a", "--kinds", "hosts", "--db", db, "--data-source", "local", "--json"); err == nil {
		t.Errorf("fleet sync --data-source local ran: %q", out)
	}
}

// pagedFalcon serves the Falcon combined hosts list ({"meta":{"pagination":
// {"offset","limit","total"}},"resources":[...]}) for one child CID from a
// fixed set of hosts. maxLimit caps the page size the way a server does;
// ignoreOffset answers the first page for every offset.
type pagedFalcon struct {
	total, maxLimit int
	ignoreOffset    bool
	gets            atomic.Int32
}

func (f *pagedFalcon) server(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/oauth2/token" {
			raw, _ := io.ReadAll(r.Body)
			form, _ := url.ParseQuery(string(raw))
			_, _ = io.WriteString(w, `{"access_token":"paged-token-`+form.Get("member_cid")+`","token_type":"bearer","expires_in":1799}`)
			return
		}
		if f.gets.Add(1) > 50 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		q := r.URL.Query()
		limit, _ := strconv.Atoi(q.Get("limit"))
		if limit <= 0 {
			limit = 100
		}
		if f.maxLimit > 0 && limit > f.maxLimit {
			limit = f.maxLimit
		}
		offset, _ := strconv.Atoi(q.Get("offset"))
		if f.ignoreOffset {
			offset = 0
		}
		var items []string
		for i := offset; i < offset+limit && i < f.total; i++ {
			items = append(items, fmt.Sprintf(`{"device_id":"dev-%04d","hostname":"host-%d","status":"normal","last_seen":"2026-10-28T00:00:00Z"}`, i, i))
		}
		fmt.Fprintf(w, `{"meta":{"pagination":{"offset":%d,"limit":%d,"total":%d}},"resources":[%s],"errors":[]}`, offset, limit, f.total, strings.Join(items, ","))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// The class: whatever ends a fleet sync walk, a walk whose page metadata says
// more hosts exist than it stored never reads as complete. Control: a server
// that honors fleet sync's paging stores every host with no warning.
func TestR1FleetSyncNeverReadsAShortWalkAsComplete(t *testing.T) {
	for _, c := range []struct {
		name    string
		f       *pagedFalcon
		control bool
	}{
		{"server honors offset and limit", &pagedFalcon{total: 1203}, true},
		{"server caps the page size below fleet sync's", &pagedFalcon{total: 263, maxLimit: 100}, false},
		{"server ignores the offset", &pagedFalcon{total: 700, ignoreOffset: true}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			testenv.Isolate(t)
			srv := c.f.server(t)
			t.Setenv("CROWDSTRIKE_BASE_URL", srv.URL)
			t.Setenv("FALCON_CLIENT_ID", "pagingParentId-3c1a")
			t.Setenv("FALCON_CLIENT_SECRET", "pagingParentSecret-7e2b")
			t.Setenv("CROWDSTRIKE_NO_CONFIG_WRITE", "1")
			db := filepath.Join(t.TempDir(), "fleet.db")
			out, errb, err := runFleet(t, "fleet", "sync", "--cids", "cid-paged", "--kinds", "hosts", "--db", db, "--json")
			if err != nil {
				t.Fatalf("fleet sync: %v\n%s%s", err, out, errb)
			}
			ents, err := loadFleetEntities(context.Background(), db, kindHost)
			if err != nil {
				t.Fatal(err)
			}
			var summary fleetSyncSummary
			if err := json.Unmarshal([]byte(out), &summary); err != nil {
				t.Fatalf("fleet sync --json: %v\n%s", err, out)
			}
			if c.control {
				if len(ents) != c.f.total || len(summary.Warnings) != 0 {
					t.Fatalf("control: stored %d of %d hosts, warnings %v", len(ents), c.f.total, summary.Warnings)
				}
				return
			}
			if len(ents) < c.f.total && len(summary.Warnings) == 0 {
				t.Errorf("stored %d of %d hosts and reported the sync complete (no warning):\n%s", len(ents), c.f.total, out)
			}
		})
	}
}

// credential-persisted-in-files, the typed-table half (ported from the store
// package's typed account and assisted_sales tables, which CrowdStrike does
// not have): a fleet_entities row keeps no credential-named value the process
// never held, at any depth, and keeps the record.
func TestR1FleetStoreKeepsNoCredentialNamedValue(t *testing.T) {
	testenv.Isolate(t)
	const password, token, inArray = "unheldFleetPass-5e1c7a3f", "unheldFleetToken-2d9b4f6a", "unheldFleetSecret-6c1e8b3d"
	dir := t.TempDir()
	db := filepath.Join(dir, "fleet.db")
	raw := `{"device_id":"dev-77","hostname":"FleetKeptHost","settings":{"password":"` + password + `"},"secrets":["` + inArray + `"],"provision_token":"` + token + `"}`
	seedFleet(t, db, fleetEntity{CID: "cid-a", Kind: kindHost, ID: "dev-77", Name: "FleetKeptHost", Raw: json.RawMessage(raw)})
	var all []byte
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if b, err := os.ReadFile(filepath.Join(dir, e.Name())); err == nil {
			all = append(all, b...)
		}
	}
	for _, v := range []string{password, token, inArray} {
		if bytes.Contains(all, []byte(v)) {
			t.Errorf("the fleet store kept the credential-named value %q", v)
		}
	}
	if !bytes.Contains(all, []byte("FleetKeptHost")) || !bytes.Contains(all, []byte("dev-77")) {
		t.Errorf("the fleet record was not stored")
	}
}
