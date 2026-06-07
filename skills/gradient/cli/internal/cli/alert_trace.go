// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command (Phase 3); survives regeneration as a whole file.

// pp:data-source local

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"gradient-pp-cli/internal/ledger"
)

type alertTraceView struct {
	Alerts     []ledger.AlertRecord `json:"alerts"`
	Total      int                  `json:"total"`
	StuckCount int                  `json:"stuck_count"`
	Note       string               `json:"note,omitempty"`
}

func newNovelAlertTraceCmd(flags *rootFlags) *cobra.Command {
	var flagStuck bool
	var flagLimit int

	cmd := &cobra.Command{
		Use:   "trace",
		Short: "Review dispatched alerts and find ones that never became tickets",
		Long: strings.Trim(`
Use this command to review past alert dispatches and find ones that never
became tickets. Do NOT use this command to dispatch a new alert; use
'alert send'.

The Synthesize API has no list route for dispatched alerts; the local alert
ledger written by 'alert send' is the only record. Each entry carries the
messageId, account, dispatch time, and last-known ticket state. --stuck
filters to alerts still pending or timed out - the queue-debugging work
queue.
`, "\n"),
		Example: strings.Trim(`
  gradient-cli alert trace --agent
  gradient-cli alert trace --stuck
  gradient-cli alert trace --limit 10 --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("alert trace takes no positional arguments"))
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would list dispatched alerts from the local alert ledger")
				return nil
			}
			if flags.dataSource == "live" {
				return fmt.Errorf("alert trace has no live equivalent: the API has no list-my-alerts route; this command reads the local alert ledger (use 'ticket-events <messageId>' for one live lookup)")
			}

			dir, err := ledger.Dir()
			if err != nil {
				return err
			}
			alerts, err := ledger.ReadAlerts(dir)
			if err != nil {
				return err
			}

			view := alertTraceView{Alerts: []ledger.AlertRecord{}}
			view.Total = len(alerts)
			for _, a := range alerts {
				stuck := a.TicketStatus == "pending" || a.TicketStatus == "timeout"
				if stuck {
					view.StuckCount++
				}
				if flagStuck && !stuck {
					continue
				}
				view.Alerts = append(view.Alerts, a)
			}
			// Newest first; the ledger appends in dispatch order.
			for i, j := 0, len(view.Alerts)-1; i < j; i, j = i+1, j-1 {
				view.Alerts[i], view.Alerts[j] = view.Alerts[j], view.Alerts[i]
			}
			if flagLimit > 0 && len(view.Alerts) > flagLimit {
				view.Alerts = view.Alerts[:flagLimit]
			}
			if view.Total == 0 {
				view.Note = "alert ledger is empty; dispatch with 'alert send' to start recording"
			}
			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().BoolVar(&flagStuck, "stuck", false, "Only alerts still pending or timed out (never became tickets)")
	cmd.Flags().IntVar(&flagLimit, "limit", 25, "Maximum alerts to return, newest first (0 = all)")
	return cmd
}
