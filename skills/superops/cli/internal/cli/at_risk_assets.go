// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newNovelAtRiskAssetsCmd(flags *rootFlags) *cobra.Command {
	var flagClient string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "at-risk-assets",
		Short: "List assets missing a critical patch that also carry an active alert.",
		Long: `Intersect assets whose patch status signals missing/critical patches with assets
that have an unresolved alert (asset -> alert link), surfacing endpoints that are
both vulnerable and actively alerting.

Note: the asset -> ticket link is not present on the SuperOps list payloads, so an
active (unresolved) alert is the synced proxy for "currently causing pain". See
README "Known Gaps".`,
		Example: strings.Trim(`
  superops-cli at-risk-assets
  superops-cli at-risk-assets --client Acme --json
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			db, err := openStoreForNovel(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			assets, err := queryRecs(db, "assets")
			if err != nil {
				return err
			}
			alerts, err := queryRecs(db, "alerts")
			if err != nil {
				return err
			}
			rows := computeAtRiskAssets(assets, alerts, flagClient)
			if flags.asJSON {
				return printJSONFiltered(cmd.OutOrStdout(), rows, flags)
			}
			out := cmd.OutOrStdout()
			if len(rows) == 0 {
				fmt.Fprintln(out, "No at-risk assets found (patch-risk + active alert). (Run 'sync' if this looks empty.)")
				return nil
			}
			fmt.Fprintf(out, "%-24s %-20s %-18s %6s\n", "ASSET", "CLIENT", "PATCH_STATUS", "ALERTS")
			for _, r := range rows {
				name := r.Name
				if name == "" {
					name = r.HostName
				}
				fmt.Fprintf(out, "%-24s %-20s %-18s %6d\n", truncate(name, 24), truncate(r.Client, 20), truncate(r.PatchStatus, 18), r.OpenAlerts)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagClient, "client", "", "Limit to a single client (by name)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/superops-cli/data.db)")
	return cmd
}
