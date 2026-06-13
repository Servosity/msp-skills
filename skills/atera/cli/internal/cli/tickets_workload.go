// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

type techLoad struct {
	Technician           string `json:"Technician"`
	OpenTickets          int    `json:"OpenTickets"`
	TotalDurationMinutes int64  `json:"TotalDurationMinutes"`
}

// pp:data-source local
func newNovelTicketsWorkloadCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var includeAll bool

	cmd := &cobra.Command{
		Use:   "workload",
		Short: "Group open tickets and total logged duration by technician to spot who is overloaded.",
		Long: "Use this command to see open-ticket count + logged duration per technician. Do NOT use it to rank tickets by SLA-breach urgency; use 'tickets sla' instead.\n\n" +
			"Aggregates open tickets and their total logged minutes per assigned technician.\n" +
			"Reads the local store; run `atera-cli sync` first. Cross-ticket aggregation\n" +
			"like this is never returned by a single API call.",
		Example:     "  atera-cli tickets workload --agent",
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

			if !hintIfUnsynced(cmd, s, "tickets") {
				hintIfStale(cmd, s, "tickets", flags.maxAge)
			}

			tickets, err := nvLoad(s, "tickets")
			if err != nil {
				return fmt.Errorf("loading tickets: %w", err)
			}

			byTech := map[string]*techLoad{}
			for _, o := range tickets {
				if !includeAll && !nvIsOpenTicket(nvStr(o, "TicketStatus")) {
					continue
				}
				tech := nvStr(o, "TechnicianFullName")
				if tech == "" {
					tech = "(unassigned)"
				}
				tl := byTech[tech]
				if tl == nil {
					tl = &techLoad{Technician: tech}
					byTech[tech] = tl
				}
				tl.OpenTickets++
				if dur, ok := nvInt(o, "TotalDurationMinutes"); ok {
					tl.TotalDurationMinutes += dur
				}
			}

			results := make([]techLoad, 0, len(byTech))
			for _, tl := range byTech {
				results = append(results, *tl)
			}
			sort.SliceStable(results, func(i, j int) bool {
				if results[i].OpenTickets != results[j].OpenTickets {
					return results[i].OpenTickets > results[j].OpenTickets
				}
				return results[i].TotalDurationMinutes > results[j].TotalDurationMinutes
			})

			return nvEmit(cmd, flags, results, func() {
				w := cmd.OutOrStdout()
				if len(results) == 0 {
					fmt.Fprintln(w, "No open tickets to attribute.")
					return
				}
				rows := make([][]string, 0, len(results))
				for _, r := range results {
					rows = append(rows, []string{
						r.Technician,
						fmt.Sprintf("%d", r.OpenTickets),
						fmt.Sprintf("%d", r.TotalDurationMinutes),
					})
				}
				nvTable(w, []string{"TECHNICIAN", "OPEN", "MINUTES"}, rows)
			})
		},
	}
	cmd.Flags().BoolVar(&includeAll, "all", false, "Include resolved/closed tickets too (default: open only)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/atera-cli/data.db)")
	return cmd
}
