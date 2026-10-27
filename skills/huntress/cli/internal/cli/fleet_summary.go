// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
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
			"and open criticals, the oldest unactioned critical, stale agents, and orgs with zero agents.\n" +
			"Do NOT use it for the actual incident queue; use 'fleet-incidents' instead.\n" +
			"Do NOT use it for per-agent posture detail; use 'coverage-gaps' instead.\n" +
			"Reads the local store; run `sync` first.",
		Example:     "  huntress-cli fleet-summary --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireNonNegative("stale-days", staleDays); err != nil {
				return err
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would aggregate fleet counts from the local store")
				return nil
			}
			ctx := cmd.Context()
			st, err := openStoreFor(cmd, flags, dbPath, entOrgs, entAgents, entIncidents)
			if err != nil {
				return err
			}
			defer st.Close()

			orgs, agents, incidents := st.src(entOrgs), st.src(entAgents), st.src(entIncidents)
			// An open critical whose sent_at cannot be read has an unknown
			// age, so the oldest age is unknown too (MAX would skip it); an
			// agent whose last_callback_at cannot be read is neither stale nor
			// fresh, and is counted under stale_agents_unknown.
			q := fmt.Sprintf(`SELECT
				(SELECT COUNT(*) FROM %s o) AS organizations,
				(SELECT COUNT(*) FROM %s a) AS agents,
				(SELECT COUNT(*) FROM %s i WHERE %s='sent') AS open_incidents,
				(SELECT COUNT(*) FROM %s i WHERE %s='sent' AND %s='critical') AS open_criticals,
				(SELECT CAST(MAX((julianday('now') - %s) * 24) AS INTEGER)
					FROM %s i WHERE %s='sent' AND %s='critical'
					AND %s IS NOT NULL AND %s != '') AS oldest_open_critical_hours,
				(SELECT COUNT(*) FROM %s i WHERE %s='sent' AND %s='critical' AND %s) AS pp_unreadable_critical_age,
				(SELECT COUNT(*) FROM %s a WHERE %s IS NOT NULL AND %s != ''
					AND julianday('now') - %s > ?) AS stale_agents,
				(SELECT COUNT(*) FROM %s a WHERE %s) AS stale_agents_unknown,
				(SELECT COUNT(*) FROM %s o
					WHERE NOT EXISTS (SELECT 1 FROM %s a WHERE %s = %s)) AS orgs_with_zero_agents`,
				orgs, agents,
				incidents, jx("i", "status"),
				incidents, jx("i", "status"), jx("i", "severity"),
				jdx("i", "sent_at"), incidents, jx("i", "status"), jx("i", "severity"), jx("i", "sent_at"), jx("i", "sent_at"),
				incidents, jx("i", "status"), jx("i", "severity"), unreadableTimeSQL("i", "sent_at", true),
				agents, jx("a", "last_callback_at"), jx("a", "last_callback_at"), jdx("a", "last_callback_at"),
				agents, unreadableTimeSQL("a", "last_callback_at", true),
				orgs, agents, jx("a", "organization_id"), jx("o", "id"))

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
			if n, _ := toFloat(out["pp_unreadable_critical_age"]); n > 0 {
				out["oldest_open_critical_hours"] = nil
				warnUnreadable(cmd, int(n), "open critical incident(s) have a missing or unreadable sent_at", "oldest_open_critical_hours is withheld (null) because the oldest age is unknown.")
			}
			delete(out, "pp_unreadable_critical_age")
			if n, _ := toFloat(out["stale_agents_unknown"]); n > 0 {
				warnUnreadable(cmd, int(n), "agent(s) have a missing or unreadable last_callback_at", "whether they are stale is unknown; they are counted under stale_agents_unknown, not stale_agents.")
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
