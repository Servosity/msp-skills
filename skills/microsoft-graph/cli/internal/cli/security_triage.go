// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// pp:data-source local

package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"microsoft-graph-pp-cli/internal/insights"
)

func newNovelSecurityTriageCmd(flags *rootFlags) *cobra.Command {
	var flagSince string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "triage",
		Short: "Group open security alerts in a recent window by severity and source",
		Long: strings.Trim(`
Filters the locally synced security alerts to those still open
(status new or inProgress) created within a recent window, then counts them by
severity and by detection source — the morning triage view alerts_v2 does not
return in one call.

Requires alerts synced: run 'microsoft-graph-cli pull' first.`, "\n"),
		Example: strings.Trim(`
  microsoft-graph-cli security triage --since 24h --agent
  microsoft-graph-cli security triage --since 7d --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			window, err := parseWindow(flagSince, 24*time.Hour)
			if err != nil {
				return err
			}
			since := time.Now().UTC().Add(-window)
			alerts, err := loadDomainRows(dbPath, `SELECT data FROM security WHERE title IS NOT NULL AND title != ''`)
			if err != nil {
				return apiErr(fmt.Errorf("reading local store: %w", err))
			}
			result := insights.SecurityTriage(alerts, since)
			hintUnsyncedIfEmpty(cmd, dbPath, result.TotalOpen == 0)
			return printJSONFiltered(cmd.OutOrStdout(), result, flags)
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "24h", "Only alerts created within this window (e.g. 24h, 90m, 7d)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local store path (default: ~/.local/share/microsoft-graph-cli/data.db)")
	return cmd
}
