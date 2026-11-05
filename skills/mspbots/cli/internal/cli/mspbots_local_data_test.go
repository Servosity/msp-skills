// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"testing"
)

// mspbots handfix local-data-off-covers-snapshot: MSPBOTS_NO_LOCAL_DATA=1
// promises that nothing new is written to the store. `snapshot` exists to copy
// dataset rows into the store, so it refuses like the generated store writers;
// `--dry-run` previews still run, and the reads (`pull`, `trend`) stay
// available.
func TestNoLocalDataRefusesSnapshot(t *testing.T) {
	t.Setenv(noLocalDataEnvVar, "1")
	root := RootCmd()
	snap, _, err := root.Find([]string{"snapshot"})
	if err != nil || snap.Name() != "snapshot" {
		t.Fatalf("snapshot command not found: %v", err)
	}
	if err := applyLocalDataEnv(snap, &rootFlags{dataSource: "auto"}); err == nil || !strings.Contains(err.Error(), "snapshot is disabled") {
		t.Fatalf("snapshot ran with %s=1: err %v", noLocalDataEnvVar, err)
	}
	if err := applyLocalDataEnv(snap, &rootFlags{dataSource: "auto", dryRun: true}); err != nil {
		t.Fatalf("a --dry-run snapshot preview was refused: %v", err)
	}
	pull, _, _ := root.Find([]string{"pull"})
	if err := applyLocalDataEnv(pull, &rootFlags{dataSource: "auto"}); err != nil {
		t.Fatalf("pull was refused: %v", err)
	}
}
