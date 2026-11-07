// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (NOT generated): pinned by skills/runzero/handfixes.json under the
// "no-create-alias-on-destructive-commands" entry.

package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestDestructiveCommandsDoNotAnswerToCreate pins the removal of the hidden
// Cobra alias "create" that the generator gives a POST endpoint command. On
// these commands the POST deletes, cancels, disables, stops, rolls back or
// otherwise destroys or disrupts something, so "<parent> create" ran that call
// while reading like a harmless create. tools/maintainer/check_command_tree.py
// is the fleet gate; this test keeps the fix alive through a reprint.
// Class rule (runzero 4.34.2 reprint, create-alias-only-on-create): press 4.34.2
// gives `create` to a POST whose name has no action verb it knows, and `clear`
// is not one, so `org create` ran clear-bulk-asset-owners. Every command that
// answers to `create` must itself be create-shaped (create, add, insert, import).
func TestDestructiveCommandsDoNotAnswerToCreate(t *testing.T) {
	root := RootCmd()
	for _, path := range [][]string{
		{"org", "bulk-remove-custom-integration"},
		{"org", "clear-bulk-asset-owners"},
	} {
		cmd := root
		for _, name := range path {
			var next *cobra.Command
			for _, child := range cmd.Commands() {
				if child.Name() == name {
					next = child
					break
				}
			}
			if next == nil {
				t.Fatalf("command %v not found; the tree changed, update this test", path)
			}
			cmd = next
		}
		if cmd.HasAlias("create") {
			t.Errorf("%v still answers to the alias %q", path, "create")
		}
		asCreate := append(append([]string{}, path[:len(path)-1]...), "create")
		if found, _, err := root.Find(asCreate); err == nil && found == cmd {
			t.Errorf("%v resolves to the destructive command %v", asCreate, path)
		}
	}
}

func TestOnlyCreateShapedCommandsAnswerToCreate(t *testing.T) {
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		for _, c := range cmd.Commands() {
			if c.HasAlias("create") && !createShapedName(c.Name()) {
				t.Errorf("%s answers to the alias %q but is not a create", c.CommandPath(), "create")
			}
			walk(c)
		}
	}
	walk(RootCmd())
}

func createShapedName(name string) bool {
	for _, w := range strings.FieldsFunc(strings.ToLower(name), func(r rune) bool { return r == '-' || r == '_' }) {
		if w == "create" || w == "add" || w == "insert" || w == "import" {
			return true
		}
	}
	return false
}
