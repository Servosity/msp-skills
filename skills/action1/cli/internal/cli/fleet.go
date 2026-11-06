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
		Short:       "Work with fleet",
		Example:     "  action1-cli fleet automation-health --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto", "pp:typed-exit-codes": "0,2"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	addNovelCommandIfAbsent(cmd, newNovelFleetAutomationHealthCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetHealthScoreCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetOrgScorecardCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetPatchDriftCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetPatchPostureCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetRebootPendingCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetSoftwareRollupCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetStaleCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetVulnTriageCmd(flags))
	return cmd
}
