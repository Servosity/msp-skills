// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: the fleet parent groups every multi-instance command.
// pp:data-source live

package cli

import (
	"github.com/spf13/cobra"
)

func newNovelFleetCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:   "fleet",
		Short: "Report on every registered client Jamf Pro instance at once",
		Long: `Read every registered Jamf Pro instance in parallel, each with its own API
client: find a device anywhere, roll up compliance and patch status, check
enrollment and VPP tokens, audit policies, compare clients to a golden
instance, or run any read-only API GET (fleet exec). Register instances first
with 'instance add'. Use --only or --tag to narrow a run, or the global
--instance flag to target one client.`,
		Example:     "  jamf-pro-cli fleet posture --stale-days 14 --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:typed-exit-codes": "0,2"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	addNovelCommandIfAbsent(cmd, newNovelFleetAppCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetBaselineCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetExecCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetFindCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetHealthCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetPatchCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetPolicyHygieneCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetPostureCmd(flags))
	return cmd
}
