// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// Class read-partition-is-the-sync-partition (runzero handfix, from the
// proofpoint review): a generated command whose pp:path is a sync path reads
// and writes the local store under that path's sync partition, not its group
// name. Press 4.34.2 keyed every `org` and `account` read on the group, so
// `org get-sites --data-source local` was served whatever another `org` read
// had stored last (agents, services, metrics). tools/apply_edits.py re-points
// them; this walks every generated command file against syncResourcePath.
func TestEveryReadUsesItsOwnSyncPartition(t *testing.T) {
	byPath := map[string][]string{}
	for _, name := range knownSyncResourceNames() {
		if p, err := syncResourcePath(name); err == nil {
			byPath[p] = append(byPath[p], name)
		}
	}
	pathRe := regexp.MustCompile(`"pp:path": "([^"]+)"`)
	keyRe := regexp.MustCompile(`Strategy[A-Za-z]*\(cmd\.Context\(\), c, flags, "[a-z0-9-]+", "([a-z0-9-]+)"`)
	files, _ := filepath.Glob("*.go")
	checked := 0
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		mp, mk := pathRe.FindSubmatch(b), keyRe.FindSubmatch(b)
		if mp == nil || mk == nil || len(byPath[string(mp[1])]) != 1 {
			continue
		}
		checked++
		if want := byPath[string(mp[1])][0]; string(mk[1]) != want {
			t.Errorf("%s (%s) reads the local store as %q, want its sync partition %q", f, mp[1], mk[1], want)
		}
	}
	if checked < 20 {
		t.Fatalf("only %d generated reads checked; the walk is broken", checked)
	}
}
