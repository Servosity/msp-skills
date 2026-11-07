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

func newNovelUsersLastLoginCmd(flags *rootFlags) *cobra.Command {
	var mc string
	var days int
	cmd := &cobra.Command{
		Use:   "last-login",
		Short: "Show each user's last login within a window",
		Long: `Use this command for a live last-login read of one enterprise or managed company.
Do NOT use this command for an all-client cleanup list; use 'users hygiene' instead.

Runs 'user-report --format=json --last-login --days N'. Reads in the MSP's own enterprise, or with --mc inside one managed company (the Service Mode session switches in and back).
last_login is a UTC time, "> N DAYS AGO" when no login fell in the window, or "N/A" for invited users.`,
		Example: strings.Trim(`
  keeper-cli users last-login --days 90 --agent
  keeper-cli users last-login --mc 4421 --days 30 --agent`, "\n"),
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runKeeperReport(cmd, flags, mc, func() (keeper.Command, error) { return keeper.LastLogin(days) })
		},
	}
	cmd.Flags().StringVar(&mc, "mc", "", "Managed company id (company_id from 'companies list'); omit for the MSP's own enterprise")
	cmd.Flags().IntVar(&days, "days", 90, "Days to look back for the last login (1 to 3650)")
	return cmd
}
