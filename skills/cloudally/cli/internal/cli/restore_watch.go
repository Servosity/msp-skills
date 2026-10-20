// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"cloudally-pp-cli/internal/cadate"
	"cloudally-pp-cli/internal/cliutil"
	"github.com/spf13/cobra"
)

const (
	defaultWatchInterval = 30 * time.Second
	defaultWatchTimeout  = 2 * time.Hour
	// watchCreateSlack tolerates clock skew between submit and the API's createDate.
	watchCreateSlack = 2 * time.Minute
	// unmatchedAfterPolls is how many polls a submitted row may go without a task.
	unmatchedAfterPolls = 2
)

// watchTally is the per-poll count line.
type watchTally struct {
	Running   int `json:"running"`
	Finished  int `json:"finished"`
	Failed    int `json:"failed"`
	Unmatched int `json:"unmatched"`
	Unknown   int `json:"unknown"`
	Pending   int `json:"pending"`
	// Unsent counts rows still PLANNED: never submitted, so the batch is
	// incomplete and watch cannot finish it.
	Unsent int `json:"unsent"`
}

// jobOutcomeUnknown reports a row whose submission was never confirmed.
// Watch can settle it by matching a RESTORE task, but never marks it
// UNMATCHED: it stays UNKNOWN so --retry-unknown can still pick it up.
func jobOutcomeUnknown(j restoreJob) bool {
	o := j.submitOutcome()
	return o == jobUnknown || o == jobSubmitting
}

// watchOutput is the JSON document 'restore watch' prints.
type watchOutput struct {
	BatchID string       `json:"batch_id"`
	Polls   int          `json:"polls"`
	Done    bool         `json:"done"`
	Tally   watchTally   `json:"tally"`
	Rows    []restoreJob `json:"rows"`
}

// jobSubmittedForMatch reports whether a ledger row has a restore to find:
// one the API accepted or may have, decided on the submission outcome, so
// a row whose RESTORE task already reported FAILED keeps its task.
func jobSubmittedForMatch(j restoreJob) bool {
	switch j.submitOutcome() {
	case jobSubmitted, jobUnknown, jobSubmitting:
		return j.SubmittedAt != nil
	}
	return false
}

// matchRestoreTasks pairs ledger rows with RESTORE tasks: same entity
// (case-insensitive), same source, created no earlier than submitted_at
// minus the slack. A task that reports a snapshotDate only ever matches a
// row whose resolved backup is the same backup (sameBackup: within
// sameBackupWindow), so a task another batch created for another date of
// the same mailbox is never taken for this row. Rows whose resolved backup
// matches a task's snapshotDate claim it first; a confirmed row may then
// take the earliest unclaimed task that reports no snapshotDate. An
// UNKNOWN or SUBMITTING row settles only on a snapshotDate match: settling
// it on a guess would stop --retry-unknown from ever sending a restore that
// never happened.
func matchRestoreTasks(jobs []restoreJob, tasks []restoreTaskView, source string) map[int64]*restoreTaskView {
	type candidate struct {
		task    *restoreTaskView
		created time.Time
	}
	var cands []candidate
	for i := range tasks {
		t := &tasks[i]
		if source != "" && t.Source != "" && !strings.EqualFold(t.Source, source) {
			continue
		}
		created, err := cadate.ParseLoose(t.CreateDate)
		if err != nil {
			continue
		}
		cands = append(cands, candidate{task: t, created: created})
	}
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].created.Before(cands[j].created) })
	ordered := make([]restoreJob, 0, len(jobs))
	for _, j := range jobs {
		if jobSubmittedForMatch(j) {
			ordered = append(ordered, j)
		}
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].SubmittedAt.Equal(*ordered[j].SubmittedAt) {
			return ordered[i].ID < ordered[j].ID
		}
		return ordered[i].SubmittedAt.Before(*ordered[j].SubmittedAt)
	})
	claimed := map[*restoreTaskView]bool{}
	matched := map[int64]*restoreTaskView{}
	eligible := func(j restoreJob, c candidate) bool {
		if snap := int64(c.task.SnapshotDate); snap != 0 && !sameBackup(snap, j.ResolvedRaw) {
			return false
		}
		return !claimed[c.task] && strings.EqualFold(strings.TrimSpace(c.task.Account), strings.TrimSpace(j.Entity)) &&
			!c.created.Before(j.SubmittedAt.Add(-watchCreateSlack))
	}
	for _, j := range ordered {
		if j.TaskID != "" {
			for _, c := range cands {
				if string(c.task.ID) == j.TaskID && !claimed[c.task] {
					claimed[c.task], matched[j.ID] = true, c.task
					break
				}
			}
			continue
		}
		for _, c := range cands {
			if eligible(j, c) && int64(c.task.SnapshotDate) != 0 {
				claimed[c.task], matched[j.ID] = true, c.task
				break
			}
		}
	}
	for _, j := range ordered {
		if _, ok := matched[j.ID]; ok || jobOutcomeUnknown(j) {
			continue
		}
		for _, c := range cands {
			if eligible(j, c) {
				claimed[c.task], matched[j.ID] = true, c.task
				break
			}
		}
	}
	return matched
}

// jobTerminal reports whether a row needs no further polling. A row whose
// submission is unconfirmed (UNKNOWN, or SUBMITTING from a run that may
// still be sending) is not finished: watch keeps looking for its RESTORE
// task until the timeout.
func jobTerminal(j restoreJob) bool {
	if jobOutcomeUnknown(j) && strings.TrimSpace(j.TaskID) == "" {
		return false
	}
	switch j.Status {
	case jobPlanned, jobFailed, jobFailedRedirect, jobUnmatched:
		return true
	case jobSubmitted:
		return false
	}
	return !restoreTaskRunning(j.Status)
}

func tallyJobs(jobs []restoreJob) watchTally {
	var t watchTally
	for _, j := range jobs {
		switch {
		case j.Status == jobFailed || j.Status == jobFailedRedirect:
			t.Failed++
		case j.Status == jobUnmatched:
			t.Unmatched++
		case jobOutcomeUnknown(j):
			t.Unknown++
		case j.Status == jobPlanned:
			t.Unsent++
		case j.Status == jobSubmitted:
			t.Pending++
		case restoreTaskRunning(j.Status):
			t.Running++
		default:
			t.Finished++
		}
	}
	return t
}

func printWatchTable(cmd *cobra.Command, poll int, jobs []restoreJob, t watchTally) error {
	rows := make([][]string, 0, len(jobs))
	for _, j := range jobs {
		rows = append(rows, []string{j.Entity, j.Requested, j.Status, j.TaskID, j.Progress})
	}
	if err := printRestoreTable(cmd.OutOrStdout(), []string{"entity", "requested", "status", "task_id", "progress"}, rows); err != nil {
		return err
	}
	_, err := termf(cmd.OutOrStdout(), "poll %d: %d running, %d finished, %d failed, %d unmatched, %d unknown, %d pending, %d never submitted (%s)\n\n",
		poll, t.Running, t.Finished, t.Failed, t.Unmatched, t.Unknown, t.Pending, t.Unsent, time.Now().UTC().Format(time.RFC3339))
	return err
}

func newRestoreWatchCmd(flags *rootFlags) *cobra.Command {
	var flagBatch string
	var flagInterval, flagMaxWait time.Duration
	var flagOnce bool

	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Follow one submitted batch until every restore finishes",
		Long: strings.Trim(`
Use this command to follow the restores of one submitted batch to completion. Do NOT use
it to list raw RESTORE tasks; use 'tasks list --type RESTORE' instead. Do NOT use it for
past batches offline; use 'restore history' instead.

Exits 0 only when every row of the batch was submitted and its RESTORE task finished.
It exits non-zero when a restore failed, when a row is still UNKNOWN (watch keeps
polling those until --max-wait), and when a row was never submitted (still PLANNED:
the batch is incomplete; continue it with 'restore submit ... --resume'). --timeout
bounds each status request; --max-wait bounds the whole watch.
`, "\n"),
		Example: strings.Trim(`
  cloudally-cli restore watch --batch 20261015-120000-ab12
  cloudally-cli restore watch --batch 20261015-120000-ab12 --interval 1m --max-wait 4h
  cloudally-cli restore watch --batch 20261015-120000-ab12 --once --json
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":  "true",
			"pp:data-source": "live",
			"pp:happy-args":  "--batch=20261015-120000-ab12;--once",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if !hasChangedLocalFlags(cmd) && len(args) == 0 {
				return cmd.Help()
			}
			if err := validateDataSourceStrategy(flags, "live"); err != nil {
				return usageErr(err)
			}
			if strings.TrimSpace(flagBatch) == "" {
				return usageErr(errors.New("--batch <batch id> is required"))
			}
			if err := requireBoundStoreScope(); err != nil {
				return err
			}
			if flagInterval <= 0 {
				return usageErr(errors.New("--interval must be greater than 0"))
			}
			if cliutil.IsDogfoodEnv() {
				flagOnce = true
			}
			if restoreShortCircuit(flags) {
				would := fmt.Sprintf("load batch %s from the ledger and poll its RESTORE tasks every %s until finished or %s", flagBatch, flagInterval, flagMaxWait)
				if flagOnce {
					would = fmt.Sprintf("load batch %s from the ledger and poll its RESTORE tasks once", flagBatch)
				}
				return writeWould(cmd.OutOrStdout(), flags, "restore watch", would, map[string]any{"batch_id": flagBatch})
			}
			ctx := cmd.Context()
			db, err := openLedger(ctx)
			if err != nil {
				return fmt.Errorf("opening ledger: %w", err)
			}
			defer db.Close()
			sqlDB := db.DB()
			batch, err := ledgerGetBatch(sqlDB, flagBatch)
			if err != nil {
				return fmt.Errorf("reading ledger: %w", err)
			}
			if batch == nil {
				return usageErr(fmt.Errorf("unknown batch %q; list batches with 'cloudally-cli restore history'", flagBatch))
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			scope := scopeFromKind(batch.Scope, batch.AccountID)
			// --max-wait bounds the watch; --timeout (the root flag)
			// bounds each request through the client.
			deadline := time.Now().Add(flagMaxWait)
			human := wantsHumanTable(cmd.OutOrStdout(), flags)
			out := watchOutput{BatchID: batch.BatchID}
			for {
				out.Polls++
				jobs, err := ledgerListJobs(sqlDB, batch.BatchID, "")
				if err != nil {
					return fmt.Errorf("reading ledger: %w", err)
				}
				tasks, err := fetchRestoreTasks(ctx, c, scope)
				if err != nil {
					return classifyAPIError(cmd.OutOrStdout(), err, flags)
				}
				matched := matchRestoreTasks(jobs, tasks, batch.Source)
				for i := range jobs {
					j := &jobs[i]
					if !jobSubmittedForMatch(*j) && j.Status != jobUnmatched {
						continue
					}
					// The task status only ever replaces what the row
					// shows; the submission outcome is kept, except that a
					// found task settles an unconfirmed row as SUBMITTED.
					t, found := matched[j.ID]
					if found {
						j.Status, j.TaskID, j.Progress = strings.ToUpper(strings.TrimSpace(t.Status)), string(t.ID), t.Progress.String()
						if jobOutcomeUnknown(*j) {
							j.SubmitStatus = jobSubmitted
						}
					} else if j.TaskID == "" && out.Polls >= unmatchedAfterPolls && !jobOutcomeUnknown(*j) {
						j.Status = jobUnmatched
					} else {
						continue
					}
					if err := ledgerUpdateJobWatch(sqlDB, j.ID, j.Status, j.TaskID, j.Progress, found); err != nil {
						termf(cmd.ErrOrStderr(), "warning: ledger update failed for %s: %v\n", j.Entity, err)
					}
				}
				_ = ledgerTouchBatch(sqlDB, batch.BatchID, time.Now())
				out.Rows, out.Tally = jobs, tallyJobs(jobs)
				finished := true
				for _, j := range jobs {
					if !jobTerminal(j) {
						finished = false
						break
					}
				}
				// A batch with rows never submitted is not done, even when
				// nothing is left to poll.
				out.Done = finished && out.Tally.Unsent == 0
				if human {
					if err := printWatchTable(cmd, out.Polls, jobs, out.Tally); err != nil {
						return err
					}
				}
				if finished || flagOnce {
					break
				}
				if flagMaxWait > 0 && time.Now().After(deadline) {
					if !human {
						_ = printJSONFiltered(cmd.OutOrStdout(), out, flags)
					}
					return apiErr(fmt.Errorf("gave up after --max-wait %s with %d restore(s) still running, %d still unconfirmed (UNKNOWN), and %d never submitted in batch %s", flagMaxWait, out.Tally.Running+out.Tally.Pending, out.Tally.Unknown, out.Tally.Unsent, batch.BatchID))
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(flagInterval):
				}
			}
			if out.Tally.Unmatched > 0 {
				termf(cmd.ErrOrStderr(), "warning: %d row(s) have no matching RESTORE task after %d poll(s)\n", out.Tally.Unmatched, out.Polls)
			}
			if out.Tally.Unknown > 0 {
				termf(cmd.ErrOrStderr(), "warning: %d UNKNOWN row(s) have no matching RESTORE task yet; if they stay missing, re-run the 'restore submit' command that created batch %s with --resume --retry-unknown --yes added\n", out.Tally.Unknown, batch.BatchID)
			}
			if !human {
				if err := printJSONFiltered(cmd.OutOrStdout(), out, flags); err != nil {
					return err
				}
			}
			var problems []string
			if out.Tally.Failed > 0 {
				var afterAccept []string
				for _, j := range out.Rows {
					if (j.Status == jobFailed || j.Status == jobFailedRedirect) && jobEverAccepted(j) {
						afterAccept = append(afterAccept, j.Entity)
					}
				}
				if len(afterAccept) == 0 {
					problems = append(problems, fmt.Sprintf("%d restore(s) failed in batch %s", out.Tally.Failed, batch.BatchID))
				} else {
					problems = append(problems, fmt.Sprintf("%d restore(s) failed in batch %s; %d of them failed after CloudAlly accepted them and may have restored part of the mailbox, so neither 'restore submit --resume' nor a new batch sends them again on its own. Check what they restored first; to restore just those mailboxes a second time on purpose, run the original 'restore submit' with --entities %s --new-batch",
						out.Tally.Failed, batch.BatchID, len(afterAccept), termSafe(shellQuote(strings.Join(afterAccept, ",")))))
				}
			}
			if out.Tally.Unknown > 0 {
				problems = append(problems, fmt.Sprintf("%d restore(s) in batch %s are still UNKNOWN: no RESTORE task was found for them, so it is not known whether CloudAlly accepted them", out.Tally.Unknown, batch.BatchID))
			}
			if out.Tally.Unsent > 0 {
				problems = append(problems, fmt.Sprintf("batch %s is incomplete: %d restore(s) were never submitted (still PLANNED); send them by re-running the 'restore submit' command that created the batch with --resume --yes added", batch.BatchID, out.Tally.Unsent))
			}
			if len(problems) > 0 {
				return apiErr(errors.New(strings.Join(problems, "; ")))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagBatch, "batch", "", "Batch id printed by 'restore submit'")
	cmd.Flags().DurationVar(&flagInterval, "interval", defaultWatchInterval, "Time between polls")
	cmd.Flags().DurationVar(&flagMaxWait, "max-wait", defaultWatchTimeout, "Give up watching after this long (0 waits without limit); --timeout bounds each request")
	cmd.Flags().BoolVar(&flagOnce, "once", false, "Poll one time and exit")
	return cmd
}
