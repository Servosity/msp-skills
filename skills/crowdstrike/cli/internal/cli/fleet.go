// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel-feature parent (Printing Press transcendence).

package cli

import (
	"github.com/spf13/cobra"
)

func newNovelFleetCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:   "fleet",
		Short: "Cross-tenant Flight Control rollups over a CID-keyed local store",
		Long: "Flight-Control-aware fleet commands. 'fleet sync' pulls hosts, alerts, " +
			"vulnerabilities, prevention policies, and the Flight Control fabric from every " +
			"child CID into one local store; the rollups (scorecard, vulns, stale, " +
			"policy-drift, alerts, search, tenants, remediate, trend) then answer " +
			"cross-tenant questions instantly and offline.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newNovelFleetAlertsCmd(flags))
	cmd.AddCommand(newNovelFleetPolicyDriftCmd(flags))
	cmd.AddCommand(newNovelFleetRemediateCmd(flags))
	cmd.AddCommand(newNovelFleetScorecardCmd(flags))
	cmd.AddCommand(newNovelFleetSearchCmd(flags))
	cmd.AddCommand(newNovelFleetStaleCmd(flags))
	cmd.AddCommand(newNovelFleetSyncCmd(flags))
	cmd.AddCommand(newNovelFleetTenantsCmd(flags))
	cmd.AddCommand(newNovelFleetTrendCmd(flags))
	cmd.AddCommand(newNovelFleetVulnsCmd(flags))
	return cmd
}
