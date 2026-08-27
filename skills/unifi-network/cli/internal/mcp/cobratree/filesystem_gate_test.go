// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/unifi-network/handfixes.json under the
// "mcp-filesystem-destination-gate" entry.

package cobratree

import (
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

// forwardedCLIArgs runs the MCP-argument-to-CLI-argument translation the tool
// handler uses, with nothing pre-blocked by the caller, so the assertions below
// measure this file's gate and not a per-command denylist computed elsewhere.
func forwardedCLIArgs(args map[string]any) []string {
	return cliArgsFromMCP(args, map[string]bool{})
}

func hasFlagPair(argv []string, flag, value string) bool {
	for i := 0; i+1 < len(argv); i++ {
		if argv[i] == flag && argv[i+1] == value {
			return true
		}
	}
	return false
}

// TestMCPCannotDirectTheLocalStore is the regression test for the filesystem
// gate. The local-store commands expose --db and the shell-out layer forwards
// command-local flags straight through as CLI arguments, so a tool call
// carrying {"db": "..."} pointed the store at an arbitrary SQLite file, whose
// migration then ran `DROP TABLE IF EXISTS resources_fts` and rebuilt the
// resources table inside it. Both directions are asserted: --db must not
// survive, and an ordinary flag must, because a gate that swallows the real
// surface is as harmful as one that misses the defect.
func TestMCPCannotDirectTheLocalStore(t *testing.T) {
	got := forwardedCLIArgs(map[string]any{
		"db":     "/tmp/someone-elses.sqlite",
		"tenant": "contoso.example.com",
	})
	joined := strings.Join(got, " ")
	if strings.Contains(joined, "--db") || strings.Contains(joined, "someone-elses.sqlite") {
		t.Fatalf("an MCP-supplied --db reached the CLI: %v", got)
	}
	if !hasFlagPair(got, "--tenant", "contoso.example.com") {
		t.Fatalf("an ordinary flag was dropped; the gate must not break the MCP surface: %v", got)
	}
}

// TestMCPCannotSmuggleAFlagThroughAnEqualsSign closes the bypass a name-only
// denylist leaves open: an argument named "db=/tmp/evil" is emitted as
// "--db=/tmp/evil", which pflag parses as --db even though the denylist only
// ever sees the key "db=/tmp/evil".
func TestMCPCannotSmuggleAFlagThroughAnEqualsSign(t *testing.T) {
	got := forwardedCLIArgs(map[string]any{"db=/tmp/evil": "ignored"})
	if len(got) != 0 {
		t.Fatalf("an argument name containing = was forwarded: %v", got)
	}
}

// TestBlockedDestinationFlagsCoverTheNamedSinks pins the floor. The usage-text
// rule is the general gate, but these names are load bearing enough that a
// reworded usage string must not silently reopen them.
func TestBlockedDestinationFlagsCoverTheNamedSinks(t *testing.T) {
	for _, name := range []string{"audit-dir", "db", "home", "input", "o", "out", "output", "receipt-file"} {
		if !blockedDestinationFlags[name] {
			t.Fatalf("blockedDestinationFlags is missing %q; an MCP caller could direct a filesystem read or write", name)
		}
	}
}

// TestIsFilesystemPathFlagReadsUsageNotName pins the reason the rule matches on
// usage text. The generated command surface carries API body fields called
// --path, --seed-path and --source-paths whose values go to the vendor API and
// never touch this account's filesystem; a name-based rule would refuse those
// real surfaces and still miss the next generated flag that does take a local
// path.
func TestIsFilesystemPathFlagReadsUsageNotName(t *testing.T) {
	cases := []struct {
		name  string
		usage string
		want  bool
	}{
		{"db", "Database path (default: ~/.local/share/cli/data.db)", true},
		{"input", "Input JSONL file path (use - for stdin)", true},
		{"output", "Output file path (default: stdout)", true},
		{"audit-dir", "Aggregate the receipt and index under this audit directory", true},
		{"notes-file", "Path to a markdown file with the notes", true},
		{"out-dir", "Output directory (one file per company)", true},
		{"local-store", "Local SQLite store path (default: standard data dir)", true},
		{"mirror", "Local SQLite mirror path (default: standard cache location)", true},
		{"receipt-file", "Override the run receipt destination", true},
		{"home", "Root directory for config, data, state, and cache files", true},

		{"path", "Path", false},
		{"seed-path", "Seed path", false},
		{"source-paths", "Source paths", false},
		{"exclude-paths", "Exclude paths", false},
		{"filename", "Filename", false},
		{"working-dir", "Working dir", false},
		{"full-path", "Full path", false},
		{"endpoint", "Endpoint", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			set := pflag.NewFlagSet("probe", pflag.ContinueOnError)
			set.String(tc.name, "", tc.usage)
			if got := isFilesystemPathFlag(set.Lookup(tc.name)); got != tc.want {
				t.Fatalf("isFilesystemPathFlag(%q) = %v, want %v (usage %q)", tc.name, got, tc.want, tc.usage)
			}
		})
	}
}
