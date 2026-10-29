// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"pax8-pp-cli/internal/cliutil/testenv"
)

// pax8 handfix: novel-json-empty-is-array. On an empty store every list a
// novel command prints under --json is [], never null: an agent reads null
// as "no data was produced", not "nothing matched".
func TestNovelJSONListsAreArraysOnAnEmptyStore(t *testing.T) {
	home := testenv.Isolate(t)
	db := filepath.Join(home, "empty.db")
	for _, args := range [][]string{{"since", "7d"}, {"overage"}, {"reconcile"}, {"spend"}, {"mrr"}} {
		root := RootCmd()
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&bytes.Buffer{})
		root.SetArgs(append(args, "--json", "--db", db))
		if err := root.Execute(); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if strings.Contains(out.String(), "null") {
			t.Errorf("%v --json printed null on an empty store:\n%s", args, out.String())
		}
	}
}
