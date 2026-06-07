// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-implemented novel feature for skykick-cli (stub replaced; survives regen-merge).
package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"skykick-pp-cli/internal/fleet"
)

type driftView struct {
	Meta       fleetEnvelopeMeta `json:"meta"`
	ComparedTo fleetEnvelopeMeta `json:"compared_to"`
	Drift      fleet.DriftReport `json:"drift"`
	Findings   int               `json:"findings"`
	Note       string            `json:"note,omitempty"`
}

func newNovelDriftCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var staleHours int

	cmd := &cobra.Command{
		Use:   "drift",
		Short: "Diff the two newest fleet-sync runs and report protection-state changes",
		Long: strings.Trim(`
Use this to see what changed between the two most recent fleet-sync runs:
added/removed subscriptions, Exchange/SharePoint/autodiscover enablement
flips, newly-stale mailboxes, mailbox protection flips, and retention changes.
Only transitions between KNOWN states count - parse-coverage noise
(unknown->known) is never reported as drift. With fewer than two completed
runs it reports that honestly instead of fabricating a diff. For current-state
posture without history, use 'fleet-health'.
`, "\n"),
		Example: strings.Trim(`
  skykick-cli drift --agent
  skykick-cli drift --stale-hours 24 --agent
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would diff the two newest fleet-sync runs in the local store")
				return nil
			}
			if staleHours <= 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--stale-hours must be a positive number of hours"))
			}
			db, err := openFleetStore(cmdContext(cmd), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()

			prev, cur, prevRun, curRun, err := latestTwoFleetStates(cmdContext(cmd), db)
			if err != nil {
				return err
			}
			if prevRun.ID == 0 {
				view := driftView{
					Meta:  metaForRun(curRun),
					Drift: fleet.Drift(cur, cur, time.Duration(staleHours)*time.Hour, time.Now().UTC()),
					Note:  "only one completed fleet-sync run exists; nothing to compare yet - run fleet-sync again later to enable drift",
				}
				return fleetPrint(cmd, flags, view, nil, view.Note)
			}

			rep := fleet.Drift(prev, cur, time.Duration(staleHours)*time.Hour, time.Now().UTC())
			// Partial-run guard: a --limit'd or dogfood-curtailed sync stores
			// only a slice of the fleet. Comparing subscription presence
			// across a partial boundary would report every unsynced tenant as
			// added/removed - parse-coverage noise, not real-world change.
			if prevRun.Partial() || curRun.Partial() {
				rep.AddedSubscriptions = []string{}
				rep.RemovedSubscriptions = []string{}
			}
			findings := len(rep.AddedSubscriptions) + len(rep.RemovedSubscriptions) + len(rep.EnablementFlips) +
				len(rep.NewlyStaleMailboxes) + len(rep.MailboxFlips) + len(rep.RetentionChanges)
			view := driftView{
				Meta:       metaForRun(curRun),
				ComparedTo: metaForRun(prevRun),
				Drift:      rep,
				Findings:   findings,
			}
			if prevRun.Partial() || curRun.Partial() {
				view.Note = fmt.Sprintf("run %d and/or run %d was a partial (--limit'd) sync; added/removed-subscription detection suppressed - only tenants present in both runs are compared", prevRun.ID, curRun.ID)
			} else if rep.Empty() {
				view.Note = fmt.Sprintf("no drift between run %d and run %d", prevRun.ID, curRun.ID)
			}

			var humanRows []map[string]any
			for _, f := range rep.EnablementFlips {
				humanRows = append(humanRows, map[string]any{"company": f.Company, "what": f.What, "change": f.From + " -> " + f.To, "name": f.Name})
			}
			for _, f := range rep.MailboxFlips {
				humanRows = append(humanRows, map[string]any{"company": f.Company, "what": f.What, "change": f.From + " -> " + f.To, "name": f.Name})
			}
			for _, s := range rep.NewlyStaleMailboxes {
				humanRows = append(humanRows, map[string]any{"company": s.Company, "what": "newly_stale", "change": "", "name": s.Mailbox})
			}
			summary := fmt.Sprintf("%d drift findings between run %d and run %d", findings, prevRun.ID, curRun.ID)
			return fleetPrint(cmd, flags, view, humanRows, summary)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default ~/.local/share/skykick-cli/data.db)")
	cmd.Flags().IntVar(&staleHours, "stale-hours", 48, "Staleness threshold used for the newly-stale-mailboxes comparison")
	return cmd
}
