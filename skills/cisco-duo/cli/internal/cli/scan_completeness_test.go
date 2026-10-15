// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// Codex merge review round 2 (P2): a scan limit below one ran no request at
// all and still exited 0 with an empty, apparently clean result. Both
// commands that take --max-scan-pages without validating it must refuse it as
// a usage error before building a client.
func TestMaxScanPagesBelowOneIsRefused(t *testing.T) {
	for name, build := range map[string]func(*rootFlags) *cobra.Command{
		"failures":         newNovelFailuresCmd,
		"enrollment chase": newNovelEnrollmentChaseCmd,
	} {
		for _, v := range []string{"0", "-1"} {
			t.Run(name+" "+v, func(t *testing.T) {
				cmd := build(&rootFlags{asJSON: true})
				cmd.SetOut(io.Discard)
				cmd.SetErr(io.Discard)
				cmd.SetArgs([]string{"--max-scan-pages=" + v})
				err := cmd.Execute()
				if err == nil {
					t.Fatalf("--max-scan-pages=%s was accepted; a scan that fetches nothing must not succeed", v)
				}
				if code := ExitCode(err); code != 2 {
					t.Fatalf("exit code %d, want 2 (usage error): %v", code, err)
				}
				if !strings.Contains(err.Error(), "--max-scan-pages") {
					t.Fatalf("error %q does not name --max-scan-pages", err)
				}
			})
		}
	}
}

// Codex merge review round 2 (P2), and its siblings: a row's completeness or
// risk marker exists only on the rows it describes (fleet risk undecoded_rows,
// fleet summary partial, failures flag), so the --agent/--compact 80% rule
// dropped it and the agent read a partial or flagged row as clean.
func TestAgentOutputKeepsSparseRowMarkers(t *testing.T) {
	cases := map[string]struct {
		view any
		want string
	}{
		"fleet risk undecoded_rows": {
			view: map[string]any{"accounts": 5, "rows": []fleetRiskRow{
				{AccountID: "DA1", Name: "a"}, {AccountID: "DA2", Name: "b"}, {AccountID: "DA3", Name: "c"},
				{AccountID: "DA4", Name: "d"}, {AccountID: "DA5", Name: "e", UndecodedRows: 7},
			}},
			want: `"undecoded_rows": 7`,
		},
		"fleet summary partial": {
			view: map[string]any{"accounts": 5, "rows": []fleetSummaryRow{
				{AccountID: "DA1", Name: "a"}, {AccountID: "DA2", Name: "b"}, {AccountID: "DA3", Name: "c"},
				{AccountID: "DA4", Name: "d"}, {AccountID: "DA5", Name: "e", Partial: []string{"user_limit: HTTP 500"}},
			}},
			want: "user_limit: HTTP 500",
		},
		"failures flag": {
			view: map[string]any{"rows": []failureRow{
				{User: "a", Reason: "deny_unenrolled_user", Count: 3}, {User: "b", Reason: "deny_unenrolled_user", Count: 2},
				{User: "c", Reason: "deny_unenrolled_user", Count: 2}, {User: "d", Reason: "deny_unenrolled_user", Count: 1},
				{User: "e", Reason: "user_marked_fraud", Count: 1, Flag: "user reported fraud"},
			}},
			want: "user reported fraud",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := printMarkedRowsJSON(&buf, tc.view, agentFlags()); err != nil {
				t.Fatalf("printMarkedRowsJSON: %v", err)
			}
			if !strings.Contains(buf.String(), tc.want) {
				t.Fatalf("--agent output dropped %s:\n%s", tc.want, buf.String())
			}
		})
	}
}
