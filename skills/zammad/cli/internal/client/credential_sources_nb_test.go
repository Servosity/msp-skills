// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): next-batch Codex rounds 1 and 2 on the
// zammad 4.34.0 publish (2026-10-27/28). Pinned by this connector's
// handfixes.json under the "credkit-wiring" entry.

package client

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"zammad-pp-cli/internal/cliutil/testenv"
)

// TestNBBrokenConfigIsRefusedAtStartup: a line heuristic read files that do
// not parse and missed one shape per review (Codex nb-r1-1, nb-r2). The
// class: every configuration or credentials file the process can name that
// does not parse makes startup fail, for the MCP server (no arguments,
// ZAMMAD_CONFIG) as for the CLI, with an error that never quotes the file.
func TestNBBrokenConfigIsRefusedAtStartup(t *testing.T) {
	home := testenv.Isolate(t)
	token := "nbr-" + strings.Repeat("Rf3m", 12)
	path := filepath.Join(home, "broken.toml")
	if err := os.WriteFile(path, []byte("api_token = \"\"\"\n"+token+"\"\"\"\nbroken = [\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZAMMAD_CONFIG", path)
	err := HoldStartupCredentials(nil)
	var broken *BrokenCredentialFileError
	if !errors.As(err, &broken) || broken.Path != path || strings.Contains(err.Error(), token) {
		t.Fatalf("a broken ZAMMAD_CONFIG was not refused by path alone: %v", err)
	}
	if err := os.WriteFile(path, []byte("base_url = \"https://zammad.example.com/api/v1\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := HoldStartupCredentials(nil); err != nil {
		t.Fatalf("a valid ZAMMAD_CONFIG was refused: %v", err)
	}
}
