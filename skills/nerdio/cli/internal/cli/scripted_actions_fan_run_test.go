// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored tests for the scripted-actions fan-run novel feature.

package cli

import "testing"

func TestParseAccountsCSV(t *testing.T) {
	cases := []struct {
		in        string
		wantIDs   []int64
		expectErr bool
	}{
		{"101,102,103", []int64{101, 102, 103}, false},
		{" 101 , 102 ", []int64{101, 102}, false},
		{"101,,102", []int64{101, 102}, false},
		{"", nil, false},
		{"   ", nil, false},
		{"101,abc", nil, true},
		{"contoso", nil, true},
	}
	for _, tc := range cases {
		got, err := parseAccountsCSV(tc.in)
		if tc.expectErr {
			if err == nil {
				t.Errorf("parseAccountsCSV(%q) expected error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseAccountsCSV(%q) unexpected error: %v", tc.in, err)
			continue
		}
		if len(got) != len(tc.wantIDs) {
			t.Errorf("parseAccountsCSV(%q) = %d accounts, want %d", tc.in, len(got), len(tc.wantIDs))
			continue
		}
		for i, want := range tc.wantIDs {
			if got[i].ID != want {
				t.Errorf("parseAccountsCSV(%q)[%d] = %d, want %d", tc.in, i, got[i].ID, want)
			}
		}
	}
}

func TestFanRunIsNotReadOnly(t *testing.T) {
	cmd := newNovelScriptedActionsFanRunCmd(&rootFlags{})
	if cmd.Annotations["mcp:read-only"] == "true" {
		t.Error("fan-run executes scripted actions and must NOT be annotated read-only")
	}
	for _, flag := range []string{"accounts", "wait", "interval", "timeout", "params", "minutes-to-wait"} {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("missing --%s flag", flag)
		}
	}
}

func TestCountJobFailures(t *testing.T) {
	yes, no := true, false
	cases := []struct {
		name    string
		results []fanRunResult
		want    int
	}{
		{"no wait outcomes", []fanRunResult{{Status: "dispatched"}, {Status: "dispatched"}}, 0},
		{"all succeeded", []fanRunResult{{Succeeded: &yes}, {Succeeded: &yes}}, 0},
		{"one failed terminal", []fanRunResult{{Succeeded: &yes}, {Succeeded: &no}}, 1},
		{"all failed terminal", []fanRunResult{{Succeeded: &no}, {Succeeded: &no}, {Succeeded: &no}}, 3},
		{"mixed nil and failed", []fanRunResult{{}, {Succeeded: &no}}, 1},
	}
	for _, tc := range cases {
		if got := countJobFailures(tc.results); got != tc.want {
			t.Errorf("%s: countJobFailures = %d, want %d", tc.name, got, tc.want)
		}
	}
}
