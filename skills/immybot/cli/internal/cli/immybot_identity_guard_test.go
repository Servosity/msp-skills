// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Structural guard: every resource's join reads its computer id field and
// nothing else, and no hand-written file in this package compares a mirrored
// computer name in SQL or builds a join from a shape's name field.
func TestComputerRowsJoinByIDOnly(t *testing.T) {
	for rt, shape := range immyComputerShapes {
		clause, args := immyComputerJoinFor(rt, "42")
		if len(args) != 1 || !strings.Contains(clause, immyJSONPath(shape.id[0])) {
			t.Errorf("%s: join %q binds %d values, want the id field alone", rt, clause, len(args))
		}
		for _, n := range shape.name {
			if strings.Contains(clause, immyJSONPath(n)) {
				t.Errorf("%s: join %q reads the name field %s", rt, clause, n)
			}
		}
	}
	nameSQL := regexp.MustCompile(`json_extract\(\s*data\s*,\s*'\$\.(computerName|name)'\s*\)\s*(=|IN\b|LIKE\b)`)
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if m := nameSQL.Find(b); m != nil {
			t.Errorf("%s compares a computer name in SQL (%s): join by computer id", f, m)
		}
		if strings.Contains(string(b), "immyJSONPath(shape.name") {
			t.Errorf("%s builds a JSON path from a shape's name field: join by computer id", f)
		}
	}
}
