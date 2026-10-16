// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
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
		Short:       "Run cross-tenant views over every child company: health, incident-queue, threats, locate, rmm-gap, changes",
		Example:     "  bitdefender-cli fleet changes --since 7d --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto", "pp:typed-exit-codes": "0,2"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	addNovelCommandIfAbsent(cmd, newNovelFleetChangesCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetHealthCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetIncidentQueueCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetLocateCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetRmmGapCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetThreatsCmd(flags))
	return cmd
}
