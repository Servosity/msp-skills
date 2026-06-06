// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// pp:data-source local

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"microsoft-graph-pp-cli/internal/insights"
)

func newNovelAdminsAuditCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "List every privileged-role holder with role and risk flags",
		Long: strings.Trim(`
Flattens the locally synced directory roles and their members into one
(role, member) table: every holder of a privileged Entra role, with the role
name, account-enabled state, and a risk flag for guest or disabled accounts
that still hold admin access.

Requires roles synced with their members: run 'microsoft-graph-cli pull'
first (pull embeds each role's members).`, "\n"),
		Example: strings.Trim(`
  microsoft-graph-cli admins audit --agent
  microsoft-graph-cli admins audit --json --select role,userPrincipalName,risk`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			roles, err := loadDomainRows(dbPath, `SELECT data FROM directory_roles`)
			if err != nil {
				return apiErr(fmt.Errorf("reading local store: %w", err))
			}
			result := insights.AdminsAudit(roles)
			hintUnsyncedIfEmpty(cmd, dbPath, len(result) == 0)
			return printJSONFiltered(cmd.OutOrStdout(), result, flags)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Local store path (default: ~/.local/share/microsoft-graph-cli/data.db)")
	return cmd
}
