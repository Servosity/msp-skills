// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/servosity/handfixes.json under
// the "mcp-filesystem-path-flags" entry. See docs/reprint-survival.md.

package cobratree

import (
	"testing"

	"github.com/spf13/cobra"
)

// TestFilesystemPathFlagsAreRefusedAsMCPParameters guards the MCP filesystem
// gate. The MCP server runs the companion CLI as the server account, so a flag
// that names a local path lets a tool caller choose where that account reads or
// writes. Before this gate, --db (every local-store command), --out (qbr,
// qbr-all), --input (import), --notes-file / --playbook-file /
// --playbook-notes-file (teach) and --reconcile (bill) were all forwarded, and
// driving learnings_list with {"db": "<any path>"} created a SQLite file there.
func TestFilesystemPathFlagsAreRefusedAsMCPParameters(t *testing.T) {
	cases := []struct {
		name  string
		usage string
		want  bool
	}{
		// Real local-filesystem flags this CLI ships.
		{"db-cache", "Database path (default: standard cache location)", true},
		{"db-sqlite", "SQLite database file path (default: resolved data directory data.db)", true},
		{"out-dir", "Output directory (one file per company)", true},
		{"out-file", "Output file path (required for html/pdf; optional for md)", true},
		{"output", "Output file path (default: stdout)", true},
		{"input", "Input JSONL file path (use - for stdin)", true},
		{"notes-file", "Path to a markdown file with the notes", true},
		{"playbook-file", "Path to a JSON file with the playbook (steps, entity_slots)", true},
		{"playbook-notes-file", "Optional path to a markdown file with playbook notes", true},
		{"reconcile", "Path to invoicing CSV (columns: company_id,company_name,invoiced_amount)", true},
		{"audit-dir", "Aggregate the receipt and index under this audit directory", true},
		{"config", "Config file path", true},

		// Generated API body fields. Their values are sent to the vendor API and
		// never touch the server's filesystem, so a name-based rule that blocked
		// anything called *path* would break real API surfaces. These must stay
		// callable.
		{"path", "Path", false},
		{"seed-path", "Seed path", false},
		{"source-paths", "Source paths", false},
		{"exclude-paths", "Exclude paths", false},
		{"filename", "Filename", false},
		{"working-dir", "Working dir", false},
		{"mysqldump-path", "Mysqldump path", false},
		{"one-file-system", "One file system", false},
		{"delete-temp-file", "Delete temp file", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "probe", Run: func(*cobra.Command, []string) {}}
			cmd.Flags().String(tc.name, "", tc.usage)
			blocked := blockedStructuredArgsForCommand(cmd)
			if blocked[tc.name] != tc.want {
				t.Fatalf("blocked[%q] = %v, want %v (usage %q)", tc.name, blocked[tc.name], tc.want, tc.usage)
			}
			allowed := allowedStructuredArgsForCommand(cmd, blocked, nil, false)
			if allowed[tc.name] == tc.want {
				t.Fatalf("allowed[%q] = %v, want %v (a blocked flag must not stay in the tool schema)", tc.name, allowed[tc.name], !tc.want)
			}
		})
	}
}

// TestBlockedDestinationFlagsCoverTheNamedSinks pins the static floor. The
// usage-text rule above is the general gate, but these four names are load
// bearing enough that a reworded usage string must not silently reopen them.
func TestBlockedDestinationFlagsCoverTheNamedSinks(t *testing.T) {
	for _, name := range []string{"audit-dir", "db", "input", "out", "output", "receipt-file"} {
		if !blockedDestinationFlags[name] {
			t.Fatalf("blockedDestinationFlags is missing %q; an MCP caller could direct a filesystem read or write", name)
		}
	}
}

// TestFilesystemPathFlagBeatsLocalOverride proves the rule wins over the
// command-local escape hatch. A command-local flag normally overrides the root
// denylist (so a per-command --config-shaped flag stays usable), but a local
// flag that names a filesystem location is exactly what the gate exists to
// refuse.
func TestFilesystemPathFlagBeatsLocalOverride(t *testing.T) {
	parent := &cobra.Command{Use: "root"}
	parent.PersistentFlags().String("config", "", "Config file path")
	child := &cobra.Command{Use: "child", Run: func(*cobra.Command, []string) {}}
	child.Flags().String("config", "", "Config file path")
	parent.AddCommand(child)

	if blocked := blockedStructuredArgsForCommand(child); !blocked["config"] {
		t.Fatal("a command-local flag whose usage names a file path must stay blocked")
	}
}
