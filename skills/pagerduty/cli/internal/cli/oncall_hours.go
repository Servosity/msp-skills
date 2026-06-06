// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// oncall hours: on-call hours per user over a time window, computed from the
// synced on-call entries (each entry's [start,end) interval clipped to the
// window). Reproduces the gist of PagerDuty's paid on-call-compensation report
// offline — useful for monthly fairness reviews and MSP billing.
package cli

import (
	"fmt"
	"io"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

type pdOncallHoursUser struct {
	UserID string  `json:"user_id"`
	User   string  `json:"user"`
	Hours  float64 `json:"hours"`
	Shifts int     `json:"shifts"`
}

type pdOncallHoursResult struct {
	Window struct {
		Since string `json:"since"`
		Until string `json:"until"`
	} `json:"window"`
	TotalHours float64             `json:"total_hours"`
	Users      []pdOncallHoursUser `json:"users"`
}

// pp:data-source local
func newNovelOncallHoursCmd(flags *rootFlags) *cobra.Command {
	var flagSince, flagUntil, flagUser string

	cmd := &cobra.Command{
		Use:         "hours",
		Short:       "On-call hours per user over a time window, from synced on-call entries",
		Long:        "Computes on-call hours per user over a window by clipping each synced on-call interval to [since, until]. --since/--until accept a relative duration (e.g. 30d, 24h) or an RFC3339 timestamp; --since defaults to 30 days ago and --until to now. Run `sync` first; exits 0 with an empty result when nothing is synced.",
		Example:     "  pagerduty-cli oncall hours --since 30d\n  pagerduty-cli oncall hours --since 7d --agent\n  pagerduty-cli oncall hours --user PUSER01 --since 90d",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			pdHintSync(cmd, flags, "oncalls")
			win, err := pdParseWindow(flagSince, flagUntil, time.Now())
			if err != nil {
				return err
			}
			oncalls, err := pdLoadResource(cmd.Context(), "oncalls")
			if err != nil {
				return fmt.Errorf("reading oncalls from local store: %w", err)
			}
			res := buildOncallHours(oncalls, win, flagUser)
			return pdEmit(cmd, flags, res, func(w io.Writer) {
				if len(res.Users) == 0 {
					fmt.Fprintln(w, "No on-call hours in the window (run `pagerduty-cli sync` first, and sync a time range with --since/--until to populate rotations).")
					return
				}
				fmt.Fprintf(w, "On-call hours %s → %s (total %.1fh)\n\n", res.Window.Since, res.Window.Until, res.TotalHours)
				tw := tabwriter.NewWriter(w, 2, 4, 2, ' ', 0)
				fmt.Fprintln(tw, "USER\tHOURS\tSHIFTS")
				for _, u := range res.Users {
					fmt.Fprintf(tw, "%s\t%.1f\t%d\n", u.User, u.Hours, u.Shifts)
				}
				_ = tw.Flush()
			})
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "", "Window start: relative (30d, 24h) or RFC3339 (default 30 days ago)")
	cmd.Flags().StringVar(&flagUntil, "until", "", "Window end: relative or RFC3339 (default now)")
	cmd.Flags().StringVar(&flagUser, "user", "", "Limit to a single user ID or name")
	return cmd
}

// buildOncallHours is the pure split-out for tests.
func buildOncallHours(oncalls []map[string]any, win pdWindow, userFilter string) pdOncallHoursResult {
	var res pdOncallHoursResult
	res.Window.Since = win.Since.UTC().Format(time.RFC3339)
	res.Window.Until = win.Until.UTC().Format(time.RFC3339)
	res.Users = []pdOncallHoursUser{}

	type acc struct {
		u     pdOncallHoursUser
		hours float64
	}
	byUser := map[string]*acc{}
	order := []string{}

	for _, oc := range oncalls {
		userRef := pdMap(oc, "user")
		uid := pdString(userRef, "id")
		uname := pdRefLabel(userRef)
		if uname == "" {
			continue
		}
		if userFilter != "" && uid != userFilter && uname != userFilter {
			continue
		}
		start, hasStart := pdParseTime(pdString(oc, "start"))
		end, hasEnd := pdParseTime(pdString(oc, "end"))
		// Clip the interval to the window; treat open ends as the window bounds.
		if !hasStart || start.Before(win.Since) {
			start = win.Since
		}
		if !hasEnd || end.After(win.Until) {
			end = win.Until
		}
		if !end.After(start) {
			continue // no overlap with the window
		}
		key := uid
		if key == "" {
			key = uname
		}
		a := byUser[key]
		if a == nil {
			a = &acc{u: pdOncallHoursUser{UserID: uid, User: uname}}
			byUser[key] = a
			order = append(order, key)
		}
		a.hours += end.Sub(start).Hours()
		a.u.Shifts++
	}

	for _, key := range order {
		a := byUser[key]
		a.u.Hours = pdRound2(a.hours)
		res.TotalHours += a.u.Hours
		res.Users = append(res.Users, a.u)
	}
	res.TotalHours = pdRound2(res.TotalHours)
	sort.SliceStable(res.Users, func(i, j int) bool {
		return res.Users[i].Hours > res.Users[j].Hours
	})
	return res
}
