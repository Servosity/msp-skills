// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "github.com/spf13/cobra"

// init wires the hand-authored (novel) Servosity commands onto the root
// command through the additive registerNovelCommand hook the generated root
// exposes. Registering here instead of editing internal/cli/root.go keeps the
// generated root 100% templated, so a future reprint preserves both the novel
// sources and their wiring without a hand-merge.
//
// addNovelCommandIfAbsent is a no-op when a generated or promoted command
// already owns the same name, so this file can never shadow API-owned surface.
func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		for _, newCmd := range []func(*rootFlags) *cobra.Command{
			newNovelAttentionCmd,
			newNovelBackupFactsCmd,
			newNovelBillCmd,
			newNovelDriftCmd,
			newNovelEmailDraftCmd,
			newNovelFleetHealthCmd,
			newNovelQbrCmd,
			newNovelQbrAllCmd,
			newNovelRestoreQueueCmd,
			newNovelStaleBackupsCmd,
			newNovelStorageTrendCmd,
			newNovelTriageCmd,
			newNovelUnprovisionedCmd,
		} {
			addNovelCommandIfAbsent(root, newCmd(flags))
		}
	})
}
