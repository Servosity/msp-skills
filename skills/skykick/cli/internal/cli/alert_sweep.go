// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-implemented novel feature for skykick-cli (stub replaced; survives regen-merge).
package cli

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	"skykick-pp-cli/internal/fleet"
)

type alertSweepView struct {
	Meta           fleetEnvelopeMeta `json:"meta"`
	Alerts         []fleet.Alert     `json:"alerts"`
	Total          int               `json:"total"`
	BySubscription map[string]int    `json:"by_subscription"`
	Completed      []string          `json:"completed,omitempty"`
	WouldComplete  []string          `json:"would_complete,omitempty"`
	CompleteErrors []string          `json:"complete_errors,omitempty"`
	Note           string            `json:"note,omitempty"`
}

func newNovelAlertSweepCmd(flags *rootFlags) *cobra.Command {
	var dbPath, completeIDs string
	var apply bool
	var limit int

	cmd := &cobra.Command{
		Use:   "alert-sweep",
		Short: "One ranked list of open alerts across the whole fleet, with optional bulk mark-complete",
		Long: strings.Trim(`
Use this for the cross-fleet alert triage view: every alert captured by the
last fleet-sync, ranked critical-first then newest-first. The API only serves
alerts per-subscription-id; the sweep is the fleet-wide collation it cannot do.

Bulk closure: pass --complete with comma-separated alert ids. Without --apply
it only prints what would be completed (dry-run by default); with --apply it
POSTs each completion to the live API. Single-id alert reads/marks go through
'alerts list' / 'alerts complete'.
`, "\n"),
		Example: strings.Trim(`
  skykick-cli alert-sweep --agent
  skykick-cli alert-sweep --limit 50 --agent
  skykick-cli alert-sweep --complete id-1,id-2 --apply
`, "\n"),
		// Not read-only: --complete --apply mutates upstream alert state.
		Annotations: map[string]string{"mcp:read-only": "false"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would rank fleet alerts from the local store; with --complete --apply, would mark the named alerts complete upstream")
				return nil
			}
			db, err := openFleetStore(cmdContext(cmd), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()

			state, run, err := latestFleetState(cmdContext(cmd), db)
			if err != nil {
				return err
			}
			ranked := fleet.RankAlerts(state.Alerts)
			view := alertSweepView{
				Meta:           metaForRun(run),
				Total:          len(ranked),
				BySubscription: map[string]int{},
			}
			for _, a := range ranked {
				view.BySubscription[a.SubscriptionID]++
			}
			if limit > 0 && len(ranked) > limit {
				ranked = ranked[:limit]
			}
			view.Alerts = ranked
			if view.Alerts == nil {
				view.Alerts = []fleet.Alert{}
			}
			if view.Total == 0 {
				view.Note = fmt.Sprintf("no alerts captured in run %d; if alerts were skipped during fleet-sync (--skip alerts), re-run without it", run.ID)
			}

			// Bulk completion path.
			if completeIDs != "" {
				ids := []string{}
				for _, id := range strings.Split(completeIDs, ",") {
					v := strings.TrimSpace(id)
					if v == "" {
						continue
					}
					// Alert ids become a URL path segment on a WRITE call; a
					// stray '/' or dot-segment would silently re-target the
					// POST and fabricate a success record. Reject rather than
					// escape-and-hope.
					if strings.ContainsAny(v, "/\\?#") || strings.Contains(v, "..") {
						_ = cmd.Usage()
						return usageErr(fmt.Errorf("alert id %q contains path characters; pass the raw alert GUID", v))
					}
					ids = append(ids, v)
				}
				if len(ids) == 0 {
					_ = cmd.Usage()
					return usageErr(fmt.Errorf("--complete requires at least one alert id"))
				}
				if !apply {
					view.WouldComplete = ids
					view.Note = strings.TrimSpace(view.Note + " --complete provided without --apply: listed alerts NOT completed; add --apply to mark them complete upstream.")
				} else {
					c, err := flags.newClient()
					if err != nil {
						return err
					}
					for _, id := range ids {
						if _, _, err := c.Post(cmdContext(cmd), "/Alerts/"+url.PathEscape(id), nil); err != nil {
							view.CompleteErrors = append(view.CompleteErrors, fmt.Sprintf("%s: %v", id, err))
							continue
						}
						view.Completed = append(view.Completed, id)
					}
					if len(view.CompleteErrors) > 0 {
						fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of %d completions failed; see complete_errors\n", len(view.CompleteErrors), len(ids))
					}
				}
			}

			humanRows := make([]map[string]any, 0, len(view.Alerts))
			for _, a := range view.Alerts {
				created := ""
				if a.Created != nil {
					created = a.Created.Format("2006-01-02 15:04")
				}
				desc := a.Description
				if len(desc) > 60 {
					desc = desc[:57] + "..."
				}
				humanRows = append(humanRows, map[string]any{
					"severity": a.Severity, "created": created, "subscription": a.SubscriptionID, "id": a.ID, "description": desc,
				})
			}
			summary := fmt.Sprintf("%d alerts across %d subscriptions (run %d)", view.Total, len(view.BySubscription), run.ID)
			if len(view.Completed) > 0 {
				summary += fmt.Sprintf("; %d marked complete", len(view.Completed))
			}
			if err := fleetPrint(cmd, flags, view, humanRows, summary); err != nil {
				return err
			}
			if len(view.CompleteErrors) > 0 {
				return partialFailureErr(fmt.Errorf("%d of %d alert completions failed", len(view.CompleteErrors), len(view.Completed)+len(view.CompleteErrors)))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default ~/.local/share/skykick-cli/data.db)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum alerts to return (0 = all)")
	cmd.Flags().StringVar(&completeIDs, "complete", "", "Comma-separated alert ids to mark complete (requires --apply to take effect)")
	cmd.Flags().BoolVar(&apply, "apply", false, "Actually POST completions for --complete ids (otherwise prints would-complete)")
	return cmd
}
