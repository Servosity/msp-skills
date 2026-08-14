// Copyright 2026 geekbrownbear and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command. Preserved across `generate --force`.
// pp:data-source local

package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

// scoreRegressionRow is one client's score movement over the requested window.
type scoreRegressionRow struct {
	ClientUUID string `json:"client_uuid"`
	Client     string `json:"client"`
	ScoreThen  int    `json:"score_then"`
	ScoreNow   int    `json:"score_now"`
	Delta      int    `json:"delta"`
	PointsUsed int    `json:"points_used"`
	FirstAt    string `json:"first_at"`
	LastAt     string `json:"last_at"`
}

type scoreRegressionView struct {
	Window           string               `json:"window"`
	ClientsExamined  int                  `json:"clients_examined"`
	ClientsMoved     int                  `json:"clients_moved"`
	Regressions      []scoreRegressionRow `json:"regressions"`
	HistoryTruncated int                  `json:"history_truncated"`
	Note             string               `json:"note,omitempty"`
}

func newNovelScoreRegressionsCmd(flags *rootFlags) *cobra.Command {
	var flagSince string
	var flagMinDrop int
	var flagLimit int
	var flagDB string
	var flagIncludeHidden bool
	var flagImproved bool

	cmd := &cobra.Command{
		Use:   "regressions",
		Short: "Rank every client by score change over a window, worst movers first.",
		Long: "Rank every client by score change over a window, worst movers first.\n\n" +
			"Reads the score points embedded in the locally mirrored client roster, so one\n" +
			"local query answers a question the API can only answer with a per-client fan-out.\n\n" +
			"Use this command to find which clients moved backwards across the whole book of\n" +
			"business over a time window. Do NOT use this command to explain the cause of one\n" +
			"client's movement; use 'score attribute' instead.",
		Example: "  cork-cli score regressions --since 7d --min-drop 10 --agent",
		Annotations: map[string]string{
			"mcp:read-only": "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "score regressions")
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			window, err := corkSince(flagSince, 7*24*time.Hour)
			if err != nil {
				return err
			}

			db, ok, err := corkOpenStore(ctx, flagDB, cmd.ErrOrStderr(), cmd.OutOrStdout(), "clients")
			if err != nil {
				return err
			}
			if !ok {
				if !wantsHumanTable(cmd.OutOrStdout(), flags) {
					return printJSONFiltered(cmd.OutOrStdout(), scoreRegressionView{
						Window:      window.String(),
						Regressions: make([]scoreRegressionRow, 0),
						Note:        "no local mirror; run: cork-cli sync --resources clients",
					}, flags)
				}
				return nil
			}
			defer db.Close()

			if !hintIfUnsynced(cmd, db, "clients") {
				hintIfStale(cmd, db, "clients", flags.maxAge)
			}

			clients, err := corkLoadClients(ctx, db)
			if err != nil {
				return err
			}

			cutoff := time.Now().Add(-window)
			rows := make([]scoreRegressionRow, 0, len(clients))
			examined := 0
			historyTruncated := 0
			for ci := range clients {
				c := clients[ci]
				if c.Hidden && !flagIncludeHidden {
					continue
				}
				examined++
				// Cork returns score_history newest-first. Take the newest point
				// overall and the oldest point still inside the window.
				var newest, oldest *corkScorePoint
				var newestT, oldestT time.Time
				used := 0
				for i := range c.ScoreHistory {
					p := c.ScoreHistory[i]
					t, parsed := corkParseTime(p.CreatedAt)
					if !parsed {
						continue
					}
					if newest == nil || t.After(newestT) {
						newest = &c.ScoreHistory[i]
						newestT = t
					}
					if t.Before(cutoff) {
						continue
					}
					used++
					if oldest == nil || t.Before(oldestT) {
						oldest = &c.ScoreHistory[i]
						oldestT = t
					}
				}
				if newest == nil || oldest == nil || used < 2 {
					// Not enough in-window history to assert a movement.
					continue
				}
				if used == len(c.ScoreHistory) && oldestT.After(cutoff) {
					// Every embedded point fell inside the window, so the real
					// span is bounded by the ten-point payload, not by --since.
					historyTruncated++
				}
				delta := newest.Score - oldest.Score
				if flagImproved {
					if delta < flagMinDrop {
						continue
					}
				} else if -delta < flagMinDrop {
					continue
				}
				rows = append(rows, scoreRegressionRow{
					ClientUUID: c.UUID,
					Client:     c.Name,
					ScoreThen:  oldest.Score,
					ScoreNow:   newest.Score,
					Delta:      delta,
					PointsUsed: used,
					FirstAt:    oldest.CreatedAt,
					LastAt:     newest.CreatedAt,
				})
			}

			corkSortStable(rows, func(a, b scoreRegressionRow) bool {
				if flagImproved {
					return a.Delta > b.Delta
				}
				return a.Delta < b.Delta
			})
			moved := len(rows)
			if flagLimit > 0 && len(rows) > flagLimit {
				rows = rows[:flagLimit]
			}

			view := scoreRegressionView{
				Window:           window.String(),
				ClientsExamined:  examined,
				ClientsMoved:     moved,
				Regressions:      rows,
				HistoryTruncated: historyTruncated,
			}
			direction := "dropped"
			if flagImproved {
				direction = "improved"
			}
			if examined == 0 {
				view.Note = "no clients in the local mirror; run: cork-cli sync --resources clients"
			} else if moved == 0 {
				view.Note = fmt.Sprintf("examined %d clients; none %s by at least %d points in the last %s", examined, direction, flagMinDrop, window)
			}
			// Appended last so it cannot be overwritten by the branches above.
			// Cork embeds only the ten most recent score points in the client
			// payload, so a long --since can silently measure a shorter span.
			if historyTruncated > 0 {
				if view.Note != "" {
					view.Note += "; "
				}
				view.Note += fmt.Sprintf("%d client(s) had no embedded score point older than the requested window, so their delta covers a shorter span than %s — use 'score attribute' for one client's full history", historyTruncated, window)
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			if len(rows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), view.Note)
				return nil
			}
			tw := newTabWriter(cmd.OutOrStdout())
			fmt.Fprintln(tw, "CLIENT\tTHEN\tNOW\tDELTA\tPOINTS")
			for _, r := range rows {
				fmt.Fprintf(tw, "%s\t%d\t%d\t%+d\t%d\n", truncate(r.Client, 40), r.ScoreThen, r.ScoreNow, r.Delta, r.PointsUsed)
			}
			return tw.Flush()
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "7d", "Window to measure movement over (7d, 24h, 1w)")
	cmd.Flags().IntVar(&flagMinDrop, "min-drop", 1, "Only report clients that moved at least this many points")
	cmd.Flags().IntVar(&flagLimit, "limit", 25, "Maximum clients to return (0 for all)")
	cmd.Flags().StringVar(&flagDB, "db", "", "Database path")
	cmd.Flags().BoolVar(&flagIncludeHidden, "include-hidden", false, "Include clients marked hidden in Cork")
	cmd.Flags().BoolVar(&flagImproved, "improved", false, "Rank improvements instead of regressions")
	return cmd
}
