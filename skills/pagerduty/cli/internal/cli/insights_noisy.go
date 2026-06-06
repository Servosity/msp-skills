// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// insights noisy: ranks services by incident volume over a window, with their
// high-urgency share, auto-resolve rate, and reopened (re-triggered) count.
// Surfaces the services driving alert fatigue — a service leaderboard the API's
// per-incident outlier endpoint does not provide.
package cli

import (
	"fmt"
	"io"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

type pdNoisyService struct {
	Service         string  `json:"service"`
	ServiceID       string  `json:"service_id"`
	Incidents       int     `json:"incidents"`
	HighUrgency     int     `json:"high_urgency"`
	AutoResolved    int     `json:"auto_resolved"`
	AutoResolveRate float64 `json:"auto_resolve_rate"`
	Reopened        int     `json:"reopened"`
}

type pdNoisyResult struct {
	Window struct {
		Since string `json:"since"`
		Until string `json:"until"`
	} `json:"window"`
	Services []pdNoisyService `json:"services"`
}

// pp:data-source local
func newNovelInsightsNoisyCmd(flags *rootFlags) *cobra.Command {
	var flagTop int
	var flagSince, flagUntil string

	cmd := &cobra.Command{
		Use:         "noisy",
		Short:       "Rank services by incident volume, auto-resolve rate, and reopened count over a window",
		Long:        "Ranks services by incident volume in the window and reports each one's high-urgency share, auto-resolve rate (incidents resolved by the service rather than a person), and reopened (re-triggered) count — the noise signals that drive alert fatigue. --since/--until accept a relative duration (7d, 24h) or RFC3339. Run `sync` first; exits 0 with an empty result when nothing is synced.",
		Example:     "  pagerduty-cli insights noisy --top 10 --since 7d\n  pagerduty-cli insights noisy --agent\n  pagerduty-cli insights noisy --since 30d",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			pdHintSync(cmd, flags, "incidents")
			win, err := pdParseWindow(flagSince, flagUntil, time.Now())
			if err != nil {
				return err
			}
			incidents, err := pdLoadResource(cmd.Context(), "incidents")
			if err != nil {
				return fmt.Errorf("reading incidents from local store: %w", err)
			}
			logs, err := pdLoadResource(cmd.Context(), "log_entries")
			if err != nil {
				return fmt.Errorf("reading log_entries from local store: %w", err)
			}
			res := buildNoisy(incidents, logs, win, flagTop)
			return pdEmit(cmd, flags, res, func(w io.Writer) {
				if len(res.Services) == 0 {
					fmt.Fprintln(w, "No incidents in the window (run `pagerduty-cli sync` first).")
					return
				}
				fmt.Fprintf(w, "Noisiest services %s → %s\n\n", res.Window.Since, res.Window.Until)
				tw := tabwriter.NewWriter(w, 2, 4, 2, ' ', 0)
				fmt.Fprintln(tw, "SERVICE\tINCIDENTS\tHIGH\tAUTO-RESOLVED\tAUTO%\tREOPENED")
				for _, s := range res.Services {
					fmt.Fprintf(tw, "%s\t%d\t%d\t%d\t%.0f%%\t%d\n", s.Service, s.Incidents, s.HighUrgency, s.AutoResolved, s.AutoResolveRate*100, s.Reopened)
				}
				_ = tw.Flush()
			})
		},
	}
	cmd.Flags().IntVar(&flagTop, "top", 10, "Show the top N noisiest services (0 = all)")
	cmd.Flags().StringVar(&flagSince, "since", "", "Window start: relative (7d, 24h) or RFC3339 (default 30 days ago)")
	cmd.Flags().StringVar(&flagUntil, "until", "", "Window end: relative or RFC3339 (default now)")
	return cmd
}

// buildNoisy is the pure split-out for tests.
func buildNoisy(incidents, logs []map[string]any, win pdWindow, top int) pdNoisyResult {
	var res pdNoisyResult
	res.Window.Since = win.Since.UTC().Format(time.RFC3339)
	res.Window.Until = win.Until.UTC().Format(time.RFC3339)
	res.Services = []pdNoisyService{}

	byService := map[string]*pdNoisyService{}
	order := []string{}

	for _, lc := range pdBuildLifecycles(incidents, logs) {
		if !lc.hasTrigger || !win.contains(lc.TriggerAt) {
			continue
		}
		name := lc.Service
		if name == "" {
			name = "(unknown service)"
		}
		key := lc.ServiceID
		if key == "" {
			key = name
		}
		s := byService[key]
		if s == nil {
			s = &pdNoisyService{Service: name, ServiceID: lc.ServiceID}
			byService[key] = s
			order = append(order, key)
		}
		s.Incidents++
		if lc.Urgency == "high" {
			s.HighUrgency++
		}
		if lc.autoResolved {
			s.AutoResolved++
		}
		if lc.triggerCount > 1 {
			s.Reopened++
		}
	}

	for _, key := range order {
		s := byService[key]
		if s.Incidents > 0 {
			s.AutoResolveRate = pdRound2(float64(s.AutoResolved) / float64(s.Incidents))
		}
		res.Services = append(res.Services, *s)
	}
	sort.SliceStable(res.Services, func(i, j int) bool {
		if res.Services[i].Incidents != res.Services[j].Incidents {
			return res.Services[i].Incidents > res.Services[j].Incidents
		}
		return res.Services[i].HighUrgency > res.Services[j].HighUrgency
	})
	if top > 0 && len(res.Services) > top {
		res.Services = res.Services[:top]
	}
	return res
}
