// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"threatdown-pp-cli/internal/cliutil"
	"threatdown-pp-cli/internal/fleet"
)

type jobRow struct {
	ID          string          `json:"id"`
	AccountID   string          `json:"account_id"`
	AccountName string          `json:"account_name,omitempty"`
	Command     string          `json:"command"`
	MachineID   string          `json:"machine_id"`
	MachineName string          `json:"machine_name"`
	Status      int             `json:"status"`
	StatusName  string          `json:"status_name"`
	IssuedAt    string          `json:"issued_at"`
	Online      *bool           `json:"online,omitempty"` // nil when OneView did not report it
	Data        json.RawMessage `json:"data,omitempty"`   // job settings; OneView sends a JSON string
}

func parseJobs(raws []json.RawMessage) []jobRow {
	out := make([]jobRow, 0, len(raws))
	for _, r := range raws {
		var j jobRow
		if json.Unmarshal(r, &j) == nil && j.ID != "" {
			j.StatusName = fleet.DecodeJobStatus(j.Status)
			out = append(out, j)
		}
	}
	return out
}

type customerReportView struct {
	Site       fleet.Site        `json:"site"`
	Since      string            `json:"since"`
	Health     fleet.HealthRow   `json:"health"`
	Stale      []fleet.StaleRow  `json:"stale_endpoints"`
	TopThreats []fleet.SpreadRow `json:"top_threats"`
	Isolated   []fleet.Endpoint  `json:"isolated_endpoints"`
	JobsNotRan []jobRow          `json:"jobs_not_completed"`
	Note       string            `json:"note,omitempty"`
}

// matchSite resolves a site by exact id/account_id, then by a unique exact
// name, then by a unique case-insensitive substring of the name. Two sites
// with the same name are ambiguous, never a silent first pick.
func matchSite(sites []fleet.Site, query string) (fleet.Site, []string, bool) {
	q := strings.ToLower(strings.TrimSpace(query))
	for _, s := range sites {
		if strings.EqualFold(s.ID, q) || strings.EqualFold(s.AccountID, q) {
			return s, nil, true
		}
	}
	var hits []fleet.Site
	for _, s := range sites {
		if strings.ToLower(s.Name) == q {
			hits = append(hits, s)
		}
	}
	if len(hits) == 0 {
		for _, s := range sites {
			if strings.Contains(strings.ToLower(s.Name), q) {
				hits = append(hits, s)
			}
		}
	}
	names := make([]string, 0, len(hits))
	for _, h := range hits {
		names = append(names, h.Name+" ("+h.ID+")")
	}
	sort.Strings(names)
	if len(hits) == 1 {
		return hits[0], nil, true
	}
	return fleet.Site{}, names, false
}

func newNovelCustomerReportCmd(flags *rootFlags) *cobra.Command {
	var since, dbPath string

	cmd := &cobra.Command{
		Use:   "report [site]",
		Short: "Everything about one customer site in one output: health, stale machines, top threats, isolated endpoints and pending jobs.",
		Long: `Use this command for everything about one customer site in one output. Do NOT use this command to compare all sites side by side; use 'fleet health' instead.

The site argument matches a site id, account id, exact name, or a unique part
of the name (case-insensitive). Everything is read from the store that
'fleet sync' fills, so it answers instantly while a customer is on the phone.
Before the first 'fleet sync' it exits 3 (not found) instead of printing an
empty profile.`,
		Example: strings.Trim(`
  threatdown-cli customer report "Acme Corp" --agent
  threatdown-cli customer report acme --since 30d --json --select health,top_threats.threat`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local", "pp:happy-args": "site=Acme"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "customer report")
			}
			if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("site name is required, e.g. customer report \"Acme Corp\""))
			}
			window, err := cliutil.ParseDurationLoose(since)
			if err != nil || window <= 0 {
				return usageErr(fmt.Errorf("--since must be a duration like 30d, got %q", since))
			}
			path := tdDBPath(dbPath)
			db, ok, err := openFleetStore(cmd, path)
			if err != nil {
				return err
			}
			empty := customerReportView{Since: since, Stale: []fleet.StaleRow{}, TopThreats: []fleet.SpreadRow{}, Isolated: []fleet.Endpoint{}, JobsNotRan: []jobRow{}}
			// With nothing synced the named site cannot be checked; an all-zero
			// profile with exit 0 would read as a healthy customer.
			if !ok {
				return notFoundErr(fmt.Errorf("cannot look up %q: there is no local mirror yet; run %s first", args[0], tdFleetSyncCommand))
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, tdResSites) {
				hintIfStale(cmd, db, tdResSites, tdMaxAge(cmd))
			}
			fleetHint(cmd, db, tdResEndpoints, tdMaxAge(cmd))
			var unsynced []string
			for _, res := range []string{tdResEndpoints, tdResDetections, tdResJobs} {
				if state, err := readSyncHintState(db, res); err == nil && !state.hasState {
					unsynced = append(unsynced, res)
				}
			}
			ctx := cmd.Context()
			sites, err := loadLocalSites(ctx, db)
			if err != nil {
				return err
			}
			if len(sites) == 0 {
				return notFoundErr(fmt.Errorf("cannot look up %q: the local store has no sites yet; run %s first", args[0], tdFleetSyncCommand))
			}
			site, candidates, found := matchSite(sites, args[0])
			if !found {
				if len(candidates) > 1 {
					return usageErr(fmt.Errorf("%q matches %d sites: %s; be more specific", args[0], len(candidates), strings.Join(candidates, ", ")))
				}
				return notFoundErr(fmt.Errorf("no site matches %q; list sites with: %s sites list", args[0], tdBinary))
			}
			endpoints, err := loadLocalEndpoints(ctx, db)
			if err != nil {
				return err
			}
			detRaw, err := loadLocal(ctx, db, tdResDetections)
			if err != nil {
				return err
			}
			jobRaw, err := loadLocal(ctx, db, tdResJobs)
			if err != nil {
				return err
			}
			now := time.Now().UTC()
			mine := make([]fleet.Endpoint, 0)
			for _, e := range endpoints {
				if e.AccountID == site.AccountID {
					mine = append(mine, e)
				}
			}
			dets := make([]fleet.Detection, 0)
			for _, d := range parseDetections(detRaw) {
				if d.AccountID == site.AccountID {
					dets = append(dets, d)
				}
			}
			idx := map[string]fleet.Site{site.AccountID: site}
			view := empty
			view.Site = site
			view.Health = fleet.HealthRow{Site: site.Name, AccountID: site.AccountID}
			// A site without an account_id has no subscription, so OneView holds
			// nothing for it. Records missing an account_id must not be
			// attributed to it.
			if site.AccountID == "" {
				view.Note = "this site has no account_id yet (no subscription), so OneView holds no endpoints, threats or jobs for it"
				return printCustomerReport(cmd, flags, view)
			}
			notes := []string{}
			if len(unsynced) > 0 {
				notes = append(notes, "not synced yet: "+strings.Join(unsynced, ", ")+"; run "+tdFleetSyncCommand+" (those sections read empty until then)")
			}
			for _, res := range []string{tdResDetections, tdResJobs} {
				if n := coverageNote(ctx, db, res, time.Now().UTC().Add(-window)); n != "" {
					notes = append(notes, n)
				}
			}
			view.Note = strings.Join(notes, "; ")
			if h := fleet.HealthFromEndpoints(mine, []fleet.Site{site}, now, 24, 7); len(h) > 0 {
				view.Health = h[0]
			}
			view.Stale = fleet.Stale(mine, idx, now, fleet.StaleOptions{Hours: 24})
			view.TopThreats = fleet.Spread(dets, idx, now.Add(-window), 1)
			// Within one site reach is flat, so lead with what happened last.
			sort.SliceStable(view.TopThreats, func(i, j int) bool {
				return view.TopThreats[i].LastSeen > view.TopThreats[j].LastSeen
			})
			if len(view.TopThreats) > 10 {
				view.TopThreats = view.TopThreats[:10]
			}
			for _, e := range mine {
				if e.Isolated && !e.IsDeleted {
					view.Isolated = append(view.Isolated, e)
				}
			}
			for _, j := range parseJobs(jobRaw) {
				t, ok := fleet.ParseTime(j.IssuedAt)
				if j.AccountID == site.AccountID && j.Status&fleet.JobStatusNotRan != 0 && (!ok || !t.Before(now.Add(-window))) {
					view.JobsNotRan = append(view.JobsNotRan, j)
				}
			}
			return printCustomerReport(cmd, flags, view)
		},
	}
	cmd.Flags().StringVar(&since, "since", "30d", "Window for threats and jobs (e.g. 7d, 30d)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: the store 'fleet sync' writes)")
	return cmd
}

func printCustomerReport(cmd *cobra.Command, flags *rootFlags, v customerReportView) error {
	if !wantsHumanTable(cmd.OutOrStdout(), flags) {
		return printJSONFiltered(cmd.OutOrStdout(), v, flags)
	}
	w := cmd.OutOrStdout()
	if v.Site.Name == "" {
		fmt.Fprintln(w, v.Note)
		return nil
	}
	h := v.Health
	if v.Site.AccountID == "" {
		fmt.Fprintf(w, "%s (no account_id)\n", v.Site.Name)
	} else {
		fmt.Fprintf(w, "%s (account %s)\n", v.Site.Name, v.Site.AccountID)
	}
	if v.Note != "" {
		fmt.Fprintf(w, "  note: %s\n", v.Note)
	}
	fmt.Fprintf(w, "  endpoints %d | online %d | stale %d | infected %d | suspicious %d | isolated %d | reboot %d | unprotected %d | scan overdue %d\n",
		h.Total, h.Online, h.Stale, h.Infected, h.Suspicious, h.Isolated, h.RebootRequired, h.Unprotected, h.ScanOverdue)
	sections := []struct {
		title string
		rows  any
		n     int
		cols  []string
	}{
		{"Stale endpoints (24h)", v.Stale, len(v.Stale), []string{"host_name", "hours_since_seen", "last_seen", "isolated", "infection_count"}},
		{"Top threats (" + v.Since + ")", v.TopThreats, len(v.TopThreats), []string{"threat", "category", "endpoints", "detections", "last_seen"}},
		{"Isolated endpoints", v.Isolated, len(v.Isolated), []string{"host_name", "display_name", "online", "infection_count", "last_day_seen"}},
		{"Jobs not completed (" + v.Since + ")", v.JobsNotRan, len(v.JobsNotRan), []string{"machine_name", "command", "status_name", "issued_at"}},
	}
	for _, s := range sections {
		fmt.Fprintf(w, "\n%s: %d\n", s.title, s.n)
		if s.n > 0 {
			if err := printColumns(w, s.rows, s.cols...); err != nil {
				return err
			}
		}
	}
	return nil
}
