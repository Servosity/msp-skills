// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// overbooked: every resource-day where booked minutes exceed daily capacity,
// fleet-wide. Hand-authored novel command.

package cli

import (
	"fmt"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

type overbookedRow struct {
	ResourceID      string `json:"resource_id"`
	Name            string `json:"name"`
	Date            string `json:"date"`
	BookedMinutes   int    `json:"booked_minutes"`
	CapacityMinutes int    `json:"capacity_minutes"`
	OverMinutes     int    `json:"over_minutes"`
}

type overbookedView struct {
	Start string          `json:"start"`
	End   string          `json:"end"`
	Count int             `json:"count"`
	Rows  []overbookedRow `json:"overbooked"`
}

func newNovelOverbookedCmd(flags *rootFlags) *cobra.Command {
	var flagStart string
	var flagEnd string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "overbooked",
		Short: "List every resource-day where bookings exceed daily capacity, fleet-wide.",
		Long: "Scan the local store for resource-days whose booked minutes exceed that resource's " +
			"daily capacity. Resource Guru only clashes on write; this reports overcommitment across " +
			"the whole fleet for a window. Run `sync` first.",
		Example:     "  resourceguru-cli overbooked --start 2026-06-01 --end 2026-06-30 --agent",
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
			rows := make([]overbookedRow, 0)
			for _, r := range grid {
				for _, d := range r.Days {
					if d.CapacityMinutes > 0 && d.BookedMinutes > d.CapacityMinutes {
						rows = append(rows, overbookedRow{
							ResourceID:      r.ID,
							Name:            r.Name,
							Date:            d.Date,
							BookedMinutes:   d.BookedMinutes,
							CapacityMinutes: d.CapacityMinutes,
							OverMinutes:     d.BookedMinutes - d.CapacityMinutes,
						})
					}
				}
			}
			sort.Slice(rows, func(i, j int) bool {
				if rows[i].OverMinutes != rows[j].OverMinutes {
					return rows[i].OverMinutes > rows[j].OverMinutes
				}
				return rows[i].Date < rows[j].Date
			})
			view := overbookedView{Start: start.Format(utilDateFmt), End: end.Format(utilDateFmt), Count: len(rows), Rows: rows}
			out := cmd.OutOrStdout()
			if flags.asJSON || flags.csv || flags.quiet || flags.plain || !isTerminal(out) {
				return printJSONFiltered(out, view, flags)
			}
			emptyHint(cmd, len(grid))
			if len(rows) == 0 {
				fmt.Fprintf(out, "No overbooked resource-days between %s and %s.\n", view.Start, view.End)
				return nil
			}
			tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
			fmt.Fprintf(out, "Overbooked resource-days %s → %s (%d)\n", view.Start, view.End, view.Count)
			fmt.Fprintln(tw, "RESOURCE\tDATE\tBOOKED(h)\tCAPACITY(h)\tOVER(h)")
			for _, r := range rows {
				fmt.Fprintf(tw, "%s\t%s\t%.1f\t%.1f\t%.1f\n",
					nameOrID(r.Name, r.ResourceID), r.Date,
					float64(r.BookedMinutes)/60, float64(r.CapacityMinutes)/60, float64(r.OverMinutes)/60)
			}
			return tw.Flush()
		},
	}
	cmd.Flags().StringVar(&flagStart, "start", "", "Start of the range (YYYY-MM-DD; default today)")
	cmd.Flags().StringVar(&flagEnd, "end", "", "End of the range (YYYY-MM-DD; default start+4 weeks)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local store path (default: standard cache location)")
	return cmd
}
