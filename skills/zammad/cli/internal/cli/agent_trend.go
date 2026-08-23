// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command scaffold. Implement the RunE body before shipping.
// generate --force preserves implemented bodies; untouched TODO scaffolds may refresh.

package cli

import (
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"
)

func newNovelAgentTrendCmd(flags *rootFlags) *cobra.Command {
	var weeks int
	var dbPath string
	var limit int

	cmd := &cobra.Command{
		Use:         "agent-trend",
		Short:       "Show whether each agent's current-window ticket flow is growing, shrinking, or flat.",
		Long:        "Show current and prior opened-versus-closed ticket flow by agent. Historical opens and closes are attributed to each ticket's current owner because the offline snapshot has no per-event ownership history.",
		Example:     "  zammad-cli agent-trend --weeks 2 --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if weeks <= 0 {
				return usageErr(fmt.Errorf("--weeks must be greater than zero"))
			}
			db, err := openNovelStore(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			maybeEmitSyncHints(cmd, db, "tickets", flags.maxAge)

			agentNames, err := loadZammadAgentNames(db)
			if err != nil {
				return err
			}

			type agentTrendRow struct {
				Agent         string `json:"agent"`
				OwnerID       string `json:"owner_id"`
				OpenedCurrent int    `json:"opened_current"`
				ClosedCurrent int    `json:"closed_current"`
				OpenedPrior   int    `json:"opened_prior"`
				ClosedPrior   int    `json:"closed_prior"`
				NetCurrent    int    `json:"net_current"`
				VsPrior       int    `json:"vs_prior"`
				Trend         string `json:"trend"`
			}

			now := time.Now()
			currentStart := now.AddDate(0, 0, -7*weeks)
			priorStart := now.AddDate(0, 0, -14*weeks)
			rows, err := db.Query(`SELECT CAST(owner_id AS TEXT), created_at, data FROM tickets`)
			if err != nil {
				return fmt.Errorf("querying tickets: %w", err)
			}
			defer rows.Close()

			byOwner := make(map[string]*agentTrendRow)
			for rows.Next() {
				var ownerID, createdAt, data sql.NullString
				if err := rows.Scan(&ownerID, &createdAt, &data); err != nil {
					return fmt.Errorf("scanning tickets: %w", err)
				}
				key := zammadOwnerID(ownerID)
				row := byOwner[key]
				if row == nil {
					row = &agentTrendRow{Agent: zammadAgentName(ownerID, agentNames), OwnerID: key}
					byOwner[key] = row
				}
				if createdAt.Valid {
					if created, ok := parseZammadTime(createdAt.String); ok {
						switch {
						case !created.Before(currentStart) && !created.After(now):
							row.OpenedCurrent++
						case !created.Before(priorStart) && created.Before(currentStart):
							row.OpenedPrior++
						}
					}
				}
				if data.Valid {
					if closedRaw := zammadCloseAtFromData(data.String); closedRaw != "" {
						if closed, ok := parseZammadTime(closedRaw); ok {
							switch {
							case !closed.Before(currentStart) && !closed.After(now):
								row.ClosedCurrent++
							case !closed.Before(priorStart) && closed.Before(currentStart):
								row.ClosedPrior++
							}
						}
					}
				}
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("reading tickets: %w", err)
			}

			out := make([]agentTrendRow, 0, len(byOwner))
			for _, row := range byOwner {
				row.NetCurrent = row.OpenedCurrent - row.ClosedCurrent
				row.VsPrior = row.NetCurrent - (row.OpenedPrior - row.ClosedPrior)
				row.Trend = agentTrendDirection(row.NetCurrent)
				if row.OpenedCurrent+row.ClosedCurrent+row.OpenedPrior+row.ClosedPrior == 0 {
					continue
				}
				out = append(out, *row)
			}
			sort.SliceStable(out, func(i, j int) bool {
				left := out[i].OpenedCurrent + out[i].ClosedCurrent + out[i].OpenedPrior + out[i].ClosedPrior
				right := out[j].OpenedCurrent + out[j].ClosedCurrent + out[j].OpenedPrior + out[j].ClosedPrior
				if left != right {
					return left > right
				}
				return out[i].Agent < out[j].Agent
			})
			if limit > 0 && len(out) > limit {
				out = out[:limit]
			}
			return printJSONFiltered(cmd.OutOrStdout(), out, flags)
		},
	}
	cmd.Flags().IntVar(&weeks, "weeks", 2, "Window size in weeks for current and prior trend comparison")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database file path (default: resolved data directory data.db)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum agents to return (0 means all)")
	return cmd
}

func agentTrendDirection(netCurrent int) string {
	switch {
	case netCurrent > 0:
		return "growing"
	case netCurrent < 0:
		return "shrinking"
	default:
		return "flat"
	}
}
