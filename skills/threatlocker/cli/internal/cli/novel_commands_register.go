// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored (novel file, not generated).
//
// Registers `applications hunt` through the generator's novelCommandHooks
// mechanism instead of hand-editing the generated applications.go.
//
// Every other ThreatLocker novel command (devices health, approvals triage,
// approvals approve-batch, audit drift, audit export, audit retention-check)
// is emitted and wired by the press itself when generation is given the
// research directory. `applications hunt` is the one novel feature the
// research manifest does not carry, so it stays hand-authored here.
//
// Before the 4.30.2 reprint this wiring lived as a hand-edit inside the
// generated applications.go, which meant every regeneration silently dropped
// the command -- observed during this reprint (msp-skills #208/#215).
// registerNovelCommand runs after the generated tree is built, so the wiring
// now survives regeneration.
//
// addNovelCommandIfAbsent keeps this idempotent: if a future press version
// starts emitting `applications hunt` natively, the generated one wins and
// this hook becomes a no-op.

package cli

import "github.com/spf13/cobra"

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		if applications := findSubcommand(root, "applications"); applications != nil {
			addNovelCommandIfAbsent(applications, newNovelApplicationsHuntCmd(flags))
		}
	})
}
