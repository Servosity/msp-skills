// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

type slaTicket struct {
	TicketID             int64  `json:"TicketID"`
	TicketTitle          string `json:"TicketTitle"`
	CustomerName         string `json:"CustomerName"`
	TicketStatus         string `json:"TicketStatus"`
	TicketPriority       string `json:"TicketPriority"`
	TechnicianFullName   string `json:"TechnicianFullName"`
	FirstResponseDueDate string `json:"FirstResponseDueDate"`
	ClosedTicketDueDate  string `json:"ClosedTicketDueDate"`
	MinutesToBreach      int64  `json:"MinutesToBreach"` // negative = already breached
	Breached             bool   `json:"Breached"`
	DueKind              string `json:"DueKind"` // which deadline is nearest: first-response | resolution
}

// pp:data-source local
func newNovelTicketsSlaCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var includeAll bool

	cmd := &cobra.Command{
		Use:   "sla",
		Short: "Rank open tickets by how close they are to breaching first-response or resolution SLA.",
		Long: "Use this command to rank OPEN tickets by how soon they breach SLA. Do NOT use it to see per-technician open-ticket load; use 'tickets workload' instead.\n\n" +
			"Computes minutes-to-breach for each open ticket from its first-response and\n" +
			"resolution due dates, then ranks soonest/most-overdue first. Reads the local\n" +
			"store; run `atera-cli sync` first. This is arithmetic over the ticket set —\n" +
			"not a filter the API offers.",
		Example:     "  atera-cli tickets sla --agent",
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

			now := nvNow()
			results := make([]slaTicket, 0)
			for _, o := range tickets {
				status := nvStr(o, "TicketStatus")
				if !includeAll && !nvIsOpenTicket(status) {
					continue
				}
				// Nearest of the two deadlines drives the ranking.
				var nearest *int64
				dueKind := ""
				if t, ok := nvTime(o, "FirstResponseDueDate"); ok {
					m := int64(t.Sub(now).Minutes())
					nearest = &m
					dueKind = "first-response"
				}
				if t, ok := nvTime(o, "ClosedTicketDueDate"); ok {
					m := int64(t.Sub(now).Minutes())
					if nearest == nil || m < *nearest {
						nearest = &m
						dueKind = "resolution"
					}
				}
				if nearest == nil {
					// No SLA deadline on this ticket — skip (nothing to rank).
					continue
				}
				id, _ := nvInt(o, "TicketID")
				results = append(results, slaTicket{
					TicketID:             id,
					TicketTitle:          nvStr(o, "TicketTitle"),
					CustomerName:         nvStr(o, "CustomerName"),
					TicketStatus:         status,
					TicketPriority:       nvStr(o, "TicketPriority"),
					TechnicianFullName:   nvStr(o, "TechnicianFullName"),
					FirstResponseDueDate: nvStr(o, "FirstResponseDueDate"),
					ClosedTicketDueDate:  nvStr(o, "ClosedTicketDueDate"),
					MinutesToBreach:      *nearest,
					Breached:             *nearest < 0,
					DueKind:              dueKind,
				})
			}
			// Most overdue / soonest-to-breach first.
			sort.SliceStable(results, func(i, j int) bool {
				return results[i].MinutesToBreach < results[j].MinutesToBreach
			})

			return nvEmit(cmd, flags, results, func() {
				w := cmd.OutOrStdout()
				if len(results) == 0 {
					fmt.Fprintln(w, "No open tickets with SLA deadlines.")
					return
				}
				rows := make([][]string, 0, len(results))
				for _, r := range results {
					eta := fmt.Sprintf("%dm", r.MinutesToBreach)
					if r.Breached {
						eta = "BREACHED " + eta
					}
					rows = append(rows, []string{
						fmt.Sprintf("%d", r.TicketID), r.CustomerName,
						r.TicketPriority, r.DueKind, eta,
					})
				}
				nvTable(w, []string{"TICKET", "CUSTOMER", "PRIORITY", "DUE", "TO-BREACH"}, rows)
			})
		},
	}
	cmd.Flags().BoolVar(&includeAll, "all", false, "Include resolved/closed tickets too (default: open only)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/atera-cli/data.db)")
	return cmd
}
