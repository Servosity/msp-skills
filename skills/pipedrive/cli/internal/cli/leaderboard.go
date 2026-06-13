// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// Novel feature: owner leaderboard. Hand-authored against the local store.

package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"pipedrive-pp-cli/internal/pipeintel"
)

type leaderboardRow struct {
	OwnerID          string  `json:"owner_id"`
	OwnerName        string  `json:"owner_name"`
	OpenCount        int     `json:"open_count"`
	WonCount         int     `json:"won_count"`
	LostCount        int     `json:"lost_count"`
	OpenValue        float64 `json:"open_value"`
	WonValue         float64 `json:"won_value"`
	WeightedPipeline float64 `json:"weighted_pipeline"`
	WinRate          float64 `json:"win_rate"`
}

type leaderboardResult struct {
	By         string           `json:"by"`
	WindowDays string           `json:"window,omitempty"`
	Rows       []leaderboardRow `json:"rows"`
}

var leaderboardSortKeys = map[string]func(leaderboardRow) float64{
	"won-value":  func(r leaderboardRow) float64 { return r.WonValue },
	"open-value": func(r leaderboardRow) float64 { return r.OpenValue },
	"weighted":   func(r leaderboardRow) float64 { return r.WeightedPipeline },
	"won":        func(r leaderboardRow) float64 { return float64(r.WonCount) },
	"open":       func(r leaderboardRow) float64 { return float64(r.OpenCount) },
	"win-rate":   func(r leaderboardRow) float64 { return r.WinRate },
}

// pp:data-source local
func newNovelLeaderboardCmd(flags *rootFlags) *cobra.Command {
	var by string
	var window string
	var limit int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "leaderboard",
		Short: "Per-rep open/won/lost counts, weighted pipeline, won value, and activity count over a window.",
		Long: `Ranks deal owners by contribution: open/won/lost counts, open pipeline value,
weighted pipeline, won value, and win rate. Use --window to scope won/lost
counting to a recent period (open pipeline is always current).

Reads the local store, so run 'pipedrive-cli sync' first.`,
		Example: strings.Trim(`
  pipedrive-cli leaderboard --by won-value --window 90d
  pipedrive-cli leaderboard --by weighted --json
  pipedrive-cli leaderboard --by win-rate --window 30d --agent
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			sortFn, ok := leaderboardSortKeys[by]
			if !ok {
				return fmt.Errorf("--by must be one of: won-value, open-value, weighted, won, open, win-rate (got %q)", by)
			}
			db, err := pdOpenStore(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local store: %w", err)
			}
			defer db.Close()

			if !hintIfUnsynced(cmd, db, "deals") {
				hintIfStale(cmd, db, "deals", flags.maxAge)
			}

			wonCond := "status='won'"
			lostCond := "status='lost'"
			if window != "" {
				cut, perr := pipeintel.ParseSince(window, time.Now().UTC())
				if perr != nil {
					return usageErr(fmt.Errorf("--window: %w", perr))
				}
				cs := cut.Format("2006-01-02 15:04:05")
				// cs is a formatted timestamp literal — injection-safe.
				wonCond += fmt.Sprintf(" AND won_time >= '%s'", cs)
				lostCond += fmt.Sprintf(" AND lost_time >= '%s'", cs)
			}

			query := fmt.Sprintf(`
				SELECT COALESCE(user_id,''), COALESCE(owner_name,'(unassigned)'),
				  SUM(CASE WHEN status='open' THEN 1 ELSE 0 END) AS open_count,
				  SUM(CASE WHEN %s THEN 1 ELSE 0 END) AS won_count,
				  SUM(CASE WHEN %s THEN 1 ELSE 0 END) AS lost_count,
				  COALESCE(SUM(CASE WHEN status='open' THEN value ELSE 0 END),0) AS open_value,
				  COALESCE(SUM(CASE WHEN %s THEN value ELSE 0 END),0) AS won_value,
				  COALESCE(SUM(CASE WHEN status='open' THEN value*COALESCE(NULLIF(probability,0),100)/100.0 ELSE 0 END),0) AS weighted
				FROM deals
				WHERE (is_archived IS NULL OR is_archived=0)
				GROUP BY user_id`, wonCond, lostCond, wonCond)
			rows, err := db.DB().QueryContext(cmd.Context(), query)
			if err != nil {
				return fmt.Errorf("querying leaderboard: %w", err)
			}
			defer rows.Close()

			res := leaderboardResult{By: by, WindowDays: window, Rows: []leaderboardRow{}}
			for rows.Next() {
				var r leaderboardRow
				if err := rows.Scan(&r.OwnerID, &r.OwnerName, &r.OpenCount, &r.WonCount, &r.LostCount,
					&r.OpenValue, &r.WonValue, &r.WeightedPipeline); err != nil {
					return fmt.Errorf("scanning row: %w", err)
				}
				if closed := r.WonCount + r.LostCount; closed > 0 {
					r.WinRate = float64(r.WonCount) / float64(closed)
				}
				res.Rows = append(res.Rows, r)
			}
			if err := rows.Err(); err != nil {
				return err
			}
			sort.SliceStable(res.Rows, func(i, j int) bool { return sortFn(res.Rows[i]) > sortFn(res.Rows[j]) })
			if limit > 0 && len(res.Rows) > limit {
				res.Rows = res.Rows[:limit]
			}

			return emitNovel(cmd, flags, res, func(w io.Writer) {
				if len(res.Rows) == 0 {
					fmt.Fprintln(w, "No deals found. (Run 'sync' if the local store is empty.)")
					return
				}
				win := "all-time"
				if window != "" {
					win = "won/lost within " + window
				}
				fmt.Fprintf(w, "Owner leaderboard by %s (%s):\n\n", by, win)
				tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
				fmt.Fprintln(tw, "OWNER\tOPEN\tWON\tLOST\tWIN%\tOPEN_VALUE\tWON_VALUE\tWEIGHTED")
				for _, r := range res.Rows {
					fmt.Fprintf(tw, "%s\t%d\t%d\t%d\t%.0f%%\t%.0f\t%.0f\t%.0f\n",
						r.OwnerName, r.OpenCount, r.WonCount, r.LostCount, r.WinRate*100,
						r.OpenValue, r.WonValue, r.WeightedPipeline)
				}
				_ = tw.Flush()
			})
		},
	}
	cmd.Flags().StringVar(&by, "by", "won-value", "Sort by: won-value, open-value, weighted, won, open, win-rate")
	cmd.Flags().StringVar(&window, "window", "", "Scope won/lost to a recent window, e.g. 90d, 30d, 12w")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum owners to return (0 = no limit)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/pipedrive-cli/data.db)")
	return cmd
}
