// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// pp:data-source local

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"microsoft-graph-pp-cli/internal/insights"
)

func newNovelGroupsRiskCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var guestRatio float64
	cmd := &cobra.Command{
		Use:   "risk",
		Short: "Flag ownerless, empty, and guest-heavy groups across the tenant",
		Long: strings.Trim(`
Audits every locally synced group for governance risk: groups with no owners
(nobody accountable), groups with no members (sprawl debris), and groups whose
membership is dominated by guest accounts (external-access risk). Graph has no
server-side filter for any of these shapes; the audit joins the embedded
members and owners associations in the local store.

Requires groups synced with associations: run 'microsoft-graph-cli pull'
(or 'pull --only groups') first. Group rows without embedded members/owners
data are counted in missingAssociationData and never flagged.`, "\n"),
		Example: strings.Trim(`
  microsoft-graph-cli groups risk --agent
  microsoft-graph-cli groups risk --guest-ratio 0.3 --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if guestRatio < 0 || guestRatio > 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--guest-ratio must be between 0 and 1 (0 disables the guest-heavy check)"))
			}
			rows, err := loadDomainRows(dbPath, `SELECT data FROM groups`)
			if err != nil {
				return apiErr(fmt.Errorf("reading local store: %w", err))
			}
			result := insights.GroupsRisk(rows, guestRatio)
			hintUnsyncedIfEmpty(cmd, dbPath, result.ScannedGroups == 0)
			return printJSONFiltered(cmd.OutOrStdout(), result, flags)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Local store path (default: ~/.local/share/microsoft-graph-cli/data.db)")
	cmd.Flags().Float64Var(&guestRatio, "guest-ratio", 0.5, "Guest-member ratio at or above which a group is flagged guest-heavy (0 disables)")
	return cmd
}
