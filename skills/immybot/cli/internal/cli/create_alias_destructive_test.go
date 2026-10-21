// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (NOT generated): pinned by skills/immybot/handfixes.json under the
// "no-create-alias-on-destructive-commands" entry.

package cli

import (
	"testing"

	"github.com/spf13/cobra"
)

// TestDestructiveCommandsDoNotAnswerToCreate pins the removal of the hidden
// Cobra alias "create" that the generator gives a POST endpoint command. On
// these commands the POST deletes, cancels, disables, stops, rolls back or
// otherwise destroys or disrupts something, so "<parent> create" ran that call
// while reading like a harmless create. tools/maintainer/check_command_tree.py
// is the fleet gate; this test keeps the fix alive through a reprint.
func TestDestructiveCommandsDoNotAnswerToCreate(t *testing.T) {
	root := RootCmd()
	for _, path := range [][]string{
		{"access", "create-delete-azure-tenant-auth-details"},
		{"application-locks", "create-request-cancellation"},
		{"billing", "create-cancel-subscription"},
		{"computers", "ephemeral-agent-circuit-breaker", "create-reset"},
		{"maintenance-sessions", "actions", "create-by-id-rerun"},
		{"maintenance-sessions", "create-cancel"},
		{"metrics", "create-circuit-breakers-isolate"},
		{"plugins", "create-api-v1-by-provider-link-id-by-catch-all"},
		{"provider-agents", "create-bulk-delete-pending"},
		{"provider-links", "cross-references", "create-by-external-link-id-disable-client-linking"},
		{"scripts", "create-debug-cancel-by-cancellation-id"},
		{"system", "create-disable-immy-support-access"},
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
