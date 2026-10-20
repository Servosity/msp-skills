// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED (replaces the generated novel scaffold).
// pp:data-source local

package cli

import (
	"strings"

	"github.com/spf13/cobra"
)

func newNovelFleetCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fleet",
		Short: "Register client NAS units and roll up their Active Backup jobs and storage health",
		Long: strings.Trim(`
An MSP fleet: one entry per client NAS, each with its own login read from
env vars (never stored). 'fleet sync' mirrors ABB job status and storage
health into a local database; 'fleet status', 'fleet health', 'abb failures',
'abb stale', 'abb device' and 'abb trend' read that mirror. With no NAS
registered, the NAS configured through SYNOLOGY_BASE_URL is used as "default".`, "\n"),
		Example: strings.Trim(`
  synology-cli fleet add acme --url https://nas.acme.example:5001 --client "Acme Corp"
  synology-cli fleet sync
  synology-cli fleet status --agent`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local", "pp:typed-exit-codes": "0,2"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	addNovelCommandIfAbsent(cmd, newNovelFleetHealthCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetStatusCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelFleetSyncCmd(flags))
	return cmd
}
