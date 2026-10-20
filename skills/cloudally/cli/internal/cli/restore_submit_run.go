// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
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
//
// The ledger is per login, so on top of it every row passes the
// machine-wide restore guard (restore_guard.go): the whole run is refused
// up front when any restore it would send was already sent from this
// machine under any login or token, and each row is claimed atomically
// right before its POST so two racing runs cannot both send it.
func runSubmit(cmd *cobra.Command, flags *rootFlags, c *client.Client, req submitRequest) error {
	ctx := cmd.Context()
	stderr := cmd.ErrOrStderr()
	db, err := openLedger(ctx)
	if err != nil {
		return fmt.Errorf("opening ledger: %w", err)
	}
	defer db.Close()
	sqlDB := db.DB()
	guard, err := openRestoreGuard(ctx)
	if err != nil {
		return fmt.Errorf("opening the machine-wide restore guard: %w; nothing was sent", err)
	}
	defer guard.Close()
	owner := claimOwner{login: req.account.label(), accountID: req.account.id, scope: req.scope.kind()}
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
			recordedRows, err := ledgerListJobs(sqlDB, existing.BatchID, "")
			if err != nil {
				return fmt.Errorf("reading ledger: %w", err)
			}
			opts, source, err := resumeSettings(cmd, existing, recordedRows, req)
			if err != nil {
				return err
			}
			req.opts, req.source = opts, source
			batch, resumed = existing, true
		} else {
			return usageErr(errors.New("--resume found no earlier batch with the same scope, account, task, entities, and dates in this login's ledger; nothing was sent. Each login (partner, or one company after auth use-account) keeps its own ledger, so run `restore history` under the login that started the batch. Dropping --resume starts a new batch, which the machine-wide restore guard refuses for any restore already sent from this machine"))
		}
	} else if !req.newBatch {
		prior, n, err := ledgerAcceptedRowsByKey(sqlDB, key, legacyKey)
		if err != nil {
			return fmt.Errorf("reading ledger: %w", err)
		}
		if n > 0 {
			return usageErr(fmt.Errorf("batch %s already has %d restore(s) for this same scope, account, task, entities, and dates that the API accepted or may have accepted; nothing was sent. Continue that batch with: %s. --new-batch would instead send all of this run's restores again, those %d included",
				prior, n, termSafe(submitRerunCommand(cmd, "--resume", "--yes")), n))
		}
	}
	if batch == nil {
		// A new batch is checked against the machine-wide guard before it
		// is recorded, so a refused run leaves nothing behind.
		if err := guardPreflight(ctx, cmd, guard, req, owner, req.rows); err != nil {
			return err
		}
		now := time.Now()
		batch = &restoreBatch{BatchID: newBatchID(now), BatchKey: key, ResumeKey: rkey, CreatedAt: now.UTC(), Scope: kind,
			AccountID: req.account.id, AuthScope: req.account.authScope, TaskID: req.taskID, Source: req.source, Label: req.label,
			Dates: req.dateTexts, Options: req.opts.asMap()}
		if err := ledgerInsertBatch(sqlDB, *batch); err != nil {
			return fmt.Errorf("recording batch: %w", err)
		}
	}
	owner.batchID = batch.BatchID
	stillSending := func(j restoreJob) bool {
		return guard.inFlight(ctx, newRestoreIdentity(batch.TaskID, j.Entity, j.ResolvedRaw), batch.BatchID)
	}
	if n, busy, err := ledgerRecoverInFlight(sqlDB, batch.BatchID, stillSending); err != nil {
		return fmt.Errorf("reading ledger: %w", err)
	} else {
		if n > 0 {
			termf(stderr, "warning: %d row(s) in batch %s were still SUBMITTING from a run that did not finish; they are now UNKNOWN\n", n, batch.BatchID)
		}
		if busy > 0 {
			termf(stderr, "note: %d row(s) in batch %s are being sent by another submit that is still running; they are left alone\n", busy, batch.BatchID)
		}
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
	if resumed {
		toSend := make([]planRow, 0, len(pending))
		for _, j := range pending {
			toSend = append(toSend, planRow{Entity: j.Entity, Requested: j.Requested, ResolvedRaw: j.ResolvedRaw})
		}
		if err := guardPreflight(ctx, cmd, guard, req, owner, toSend); err != nil {
			return err
		}
	}
	out := submitOutput{BatchID: batch.BatchID, BatchKey: batch.BatchKey, Resumed: resumed, Scope: kind, AccountID: batch.AccountID,
		AuthScope: batch.AuthScope, TaskID: req.taskID, Source: req.source, Total: len(jobs), Skipped: skipped, HeldUnknown: len(held),
		AddedEntities: added, FetchFailures: []submitFailure{}}
	authFailed := false
	var unrecorded []string
	var newUnknown []restoreJob
	// Workers claim rows and the result loop records outcomes; one mutex
	// keeps those ledger and guard writes in order. A row is claimed in the
	// ledger first and then in the machine-wide guard, the last step before
	// its POST; a guard refusal puts the ledger row back unsent.
	var ledgerMu sync.Mutex
	claims := map[int64]int64{}
	beforeSend := func(j restoreJob) error {
		ledgerMu.Lock()
		defer ledgerMu.Unlock()
		if err := ledgerMarkSubmitting(sqlDB, j.ID, time.Now()); err != nil {
			return err
		}
		claimID, err := guard.claim(ctx, newRestoreIdentity(batch.TaskID, j.Entity, j.ResolvedRaw), owner, req.retryUnknown, req.newBatch)
		if err != nil {
			if rerr := ledgerRevertSubmitting(sqlDB, j); rerr != nil {
				termf(stderr, "warning: %s was not sent, but its ledger row could not be put back (%v); the next run treats it as UNKNOWN\n", j.Entity, rerr)
			}
			return err
		}
		claims[j.ID] = claimID
		return nil
	}
	runSubmitPool(ctx, c, req.restorePath, req.taskID, req.source, pending, req.opts, req.concurrency, req.rate, beforeSend, func(res submitResult) {
		if res.notSent {
			out.NotSent++
			out.FetchFailures = append(out.FetchFailures, submitFailure{Entity: res.job.Entity, Requested: res.job.Requested, Status: res.status, Error: res.errText})
			return
		}
		ledgerMu.Lock()
		// The guard is settled first: if this process dies between the two
		// writes, the guard already holds the outcome and the ledger row is
		// recovered as UNKNOWN, which never sends it again on its own.
		if claimID, ok := claims[res.job.ID]; ok {
			if gerr := guard.settle(context.Background(), claimID, res.status); gerr != nil {
				termf(stderr, "warning: the restore guard could not record %s for %s (%v); it stays held and later runs treat it as unknown\n", res.status, res.job.Entity, gerr)
			}
		}
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
// whose options cannot be read is not resumed at all. rows are the batch's
// ledger rows, for the advice on sending only what it never sent.
func resumeSettings(cmd *cobra.Command, b *restoreBatch, rows []restoreJob, req submitRequest) (submitOptions, string, error) {
	recorded, ok := submitOptionsFromMap(b.Options)
	if !ok {
		return submitOptions{}, "", usageErr(fmt.Errorf("batch %s has no complete record of its restore options (target account, restore option, include flags), so where its remaining restores would go cannot be confirmed; nothing was sent. Check 'restore history --batch %s'. %s",
			b.BatchID, b.BatchID, termSafe(neverSentAdvice(cmd, rows))))
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
		return submitOptions{}, "", usageErr(fmt.Errorf("batch %s sends its restores with different options than this run: %s; nothing was sent. Drop the flag(s) to resume with the recorded values. %s",
			b.BatchID, termSafe(strings.Join(conflicts, ", ")), termSafe(neverSentAdvice(cmd, rows))))
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

// neverSentAdvice is the way out of a refused resume that does not send an
// accepted restore again: a separate batch limited with --entities to the
// mailboxes this batch never sent, and what --new-batch would cost instead.
func neverSentAdvice(cmd *cobra.Command, rows []restoreJob) string {
	var order []string
	seen := map[string]bool{}
	sent := map[string]bool{}
	accepted := 0
	for _, j := range rows {
		e := strings.ToLower(strings.TrimSpace(j.Entity))
		if !seen[e] {
			seen[e] = true
			order = append(order, j.Entity)
		}
		if jobEverAccepted(j) {
			accepted++
			sent[e] = true
		}
	}
	var never []string
	for _, name := range order {
		if !sent[strings.ToLower(strings.TrimSpace(name))] {
			never = append(never, name)
		}
	}
	var b strings.Builder
	if len(never) > 0 {
		fmt.Fprintf(&b, "To send only the %d mailbox(es) this batch never sent, as a separate batch with this run's options: %s.", len(never), submitRerunCommandEntities(cmd, never, "--yes"))
	} else {
		b.WriteString("Every mailbox in this batch was already sent, so there is nothing left to send.")
	}
	if accepted > 0 {
		fmt.Fprintf(&b, " Do not use --new-batch here: it would send the %d restore(s) the API accepted or may have accepted again.", accepted)
	}
	return b.String()
}

// guardPreflight checks every row a run is about to send against the
// machine-wide restore guard and refuses the whole run, before anything is
// sent or recorded, when any of them is already accepted, unknown, or in
// flight from another batch, login, token, or process on this machine.
// Under --new-batch only a send in flight right now refuses; what
// --new-batch is about to send again is reported on stderr.
func guardPreflight(ctx context.Context, cmd *cobra.Command, g *restoreGuard, req submitRequest, owner claimOwner, rows []planRow) error {
	ids := make([]restoreIdentity, len(rows))
	for i, r := range rows {
		ids[i] = newRestoreIdentity(req.taskID, r.Entity, r.ResolvedRaw)
	}
	held, err := g.conflicts(ctx, ids, owner, req.retryUnknown, req.newBatch)
	if err != nil {
		return fmt.Errorf("reading the machine-wide restore guard %s: %w; nothing was sent", g.path, err)
	}
	if req.newBatch {
		again, err := g.conflicts(ctx, ids, owner, req.retryUnknown, false)
		if err != nil {
			return fmt.Errorf("reading the machine-wide restore guard %s: %w; nothing was sent", g.path, err)
		}
		if n := len(again) - len(held); n > 0 {
			termf(cmd.ErrOrStderr(), "warning: --new-batch sends %d restore(s) again that were already sent from this machine (%s)\n", n, describeClaims(again, held))
		}
	}
	if len(held) == 0 {
		return nil
	}
	var never []string
	seen := map[string]bool{}
	heldEntity := map[string]bool{}
	for i := range rows {
		if _, ok := held[ids[i]]; ok {
			heldEntity[ids[i].Entity] = true
		}
	}
	for i, r := range rows {
		if e := ids[i].Entity; !heldEntity[e] && !seen[e] {
			seen[e] = true
			never = append(never, r.Entity)
		}
	}
	accepted := 0
	for _, c := range held {
		if c.State == claimAccepted {
			accepted++
		}
	}
	advice := "None of this run's mailboxes is free of them, so there is nothing else to send."
	if len(never) > 0 {
		advice = fmt.Sprintf("To send only the %d mailbox(es) with none of these restores sent: %s.", len(never), submitRerunCommandEntities(cmd, never, "--yes"))
	}
	newBatch := fmt.Sprintf(" --new-batch would send these %d again, %d of them already accepted; use it only to restore those mailboxes a second time on purpose.", len(held), accepted)
	if req.newBatch {
		newBatch = " A send that is in flight right now is refused even with --new-batch; wait for it to finish."
	}
	return usageErr(fmt.Errorf("%d of the %d restore(s) this run would send were already sent from this machine, under this or another login or token: %s. Nothing was sent; sending them again would restore those mailboxes twice. Look the batches up with 'restore history --batch <id>' under the login named. %s%s (restore guard: %s)",
		len(held), len(rows), termSafe(describeClaims(held, nil)), termSafe(advice), newBatch, g.path))
}

// describeClaims summarises claims by state and by batch, leaving out the
// identities in skip.
func describeClaims(claims, skip map[restoreIdentity]guardClaim) string {
	states := map[string]int{}
	type batchKey struct{ batch, login string }
	perBatch := map[batchKey]int{}
	for id, c := range claims {
		if _, ok := skip[id]; ok {
			continue
		}
		states[c.State]++
		perBatch[batchKey{c.BatchID, c.Login}]++
	}
	var parts []string
	for _, st := range []string{claimAccepted, claimUnknown, claimSending} {
		if n := states[st]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, claimStateText(st)))
		}
	}
	keys := make([]batchKey, 0, len(perBatch))
	for k := range perBatch {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].batch < keys[j].batch })
	var batches []string
	for _, k := range keys {
		batches = append(batches, fmt.Sprintf("batch %s (login %s): %d", k.batch, k.login, perBatch[k]))
	}
	return strings.Join(parts, ", ") + "; " + strings.Join(batches, "; ")
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
	return strings.Join(append([]string{submitRerunCommandSkipping(cmd, nil)}, extra...), " ")
}

func submitRerunCommandSkipping(cmd *cobra.Command, more map[string]bool) string {
	skip := map[string]bool{"yes": true, "resume": true, "retry-unknown": true, "new-batch": true, "dry-run": true}
	for k := range more {
		skip[k] = true
	}
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
	return strings.Join(parts, " ")
}

// submitRerunCommandEntities is submitRerunCommand for a new batch limited
// to entities: --entities and --entities-file are replaced, and --resume,
// --retry-unknown, and --new-batch are left out.
func submitRerunCommandEntities(cmd *cobra.Command, entities []string, extra ...string) string {
	base := submitRerunCommandSkipping(cmd, map[string]bool{"entities": true, "entities-file": true})
	return strings.Join(append([]string{base, "--entities", shellQuote(strings.Join(entities, ","))}, extra...), " ")
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
