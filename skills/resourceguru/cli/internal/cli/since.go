// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// since: bookings created or updated within a recent window, from the local
// store. Hand-authored novel command.

package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"text/tabwriter"
	"time"

	"resourceguru-pp-cli/internal/cliutil"

	"github.com/spf13/cobra"
)

type sinceBooking struct {
	ID         json.Number `json:"id"`
	ResourceID json.Number `json:"resource_id"`
	StartDate  string      `json:"start_date"`
	EndDate    string      `json:"end_date"`
	Details    string      `json:"details"`
	ProjectID  json.Number `json:"project_id"`
	ClientID   json.Number `json:"client_id"`
	CreatedAt  string      `json:"created_at"`
	UpdatedAt  string      `json:"updated_at"`
}

type sinceView struct {
	Window   string         `json:"window"`
	Since    string         `json:"since"`
	Count    int            `json:"count"`
	Bookings []sinceBooking `json:"bookings"`
}

func newNovelSinceCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "since [window]",
		Short: "Bookings created or updated within a recent window (default 7d), from the local store.",
		Long: "Surface bookings whose created_at or updated_at falls within the given window — a quick " +
			"'what moved on the schedule' feed over the local store. Window accepts Go durations plus " +
			"d/w shorthand (e.g. 48h, 7d, 2w). Run `sync` first.",
		Example:     "  resourceguru-cli since 7d --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Args:        cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			window := "7d"
			if len(args) == 1 {
				window = args[0]
			}
			dur, err := cliutil.ParseDurationLoose(window)
			if err != nil || dur <= 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("invalid window %q: use a duration like 48h, 7d, or 2w", window))
			}
			cutoff := time.Now().UTC().Add(-dur)
			s, err := openUtilStore(cmd, dbPath)
			if err != nil {
				return fmt.Errorf("opening local store: %w", err)
			}
			defer s.Close()
			raws, err := s.List("bookings", 0)
			if err != nil {
				return fmt.Errorf("reading bookings: %w", err)
			}
			rows := make([]sinceBooking, 0)
			for _, raw := range raws {
				var b sinceBooking
				if err := json.Unmarshal(raw, &b); err != nil {
					continue
				}
				ts := mostRecent(b.UpdatedAt, b.CreatedAt)
				if !ts.IsZero() && !ts.Before(cutoff) {
					rows = append(rows, b)
				}
			}
			sort.Slice(rows, func(i, j int) bool {
				return mostRecent(rows[i].UpdatedAt, rows[i].CreatedAt).After(mostRecent(rows[j].UpdatedAt, rows[j].CreatedAt))
			})
			view := sinceView{Window: window, Since: cutoff.Format(time.RFC3339), Count: len(rows), Bookings: rows}
			out := cmd.OutOrStdout()
			if flags.asJSON || flags.csv || flags.quiet || flags.plain || !isTerminal(out) {
				return printJSONFiltered(out, view, flags)
			}
			if len(raws) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "note: no bookings in the local store — run `resourceguru-cli sync` first")
			}
			if len(rows) == 0 {
				fmt.Fprintf(out, "No bookings changed in the last %s.\n", window)
				return nil
			}
			tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
			fmt.Fprintf(out, "Bookings changed in the last %s (%d)\n", window, view.Count)
			fmt.Fprintln(tw, "BOOKING\tRESOURCE\tSTART\tEND\tUPDATED")
			for _, b := range rows {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
					b.ID.String(), b.ResourceID.String(), b.StartDate, b.EndDate,
					mostRecent(b.UpdatedAt, b.CreatedAt).Format("2006-01-02 15:04"))
			}
			return tw.Flush()
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Local store path (default: standard cache location)")
	return cmd
}

// parseRGTime parses Resource Guru timestamps, tolerating RFC3339 and a plain
// space-separated form.
func parseRGTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05 -0700", "2006-01-02 15:04:05", utilDateFmt} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

// mostRecent returns the later of two timestamps (zero-safe).
func mostRecent(a, b string) time.Time {
	ta, tb := parseRGTime(a), parseRGTime(b)
	if ta.After(tb) {
		return ta
	}
	return tb
}
