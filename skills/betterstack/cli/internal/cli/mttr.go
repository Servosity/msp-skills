// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Novel feature (hand-built): incident MTTA/MTTR rollups from the local mirror.

package cli

import (
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"
)

type mttrReport struct {
	WindowDays      int          `json:"window_days"`
	IncidentsInWin  int          `json:"incidents_in_window"`
	AcknowledgedN   int          `json:"acknowledged_count"`
	ResolvedN       int          `json:"resolved_count"`
	MeanMTTASeconds float64      `json:"mean_mtta_seconds"`
	MedMTTASeconds  float64      `json:"median_mtta_seconds"`
	MeanMTTRSeconds float64      `json:"mean_mttr_seconds"`
	MedMTTRSeconds  float64      `json:"median_mttr_seconds"`
	MeanMTTA        string       `json:"mean_mtta"`
	MeanMTTR        string       `json:"mean_mttr"`
	ByMonitor       []mttrBucket `json:"by_monitor,omitempty"`
	Hint            string       `json:"hint,omitempty"`
}

type mttrBucket struct {
	Source       string  `json:"source"`
	Incidents    int     `json:"incidents"`
	MeanMTTRSecs float64 `json:"mean_mttr_seconds"`
	MeanMTTR     string  `json:"mean_mttr"`
}

// pp:data-source local
func newNovelMttrCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var days int
	var byMonitor bool
	var top int

	cmd := &cobra.Command{
		Use:         "mttr",
		Short:       "Mean time to acknowledge and resolve, computed across incidents over a window and broken down by monitor.",
		Long:        "Use this command for response-time rollups (how fast incidents are acknowledged/resolved). Do NOT use it to rank monitors by incident count; use 'flapping' instead. Computes MTTA (acknowledged_at − started_at) and MTTR (resolved_at − started_at) across incidents started within the window, from the local mirror. Run `sync` first.",
		Example:     "  betterstack-cli mttr --days 30\n  betterstack-cli mttr --days 30 --by-monitor --top 10 --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return nil
			}
			if days <= 0 {
				days = 30
			}
			s, err := openAnalyticsStore(cmd, dbPath)
			if err != nil {
				return err
			}
			defer s.Close()
			maybeEmitSyncHints(cmd, s, "incidents", flags.maxAge)

			incidents, err := loadIncidents(cmd.Context(), s)
			if err != nil {
				return fmt.Errorf("reading incidents: %w", err)
			}

			cutoff := time.Now().AddDate(0, 0, -days)
			var mttaVals, mttrVals []float64
			perSource := map[string][]float64{}
			inWindow := 0
			for _, inc := range incidents {
				started, ok := parseTime(inc.StartedAt)
				if !ok || started.Before(cutoff) {
					continue
				}
				inWindow++
				if ack, ok := parseTime(inc.AcknowledgedAt); ok {
					if d := ack.Sub(started).Seconds(); d >= 0 {
						mttaVals = append(mttaVals, d)
					}
				}
				if res, ok := parseTime(inc.ResolvedAt); ok {
					if d := res.Sub(started).Seconds(); d >= 0 {
						mttrVals = append(mttrVals, d)
						perSource[inc.Source] = append(perSource[inc.Source], d)
					}
				}
			}

			var rep mttrReport
			rep.WindowDays = days
			rep.IncidentsInWin = inWindow
			rep.AcknowledgedN = len(mttaVals)
			rep.ResolvedN = len(mttrVals)
			rep.MeanMTTASeconds = mean(mttaVals)
			rep.MedMTTASeconds = median(mttaVals)
			rep.MeanMTTRSeconds = mean(mttrVals)
			rep.MedMTTRSeconds = median(mttrVals)
			rep.MeanMTTA = humanizeSeconds(rep.MeanMTTASeconds)
			rep.MeanMTTR = humanizeSeconds(rep.MeanMTTRSeconds)
			if inWindow == 0 {
				rep.Hint = fmt.Sprintf("no incidents in the last %d days (or local mirror empty — run `sync`)", days)
			}

			if byMonitor {
				for src, vals := range perSource {
					rep.ByMonitor = append(rep.ByMonitor, mttrBucket{
						Source: src, Incidents: len(vals),
						MeanMTTRSecs: mean(vals), MeanMTTR: humanizeSeconds(mean(vals)),
					})
				}
				sort.SliceStable(rep.ByMonitor, func(i, j int) bool {
					if rep.ByMonitor[i].MeanMTTRSecs != rep.ByMonitor[j].MeanMTTRSecs {
						return rep.ByMonitor[i].MeanMTTRSecs > rep.ByMonitor[j].MeanMTTRSecs
					}
					return rep.ByMonitor[i].Source < rep.ByMonitor[j].Source
				})
				if top > 0 && len(rep.ByMonitor) > top {
					rep.ByMonitor = rep.ByMonitor[:top]
				}
			}

			if flags.asJSON {
				return flags.printJSON(cmd, rep)
			}
			if rep.Hint != "" {
				fmt.Fprintln(cmd.ErrOrStderr(), rep.Hint)
			}
			rows := [][]string{
				{"Window", fmt.Sprintf("last %d days", rep.WindowDays)},
				{"Incidents", fmt.Sprintf("%d", rep.IncidentsInWin)},
				{"MTTA (mean)", fmt.Sprintf("%s (n=%d)", rep.MeanMTTA, rep.AcknowledgedN)},
				{"MTTA (median)", humanizeSeconds(rep.MedMTTASeconds)},
				{"MTTR (mean)", fmt.Sprintf("%s (n=%d)", rep.MeanMTTR, rep.ResolvedN)},
				{"MTTR (median)", humanizeSeconds(rep.MedMTTRSeconds)},
			}
			if err := flags.printTable(cmd, []string{"METRIC", "VALUE"}, rows); err != nil {
				return err
			}
			if byMonitor && len(rep.ByMonitor) > 0 {
				fmt.Fprintln(cmd.OutOrStdout())
				brows := make([][]string, 0, len(rep.ByMonitor))
				for _, b := range rep.ByMonitor {
					brows = append(brows, []string{truncateField(b.Source, 44), fmt.Sprintf("%d", b.Incidents), b.MeanMTTR})
				}
				return flags.printTable(cmd, []string{"SOURCE", "INCIDENTS", "MEAN MTTR"}, brows)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/betterstack-cli/data.db)")
	cmd.Flags().IntVar(&days, "days", 30, "Window in days (incidents started within this many days back)")
	cmd.Flags().BoolVar(&byMonitor, "by-monitor", false, "Break MTTR down by source monitor")
	cmd.Flags().IntVar(&top, "top", 10, "With --by-monitor, show only the top N slowest-resolving sources")
	return cmd
}

func mean(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	var sum float64
	for _, x := range v {
		sum += x
	}
	return sum / float64(len(v))
}

func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	c := append([]float64(nil), v...)
	sort.Float64s(c)
	n := len(c)
	if n%2 == 1 {
		return c[n/2]
	}
	return (c[n/2-1] + c[n/2]) / 2
}

func humanizeSeconds(s float64) string {
	if s <= 0 {
		return "—"
	}
	d := time.Duration(s) * time.Second
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	default:
		return fmt.Sprintf("%dd%dh", int(d.Hours())/24, int(d.Hours())%24)
	}
}
