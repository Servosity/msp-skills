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
		Short:       "Roll up fleet health across companies: offline agents, patch risk, reboots, user lookup, task failures",
		Example:     "  connectwise-rmm-cli fleet brief 4a9caa07-d45a-4c14-aa36-8c1c26e7a029 --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:parent-group": "true", "pp:data-source": "auto", "pp:typed-exit-codes": "0,2"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	addNovelCommandIfAbsent(cmd, newNovelFleetBriefCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetPatchPostureCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetRebootDueCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetScorecardCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetStaleCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetTaskFailuresCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetWhoisCmd(flags))
	return cmd
}
