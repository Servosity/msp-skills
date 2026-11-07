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
		Example:     "  datto-rmm-cli fleet agent-drift --agent --select hostname,siteName,cagVersion,lastSeen",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto", "pp:typed-exit-codes": "0,2"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	addNovelCommandIfAbsent(cmd, newNovelFleetAgentDriftCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetAvGapsCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetDeviceTimelineCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetDiffCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetJobResultsCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetOrphansCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetPatchGapsCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetResolveStormCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetScorecardCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetSnapshotCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetSprawlCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetStaleCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetStormsCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetSyncGapsCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetUdfCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetWarrantyCmd(flags))
	return cmd
}
