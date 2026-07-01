// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// bench: resources running below a utilization threshold in a window — who is
// free or under-used. Hand-authored novel command.

package cli

import (
	"fmt"
	"sort"
	"strconv"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

type benchRow struct {
	ResourceID      string   `json:"resource_id"`
	Name            string   `json:"name"`
	AvgUtilization  *float64 `json:"avg_utilization"`
	BookedMinutes   int      `json:"booked_minutes"`
	CapacityMinutes int      `json:"capacity_minutes"`
	FreeMinutes     int      `json:"free_minutes"`
}

type benchView struct {
	Start        string     `json:"start"`
	End          string     `json:"end"`
	ThresholdPct int        `json:"threshold_pct"`
	Count        int        `json:"count"`
	Resources    []benchRow `json:"resources"`
}

func newNovelBenchCmd(flags *rootFlags) *cobra.Command {
	var flagStart string
	var flagEnd string
	var flagThreshold string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "bench",
		Short: "Resources below a utilization threshold in a window — who has slack.",
		Long: "Rank resources whose average utilization over the window is below --threshold percent. " +
			"The inverse of overbooked: use it to find who is free before staffing new work. " +
			"Run `sync` first.",
		Example:     "  resourceguru-cli bench --start 2026-06-01 --end 2026-06-07 --threshold 50 --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			threshold := 50
			if flagThreshold != "" {
				v, err := strconv.Atoi(flagThreshold)
				if err != nil || v < 0 || v > 100 {
					_ = cmd.Usage()
					return usageErr(fmt.Errorf("--threshold must be an integer percent 0-100, got %q", flagThreshold))
				}
				threshold = v
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
			cut := float64(threshold) / 100
			rows := make([]benchRow, 0)
			for _, r := range grid {
				// Only resources with real capacity can be "on the bench".
				if r.AvgUtilization == nil || *r.AvgUtilization >= cut {
					continue
				}
				rows = append(rows, benchRow{
					ResourceID:      r.ID,
					Name:            r.Name,
					AvgUtilization:  r.AvgUtilization,
					BookedMinutes:   r.BookedMinutes,
					CapacityMinutes: r.CapacityMinutes,
					FreeMinutes:     r.CapacityMinutes - r.BookedMinutes,
				})
			}
			sort.Slice(rows, func(i, j int) bool {
				return *rows[i].AvgUtilization < *rows[j].AvgUtilization
			})
			view := benchView{Start: start.Format(utilDateFmt), End: end.Format(utilDateFmt), ThresholdPct: threshold, Count: len(rows), Resources: rows}
			out := cmd.OutOrStdout()
			if flags.asJSON || flags.csv || flags.quiet || flags.plain || !isTerminal(out) {
				return printJSONFiltered(out, view, flags)
			}
			emptyHint(cmd, len(grid))
			if len(rows) == 0 {
				fmt.Fprintf(out, "No resources under %d%% utilization between %s and %s.\n", threshold, view.Start, view.End)
				return nil
			}
			tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
			fmt.Fprintf(out, "On the bench (<%d%%) %s → %s (%d)\n", threshold, view.Start, view.End, view.Count)
			fmt.Fprintln(tw, "RESOURCE\tAVG\tFREE(h)\tBOOKED(h)\tCAPACITY(h)")
			for _, r := range rows {
				fmt.Fprintf(tw, "%s\t%s\t%.1f\t%.1f\t%.1f\n",
					nameOrID(r.Name, r.ResourceID), pct(r.AvgUtilization),
					float64(r.FreeMinutes)/60, float64(r.BookedMinutes)/60, float64(r.CapacityMinutes)/60)
			}
			return tw.Flush()
		},
	}
	cmd.Flags().StringVar(&flagStart, "start", "", "Start of the range (YYYY-MM-DD; default today)")
	cmd.Flags().StringVar(&flagEnd, "end", "", "End of the range (YYYY-MM-DD; default start+4 weeks)")
	cmd.Flags().StringVar(&flagThreshold, "threshold", "", "Utilization percent below which a resource counts as benched (default 50)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local store path (default: standard cache location)")
	return cmd
}
