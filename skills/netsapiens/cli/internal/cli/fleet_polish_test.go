// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"netsapiens-pp-cli/internal/cliutil/testenv"
	"netsapiens-pp-cli/internal/store"
)

// runFleetCmd runs the root command and returns stdout.
func runFleetCmd(t *testing.T, args ...string) []byte {
	t.Helper()
	cmd := RootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("%v: %v\n%s", args, err, out.String())
	}
	return out.Bytes()
}

func agentResults(t *testing.T, out []byte, v any) {
	t.Helper()
	var env struct {
		Results json.RawMessage `json:"results"`
	}
	if err := json.Unmarshal(out, &env); err != nil || len(env.Results) == 0 {
		t.Fatalf("not an --agent envelope: %v\n%s", err, out)
	}
	if err := json.Unmarshal(env.Results, v); err != nil {
		t.Fatalf("decode results: %v\n%s", err, out)
	}
}

func TestFleetChangesNoMirrorIsFirstRun(t *testing.T) {
	testenv.Isolate(t)
	path := filepath.Join(t.TempDir(), "absent.db")
	var rep changesReport
	if err := json.Unmarshal(runFleetCmd(t, "fleet", "changes", "--db", path, "--json"), &rep); err != nil {
		t.Fatal(err)
	}
	if !rep.FirstRun || rep.Note == "" {
		t.Fatalf("no mirror means no baseline; stdout must say so, not read as zero changes: %+v", rep)
	}
}

// --agent implies --compact, which drops keys present on few rows. The from/to
// diff exists only on "changed" rows, so it must survive a removal-heavy report.
func TestFleetChangesAgentKeepsDiff(t *testing.T) {
	testenv.Isolate(t)
	path := filepath.Join(t.TempDir(), "data.db")
	seedMirror(t, path, map[string]bool{"users": true, "phonenumbers": true}, map[string][]string{
		"users": {
			`{"domain":"acme.example","user":"100"}`, `{"domain":"acme.example","user":"101"}`,
			`{"domain":"acme.example","user":"102"}`, `{"domain":"acme.example","user":"103"}`},
		"phonenumbers": {`{"domain":"acme.example","phonenumber":"16025550100","dial-rule-translation-destination-user":"100"}`}})
	runFleetCmd(t, "fleet", "changes", "--db", path, "--rebaseline", "--json")
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`DELETE FROM resources WHERE resource_type='users' AND id IN ('users-1','users-2','users-3')`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	seedMirror(t, path, map[string]bool{"phonenumbers": true}, map[string][]string{
		"phonenumbers": {`{"domain":"acme.example","phonenumber":"16025550100","dial-rule-translation-destination-user":"700"}`}})

	var rep changesReport
	agentResults(t, runFleetCmd(t, "fleet", "changes", "--db", path, "--agent"), &rep)
	if rep.Removed != 3 || rep.Changed != 1 {
		t.Fatalf("want 3 removed + 1 changed: %+v", rep)
	}
	for _, c := range rep.Changes {
		if c.Change == "changed" && c.Fields["destination_user"].To != "700" {
			t.Fatalf("--agent dropped the from/to diff on the changed row: %+v", c)
		}
	}
}

func TestFleetOfflineAgentKeepsSparseEvidence(t *testing.T) {
	testenv.Isolate(t)
	path := filepath.Join(t.TempDir(), "data.db")
	seedMirror(t, path, map[string]bool{"devices": true, "users": true}, map[string][]string{"devices": {
		`{"domain":"a.example","device":"100a","device-sip-registration-state":"unregistered"}`,
		`{"domain":"a.example","device":"101a","device-sip-registration-state":"unregistered"}`,
		`{"domain":"a.example","device":"102a","device-sip-registration-state":"unregistered"}`,
		`{"domain":"a.example","device":"103a","device-sip-registration-state":"unregistered"}`,
		`{"domain":"a.example","device":"104a","device-sip-registration-state":"registered","device-sip-registration-expires-datetime":"2099-01-01T00:00:00Z","device-sip-latency-seconds-average":0.9}`}})
	var rep struct {
		Complete *bool            `json:"complete"`
		Issues   []map[string]any `json:"issues"`
	}
	agentResults(t, runFleetCmd(t, "fleet", "offline", "--db", path, "--agent"), &rep)
	if rep.Complete == nil || *rep.Complete { // no NDP table synced: never complete, and --agent must keep saying so
		t.Fatalf("--agent must keep complete=false: %+v", rep)
	}
	issues := rep.Issues
	found := false
	for _, i := range issues {
		if i["id"] == "104a" {
			found = true
			if _, ok := i["latency_avg_seconds"]; !ok {
				t.Fatalf("--agent dropped the latency behind high-latency: %v", i)
			}
		}
	}
	if !found {
		t.Fatalf("high-latency device not reported: %v", issues)
	}
}

func TestFleetChangesNotesNameTheCause(t *testing.T) {
	testenv.Isolate(t)
	path := filepath.Join(t.TempDir(), "data.db")
	// first baseline while users is complete and devices is not
	seedMirror(t, path, map[string]bool{"users": true, "devices": false}, map[string][]string{
		"users":   {`{"domain":"acme.example","user":"100"}`},
		"devices": {`{"domain":"acme.example","device":"100a","user":"100"}`}})
	var first changesReport
	if err := json.Unmarshal(runFleetCmd(t, "fleet", "changes", "--db", path, "--rebaseline", "--json"), &first); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(first.Note, "recorded with 1 entities") || !strings.Contains(first.Note, "device") {
		t.Fatalf("first-baseline note must count stored entities and name the missing ones: %q", first.Note)
	}
	// devices now complete: skipped because the baseline lacks it, not because of the sync
	seedMirror(t, path, map[string]bool{"devices": true}, nil)
	var next changesReport
	if err := json.Unmarshal(runFleetCmd(t, "fleet", "changes", "--db", path, "--json"), &next); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(next.Note, "not in the baseline") || !strings.Contains(next.Note, "device") {
		t.Fatalf("skip note must blame the baseline for devices: %q", next.Note)
	}
	if strings.Contains(next.Note, "sync incomplete or never run): device") {
		t.Fatalf("devices synced completely; the note must not blame the sync: %q", next.Note)
	}
}

func TestPrioritizeFieldsIsDeterministic(t *testing.T) {
	item := map[string]any{"domain": "a", "number": "1", "app": "x", "to": "y", "problems": "p", "target": "t"}
	want := prioritizeFields(item, false)
	for i := 0; i < 50; i++ {
		if got := prioritizeFields(item, false); !reflect.DeepEqual(got, want) {
			t.Fatalf("column order changed between calls: %v vs %v", got, want)
		}
	}
}
