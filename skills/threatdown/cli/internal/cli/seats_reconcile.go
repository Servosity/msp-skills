// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"threatdown-pp-cli/internal/client"
	"threatdown-pp-cli/internal/fleet"
)

type seatAllocations struct {
	EP   int `json:"ep"`
	EDR  int `json:"edr"`
	IR   int `json:"ir"`
	EPS  int `json:"eps"`
	EDRS int `json:"edrs"`
	MOB  int `json:"mob"`
}

type seatRow struct {
	Site                 string   `json:"site"`
	AccountID            string   `json:"account_id"`
	SiteEndDate          string   `json:"site_end_date,omitempty"`
	WorkstationAllocated int      `json:"workstation_allocated"`
	WorkstationDeployed  int      `json:"workstation_deployed"`
	WorkstationActive    int      `json:"workstation_active"`
	ServerAllocated      int      `json:"server_allocated"`
	ServerDeployed       int      `json:"server_deployed"`
	ServerActive         int      `json:"server_active"`
	MobileAllocated      int      `json:"mobile_allocated"`
	StaleSeats           int      `json:"stale_seats"`
	Products             []string `json:"products"`
	Flags                []string `json:"flags"`
}

type seatsView struct {
	ActiveDays    int            `json:"active_days"`
	Note          string         `json:"note,omitempty"`
	Sites         []seatRow      `json:"sites"`
	FetchFailures []fetchFailure `json:"fetch_failures,omitempty"`
}

// seatFlags names the reconciliation findings for one site.
func seatFlags(r seatRow) []string {
	flags := make([]string, 0)
	check := func(kind string, alloc, deployed, active int) {
		switch {
		case deployed > alloc:
			flags = append(flags, fmt.Sprintf("%s-under-allocated:%d-unlicensed", kind, deployed-alloc))
		case alloc > deployed:
			flags = append(flags, fmt.Sprintf("%s-over-allocated:%d-unused", kind, alloc-deployed))
		}
		if deployed > active {
			flags = append(flags, fmt.Sprintf("%s-stale:%d", kind, deployed-active))
		}
	}
	check("workstation", r.WorkstationAllocated, r.WorkstationDeployed, r.WorkstationActive)
	check("server", r.ServerAllocated, r.ServerDeployed, r.ServerActive)
	return flags
}

func newNovelSeatsReconcileCmd(flags *rootFlags) *cobra.Command {
	var activeDays int
	var siteFilter string

	cmd := &cobra.Command{
		Use:   "reconcile",
		Short: "Per-site seat allocations compared with deployed and recently seen endpoints, with expiry flags.",
		Long: `Use this command to compare per-site seat allocations against deployed and recently-seen endpoints for billing. Do NOT use this command to list the individual stale machines inflating a count; use 'fleet stale' instead.

Allocations come from each site's v2 subscription (workstation = ep + edr + ir,
server = eps + edrs). Deployed and active counts come from endpoint
search-groupby by account, split by os_info.os_type and by last seen within
--active-days. Expired and expiring subscriptions come from the
subscriptions report.`,
		Example: strings.Trim(`
  threatdown-cli seats reconcile --agent
  threatdown-cli seats reconcile --active-days 14 --site "Acme" --json --select sites.site,sites.flags`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "seats reconcile")
			}
			if activeDays <= 0 {
				return usageErr(fmt.Errorf("--active-days must be positive"))
			}
			ctx := cmd.Context()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			view, err := seatsReconcile(ctx, c, siteFilter, activeDays, time.Now().UTC())
			if err != nil {
				return classifyAPIErrorOnly(err)
			}
			if len(view.FetchFailures) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d lookups failed; affected sites are flagged and their numbers are incomplete\n", len(view.FetchFailures))
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFilteredKeep(cmd.OutOrStdout(), view, flags, "flags", "fetch_failures", "note")
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
			return printColumns(cmd.OutOrStdout(), view.Sites, "site", "workstation_allocated", "workstation_deployed", "workstation_active", "server_allocated", "server_deployed", "server_active", "stale_seats", "flags")
		},
	}
	cmd.Flags().IntVar(&activeDays, "active-days", 30, "Count an endpoint as active when seen within this many days")
	cmd.Flags().StringVar(&siteFilter, "site", "", "Only sites whose name contains this text")
	return cmd
}

func seatsReconcile(ctx context.Context, c *client.Client, siteFilter string, activeDays int, now time.Time) (seatsView, error) {
	view := seatsView{ActiveDays: activeDays, Sites: make([]seatRow, 0)}
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
	activeCut := isoAgo(now, time.Duration(activeDays)*24*time.Hour)
	counts := map[string]map[string]int{}
	for _, q := range []struct {
		name   string
		filter map[string]any
	}{
		{"workstation_deployed", map[string]any{"os_info.os_type": "Workstation"}},
		{"workstation_active", map[string]any{"os_info.os_type": "Workstation", "last_day_seen_after": activeCut}},
		{"server_deployed", map[string]any{"os_info.os_type": "Server"}},
		{"server_active", map[string]any{"os_info.os_type": "Server", "last_day_seen_after": activeCut}},
	} {
		m, err := countByAccount(ctx, c, accounts, q.filter)
		if err != nil {
			view.FetchFailures = append(view.FetchFailures, fetchFailure{Item: q.name, Error: err.Error()})
			continue
		}
		counts[q.name] = m
	}
	expiry := map[string][]string{}
	if data, err := c.Get(ctx, "/v1/subscriptions/report", nil); err != nil {
		view.FetchFailures = append(view.FetchFailures, fetchFailure{Item: "subscriptions_report", Error: err.Error()})
	} else {
		var rep map[string]json.RawMessage
		if json.Unmarshal(data, &rep) == nil {
			for key, raw := range rep {
				var ids []string
				if !strings.HasSuffix(key, "_ids") && !strings.HasSuffix(key, "_ids_trial") {
					continue
				}
				if json.Unmarshal(raw, &ids) != nil {
					continue
				}
				label := strings.ReplaceAll(strings.ReplaceAll(key, "_ids", ""), "_", "-")
				for _, id := range ids {
					expiry[id] = append(expiry[id], label)
				}
			}
		}
	}
	for _, s := range sites {
		if s.AccountID == "" {
			continue
		}
		row := seatRow{Site: s.Name, AccountID: s.AccountID, SiteEndDate: s.EndDate, Products: make([]string, 0)}
		row.WorkstationDeployed = counts["workstation_deployed"][s.AccountID]
		row.WorkstationActive = counts["workstation_active"][s.AccountID]
		row.ServerDeployed = counts["server_deployed"][s.AccountID]
		row.ServerActive = counts["server_active"][s.AccountID]
		subs, err := siteSubscriptions(ctx, c, s.ID)
		if err != nil {
			view.FetchFailures = append(view.FetchFailures, fetchFailure{Item: "subscriptions:" + s.Name, Error: err.Error()})
		}
		for _, sub := range subs {
			a := sub.Allocations
			row.WorkstationAllocated += a.EP + a.EDR + a.IR
			row.ServerAllocated += a.EPS + a.EDRS
			row.MobileAllocated += a.MOB
			if sub.Product != "" {
				row.Products = append(row.Products, sub.Product)
			}
		}
		row.StaleSeats = row.WorkstationDeployed - row.WorkstationActive + row.ServerDeployed - row.ServerActive
		row.Flags = seatFlags(row)
		if err != nil {
			row.Flags = append(row.Flags, "allocations-unknown")
		}
		if len(counts) < 4 {
			row.Flags = append(row.Flags, "counts-incomplete")
		}
		labels := append(append([]string{}, expiry[s.ID]...), expiry[s.AccountID]...)
		sort.Strings(labels)
		row.Flags = append(row.Flags, labels...)
		view.Sites = append(view.Sites, row)
	}
	sort.SliceStable(view.Sites, func(i, j int) bool {
		if len(view.Sites[i].Flags) != len(view.Sites[j].Flags) {
			return len(view.Sites[i].Flags) > len(view.Sites[j].Flags)
		}
		return view.Sites[i].Site < view.Sites[j].Site
	})
	return view, nil
}

type siteSubscription struct {
	Product     string          `json:"product"`
	TermType    string          `json:"term_type"`
	Allocations seatAllocations `json:"allocations"`
}

// siteSubscriptions reads GET /v2/sites/{id}/subscriptions (array or object).
func siteSubscriptions(ctx context.Context, c *client.Client, siteID string) ([]siteSubscription, error) {
	data, err := c.Get(ctx, "/v2/sites/"+siteID+"/subscriptions", nil)
	if err != nil {
		return nil, err
	}
	var subs []siteSubscription
	if err := json.Unmarshal(data, &subs); err == nil {
		return subs, nil
	}
	var one siteSubscription
	if err := json.Unmarshal(data, &one); err != nil {
		return nil, fmt.Errorf("parsing subscriptions for site %s: %w", siteID, err)
	}
	return []siteSubscription{one}, nil
}
