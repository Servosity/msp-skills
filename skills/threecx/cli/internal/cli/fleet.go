// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: the fleet parent groups every multi-PBX command.
// pp:data-source auto

package cli

import (
	"github.com/spf13/cobra"
)

func newNovelFleetCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:   "fleet",
		Short: "Register client 3CX phone systems and report on all of them at once",
		Long: `Register each client's 3CX phone system once (fleet add), then read every one of
them in parallel, each with its own credential: health, trunks, licences,
versions, backups, change history, and any read-only XAPI query (fleet exec).
Use --only or --tag to narrow a run, or the global --pbx to target one system.`,
		Example:     "  threecx-cli fleet backups --stale 48h --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto", "pp:typed-exit-codes": "0,2"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	addNovelCommandIfAbsent(cmd, newFleetAddCmd(flags))
	addNovelCommandIfAbsent(cmd, newFleetListCmd(flags))
	addNovelCommandIfAbsent(cmd, newFleetRemoveCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetBackupsCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetChangesCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetExecCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetHealthCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetLicencesCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetTrunksCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetVersionsCmd(flags))
	return cmd
}
