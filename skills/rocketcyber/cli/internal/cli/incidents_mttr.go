// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel feature command.
//
// pp:data-source local

package cli

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"rocketcyber-pp-cli/internal/cliutil"
	"rocketcyber-pp-cli/internal/store"
)

type mttrIncident struct {
	CreatedAt  time.Time
	ResolvedAt time.Time
	Resolved   bool
}

type mttrMonthly struct {
	Month         string  `json:"month"`
	ResolvedCount int     `json:"resolved_count"`
	MeanHours     float64 `json:"mean_hours"`
}

type mttrView struct {
	Since           string         `json:"since"`
	TotalIncidents  int            `json:"total_incidents"`
	ResolvedCount   int            `json:"resolved_count"`
	OpenCount       int            `json:"open_count"`
	MTTRHoursMean   float64        `json:"mttr_hours_mean"`
	MTTRHoursMedian float64        `json:"mttr_hours_median"`
	OpenAging       map[string]int `json:"open_aging"`
	Monthly         []mttrMonthly  `json:"monthly"`
	Note            string         `json:"note,omitempty"`
}

func round1(f float64) float64 { return math.Round(f*10) / 10 }

// computeMTTR aggregates resolution velocity over incidents created within
// the window: mean/median createdAt->resolvedAt hours, open-incident aging
// buckets, and a per-month resolved breakdown.
func computeMTTR(incidents []mttrIncident, now time.Time, window time.Duration, since string) mttrView {
	view := mttrView{
		Since:     since,
		OpenAging: map[string]int{"0-7d": 0, "7-30d": 0, "over-30d": 0},
		Monthly:   []mttrMonthly{},
	}
	cutoff := now.Add(-window)
	var resolvedHours []float64
	monthly := map[string][]float64{}
	for _, inc := range incidents {
		if inc.CreatedAt.IsZero() || inc.CreatedAt.Before(cutoff) {
			continue
		}
		view.TotalIncidents++
		if inc.Resolved && !inc.ResolvedAt.IsZero() && inc.ResolvedAt.After(inc.CreatedAt) {
			hours := inc.ResolvedAt.Sub(inc.CreatedAt).Hours()
			resolvedHours = append(resolvedHours, hours)
			month := inc.ResolvedAt.UTC().Format("2006-01")
			monthly[month] = append(monthly[month], hours)
			view.ResolvedCount++
			continue
		}
		view.OpenCount++
		age := now.Sub(inc.CreatedAt)
		switch {
		case age <= 7*24*time.Hour:
			view.OpenAging["0-7d"]++
		case age <= 30*24*time.Hour:
			view.OpenAging["7-30d"]++
		default:
			view.OpenAging["over-30d"]++
		}
	}
	if len(resolvedHours) > 0 {
		sort.Float64s(resolvedHours)
		var sum float64
		for _, h := range resolvedHours {
			sum += h
		}
		view.MTTRHoursMean = round1(sum / float64(len(resolvedHours)))
		mid := len(resolvedHours) / 2
		if len(resolvedHours)%2 == 1 {
			view.MTTRHoursMedian = round1(resolvedHours[mid])
		} else {
			view.MTTRHoursMedian = round1((resolvedHours[mid-1] + resolvedHours[mid]) / 2)
		}
	}
	months := make([]string, 0, len(monthly))
	for m := range monthly {
		months = append(months, m)
	}
	sort.Strings(months)
	for _, m := range months {
		var sum float64
		for _, h := range monthly[m] {
			sum += h
		}
		view.Monthly = append(view.Monthly, mttrMonthly{
			Month:         m,
			ResolvedCount: len(monthly[m]),
			MeanHours:     round1(sum / float64(len(monthly[m]))),
		})
	}
	return view
}

func newNovelIncidentsMttrCmd(flags *rootFlags) *cobra.Command {
	var flagSince string
	var flagDB string

	cmd := &cobra.Command{
		Use:   "mttr",
		Short: "Mean and median time-to-resolve plus open-incident aging buckets, computed from incident created/resolved timestamps.",
		Long: strings.Trim(`
Resolution-velocity math over synced incidents: mean and median
createdAt->resolvedAt hours, open-incident aging buckets (0-7d, 7-30d,
over-30d), and a per-month resolved breakdown.

Reads the local store - run 'rocketcyber-cli sync --resources incidents'
first. Use this for resolution-speed/SLA math over the incident history
(MTTR, aging buckets). Do NOT use it to list or read individual incidents;
use 'incidents' instead.
`, "\n"),
		Example: strings.Trim(`
  rocketcyber-cli incidents mttr --since 90d --json
  rocketcyber-cli incidents mttr --since 30d --agent
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would compute MTTR from synced incidents in the local store")
				return nil
			}
			if flagSince == "" {
				flagSince = "90d"
			}
			window, err := cliutil.ParseDurationLoose(flagSince)
			if err != nil {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("invalid --since value %q: %w", flagSince, err))
			}
			if flagDB == "" {
				flagDB = defaultDBPath("rocketcyber-cli")
			}
			db, err := store.OpenWithContext(cmd.Context(), flagDB)
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "incidents") {
				hintIfStale(cmd, db, "incidents", flags.maxAge)
			}
			rows, err := db.List("incidents", 10000)
			if err != nil {
				return fmt.Errorf("reading incidents from local store: %w", err)
			}
			incidents := make([]mttrIncident, 0, len(rows))
			for _, raw := range rows {
				var probe map[string]json.RawMessage
				if err := json.Unmarshal(raw, &probe); err != nil {
					continue
				}
				rec := mttrIncident{
					CreatedAt:  parseAPITime(extractString(probe, "createdAt")),
					ResolvedAt: parseAPITime(extractString(probe, "resolvedAt")),
				}
				rec.Resolved = strings.EqualFold(extractString(probe, "status"), "resolved") || !rec.ResolvedAt.IsZero()
				incidents = append(incidents, rec)
			}
			view := computeMTTR(incidents, time.Now().UTC(), window, flagSince)
			if view.TotalIncidents == 0 {
				view.Note = fmt.Sprintf("no incidents created within %s found in the local store; run 'rocketcyber-cli sync --resources incidents --full' or widen --since", flagSince)
			}
			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "90d", "Window of incident creation dates to include (e.g. 30d, 90d, 26w)")
	cmd.Flags().StringVar(&flagDB, "db", "", "Database path (default: local store)")
	return cmd
}
