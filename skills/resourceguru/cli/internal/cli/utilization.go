// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// utilization: per-resource per-day booked-vs-available utilization computed
// from the local store. Hand-authored novel command.

package cli

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

type utilizationView struct {
	Start     string         `json:"start"`
	End       string         `json:"end"`
	Days      int            `json:"days"`
	Resources []resourceUtil `json:"resources"`
}

func newNovelUtilizationCmd(flags *rootFlags) *cobra.Command {
	var flagStart string
	var flagEnd string
	var flagResource string
	var flagHeatmap bool
	var dbPath string

	cmd := &cobra.Command{
		Use:   "utilization",
		Short: "Per-resource per-day utilization (booked vs available) over a date range.",
		Long: "Show each resource's booked-vs-available utilization for every day in a range, " +
			"not just a range average. Reads synced bookings and resources from the local store " +
			"(run `sync` first). Use --heatmap for a resource×day grid, --resource to focus one resource.",
		Example:     "  resourceguru-cli utilization --start 2026-06-01 --end 2026-06-30 --agent",
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
			rows, err := buildUtilization(s, start, end, flagResource)
			if err != nil {
				return fmt.Errorf("computing utilization: %w", err)
			}
			view := utilizationView{
				Start:     start.Format(utilDateFmt),
				End:       end.Format(utilDateFmt),
				Days:      len(dayRange(start, end)),
				Resources: rows,
			}
			out := cmd.OutOrStdout()
			machine := flags.asJSON || flags.csv || flags.quiet || flags.plain
			// --heatmap is an explicit human-format request, so honor it even
			// when piped; otherwise default piped output to JSON for agents.
			if machine || (!flagHeatmap && !isTerminal(out)) {
				return printJSONFiltered(out, view, flags)
			}
			emptyHint(cmd, len(rows))
			if flagHeatmap {
				renderHeatmap(cmd, view)
				return nil
			}
			tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
			fmt.Fprintf(out, "Utilization %s → %s (%d days)\n", view.Start, view.End, view.Days)
			fmt.Fprintln(tw, "RESOURCE\tAVG\tBOOKED(h)\tCAPACITY(h)\tOVERBOOKED DAYS")
			for _, r := range rows {
				fmt.Fprintf(tw, "%s\t%s\t%.1f\t%.1f\t%d\n",
					nameOrID(r.Name, r.ID), pct(r.AvgUtilization),
					float64(r.BookedMinutes)/60, float64(r.CapacityMinutes)/60, r.OverbookedDays)
			}
			return tw.Flush()
		},
	}
	cmd.Flags().StringVar(&flagStart, "start", "", "Start of the range (YYYY-MM-DD; default today)")
	cmd.Flags().StringVar(&flagEnd, "end", "", "End of the range (YYYY-MM-DD; default start+4 weeks)")
	cmd.Flags().StringVar(&flagResource, "resource", "", "Limit to a single resource id")
	cmd.Flags().BoolVar(&flagHeatmap, "heatmap", false, "Render a resource×day utilization grid")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local store path (default: standard cache location)")
	return cmd
}

func nameOrID(name, id string) string {
	if name != "" {
		return name
	}
	return "#" + id
}

// renderHeatmap prints a compact resource×day grid: one glyph per day bucketed
// by utilization. Pattern over precision — the JSON view carries the numbers.
func renderHeatmap(cmd *cobra.Command, view utilizationView) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Utilization heatmap %s → %s (%d days)\n", view.Start, view.End, view.Days)
	fmt.Fprintln(out, "legend: · idle  ░ <50%  ▒ <80%  ▓ <100%  █ 100%  X over  ? no-capacity")
	tw := tabwriter.NewWriter(out, 0, 2, 1, ' ', 0)
	for _, r := range view.Resources {
		row := ""
		for _, d := range r.Days {
			row += heatGlyph(d)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", nameOrID(r.Name, r.ID), row, pct(r.AvgUtilization))
	}
	_ = tw.Flush()
}

func heatGlyph(d dayCell) string {
	if d.Utilization == nil {
		return "?"
	}
	u := *d.Utilization
	switch {
	case u <= 0:
		return "·"
	case u < 0.5:
		return "░"
	case u < 0.8:
		return "▒"
	case u < 1.0:
		return "▓"
	case u == 1.0:
		return "█"
	default:
		return "X"
	}
}
