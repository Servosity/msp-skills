// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// TestRosterRowsAreReadOnlyThroughTheMergedLoader: a computer the roster
// resources (computers, computers-paged, computers-dx) describe is read only
// through immyLoadComputers, which merges one machine's rows into one record.
// A command that selects a roster resource row by row could key the same
// machine twice and let one source overwrite the other.
func TestRosterRowsAreReadOnlyThroughTheMergedLoader(t *testing.T) {
	call := regexp.MustCompile(`immyComputerSelect\(([^)]*)\)`)
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || f == "immybot_computer_row.go" {
			continue
		}
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range call.FindAllStringSubmatch(string(raw), -1) {
			arg := strings.TrimSpace(m[1])
			lit := strings.Trim(arg, "\"")
			if lit == arg {
				t.Errorf("%s: immyComputerSelect(%s) takes a computed resource type; read roster resources with immyLoadComputers", f, arg)
				continue
			}
			if slices.Contains(immyComputerRosterTypes, lit) {
				t.Errorf("%s: immyComputerSelect(%q) reads a roster resource row by row; use immyLoadComputers", f, lit)
			}
		}
		if strings.Contains(string(raw), "range immyComputerRosterTypes") {
			t.Errorf("%s: walks immyComputerRosterTypes itself; use immyLoadComputers", f)
		}
	}
}
