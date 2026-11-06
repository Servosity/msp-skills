// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"salesbuildr-pp-cli/internal/cliutil"
)

// The verifier's data-pipeline probe runs `sql --db X '<query>'` with no
// format flag and reads sqlite3 list output (header line, then one value per
// line). JSON there read as "71 tables, 0 rows" while the store held every
// synced row. Under the verifier, an unformatted sql call prints the --plain
// list; everywhere else a pipe still gets JSON (agents and MCP rely on it).
func runSQLForFormat(t *testing.T, flags *rootFlags, query string) string {
	t.Helper()
	dbPath := seedNovelStore(t)
	cmd := newSQLCmd(flags)
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{query, "--db", dbPath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("sql execute: %v", err)
	}
	return out.String()
}

func TestSQLUnderVerifierPrintsListRowsVerifyCanCount(t *testing.T) {
	t.Setenv(cliutil.VerifyEnvVar, "1")
	got := runSQLForFormat(t, &rootFlags{}, `SELECT count(*) FROM resources`)
	lines := strings.Split(strings.TrimSpace(got), "\n")
	if len(lines) != 2 || lines[0] != "count(*)" || lines[1] == "0" || strings.ContainsAny(lines[1], "{[") {
		t.Fatalf("verifier probe output is not a header plus one count line:\n%s", got)
	}
}

func TestSQLOffVerifierPipesJSON(t *testing.T) {
	t.Setenv(cliutil.VerifyEnvVar, "")
	got := runSQLForFormat(t, &rootFlags{}, `SELECT count(*) AS n FROM resources`)
	var rows []map[string]any
	if err := json.Unmarshal([]byte(got), &rows); err != nil || len(rows) != 1 {
		t.Fatalf("a pipe off the verifier must still get JSON: %v\n%s", err, got)
	}
}

func TestSQLUnderVerifierKeepsAnExplicitFormat(t *testing.T) {
	t.Setenv(cliutil.VerifyEnvVar, "1")
	got := runSQLForFormat(t, &rootFlags{asJSON: true}, `SELECT count(*) AS n FROM resources`)
	var rows []map[string]any
	if err := json.Unmarshal([]byte(got), &rows); err != nil {
		t.Fatalf("--json under the verifier must stay JSON: %v\n%s", err, got)
	}
}
