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

func newNovelSecurityAuditReportCmd(flags *rootFlags) *cobra.Command {
	var mc string
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Show each user's weak, fair, medium, strong, reused and unique password counts, security score and 2FA",
		Long: `Use this command for one enterprise's or managed company's live password health per user.
Do NOT use this command for users across every client; use 'users risk' instead.

Runs 'security-audit-report --format=json' (summary only: --record-details, --save and
--attempt-fix are never sent). Reads in the MSP's own enterprise, or with --mc inside one managed company (the Service Mode session switches in and back).
When Keeper answers with its error report (vaults with invalid security data) the command
fails and says so; it never shows that as zero users.`,
		Example: strings.Trim(`
  keeper-cli security-audit report --agent
  keeper-cli security-audit report --mc 4421 --agent --select email,weak,reused,securityScore`, "\n"),
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runKeeperReport(cmd, flags, mc, fixed(keeper.SecurityAudit(false)))
		},
	}
	cmd.Flags().StringVar(&mc, "mc", "", "Managed company id (company_id from 'companies list'); omit for the MSP's own enterprise")
	return cmd
}
