// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command scaffold. Implement the RunE body before shipping.
// generate --force preserves implemented bodies; untouched TODO scaffolds may refresh.
// pp:data-source auto
// Supported strategies: auto, local, live, or computed. Change this default deliberately.

package cli

import (
	"github.com/spf13/cobra"
)

func newNovelFleetCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:         "fleet",
		Short:       "Reseller-wide audits across every client domain (seats, offline phones, DID routing, E911, missed calls, queues, changes)",
		Example:     "  netsapiens-cli fleet changes --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto", "pp:typed-exit-codes": "0,2"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	addNovelCommandIfAbsent(cmd, newNovelFleetChangesCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetDidsCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetE911Cmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetMissedCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetOfflineCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetQueuesCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetSeatsCmd(flags))
	return cmd
}
