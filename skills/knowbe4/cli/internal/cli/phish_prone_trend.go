// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Transcendence command — hand-built over the local insights store.

package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"knowbe4-pp-cli/internal/insights"
)

// pp:data-source local
func newNovelPhishProneTrendCmd(flags *rootFlags) *cobra.Command {
	var group string
	var since string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "phish-prone-trend",
		Short: "Phish-prone % across sequential tests (optionally by group)",
		Long: strings.TrimSpace(`
Plot phish-prone percentage across sequential phishing tests to show whether
training is working. Filter to a group by name with --group; omit it for the
account-wide arc. A single PST is one data point — the trend only exists once many
tests are stored and ordered together.

Run 'knowbe4-cli sync' first.`),
		Example:     "  knowbe4-cli phish-prone-trend --group \"Finance\" --since 12mo --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			db, st, closeFn, err := openInsightsDB(cmd, dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			insightsSyncHints(cmd, st, "phishing-tests", flags)
			trend, err := insights.PhishProneTrendQuery(cmd.Context(), db, group, since)
			if err != nil {
				return err
			}
			return flags.printJSON(cmd, trend)
		},
	}
	cmd.Flags().StringVar(&group, "group", "", "Filter to phishing tests targeting this group name (substring match)")
	cmd.Flags().StringVar(&since, "since", "", "Only tests started within this window (e.g. 12mo, 90d)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: standard location)")
	return cmd
}
