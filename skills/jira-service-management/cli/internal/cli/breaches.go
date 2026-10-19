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

	"jira-service-management-pp-cli/internal/cliutil"
)

type breachRow struct {
	Key     string   `json:"key"`
	Summary string   `json:"summary,omitempty"`
	DeskID  string   `json:"desk_id"`
	Queues  []string `json:"queues"`
	slaRisk
}

// queueSummary counts, per queue, tickets that passed the filters (before
// --limit trims the list): Breached, and Running (an SLA not yet breached,
// within --within if set; paused ones only with --include-paused).
type queueSummary struct {
	DeskID   string `json:"desk_id"`
	QueueID  string `json:"queue_id"`
	Queue    string `json:"queue"`
	Issues   int    `json:"issues"`
	Breached int    `json:"breached"`
	Running  int    `json:"running"`
}

type breachesView struct {
	Items         []breachRow    `json:"items"`
	Queues        []queueSummary `json:"queues"`
	ScannedIssues int            `json:"scanned_issues"`
	Within        string         `json:"within,omitempty"`
	Truncated     bool           `json:"truncated,omitempty"`
	Note          string         `json:"note,omitempty"`
	FetchFailures []fetchFailure `json:"fetch_failures,omitempty"`
}

type queueIssue struct {
	Key    string `json:"key"`
	Fields struct {
		Summary string `json:"summary"`
	} `json:"fields"`
}

func newNovelBreachesCmd(flags *rootFlags) *cobra.Command {
	var desks, queues, within string
	var limit, maxScanPages int
	var includePaused bool

	cmd := &cobra.Command{
		Use:   "breaches",
		Short: "Rank open tickets with a running SLA across your queues by time remaining, breached first, each counted once.",
		Long: strings.Trim(`
Walks every queue on the chosen service desks, de-duplicates tickets that sit in
several queues, reads each ticket's SLA metrics, and ranks them by the least time
remaining on any ongoing SLA. Breached tickets come first. A per-queue footer
counts issues, breached and still-running tickets. Desks or queues the account
cannot read are skipped and listed under fetch_failures.

Use this command for live, right-now SLA risk on open tickets across queues.
Do NOT use this command for historical breach rates over a period; use 'sla-report' instead.
Do NOT use this command for one client organization's ticket picture; use 'client' instead.`, "\n"),
		Example: strings.Trim(`
  jira-service-management-cli breaches --desk 3 --within 2h --agent
  jira-service-management-cli breaches --desk 3,4 --queue 10 --limit 20`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "breaches")
			}
			var withinDur time.Duration
			if within != "" {
				d, err := cliutil.ParseDurationLoose(within)
				if err != nil || d < 0 {
					return usageErr(fmt.Errorf("--within %q: want a duration like 2h, 30m or 1d", within))
				}
				withinDur = d
			}
			if limit < 1 || maxScanPages < 1 {
				return usageErr(fmt.Errorf("--limit and --max-scan-pages must be at least 1"))
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			if cliutil.IsDogfoodEnv() {
				maxScanPages = 1
			}

			deskIDs := cliutil.SplitCSV(desks)
			autoDesks := len(deskIDs) == 0
			for _, d := range deskIDs {
				if _, err := jsmPathSeg(d); err != nil {
					return usageErr(fmt.Errorf("--desk: %w", err))
				}
			}
			if autoDesks {
				items, _, err := jsmWalk(ctx, c, "/servicedesk", nil, 0, jsonKey("id"))
				if err != nil {
					return classifyAPIError(cmd.OutOrStdout(), err, flags)
				}
				for _, raw := range items {
					deskIDs = append(deskIDs, jsonKey("id")(raw))
				}
			}
			queueIDs := cliutil.SplitCSV(queues)
			queueFilter := map[string]bool{}
			for _, q := range queueIDs {
				queueFilter[q] = true
			}
			queueSeen := map[string]bool{}
			listedDesks := 0

			view := breachesView{Items: make([]breachRow, 0), Queues: make([]queueSummary, 0), Within: within}
			deskFailures := make([]fetchFailure, 0)
			rows := map[string]*breachRow{}
			order := make([]string, 0)
			queueOfKey := map[string][]int{} // ticket -> indexes into view.Queues
			for _, desk := range deskIDs {
				deskSeg, err := jsmPathSeg(desk)
				if err != nil {
					deskFailures = append(deskFailures, fetchFailure{ID: "desk " + desk, Error: err.Error(), err: err})
					continue
				}
				qItems, _, err := jsmWalk(ctx, c, "/servicedesk/"+deskSeg+"/queue", nil, 0, jsonKey("id"))
				if err != nil {
					// Auto-listed desks include ones the account is only a
					// customer on; skip unreadable desks instead of aborting
					// the scan (fatal below only if nothing was readable).
					deskFailures = append(deskFailures, fetchFailure{ID: "desk " + desk, Error: err.Error(), err: err})
					continue
				}
				listedDesks++
				for _, rawQ := range qItems {
					var q struct {
						ID   flexString `json:"id"`
						Name string     `json:"name"`
					}
					if json.Unmarshal(rawQ, &q) != nil {
						continue
					}
					if len(queueFilter) > 0 && !queueFilter[string(q.ID)] {
						continue
					}
					queueSeen[string(q.ID)] = true
					qSeg, err := jsmPathSeg(string(q.ID))
					if err != nil {
						continue
					}
					issues, truncated, err := jsmWalk(ctx, c, "/servicedesk/"+deskSeg+"/queue/"+qSeg+"/issue", nil, maxScanPages, jsonKey("key"))
					if err != nil {
						deskFailures = append(deskFailures, fetchFailure{ID: "queue " + string(q.ID), Error: err.Error(), err: err})
						continue
					}
					view.Truncated = view.Truncated || truncated
					qi := len(view.Queues)
					view.Queues = append(view.Queues, queueSummary{DeskID: desk, QueueID: string(q.ID), Queue: q.Name, Issues: len(issues)})
					for _, rawI := range issues {
						var is queueIssue
						if json.Unmarshal(rawI, &is) != nil || is.Key == "" {
							continue
						}
						if _, ok := rows[is.Key]; !ok {
							rows[is.Key] = &breachRow{Key: is.Key, Summary: is.Fields.Summary, DeskID: desk}
							order = append(order, is.Key)
						}
						rows[is.Key].Queues = append(rows[is.Key].Queues, q.Name)
						queueOfKey[is.Key] = append(queueOfKey[is.Key], qi)
					}
				}
			}
			// A --queue id found on no readable desk is a typo, not "all clear".
			var unknownQueues []string
			for _, q := range queueIDs {
				if !queueSeen[q] {
					unknownQueues = append(unknownQueues, q)
				}
			}
			// Unless every desk failed (then the real error is reported below).
			if len(queueFilter) > 0 && len(queueSeen) == 0 && (listedDesks > 0 || len(deskFailures) == 0) {
				return usageErr(fmt.Errorf("no queue %s on the readable desks (%d desks/queues unreadable); list them with 'servicedesk queue list <desk-id>'", strings.Join(unknownQueues, ","), len(deskFailures)))
			}
			for _, q := range unknownQueues {
				deskFailures = append(deskFailures, fetchFailure{ID: "queue " + q, Error: "no such queue on the readable desks"})
			}
			// Fatal only when nothing at all was readable; otherwise report
			// what was skipped and rank what was read.
			if len(view.Queues) == 0 && len(deskFailures) > 0 {
				return classifyAPIError(cmd.OutOrStdout(), deskFailures[0].err, flags)
			}
			if len(deskFailures) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d desks/queues failed and were skipped; see fetch_failures\n", len(deskFailures))
			}
			view.ScannedIssues = len(order)
			if cliutil.IsDogfoodEnv() && len(order) > 10 {
				order = order[:10]
			}

			slas, failures := fanOut(ctx, order, 4, func(ctx context.Context, key string) ([]jsmSLA, error) {
				return fetchSLAs(ctx, c, key)
			})
			view.FetchFailures = append(deskFailures, failures...)
			if err := allFailed(failures, len(order)); err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			if len(failures) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of %d SLA fetches failed; ranking covers the remaining %d tickets\n", len(failures), len(order), len(order)-len(failures))
			}
			view.Items = rankBreaches(order, rows, slas, queueOfKey, view.Queues, withinDur, includePaused, limit)
			if len(view.Items) == 0 {
				view.Note = fmt.Sprintf("no open tickets with a running SLA matched across %d scanned tickets", view.ScannedIssues)
				if view.Truncated {
					view.Note += "; queue scan hit --max-scan-pages, raise it to look further"
				}
			}
			if view.Truncated && len(view.Items) > 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "warning: a queue scan hit --max-scan-pages; tickets beyond it were not ranked")
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			w := cmd.OutOrStdout()
			if len(view.Items) == 0 {
				fmt.Fprintln(w, view.Note)
			}
			for _, r := range view.Items {
				state := "ok"
				if r.Breached {
					state = "BREACHED"
				}
				fmt.Fprintf(w, "%-12s %-9s %-10s %-28s %s\n", termSafe(r.Key), state, termSafe(r.Remaining), truncateText(r.Metric, 28), truncateText(r.Summary, 60))
			}
			fmt.Fprintln(w)
			for _, q := range view.Queues {
				fmt.Fprintf(w, "desk %s / %-24s issues=%d breached=%d running=%d\n", termSafe(q.DeskID), truncateText(q.Queue, 24), q.Issues, q.Breached, q.Running)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&desks, "desk", "", "Service desk ids to scan, comma-separated (default: every desk you can see)")
	cmd.Flags().StringVar(&queues, "queue", "", "Only these queue ids, comma-separated")
	cmd.Flags().StringVar(&within, "within", "", "Only tickets breached or breaching within this window (e.g. 2h, 30m, 1d); empty ranks every running SLA")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum tickets to return")
	cmd.Flags().IntVar(&maxScanPages, "max-scan-pages", 10, "Maximum 50-ticket pages to read per queue")
	cmd.Flags().BoolVar(&includePaused, "include-paused", false, "Include tickets whose most urgent SLA is paused (already-breached paused SLAs are always included)")
	return cmd
}

// rankBreaches keeps tickets with a running SLA (paused-only ones unless
// includePaused), drops those outside within (0 = keep all), counts the
// survivors per queue, and returns them most urgent first, capped at limit.
func rankBreaches(order []string, rows map[string]*breachRow, slas map[string][]jsmSLA, queueOfKey map[string][]int, queues []queueSummary, within time.Duration, includePaused bool, limit int) []breachRow {
	items := make([]breachRow, 0)
	for _, key := range order {
		s, ok := slas[key]
		if !ok {
			continue
		}
		risk, found := mostUrgent(s)
		if !found || (risk.Paused && !risk.Breached && !includePaused) {
			continue
		}
		if within > 0 && !risk.Breached && risk.RemainingMS > within.Milliseconds() {
			continue
		}
		row := *rows[key]
		row.slaRisk = risk
		items = append(items, row)
		for _, qi := range queueOfKey[key] {
			if risk.Breached {
				queues[qi].Breached++
			} else {
				queues[qi].Running++
			}
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return urgentBefore(items[i].slaRisk, items[j].slaRisk) })
	if len(items) > limit {
		items = items[:limit]
	}
	return items
}
