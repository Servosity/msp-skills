// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ninjaone-pp-cli/internal/cliutil/testenv"
	"ninjaone-pp-cli/internal/store"
)

// Class endpoint-reads-use-the-sync-partition: endpoint commands are generated
// per command group ("queries" for all 24 /v2/queries/* reports, "organization"
// for /v2/organization/documents and /v2/organization/{id} alike), but each sync
// partition holds one endpoint's rows. A local read, an auto-mode write-through
// and a mutation write-through must use the partition that owns the endpoint,
// and never a partition that holds a different endpoint's rows.

type endpointFixture struct {
	srv    *httptest.Server
	dbPath string
}

func newEndpointFixture(t *testing.T, handler http.HandlerFunc) endpointFixture {
	t.Helper()
	testenv.Isolate(t)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	cfg := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfg, []byte("access_token = \"fixture-token\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NINJAONE_CONFIG", cfg)
	t.Setenv("NINJAONE_BASE_URL", srv.URL)
	t.Setenv("NINJAONE_CLIENT_ID", "")
	t.Setenv("NINJAONE_CLIENT_SECRET", "")
	dbPath := defaultDBPath("ninjaone-cli")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	seed := map[string][]string{
		"queries": {
			`{"deviceId":1,"name":"\\\\.\\PHYSICALDRIVE0","size":512}`,
			`{"deviceId":2,"name":"\\\\.\\PHYSICALDRIVE0","size":256}`,
		},
		"queries-antivirus-status": {
			`{"deviceId":1,"productName":"Defender","productState":"ON","definitionStatus":"UP_TO_DATE"}`,
		},
		"organization": {
			`{"documentId":5,"organizationId":3,"documentName":"Runbook"}`,
		},
	}
	for rt, rows := range seed {
		raw := make([]json.RawMessage, len(rows))
		for i, r := range rows {
			raw[i] = json.RawMessage(r)
		}
		if _, _, err := db.UpsertBatch(rt, raw); err != nil {
			t.Fatal(err)
		}
	}
	return endpointFixture{srv: srv, dbPath: dbPath}
}

func (f endpointFixture) run(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	root := newRootCmd(&rootFlags{})
	var out, errBuf bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errBuf)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), errBuf.String(), err
}

func (f endpointFixture) rows(t *testing.T, rt string) []string {
	t.Helper()
	db, err := store.OpenWithContext(context.Background(), f.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query(`SELECT data FROM resources WHERE resource_type = ?`, rt)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			t.Fatal(err)
		}
		out = append(out, d)
	}
	return out
}

func TestReportLocalReadUsesItsOwnPartition(t *testing.T) {
	f := newEndpointFixture(t, func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	out, stderr, err := f.run(t, "queries", "get-antivirus-status-report", "--data-source", "local", "--json")
	if err != nil {
		t.Fatalf("local read failed: %v\n%s", err, stderr)
	}
	if !strings.Contains(out, "Defender") || strings.Contains(out, "PHYSICALDRIVE") {
		t.Fatalf("antivirus-status local read returned the wrong partition:\n%s", out)
	}
}

func TestReportWriteThroughUsesItsOwnPartition(t *testing.T) {
	f := newEndpointFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v2/queries/os-patches":
			fmt.Fprint(w, `{"cursor":{"name":"C","offset":0,"count":1},"results":[{"deviceId":7,"id":"patch-1","name":"KB5034123","status":"PENDING"}]}`)
		case "/v2/queries/software":
			fmt.Fprint(w, `{"cursor":{"name":"C","offset":0,"count":1},"results":[{"deviceId":7,"name":"Acme Agent","version":"2","productCode":"B"}]}`)
		default:
			http.NotFound(w, r)
		}
	})
	if _, stderr, err := f.run(t, "queries", "get-pending-failed-rejected-ospatches", "--json"); err != nil {
		t.Fatalf("live read failed: %v\n%s", err, stderr)
	}
	if got := f.rows(t, "queries-os-patches"); len(got) != 1 {
		t.Fatalf("queries-os-patches holds %d rows after the live read, want 1: %v", len(got), got)
	}
	// Class report-row-identity: a report keyed by row ordinal is written only by sync; a
	// live read cannot know which page or filter its rows came from.
	if out, stderr, err := f.run(t, "queries", "get-software", "--json"); err != nil || !strings.Contains(out, "Acme Agent") {
		t.Fatalf("live software read failed: %v\n%s%s", err, out, stderr)
	}
	if got := f.rows(t, "queries-software"); len(got) != 0 {
		t.Fatalf("a live read wrote %d row(s) through into the ordinal queries-software partition: %v", len(got), got)
	}
	if got := f.rows(t, "queries"); len(got) != 2 {
		t.Fatalf("the disks partition changed after a report read: %v", got)
	}
}

func TestEndpointsAPartitionDoesNotOwnNeverReadOrWriteIt(t *testing.T) {
	f := newEndpointFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v2/organization/5":
			fmt.Fprint(w, `{"id":5,"name":"Acme","nodeApprovalMode":"AUTOMATIC"}`)
		case "/v2/organization/checklists":
			fmt.Fprint(w, `[{"id":9,"name":"Onboarding","organizationId":3}]`)
		default:
			http.NotFound(w, r)
		}
	})
	// /v2/organization/{id} is an organization, not a row of the documents partition.
	if out, _, err := f.run(t, "organization", "get", "5", "--data-source", "local", "--json"); err == nil {
		t.Fatalf("organization get read the documents partition locally: %s", out)
	}
	if _, stderr, err := f.run(t, "organization", "get", "5", "--json"); err != nil {
		t.Fatalf("live organization get failed: %v\n%s", err, stderr)
	}
	// /v2/organization/checklists rows belong to organization-checklists.
	if _, stderr, err := f.run(t, "organization", "get-client-checklists", "--json"); err != nil {
		t.Fatalf("live checklists read failed: %v\n%s", err, stderr)
	}
	if got := f.rows(t, "organization"); len(got) != 1 || !strings.Contains(got[0], "documentId") {
		t.Fatalf("documents partition took rows from other endpoints: %v", got)
	}
	if got := f.rows(t, "organization-checklists"); len(got) != 1 {
		t.Fatalf("organization-checklists holds %d rows after the checklists read, want 1", len(got))
	}
	// A mutation response cannot prove which list endpoint it belongs to: it never
	// lands in a sync partition. A non-sync partition keeps the generated behavior.
	writeMutationResponseToStore(context.Background(), "organization", json.RawMessage(`{"id":5,"name":"Acme"}`), "")
	if got := f.rows(t, "organization"); len(got) != 1 {
		t.Fatalf("a mutation response landed in the documents partition: %v", got)
	}
	writeMutationResponseToStore(context.Background(), "device", json.RawMessage(`{"id":11,"systemName":"PC-11"}`), "")
	if got := f.rows(t, "device"); len(got) != 1 {
		t.Fatalf("non-sync partition write-through changed: %v", got)
	}
}
