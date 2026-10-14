// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source auto

package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"threatdown-pp-cli/internal/cliutil"
	"threatdown-pp-cli/internal/fleet"
)

type fleetStaleView struct {
	Source    string           `json:"source"`
	Hours     int              `json:"hours"`
	Sites     int              `json:"sites"`
	Scanned   int              `json:"scanned_endpoints"`
	Count     int              `json:"count"`
	Truncated bool             `json:"truncated"`
	Note      string           `json:"note,omitempty"`
	Endpoints []fleet.StaleRow `json:"endpoints"`
}

func newNovelFleetStaleCmd(flags *rootFlags) *cobra.Command {
	var hours, limit, maxPages int
	var siteFilter, newSince, dbPath string
	var twins bool

	cmd := &cobra.Command{
		Use:   "stale",
		Short: "Every endpoint not seen in N hours across all your sites, with site name and exact hours since last seen.",
		Long: `Use this command for an endpoint-by-endpoint list of machines not seen in N hours across every site. Do NOT use this command for per-site counts or a health scorecard; use 'fleet health' instead.

This is the ThreatDown "endpoints not seen for more than 24 hours" PowerShell
example as one fleet query: every site's account_id is resolved for you, the
2000-row search pages are followed, and hours since last seen come from the
newer of last_active and last_day_seen. Never-seen endpoints are included with
hours_since_seen = -1.`,
		Example: strings.Trim(`
  threatdown-cli fleet stale --hours 24 --agent
  threatdown-cli fleet stale --hours 72 --site "Acme" --new-since 7d
  threatdown-cli fleet stale --twins --json --select endpoints.host_name,endpoints.twin_of`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet stale")
			}
			if hours <= 0 {
				return usageErr(fmt.Errorf("--hours must be positive, got %d", hours))
			}
			opts := fleet.StaleOptions{Hours: hours, Twins: twins}
			if newSince != "" {
				d, err := cliutil.ParseDurationLoose(newSince)
				if err != nil || d <= 0 {
					return usageErr(fmt.Errorf("--new-since must be a duration like 7d, got %q", newSince))
				}
				opts.NewSince = d
			}
			if cliutil.IsDogfoodEnv() {
				maxPages = 1
			}
			ctx := cmd.Context()
			now := time.Now().UTC()

			var view fleetStaleView
			var err error
			mode := resolveDataSource(flags)
			if mode != "local" {
				view, err = fleetStaleLive(ctx, flags, opts, siteFilter, maxPages, now)
				if err != nil && (mode == "live" || stopFallback(err)) {
					return classifyAPIErrorOnly(err)
				}
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "live query failed (%v); falling back to the local store\n", err)
				}
			}
			if mode == "local" || err != nil {
				localView, ok, lerr := fleetStaleLocal(cmd, opts, siteFilter, tdDBPath(dbPath), now)
				if lerr != nil {
					return lerr
				}
				if !ok && err != nil {
					return classifyAPIErrorOnly(err)
				}
				view = localView
			}
			if limit > 0 && len(view.Endpoints) > limit {
				view.Endpoints = view.Endpoints[:limit]
				view.Truncated = true
			}
			view.Count = len(view.Endpoints)
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFilteredKeep(cmd.OutOrStdout(), view, flags, "twin_of", "twin_of_id", "newly_dark", "last_seen", "note")
			}
			if view.Note != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "note: %s\n", view.Note)
			}
			if view.Count == 0 {
				if view.Note == "" {
					fmt.Fprintf(cmd.OutOrStdout(), "No endpoints unseen for %dh across %d sites.\n", hours, view.Sites)
				}
				return nil
			}
			// newly_dark and twin_of are only computed with --new-since and
			// --twins; an always-present column would read as a negative result.
			cols := []string{"site", "host_name", "hours_since_seen", "last_seen"}
			if opts.NewSince > 0 {
				cols = append(cols, "newly_dark")
			}
			cols = append(cols, "isolated", "infection_count")
			if twins {
				cols = append(cols, "twin_of")
			}
			return printColumns(cmd.OutOrStdout(), view.Endpoints, cols...)
		},
	}
	cmd.Flags().IntVar(&hours, "hours", 24, "Report endpoints not seen for at least this many hours")
	cmd.Flags().StringVar(&siteFilter, "site", "", "Only sites whose name contains this text")
	cmd.Flags().StringVar(&newSince, "new-since", "", "Mark endpoints that went dark within this window as newly_dark (e.g. 7d)")
	cmd.Flags().BoolVar(&twins, "twins", false, "Mark stale records whose machine has a live twin (same serial, MAC or host name in the site): ghosts left by re-imaging")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum rows to return (0 = all)")
	cmd.Flags().IntVar(&maxPages, "max-scan-pages", tdDefaultMaxPages, "Maximum 2000-row search pages to scan")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path for --data-source local")
	return cmd
}

func fleetStaleLive(ctx context.Context, flags *rootFlags, opts fleet.StaleOptions, siteFilter string, maxPages int, now time.Time) (fleetStaleView, error) {
	c, err := flags.newClient()
	if err != nil {
		return fleetStaleView{}, err
	}
	sites, err := fetchSites(ctx, c)
	if err != nil {
		return fleetStaleView{}, err
	}
	if sites, err = scopeSites(sites, siteFilter); err != nil {
		return fleetStaleView{}, err
	}
	accounts := fleet.AccountIDs(sites, "")
	view := fleetStaleView{Source: "live", Hours: opts.Hours, Sites: len(sites), Endpoints: make([]fleet.StaleRow, 0)}
	if siteFilter != "" {
		view.Note = accountlessNote(sites)
	}
	if len(accounts) == 0 {
		if view.Note == "" {
			view.Note = "no sites with an account_id matched; nothing to scan"
		}
		return view, nil
	}
	cutoff := isoAgo(now, time.Duration(opts.Hours)*time.Hour)
	raws, info, err := searchPages(ctx, c, "/v1/endpoints", map[string]any{"account_ids": accounts, "last_day_seen_before": cutoff}, "endpoints", maxPages)
	more := info.Truncated
	if err != nil {
		return view, err
	}
	all, bad := parseEndpoints(raws)
	if opts.Twins {
		live, liveInfo, err := searchPages(ctx, c, "/v1/endpoints", map[string]any{"account_ids": accounts, "last_day_seen_after": cutoff}, "endpoints", maxPages)
		if err != nil {
			return view, err
		}
		liveEps, liveBad := parseEndpoints(live)
		all = append(all, liveEps...)
		bad += liveBad
		more = more || liveInfo.Truncated
	}
	view.Scanned = len(all)
	view.Truncated = more
	if more {
		view.Note = joinNotes(view.Note, "scan stopped at --max-scan-pages; raise it to cover the whole fleet")
	}
	if bad > 0 {
		view.Note = joinNotes(view.Note, fmt.Sprintf("%d endpoint record(s) could not be read and are not listed", bad))
	}
	view.Endpoints = fleet.Stale(all, fleet.SiteIndex(sites), now, opts)
	return view, nil
}

func fleetStaleLocal(cmd *cobra.Command, opts fleet.StaleOptions, siteFilter, dbPath string, now time.Time) (fleetStaleView, bool, error) {
	view := fleetStaleView{Source: "local", Hours: opts.Hours, Endpoints: make([]fleet.StaleRow, 0)}
	db, ok, err := openFleetStore(cmd, dbPath)
	if err != nil || !ok {
		view.Note = "no local mirror; run " + tdFleetSyncCommand
		return view, ok, err
	}
	defer db.Close()
	if !hintIfUnsynced(cmd, db, tdResSites) {
		hintIfStale(cmd, db, tdResSites, 24*time.Hour)
	}
	fleetHint(cmd, db, tdResEndpoints, 24*time.Hour)
	if state, err := readSyncHintState(db, tdResEndpoints); err == nil && !state.hasState {
		view.Note = "no endpoints synced yet; run " + tdFleetSyncCommand
	}
	sites, err := loadLocalSites(cmd.Context(), db)
	if err != nil {
		return view, true, err
	}
	if sites, err = scopeSites(sites, siteFilter); err != nil {
		return view, true, err
	}
	if siteFilter != "" {
		view.Note = joinNotes(view.Note, accountlessNote(sites))
	}
	all, err := loadLocalEndpoints(cmd.Context(), db)
	if err != nil {
		return view, true, err
	}
	keep := map[string]bool{}
	for _, a := range fleet.AccountIDs(sites, "") {
		keep[a] = true
	}
	scoped := all[:0:0]
	for _, e := range all {
		if siteFilter == "" || keep[e.AccountID] {
			scoped = append(scoped, e)
		}
	}
	view.Sites, view.Scanned = len(sites), len(scoped)
	view.Endpoints = fleet.Stale(scoped, fleet.SiteIndex(sites), now, opts)
	return view, true, nil
}
