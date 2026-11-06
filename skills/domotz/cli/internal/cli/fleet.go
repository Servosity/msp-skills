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
		Example:     "  domotz-cli fleet agents --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto", "pp:typed-exit-codes": "0,2"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	addNovelCommandIfAbsent(cmd, newNovelFleetAgentsCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetAlertsCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetBreakdownCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetEventsCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetHealthCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetInventoryCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetIpConflictsCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetNewCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetOfflineCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetSpeedtestCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetStaleCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetTriggersCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetUnalertedCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetUnmonitoredCmd(flags))
	return cmd
}
