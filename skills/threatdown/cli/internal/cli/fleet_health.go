// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source auto

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"threatdown-pp-cli/internal/client"
	"threatdown-pp-cli/internal/fleet"
)

type fetchFailure struct {
	Item  string `json:"item"`
	Error string `json:"error"`
}

type fleetHealthView struct {
	Source        string            `json:"source"`
	Note          string            `json:"note,omitempty"`
	StaleHours    int               `json:"stale_hours"`
	ScanDays      int               `json:"scan_days"`
	Sites         []fleet.HealthRow `json:"sites"`
	FetchFailures []fetchFailure    `json:"fetch_failures,omitempty"`
}

// healthCondition is one server-side count: the endpoint search filter that
// selects it and the HealthRow column it fills.
type healthCondition struct {
	name   string
	filter map[string]any
	set    func(*fleet.HealthRow, int)
}

func healthConditions(now time.Time, staleHours, scanDays int) []healthCondition {
	return []healthCondition{
		{"total", map[string]any{}, func(r *fleet.HealthRow, n int) { r.Total = n }},
		{"online", map[string]any{"connected": true}, func(r *fleet.HealthRow, n int) { r.Online = n }},
		{"stale", map[string]any{"last_day_seen_before": isoAgo(now, time.Duration(staleHours)*time.Hour)}, func(r *fleet.HealthRow, n int) { r.Stale = n }},
		{"infected", map[string]any{"infection_found": true}, func(r *fleet.HealthRow, n int) { r.Infected = n }},
		{"suspicious", map[string]any{"suspicious_activity_found": true}, func(r *fleet.HealthRow, n int) { r.Suspicious = n }},
		{"isolated", map[string]any{"is_isolated": true}, func(r *fleet.HealthRow, n int) { r.Isolated = n }},
		{"reboot_required", map[string]any{"reboot_required": true}, func(r *fleet.HealthRow, n int) { r.RebootRequired = n }},
		{"unprotected", map[string]any{"protection_status": "unprotected"}, func(r *fleet.HealthRow, n int) { r.Unprotected = n }},
		{"scan_overdue", map[string]any{"last_scan_before": isoAgo(now, time.Duration(scanDays)*24*time.Hour)}, func(r *fleet.HealthRow, n int) { r.ScanOverdue = n }},
		{"update_available", map[string]any{"is_software_update_available": true}, func(r *fleet.HealthRow, n int) { r.UpdateAvailable = n }},
	}
}

func newNovelFleetHealthCmd(flags *rootFlags) *cobra.Command {
	var staleHours, scanDays int
	var siteFilter, dbPath string

	cmd := &cobra.Command{
		Use:   "health",
		Short: "One ranked row per site with total, online, stale, infected, suspicious, isolated, reboot-required, unprotected, scan-overdue and update-available counts.",
		Long: `Use this command for one row per site with stale, infected, isolated, reboot-required and scan-overdue counts across all sites. Do NOT use this command for a full profile of one customer; use 'customer report' instead.

Live mode asks OneView for each count with one endpoint search-groupby per
condition (grouped by account_id), so it never pages individual endpoints.
Rows are ranked by attention_score: infected x10, suspicious x8, unprotected
x6, isolated x4, stale x3, reboot x2, scan-overdue and update-available x1.`,
		Example: strings.Trim(`
  threatdown-cli fleet health --agent
  threatdown-cli fleet health --stale-hours 72 --scan-days 14
  threatdown-cli fleet health --data-source local --json --select sites.site,sites.attention_score`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet health")
			}
			if staleHours <= 0 || scanDays <= 0 {
				return usageErr(fmt.Errorf("--stale-hours and --scan-days must be positive"))
			}
			ctx := cmd.Context()
			now := time.Now().UTC()

			var view fleetHealthView
			var err error
			mode := resolveDataSource(flags)
			if mode != "local" {
				view, err = fleetHealthLive(ctx, flags, siteFilter, staleHours, scanDays, now)
				if err != nil && (mode == "live" || stopFallback(err)) {
					return classifyAPIErrorOnly(err)
				}
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "live query failed (%v); falling back to the local store\n", err)
				}
			}
			if mode == "local" || err != nil {
				localView, ok, lerr := fleetHealthLocal(cmd, siteFilter, staleHours, scanDays, tdDBPath(dbPath), now)
				if lerr != nil {
					return lerr
				}
				if !ok && err != nil {
					return classifyAPIErrorOnly(err)
				}
				view = localView
			}
			if len(view.FetchFailures) > 0 {
				names := make([]string, 0, len(view.FetchFailures))
				for _, f := range view.FetchFailures {
					names = append(names, f.Item)
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of %d counts failed (%s); those columns read 0 and attention scores exclude them\n", len(view.FetchFailures), len(healthConditions(now, 1, 1)), strings.Join(names, ", "))
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFilteredKeep(cmd.OutOrStdout(), view, flags, "fetch_failures", "note")
			}
			if view.Note != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "note: %s\n", view.Note)
			}
			if len(view.Sites) == 0 {
				if view.Note == "" {
					fmt.Fprintln(cmd.OutOrStdout(), "No sites with an account_id found.")
				}
				return nil
			}
			return printColumns(cmd.OutOrStdout(), view.Sites, "site", "attention_score", "total", "online", "stale", "infected", "suspicious", "isolated", "reboot_required", "unprotected", "scan_overdue", "update_available")
		},
	}
	cmd.Flags().IntVar(&staleHours, "stale-hours", 24, "Count endpoints not seen for this many hours as stale")
	cmd.Flags().IntVar(&scanDays, "scan-days", 7, "Count endpoints not scanned for this many days as scan-overdue")
	cmd.Flags().StringVar(&siteFilter, "site", "", "Only sites whose name contains this text")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path for --data-source local")
	return cmd
}

func fleetHealthLive(ctx context.Context, flags *rootFlags, siteFilter string, staleHours, scanDays int, now time.Time) (fleetHealthView, error) {
	view := fleetHealthView{Source: "live", StaleHours: staleHours, ScanDays: scanDays, Sites: make([]fleet.HealthRow, 0)}
	c, err := flags.newClient()
	if err != nil {
		return view, err
	}
	sites, err := fetchSites(ctx, c)
	if err != nil {
		return view, err
	}
	if sites, err = scopeSites(sites, siteFilter); err != nil {
		return view, err
	}
	view.Note = accountlessNote(sites)
	accounts := fleet.AccountIDs(sites, "")
	if len(accounts) == 0 {
		return view, nil
	}
	rows := map[string]*fleet.HealthRow{}
	order := make([]string, 0, len(accounts))
	for _, s := range sites {
		if s.AccountID != "" {
			rows[s.AccountID] = &fleet.HealthRow{Site: s.Name, AccountID: s.AccountID}
			order = append(order, s.AccountID)
		}
	}
	succeeded := 0
	for _, cond := range healthConditions(now, staleHours, scanDays) {
		counts, err := countByAccount(ctx, c, accounts, cond.filter)
		if err != nil && stopFallback(err) {
			return view, err
		}
		if err != nil {
			view.FetchFailures = append(view.FetchFailures, fetchFailure{Item: cond.name, Error: err.Error()})
			continue
		}
		succeeded++
		for acct, n := range counts {
			if r := rows[acct]; r != nil {
				cond.set(r, n)
			}
		}
	}
	if succeeded == 0 {
		return view, fmt.Errorf("every endpoint count failed: %s", view.FetchFailures[0].Error)
	}
	for _, acct := range order {
		r := rows[acct]
		r.Attention = r.AttentionScore()
		view.Sites = append(view.Sites, *r)
	}
	fleet.SortHealth(view.Sites)
	return view, nil
}

// countByAccount runs one endpoint search-groupby grouped by account_id.
func countByAccount(ctx context.Context, c *client.Client, accounts []string, filter map[string]any) (map[string]int, error) {
	body := map[string]any{"account_ids": accounts, "group_by": "account_id", "page_size": tdSearchPageSize}
	for k, v := range filter {
		body[k] = v
	}
	out := map[string]int{}
	for page := 0; ; page++ {
		if page == tdDefaultMaxPages {
			return nil, fmt.Errorf("search-groupby did not finish within %d pages", tdDefaultMaxPages)
		}
		data, _, err := c.Post(ctx, "/v1/endpoints/search-groupby", body)
		if err != nil {
			return nil, err
		}
		var env struct {
			Results *[]*struct {
				Value json.RawMessage `json:"value"`
				Total *int            `json:"total"`
			} `json:"results"`
			NextCursor *string `json:"next_cursor"`
		}
		if err := json.Unmarshal(data, &env); err != nil {
			return nil, fmt.Errorf("parsing search-groupby: %w", err)
		}
		// A missing list or cursor is not "zero endpoints"; it is a bad answer.
		if env.Results == nil || env.NextCursor == nil {
			return nil, fmt.Errorf("search-groupby response has no results list or next_cursor")
		}
		for _, r := range *env.Results {
			var acct string
			if r == nil || r.Total == nil || json.Unmarshal(r.Value, &acct) != nil || acct == "" {
				return nil, fmt.Errorf("search-groupby returned a result without an account_id value and total")
			}
			out[acct] += *r.Total
		}
		if *env.NextCursor == "" {
			return out, nil
		}
		body["next_cursor"] = *env.NextCursor
	}
}

func fleetHealthLocal(cmd *cobra.Command, siteFilter string, staleHours, scanDays int, dbPath string, now time.Time) (fleetHealthView, bool, error) {
	view := fleetHealthView{Source: "local", StaleHours: staleHours, ScanDays: scanDays, Sites: make([]fleet.HealthRow, 0)}
	db, ok, err := openFleetStore(cmd, dbPath)
	if err != nil || !ok {
		view.Note = "no local mirror; run " + tdFleetSyncCommand
		return view, ok, err
	}
	defer db.Close()
	if !hintIfUnsynced(cmd, db, tdResSites) {
		hintIfStale(cmd, db, tdResSites, tdMaxAge(cmd))
	}
	fleetHint(cmd, db, tdResEndpoints, tdMaxAge(cmd))
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
	view.Note = joinNotes(view.Note, accountlessNote(sites))
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
		if keep[e.AccountID] {
			scoped = append(scoped, e)
		}
	}
	view.Sites = fleet.HealthFromEndpoints(scoped, sites, now, staleHours, scanDays)
	return view, true, nil
}
