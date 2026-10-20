// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http/httptrace"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"cloudally-pp-cli/internal/cadate"
	"cloudally-pp-cli/internal/cliutil"
	"cloudally-pp-cli/internal/portal"
	"github.com/spf13/cobra"
)

// Ledger statuses for one (mailbox, sub-source) archive.
//
// REQUESTED is written to disk before the create call leaves, and the
// call's answer is recorded as accepted_at. CREATED means the job was
// afterwards found on the portal, which is the only proof it exists: the
// create call answers "Your export request has been scheduled." even when it
// quietly does nothing. MISSING is a row that was accepted and never
// appeared, and is what a resume re-sends. FAILED is a create the portal
// provably refused or never received, which a resume also re-sends.
//
// UNKNOWN is a create that may have reached the portal but whose outcome
// was never learned: a timeout, a dropped connection or a server error
// after the request left, or a run that was killed mid-call. A resume never
// re-sends it on its own. A later reconcile that finds the job marks it
// CREATED; otherwise only --retry-unknown sends it again. An UNKNOWN row
// marked ambiguous (several portal jobs could be it) is never sent again.
//
// SKIPPED is an archive another batch holds in the download guard; it is
// never this batch's. JOB_FAILED is a confirmed job that then ended in
// failure (exportJobFailed): the archive does not exist, and a resume of this
// batch creates it once more.
const (
	dlPlanned   = "PLANNED"
	dlRequested = "REQUESTED"
	dlCreated   = "CREATED"
	dlMissing   = "MISSING"
	dlFailed    = "FAILED"
	dlUnknown   = "UNKNOWN"
	dlSkipped   = "SKIPPED"
	dlJobFailed = "JOB_FAILED"
	// EMPTY is a job that was created and succeeded with nothing to
	// download. getRestoreDownloadStatus drops such jobs, so without the
	// dashboard cross-check they look identical to a job that was never
	// created, and a resume would create duplicates.
	dlEmpty = "EMPTY"
)

// downloadBatchDirName is the state subdirectory holding batch ledgers.
const downloadBatchDirName = "download-batches"

// batchFilePerm is the mode for a ledger file.
const batchFilePerm = 0o600

// reconcileDelay is how long to wait after the last create before listing
// the portal. A new job took about three minutes to appear in testing, so a
// shorter wait would report false MISSING rows.
var reconcileDelay = 4 * time.Minute

// defaultDownloadConcurrency bounds parallel create calls. The portal's own
// rate limiter still paces the requests.
const defaultDownloadConcurrency = 4

// downloadRow is one archive in a batch: one mailbox, one sub-source.
// AcceptedAt is when the create call answered success; a REQUESTED row
// without it was cut off mid-call and becomes UNKNOWN on resume.
type downloadRow struct {
	Entity       string `json:"entity"`
	EntityStatus string `json:"entity_status,omitempty"`
	ExtID        string `json:"ext_id,omitempty"`
	MailboxSize  string `json:"mailbox_size,omitempty"`
	SubSource    string `json:"sub_source"`
	// SnapshotRaw is the sub-source's OWN compact timestamp, not the
	// parent run's.
	SnapshotRaw  int64  `json:"snapshot_raw"`
	Snapshot     string `json:"snapshot"`
	ParentRaw    int64  `json:"parent_raw,omitempty"`
	BatchID      string `json:"backup_batch_id,omitempty"`
	Status       string `json:"status"`
	Detail       string `json:"detail,omitempty"`
	JobID        string `json:"job_id,omitempty"`
	RequestedAt  string `json:"requested_at,omitempty"`
	AcceptedAt   string `json:"accepted_at,omitempty"`
	ReconciledAt string `json:"reconciled_at,omitempty"`
	// Label is the unique job label sent with this row's create, so its
	// job can be told apart from any other export of the same snapshot.
	Label string `json:"label,omitempty"`
	// ConfirmedAt is when the row's job was first found on the portal.
	// A confirmed row stays CREATED for good; LastSeenAt and JobStatus
	// record what later listings showed without changing that.
	ConfirmedAt string `json:"confirmed_at,omitempty"`
	LastSeenAt  string `json:"last_seen_at,omitempty"`
	JobStatus   string `json:"job_status,omitempty"`
	// Ambiguous is set when several portal jobs could be this row's and
	// none is proven; such a row is never sent again on its own.
	Ambiguous bool `json:"ambiguous,omitempty"`
	// HeldBy names the batch that holds this archive when it is SKIPPED.
	HeldBy string `json:"held_by,omitempty"`
}

// downloadBatch is the on-disk ledger for one year-end run.
type downloadBatch struct {
	BatchKey  string `json:"batch_key"`
	Scope     string `json:"scope"`
	AccountID string `json:"account_id"`
	TaskID    string `json:"task_id"`
	Source    string `json:"source"`
	OnDate    string `json:"on_date"`
	// Timezone names the zone OnDate is expressed in. A backup stamped
	// 2022-12-31 UTC is the 30 December end-of-day state for a US_EST
	// account, so the zone is recorded rather than assumed.
	Timezone string `json:"timezone"`
	Format   string `json:"format"`
	// IncludeDeleted and IncludeLegalHold are what every create in the
	// batch asks for, so a resume sends the same archive the first run
	// did. A ledger without them is not resumed.
	IncludeDeleted   *bool `json:"include_deleted,omitempty"`
	IncludeLegalHold *bool `json:"include_legal_hold,omitempty"`
	// EntityFilter, SubSourceFilter and MinSize record what selected the
	// rows; RerunKey is the identity the re-run guard compares.
	EntityFilter    []string       `json:"entity_filter,omitempty"`
	SubSourceFilter []string       `json:"sub_source_filter,omitempty"`
	MinSize         string         `json:"min_size,omitempty"`
	RerunKey        string         `json:"rerun_key,omitempty"`
	CreatedAt       string         `json:"created_at"`
	UpdatedAt       string         `json:"updated_at"`
	Rows            []downloadRow  `json:"rows"`
	Skipped         []downloadSkip `json:"skipped_entities,omitempty"`
}

// downloadSkip records a mailbox that produced no archive, and why.
type downloadSkip struct {
	Entity       string `json:"entity"`
	EntityStatus string `json:"entity_status,omitempty"`
	MailboxSize  string `json:"mailbox_size,omitempty"`
	Reason       string `json:"reason"`
}

func (b *downloadBatch) count(status string) int {
	n := 0
	for _, r := range b.Rows {
		if r.Status == status {
			n++
		}
	}
	return n
}

// planDownloadBatch expands every entity on the task into one row per
// successful sub-source of that date's run.
func planDownloadBatch(ctx context.Context, task *taskWithEntities, filter map[string]bool, day time.Time, offset time.Duration, resolver *portal.Resolver, subFilter map[string]bool, sizes map[string]portal.TaskEntityInfo, minSize int64) (*downloadBatch, error) {
	batch := &downloadBatch{
		TaskID: string(task.ID),
		Source: task.Source,
		OnDate: day.UTC().Format("2006-01-02"),
	}
	for _, ent := range planEntities(task, filter) {
		info, haveSize := sizes[strings.ToLower(strings.TrimSpace(ent.Entity))]
		// Skip mailboxes that cannot produce an archive worth having,
		// before any job is created. An unreadable size (-1) is never
		// treated as empty: that would drop a mailbox that may hold data.
		if haveSize {
			switch {
			case !info.Accessible():
				batch.Skipped = append(batch.Skipped, downloadSkip{
					Entity: ent.Entity, EntityStatus: ent.Status, MailboxSize: info.Size,
					Reason: "the backup reports " + info.Status + ", so no archive can be produced",
				})
				continue
			case info.SizeBytes == 0:
				batch.Skipped = append(batch.Skipped, downloadSkip{
					Entity: ent.Entity, EntityStatus: ent.Status, MailboxSize: info.Size,
					Reason: "the mailbox holds nothing (" + info.Size + "), so every archive would be empty",
				})
				continue
			case minSize > 0 && info.SizeBytes > 0 && info.SizeBytes < minSize:
				batch.Skipped = append(batch.Skipped, downloadSkip{
					Entity: ent.Entity, EntityStatus: ent.Status, MailboxSize: info.Size,
					Reason: fmt.Sprintf("the mailbox holds %s, below --min-size", info.Size),
				})
				continue
			}
		}
		extID := strings.TrimSpace(string(ent.ExtID))
		if extID == "" {
			batch.Skipped = append(batch.Skipped, downloadSkip{
				Entity: ent.Entity, EntityStatus: ent.Status,
				Reason: "the task carries no external id, so this mailbox's backups cannot be read at all",
			})
			continue
		}
		run, found, err := resolver.RunOnLocalDate(ctx, portal.Entity{
			TaskID: string(task.ID), AccountID: ent.Entity, ExtID: extID,
		}, day, offset)
		if err != nil {
			batch.Skipped = append(batch.Skipped, downloadSkip{
				Entity: ent.Entity, EntityStatus: ent.Status,
				Reason: "the backup calendar could not be read: " + err.Error(),
			})
			continue
		}
		if !found {
			batch.Skipped = append(batch.Skipped, downloadSkip{
				Entity: ent.Entity, EntityStatus: ent.Status,
				Reason: "no successful backup on " + batch.OnDate + " " + batch.Timezone,
			})
			continue
		}
		subs := run.SuccessfulSubSources()
		if len(subs) == 0 {
			batch.Skipped = append(batch.Skipped, downloadSkip{
				Entity: ent.Entity, EntityStatus: ent.Status,
				Reason: "the run on " + batch.OnDate + " reported no successful sub-source",
			})
			continue
		}
		added := 0
		for _, sub := range subs {
			if len(subFilter) > 0 && !subFilter[strings.ToUpper(sub.Source)] {
				continue
			}
			row := downloadRow{
				Entity: ent.Entity, EntityStatus: ent.Status, ExtID: extID,
				MailboxSize: info.Size,
				SubSource:   sub.Source, SnapshotRaw: sub.DateTime,
				ParentRaw: run.DateTime, BatchID: run.BatchID, Status: dlPlanned,
			}
			if t, terr := sub.Time(); terr == nil {
				row.Snapshot = isoUTC(t)
			}
			batch.Rows = append(batch.Rows, row)
			added++
		}
		if added == 0 {
			batch.Skipped = append(batch.Skipped, downloadSkip{
				Entity: ent.Entity, EntityStatus: ent.Status,
				Reason: "no sub-source matched --sub-sources",
			})
		}
	}
	sort.SliceStable(batch.Rows, func(i, j int) bool {
		if a, b := strings.ToLower(batch.Rows[i].Entity), strings.ToLower(batch.Rows[j].Entity); a != b {
			return a < b
		}
		return batch.Rows[i].SubSource < batch.Rows[j].SubSource
	})
	return batch, nil
}

// planLatestBatch builds a row per sub-source from each entity's most
// recent successful backup, whenever that was.
//
// This needs no calendar reads: the task record's lastBackupDates already
// holds the latest successful timestamp per sub-source, which is exactly
// what a download has to be asked for. It therefore reaches mailboxes whose
// last backup was years ago, which a month-by-month calendar walk would
// have to grind back through.
func planLatestBatch(task *taskWithEntities, filter map[string]bool, subFilter map[string]bool, sizes map[string]portal.TaskEntityInfo, minSize int64) *downloadBatch {
	batch := &downloadBatch{
		TaskID: string(task.ID),
		Source: task.Source,
		OnDate: latestLabel,
	}
	for _, ent := range planEntities(task, filter) {
		info, haveSize := sizes[strings.ToLower(strings.TrimSpace(ent.Entity))]
		if haveSize {
			switch {
			case !info.Accessible():
				batch.Skipped = append(batch.Skipped, downloadSkip{
					Entity: ent.Entity, EntityStatus: ent.Status, MailboxSize: info.Size,
					Reason: "the backup reports " + info.Status + ", so no archive can be produced",
				})
				continue
			case info.SizeBytes == 0:
				batch.Skipped = append(batch.Skipped, downloadSkip{
					Entity: ent.Entity, EntityStatus: ent.Status, MailboxSize: info.Size,
					Reason: "the mailbox holds nothing (" + info.Size + "), so every archive would be empty",
				})
				continue
			case minSize > 0 && info.SizeBytes > 0 && info.SizeBytes < minSize:
				batch.Skipped = append(batch.Skipped, downloadSkip{
					Entity: ent.Entity, EntityStatus: ent.Status, MailboxSize: info.Size,
					Reason: fmt.Sprintf("the mailbox holds %s, below --min-size", info.Size),
				})
				continue
			}
		}
		extID := strings.TrimSpace(string(ent.ExtID))
		if extID == "" {
			batch.Skipped = append(batch.Skipped, downloadSkip{
				Entity: ent.Entity, EntityStatus: ent.Status, MailboxSize: info.Size,
				Reason: "the task carries no external id, so this mailbox cannot be archived",
			})
			continue
		}
		dates := ent.backupDates()
		if len(dates) == 0 {
			batch.Skipped = append(batch.Skipped, downloadSkip{
				Entity: ent.Entity, EntityStatus: ent.Status, MailboxSize: info.Size,
				Reason: "the task records no successful backup for any sub-source",
			})
			continue
		}
		subs := make([]string, 0, len(dates))
		for sub := range dates {
			subs = append(subs, sub)
		}
		sort.Strings(subs)
		added := 0
		for _, sub := range subs {
			raw := dates[sub]
			if raw <= 0 {
				continue
			}
			if len(subFilter) > 0 && !subFilter[strings.ToUpper(sub)] {
				continue
			}
			row := downloadRow{
				Entity: ent.Entity, EntityStatus: ent.Status, ExtID: extID,
				MailboxSize: info.Size,
				SubSource:   sub, SnapshotRaw: raw, Status: dlPlanned,
			}
			if t, terr := cadate.ParseCompact(raw); terr == nil {
				row.Snapshot = isoUTC(t)
			}
			batch.Rows = append(batch.Rows, row)
			added++
		}
		if added == 0 {
			batch.Skipped = append(batch.Skipped, downloadSkip{
				Entity: ent.Entity, EntityStatus: ent.Status, MailboxSize: info.Size,
				Reason: "no sub-source matched --sub-sources",
			})
		}
	}
	sort.SliceStable(batch.Rows, func(i, j int) bool {
		if a, b := strings.ToLower(batch.Rows[i].Entity), strings.ToLower(batch.Rows[j].Entity); a != b {
			return a < b
		}
		return batch.Rows[i].SubSource < batch.Rows[j].SubSource
	})
	return batch
}

// latestLabel marks a batch built from each mailbox's most recent backup.
const latestLabel = "latest"

var downloadRowHeaders = []string{"entity", "entity_status", "mailbox_size", "sub_source", "snapshot", "snapshot_raw", "status"}

func downloadRowsAsMaps(rows []downloadRow) [][]string {
	out := make([][]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, []string{
			r.Entity, r.EntityStatus, r.MailboxSize, r.SubSource, r.Snapshot,
			fmt.Sprintf("%d", r.SnapshotRaw), r.Status,
		})
	}
	return out
}

// rowLabelPrefix starts every job label this tool sends.
const rowLabelPrefix = "cloudally-cli "

// newRowLabel makes the unique label one row's create carries.
func newRowLabel() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return rowLabelPrefix + hex.EncodeToString(b[:])
}

// unrelatedAge is how long before a row was requested a job must have been
// created to be certainly not that row's. Within it, a job could still be
// the row's own as far as two clocks can tell.
const unrelatedAge = 24 * time.Hour

// labelsKept reports whether the portal evidently stores job labels: some
// job it lists carries one this tool sent.
// rowLabelKey is the unique part of a tool label ("cloudally-cli " and 16
// hex digits), the only part a job is matched by: download start appends
// the operator's text after it, and the portal may cut or an operator edit
// that text. A label without it (an operator's own, or one from an older
// start) proves nothing and gives "".
func rowLabelKey(label string) string {
	label = strings.TrimSpace(label)
	n := len(rowLabelPrefix) + 16
	if !strings.HasPrefix(label, rowLabelPrefix) || len(label) < n {
		return ""
	}
	if len(label) > n && label[n] != ' ' {
		return ""
	}
	return label[:n]
}

func labelsKept(jobs []portal.DownloadJob) bool {
	for _, j := range jobs {
		if strings.HasPrefix(strings.TrimSpace(j.JobLabel), rowLabelPrefix) {
			return true
		}
	}
	return false
}

// rowMatch is what the portal listing says about one row.
type rowMatch struct {
	job       portal.DownloadJob
	found     bool
	ambiguous bool
}

// matchRow finds the portal job that is an unconfirmed row's archive. A row
// already confirmed is never matched again: only its pinned job id counts
// (see reconcileRows). For the rest, among the listed jobs of the same
// mailbox, sub-source and snapshot:
//
//   - the job carrying the row's own label is it;
//   - a job that is provably another archive is ignored: it carries a label
//     the download guard records for a different archive, its files
//     contradict the row's format, or it was created more than unrelatedAge
//     before the row was requested;
//   - when the portal keeps labels, any other such job could be this row's
//     with its label lost or edited, so the row is ambiguous;
//   - when it does not, exactly one job created at or after the row was
//     requested is the row's. A job created before then, a job whose
//     creation time cannot be read, or more than one candidate makes the
//     row ambiguous.
//
// An ambiguous row is never inferred EMPTY or MISSING.
func matchRow(r *downloadRow, jobs []portal.DownloadJob, format string, keptLabels bool, foreign func(label string) bool) rowMatch {
	key := jobKey(r.Entity, r.SubSource, r.SnapshotRaw)
	since, sinceErr := time.Parse(time.RFC3339, r.RequestedAt)
	var candidates []portal.DownloadJob
	unsure := false
	for _, j := range jobs {
		if jobKey(j.Account, j.SubSource, j.SnapshotDate) != key {
			continue
		}
		label := strings.TrimSpace(j.JobLabel)
		if k := rowLabelKey(r.Label); k != "" && rowLabelKey(label) == k {
			return rowMatch{job: j, found: true}
		}
		if (label != "" && foreign != nil && foreign(label)) || !formatFits(j, format) {
			continue
		}
		created, err := cadate.ParseCompact(j.InitDate)
		switch {
		case sinceErr != nil || err != nil:
			unsure = true
		case created.Before(since.Add(-unrelatedAge)):
			// Made well before this row was requested: not its job.
		case keptLabels || label != "" || created.Before(since):
			unsure = true
		default:
			candidates = append(candidates, j)
		}
	}
	switch {
	case len(candidates) == 1 && !unsure:
		return rowMatch{job: candidates[0], found: true}
	case len(candidates) > 0 || unsure:
		return rowMatch{ambiguous: true}
	}
	return rowMatch{}
}

// formatFits reports whether a job's files could be an archive of format.
// Only a plain contradiction excludes a job: zip asked for, PST files
// listed.
func formatFits(j portal.DownloadJob, format string) bool {
	if format != downloadFormatZip || len(j.Chunks) == 0 {
		return true
	}
	for _, c := range j.Chunks {
		if !strings.HasSuffix(strings.ToLower(strings.TrimSpace(c.FileName)), ".pst") {
			return true
		}
	}
	return false
}

// confirmRow records that the row's job is on the portal. A row is
// confirmed once; later listings only refresh what was seen.
func confirmRow(r *downloadRow, j portal.DownloadJob, now string) {
	r.JobID, r.Status, r.Detail, r.Ambiguous = j.JobID, dlCreated, "", false
	if r.ConfirmedAt == "" {
		r.ConfirmedAt = now
	}
	r.LastSeenAt, r.JobStatus = now, j.Status
}

// markAmbiguous holds a row whose job cannot be told apart from another
// export: it may exist, so it is UNKNOWN and is never sent again on its own.
func markAmbiguous(r *downloadRow) {
	r.Status, r.Ambiguous = dlUnknown, true
	r.Detail = "a portal job of this mailbox, sub-source and snapshot is listed but cannot be proved to be this row's (its label is missing or different); it is not created again"
}

// markJobFailed records that a confirmed row's job ended as a failure
// (exportJobFailed). The archive does not exist; a resume creates it once
// more.
func markJobFailed(r *downloadRow, status string) {
	r.Status, r.JobStatus = dlJobFailed, status
	r.Detail = "the portal job " + r.JobID + " finished as " + status + " with no archive; a resume of this batch creates it once more"
}

// reconcileResult counts what one reconcile pass changed.
type reconcileResult struct {
	confirmed, jobFailed, ambiguous int
}

// reconcileRows settles a batch's rows against one portal listing and
// mirrors every change into the download guard.
//
//   - A confirmed (CREATED) row is matched only by its pinned job id. When
//     that job is not listed (an empty job drops out once it finishes) the
//     row stays CREATED; when it is listed as a failure (exportJobFailed:
//     FAILED, CANCELLED, ABORTED, STOPPED, ERROR, ...) the row is
//     JOB_FAILED. It is never re-matched to another job.
//   - REQUESTED, MISSING and UNKNOWN rows go through matchRow: found is
//     CREATED, ambiguous is UNKNOWN and held.
//   - Only after a run (final), and only when nothing listed could be the
//     row's, is an unfound REQUESTED row EMPTY (when it is in empty, see
//     emptyRows) or MISSING. A pass before re-sending only ever confirms.
func reconcileRows(ctx context.Context, guard *downloadGuard, batch *downloadBatch, jobs []portal.DownloadJob, final bool, empty map[*downloadRow]bool) (reconcileResult, error) {
	var res reconcileResult
	// A run killed between a create and recording its answer leaves its
	// claim IN_FLIGHT; recover those first so a confirmation found below
	// can be pinned on them.
	if err := guard.recoverAbandoned(ctx); err != nil {
		return res, err
	}
	labels, err := guard.labels(ctx)
	if err != nil {
		return res, err
	}
	known := map[string]bool{}
	for l := range labels {
		if k := rowLabelKey(l); k != "" {
			known[k] = true
		}
	}
	kept := labelsKept(jobs)
	byID := map[string]portal.DownloadJob{}
	for _, j := range jobs {
		byID[strings.ToLower(strings.TrimSpace(j.JobID))] = j
	}
	now := isoUTC(time.Now())
	for i := range batch.Rows {
		r := &batch.Rows[i]
		id := rowIdentity(batch, r)
		if r.Status == dlCreated && r.JobID != "" {
			j, listed := byID[strings.ToLower(strings.TrimSpace(r.JobID))]
			if !listed {
				continue
			}
			r.LastSeenAt, r.JobStatus = now, j.Status
			if exportJobFailed(j.Status) {
				if _, err := guard.jobFailed(ctx, r.JobID); err != nil {
					return res, err
				}
				markJobFailed(r, j.Status)
				res.jobFailed++
			}
			continue
		}
		if r.Status != dlRequested && r.Status != dlMissing && r.Status != dlUnknown && r.Status != dlCreated {
			continue
		}
		r.ReconciledAt = now
		// A label the guard recorded for any other create, this row's own
		// earlier (failed) create included, is provably not this row's job.
		foreign := func(label string) bool {
			k := rowLabelKey(label)
			return k != "" && known[k] && k != rowLabelKey(r.Label)
		}
		m := matchRow(r, jobs, batch.Format, kept, foreign)
		switch {
		case m.found:
			// The guard is pinned first: a row is never confirmed in the
			// ledger while the guard says otherwise.
			if err := guard.pin(ctx, id, batch.BatchKey, m.job.JobID); err != nil {
				return res, err
			}
			confirmRow(r, m.job, now)
			res.confirmed++
			if exportJobFailed(m.job.Status) {
				if _, err := guard.jobFailed(ctx, m.job.JobID); err != nil {
					return res, err
				}
				markJobFailed(r, m.job.Status)
				res.jobFailed++
			}
		case m.ambiguous:
			markAmbiguous(r)
			if err := guard.hold(ctx, id, batch.BatchKey); err != nil {
				return res, err
			}
			res.ambiguous++
		case r.Status == dlRequested && acceptedBefore(*r, time.Now().Add(-staleRequestedAge)):
			// Accepted long ago and still not listed on a fresh listing:
			// a dropped (or empty) create. MISSING, never EMPTY, so a
			// resume sends it again; without this a run that never
			// reconciled (--no-reconcile, an interrupted wait, download
			// start) left it REQUESTED for good.
			r.Status = dlMissing
			r.Detail = "accepted over an hour ago, but no job that could be this row's is on the portal"
		case !final, r.Status == dlUnknown, r.Status == dlCreated:
			// Absence proves nothing for these: a job takes minutes to
			// appear and an empty one never appears at all.
		case empty[r]:
			r.Status = dlEmpty
			r.Detail = "created and finished with nothing to download"
		default:
			r.Status = dlMissing
			r.Detail = "requested, but no job that could be this row's appeared on the portal"
		}
	}
	return res, nil
}

// staleRequestedAge is how long after its create was accepted a row's job
// must have appeared on the portal. A new job is listed within minutes.
const staleRequestedAge = time.Hour

// acceptedBefore reports whether the portal accepted r's create before t.
func acceptedBefore(r downloadRow, t time.Time) bool {
	at, err := time.Parse(time.RFC3339, r.AcceptedAt)
	return err == nil && at.Before(t)
}

// reconcileBatch lists the portal after a run and settles each row it
// sent; an unfound row is EMPTY only when it is in empty (emptyRows).
func reconcileBatch(ctx context.Context, sess *portal.Session, guard *downloadGuard, batch *downloadBatch, empty map[*downloadRow]bool) (reconcileResult, error) {
	jobs, err := sess.DownloadJobs(ctx)
	if err != nil {
		return reconcileResult{}, err
	}
	return reconcileRows(ctx, guard, batch, jobs, true, empty)
}

// countWindow is what a run read around its creates to tell an empty job
// from a dropped create. The portal's job count grows for every job,
// empty ones included, while the listing drops a job that finished with
// nothing to download; the count is account-wide, so it says something
// about this run's creates only when nothing else could have moved it.
type countWindow struct {
	counted bool // the count was read before and after
	delta   int  // how much it grew
	// outsideUnsettled: at the window's open or close, a download or
	// restore outside this run was in flight or had an unknown outcome, or
	// any claim outside it was made or changed from reconcileDelay before
	// the window onwards, confirmed or not.
	outsideUnsettled bool
	outsideChanged   bool // a download or restore outside this run changed inside the window
	unreadable       bool // a guard could not be read: treated as activity
}

// emptyRows returns the rows an unfound result marks EMPTY, and nil when
// none may be. A row qualifies only when all of these hold:
//
//   - it was sent, and accepted, in this run (sent), between the two reads;
//   - the count grew by exactly the number of creates this run had
//     accepted;
//   - every other create this run sent has a known answer (refused, or
//     never sent), and no row of the batch it did not send is UNKNOWN or
//     REQUESTED with no answer: any of those may be in the count;
//   - nothing outside this run was unsettled at the window's open or close,
//     or changed inside it, and both guards could be read.
//
// Every other unfound row stays MISSING (or REQUESTED/UNKNOWN). Activity
// the guards cannot see (another machine, the portal UI) is the residual.
func emptyRows(batch *downloadBatch, sent []*downloadRow, w countWindow) map[*downloadRow]bool {
	if !w.counted || w.outsideUnsettled || w.outsideChanged || w.unreadable {
		return nil
	}
	accepted := map[*downloadRow]bool{}
	sentNow := map[*downloadRow]bool{}
	for _, r := range sent {
		sentNow[r] = true
		switch {
		case r.Status == dlRequested && r.AcceptedAt != "":
			accepted[r] = true
		case r.Status == dlFailed, r.Status == dlSkipped, r.Status == dlCreated:
			// Refused before it ran, or not sent at all (held elsewhere, or
			// already confirmed): no job of it can be in the count.
		default:
			return nil
		}
	}
	for i := range batch.Rows {
		r := &batch.Rows[i]
		if !sentNow[r] && (r.Status == dlUnknown || (r.Status == dlRequested && r.AcceptedAt == "")) {
			return nil
		}
	}
	if len(accepted) == 0 || w.delta != len(accepted) {
		return nil
	}
	return accepted
}

// outsideActivity reads the download guard (this account) and the restore
// guard (every account: a restore's job is counted too) for activity
// outside this run around since; see countWindow. Any error is returned
// and counts as activity.
func outsideActivity(ctx context.Context, guard *downloadGuard, account string, since time.Time) (unsettled, changed bool, err error) {
	unsettled, changed, err = guard.countActivity(ctx, account, since, reconcileDelay)
	if err != nil {
		return true, true, err
	}
	dir, err := cliutil.StateDir()
	if err != nil {
		return true, true, err
	}
	path := filepath.Join(dir, restoreGuardFile)
	if _, serr := os.Stat(path); errors.Is(serr, os.ErrNotExist) {
		return unsettled, changed, nil
	}
	db, err := sql.Open("sqlite", path+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		return true, true, err
	}
	defer db.Close()
	var u, c int
	early, open := isoUTC(since.Add(-reconcileDelay)), isoUTC(since)
	if err := db.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(state IN (?, ?) OR created_at >= ? OR updated_at >= ?), 0),
		COALESCE(SUM(created_at >= ? OR updated_at >= ?), 0) FROM restore_claims`,
		claimSending, claimUnknown, early, early, open, open).Scan(&u, &c); err != nil {
		return true, true, err
	}
	return unsettled || u > 0, changed || c > 0, nil
}

// reconcileBeforeResend lists the portal before a resume sends anything: it
// confirms rows whose job is now there, holds rows it cannot tell apart,
// and finds confirmed jobs that failed. It never makes a row sendable
// except a confirmed one whose job failed, which is created once more.
func reconcileBeforeResend(ctx context.Context, sess *portal.Session, guard *downloadGuard, batch *downloadBatch) (reconcileResult, error) {
	jobs, err := sess.DownloadJobs(ctx)
	if err != nil {
		return reconcileResult{}, err
	}
	return reconcileRows(ctx, guard, batch, jobs, false, nil)
}

// recoverInterruptedRows turns every REQUESTED row whose create never
// recorded an answer into UNKNOWN: the run that sent it was killed before
// it learned whether the portal created the job.
func recoverInterruptedRows(batch *downloadBatch) int {
	n := 0
	for i := range batch.Rows {
		r := &batch.Rows[i]
		if r.Status == dlRequested && r.AcceptedAt == "" {
			r.Status = dlUnknown
			r.Detail = "the run that sent this create stopped before the portal answered; the job may exist"
			n++
		}
	}
	return n
}

func newDownloadSubmitCmd(flags *rootFlags) *cobra.Command {
	var scope restoreScope
	var flagTask, flagOnDate, flagFormat, flagPortalURL, flagBatch, flagPlanOut, flagMinSize string
	var flagUTCOffset float64
	var flagEntities, flagSubSources []string
	var flagEntitiesFile string
	var flagConcurrency int
	var flagIncludeDeleted, flagIncludeLegalHold, flagResume, flagNoReconcile, flagLatest, flagRetryUnknown, flagNewBatch bool
	var flagLookback int

	cmd := &cobra.Command{
		Use:   "submit",
		Short: "Build year-end archives for every mailbox backed up on one date",
		Long: strings.Trim(`
Creates one download job per (mailbox, sub-source) for a single backup date,
and records every row in a local ledger.

Selection is by that exact date: a mailbox is included only if it has a
successful backup ON the date given. A mailbox whose newest backup is older
is left out and listed as skipped, never archived from a different date.
Mailbox coverage differs per year, so each year must be submitted separately.

An MS365 mailbox has no single archive. It splits into sub-sources (mail,
calendar, contacts, tasks, notes) and each is its own job, asked for with
that sub-source's OWN timestamp: a run's headline timestamp is the maximum of
its sub-sources, and using it would request the wrong snapshot.

Creation is unverifiable at the time of the call: the portal answers "Your
export request has been scheduled." even when it creates nothing, and returns
no job id. So each row is written to the ledger as REQUESTED before its
create call leaves, and a reconcile pass then lists the portal and marks each
row CREATED or MISSING. Re-run with --resume to send the MISSING and FAILED
ones again.

A create whose outcome was never learned (a timeout, a dropped connection, a
server error after the request left, or a run killed mid-call) is UNKNOWN.
--resume first looks for UNKNOWN and MISSING jobs on the portal and marks the
ones it finds CREATED, and never sends an UNKNOWN row again unless
--retry-unknown is given; even then it looks first. A REQUESTED row accepted
over an hour ago that the resume still does not find is MISSING, and that
resume sends it again; a resume that sends nothing does not wait to reconcile.
A resend of an UNKNOWN or MISSING row that is refused or never sent leaves the
row UNKNOWN: the first create may still exist. PLANNED and FAILED rows of a
ledger from the first build (which recorded no include flags) are held as
UNKNOWN for the same reason. A resume with an --account other than the batch's
is refused (exit 2). A resume sends every row with the batch's recorded
--format, --include-deleted and --include-legal-hold.

Each create carries a unique job label, and a row is confirmed only by the
portal job with its label (or, where the portal does not keep labels, the one
job of that snapshot created after the row was requested). A confirmed row
keeps its job for good. When a listed job of the same mailbox, sub-source and
snapshot could be the row's but cannot be proved to be, the row is held as
ambiguous: it is never created again, and the snapshot-wide 'download fetch'
printed for it collects every archive of that snapshot. A confirmed job that
ends in failure (any status naming one: FAILED, CANCELLED, ABORTED, STOPPED,
ERROR) is JOB_FAILED, and a resume creates it once more.

Every archive (account, task, mailbox, sub-source, snapshot, format and both
include flags) is claimed in one download guard in the state directory right
before its create, so no two batches or runs can create the same archive. A
new batch creates only the archives no other batch has created; the others are
SKIPPED and named, and this batch's 'download fetch --batch' collects them
through the holding batch's confirmed jobs. A new batch is refused only while
another batch's create of one of its archives is still unsettled (UNKNOWN or
being sent), with that batch's resume command. The batch key is printed, and
the ledger saved, before the first create. --new-batch is the only override:
it creates every export of the run again; a --new-batch create that is refused
or never sent leaves the archive held by the earlier batch, as before. Two
runs never work on one batch at once.

An unfound row is EMPTY only when this run sent it and the portal accepted it,
the portal's job count grew by exactly the creates this run had accepted,
every other create this run sent has a known answer, no row of the batch it
did not send is UNKNOWN, and no other download or restore in this state
directory was in flight or unknown, or made or changed any claim (confirmed or
not) from the reconcile delay before this run's first count read until its
last. Any other unfound row is MISSING (or stays UNKNOWN).

--timeout bounds each API call, not the run: archives stream for as long as
they keep arriving, and the reconcile wait is not cut short.

Exit codes: 0 every row settled (CREATED, EMPTY, or held by another batch);
2 nothing sent (usage, a refused re-run, another batch's create still
unsettled, a batch in use, an unreadable ledger); 6 rows remain FAILED,
MISSING, JOB_FAILED, UNKNOWN or ambiguous, or REQUESTED and not yet confirmed,
including when the reconcile wait is interrupted (the resume command is
printed).

Nothing is created without --yes: without it the run plans the rows (it
reads the portal), prints them and exits 2. --dry-run does not open the
portal at all: it says what a run would do and exits 0 (with --resume it
prints the ledger's rows).
`, "\n"),
		Example: strings.Trim(`
  cloudally-cli download submit --account 5f1c2a --task 64b0e9 --on-date 2022-12-31 --dry-run
  cloudally-cli download submit --account 5f1c2a --task 64b0e9 --on-date 2022-12-31 --entities jane.doe@contoso.com --yes
  cloudally-cli download submit --account 5f1c2a --task 64b0e9 --on-date 2022-12-31 --yes
  cloudally-cli download submit --batch 20261015-201500-ab12 --resume --yes
  cloudally-cli download submit --batch 20261015-201500-ab12 --resume --retry-unknown --yes
`, "\n"),
		Annotations: map[string]string{
			"mcp:write-flags": "plan-out entities-file",
			"pp:data-source":  "live",
			"pp:happy-args":   "--account=5f1c2a;--task=64b0e9;--on-date=2022-12-31;--dry-run",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if !hasChangedLocalFlags(cmd) && len(args) == 0 {
				return cmd.Help()
			}
			if err := validateDataSourceStrategy(flags, "live"); err != nil {
				return usageErr(err)
			}
			if err := scope.validate(); err != nil {
				return err
			}
			if flagConcurrency < 1 {
				return usageErr(errors.New("--concurrency must be at least 1"))
			}
			if flagRetryUnknown && !flagResume {
				return usageErr(errors.New("--retry-unknown only applies with --resume"))
			}
			if flagNewBatch && flagResume {
				return usageErr(errors.New("--new-batch and --resume cannot be combined: --resume continues the earlier batch, --new-batch starts another that creates every export again"))
			}
			format := strings.ToLower(strings.TrimSpace(flagFormat))
			if format == "" {
				format = downloadFormatZip
			}
			if format != downloadFormatZip && format != downloadFormatPST && format != downloadFormatSinglePST {
				return usageErr(fmt.Errorf("--format %q is not one of %s", flagFormat, strings.Join(downloadFormats, ", ")))
			}

			var batch *downloadBatch
			if flagResume {
				if strings.TrimSpace(flagBatch) == "" {
					return usageErr(errors.New("--resume needs --batch <key>"))
				}
				b, err := loadBatch(strings.TrimSpace(flagBatch))
				if err != nil {
					return usageErr(fmt.Errorf("--batch: %w", err))
				}
				if err := checkBatchScope(cmd, scope, b); err != nil {
					return err
				}
				if err := checkResumeSettings(cmd, b, format, flagIncludeDeleted, flagIncludeLegalHold); err != nil {
					return err
				}
				batch = b
				scope = scopeFromKind(b.Scope, b.AccountID)
				flagTask, format = b.TaskID, b.Format
				// Shown in the plan only; the ledger is changed under the
				// batch lock below.
				recoverInterruptedRows(batch)
				holdLegacyRows(batch)
			}
			if scope.account == "" {
				return usageErr(errPortalNeedsAccount)
			}
			if batch == nil && strings.TrimSpace(flagTask) == "" {
				return usageErr(errors.New("--task <backup task id> is required"))
			}

			var day time.Time
			if batch == nil {
				switch {
				case flagLatest && strings.TrimSpace(flagOnDate) != "":
					return usageErr(errors.New("--latest and --on-date are alternatives; pass one"))
				case flagLatest:
					// No date to parse: each mailbox brings its own.
				default:
					d, _, err := cadate.ParseUserDate(strings.TrimSpace(flagOnDate))
					if err != nil {
						return usageErr(fmt.Errorf("--on-date %w", err))
					}
					day = d
				}
			}

			offset := time.Duration(flagUTCOffset * float64(time.Hour))
			subFilter := map[string]bool{}
			for _, s := range flagSubSources {
				for _, part := range strings.Split(s, ",") {
					if p := strings.ToUpper(strings.TrimSpace(part)); p != "" {
						subFilter[p] = true
					}
				}
			}
			filter, requested, err := parseEntityFilter(flagEntities, flagEntitiesFile)
			if err != nil {
				return err
			}

			// --dry-run never opens the portal: opening it mints a partner
			// handoff token with a POST, which a dry-run client only prints,
			// so the run failed before showing anything. It says what a real
			// run would do; the plan with real rows is a run without --yes.
			if dryRunOK(flags) {
				if batch != nil {
					// A resume's rows are in its ledger. The real resume
					// looks on the portal first, so it may send fewer.
					return printDownloadBatchPlan(cmd, flags, batch, pendingDownloadRows(batch, true, flagRetryUnknown), true)
				}
				if flagPlanOut != "" {
					return usageErr(errors.New("--plan-out needs the planned rows, which come from the portal: run without --yes (and without --dry-run) to write the plan"))
				}
				return printDownloadSubmitDryRun(cmd, flags, scope, flagTask, flagOnDate, flagLatest, format, sortedKeys(filter), sortedKeys(subFilter), flagIncludeDeleted, flagIncludeLegalHold)
			}
			if cliutil.IsAnyHarness() {
				return writeWould(cmd.OutOrStdout(), flags, "download submit",
					"plan and create year-end archives", nil)
			}

			c, err := flags.newClient()
			if err != nil {
				return err
			}
			// No overall deadline: --timeout bounds each API and portal call
			// (the clients apply it per request), while a batch's creates
			// and its reconcile wait take as long as they take.
			ctx := cmd.Context()

			sess, err := openPortalSession(ctx, c, scope, flagPortalURL)
			if err != nil {
				return err
			}

			if batch == nil {
				task, terr := fetchTaskWithEntities(ctx, c, scope.taskPath(flagTask))
				if terr != nil {
					return classifyAPIError(cmd.OutOrStdout(), terr, flags)
				}
				for _, m := range missingEntities(task, requested) {
					tenantf(cmd.ErrOrStderr(), "warning: entity %q is not on task %s\n", m, flagTask)
				}
				// Per-entity sizes come from the portal: the documented
				// entity record carries none, and without them a run
				// creates jobs for mailboxes that can only be empty.
				sizes, serr := sess.TaskEntityInfos(ctx, flagTask)
				if serr != nil {
					tenantf(cmd.ErrOrStderr(), "warning: mailbox sizes unavailable (%v); no mailbox skipped for being empty\n", serr)
					sizes = map[string]portal.TaskEntityInfo{}
				}
				minSize := int64(0)
				if strings.TrimSpace(flagMinSize) != "" {
					mb := portal.ParseSizeBytes(flagMinSize)
					if mb < 0 {
						return usageErr(fmt.Errorf("--min-size %q: want a size like 10MB or 500KB", flagMinSize))
					}
					minSize = mb
				}
				var b *downloadBatch
				if flagLatest {
					b = planLatestBatch(task, filter, subFilter, sizes, minSize)
				} else {
					resolver := portal.NewResolver(sess, flagLookback)
					var perr error
					b, perr = planDownloadBatch(ctx, task, filter, day, offset, resolver, subFilter, sizes, minSize)
					if perr != nil {
						return perr
					}
				}
				b.Scope, b.AccountID, b.Format = scope.kind(), scope.account, format
				b.Timezone = offsetLabel(offset)
				incDel, incLH := flagIncludeDeleted, flagIncludeLegalHold
				b.IncludeDeleted, b.IncludeLegalHold = &incDel, &incLH
				b.EntityFilter, b.SubSourceFilter = sortedKeys(filter), sortedKeys(subFilter)
				b.MinSize = strings.TrimSpace(flagMinSize)
				b.RerunKey = rerunIdentity(b.Scope, b.AccountID, b.TaskID, b.OnDate, b.Format, b.EntityFilter, b.SubSourceFilter, incDel, incLH)
				b.BatchKey = newBatchKey(time.Now(), scope.account)
				b.CreatedAt = isoUTC(time.Now())
				batch = b
			}

			sendable := pendingDownloadRows(batch, flagResume, flagRetryUnknown)
			if flagPlanOut != "" {
				if werr := writeBatchTo(flagPlanOut, batch); werr != nil {
					return fmt.Errorf("writing --plan-out: %w", werr)
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "plan written to %s\n", flagPlanOut)
			}
			if !flags.yes {
				if held := heldUnknownRows(batch, sendable); len(held) > 0 {
					printUnknownDownloadNotice(cmd.ErrOrStderr(), batch, held)
				}
				if err := printDownloadBatchPlan(cmd, flags, batch, sendable, false); err != nil {
					return err
				}
				if len(sendable) == 0 {
					return unsettledDownloadErr(batch)
				}
				return usageErr(fmt.Errorf("add --yes to create these %d archive(s)", len(sendable)))
			}
			// The download guard holds one claim per archive across every
			// batch in this state directory; every ledger is imported into
			// it once before anything is decided.
			guard, err := openDownloadGuard(ctx)
			if err != nil {
				return err
			}
			defer guard.Close()
			if err := guard.seedFromLedgers(ctx); err != nil {
				return usageErr(err)
			}
			run := submitRun{
				cmd: cmd, flags: flags, sess: sess, guard: guard, scope: scope, concurrency: flagConcurrency,
				resume: flagResume, retryUnknown: flagRetryUnknown, noReconcile: flagNoReconcile, newBatch: flagNewBatch,
			}
			if flagResume {
				key := batch.BatchKey
				return withBatchLock(key, func() error {
					// Reload under the lock: the ledger on disk is the truth.
					b, err := loadBatch(key)
					if err != nil {
						return fmt.Errorf("--batch: %w", err)
					}
					if err := checkResumeSettings(cmd, b, format, flagIncludeDeleted, flagIncludeLegalHold); err != nil {
						return err
					}
					changed := false
					if b.IncludeDeleted == nil || b.IncludeLegalHold == nil {
						// An older ledger: none of its rows is proved unsent,
						// so its PLANNED and FAILED rows are held as UNKNOWN
						// (and looked for before anything is sent). Then
						// record what the operator stated, in the ledger and
						// on the claims seeded from it.
						if n := holdLegacyRows(b); n > 0 {
							fmt.Fprintf(cmd.ErrOrStderr(), "%d row(s) of batch %s were recorded PLANNED or FAILED by an older version that did not record whether they were sent; they are now UNKNOWN, looked for on the portal, and not sent again on their own\n", n, key)
						}
						incDel, incLH := flagIncludeDeleted, flagIncludeLegalHold
						b.IncludeDeleted, b.IncludeLegalHold = &incDel, &incLH
						if err := guard.narrowFlags(ctx, b); err != nil {
							return fmt.Errorf("recording the include flags in the download guard: %w", err)
						}
						changed = true
					}
					if n := recoverInterruptedRows(b); n > 0 {
						fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d row(s) in batch %s were still REQUESTED from a run that did not finish; they are now UNKNOWN and will not be sent again on their own\n", n, key)
						changed = true
					}
					if changed {
						if err := saveBatch(b); err != nil {
							return fmt.Errorf("saving the batch ledger: %w", err)
						}
					}
					return run.send(ctx, b)
				})
			}
			if len(sendable) == 0 {
				if err := printDownloadBatchPlan(cmd, flags, batch, sendable, false); err != nil {
					return err
				}
				return unsettledDownloadErr(batch)
			}
			leftOut := 0
			if !flagNewBatch {
				if leftOut, err = refuseHeldArchives(ctx, cmd, guard, batch); err != nil {
					return err
				}
			}
			return withBatchLock(batch.BatchKey, func() error {
				if err := claimNewBatch(batch, flagNewBatch); err != nil {
					return err
				}
				printLeftOut(cmd.ErrOrStderr(), leftOut)
				path, _ := batchPath(batch.BatchKey)
				fmt.Fprintf(cmd.ErrOrStderr(), "batch %s recorded in %s; if this run stops, continue it with: cloudally-cli download submit --batch %s --resume --yes\n",
					batch.BatchKey, path, batch.BatchKey)
				if downloadClaimPause > 0 {
					time.Sleep(downloadClaimPause)
				}
				return run.send(ctx, batch)
			})
		},
	}
	addRestoreScopeFlags(cmd, &scope)
	cmd.Flags().StringVar(&flagTask, "task", "", "Backup task id whose mailboxes will be archived")
	cmd.Flags().StringVar(&flagOnDate, "on-date", "", "The backup date to archive, as YYYY-MM-DD (UTC). A mailbox is included only if it has a successful backup on this exact date")
	cmd.Flags().Float64Var(&flagUTCOffset, "utc-offset", 0,
		"The account's offset from UTC in hours, for example -5 for US_EST. --on-date is then that calendar day in the account's own timezone. Leaving this 0 matches the UTC date, which for a nightly backup is usually the day BEFORE the one intended")
	cmd.Flags().BoolVar(&flagLatest, "latest", false,
		"Archive each mailbox's most recent successful backup instead of one shared date, however old it is. Reaches mailboxes whose last backup was years ago, which --on-date cannot")
	cmd.Flags().StringSliceVar(&flagEntities, "entities", nil, "Comma-separated mailboxes to include; default is every mailbox on the task")
	cmd.Flags().StringVar(&flagEntitiesFile, "entities-file", "", "File with one mailbox per line")
	cmd.Flags().StringSliceVar(&flagSubSources, "sub-sources", nil, "Sub-sources to archive, for example O365MAIL,O365CALENDAR; default is every one that succeeded")
	cmd.Flags().StringVar(&flagFormat, "format", downloadFormatZip, "Archive format: zip, pst or single-pst")
	cmd.Flags().IntVar(&flagConcurrency, "concurrency", defaultDownloadConcurrency, "Parallel create calls")
	cmd.Flags().BoolVar(&flagIncludeDeleted, "include-deleted", false, "Include deleted items. Recorded in the batch; --resume uses the recorded value")
	cmd.Flags().BoolVar(&flagIncludeLegalHold, "include-legal-hold", false, "Include items under legal hold. Recorded in the batch; --resume uses the recorded value")
	cmd.Flags().StringVar(&flagMinSize, "min-size", "",
		"Skip mailboxes holding less than this, for example 10MB. Mailboxes reporting 0 bytes or Not Accessible are always skipped, since they can only produce empty archives")
	cmd.Flags().StringVar(&flagBatch, "batch", "", "Batch key to resume")
	cmd.Flags().BoolVar(&flagResume, "resume", false, "Re-send only the rows that never appeared on the portal (MISSING) or were refused (FAILED); UNKNOWN rows are held")
	cmd.Flags().BoolVar(&flagRetryUnknown, "retry-unknown", false, "With --resume, also send UNKNOWN rows that are still not on the portal; the portal is always checked first, even with --no-reconcile")
	cmd.Flags().BoolVar(&flagNewBatch, "new-batch", false, "Start a new batch even though other batches already hold its archives; every export of the run is created again. The only override of the download guard")
	cmd.Flags().BoolVar(&flagNoReconcile, "no-reconcile", false, "Skip the reconcile pass; rows stay REQUESTED and unverified")
	cmd.Flags().StringVar(&flagPlanOut, "plan-out", "", "Write the batch plan JSON to this path")
	cmd.Flags().StringVar(&flagPortalURL, "portal-url", portal.DefaultBaseURL, portalURLFlagHelp)
	cmd.Flags().IntVar(&flagLookback, "lookback-months", portal.DefaultMaxLookbackMonths, "Unused for --on-date; kept so flags match 'restore plan'")
	_ = cmd.Flags().MarkHidden("portal-url")
	_ = cmd.Flags().MarkHidden("lookback-months")
	return cmd
}

// submitRun carries what sending one batch needs.
type submitRun struct {
	cmd                                         *cobra.Command
	flags                                       *rootFlags
	sess                                        *portal.Session
	guard                                       *downloadGuard
	scope                                       restoreScope
	concurrency                                 int
	resume, retryUnknown, noReconcile, newBatch bool
}

// send creates a batch's pending archives and reconciles them. The caller
// holds the batch lock and the ledger is already saved, so the batch can be
// resumed from the first create on.
func (r submitRun) send(ctx context.Context, batch *downloadBatch) error {
	stderr := r.cmd.ErrOrStderr()
	retry := r.retryUnknown
	// Before anything is sent, a resume lists the portal: rows whose job is
	// there are confirmed and never sent again, rows it cannot tell apart
	// are held, and a confirmed job that failed is created once more.
	// --retry-unknown always looks first, even with --no-reconcile, and
	// sends nothing UNKNOWN when the portal cannot be read.
	if r.resume && (!r.noReconcile || retry) {
		res, rerr := reconcileBeforeResend(ctx, r.sess, r.guard, batch)
		if rerr != nil {
			tenantf(stderr, "warning: could not list the portal before re-sending (%v); UNKNOWN rows stay held\n", rerr)
			retry = false
		}
		if res.confirmed > 0 {
			fmt.Fprintf(stderr, "%d row(s) were found on the portal and are now CREATED, so they are not sent again\n", res.confirmed)
		}
		if res.jobFailed > 0 {
			fmt.Fprintf(stderr, "%d confirmed job(s) ended in failure; this run creates each of those archives once more\n", res.jobFailed)
		}
		if err := saveBatch(batch); err != nil {
			return fmt.Errorf("saving the batch ledger: %w", err)
		}
	}
	sendable := pendingDownloadRows(batch, r.resume, retry)
	if held := heldUnknownRows(batch, sendable); len(held) > 0 {
		printUnknownDownloadNotice(stderr, batch, held)
	}
	if len(sendable) == 0 {
		if err := printDownloadBatchPlan(r.cmd, r.flags, batch, sendable, false); err != nil {
			return err
		}
		printBatchNotices(stderr, batch)
		return unsettledDownloadErr(batch)
	}
	windowStart := time.Now()
	var w countWindow
	openUnsettled, _, openErr := outsideActivity(ctx, r.guard, batch.AccountID, windowStart)
	before, cerr := r.sess.JobCounts(ctx)
	if cerr != nil {
		tenantf(stderr, "warning: could not read the job count first (%v); empty jobs may read as missing\n", cerr)
	}
	if err := runDownloadBatch(ctx, r.cmd, r.sess, r.guard, batch, sendable, r.concurrency, retry, r.newBatch); err != nil {
		return err
	}
	accepted, sentCount := 0, 0
	for _, row := range sendable {
		switch {
		case row.Status == dlRequested && row.AcceptedAt != "":
			accepted++
			sentCount++
		case row.Status == dlRequested, row.Status == dlUnknown:
			sentCount++
		}
	}
	// Nothing sent (every row held elsewhere, refused, or not sent): there is
	// no new job to wait for, and a resume already looked before sending.
	if !r.noReconcile && sentCount > 0 {
		fmt.Fprintf(stderr, "waiting %s before reconciling, because a new job takes minutes to appear (batch %s)\n", reconcileDelay, batch.BatchKey)
		select {
		case <-ctx.Done():
			// The creates are recorded; only their confirmation is missing,
			// which is an unsettled batch, not a crash.
			return partialFailureErr(fmt.Errorf("stopped before reconciling, so the %d archive(s) requested are not confirmed yet; batch %s is saved, continue it with: cloudally-cli download submit --batch %s --resume --yes (%v)",
				sentCount, batch.BatchKey, batch.BatchKey, ctx.Err()))
		case <-time.After(reconcileDelay):
		}
		if after, aerr := r.sess.JobCounts(ctx); aerr == nil && cerr == nil {
			w.counted, w.delta = true, after.Total-before.Total
			fmt.Fprintf(stderr, "portal job count went from %d to %d (+%d) for %d accepted create(s)\n",
				before.Total, after.Total, w.delta, accepted)
		}
		closeUnsettled, changed, closeErr := outsideActivity(ctx, r.guard, batch.AccountID, windowStart)
		w.outsideUnsettled, w.outsideChanged = openUnsettled || closeUnsettled, changed
		w.unreadable = openErr != nil || closeErr != nil
		if w.counted && (w.outsideUnsettled || w.outsideChanged || w.unreadable) {
			fmt.Fprintf(stderr, "another create or restore was unsettled or active while this one ran, so the job count cannot show which rows finished empty; rows not found are MISSING\n")
		}
		if _, err := reconcileBatch(ctx, r.sess, r.guard, batch, emptyRows(batch, sendable, w)); err != nil {
			tenantf(stderr, "warning: reconcile failed (%v); rows stay REQUESTED and are unverified\n", err)
		}
		if err := saveBatch(batch); err != nil {
			return fmt.Errorf("saving the batch ledger: %w", err)
		}
	}
	if err := printDownloadBatchResult(r.cmd, r.flags, batch, r.scope); err != nil {
		return err
	}
	printBatchNotices(stderr, batch)
	return unsettledDownloadErr(batch)
}

// refuseHeldArchives looks, before anything is sent, at the archives of a
// new batch that other batches already hold. Archives another batch has
// created (or must create again after its job failed) are left out at
// claim time, SKIPPED with that batch named, and the rest are created: a
// pilot run for a few mailboxes followed by the whole task creates only
// what the pilot did not. The run is refused only when another batch holds
// an archive with an outcome not yet known, or is creating it right now:
// whether that archive exists has to be settled in that batch first.
//
// It returns how many archives will be left out; the caller says so only
// once the batch is recorded, so a run refused after this never reports
// SKIPPED archives next to creates that never happened.
func refuseHeldArchives(ctx context.Context, cmd *cobra.Command, guard *downloadGuard, batch *downloadBatch) (leftOut int, err error) {
	unsettled := map[string]int{}
	for i := range batch.Rows {
		c, err := guard.conflict(ctx, rowIdentity(batch, &batch.Rows[i]), batch.BatchKey)
		if err != nil {
			return 0, fmt.Errorf("reading the download guard: %w", err)
		}
		switch {
		case c == nil:
		case c.State == dgUnknown || c.State == dgInFlight:
			unsettled[c.BatchKey]++
		default:
			leftOut++
		}
	}
	w := cmd.ErrOrStderr()
	if len(unsettled) == 0 {
		return leftOut, nil
	}
	keys := make([]string, 0, len(unsettled))
	for k := range unsettled {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	held := 0
	for _, k := range keys {
		held += unsettled[k]
		fmt.Fprintf(w, "batch %s holds %d of these archives with an outcome not yet known, or is creating them now. Settle it first with: cloudally-cli download submit --batch %s --resume --yes\n",
			k, unsettled[k], k)
	}
	return 0, usageErr(fmt.Errorf("%d archive(s) this run would create are held by other batches that have not settled whether they exist; nothing was sent (details above)", held))
}

// printLeftOut says how many archives of a recorded batch other batches
// already created.
func printLeftOut(w io.Writer, leftOut int) {
	if leftOut > 0 {
		fmt.Fprintf(w, "%d of these archive(s) are already created by other batches and will be left out (SKIPPED); the rest are created\n", leftOut)
	}
}

func boolValue(p *bool) bool { return p != nil && *p }

// checkResumeSettings refuses to resume a batch with settings that differ
// from the ones its archives were created with. What goes into an archive
// is part of what the batch promised: a resumed legal-hold export that
// quietly left out held items would be worse than no export.
func checkResumeSettings(cmd *cobra.Command, b *downloadBatch, format string, includeDeleted, includeLegalHold bool) error {
	f := cmd.Flags()
	var conflicts []string
	if f.Changed("format") && format != b.Format {
		conflicts = append(conflicts, fmt.Sprintf("--format %s (batch recorded %s)", format, b.Format))
	}
	if b.IncludeDeleted == nil || b.IncludeLegalHold == nil {
		// A ledger from an older build. Saying what the first run used is
		// enough to resume it; guessing is not. Its format is recorded,
		// so a different one is refused here too.
		if len(conflicts) > 0 {
			return usageErr(fmt.Errorf("batch %s creates its archives with different settings than this run: %s; nothing was sent. Drop the flag to resume with the recorded value",
				b.BatchKey, strings.Join(conflicts, ", ")))
		}
		if f.Changed("include-deleted") && f.Changed("include-legal-hold") {
			return nil
		}
		msg := fmt.Sprintf("batch %s was recorded by an older version and does not say whether its archives include deleted or legal-hold items, so a resumed archive could differ from the ones already created; nothing was sent. Resume it by stating how it was first run: cloudally-cli download submit --batch %s --resume --include-deleted=false --include-legal-hold=false --yes (change each to true if the first run used it).",
			b.BatchKey, b.BatchKey)
		if cmds := neverCreatedCommands(b); len(cmds) > 0 {
			const maxShown = 20
			shown := cmds
			if len(shown) > maxShown {
				shown = shown[:maxShown]
			}
			w := cmd.ErrOrStderr()
			fmt.Fprintf(w, "archives batch %s never created, one command per mailbox (add --include-deleted or --include-legal-hold if needed):\n", b.BatchKey)
			for _, c := range shown {
				tenantf(w, "  %s\n", c)
			}
			if len(cmds) > len(shown) {
				fmt.Fprintf(w, "  ... and %d more mailbox(es)\n", len(cmds)-len(shown))
			}
			msg += " Or create only the archives it never created with the commands printed above."
		} else {
			msg += " Every archive in it was created or may have been, so nothing is left to create."
		}
		return usageErr(errors.New(msg))
	}
	if f.Changed("include-deleted") && includeDeleted != *b.IncludeDeleted {
		conflicts = append(conflicts, fmt.Sprintf("--include-deleted=%t (batch recorded %t)", includeDeleted, *b.IncludeDeleted))
	}
	if f.Changed("include-legal-hold") && includeLegalHold != *b.IncludeLegalHold {
		conflicts = append(conflicts, fmt.Sprintf("--include-legal-hold=%t (batch recorded %t)", includeLegalHold, *b.IncludeLegalHold))
	}
	if len(conflicts) > 0 {
		return usageErr(fmt.Errorf("batch %s creates its archives with different settings than this run: %s; nothing was sent. Drop the flag(s) to resume with the recorded values",
			b.BatchKey, strings.Join(conflicts, ", ")))
	}
	return nil
}

// pendingDownloadRows picks the rows a run should send. REQUESTED and
// CREATED rows are never sent again, and UNKNOWN ones only with explicit
// --retry-unknown. A resume also sends rows whose confirmed job failed
// (once more each) and tries again to claim SKIPPED rows: the guard still
// refuses any another batch holds.
func pendingDownloadRows(batch *downloadBatch, resume, retryUnknown bool) []*downloadRow {
	var out []*downloadRow
	for i := range batch.Rows {
		r := &batch.Rows[i]
		if r.Ambiguous {
			continue
		}
		switch r.Status {
		case dlPlanned:
			out = append(out, r)
		case dlMissing, dlFailed, dlJobFailed, dlSkipped:
			if resume {
				out = append(out, r)
			}
		case dlUnknown:
			if resume && retryUnknown && !r.Ambiguous {
				out = append(out, r)
			}
		}
	}
	return out
}

// heldUnknownRows lists the UNKNOWN rows this run is not sending.
func heldUnknownRows(batch *downloadBatch, sendable []*downloadRow) []*downloadRow {
	sending := map[*downloadRow]bool{}
	for _, r := range sendable {
		sending[r] = true
	}
	var held []*downloadRow
	for i := range batch.Rows {
		if r := &batch.Rows[i]; r.Status == dlUnknown && !sending[r] {
			held = append(held, r)
		}
	}
	return held
}

// printUnknownDownloadNotice lists rows whose create outcome is unknown and
// says how to settle them without creating a duplicate export job.
func printUnknownDownloadNotice(w io.Writer, batch *downloadBatch, rows []*downloadRow) {
	for _, r := range rows {
		tenantf(w, "  UNKNOWN  %s  %s  snapshot %d  %s\n", r.Entity, r.SubSource, r.SnapshotRaw, truncateStr(r.Detail, 80))
	}
	fmt.Fprintf(w, "these %d download job(s) may already exist; check 'cloudally-cli download list --account %s' and re-run with --batch %s --resume --retry-unknown --yes only if they are missing\n",
		len(rows), batch.AccountID, batch.BatchKey)
}

// unsettledDownloadErr reports a batch that is not finished, so a script
// never reads it as done: rows FAILED, MISSING or JOB_FAILED (a resume
// creates them), UNKNOWN or ambiguous (held), and REQUESTED rows not yet
// confirmed on the portal.
func unsettledDownloadErr(batch *downloadBatch) error {
	resend := batch.count(dlFailed) + batch.count(dlMissing) + batch.count(dlJobFailed)
	ambiguous := 0
	for _, r := range batch.Rows {
		if r.Status == dlUnknown && r.Ambiguous {
			ambiguous++
		}
	}
	unknown := batch.count(dlUnknown) - ambiguous
	unconfirmed := batch.count(dlRequested)
	if resend+unknown+ambiguous+unconfirmed == 0 {
		return nil
	}
	var parts []string
	if resend > 0 {
		parts = append(parts, fmt.Sprintf("%d to create (FAILED, MISSING or JOB_FAILED): cloudally-cli download submit --batch %s --resume --yes", resend, batch.BatchKey))
	}
	if unknown > 0 {
		parts = append(parts, fmt.Sprintf("%d UNKNOWN: check 'download list' before --retry-unknown", unknown))
	}
	if ambiguous > 0 {
		parts = append(parts, fmt.Sprintf("%d ambiguous: resolve them by hand as shown above", ambiguous))
	}
	if unconfirmed > 0 {
		parts = append(parts, fmt.Sprintf("%d REQUESTED and not yet confirmed: run cloudally-cli download submit --batch %s --resume --yes again later", unconfirmed, batch.BatchKey))
	}
	return partialFailureErr(fmt.Errorf("batch %s is not settled: %s", batch.BatchKey, strings.Join(parts, "; ")))
}

// printBatchNotices explains the rows a person has to act on: ambiguous
// rows, with the snapshot-wide fetch that collects every archive of that
// snapshot, and rows another batch holds.
func printBatchNotices(w io.Writer, batch *downloadBatch) {
	var ambiguous, skipped []downloadRow
	for _, r := range batch.Rows {
		switch {
		case r.Status == dlUnknown && r.Ambiguous:
			ambiguous = append(ambiguous, r)
		case r.Status == dlSkipped:
			skipped = append(skipped, r)
		}
	}
	if len(ambiguous) > 0 {
		fmt.Fprintf(w, "%d archive(s) are ambiguous: a portal job of the same mailbox, sub-source and snapshot is listed but cannot be proved to be this batch's, so none was created again. Collect every archive of that snapshot and check it is the export wanted:\n", len(ambiguous))
		for _, r := range ambiguous {
			tenantf(w, "  cloudally-cli download fetch --account %s --dest <folder> --year <year> --on-date %s --entities %s\n", batch.AccountID, snapshotDay(r.SnapshotRaw), r.Entity)
		}
	}
	if len(skipped) > 0 {
		holders := map[string]int{}
		for _, r := range skipped {
			holders[r.HeldBy]++
		}
		for _, k := range sortedKeysInt(holders) {
			fmt.Fprintf(w, "%d archive(s) are held by batch %s and were not created here; this batch's fetch collects them through it: cloudally-cli download fetch --account %s --batch %s --dest <folder> --year <year>\n",
				holders[k], k, batch.AccountID, batch.BatchKey)
		}
	}
}

// snapshotDay renders a compact snapshot as the YYYY-MM-DD --on-date takes.
func snapshotDay(raw int64) string {
	s := fmt.Sprintf("%d", raw)
	if len(s) < 8 {
		return s
	}
	return s[:4] + "-" + s[4:6] + "-" + s[6:8]
}

func sortedKeysInt(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// classifyDownloadCreateFailure decides what a failed create call means for
// the ledger, the same way restore submit does. connected reports whether a
// connection to the portal was obtained during the call, which is the
// earliest point request bytes can leave this process.
//
//   - No connection: nothing was sent (a local error, DNS, a refused
//     connection, a cancel before dialing), so the row is FAILED and safe to
//     send again.
//   - portal.ErrOutcomeUnknown anywhere in the error: some stage may have
//     accepted the create. A create the portal deferred (WAIT plus a reqId)
//     was received, so a later failure while polling for its answer, even
//     a 401 or 403, leaves it UNKNOWN. This wins over every refusal below.
//   - A rejected session (401/403) or a demand for an interactive redirect,
//     with no earlier stage that could have accepted it, is a clean
//     refusal: FAILED.
//   - Anything else after a connection (a timeout, a reset, a 5xx, an answer
//     that could not be read) may have created the job: UNKNOWN.
func classifyDownloadCreateFailure(err error, connected bool) string {
	switch {
	case !connected:
		return dlFailed
	case errors.Is(err, portal.ErrOutcomeUnknown):
		return dlUnknown
	case errors.Is(err, portal.ErrNoSession), errors.Is(err, portal.ErrNeedsPortalRedirect):
		return dlFailed
	default:
		return dlUnknown
	}
}

// runDownloadBatch creates each of rows. For each row, in order: the
// archive is claimed in the download guard (one IMMEDIATE transaction, so
// of two racing batches only one can claim it), the row is saved to the
// ledger as REQUESTED, the create is sent, and the claim and the row record
// the answer. A run killed at any point leaves a ledger and a guard that
// say which creates may have reached the portal. A row another batch holds
// is SKIPPED and not sent; a row whose claim or REQUESTED state cannot be
// saved is not sent.
func runDownloadBatch(ctx context.Context, cmd *cobra.Command, sess *portal.Session, guard *downloadGuard, batch *downloadBatch, rows []*downloadRow, concurrency int, retryUnknown, newBatch bool) error {
	// Records are written even when the run is interrupted, so the
	// guard and the ledger never lose an answer that came back.
	keep := context.WithoutCancel(ctx)
	includeDeleted, includeLegalHold := boolValue(batch.IncludeDeleted), boolValue(batch.IncludeLegalHold)
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var saveErr error
	notSent, sent, skipped := 0, 0, 0
	for _, row := range rows {
		wg.Add(1)
		go func(r *downloadRow) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			mu.Lock()
			if r.Status == dlJobFailed {
				// The failed job keeps its label; the new create gets its own.
				r.Label, r.JobID, r.ConfirmedAt, r.LastSeenAt, r.JobStatus = "", "", "", "", ""
			}
			if r.Label == "" {
				r.Label = newRowLabel()
			}
			id, label := rowIdentity(batch, r), r.Label
			mu.Unlock()

			claimID, reused, superseded, cerr := guard.claim(keep, id, downloadOwner{batchKey: batch.BatchKey, label: label, scope: batch.Scope}, retryUnknown, newBatch)
			mu.Lock()
			if cerr != nil {
				var held errDownloadClaimed
				if errors.As(cerr, &held) {
					adoptHeldClaim(r, held.claim, batch.BatchKey)
					skipped++
				} else {
					r.Detail = "not sent: the download guard could not record the claim: " + cerr.Error()
					notSent++
					if saveErr == nil {
						saveErr = cerr
					}
				}
				if serr := saveBatch(batch); serr != nil && saveErr == nil {
					saveErr = serr
				}
				mu.Unlock()
				return
			}
			// A resend of a create that may already exist (the claim was
			// reused, or the row was UNKNOWN or accepted) can never prove
			// that earlier create absent. Whatever happens to it, refused,
			// never sent, or rolled back, leaves the row and the claim
			// UNKNOWN: FAILED would free the archive for a plain --resume
			// or another batch to create again.
			resend := reused || mayExist(*r)
			prevStatus, prevDetail, prevRequested, prevAccepted := r.Status, r.Detail, r.RequestedAt, r.AcceptedAt
			r.Status, r.RequestedAt, r.AcceptedAt, r.Detail = dlRequested, isoUTC(time.Now()), "", ""
			r.HeldBy, r.Ambiguous = "", false
			if err := saveBatch(batch); err != nil {
				r.Status, r.Detail, r.RequestedAt, r.AcceptedAt = prevStatus, prevDetail, prevRequested, prevAccepted
				rollback := dlFailed // nothing was sent: release the claim
				if resend {
					rollback = dlUnknown
					r.Status = dlUnknown
					r.Detail = "a resend was not sent (the ledger could not be saved: " + err.Error() + "); an earlier create of this archive may exist, so it stays UNKNOWN"
				}
				_ = guard.settle(keep, claimID, rollback, false, superseded)
				notSent++
				if saveErr == nil {
					saveErr = err
				}
				mu.Unlock()
				return
			}
			mu.Unlock()
			var connected atomic.Bool
			traced := httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
				GotConn: func(httptrace.GotConnInfo) { connected.Store(true) },
			})
			err := sess.StartDownload(traced, portal.DownloadRequest{
				Entity: portal.Entity{
					TaskID: batch.TaskID, AccountID: r.Entity, ExtID: r.ExtID,
				},
				Source:           r.SubSource,
				DateRaw:          r.SnapshotRaw,
				Format:           batch.Format,
				JobLabel:         label,
				IncludeDeleted:   includeDeleted,
				IncludeLegalHold: includeLegalHold,
			})
			mu.Lock()
			sent++
			if err != nil {
				r.Status, r.Detail = classifyDownloadCreateFailure(err, connected.Load()), err.Error()
				if r.Status == dlFailed && resend {
					r.Status = dlUnknown
					r.Detail = "this resend was refused or never sent (" + err.Error() + "); an earlier create of this archive may still exist, so the row stays UNKNOWN"
				}
			} else {
				r.AcceptedAt = isoUTC(time.Now())
			}
			// If this fails the claim stays IN_FLIGHT until this run ends,
			// then reads as UNKNOWN: safe, never a duplicate.
			if serr := guard.settle(keep, claimID, r.Status, err == nil, superseded); serr != nil && saveErr == nil {
				saveErr = serr
			}
			// A failed save here leaves the row REQUESTED on disk, which a
			// resume treats as UNKNOWN: safe, never a duplicate.
			if serr := saveBatch(batch); serr != nil && saveErr == nil {
				saveErr = serr
			}
			mu.Unlock()
		}(row)
	}
	wg.Wait()
	fmt.Fprintf(cmd.ErrOrStderr(), "requested %d archive(s)\n", sent)
	if skipped > 0 {
		fmt.Fprintf(cmd.ErrOrStderr(), "%d archive(s) were not created because another batch holds them (see below)\n", skipped)
	}
	if err := saveBatch(batch); err != nil {
		return fmt.Errorf("saving the batch ledger: %w", err)
	}
	if notSent > 0 {
		return fmt.Errorf("saving the batch ledger or the download guard: %w; %d row(s) were not sent and stay as they were", saveErr, notSent)
	}
	if saveErr != nil {
		tenantf(cmd.ErrOrStderr(), "warning: a ledger or guard save during the run failed (%v); the final save succeeded\n", saveErr)
	}
	return nil
}

// adoptHeldClaim records on a row the guard claim that stopped it. The
// batch's own claim means the ledger was behind the guard; another batch's
// means that batch holds the archive.
func adoptHeldClaim(r *downloadRow, c downloadClaim, batchKey string) {
	if c.BatchKey == batchKey {
		switch c.State {
		case dgCreated:
			r.Status, r.Detail = dlCreated, ""
			if c.JobID != "" {
				r.JobID = c.JobID
			}
		case dgUnknown:
			r.Status = dlUnknown
			r.Detail = "this batch's earlier create of this archive has an unknown outcome; it is not sent again without --retry-unknown"
		default:
			r.Status, r.HeldBy = dlSkipped, batchKey
			r.Detail = "another run of this batch is creating this archive right now"
		}
		return
	}
	r.Status, r.HeldBy = dlSkipped, c.BatchKey
	r.Detail = fmt.Sprintf("batch %s holds this archive (%s); it is not created again here", c.BatchKey, downloadClaimText(c.State))
}

// printDownloadSubmitDryRun says what a new batch would do, without the
// portal: the rows themselves need each mailbox's backup calendar.
func printDownloadSubmitDryRun(cmd *cobra.Command, flags *rootFlags, scope restoreScope, taskID, onDate string, latest bool, format string, entities, subSources []string, includeDeleted, includeLegalHold bool) error {
	date := "the backup on " + onDate
	if latest {
		date = "each mailbox's newest backup (--latest)"
	}
	mode := "dry-run"
	if h := cliutil.HarnessName(); h != "" {
		mode = h + " harness"
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "%s: would resolve each mailbox's backup calendar, then POST %s once per mailbox and sub-source; nothing created. Run without --yes to see the planned rows.\n",
		mode, scope.exportPath())
	return writeWould(cmd.OutOrStdout(), flags, "download submit",
		fmt.Sprintf("resolve %s for every mailbox of task %s, then create one %s EXPORT job per mailbox and sub-source", date, taskID, format),
		map[string]any{
			"path": scope.exportPath(), "task": taskID, "on_date": onDate, "latest": latest, "format": format,
			"entities": entities, "sub_sources": subSources,
			"include_deleted": includeDeleted, "include_legal_hold": includeLegalHold,
		})
}

func printDownloadBatchPlan(cmd *cobra.Command, flags *rootFlags, batch *downloadBatch, sendable []*downloadRow, dry bool) error {
	if !wantsHumanTable(cmd.OutOrStdout(), flags) {
		return printJSONFiltered(cmd.OutOrStdout(), batch, flags)
	}
	w := cmd.OutOrStdout()
	if len(batch.Rows) > 0 {
		if err := printRestoreTable(w, downloadRowHeaders, tenantCells(downloadRowsAsMaps(batch.Rows))); err != nil {
			return err
		}
	}
	entities := map[string]bool{}
	for _, r := range batch.Rows {
		entities[strings.ToLower(r.Entity)] = true
	}
	fmt.Fprintf(w, "\n%s: %d archive(s) across %d mailbox(es) on %s\n",
		map[bool]string{true: "dry run", false: "plan"}[dry], len(batch.Rows), len(entities), batch.OnDate)
	if len(batch.Skipped) > 0 {
		fmt.Fprintf(w, "%d mailbox(es) skipped (no backup on that date, or unreadable):\n", len(batch.Skipped))
		shown := batch.Skipped
		const maxShown = 15
		if len(shown) > maxShown {
			shown = shown[:maxShown]
		}
		for _, s := range shown {
			tenantf(w, "    %-46s %-11s %s\n", s.Entity, s.EntityStatus, s.Reason)
		}
		if len(batch.Skipped) > len(shown) {
			fmt.Fprintf(w, "    ... and %d more\n", len(batch.Skipped)-len(shown))
		}
	}
	if dry {
		fmt.Fprintln(w, "\nnothing was created.")
	} else {
		fmt.Fprintf(w, "%d row(s) would be sent.\n", len(sendable))
	}
	return nil
}

func printDownloadBatchResult(cmd *cobra.Command, flags *rootFlags, batch *downloadBatch, scope restoreScope) error {
	if !wantsHumanTable(cmd.OutOrStdout(), flags) {
		return printJSONFiltered(cmd.OutOrStdout(), batch, flags)
	}
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "batch %s on %s\n", batch.BatchKey, batch.OnDate)
	fmt.Fprintf(w, "  created:     %d\n", batch.count(dlCreated))
	fmt.Fprintf(w, "  missing:     %d\n", batch.count(dlMissing))
	fmt.Fprintf(w, "  failed:      %d\n", batch.count(dlFailed))
	fmt.Fprintf(w, "  job failed:  %d\n", batch.count(dlJobFailed))
	fmt.Fprintf(w, "  unknown:     %d\n", batch.count(dlUnknown))
	fmt.Fprintf(w, "  unverified:  %d\n", batch.count(dlRequested))
	fmt.Fprintf(w, "  held by another batch: %d\n", batch.count(dlSkipped))
	if n := batch.count(dlMissing) + batch.count(dlFailed) + batch.count(dlJobFailed); n > 0 {
		fmt.Fprintf(w, "\n%d archive(s) never appeared, were refused, or their job failed. Create them with:\n", n)
		fmt.Fprintf(w, "  cloudally-cli download submit --batch %s --resume --yes\n", batch.BatchKey)
	}
	if n := batch.count(dlUnknown); n > 0 {
		fmt.Fprintf(w, "\n%d row(s) may or may not have been created. A --resume run looks for them first and never re-sends them on its own; add --retry-unknown only after 'download list' shows they are missing.\n", n)
	}
	fmt.Fprintf(w, "\nFollow progress with: cloudally-cli download list --account %s\n", scope.account)
	return nil
}

func writeBatchTo(path string, batch *downloadBatch) error {
	data, err := json.MarshalIndent(batch, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), batchFilePerm)
}

// offsetLabel renders a UTC offset the way an operator reads it.
func offsetLabel(d time.Duration) string {
	if d == 0 {
		return "UTC"
	}
	sign := "+"
	if d < 0 {
		sign, d = "-", -d
	}
	h := int(d / time.Hour)
	m := int((d % time.Hour) / time.Minute)
	if m == 0 {
		return fmt.Sprintf("UTC%s%d", sign, h)
	}
	return fmt.Sprintf("UTC%s%d:%02d", sign, h, m)
}
