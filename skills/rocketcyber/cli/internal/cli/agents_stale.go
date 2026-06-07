// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel feature command.
//
// pp:data-source local

package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"rocketcyber-pp-cli/internal/cliutil"
	"rocketcyber-pp-cli/internal/store"
)

type staleAgent struct {
	Hostname      string `json:"hostname"`
	CustomerID    int64  `json:"customer_id,omitempty"`
	Connectivity  string `json:"connectivity"`
	LastConnected string `json:"last_connected"`
	DaysSilent    int    `json:"days_silent"`
}

type staleView struct {
	Since      string         `json:"since"`
	StaleCount int            `json:"stale_count"`
	ByCustomer map[string]int `json:"by_customer"`
	Agents     []staleAgent   `json:"agents"`
	Note       string         `json:"note,omitempty"`
}

// classifyStaleAgents keeps agents whose lastConnected is older than cutoff,
// sorted longest-silent first, capped at limit, grouped by customer ID.
func classifyStaleAgents(items []json.RawMessage, cutoff, now time.Time, limit int) staleView {
	view := staleView{
		ByCustomer: map[string]int{},
		Agents:     []staleAgent{},
	}
	for _, raw := range items {
		var probe map[string]json.RawMessage
		if err := json.Unmarshal(raw, &probe); err != nil {
			continue
		}
		last := extractString(probe, "lastConnected")
		t := parseAPITime(last)
		if t.IsZero() || !t.Before(cutoff) {
			continue
		}
		row := staleAgent{
			Hostname:      extractString(probe, "hostname"),
			Connectivity:  extractString(probe, "connectivity"),
			LastConnected: last,
			DaysSilent:    int(now.Sub(t).Hours() / 24),
		}
		if id, ok := cliutil.ExtractInt(probe, "customerId"); ok {
			row.CustomerID = id
			view.ByCustomer[strconv.FormatInt(id, 10)]++
		}
		view.StaleCount++
		view.Agents = append(view.Agents, row)
	}
	sort.Slice(view.Agents, func(i, j int) bool { return view.Agents[i].DaysSilent > view.Agents[j].DaysSilent })
	if limit > 0 && len(view.Agents) > limit {
		view.Agents = view.Agents[:limit]
	}
	return view
}

func newNovelAgentsStaleCmd(flags *rootFlags) *cobra.Command {
	var flagSince string
	var flagDB string
	var flagLimit int

	cmd := &cobra.Command{
		Use:   "stale",
		Short: "Devices that stopped reporting beyond a time window, grouped by client account.",
		Long: strings.Trim(`
Filters synced agents on lastConnected age - a dimension the live API cannot
filter by - and groups the result by customer account.

Reads the local store - run 'rocketcyber-cli sync --resources agents'
first. Use this for fleet hygiene (offline/stale devices by account). Do NOT
use it for a raw filtered device dump; use 'agents' instead.
`, "\n"),
		Example: strings.Trim(`
  rocketcyber-cli agents stale --since 7d --json
  rocketcyber-cli agents stale --since 30d --limit 100 --agent
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would scan synced agents for devices silent beyond the --since window")
				return nil
			}
			if flagSince == "" {
				flagSince = "7d"
			}
			window, err := cliutil.ParseDurationLoose(flagSince)
			if err != nil {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("invalid --since value %q: %w", flagSince, err))
			}
			if flagDB == "" {
				flagDB = defaultDBPath("rocketcyber-cli")
			}
			db, err := store.OpenWithContext(cmd.Context(), flagDB)
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "agents") {
				hintIfStale(cmd, db, "agents", flags.maxAge)
			}
			rows, err := db.List("agents", 10000)
			if err != nil {
				return fmt.Errorf("reading agents from local store: %w", err)
			}
			now := time.Now().UTC()
			view := classifyStaleAgents(rows, now.Add(-window), now, flagLimit)
			view.Since = flagSince
			if view.StaleCount == 0 {
				if len(rows) == 0 {
					view.Note = "no agents in the local store; run 'rocketcyber-cli sync --resources agents --full' first"
				} else {
					view.Note = fmt.Sprintf("all %d synced agents reported within %s", len(rows), flagSince)
				}
			}
			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "7d", "Silence window - agents whose lastConnected is older than this are stale (e.g. 7d, 30d)")
	cmd.Flags().StringVar(&flagDB, "db", "", "Database path (default: local store)")
	cmd.Flags().IntVar(&flagLimit, "limit", 50, "Maximum stale agents to return")
	return cmd
}
