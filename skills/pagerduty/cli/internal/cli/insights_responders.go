// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// insights responders: per-responder acknowledge / resolve / page counts plus
// the share of pages that landed off-hours (nights and weekends), aggregated
// from the synced log entries. The fairness/burnout signal no single API call
// returns. Off-hours is classified in each timestamp's own location: a page is
// off-hours if it lands on a weekend or before 08:00 / at-or-after 20:00.
package cli

import (
	"fmt"
	"io"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

type pdResponder struct {
	UserID        string  `json:"user_id"`
	User          string  `json:"user"`
	Acks          int     `json:"acks"`
	Resolves      int     `json:"resolves"`
	Pages         int     `json:"pages"`
	OffHoursPages int     `json:"off_hours_pages"`
	OffHoursShare float64 `json:"off_hours_share"`
}

type pdRespondersResult struct {
	Window struct {
		Since string `json:"since"`
		Until string `json:"until"`
	} `json:"window"`
	Responders []pdResponder `json:"responders"`
}

// pp:data-source local
func newNovelInsightsRespondersCmd(flags *rootFlags) *cobra.Command {
	var flagSince, flagUntil string

	cmd := &cobra.Command{
		Use:         "responders",
		Short:       "Per-responder ack/resolve/page counts and off-hours page share over a window",
		Long:        "Aggregates synced log entries into per-responder acknowledge, resolve, and page counts plus the share of pages that landed off-hours (nights/weekends) — the on-call fairness and burnout signal. --since/--until accept a relative duration (30d, 24h) or RFC3339. Run `sync` first; exits 0 with an empty result when nothing is synced.",
		Example:     "  pagerduty-cli insights responders --since 30d\n  pagerduty-cli insights responders --agent\n  pagerduty-cli insights responders --since 7d --until 1d",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			pdHintSync(cmd, flags, "log-entries")
			win, err := pdParseWindow(flagSince, flagUntil, time.Now())
			if err != nil {
				return err
			}
			logs, err := pdLoadResource(cmd.Context(), "log_entries")
			if err != nil {
				return fmt.Errorf("reading log_entries from local store: %w", err)
			}
			res := buildResponders(logs, win)
			return pdEmit(cmd, flags, res, func(w io.Writer) {
				if len(res.Responders) == 0 {
					fmt.Fprintln(w, "No responder activity in the window (run `pagerduty-cli sync` first).")
					return
				}
				fmt.Fprintf(w, "Responder workload %s → %s\n\n", res.Window.Since, res.Window.Until)
				tw := tabwriter.NewWriter(w, 2, 4, 2, ' ', 0)
				fmt.Fprintln(tw, "RESPONDER\tACKS\tRESOLVES\tPAGES\tOFF-HOURS%")
				for _, r := range res.Responders {
					fmt.Fprintf(tw, "%s\t%d\t%d\t%d\t%.0f%%\n", r.User, r.Acks, r.Resolves, r.Pages, r.OffHoursShare*100)
				}
				_ = tw.Flush()
			})
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "", "Window start: relative (30d, 24h) or RFC3339 (default 30 days ago)")
	cmd.Flags().StringVar(&flagUntil, "until", "", "Window end: relative or RFC3339 (default now)")
	return cmd
}

// buildResponders is the pure split-out for tests.
func buildResponders(logs []map[string]any, win pdWindow) pdRespondersResult {
	var res pdRespondersResult
	res.Window.Since = win.Since.UTC().Format(time.RFC3339)
	res.Window.Until = win.Until.UTC().Format(time.RFC3339)
	res.Responders = []pdResponder{}

	byUser := map[string]*pdResponder{}
	order := []string{}
	get := func(ref map[string]any) *pdResponder {
		name := pdRefLabel(ref)
		if name == "" {
			return nil
		}
		id := pdString(ref, "id")
		key := id
		if key == "" {
			key = name
		}
		r := byUser[key]
		if r == nil {
			r = &pdResponder{UserID: id, User: name}
			byUser[key] = r
			order = append(order, key)
		}
		return r
	}

	for _, le := range logs {
		t, ok := pdParseTime(pdString(le, "created_at"))
		if !ok || !win.contains(t) {
			continue
		}
		switch pdString(le, "type") {
		case "acknowledge_log_entry":
			if r := get(pdMap(le, "agent")); r != nil {
				r.Acks++
			}
		case "resolve_log_entry":
			agent := pdMap(le, "agent")
			if pdString(agent, "type") == "user_reference" || pdString(agent, "type") == "user" {
				if r := get(agent); r != nil {
					r.Resolves++
				}
			}
		case "notify_log_entry":
			if r := get(pdMap(le, "user")); r != nil {
				r.Pages++
				if pdIsOffHours(t) {
					r.OffHoursPages++
				}
			}
		}
	}

	for _, key := range order {
		r := byUser[key]
		if r.Pages > 0 {
			r.OffHoursShare = pdRound2(float64(r.OffHoursPages) / float64(r.Pages))
		}
		res.Responders = append(res.Responders, *r)
	}
	sort.SliceStable(res.Responders, func(i, j int) bool {
		li := res.Responders[i].Acks + res.Responders[i].Resolves
		lj := res.Responders[j].Acks + res.Responders[j].Resolves
		if li != lj {
			return li > lj
		}
		return res.Responders[i].Pages > res.Responders[j].Pages
	})
	return res
}
