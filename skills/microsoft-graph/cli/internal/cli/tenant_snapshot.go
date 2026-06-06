// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// pp:data-source local

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"microsoft-graph-pp-cli/internal/insights"
)

func newNovelTenantSnapshotCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	cmd := &cobra.Command{
		Use:   "snapshot",
		Short: "One-shot tenant posture summary across every synced surface",
		Long: strings.Trim(`
Aggregates every locally synced surface into a single posture summary: user and
guest counts, license waste, privileged-role assignments (and risky ones),
open and high-severity alerts, and non-compliant device counts — the MSP
"where does this tenant stand" answer no single Graph call returns.

Requires a prior sync: run 'microsoft-graph-cli pull' first.`, "\n"),
		Example: strings.Trim(`
  microsoft-graph-cli tenant snapshot --agent
  microsoft-graph-cli tenant snapshot --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			users, err := loadDomainRows(dbPath, `SELECT data FROM users WHERE user_principal_name IS NOT NULL`)
			if err != nil {
				return apiErr(fmt.Errorf("reading local store: %w", err))
			}
			groups, err := loadDomainRows(dbPath, `SELECT data FROM groups`)
			if err != nil {
				return apiErr(fmt.Errorf("reading local store: %w", err))
			}
			licenses, err := loadDomainRows(dbPath, `SELECT data FROM licenses`)
			if err != nil {
				return apiErr(fmt.Errorf("reading local store: %w", err))
			}
			roles, err := loadDomainRows(dbPath, `SELECT data FROM directory_roles`)
			if err != nil {
				return apiErr(fmt.Errorf("reading local store: %w", err))
			}
			alerts, err := loadDomainRows(dbPath, `SELECT data FROM security WHERE title IS NOT NULL AND title != ''`)
			if err != nil {
				return apiErr(fmt.Errorf("reading local store: %w", err))
			}
			devices, err := loadDomainRows(dbPath, `SELECT data FROM managed_devices`)
			if err != nil {
				return apiErr(fmt.Errorf("reading local store: %w", err))
			}
			snap := insights.TenantSnapshot(users, groups, licenses, roles, alerts, devices)
			empty := snap.Users == 0 && snap.Groups == 0 && snap.LicenseSkus == 0 &&
				snap.PrivilegedAssignments == 0 && snap.OpenAlerts == 0 && snap.ManagedDevices == 0
			hintUnsyncedIfEmpty(cmd, dbPath, empty)
			return printJSONFiltered(cmd.OutOrStdout(), snap, flags)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Local store path (default: ~/.local/share/microsoft-graph-cli/data.db)")
	return cmd
}
