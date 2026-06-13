// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

type staleAgent struct {
	AgentID       int64  `json:"AgentID"`
	AgentName     string `json:"AgentName"`
	MachineName   string `json:"MachineName"`
	CustomerName  string `json:"CustomerName"`
	OS            string `json:"OS"`
	Online        bool   `json:"Online"`
	LastSeen      string `json:"LastSeen"`
	DaysSinceSeen int    `json:"DaysSinceSeen"` // -1 when LastSeen is missing/unparseable
}

// pp:data-source local
func newNovelAgentsStaleCmd(flags *rootFlags) *cobra.Command {
	var days int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "stale",
		Short: "Surface agents that have gone quiet — offline or not seen in N days — before the client calls to complain.",
		Long: "Lists agents whose last check-in is older than --days, or that are currently offline.\n" +
			"Reads the local store, so run `atera-cli sync` first. The 'who went dark' time-window\n" +
			"is something the live API never returns — it only reports current state.",
		Example:     "  atera-cli agents stale --days 30 --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if dbPath == "" {
				dbPath = defaultDBPath("atera-cli")
			}
			s, nvOK, err := nvOpenRead(dbPath)
			if err != nil {
				return fmt.Errorf("opening store: %w", err)
			}
			if nvOK {
				defer s.Close()
			}

			if !hintIfUnsynced(cmd, s, "agents") {
				hintIfStale(cmd, s, "agents", flags.maxAge)
			}

			agents, err := nvLoad(s, "agents")
			if err != nil {
				return fmt.Errorf("loading agents: %w", err)
			}

			now := nvNow()
			cutoff := now.AddDate(0, 0, -days)
			results := make([]staleAgent, 0)
			for _, o := range agents {
				online := nvBool(o, "Online")
				ls, ok := nvTime(o, "LastSeen")
				dss := -1
				stale := false
				if ok {
					dss = int(now.Sub(ls).Hours() / 24)
					if ls.Before(cutoff) {
						stale = true
					}
				} else if !online {
					// No parseable LastSeen but currently offline → still dark.
					stale = true
				}
				if !stale {
					continue
				}
				id, _ := nvInt(o, "AgentID")
				results = append(results, staleAgent{
					AgentID:       id,
					AgentName:     nvStr(o, "AgentName"),
					MachineName:   nvStr(o, "MachineName"),
					CustomerName:  nvStr(o, "CustomerName"),
					OS:            nvStr(o, "OS"),
					Online:        online,
					LastSeen:      nvStr(o, "LastSeen"),
					DaysSinceSeen: dss,
				})
			}
			// Darkest first; unknown last-seen (-1) sorts to the end.
			sort.SliceStable(results, func(i, j int) bool {
				a, b := results[i].DaysSinceSeen, results[j].DaysSinceSeen
				if (a < 0) != (b < 0) {
					return a >= 0
				}
				return a > b
			})

			return nvEmit(cmd, flags, results, func() {
				w := cmd.OutOrStdout()
				if len(results) == 0 {
					fmt.Fprintf(w, "No agents stale beyond %d days.\n", days)
					return
				}
				rows := make([][]string, 0, len(results))
				for _, r := range results {
					age := "?"
					if r.DaysSinceSeen >= 0 {
						age = fmt.Sprintf("%dd", r.DaysSinceSeen)
					}
					rows = append(rows, []string{
						r.MachineName, r.CustomerName, r.OS,
						fmt.Sprintf("%t", r.Online), age,
					})
				}
				nvTable(w, []string{"MACHINE", "CUSTOMER", "OS", "ONLINE", "DARK"}, rows)
			})
		},
	}
	cmd.Flags().IntVar(&days, "days", 30, "Flag agents not seen in this many days")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/atera-cli/data.db)")
	return cmd
}
