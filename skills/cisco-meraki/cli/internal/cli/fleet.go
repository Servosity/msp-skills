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
		Short:       "Run cross-org MSP views over every organization the API key can see: health, outages, renewals, changes, admins, lookups and bulk pushes",
		Example:     "  cisco-meraki-cli fleet admins --stale 90d --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto", "pp:typed-exit-codes": "0,2"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	addNovelCommandIfAbsent(cmd, newNovelFleetAdminsCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetBlameCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetChangesCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetDownCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetFanoutCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetFindCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetFlapsCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetHealthCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetPushCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetRenewalsCmd(flags))
	return cmd
}
