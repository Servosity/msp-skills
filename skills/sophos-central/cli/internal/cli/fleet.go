// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source auto

package cli

import (
	"github.com/spf13/cobra"
)

func newNovelFleetCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:         "fleet",
		Short:       "Cross-tenant views and actions over every customer tenant (refresh, triage, health, gaps, revenue, tamper, siem-collect)",
		Example:     "  sophos-central-cli fleet revenue --month 2026-09 --limit 20",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto", "pp:typed-exit-codes": "0,2"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	addNovelCommandIfAbsent(cmd, newNovelFleetRevenueCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetGapsCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetHealthCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetSiemCollectCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetRefreshCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetTamperCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetTriageCmd(flags))
	return cmd
}
