// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command (keeper handfix commander-allowlist).
// pp:data-source live
// pp:client-call

package cli

import (
	"strings"

	"keeper-pp-cli/internal/keeper"

	"github.com/spf13/cobra"
)

func newNovelTeamsListCmd(flags *rootFlags) *cobra.Command {
	var mc string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List teams with node and user count",
		Long:  `Runs 'enterprise-info --format=json --teams'. Reads in the MSP's own enterprise, or with --mc inside one managed company (the Service Mode session switches in and back).`,
		Example: strings.Trim(`
  keeper-cli teams list --agent
  keeper-cli teams list --mc 4421 --agent`, "\n"),
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runKeeperReport(cmd, flags, mc, fixed(keeper.Teams()))
		},
	}
	cmd.Flags().StringVar(&mc, "mc", "", "Managed company id (company_id from 'companies list'); omit for the MSP's own enterprise")
	return cmd
}
