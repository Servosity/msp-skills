// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"cloudally-pp-cli/internal/client"
	"github.com/spf13/cobra"
)

// jobIdentity is how a batch row is recognised across runs: one mailbox,
// one requested date. The resolved backup is deliberately not part of it.
type jobIdentity struct {
	entity    string
	requested string
}

func identityOf(entity, requested string) jobIdentity {
	return jobIdentity{entity: strings.ToLower(strings.TrimSpace(entity)), requested: strings.TrimSpace(requested)}
}

// runSubmit records the batch, posts the rows, and prints the outcome.
// --resume searches by the account-free resume key, so a batch recorded
// for the same scope, task, entity filter, and dates under another account
// is refused instead of being silently replaced by a new batch.
func runSubmit(cmd *cobra.Command, flags *rootFlags, c *client.Client, req submitRequest) error {
	ctx := cmd.Context()
	stderr := cmd.ErrOrStderr()
	db, err := openLedger(ctx)
	if err != nil {
		return fmt.Errorf("opening ledger: %w", err)
	}
	defer db.Close()
	sqlDB := db.DB()
	kind := req.scope.kind()
	key := batchKey(kind, req.account.id, req.taskID, req.filter, req.dateTexts)
	rkey := resumeKey(kind, req.taskID, req.filter, req.dateTexts)
	var batch *restoreBatch
	resumed := false
	if req.resume {
		existing, err := ledgerFindBatchByResumeKey(sqlDB, rkey)
		if err != nil {
			return fmt.Errorf("reading ledger: %w", err)
		}
		if existing == nil {
			// Batches recorded before resume_key existed are found by the
			// fingerprint they were written with.
			names := make([]string, 0, len(req.rows))
			for _, r := range req.rows {
				names = append(names, r.Entity)
			}
			if existing, err = ledgerFindBatchByKey(sqlDB, batchKey(kind, req.scope.account, req.taskID, names, req.dateTexts)); err != nil {
				return fmt.Errorf("reading ledger: %w", err)
			}
		}
		if existing != nil {
			if existing.AccountID != req.account.id {
				return usageErr(fmt.Errorf("batch %s was recorded for account %q but the current account is %q; refusing to resume it under a different account (switch back with `auth use-account %s` or --account %s)",
					existing.BatchID, existing.AccountID, req.account.id, existing.AccountID, existing.AccountID))
			}
			batch, resumed = existing, true
		} else {
			return usageErr(errors.New("--resume found no earlier batch with the same scope, account, task, entities, and dates; nothing was sent. Check `restore history`, or drop --resume to start a new batch"))
		}
	}
	if batch == nil {
		now := time.Now()
		batch = &restoreBatch{BatchID: newBatchID(now), BatchKey: key, ResumeKey: rkey, CreatedAt: now.UTC(), Scope: kind,
			AccountID: req.account.id, AuthScope: req.account.authScope, TaskID: req.taskID, Source: req.source, Label: req.label,
			Dates: req.dateTexts, Options: req.opts.asMap()}
		if err := ledgerInsertBatch(sqlDB, *batch); err != nil {
			return fmt.Errorf("recording batch: %w", err)
		}
	}
	if n, err := ledgerRecoverInFlight(sqlDB, batch.BatchID); err != nil {
		return fmt.Errorf("reading ledger: %w", err)
	} else if n > 0 {
		fmt.Fprintf(stderr, "warning: %d row(s) in batch %s were still SUBMITTING from a run that did not finish; they are now UNKNOWN\n", n, batch.BatchID)
	}
	existingJobs, err := ledgerListJobs(sqlDB, batch.BatchID, "")
	if err != nil {
		return fmt.Errorf("reading ledger: %w", err)
	}
	have := map[jobIdentity]restoreJob{}
	haveEntity := map[string]bool{}
	for _, j := range existingJobs {
		have[identityOf(j.Entity, j.Requested)] = j
		haveEntity[strings.ToLower(strings.TrimSpace(j.Entity))] = true
	}
	var added []string
	addedSeen := map[string]bool{}
	for _, r := range req.rows {
		if j, ok := have[identityOf(r.Entity, r.Requested)]; ok {
			if j.ResolvedRaw != r.ResolvedRaw {
				fmt.Fprintf(stderr, "note: %s %s now resolves to backup %s; the batch keeps its recorded backup %s\n", r.Entity, r.Requested, r.Resolved, j.Resolved)
			}
			continue
		}
		if err := ledgerInsertJob(sqlDB, restoreJob{BatchID: batch.BatchID, Entity: r.Entity, ExtID: r.ExtID, Requested: r.Requested, ResolvedRaw: r.ResolvedRaw, Resolved: r.Resolved, SubSource: r.SubSource}); err != nil {
			return fmt.Errorf("recording row: %w", err)
		}
		e := strings.ToLower(strings.TrimSpace(r.Entity))
		if resumed && !haveEntity[e] && !addedSeen[e] {
			addedSeen[e] = true
			added = append(added, r.Entity)
		}
	}
	if len(added) > 0 {
		fmt.Fprintf(stderr, "resume: %d entit(ies) not in batch %s were added: %s\n", len(added), batch.BatchID, strings.Join(added, ", "))
	}
	jobs, err := ledgerListJobs(sqlDB, batch.BatchID, "")
	if err != nil {
		return fmt.Errorf("reading ledger: %w", err)
	}
	var pending, held []restoreJob
	skipped := 0
	for _, j := range jobs {
		switch {
		case j.Status == jobPlanned || j.Status == jobFailed:
			pending = append(pending, j)
		case j.Status == jobUnknown && req.retryUnknown:
			pending = append(pending, j)
		case j.Status == jobUnknown:
			held = append(held, j)
			skipped++
		default:
			skipped++
		}
	}
	if len(held) > 0 {
		printUnknownNotice(stderr, batch.BatchID, held)
	}
	out := submitOutput{BatchID: batch.BatchID, BatchKey: batch.BatchKey, Resumed: resumed, Scope: kind, AccountID: batch.AccountID,
		AuthScope: batch.AuthScope, TaskID: req.taskID, Source: req.source, Total: len(jobs), Skipped: skipped, HeldUnknown: len(held),
		AddedEntities: added, FetchFailures: []submitFailure{}}
	authFailed := false
	var unrecorded []string
	var newUnknown []restoreJob
	// Workers claim rows and the result loop records outcomes; one mutex
	// keeps those ledger writes in order.
	var ledgerMu sync.Mutex
	beforeSend := func(j restoreJob) error {
		ledgerMu.Lock()
		defer ledgerMu.Unlock()
		return ledgerMarkSubmitting(sqlDB, j.ID, time.Now())
	}
	runSubmitPool(ctx, c, req.restorePath, req.taskID, req.source, pending, req.opts, req.concurrency, req.rate, beforeSend, func(res submitResult) {
		if res.notSent {
			out.NotSent++
			out.FetchFailures = append(out.FetchFailures, submitFailure{Entity: res.job.Entity, Requested: res.job.Requested, Status: res.status, Error: res.errText})
			return
		}
		ledgerMu.Lock()
		err := ledgerUpdateJobSubmit(sqlDB, res.job.ID, res.status, res.httpStatus, res.response, res.errText, res.at)
		ledgerMu.Unlock()
		if err != nil {
			fmt.Fprintf(stderr, "warning: ledger update failed for %s (%s): %v\n", res.job.Entity, res.status, err)
			if res.status == jobSubmitted {
				unrecorded = append(unrecorded, res.job.Entity)
			}
		}
		switch res.status {
		case jobSubmitted:
			out.Submitted++
			return
		case jobUnknown:
			out.Unknown++
			j := res.job
			j.Status, j.Error = jobUnknown, res.errText
			newUnknown = append(newUnknown, j)
		default:
			out.Failed++
		}
		if res.authFailure {
			authFailed = true
		}
		out.FetchFailures = append(out.FetchFailures, submitFailure{Entity: res.job.Entity, Requested: res.job.Requested, HTTPStatus: res.httpStatus, Status: res.status, Error: res.errText})
	})
	if rows, err := ledgerListJobs(sqlDB, batch.BatchID, ""); err != nil {
		fmt.Fprintf(stderr, "warning: reading ledger rows for batch %s: %v\n", batch.BatchID, err)
	} else {
		out.Rows = rows
	}
	out.Counts = countJobStatuses(out.Rows)
	if out.Failed+out.Unknown+out.NotSent > 0 {
		fmt.Fprintf(stderr, "warning: %d of %d submissions did not succeed (%d failed, %d unknown, %d not sent)\n", out.Failed+out.Unknown+out.NotSent, len(pending), out.Failed, out.Unknown, out.NotSent)
	}
	if len(newUnknown) > 0 {
		printUnknownNotice(stderr, batch.BatchID, newUnknown)
	}
	if wantsHumanTable(cmd.OutOrStdout(), flags) {
		table := make([][]string, 0, len(out.Rows))
		for _, j := range out.Rows {
			table = append(table, []string{j.Entity, j.Requested, j.Resolved, j.Status, strings.TrimSpace(truncate(j.Error, 60))})
		}
		if err := printRestoreTable(cmd.OutOrStdout(), []string{"entity", "requested", "resolved", "status", "error"}, table); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "\nbatch %s (account %s): %d submitted, %d failed, %d unknown, %d skipped. Follow with: cloudally-cli restore watch --batch %s\n",
			batch.BatchID, req.account.label(), out.Submitted, out.Failed, out.Unknown, out.Skipped, batch.BatchID)
	} else if err := printJSONFiltered(cmd.OutOrStdout(), out, flags); err != nil {
		return err
	}
	unsettled := out.Counts[jobFailed] + out.Counts[jobFailedRedirect] + out.Counts[jobUnknown] + out.Counts[jobSubmitting]
	switch {
	case len(unrecorded) > 0:
		return apiErr(fmt.Errorf("%d restore(s) were accepted but the ledger still shows them SUBMITTING in batch %s (%s); the next run treats them as UNKNOWN and will not send them again", len(unrecorded), batch.BatchID, strings.Join(unrecorded, ", ")))
	case authFailed:
		return authErr(fmt.Errorf("restore submission rejected with 401/403; remaining rows left PLANNED in batch %s", batch.BatchID))
	case out.NotSent > 0 || unsettled > 0:
		return apiErr(fmt.Errorf("batch %s is not fully submitted: %d failed, %d unknown, %d not sent (counts: %s)", batch.BatchID,
			out.Counts[jobFailed]+out.Counts[jobFailedRedirect], out.Counts[jobUnknown]+out.Counts[jobSubmitting], out.NotSent, formatCounts(out.Counts)))
	}
	return nil
}

// printUnknownNotice lists rows whose outcome is unknown and says how to
// settle them without restoring a mailbox twice.
func printUnknownNotice(w io.Writer, batchID string, rows []restoreJob) {
	for _, j := range rows {
		fmt.Fprintf(w, "  UNKNOWN  %s  requested %s  backup %s  %s\n", j.Entity, j.Requested, j.Resolved, strings.TrimSpace(truncate(j.Error, 80)))
	}
	fmt.Fprintf(w, "these %d restores may already have been accepted; check 'restore watch --batch %s --once' and re-run with --retry-unknown only if they are missing\n", len(rows), batchID)
}

func countJobStatuses(rows []restoreJob) map[string]int {
	counts := map[string]int{}
	for _, j := range rows {
		counts[j.Status]++
	}
	return counts
}

func formatCounts(counts map[string]int) string {
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", k, counts[k]))
	}
	return strings.Join(parts, " ")
}
