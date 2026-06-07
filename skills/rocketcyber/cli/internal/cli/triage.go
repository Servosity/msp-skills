// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel feature command.
//
// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"rocketcyber-pp-cli/internal/cliutil"
)

type triageIncident struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
	AgeDays   int    `json:"age_days"`

	createdTime time.Time
}

type triageAgent struct {
	Hostname      string `json:"hostname"`
	CustomerID    int64  `json:"customer_id,omitempty"`
	Connectivity  string `json:"connectivity"`
	LastConnected string `json:"last_connected,omitempty"`

	lastTime time.Time
}

type triageFailure struct {
	Source string `json:"source"`
	Error  string `json:"error"`
}

type triageView struct {
	Since              string           `json:"since"`
	OpenIncidentsTotal int64            `json:"open_incidents_total"`
	RecentOpenCount    int              `json:"recent_open_count"`
	OpenIncidents      []triageIncident `json:"open_incidents"`
	OfflineAgentsTotal int64            `json:"offline_agents_total"`
	OfflineAgents      []triageAgent    `json:"offline_agents"`
	EventSummary       json.RawMessage  `json:"event_summary,omitempty"`
	FetchFailures      []triageFailure  `json:"fetch_failures,omitempty"`
}

// parseTriageIncidents converts raw incident items into triage rows sorted
// newest-first and counts how many were created within the window cutoff.
func parseTriageIncidents(items []json.RawMessage, cutoff, now time.Time) ([]triageIncident, int) {
	rows := make([]triageIncident, 0, len(items))
	recent := 0
	for _, raw := range items {
		var probe map[string]json.RawMessage
		if err := json.Unmarshal(raw, &probe); err != nil {
			continue
		}
		row := triageIncident{
			Title:     extractString(probe, "title"),
			Status:    extractString(probe, "status"),
			CreatedAt: extractString(probe, "createdAt"),
		}
		if id, ok := cliutil.ExtractInt(probe, "id"); ok {
			row.ID = strconv.FormatInt(id, 10)
		} else {
			row.ID = extractString(probe, "id")
		}
		if t := parseAPITime(row.CreatedAt); !t.IsZero() {
			row.createdTime = t
			row.AgeDays = int(now.Sub(t).Hours() / 24)
			if t.After(cutoff) {
				recent++
			}
		}
		rows = append(rows, row)
	}
	// Sort on the parsed time, newest first; rows without a parseable
	// timestamp sort last regardless of wire format or UTC offset.
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].createdTime.IsZero() != rows[j].createdTime.IsZero() {
			return !rows[i].createdTime.IsZero()
		}
		return rows[i].createdTime.After(rows[j].createdTime)
	})
	return rows, recent
}

// parseTriageAgents converts raw agent items into triage rows sorted by
// longest-silent first.
func parseTriageAgents(items []json.RawMessage) []triageAgent {
	rows := make([]triageAgent, 0, len(items))
	for _, raw := range items {
		var probe map[string]json.RawMessage
		if err := json.Unmarshal(raw, &probe); err != nil {
			continue
		}
		row := triageAgent{
			Hostname:      extractString(probe, "hostname"),
			Connectivity:  extractString(probe, "connectivity"),
			LastConnected: extractString(probe, "lastConnected"),
		}
		if id, ok := cliutil.ExtractInt(probe, "customerId"); ok {
			row.CustomerID = id
		}
		row.lastTime = parseAPITime(row.LastConnected)
		rows = append(rows, row)
	}
	// Sort on the parsed time, longest-silent first; agents without a
	// parseable lastConnected sort last instead of falsely topping the board.
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].lastTime.IsZero() != rows[j].lastTime.IsZero() {
			return !rows[i].lastTime.IsZero()
		}
		return rows[i].lastTime.Before(rows[j].lastTime)
	})
	return rows
}

func newNovelTriageCmd(flags *rootFlags) *cobra.Command {
	var flagSince string
	var flagAccountID int

	cmd := &cobra.Command{
		Use:   "triage",
		Short: "One ranked board of open incidents, event verdict counts, and offline agents across every client account.",
		Long: strings.Trim(`
One ranked board of open incidents, event verdict counts, and offline agents,
fanned out across /incidents, /events/summary, and /agents in a single call
with partial-failure accounting.

Use this for the cross-account overnight triage board (incidents + events +
agents joined). Do NOT use it to list a single entity type with filters; use
'incidents', 'events list', or 'agents' instead.
`, "\n"),
		Example: strings.Trim(`
  rocketcyber-cli triage --since 24h --agent
  rocketcyber-cli triage --since 48h --account-id 2 --json
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if err := validateDataSourceStrategy(flags, "live"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would fan out to /incidents, /events/summary, and /agents for the triage board")
				return nil
			}
			if flagSince == "" {
				flagSince = "24h"
			}
			window, err := cliutil.ParseDurationLoose(flagSince)
			if err != nil {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("invalid --since value %q: %w", flagSince, err))
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			now := time.Now().UTC()
			cutoff := now.Add(-window)

			scoped := func(extra map[string]string) map[string]string {
				params := map[string]string{}
				if flagAccountID != 0 {
					params["accountId"] = strconv.Itoa(flagAccountID)
				}
				for k, v := range extra {
					params[k] = v
				}
				return params
			}

			type fetchResult struct {
				source string
				data   json.RawMessage
				err    error
			}
			sources := []struct {
				name   string
				path   string
				params map[string]string
			}{
				{"incidents", "/incidents", scoped(map[string]string{"status": "open", "pageSize": "50", "sort": "createdAt:desc"})},
				{"events-summary", "/events/summary", scoped(nil)},
				{"agents", "/agents", scoped(map[string]string{"connectivity": "offline", "pageSize": "50"})},
			}
			results := make(chan fetchResult, len(sources))
			var wg sync.WaitGroup
			for _, src := range sources {
				wg.Add(1)
				go func(name, path string, params map[string]string) {
					defer wg.Done()
					data, err := c.Get(ctx, path, params)
					results <- fetchResult{source: name, data: data, err: err}
				}(src.name, src.path, src.params)
			}
			wg.Wait()
			close(results)

			view := triageView{
				Since:         flagSince,
				OpenIncidents: []triageIncident{},
				OfflineAgents: []triageAgent{},
				FetchFailures: []triageFailure{},
			}
			for r := range results {
				if r.err != nil {
					view.FetchFailures = append(view.FetchFailures, triageFailure{Source: r.source, Error: r.err.Error()})
					continue
				}
				switch r.source {
				case "incidents":
					items, total := parseEnvelope(r.data)
					view.OpenIncidentsTotal = total
					view.OpenIncidents, view.RecentOpenCount = parseTriageIncidents(items, cutoff, now)
				case "events-summary":
					view.EventSummary = r.data
				case "agents":
					items, total := parseEnvelope(r.data)
					view.OfflineAgentsTotal = total
					view.OfflineAgents = parseTriageAgents(items)
				}
			}
			if len(view.FetchFailures) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of %d triage sources failed; board built from the remaining sources\n", len(view.FetchFailures), len(sources))
			}
			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "24h", "Recency window for the board (e.g. 24h, 48h, 7d)")
	cmd.Flags().IntVar(&flagAccountID, "account-id", 0, "Scope the board to a single client account ID")
	return cmd
}
