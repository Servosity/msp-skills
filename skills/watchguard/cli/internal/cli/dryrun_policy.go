// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored (handfixes.json: dry-run-side-effect-free).

package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"watchguard-pp-cli/internal/store"
)

// dryRunRefusedCommands change only local state and have no preview of their
// own; under --dry-run they stop with a usage error (exit 2) before doing
// anything. Every other command either sends everything through the API
// client (whose dry run prints the request and sends nothing) or honors
// --dry-run itself (auth login/logout/set-token, feedback, sync, workflow
// archive, import, teach*, learnings *, playbook amend, firebox-devices
// changes, doctor). TestDryRunIsSideEffectFreeEverywhere enforces the rest.
var dryRunRefusedCommands = map[string]string{
	"jobs prune":     "deletes finished jobs from the local job log",
	"profile save":   "writes a saved profile",
	"profile delete": "deletes a saved profile",
}

// applyDryRunPolicy runs from the root PersistentPreRunE after a saved
// profile has had its say. Every store open in a --dry-run process is
// read-only (no command can create, migrate or write the local database),
// and a dry run never delivers to a webhook.
func applyDryRunPolicy(cmd *cobra.Command, flags *rootFlags) error {
	store.SetReadOnlyProcess(flags.dryRun)
	if !flags.dryRun {
		return nil
	}
	path := strings.TrimPrefix(cmd.CommandPath(), cmd.Root().Name()+" ")
	if why, ok := dryRunRefusedCommands[path]; ok {
		return usageErr(fmt.Errorf("--dry-run is not supported by '%s': it %s, which a dry run cannot preview; run it without --dry-run", path, why))
	}
	if path == "export" {
		if f := cmd.Flags().Lookup("output"); f != nil && f.Changed {
			return usageErr(fmt.Errorf("--dry-run cannot be combined with export --output: a dry run writes no file"))
		}
	}
	if flags.deliverSink.Scheme == "webhook" {
		return usageErr(fmt.Errorf("--dry-run cannot be combined with --deliver webhook: a dry run sends nothing"))
	}
	return nil
}

// dryRunInvocation also catches a parse failure, where PersistentPreRunE never
// ran: a --dry-run invocation leaves no learn journal entry either.
func dryRunInvocation(flags *rootFlags) bool {
	if flags != nil && flags.dryRun {
		return true
	}
	for _, tok := range os.Args[1:] {
		if tok == "--" {
			break
		}
		if tok == "--dry-run" || tok == "--dry-run=true" || tok == "--dry-run=1" {
			return true
		}
	}
	return false
}
