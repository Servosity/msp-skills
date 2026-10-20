// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"cloudally-pp-cli/internal/cadate"
	"cloudally-pp-cli/internal/cliutil"
	"cloudally-pp-cli/internal/store"
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
	// Unrecognized counts restores whose task ended with a status that is
	// neither on the success allow-list nor a known failure; they need a
	// person and are never counted as a success.
	Unrecognized int `json:"unrecognized"`
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

// matchRestoreTasks pairs ledger rows with RESTORE tasks; it is
// matchRestoreTasksExcluding with no task pinned anywhere else.
func matchRestoreTasks(jobs []restoreJob, tasks []restoreTaskView, source string) map[int64]*restoreTaskView {
	return matchRestoreTasksExcluding(jobs, tasks, source, nil)
}

// matchRestoreTasksExcluding pairs ledger rows with RESTORE tasks: same
// entity (case-insensitive), same source, created no earlier than
// submitted_at minus the slack.
//
//   - A row that already recorded a task id (a pinned row) only ever takes
//     that task again; it is never re-matched to another.
//   - A task in pinnedElsewhere (recorded on a row of another batch, in
//     any restore ledger in this data directory) is never a candidate: a
//     deliberate --new-batch re-restore must not read the earlier batch's
//     task, and the reverse.
//   - A task that reports a snapshotDate only ever matches a row whose
//     resolved backup is the same backup (sameBackup).
//   - Among the remaining candidates, tasks created at or after the row's
//     submitted_at are preferred; one created inside the slack before it
//     counts only when none was created after. When more than one is still
//     eligible the row is not matched at all, never a guess: it stays
//     SUBMITTED and becomes UNMATCHED (or stays UNKNOWN), so watch exits 5
//     rather than reporting another restore's result.
//
// Rows whose resolved backup matches a task's snapshotDate claim first; a
// confirmed row may then take a task that reports no snapshotDate. An
// UNKNOWN or SUBMITTING row settles only on a snapshotDate match: settling
// it on a guess would stop --retry-unknown from ever sending a restore that
// never happened.
func matchRestoreTasksExcluding(jobs []restoreJob, tasks []restoreTaskView, source string, pinnedElsewhere map[string]bool) map[int64]*restoreTaskView {
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
		if pinnedElsewhere[strings.TrimSpace(string(c.task.ID))] {
			return false
		}
		if snap := int64(c.task.SnapshotDate); snap != 0 && !sameBackup(snap, j.ResolvedRaw) {
			return false
		}
		return !claimed[c.task] && strings.EqualFold(strings.TrimSpace(c.task.Account), strings.TrimSpace(j.Entity)) &&
			!c.created.Before(j.SubmittedAt.Add(-watchCreateSlack))
	}
	// pick returns the one task this row can be matched to, or nil when
	// there is none or more than one (an ambiguous row is never guessed).
	pick := func(j restoreJob, snapshotOnly bool) *restoreTaskView {
		// createDate carries whole seconds, so the row's own task, created
		// in the same second it was submitted, counts as at or after it.
		submitted := j.SubmittedAt.Truncate(time.Second)
		var atOrAfter, inSlack []*restoreTaskView
		for _, c := range cands {
			if !eligible(j, c) || (snapshotOnly && int64(c.task.SnapshotDate) == 0) {
				continue
			}
			if c.created.Before(submitted) {
				inSlack = append(inSlack, c.task)
			} else {
				atOrAfter = append(atOrAfter, c.task)
			}
		}
		switch {
		case len(atOrAfter) == 1:
			return atOrAfter[0]
		case len(atOrAfter) == 0 && len(inSlack) == 1:
			return inSlack[0]
		}
		return nil
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
		if t := pick(j, true); t != nil {
			claimed[t], matched[j.ID] = true, t
		}
	}
	for _, j := range ordered {
		if _, ok := matched[j.ID]; ok || jobOutcomeUnknown(j) || j.TaskID != "" {
			continue
		}
		if t := pick(j, false); t != nil {
			claimed[t], matched[j.ID] = true, t
		}
	}
	return matched
}

// restoreTasksPinnedElsewhere returns every RESTORE task id recorded on a
// row of another batch: in this batch's own ledger (db), and in every other
// restore ledger store in the data directory (other logins and tokens),
// read without migrating them. A store that cannot be read fails the watch
// closed, because a task it pins could otherwise be taken for this batch.
func restoreTasksPinnedElsewhere(ctx context.Context, db *sql.DB, batchID string) (map[string]bool, error) {
	pinned := map[string]bool{}
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT task_id FROM restore_jobs WHERE batch_id <> ? AND COALESCE(task_id, '') <> ''`, batchID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, err
		}
		pinned[strings.TrimSpace(id)] = true
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	own, _ := filepath.Abs(defaultDBPath(restoreCLIName))
	paths, err := listLedgerStores(ledgerDataDir())
	if err != nil {
		return nil, err
	}
	for _, path := range paths {
		if abs, aerr := filepath.Abs(path); aerr == nil && abs == own {
			continue
		}
		ids, err := pinnedTaskIDsInStore(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("the restore ledger %s cannot be read, so a RESTORE task another batch already owns could be taken for this one (%w); fix or move that file, then watch again", path, err)
		}
		for _, id := range ids {
			pinned[id] = true
		}
	}
	return pinned, nil
}

// pinnedTaskIDsInStore reads the RESTORE task ids recorded in one ledger
// store, read-only. A store without restore tables records none.
func pinnedTaskIDsInStore(ctx context.Context, path string) ([]string, error) {
	st, err := store.OpenReadOnlyContext(ctx, path)
	if err != nil {
		return nil, err
	}
	defer st.Close()
	db := st.DB()
	var tables int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'restore_jobs'`).Scan(&tables); err != nil {
		return nil, err
	}
	if tables == 0 || !ledgerHasColumn(db, "restore_jobs", "task_id") {
		return nil, nil
	}
	q, err := db.QueryContext(ctx, `SELECT DISTINCT task_id FROM restore_jobs WHERE COALESCE(task_id, '') <> ''`)
	if err != nil {
		return nil, err
	}
	defer q.Close()
	var out []string
	for q.Next() {
		var id string
		if err := q.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, strings.TrimSpace(id))
	}
	return out, q.Err()
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

// restoreTaskFailed reports a row whose submission or RESTORE task ended
// badly: FAILED and FAILED_REDIRECT, and any task status restoreTaskOutcome
// reads as a failure (a failure word, a partial result, an abort, a stop).
func restoreTaskFailed(status string) bool {
	return restoreTaskOutcome(status) == taskFailed
}

// unrecognizedStatuses lists the raw statuses tallyJobs counted as
// unrecognised, with how many rows carry each, so a person sees exactly
// what CloudAlly reported.
func unrecognizedStatuses(jobs []restoreJob) string {
	counts := map[string]int{}
	for _, j := range jobs {
		if tallyJobs([]restoreJob{j}).Unrecognized == 1 {
			counts[j.Status]++
		}
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("status %q on %d", k, counts[k]))
	}
	return strings.Join(parts, ", ")
}

func tallyJobs(jobs []restoreJob) watchTally {
	var t watchTally
	for _, j := range jobs {
		switch {
		case restoreTaskFailed(j.Status):
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
		case restoreTaskOutcome(j.Status) == taskSucceeded:
			t.Finished++
		default:
			t.Unrecognized++
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
	_, err := termf(cmd.OutOrStdout(), "poll %d: %d running, %d finished, %d failed, %d unmatched, %d unknown, %d pending, %d never submitted, %d unrecognised status (%s)\n\n",
		poll, t.Running, t.Finished, t.Failed, t.Unmatched, t.Unknown, t.Pending, t.Unsent, t.Unrecognized, time.Now().UTC().Format(time.RFC3339))
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

Exits 0 only when every row of the batch was submitted and its RESTORE task finished
with a success status (COMPLETED, COMPLETE, SUCCEEDED, SUCCEED, SUCCESS, DONE,
FINISHED). Exits 5 when a restore failed or ended partly done, aborted, stopped, or
cancelled; when its task reports a status this CLI does not recognise (the raw status
is printed; it is never read as success); when a row is still UNKNOWN (watch keeps
polling those until --max-wait); when a row was never submitted (still PLANNED: the
batch is incomplete; continue it with 'restore submit ... --resume'); and when a
restore was accepted but no RESTORE task was ever seen for it (UNMATCHED). Exits 8
when restores are still running (PENDING, QUEUED, SCHEDULED, IN_PROCESS, IN_PROGRESS,
PAUSED and the like): with --once, after --max-wait, or on Ctrl-C; run watch again later. --timeout bounds each status request; --max-wait
bounds the whole watch.
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
			// finished is true once no row needs another poll; stopped names
			// why watch stopped before that (--once, --max-wait, Ctrl-C).
			finished, stopped := false, ""
			for {
				out.Polls++
				jobs, err := ledgerListJobs(sqlDB, batch.BatchID, "")
				if err != nil {
					return fmt.Errorf("reading ledger: %w", err)
				}
				tasks, err := fetchRestoreTasks(ctx, c, scope)
				if err != nil {
					// Ctrl-C (or SIGTERM) while the status request is in
					// flight is an interrupted watch, exactly like one during
					// the interval wait: keep the last tally and say how to
					// watch again. Only an error with a live context is an
					// API failure.
					if ctx.Err() == nil && !errors.Is(err, context.Canceled) {
						return classifyAPIError(cmd.OutOrStdout(), err, flags)
					}
					out.Polls--
					if out.Polls == 0 {
						// Interrupted on the first poll: the ledger's own
						// state is the last tally there is.
						out.Rows, out.Tally = jobs, tallyJobs(jobs)
					}
					finished, out.Done = false, false
					cause := err
					if ctx.Err() != nil {
						cause = ctx.Err()
					}
					stopped = "interrupted: " + cause.Error()
					break
				}
				pinned, err := restoreTasksPinnedElsewhere(ctx, sqlDB, batch.BatchID)
				if err != nil {
					return err
				}
				matched := matchRestoreTasksExcluding(jobs, tasks, batch.Source, pinned)
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
				finished = true
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
				if finished {
					break
				}
				if flagOnce {
					stopped = "--once"
					break
				}
				if flagMaxWait > 0 && time.Now().After(deadline) {
					stopped = "--max-wait " + flagMaxWait.String()
					break
				}
				select {
				case <-ctx.Done():
					stopped = "interrupted: " + ctx.Err().Error()
				case <-time.After(flagInterval):
				}
				if stopped != "" {
					break
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
					if restoreTaskFailed(j.Status) && jobEverAccepted(j) {
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
			if out.Tally.Unrecognized > 0 {
				problems = append(problems, fmt.Sprintf("%d restore(s) in batch %s ended with a status this CLI does not recognise as success (%s); they are not counted as successes: check them with 'cloudally-cli tasks list --type RESTORE' and in the mailboxes before doing anything else",
					out.Tally.Unrecognized, batch.BatchID, termSafe(unrecognizedStatuses(out.Rows))))
			}
			if out.Tally.Unmatched > 0 {
				problems = append(problems, fmt.Sprintf("%d restore(s) in batch %s were accepted by CloudAlly but no RESTORE task was seen for them after %d poll(s); do not send them again: look for them with 'cloudally-cli tasks list --type RESTORE' and check the mailboxes", out.Tally.Unmatched, batch.BatchID, out.Polls))
			}
			if len(problems) > 0 {
				return apiErr(errors.New(strings.Join(problems, "; ")))
			}
			if !finished {
				return &cliError{code: ExitJobPending, err: fmt.Errorf("%d restore(s) in batch %s are still running (%d running, %d submitted with no task seen yet); watch stopped (%s) before they finished. Check again later with: cloudally-cli restore watch --batch %s",
					out.Tally.Running+out.Tally.Pending, batch.BatchID, out.Tally.Running, out.Tally.Pending, stopped, batch.BatchID)}
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
