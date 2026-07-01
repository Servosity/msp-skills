// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// capacity: remaining bookable minutes per resource over a window (capacity
// minus committed bookings). Hand-authored novel command.

package cli

import (
	"fmt"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

type capacityRow struct {
	ResourceID       string  `json:"resource_id"`
	Name             string  `json:"name"`
	CapacityMinutes  int     `json:"capacity_minutes"`
	BookedMinutes    int     `json:"booked_minutes"`
	RemainingMinutes int     `json:"remaining_minutes"`
	RemainingPct     float64 `json:"remaining_pct"`
}

type capacityView struct {
	Start     string        `json:"start"`
	End       string        `json:"end"`
	Resources []capacityRow `json:"resources"`
}

func newNovelCapacityCmd(flags *rootFlags) *cobra.Command {
	var flagStart string
	var flagEnd string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "capacity",
		Short: "Remaining bookable minutes per resource over a window (capacity minus bookings).",
		Long: "Show how much bookable headroom each resource has left over the window — daily capacity " +
			"minus already-committed bookings, summed and floored at zero per day. Use it to answer " +
			"'can this new project fit?' before you create the bookings. Run `sync` first.",
		Example:     "  resourceguru-cli capacity --start 2026-07-01 --end 2026-07-31 --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			start, end, err := resolveWindow(flagStart, flagEnd)
			if err != nil {
				_ = cmd.Usage()
				return usageErr(err)
			}
			s, err := openUtilStore(cmd, dbPath)
			if err != nil {
				return fmt.Errorf("opening local store: %w", err)
			}
			defer s.Close()
			grid, err := buildUtilization(s, start, end, "")
			if err != nil {
				return fmt.Errorf("computing utilization: %w", err)
			}
			rows := make([]capacityRow, 0, len(grid))
			for _, r := range grid {
				if r.CapacityMinutes == 0 {
					continue // no declared capacity → no headroom to report
				}
				// Remaining is per-day capacity-minus-booked floored at zero, so
				// an overbooked day can't lend negative headroom to another day.
				remaining := 0
				for _, d := range r.Days {
					if free := d.CapacityMinutes - d.BookedMinutes; free > 0 {
						remaining += free
					}
				}
				row := capacityRow{
					ResourceID:       r.ID,
					Name:             r.Name,
					CapacityMinutes:  r.CapacityMinutes,
					BookedMinutes:    r.BookedMinutes,
					RemainingMinutes: remaining,
				}
				row.RemainingPct = float64(remaining) / float64(r.CapacityMinutes) * 100
				rows = append(rows, row)
			}
			sort.Slice(rows, func(i, j int) bool {
				return rows[i].RemainingMinutes > rows[j].RemainingMinutes
			})
			view := capacityView{Start: start.Format(utilDateFmt), End: end.Format(utilDateFmt), Resources: rows}
			out := cmd.OutOrStdout()
			if flags.asJSON || flags.csv || flags.quiet || flags.plain || !isTerminal(out) {
				return printJSONFiltered(out, view, flags)
			}
			emptyHint(cmd, len(grid))
			tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
			fmt.Fprintf(out, "Remaining capacity %s → %s\n", view.Start, view.End)
			fmt.Fprintln(tw, "RESOURCE\tREMAINING(h)\tREMAINING%\tBOOKED(h)\tCAPACITY(h)")
			for _, r := range rows {
				fmt.Fprintf(tw, "%s\t%.1f\t%.0f%%\t%.1f\t%.1f\n",
					nameOrID(r.Name, r.ResourceID),
					float64(r.RemainingMinutes)/60, r.RemainingPct,
					float64(r.BookedMinutes)/60, float64(r.CapacityMinutes)/60)
			}
			return tw.Flush()
		},
	}
	cmd.Flags().StringVar(&flagStart, "start", "", "Start of the range (YYYY-MM-DD; default today)")
	cmd.Flags().StringVar(&flagEnd, "end", "", "End of the range (YYYY-MM-DD; default start+4 weeks)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local store path (default: standard cache location)")
	return cmd
}
