// Hand-written novel feature. Not generated.
package cli

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// buildTriageQuery returns the per-agent triage aggregate and its bind
// arguments. Split out from the command so the SQL can be exercised directly;
// see triage_agent_join_test.go.
//
// Two Halo realities shape this query, both learned live:
//
//   - Halo's ticket payload carries agent_id but no agent name, so the
//     generated tickets.agent_name column is blank on every row. The name is
//     resolved from the synced agent records instead. A populated agent_name
//     still wins, so a future Halo version (or an includeagent sync) needs no
//     change here. tickets.datecreated is blank for the same reason; Halo's
//     creation timestamp is dateoccurred, and reading the wrong one reported
//     every agent's oldest ticket as 0 days.
//   - The grouping alias must not be "who". tickets has a real who TEXT column
//     that is NULL on every row, and SQLite resolves GROUP BY names against
//     source columns before output aliases, so grouping on that alias used the
//     NULL column and collapsed every agent into a single row no matter what
//     the SELECT computed. Aggregating over a CTE that exposes only
//     agent_label keeps the grouping key unambiguous.
//
// Bind order follows statement order: scope filters (inside the CTE) bind
// first, then the aggregate's stale/breach thresholds, then the agent filter,
// then the limit.
func buildTriageQuery(team, agent string, staleDays, breachHrs, limit int) (string, []any) {
	where := []string{"COALESCE(json_extract(t.data, '$.status_id'), 0) NOT IN (8, 9)"}
	scopeArgs := []any{}
	if team != "" {
		where = append(where, "(t.team = ? OR json_extract(t.data, '$.team') = ?)")
		scopeArgs = append(scopeArgs, team, team)
	}
	// The agent filter matches the resolved label, so it runs outside the CTE
	// where that label exists.
	agentFilter := ""
	agentArgs := []any{}
	if agent != "" {
		agentFilter = "WHERE (agent_label = ? OR LOWER(agent_label) = LOWER(?))"
		agentArgs = append(agentArgs, agent, agent)
	}
	q := `WITH scoped AS (
                SELECT
                    COALESCE(NULLIF(t.agent_name, ''), a.agent_label, '(unassigned)') AS agent_label,
                    COALESCE(NULLIF(t.datecreated, ''), json_extract(t.data, '$.dateoccurred')) AS created_at,
                    t.data AS data
                FROM tickets t
                LEFT JOIN (
                    SELECT CAST(id AS INTEGER) AS agent_ref_id,
                           json_extract(data, '$.name') AS agent_label
                    FROM resources
                    WHERE resource_type = 'agent'
                ) a ON a.agent_ref_id = t.agent_id
                WHERE ` + strings.Join(where, " AND ") + `
            )
            SELECT
                agent_label AS who,
                COUNT(*) AS open_count,
                SUM(CASE WHEN (julianday('now') - julianday(COALESCE(NULLIF(json_extract(data, '$.lastactiondate'),''), created_at))) > ? THEN 1 ELSE 0 END) AS stale_count,
                SUM(CASE WHEN datetime(COALESCE(json_extract(data, '$.targetdate'), '')) BETWEEN datetime('now') AND datetime('now', '+' || ? || ' hours') THEN 1 ELSE 0 END) AS breach_count,
                CAST(MAX(julianday('now') - julianday(created_at)) AS INTEGER) AS oldest_days
            FROM scoped
            ` + agentFilter + `
            GROUP BY agent_label
            ORDER BY open_count DESC, breach_count DESC
            LIMIT ?`
	finalArgs := append([]any{}, scopeArgs...)
	finalArgs = append(finalArgs, staleDays, breachHrs)
	finalArgs = append(finalArgs, agentArgs...)
	finalArgs = append(finalArgs, limit)
	return q, finalArgs
}

// pp:data-source local
func newNovelTriageCmd(flags *rootFlags) *cobra.Command {
	var (
		dbPath    string
		team      string
		agent     string
		staleDays int
		breachHrs int
		limit     int
	)
	cmd := &cobra.Command{
		Use:   "triage",
		Short: "Per-agent open ticket load + stale count + 24h SLA-breach count, in one table",
		Long: `Cross-entity dispatcher view that Halo's UI scatters across five tabs:
per-agent open ticket count, stale-ticket count, and tickets approaching SLA breach
in the next N hours. Joins tickets x agents x statuses locally.

Run 'halopsa-cli sync' first to populate the local database.`,
		Example: strings.Trim(`
  # Default triage across all agents
  halopsa-cli triage --json

  # Scope to a team and the last 14 days of staleness
  halopsa-cli triage --team Support --stale-days 14

  # Tighten breach window
  halopsa-cli triage --breach-within 4 --json
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if dbPath == "" {
				dbPath = defaultDBPath("halopsa-cli")
			}
			db, err := halopsaOpenStoreSchemaAware(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local database: %w\nRun 'halopsa-cli sync' first.", err)
			}
			defer db.Close()

			q, finalArgs := buildTriageQuery(team, agent, staleDays, breachHrs, limit)
			rows, err := db.DB().QueryContext(cmd.Context(), q, finalArgs...)
			if err != nil {
				return fmt.Errorf("query: %w", err)
			}
			defer rows.Close()

			type row struct {
				Agent       string `json:"agent"`
				OpenCount   int    `json:"open"`
				StaleCount  int    `json:"stale"`
				BreachCount int    `json:"breaching"`
				OldestDays  int    `json:"oldest_days"`
			}
			out := []row{}
			for rows.Next() {
				var r row
				var stale, breach, oldest sql.NullInt64
				if err := rows.Scan(&r.Agent, &r.OpenCount, &stale, &breach, &oldest); err != nil {
					continue
				}
				r.StaleCount = int(stale.Int64)
				r.BreachCount = int(breach.Int64)
				r.OldestDays = int(oldest.Int64)
				out = append(out, r)
			}
			sort.SliceStable(out, func(i, j int) bool { return out[i].OpenCount > out[j].OpenCount })

			if flags.asJSON || !isTerminal(cmd.OutOrStdout()) {
				return flags.printJSON(cmd, map[string]any{
					"team":          team,
					"stale_days":    staleDays,
					"breach_within": breachHrs,
					"agents":        out,
					"generated_at":  time.Now().UTC().Format(time.RFC3339),
				})
			}
			if len(out) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No open tickets in scope. Run 'halopsa-cli sync' if the database looks empty.")
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Triage (stale > %dd, breach within %dh)\n\n", staleDays, breachHrs)
			fmt.Fprintf(cmd.OutOrStdout(), "%-30s %6s %6s %9s %12s\n", "AGENT", "OPEN", "STALE", "BREACHING", "OLDEST(DAYS)")
			fmt.Fprintln(cmd.OutOrStdout(), strings.Repeat("-", 70))
			for _, r := range out {
				fmt.Fprintf(cmd.OutOrStdout(), "%-30s %6d %6d %9d %12d\n", r.Agent, r.OpenCount, r.StaleCount, r.BreachCount, r.OldestDays)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	cmd.Flags().StringVar(&team, "team", "", "Limit to one team")
	cmd.Flags().StringVar(&agent, "agent", "", "Limit to one agent (matches the resolved agent name)")
	cmd.Flags().IntVar(&staleDays, "stale-days", 7, "Days without action before a ticket counts as stale")
	cmd.Flags().IntVar(&breachHrs, "breach-within", 24, "Hours-to-breach window")
	cmd.Flags().IntVar(&limit, "limit", 50, "Max agents to show")
	_ = json.Compact
	return cmd
}
