// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
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
	Unverified       int                  `json:"clients_unverified"`
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
			unverified := 0
			historyTruncated := 0
			for ci := range clients {
				c := clients[ci]
				if c.Hidden && !flagIncludeHidden {
					continue
				}
				m := corkScoreMove(c.ScoreHistory, cutoff)
				if !m.ok {
					// Too little history to assert a movement either way.
					unverified++
					continue
				}
				examined++
				if !m.hasBase {
					// No embedded point predates the window, so the real span is
					// bounded by the ten-point payload, not by --since.
					historyTruncated++
				}
				oldest, newest, used := m.then, m.now, m.used
				if m.hasBase {
					used++
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
				Unverified:       unverified,
			}
			direction := "dropped"
			if flagImproved {
				direction = "improved"
			}
			if examined == 0 && unverified == 0 {
				view.Note = "no clients in the local mirror; run: cork-cli sync --resources clients"
			} else if moved == 0 && examined > 0 {
				view.Note = fmt.Sprintf("examined %d clients; none %s by at least %d points in the last %s", examined, direction, flagMinDrop, corkWindowLabel(flagSince, window))
			}
			if unverified > 0 {
				if view.Note != "" {
					view.Note += "; "
				}
				view.Note += fmt.Sprintf("%d client(s) had too little score history to compare and were not checked", unverified)
			}
			// Appended last so it cannot be overwritten by the branches above.
			// Cork embeds only the ten most recent score points in the client
			// payload, so a long --since can silently measure a shorter span.
			if historyTruncated > 0 {
				if view.Note != "" {
					view.Note += "; "
				}
				view.Note += fmt.Sprintf("%d client(s) had no embedded score point older than the requested window, so their delta covers a shorter span than %s — use 'score attribute' for one client's full history", historyTruncated, corkWindowLabel(flagSince, window))
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
			if err := tw.Flush(); err != nil {
				return err
			}
			// A shorter-than-requested window changes what every delta means, so
			// the human table must carry the same caveat the JSON does.
			if view.Note != "" {
				fmt.Fprintln(cmd.OutOrStdout(), "\n"+view.Note)
			}
			return nil
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

type corkMove struct {
	then, now *corkScorePoint
	used      int  // points inside the window
	hasBase   bool // then is the last point before the window opened
	ok        bool // two distinct points to compare (or a held score)
}

// corkScoreMove measures one client's movement across the window: from the score
// at window open (the newest point before cutoff) to the newest point. Without a
// pre-window point it falls back to the oldest in-window point, which needs a
// second in-window point to compare against.
func corkScoreMove(hist []corkScorePoint, cutoff time.Time) corkMove {
	var m corkMove
	var newestT, baseT, oldestInT time.Time
	var oldestIn *corkScorePoint
	for i := range hist {
		t, ok := corkParseTime(hist[i].CreatedAt)
		if !ok {
			continue
		}
		if m.now == nil || t.After(newestT) {
			m.now, newestT = &hist[i], t
		}
		if t.Before(cutoff) {
			if !m.hasBase || t.After(baseT) {
				m.then, baseT, m.hasBase = &hist[i], t, true
			}
			continue
		}
		m.used++
		if oldestIn == nil || t.Before(oldestInT) {
			oldestIn, oldestInT = &hist[i], t
		}
	}
	if !m.hasBase {
		m.then = oldestIn
	}
	m.ok = m.now != nil && m.then != nil && (m.hasBase || m.used >= 2)
	return m
}
