// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (NOT generated): Codex round-1 classes for the local analytics.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"xero-pp-cli/internal/cliutil/testenv"
	"xero-pp-cli/internal/store"
)

func runAnalytics(t *testing.T, args ...string) []byte {
	t.Helper()
	cmd := RootCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("%v: %v\nstderr: %s", args, err, errOut.String())
	}
	return out.Bytes()
}

var analyticsArgv = [][]string{
	{"aging"}, {"reconcile"}, {"bank-recon"}, {"tie-out"}, {"exposure"},
	{"snapshot"}, {"ledger", "200"}, {"since", "7d"},
}

// Every analytics command is an MCP read-only tool, so the confirm gate never
// sees it. A read must not create the store (or its directory) as a side
// effect: on a machine that never synced, it answers empty and writes nothing.
func TestAnalyticsNeverCreateTheStore(t *testing.T) {
	testenv.Isolate(t)
	absent := filepath.Join(t.TempDir(), "nested", "absent.db")
	def := defaultDBPath("xero-cli")
	for _, argv := range analyticsArgv {
		for _, db := range []string{absent, ""} {
			args := append(append([]string{}, argv...), "--json")
			if db != "" {
				args = append(args, "--db", db)
			}
			out := runAnalytics(t, args...)
			if !json.Valid(out) {
				t.Errorf("%v: output is not JSON: %s", args, out)
			}
			for _, p := range []string{absent, filepath.Dir(absent), def} {
				if _, err := os.Stat(p); err == nil {
					t.Fatalf("%v created %s", args, p)
				}
			}
		}
	}
}

// Journal has no UpdatedDateUTC: GET /Journals dates each journal with
// CreatedDateUTC. A record whose timestamp cannot be read is reported as
// undated, never silently counted as unchanged.
func TestSinceDatesJournalsAndReportsUndated(t *testing.T) {
	testenv.Isolate(t)
	db := filepath.Join(t.TempDir(), "data.db")
	s, err := store.OpenWithContext(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range []struct{ rt, id, data string }{
		{"journals", "j1", `{"JournalID":"j1","JournalNumber":1,"CreatedDateUTC":"/Date(1541176243467+0000)/","JournalLines":[]}`},
		{"accounts", "a1", `{"AccountID":"a1","Code":"200"}`},
	} {
		if err := s.Upsert(r.rt, r.id, json.RawMessage(r.data)); err != nil {
			t.Fatal(err)
		}
	}
	_ = s.Close()

	var rep struct {
		Resources []map[string]any `json:"resources"`
	}
	if err := json.Unmarshal(runAnalytics(t, "since", "2018-11-01", "--db", db, "--json"), &rep); err != nil {
		t.Fatal(err)
	}
	got := map[string]map[string]any{}
	for _, r := range rep.Resources {
		got[r["resource_type"].(string)] = r
	}
	if n, _ := got["journals"]["changed_count"].(float64); n != 1 {
		t.Errorf("journal created 2018-11-02 not reported as changed since 2018-11-01: %v", got["journals"])
	}
	if n, _ := got["accounts"]["undated_count"].(float64); n != 1 {
		t.Errorf("account with no timestamp not reported as undated: %v", got["accounts"])
	}
}
