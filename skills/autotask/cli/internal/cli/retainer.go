// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// newNovelRetainerCmd ranks block-hour/retainer contracts by percent consumed
// and projects a run-out date from the recent burn rate in synced time
// entries — contract-burn shows the snapshot; this shows the trajectory.
// pp:data-source local
func newNovelRetainerCmd(flags *rootFlags) *cobra.Command {
	var threshold float64
	var burnWindowDays int
	var dbPath string
	cmd := &cobra.Command{
		Use:   "retainer",
		Short: "Rank block-hour contracts by percent consumed, with projected run-out dates from recent burn rate.",
		Long: `Roll up contract block consumption from the local store, then project each contract's run-out date from the hours actually logged against it in the recent burn window. Flags contracts at or past the percent-consumed threshold. Run ` + "`sync`" + ` first.

Use this command for retainer/block-hour run-out projection and over-threshold flags. Do NOT use it for a plain consumed-vs-purchased snapshot; use 'contract-burn'.`,
		Example: strings.Trim(`
  autotask-cli retainer
  autotask-cli retainer --threshold 80 --agent
  autotask-cli retainer --threshold 90 --burn-window 14 --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if threshold < 0 || threshold > 100 {
				return usageErr(fmt.Errorf("invalid --threshold %v: must be between 0 and 100", threshold))
			}
			if burnWindowDays <= 0 {
				return usageErr(fmt.Errorf("invalid --burn-window %d: must be a positive number of days", burnWindowDays))
			}
			db, err := openNovelStore(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "contract-blocks") {
				hintIfStale(cmd, db, "contract-blocks", flags.maxAge)
			}
			if !hintIfUnsynced(cmd, db, "time-entries") {
				hintIfStale(cmd, db, "time-entries", flags.maxAge)
			}
			blocks, err := listEntity(db, "contract-blocks")
			if err != nil {
				return apiErr(err)
			}
			entries, _ := listEntity(db, "time-entries")

			type retainer struct {
				ContractID      string  `json:"contractID"`
				HoursPurchased  float64 `json:"hoursPurchased"`
				HoursUsed       float64 `json:"hoursUsed"`
				HoursRemaining  float64 `json:"hoursRemaining"`
				PercentBurned   float64 `json:"percentBurned"`
				RecentDailyBurn float64 `json:"recentDailyBurnHours"`
				RunOutDays      float64 `json:"runOutDays,omitempty"`
				RunOutDate      string  `json:"runOutDate,omitempty"`
				OverThreshold   bool    `json:"overThreshold"`
			}
			byContract := map[string]*retainer{}
			for _, b := range blocks {
				cid := strAt(b, "contractID", "contractId")
				if cid == "" {
					cid = "(unknown)"
				}
				if byContract[cid] == nil {
					byContract[cid] = &retainer{ContractID: cid}
				}
				rc := byContract[cid]
				purchased, used, remaining := accrueBlock(b)
				rc.HoursPurchased += purchased
				rc.HoursUsed += used
				rc.HoursRemaining += remaining
			}

			// Recent burn rate: hours logged per contract inside the window.
			now := time.Now()
			windowStart := now.Add(-time.Duration(burnWindowDays) * 24 * time.Hour)
			recentHours := map[string]float64{}
			for _, e := range entries {
				cid := strAt(e, "contractID", "contractId")
				if cid == "" || byContract[cid] == nil {
					continue
				}
				worked, ok := timeAt(e, "dateWorked", "startDateTime")
				if !ok || worked.Before(windowStart) {
					continue
				}
				h, _ := numAt(e, "hoursWorked", "hoursToBill", "hours")
				recentHours[cid] += h
			}

			rows := make([]retainer, 0, len(byContract))
			for cid, rc := range byContract {
				if rc.HoursPurchased > 0 {
					rc.PercentBurned = (rc.HoursUsed / rc.HoursPurchased) * 100
				}
				rc.RecentDailyBurn = recentHours[cid] / float64(burnWindowDays)
				if rc.RecentDailyBurn > 0 && rc.HoursRemaining > 0 {
					rc.RunOutDays = rc.HoursRemaining / rc.RecentDailyBurn
					// Clamp: a near-zero burn rate yields an astronomically
					// distant run-out; Duration overflows past ~292y and a
					// 100-year projection carries no signal anyway. Report
					// the days figure but omit the date past the horizon.
					if rc.RunOutDays <= 36500 {
						rc.RunOutDate = now.Add(time.Duration(rc.RunOutDays * 24 * float64(time.Hour))).Format("2006-01-02")
					}
				}
				rc.OverThreshold = rc.PercentBurned >= threshold
				rows = append(rows, *rc)
			}
			sort.Slice(rows, func(i, j int) bool { return rows[i].PercentBurned > rows[j].PercentBurned })

			over := 0
			for _, r := range rows {
				if r.OverThreshold {
					over++
				}
			}
			out := map[string]any{
				"threshold":      threshold,
				"burnWindowDays": burnWindowDays,
				"contracts":      rows,
				"overThreshold":  over,
			}
			return flags.printJSON(cmd, out)
		},
	}
	cmd.Flags().Float64Var(&threshold, "threshold", 80, "flag contracts at or past this percent consumed")
	cmd.Flags().IntVar(&burnWindowDays, "burn-window", 30, "days of recent time entries used to compute the burn rate")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/autotask-cli/data.db)")
	return cmd
}
