// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Class time-filtered-walk-read-as-complete, Codex round 2: the sync command
// recorded a walk that is not the account's whole set, but the other walks
// (auto-refresh, workflow archive) went around that record. Every walk now
// passes one chokepoint, and auto-refresh is itself a whole walk.

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"betterstack-pp-cli/internal/cliutil/testenv"
	"betterstack-pp-cli/internal/store"
)

// The reviewer's input: a plain sync stores incident 7000001 open, Better
// Stack resolves it, and an hour later triage auto-refreshes. A refresh that
// sends from=<last sync> receives no rows for it (spec.yaml
// resources.incidents.endpoints.list.params[from]), saves a complete
// checkpoint, and triage kept the resolved incident open.
func TestAutoRefreshNeverKeepsAResolvedIncidentOpen(t *testing.T) {
	testenv.Isolate(t)
	now := time.Now()
	f := &bsR1Fake{}
	f.incidents = append(f.incidents, bsR1Incident("7000001", now.AddDate(0, 0, -28), false))
	f.serve(t)
	run := func(args ...string) (string, error) {
		cmd := RootCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		err := cmd.Execute()
		return out.String(), err
	}
	if out, err := run("sync", "--no-learn"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if out, err := run("triage", "--json", "--data-source", "local", "--no-learn"); err != nil || !strings.Contains(out, "7000001") {
		t.Fatalf("after a plain sync, triage must list the open incident: err %v, out %s", err, out)
	}
	f.mu.Lock()
	f.incidents[0] = bsR1Incident("7000001", now.AddDate(0, 0, -28), true)
	f.mu.Unlock()
	s, err := store.Open(defaultDBPath("betterstack-cli"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().Exec(`UPDATE sync_state SET last_synced_at = ?`, now.Add(-2*time.Hour).UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := run("triage", "--json")
	if err != nil {
		t.Fatalf("auto-refresh walked the whole account, so triage must answer: %v\n%s", err, out)
	}
	if strings.Contains(out, "7000001") {
		t.Errorf("the incident was resolved in Better Stack, and triage still lists it open after auto-refresh: %s", out)
	}
}

type bsWalkClient struct{}

func (bsWalkClient) Get(context.Context, string, map[string]string) (json.RawMessage, error) {
	return json.RawMessage(bsR1Page(nil, "")), nil
}
func (bsWalkClient) RateLimit() float64 { return 0 }

// Every walk goes through syncResource, whoever calls it, so its shape is
// decided there: only a full, reconciling, unparameterized walk is a snapshot,
// and only a finished one clears an earlier record.
func TestEveryWalkShapeIsRecordedWhereTheWalkRuns(t *testing.T) {
	testenv.Isolate(t)
	withParam, err := parseSyncUserParams([]string{"resolved=true"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	type shape struct {
		since      string
		full       bool
		maxPages   int
		latestOnly bool
		prune      bool
		params     *syncUserParams
	}
	whole := shape{full: true, prune: true}
	for name, walk := range map[string]shape{
		"auto-refresh as the press renders it": {maxPages: 1, latestOnly: true},
		"workflow archive":                     {full: true},
		"incremental":                          {prune: true},
		"time window":                          {since: "2026-10-01T00:00:00Z", full: true, prune: true},
		"latest only":                          {full: true, prune: true, latestOnly: true},
		"no prune":                             {full: true},
		"filter parameter":                     {full: true, prune: true, params: withParam},
	} {
		t.Run(name, func(t *testing.T) {
			s, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			walkIt := func(w shape) {
				t.Helper()
				if r := syncResource(context.Background(), bsWalkClient{}, s, "incidents", w.since, w.full, w.maxPages, w.latestOnly, w.prune, w.params, nil); r.Err != nil {
					t.Fatalf("walk: %v", r.Err)
				}
			}
			walkIt(whole)
			if err := requireFinishedSync(s, "incidents"); err != nil {
				t.Fatalf("a whole walk is a snapshot: %v", err)
			}
			walkIt(walk)
			if err := requireFinishedSync(s, "incidents"); err == nil {
				t.Error("this walk is not the account's whole set, and the novels would read it as complete")
			}
			walkIt(whole)
			if err := requireFinishedSync(s, "incidents"); err != nil {
				t.Errorf("a finished whole walk must clear the record: %v", err)
			}
		})
	}
}
