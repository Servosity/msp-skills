// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel feature. See internal/cli/itglue_records.go for shared helpers.

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// pp:data-source local
func newNovelChangesCmd(flags *rootFlags) *cobra.Command {
	var flagSince string
	var flagResource string
	var flagLimit int

	cmd := &cobra.Command{
		Use:   "changes",
		Short: "Show every record updated since a timestamp across all five resources, newest first",
		Long: `Show records updated since a timestamp across organizations, contacts,
passwords, configurations, and documents — newest first — from the local store.

The IT Glue API exposes recency one resource at a time; this unions updated-at
across every synced resource in a single offline query. Narrow to one resource
with --resource to get a single-resource staleness view.

Use this command for "what moved across the whole tenant since timestamp T" — a
unified recency feed for reconciliation. Do NOT use it for the credential-specific
rotation audit; use 'passwords stale' instead. Do NOT use it to look up a specific
identifier; use 'search' instead.`,
		Example: `  # Everything changed in the last 7 days
  itglue-cli changes --since 7d

  # Configurations changed since a date, as JSON
  itglue-cli changes --since 2026-05-01 --resource configurations --agent

  # Most recent changes across the fleet (no window)
  itglue-cli changes --limit 50`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}

			since, err := parseSinceArg(flagSince, time.Now())
			if err != nil {
				return usageErr(err)
			}

			resources := itgResourceTypes
			if flagResource != "" {
				valid := false
				for _, rt := range itgResourceTypes {
					if rt == flagResource {
						valid = true
						break
					}
				}
				if !valid {
					return usageErr(fmt.Errorf("invalid --resource %q: choose one of %s", flagResource, strings.Join(itgResourceTypes, ", ")))
				}
				resources = []string{flagResource}
			}

			db, err := openITGStore(cmd)
			if err != nil {
				return configErr(err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "") {
				hintIfStale(cmd, db, "", flags.maxAge)
			}

			type changeRow struct {
				summary map[string]any
				ts      time.Time
				hasTS   bool
			}
			var rows []changeRow
			for _, rt := range resources {
				recs, err := listITGRecords(db, rt)
				if err != nil {
					return apiErr(fmt.Errorf("reading %s: %w", rt, err))
				}
				for _, rec := range recs {
					ts, ok := rec.updatedAt()
					if !since.IsZero() {
						// A window was requested: drop records we can't place
						// at or after it.
						if !ok || ts.Before(since) {
							continue
						}
					}
					rows = append(rows, changeRow{summary: rec.summary(), ts: ts, hasTS: ok})
				}
			}

			sort.SliceStable(rows, func(i, j int) bool {
				if rows[i].hasTS != rows[j].hasTS {
					return rows[i].hasTS // timestamped records first
				}
				return rows[i].ts.After(rows[j].ts) // newest first
			})

			if flagLimit > 0 && len(rows) > flagLimit {
				rows = rows[:flagLimit]
			}

			out := make([]map[string]any, 0, len(rows))
			for _, r := range rows {
				out = append(out, r.summary)
			}
			return printJSONFiltered(cmd.OutOrStdout(), out, flags)
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "", "Only records updated at/after this time: a date (2006-01-02), RFC3339 timestamp, or window like 7d/24h (empty = no window)")
	cmd.Flags().StringVar(&flagResource, "resource", "", "Limit to one resource: organizations|contacts|passwords|configurations|documents")
	cmd.Flags().IntVar(&flagLimit, "limit", 100, "Maximum rows to return (0 = no limit)")
	return cmd
}
