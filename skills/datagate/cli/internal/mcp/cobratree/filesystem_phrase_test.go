// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/datagate/handfixes.json under the
// "mcp-filesystem-flag-floor" entry.

package cobratree

import (
	"strconv"
	"testing"

	"github.com/spf13/cobra"
)

// TestEveryPathPhraseRefusesALocalFlagAtRuntime pins the usage-text rule to the
// runtime gate. Every flag datagate ships today that names a path is ALSO
// refused by name, so TestNoShellOutToolCanDirectAFilesystemPath stays green
// even if a reprint drops a phrase or unhooks isFilesystemPathFlag from
// blockedStructuredArgsForCommand. This test uses flag names no denylist
// carries, so only the phrase rule can refuse them. The phrase list is copied
// here on purpose: removing one from typemap.go must fail this test.
func TestEveryPathPhraseRefusesALocalFlagAtRuntime(t *testing.T) {
	phrases := []string{
		"file path", "path to ", "database path", "output directory",
		"audit directory", "directory path", "store path", "mirror path",
		"receipt destination", "root directory for",
	}
	for i, phrase := range phrases {
		name := "probe-" + strconv.Itoa(i)
		cmd := &cobra.Command{Use: "probe", Run: func(*cobra.Command, []string) {}}
		cmd.Flags().String(name, "", "Local "+phrase+" for this run")
		cmd.Flags().String("seed-path", "", "Seed path")

		blocked := blockedStructuredArgsForCommand(cmd)
		allowed := allowedStructuredArgsForCommand(cmd, blocked, nil, false)
		if !blocked[name] || allowed[name] {
			t.Errorf("usage phrase %q: --%s blocked=%v allowed=%v; want blocked and not allowed", phrase, name, blocked[name], allowed[name])
		}
		// The other direction: a vendor body field described by a bare noun
		// must still reach the CLI, or the gate breaks the real API surface.
		if blocked["seed-path"] || !allowed["seed-path"] {
			t.Errorf("vendor body field --seed-path was refused alongside phrase %q", phrase)
		}
	}
}
