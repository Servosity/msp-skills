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
	"github.com/spf13/pflag"
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
// is refused instead of being silently replaced by a new batch, and sends
// the remaining rows with the options the batch recorded. Without --resume,
// a batch for the same key that already holds accepted or unconfirmed rows
// is refused unless --new-batch says to send everything again.
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
	// Batches recorded before resume_key existed, or before the account
	// was recorded, are found by the fingerprint they were written with.
	names := make([]string, 0, len(req.rows))
	for _, r := range req.rows {
		names = append(names, r.Entity)
	}
	legacyKey := batchKey(kind, req.scope.account, req.taskID, names, req.dateTexts)
	var batch *restoreBatch
	resumed := false
	if req.resume {
		existing, err := ledgerFindBatchByResumeKey(sqlDB, rkey)
		if err != nil {
			return fmt.Errorf("reading ledger: %w", err)
		}
		if existing == nil {
			if existing, err = ledgerFindBatchByKey(sqlDB, legacyKey); err != nil {
				return fmt.Errorf("reading ledger: %w", err)
			}
		}
		if existing != nil {
			if existing.AccountID != req.account.id {
				return usageErr(fmt.Errorf("batch %s was recorded for account %q but the current account is %q; refusing to resume it under a different account (switch back with `auth use-account %s` or --account %s)",
					existing.BatchID, termSafe(existing.AccountID), termSafe(req.account.id), termSafe(existing.AccountID), termSafe(existing.AccountID)))
			}
			opts, source, err := resumeSettings(existing, req)
			if err != nil {
				return err
			}
			req.opts, req.source = opts, source
			batch, resumed = existing, true
		} else {
			return usageErr(errors.New("--resume found no earlier batch with the same scope, account, task, entities, and dates; nothing was sent. Check `restore history`, or drop --resume to start a new batch"))
		}
	} else if !req.newBatch {
		prior, n, err := ledgerAcceptedRowsByKey(sqlDB, key, legacyKey)
		if err != nil {
			return fmt.Errorf("reading ledger: %w", err)
		}
		if n > 0 {
			return usageErr(fmt.Errorf("batch %s already has %d restore(s) for this same scope, account, task, entities, and dates that the API accepted or may have accepted; nothing was sent. Continue that batch with: %s (or add --new-batch to start a second batch that sends every restore again)",
				prior, n, termSafe(submitRerunCommand(cmd, "--resume", "--yes"))))
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
		termf(stderr, "warning: %d row(s) in batch %s were still SUBMITTING from a run that did not finish; they are now UNKNOWN\n", n, batch.BatchID)
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
				termf(stderr, "note: %s %s now resolves to backup %s; the batch keeps its recorded backup %s\n", r.Entity, r.Requested, r.Resolved, j.Resolved)
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
		termf(stderr, "resume: %d entit(ies) not in batch %s were added: %s\n", len(added), batch.BatchID, strings.Join(added, ", "))
	}
	jobs, err := ledgerListJobs(sqlDB, batch.BatchID, "")
	if err != nil {
		return fmt.Errorf("reading ledger: %w", err)
	}
	// Sending is decided on the submission outcome alone. A row whose
	// RESTORE task later reported FAILED was accepted and ran, possibly
	// restoring part of the mailbox, so it is never sent again here.
	var pending, held []restoreJob
	skipped := 0
	for _, j := range jobs {
		outcome := j.submitOutcome()
		switch {
		case strings.TrimSpace(j.TaskID) != "":
			skipped++
		case outcome == jobPlanned || outcome == jobFailed:
			pending = append(pending, j)
		case outcome == jobUnknown && req.retryUnknown:
			pending = append(pending, j)
		case outcome == jobUnknown:
			held = append(held, j)
			skipped++
		default:
			skipped++
		}
	}
	retryCmd := submitRerunCommand(cmd, "--resume", "--retry-unknown", "--yes")
	if len(held) > 0 {
		printUnknownNotice(stderr, batch.BatchID, retryCmd, held)
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
			termf(stderr, "warning: ledger update failed for %s (%s): %v\n", res.job.Entity, res.status, err)
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
			j.Status, j.SubmitStatus, j.Error = jobUnknown, jobUnknown, res.errText
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
		termf(stderr, "warning: reading ledger rows for batch %s: %v\n", batch.BatchID, err)
	} else {
		out.Rows = rows
	}
	out.Counts = countSubmitOutcomes(out.Rows)
	if out.Failed+out.Unknown+out.NotSent > 0 {
		termf(stderr, "warning: %d of %d submissions did not succeed (%d failed, %d unknown, %d not sent)\n", out.Failed+out.Unknown+out.NotSent, len(pending), out.Failed, out.Unknown, out.NotSent)
	}
	if len(newUnknown) > 0 {
		printUnknownNotice(stderr, batch.BatchID, retryCmd, newUnknown)
	}
	if wantsHumanTable(cmd.OutOrStdout(), flags) {
		table := make([][]string, 0, len(out.Rows))
		for _, j := range out.Rows {
			table = append(table, []string{j.Entity, j.Requested, j.Resolved, j.submitOutcome(), j.Status, strings.TrimSpace(truncate(j.Error, 60))})
		}
		if err := printRestoreTable(cmd.OutOrStdout(), []string{"entity", "requested", "resolved", "submit", "status", "error"}, table); err != nil {
			return err
		}
		termf(cmd.OutOrStdout(), "\nbatch %s (account %s): %d submitted, %d failed, %d unknown, %d skipped. Follow with: cloudally-cli restore watch --batch %s\n",
			batch.BatchID, req.account.label(), out.Submitted, out.Failed, out.Unknown, out.Skipped, batch.BatchID)
	} else if err := printJSONFiltered(cmd.OutOrStdout(), out, flags); err != nil {
		return err
	}
	unsettled := out.Counts[jobFailed] + out.Counts[jobFailedRedirect] + out.Counts[jobUnknown] + out.Counts[jobSubmitting]
	switch {
	case len(unrecorded) > 0:
		return apiErr(fmt.Errorf("%d restore(s) were accepted but the ledger still shows them SUBMITTING in batch %s (%s); the next run treats them as UNKNOWN and will not send them again", len(unrecorded), batch.BatchID, termSafe(strings.Join(unrecorded, ", "))))
	case authFailed:
		return authErr(fmt.Errorf("restore submission rejected with 401/403; remaining rows left PLANNED in batch %s", batch.BatchID))
	case out.NotSent > 0 || unsettled > 0:
		return apiErr(fmt.Errorf("batch %s is not fully submitted: %d failed, %d unknown, %d not sent (counts: %s)", batch.BatchID,
			out.Counts[jobFailed]+out.Counts[jobFailedRedirect], out.Counts[jobUnknown]+out.Counts[jobSubmitting], out.NotSent, formatCounts(out.Counts)))
	}
	return nil
}

// printUnknownNotice lists rows whose outcome is unknown and says how to
// settle them without restoring a mailbox twice: watch first, which settles
// every row it finds a RESTORE task for, then retryCmd, which sends only
// the rows still UNKNOWN.
func printUnknownNotice(w io.Writer, batchID, retryCmd string, rows []restoreJob) {
	for _, j := range rows {
		termf(w, "  UNKNOWN  %s  requested %s  backup %s  %s\n", j.Entity, j.Requested, j.Resolved, strings.TrimSpace(truncate(j.Error, 80)))
	}
	termf(w, "these %d restore(s) may already have been accepted and are not sent again on their own. First check them with: cloudally-cli restore watch --batch %s --once\n", len(rows), batchID)
	termf(w, "then, only if watch found no RESTORE task for them, send the ones still UNKNOWN with: %s\n", retryCmd)
}

// countSubmitOutcomes counts rows by submission outcome, so a RESTORE task
// that failed after the API accepted it counts as SUBMITTED here.
func countSubmitOutcomes(rows []restoreJob) map[string]int {
	counts := map[string]int{}
	for _, j := range rows {
		counts[j.submitOutcome()]++
	}
	return counts
}

// resumeSettings returns the restore options and source a resumed batch
// sends its remaining rows with: the ones the batch recorded. Where a
// restore lands is part of what the batch promised, so a flag passed on
// this run that disagrees with the recorded value is refused, and a batch
// whose options cannot be read is not resumed at all.
func resumeSettings(b *restoreBatch, req submitRequest) (submitOptions, string, error) {
	recorded, ok := submitOptionsFromMap(b.Options)
	if !ok {
		return submitOptions{}, "", usageErr(fmt.Errorf("batch %s has no complete record of its restore options (target account, restore option, include flags), so where its remaining restores would go cannot be confirmed; nothing was sent. Check 'restore history --batch %s' and submit the missing rows as a separate batch with --new-batch", b.BatchID, b.BatchID))
	}
	var conflicts []string
	differs := func(name string, rec, cur any) {
		if req.optsSet[name] && rec != cur {
			conflicts = append(conflicts, fmt.Sprintf("--%s %s (batch recorded %s)", name, optionText(cur), optionText(rec)))
		}
	}
	differs(flagNameTargetAccount, strings.TrimSpace(recorded.targetAccount), strings.TrimSpace(req.opts.targetAccount))
	differs(flagNameRestoreOption, strings.TrimSpace(recorded.restoreOption), strings.TrimSpace(req.opts.restoreOption))
	differs(flagNameIncludeDeleted, recorded.includeDeleted, req.opts.includeDeleted)
	differs(flagNameIncludeLegalHold, recorded.includeLegalHold, req.opts.includeLegalHold)
	if len(conflicts) > 0 {
		return submitOptions{}, "", usageErr(fmt.Errorf("batch %s sends its restores with different options than this run: %s; nothing was sent. Drop the flag(s) to resume with the recorded values, or start a separate batch with --new-batch instead of --resume",
			b.BatchID, termSafe(strings.Join(conflicts, ", "))))
	}
	source := req.source
	if recordedSource := strings.TrimSpace(b.Source); recordedSource != "" {
		if source != "" && !strings.EqualFold(recordedSource, source) {
			return submitOptions{}, "", usageErr(fmt.Errorf("batch %s was recorded for source %s but task %s now reports source %s; refusing to resume it; nothing was sent",
				b.BatchID, termSafe(recordedSource), termSafe(req.taskID), termSafe(source)))
		}
		source = recordedSource
	}
	return recorded, source, nil
}

// optionText renders an option value for a refusal message.
func optionText(v any) string {
	if s, ok := v.(string); ok {
		if s == "" {
			return "(not set: the API default)"
		}
		return fmt.Sprintf("%q", s)
	}
	return fmt.Sprint(v)
}

// submitRerunCommand rebuilds this invocation as a command line that can be
// pasted back: the command path and every flag that was set, minus the
// ones that choose how a run treats an earlier batch (and --dry-run),
// followed by extra.
func submitRerunCommand(cmd *cobra.Command, extra ...string) string {
	skip := map[string]bool{"yes": true, "resume": true, "retry-unknown": true, "new-batch": true, "dry-run": true}
	parts := []string{cmd.CommandPath()}
	cmd.Flags().Visit(func(f *pflag.Flag) {
		if skip[f.Name] {
			return
		}
		if sv, ok := f.Value.(pflag.SliceValue); ok {
			for _, v := range sv.GetSlice() {
				parts = append(parts, "--"+f.Name, shellQuote(v))
			}
			return
		}
		if f.Value.Type() == "bool" {
			if f.Value.String() == "true" {
				parts = append(parts, "--"+f.Name)
			} else {
				parts = append(parts, "--"+f.Name+"=false")
			}
			return
		}
		parts = append(parts, "--"+f.Name, shellQuote(f.Value.String()))
	})
	return strings.Join(append(parts, extra...), " ")
}

// shellQuote leaves a plain value bare and single-quotes anything else.
func shellQuote(s string) string {
	plain := s != ""
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("@%_+=:,./-", r)) {
			plain = false
			break
		}
	}
	if plain {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
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
