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

func newNovelUsersListCmd(flags *rootFlags) *cobra.Command {
	var mc string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List users with status, transfer status, node, roles and 2FA",
		Long: `Use this command to list every user of one known enterprise or managed company.
Do NOT use this command to find which company a user belongs to; use 'users find' instead.

Runs 'enterprise-info --format=json --users'. Reads in the MSP's own enterprise, or with --mc inside one managed company (the Service Mode session switches in and back).
2fa_enabled is present only when 2FA is on (Commander omits it when off).`,
		Example: strings.Trim(`
  keeper-cli users list --agent
  keeper-cli users list --mc 4421 --agent --select email,status,2fa_enabled`, "\n"),
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runKeeperReport(cmd, flags, mc, fixed(keeper.Users()))
		},
	}
	cmd.Flags().StringVar(&mc, "mc", "", "Managed company id (company_id from 'companies list'); omit for the MSP's own enterprise")
	return cmd
}
