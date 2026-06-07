// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type sinceChange struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Change string `json:"change"`
	When   string `json:"when"`
}

type sinceReport struct {
	Window   string        `json:"window"`
	Since    string        `json:"since"`
	New      int           `json:"new"`
	Modified int           `json:"modified"`
	Changes  []sinceChange `json:"changes"`
	Note     string        `json:"note,omitempty"`
}

// parseWindow parses a duration like "4h", "2d", "90m", "1w". Bare numbers are
// treated as hours. Returns the lookback duration.
func parseWindow(s string) (time.Duration, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 24 * time.Hour, nil
	}
	if strings.HasSuffix(s, "d") {
		n, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return 0, fmt.Errorf("invalid window %q: expected like 2d, 4h, 90m", s)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	if strings.HasSuffix(s, "w") {
		n, err := strconv.Atoi(strings.TrimSuffix(s, "w"))
		if err != nil {
			return 0, fmt.Errorf("invalid window %q: expected like 1w, 4h", s)
		}
		return time.Duration(n) * 7 * 24 * time.Hour, nil
	}
	if n, err := strconv.Atoi(s); err == nil {
		return time.Duration(n) * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid window %q: expected like 2d, 4h, 90m", s)
	}
	return d, nil
}

// newNovelSinceCmd implements the "since" transcendence command: documents
// created or modified within a recent time window.
// pp:data-source local
func newNovelSinceCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var limit int
	cmd := &cobra.Command{
		Use:         "since [window]",
		Short:       "Show what changed in the last N hours — new documents and status transitions.",
		Long:        "Show what changed in the last window: documents created and documents modified within the lookback (default 24h). Accepts windows like 4h, 90m, 2d, 1w.\n\nReads the local store — run `sync` first. A time-windowed change view across the whole corpus.",
		Example:     "  pandadoc-cli since 4h --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			window := "24h"
			if len(args) > 0 {
				window = args[0]
			}
			dur, err := parseWindow(window)
			if err != nil {
				return err
			}
			if dbPath == "" {
				dbPath = defaultDBPath("pandadoc-cli")
			}
			docs, err := loadDocumentsHinted(cmd, flags, dbPath)
			if err != nil {
				return err
			}

			cutoff := time.Now().UTC().Add(-dur)
			report := sinceReport{Window: window, Since: cutoff.Format(time.RFC3339), Changes: make([]sinceChange, 0)}
			for _, d := range docs {
				switch {
				case !d.DateCreated.IsZero() && d.DateCreated.After(cutoff):
					report.New++
					report.Changes = append(report.Changes, sinceChange{
						ID: d.ID, Name: d.Name, Status: shortStatus(d.Status),
						Change: "created", When: d.DateCreated.Format(time.RFC3339),
					})
				case !d.DateModified.IsZero() && d.DateModified.After(cutoff):
					report.Modified++
					report.Changes = append(report.Changes, sinceChange{
						ID: d.ID, Name: d.Name, Status: shortStatus(d.Status),
						Change: "modified", When: d.DateModified.Format(time.RFC3339),
					})
				}
			}
			sort.Slice(report.Changes, func(i, j int) bool { return report.Changes[i].When > report.Changes[j].When })
			if limit > 0 && len(report.Changes) > limit {
				report.Changes = report.Changes[:limit]
			}
			if len(docs) == 0 {
				report.Note = "no documents in local store — run `pandadoc-cli sync` first"
			}

			w := cmd.OutOrStdout()
			if flags.asJSON || flags.agent || !isTerminal(w) {
				return printJSONFiltered(w, report, flags)
			}
			fmt.Fprintf(w, "Changes in last %s — %d new, %d modified\n\n", window, report.New, report.Modified)
			for _, c := range report.Changes {
				fmt.Fprintf(w, "  %-9s %-12s %s  %s\n", c.Change, c.Status, c.ID, c.Name)
			}
			if report.Note != "" {
				fmt.Fprintf(w, "\n  %s\n", report.Note)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (defaults to standard location)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum changes to return (0 = no limit)")
	return cmd
}
