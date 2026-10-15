// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"quotewerks-pp-cli/internal/qw"
)

type winrateRow struct {
	Key       string   `json:"key"`
	Quotes    int      `json:"quotes"`
	Won       int      `json:"won"`
	Lost      int      `json:"lost"`
	Open      int      `json:"open"`
	WinRate   *float64 `json:"win_rate_pct"`
	WonValue  float64  `json:"won_value"`
	LostValue float64  `json:"lost_value"`
	OpenValue float64  `json:"open_value"`
	// UnknownValue counts quotes with no GrandTotal; they are counted but add
	// nothing to the value totals.
	UnknownValue int `json:"unknown_value,omitempty"`
}

func (r *winrateRow) add(outcome string, value float64, known bool) {
	r.Quotes++
	if !known {
		r.UnknownValue++
	}
	switch outcome {
	case qw.OutcomeWon:
		r.Won++
		r.WonValue += value
	case qw.OutcomeLost:
		r.Lost++
		r.LostValue += value
	default:
		r.Open++
		r.OpenValue += value
	}
}

func (r *winrateRow) finish() {
	r.WonValue, r.LostValue, r.OpenValue = qw.Round2(r.WonValue), qw.Round2(r.LostValue), qw.Round2(r.OpenValue)
	r.WinRate = pctPtr(float64(r.Won), float64(r.Won+r.Lost))
}

func newNovelWinrateCmd(flags *rootFlags) *cobra.Command {
	var by, since, wonStatus, lostStatus, dbPath string
	var limit int

	cmd := &cobra.Command{
		Use:   "winrate",
		Short: "Won, lost and open quotes and win rate by rep, customer or month from the local mirror",
		Long: `Classify every mirrored QUOTE as won, lost or open and report counts, values and win
rate (won / (won + lost)). The outcome comes from, in order: your own DocStatus values
(--won-status / --lost-status), then QuoteValetStatusCode (1 or 2 accepted, 3 lost). QuoteValet only records online
acceptance, so deals won by phone read as open unless you pass --won-status; the output
counts which signal decided each quote. Reads the local mirror; run 'mirror' first.

Use this command for quote outcomes (won, lost, open, win rate) grouped by rep, customer
or month. Do NOT use it for profit or margin; use 'margin' instead. Do NOT use it for the
age and value of currently open quotes; use 'pipeline' instead.`,
		Example: strings.Trim(`
  quotewerks-cli winrate --by rep --since 90d --json
  quotewerks-cli winrate --by month --won-status Won,Ordered --lost-status Lost --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "winrate")
			}
			if err := validateBy(by, "rep", "customer", "month"); err != nil {
				return err
			}
			cutoff, hasCutoff, err := sinceCutoff(since, time.Now())
			if err != nil {
				return err
			}
			empty := &winrateRow{Key: "(all)"}
			empty.finish()
			view := map[string]any{"mirrored": false, "filters": filterEcho(cmd), "by": by, "rows": make([]winrateRow, 0), "signals": map[string]int{}, "total": empty}
			ctx := cmd.Context()
			db, ok, err := openMirror(ctx, cmd, flags, dbPath, qwResDocuments)
			if err != nil {
				return err
			}
			if !ok {
				return noMirrorResult(cmd, flags, view)
			}
			defer db.Close()
			view["mirrored"] = true
			view["partial_resources"] = partialResources(db, qwResDocuments)
			quotes, err := mirroredDocs(db, map[string]bool{"QUOTE": true}, cutoff, hasCutoff)
			if err != nil {
				return err
			}
			cls := qw.NewClassifier(wonStatus, lostStatus)
			signals := map[string]int{}
			groups := map[string]*winrateRow{}
			total := &winrateRow{Key: "(all)"}
			for _, q := range quotes {
				outcome, signal := cls.Classify(q)
				signals[signal]++
				k := groupKey(q, by)
				if groups[k] == nil {
					groups[k] = &winrateRow{Key: k}
				}
				v, known := qw.Num(q["GrandTotal"])
				groups[k].add(outcome, v, known)
				total.add(outcome, v, known)
			}
			rows := make([]winrateRow, 0, len(groups))
			for _, g := range groups {
				g.finish()
				rows = append(rows, *g)
			}
			sort.Slice(rows, func(i, j int) bool {
				if rows[i].Quotes != rows[j].Quotes {
					return rows[i].Quotes > rows[j].Quotes
				}
				return rows[i].Key < rows[j].Key
			})
			if limit > 0 && len(rows) > limit {
				rows = rows[:limit]
			}
			total.finish()
			view["rows"] = rows
			view["total"] = total
			view["signals"] = signals
			if signals["QuoteValetStatusCode"] > 0 && wonStatus == "" {
				view["note"] = "outcomes from QuoteValetStatusCode count online acceptance only; pass --won-status with your DocStatus values for deals won offline"
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			if len(rows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No mirrored quotes match.")
				return nil
			}
			line := func(r winrateRow) {
				wr := "n/a"
				if r.WinRate != nil {
					wr = fmt.Sprintf("%.1f%%", *r.WinRate)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-30.30s quotes %4d  won %4d  lost %4d  open %4d  win rate %s\n", sc(r.Key), r.Quotes, r.Won, r.Lost, r.Open, wr)
			}
			for _, r := range rows {
				line(r)
			}
			line(*total)
			if note, ok := view["note"].(string); ok {
				fmt.Fprintln(cmd.OutOrStdout(), "note: "+note)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&by, "by", "rep", "Group by: rep, customer or month")
	cmd.Flags().StringVar(&since, "since", "", "Only quotes dated within this window, e.g. 90d")
	cmd.Flags().StringVar(&wonStatus, "won-status", "", "DocStatus values that mean won, comma-separated (case-insensitive)")
	cmd.Flags().StringVar(&lostStatus, "lost-status", "", "DocStatus values that mean lost, comma-separated (case-insensitive)")
	cmd.Flags().IntVar(&limit, "limit", 25, "Maximum groups to return, most quotes first")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local mirror database path")
	return cmd
}
