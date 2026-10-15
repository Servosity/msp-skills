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
		Short:       "Roll up every MSP subaccount at once: summary, risk counts, find a user",
		Example:     "  cisco-duo-cli fleet find-user jdoe@example.com --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto", "pp:typed-exit-codes": "0,2"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	addNovelCommandIfAbsent(cmd, newNovelFleetFindUserCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetRiskCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetSummaryCmd(flags))
	return cmd
}
