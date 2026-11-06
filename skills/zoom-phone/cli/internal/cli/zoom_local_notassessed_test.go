// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// An unassessed local audit says so on stdout and in the JSON note (polish,
// output review): zero counts from a partial store must not read as clean.

package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestLocalUnassessedAuditsSayNotAssessed(t *testing.T) {
	cases := []struct {
		name      string
		newCmd    func(*rootFlags) *cobra.Command
		args      []string
		rows      map[string][]string
		assessed  bool
		wantInMsg string
	}{
		{"footprint with only users synced", newNovelFootprintCmd, []string{"jane@example.com", "--no-live"},
			map[string][]string{"users": localTestUsers}, false, "; run: zoom-phone-cli sync --full --resources "},
		{"footprint fully synced", newNovelFootprintCmd, []string{"jane@example.com", "--no-live"},
			localTestFootprintRows(), true, ""},
		{"inventory without users", newNovelNumbersInventoryCmd, nil,
			map[string][]string{"numbers": localTestNumbers}, false, "for users; run: zoom-phone-cli sync --full --resources users"},
		{"inventory fully synced", newNovelNumbersInventoryCmd, nil,
			map[string][]string{"numbers": localTestNumbers, "users": localTestUsers}, true, ""},
		{"devices health without unassigned and common areas", newNovelDevicesHealthCmd, nil,
			map[string][]string{"assigned": localTestAssignedDevices, "users": localTestUsers}, false, "for common_areas, unassigned; run:"},
		{"devices health fully synced", newNovelDevicesHealthCmd, nil,
			map[string][]string{"assigned": localTestAssignedDevices, "unassigned": localTestUnassignedDevices, "users": localTestUsers, "common_areas": {}}, true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := localTestStore(t, tc.rows)

			// JSON: assessed and note agree.
			args := append(append([]string{}, tc.args...), "--db", db)
			stdout, _, err := localRun(t, tc.newCmd, args...)
			if err != nil {
				t.Fatalf("json run: %v", err)
			}
			var res struct {
				Assessed bool   `json:"assessed"`
				Note     string `json:"note"`
			}
			localDecode(t, stdout, &res)
			if res.Assessed != tc.assessed {
				t.Fatalf("assessed = %v, want %v", res.Assessed, tc.assessed)
			}
			if tc.assessed {
				if strings.Contains(res.Note, "not assessed") {
					t.Fatalf("assessed result carries a not-assessed note: %q", res.Note)
				}
			} else if !strings.HasPrefix(res.Note, "not assessed: ") || !strings.Contains(res.Note, tc.wantInMsg) {
				t.Fatalf("note = %q, want a not-assessed note containing %q", res.Note, tc.wantInMsg)
			}

			// Human table: the verdict leads stdout.
			prev := humanFriendly
			humanFriendly = true
			t.Cleanup(func() { humanFriendly = prev })
			human, _, err := localRunWith(t, &rootFlags{}, tc.newCmd, args...)
			if err != nil {
				t.Fatalf("human run: %v", err)
			}
			first := strings.SplitN(human, "\n", 2)[0]
			if tc.assessed {
				if strings.Contains(human, "NOT ASSESSED") {
					t.Fatalf("assessed table says NOT ASSESSED:\n%s", human)
				}
				return
			}
			if !strings.HasPrefix(first, "NOT ASSESSED: the local store does not cover the account") || !strings.Contains(first, tc.wantInMsg) {
				t.Fatalf("first stdout line = %q, want the NOT ASSESSED verdict containing %q\n%s", first, tc.wantInMsg, human)
			}
			if strings.Count(human, "does not cover the account") != 1 {
				t.Fatalf("verdict printed more than once:\n%s", human)
			}
		})
	}
}

// A footprint with no matched user still leads with the verdict when the
// store does not cover the account.
func TestLocalFootprintUnmatchedSaysNotAssessed(t *testing.T) {
	cases := []struct {
		name  string
		db    func(t *testing.T) string
		query string
		want  string
	}{
		{"no users synced, partial store", func(t *testing.T) string {
			return localTestStore(t, map[string][]string{"numbers": localTestNumbers})
		}, "jane@example.com", "NOT ASSESSED: the local store does not cover the account for "},
		{"empty store file", func(t *testing.T) string {
			return localTestStore(t, map[string][]string{})
		}, "jane@example.com", "NOT ASSESSED: the local store does not cover the account for "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := tc.db(t)
			stdout, _, err := localRun(t, newNovelFootprintCmd, tc.query, "--no-live", "--db", db)
			if err != nil {
				t.Fatalf("json run: %v", err)
			}
			var res struct {
				Assessed bool   `json:"assessed"`
				Note     string `json:"note"`
			}
			localDecode(t, stdout, &res)
			if res.Assessed || !strings.HasPrefix(res.Note, "not assessed: ") {
				t.Fatalf("assessed=%v note=%q, want an unassessed result with a not-assessed note", res.Assessed, res.Note)
			}
			prev := humanFriendly
			humanFriendly = true
			t.Cleanup(func() { humanFriendly = prev })
			human, _, err := localRunWith(t, &rootFlags{}, newNovelFootprintCmd, tc.query, "--no-live", "--db", db)
			if err != nil {
				t.Fatalf("human run: %v", err)
			}
			if first := strings.SplitN(human, "\n", 2)[0]; !strings.HasPrefix(first, tc.want) {
				t.Fatalf("first stdout line = %q, want prefix %q\n%s", first, tc.want, human)
			}
			if strings.Count(human, "does not cover the account") != 1 {
				t.Fatalf("verdict printed more than once:\n%s", human)
			}
		})
	}

	// No store at all: the verdict still leads the human table.
	// The press sandbox: every config, data, state and cache dir (and the
	// Windows profile) points into the test temp dir.
	localTestEnv(t)
	prev := humanFriendly
	humanFriendly = true
	t.Cleanup(func() { humanFriendly = prev })
	human, _, err := localRunWith(t, &rootFlags{}, newNovelFootprintCmd, "jane@example.com", "--no-live")
	if err != nil {
		t.Fatalf("no-store run: %v", err)
	}
	if first := strings.SplitN(human, "\n", 2)[0]; !strings.HasPrefix(first, "NOT ASSESSED: no local store") {
		t.Fatalf("no store: first stdout line = %q\n%s", first, human)
	}
}
