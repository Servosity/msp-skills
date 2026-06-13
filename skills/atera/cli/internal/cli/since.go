// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"
)

type sinceItem struct {
	Kind     string `json:"Kind"` // agent | ticket | alert
	ID       int64  `json:"ID"`
	Name     string `json:"Name"`
	Customer string `json:"Customer"`
	When     string `json:"When"`
	// whenT is the parsed instant backing When; sorting on the raw string
	// breaks across mixed formats (/Date(ms)/ vs ISO vs fractional seconds).
	whenT time.Time
}

type sinceReport struct {
	Window     string      `json:"Window"`
	Since      string      `json:"Since"`
	NewAgents  int         `json:"NewAgents"`
	NewTickets int         `json:"NewTickets"`
	NewAlerts  int         `json:"NewAlerts"`
	Items      []sinceItem `json:"Items"`
}

// pp:data-source local
func newNovelSinceCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "since [window]",
		Short: "Show what was created in a time window — new agents, new tickets, new alerts — across the synced estate.",
		Long: "Summarizes records created within the given window (default 24h; accepts forms\n" +
			"like 90m, 24h, 7d, 2d12h) by reading synced timestamps from the local store.\n" +
			"Run `atera-cli sync` first. No single API call spans entities and time; this\n" +
			"reports newly-created records only (it does not reconstruct status-change history).",
		Example:     "  atera-cli since 24h --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			window := "24h"
			if len(args) > 0 {
				window = args[0]
			}
			dur, ok := nvParseWindow(window)
			if !ok {
				return fmt.Errorf("invalid window %q: use a form like 90m, 24h, 7d, or 2d12h", window)
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

			if !hintIfUnsynced(cmd, s, "") {
				hintIfStale(cmd, s, "", flags.maxAge)
			}

			now := nvNow()
			cutoff := now.Add(-dur)
			rep := sinceReport{
				Window: window,
				Since:  cutoff.UTC().Format("2006-01-02T15:04:05Z"),
				Items:  make([]sinceItem, 0),
			}

			// Each entity contributes records whose creation timestamp is within the window.
			collect := func(resource, kind, tsField, nameField string) error {
				objs, err := nvLoad(s, resource)
				if err != nil {
					return fmt.Errorf("loading %s: %w", resource, err)
				}
				for _, o := range objs {
					t, ok := nvTime(o, tsField)
					if !ok || t.Before(cutoff) {
						continue
					}
					id, _ := nvInt(o, kindIDField(kind))
					rep.Items = append(rep.Items, sinceItem{
						Kind:     kind,
						ID:       id,
						Name:     nvStr(o, nameField),
						Customer: nvStr(o, "CustomerName"),
						When:     nvStr(o, tsField),
						whenT:    t,
					})
					switch kind {
					case "agent":
						rep.NewAgents++
					case "ticket":
						rep.NewTickets++
					case "alert":
						rep.NewAlerts++
					}
				}
				return nil
			}

			if err := collect("agents", "agent", "Created", "AgentName"); err != nil {
				return err
			}
			if err := collect("tickets", "ticket", "TicketCreatedDate", "TicketTitle"); err != nil {
				return err
			}
			if err := collect("alerts", "alert", "Created", "Title"); err != nil {
				return err
			}

			// Newest first — compare parsed instants; raw strings sort wrong
			// across mixed timestamp formats.
			sort.SliceStable(rep.Items, func(i, j int) bool {
				return rep.Items[i].whenT.After(rep.Items[j].whenT)
			})

			return nvEmit(cmd, flags, rep, func() {
				w := cmd.OutOrStdout()
				fmt.Fprintf(w, "Since %s (%s): %d agents, %d tickets, %d alerts\n\n",
					window, rep.Since, rep.NewAgents, rep.NewTickets, rep.NewAlerts)
				if len(rep.Items) == 0 {
					return
				}
				rows := make([][]string, 0, len(rep.Items))
				for _, it := range rep.Items {
					rows = append(rows, []string{it.Kind, fmt.Sprintf("%d", it.ID), it.Customer, it.Name, it.When})
				}
				nvTable(w, []string{"KIND", "ID", "CUSTOMER", "NAME", "WHEN"}, rows)
			})
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/atera-cli/data.db)")
	return cmd
}

// kindIDField maps a since item kind to the Atera ID field on its DTO.
func kindIDField(kind string) string {
	switch kind {
	case "agent":
		return "AgentID"
	case "ticket":
		return "TicketID"
	case "alert":
		return "AlertID"
	default:
		return "ID"
	}
}
