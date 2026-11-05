// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"betterstack-pp-cli/internal/cliutil/testenv"
	"betterstack-pp-cli/internal/store"
)

// Class incomplete-snapshot-read-as-complete (pre-closed, betterstack handfix
// novels-refuse-a-partial-store): every novel reads the store through a loader
// that calls requireFinishedSync. A sync that stopped part way, rows a live
// read wrote through with no sync at all, and a resource never synced while
// others were are a partial set; "no monitors down" or "every monitor
// covered" from them is a wrong answer an MSP acts on. Never synced and empty
// keeps the empty result with a sync hint.
func TestNovelsRefuseAPartialSnapshot(t *testing.T) {
	testenv.Isolate(t)
	monitor := []byte(`{"id":"2085431","type":"monitor","attributes":{"pronounceable_name":"API","url":"https://api.example.com","monitor_type":"status","status":"down","paused":false,"policy_id":null,"email":false,"sms":false,"call":false,"push":false}}`)
	run := func(db string, args ...string) (string, error) {
		cmd := RootCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(append(args, "--db", db, "--json", "--data-source", "local"))
		err := cmd.Execute()
		return out.String(), err
	}
	seed := func(t *testing.T, state func(*store.Store) error) string {
		t.Helper()
		db := filepath.Join(t.TempDir(), "data.db")
		s, err := store.Open(db)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Upsert("monitors", "2085431", monitor); err != nil {
			t.Fatal(err)
		}
		if state != nil {
			if err := state(s); err != nil {
				t.Fatal(err)
			}
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
		return db
	}
	novels := [][]string{{"fleet"}, {"coverage"}, {"down"}, {"group-health"}}
	for name, state := range map[string]func(*store.Store) error{
		"write-through rows, never synced": nil,
		"sync stopped part way":            func(s *store.Store) error { return s.SaveSyncProgress("monitors", "page=3", 1) },
		"other resources never synced":     func(s *store.Store) error { return s.SaveSyncState("monitors", "", 1) },
	} {
		db := seed(t, state)
		for _, args := range novels {
			if name == "other resources never synced" && args[0] == "coverage" {
				continue // coverage reads monitors only: a finished monitors sync is its whole input
			}
			out, err := run(db, args...)
			if err == nil || !strings.Contains(out+err.Error(), "sync") {
				t.Errorf("%s: %s answered from a partial store (err %v): %s", name, strings.Join(args, " "), err, out)
			}
		}
	}
	complete := seed(t, func(s *store.Store) error {
		for _, r := range defaultSyncResources() {
			n := 0
			if r == "monitors" {
				n = 1
			}
			if err := s.SaveSyncState(r, "", n); err != nil {
				return err
			}
		}
		return nil
	})
	if out, err := run(complete, "coverage"); err != nil || !strings.Contains(out, "2085431") {
		t.Errorf("a finished sync must answer: err %v, out %s", err, out)
	}
	empty := filepath.Join(t.TempDir(), "data.db")
	s, err := store.Open(empty)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	for _, args := range novels {
		if _, err := run(empty, args...); err != nil {
			t.Errorf("an empty, never-synced store keeps the empty result with a hint (%v), got %v", args, err)
		}
	}
}

// Class incomplete-snapshot-read-as-complete (filtered walk): a sync that sent
// a documented filter (GET /incidents resolved=true) finishes as a complete
// walk of the filtered set; reading it as the whole account reported no
// unacknowledged incidents. An unfiltered sync afterwards makes the store the
// whole account again.
func TestFilteredSyncIsNotASnapshot(t *testing.T) {
	testenv.Isolate(t)
	open := `{"id":"501","type":"incident","attributes":{"name":"API down","cause":"Timeout","started_at":"2026-10-28T10:00:00Z","acknowledged_at":null,"resolved_at":null}}`
	resolved := `{"id":"502","type":"incident","attributes":{"name":"Web down","cause":"500","started_at":"2026-10-28T09:00:00Z","acknowledged_at":"2026-10-28T09:01:00Z","resolved_at":"2026-10-28T09:05:00Z"}}`
	page := func(items ...string) string {
		return `{"data":[` + strings.Join(items, ",") + `],"pagination":{"first":null,"last":null,"prev":null,"next":null}}`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/incidents") && r.URL.Query().Get("resolved") == "true":
			fmt.Fprint(w, page(resolved))
		case strings.HasSuffix(r.URL.Path, "/incidents"):
			fmt.Fprint(w, page(open, resolved))
		default:
			fmt.Fprint(w, page())
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("BETTERSTACK_BASE_URL", srv.URL+"/api/v2")
	t.Setenv("BETTERSTACK_API_TOKEN", "snapshotUnitToken-4c1e7a")
	db := filepath.Join(t.TempDir(), "data.db")
	run := func(args ...string) (string, error) {
		cmd := RootCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(append(args, "--db", db))
		err := cmd.Execute()
		return out.String(), err
	}
	if out, err := run("sync"); err != nil {
		t.Fatalf("full sync: %v\n%s", err, out)
	}
	for _, filter := range [][]string{{"--param", "resolved=true"}, {"--resource-param", "incidents:resolved=true"}, {"--global-param", "resolved=true"}} {
		if out, err := run(append([]string{"sync", "--resources", "incidents"}, filter...)...); err != nil {
			t.Fatalf("filtered sync %v: %v\n%s", filter, err, out)
		}
		out, err := run("triage", "--json", "--data-source", "local")
		if err == nil || !strings.Contains(out+err.Error(), "sync") {
			t.Errorf("after a sync filtered by %v, triage answered from the filtered set (err %v): %s", filter, err, out)
		}
	}
	if out, err := run("sync", "--resources", "incidents"); err != nil {
		t.Fatalf("unfiltered sync: %v\n%s", err, out)
	}
	out, err := run("triage", "--json", "--data-source", "local")
	if err != nil || !strings.Contains(out, "API down") {
		t.Errorf("after an unfiltered sync, triage must answer with the open incident: err %v, out %s", err, out)
	}
	var probe any
	_ = json.Unmarshal([]byte(out), &probe)
}
