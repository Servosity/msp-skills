// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel feature (reprint 20260606): one-screen fleet top-line.

package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// pp:data-source local
func newNovelFleetSummaryCmd(flags *rootFlags) *cobra.Command {
	var staleDays int
	var dbPath string
	cmd := &cobra.Command{
		Use:   "fleet-summary",
		Short: "One-screen fleet top-line: orgs, agents, open criticals, worst ages",
		Long: "Aggregates the whole fleet into one glance: org and agent counts, open incidents\n" +
			"by severity, the oldest unactioned critical, stale agents, and orgs with zero agents.\n" +
			"Do NOT use it for the actual incident queue; use 'fleet-incidents' instead.\n" +
			"Do NOT use it for per-agent posture detail; use 'coverage-gaps' instead.\n" +
			"Reads the local store; run `sync` first.",
		Example:     "  huntress-cli fleet-summary --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would aggregate fleet counts from the local store")
				return nil
			}
			ctx := cmd.Context()
			st, err := openStoreFor(cmd, flags, dbPath, rtOrgs, rtAgents, rtIncidents)
			if err != nil {
				return err
			}
			defer st.Close()

			q := fmt.Sprintf(`SELECT
				(SELECT COUNT(*) FROM resources WHERE resource_type='%s') AS organizations,
				(SELECT COUNT(*) FROM resources WHERE resource_type='%s') AS agents,
				(SELECT COUNT(*) FROM resources i WHERE i.resource_type='%s' AND %s='sent') AS open_incidents,
				(SELECT COUNT(*) FROM resources i WHERE i.resource_type='%s' AND %s='sent' AND %s='critical') AS open_criticals,
				(SELECT CAST(MAX((julianday('now') - %s) * 24) AS INTEGER)
					FROM resources i WHERE i.resource_type='%s' AND %s='sent' AND %s='critical'
					AND %s IS NOT NULL AND %s != '') AS oldest_open_critical_hours,
				(SELECT COUNT(*) FROM resources a WHERE a.resource_type='%s'
					AND %s IS NOT NULL AND %s != ''
					AND julianday('now') - %s > ?) AS stale_agents,
				(SELECT COUNT(*) FROM resources o WHERE o.resource_type='%s'
					AND NOT EXISTS (SELECT 1 FROM resources a WHERE a.resource_type='%s'
						AND %s = %s)) AS orgs_with_zero_agents`,
				rtOrgs, rtAgents,
				rtIncidents, jx("i", "status"),
				rtIncidents, jx("i", "status"), jx("i", "severity"),
				jdx("i", "sent_at"), rtIncidents, jx("i", "status"), jx("i", "severity"), jx("i", "sent_at"), jx("i", "sent_at"),
				rtAgents, jx("a", "last_callback_at"), jx("a", "last_callback_at"), jdx("a", "last_callback_at"),
				rtOrgs, rtAgents, jx("a", "organization_id"), jx("o", "id"))

			res, err := queryMaps(ctx, st.DB(), q, staleDays)
			if err != nil {
				return err
			}
			out := map[string]interface{}{"stale_days_threshold": staleDays}
			if len(res) > 0 {
				for k, v := range res[0] {
					out[k] = v
				}
			}
			if n, ok := toFloat(out["organizations"]); ok && n == 0 {
				out["note"] = "local store is empty; run `sync` first"
			}
			return flags.printJSON(cmd, out)
		},
	}
	cmd.Flags().IntVar(&staleDays, "stale-days", 7, "Agents with no callback in this many days count as stale")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/huntress-cli/data.db)")
	return cmd
}
