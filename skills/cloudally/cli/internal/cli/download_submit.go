// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http/httptrace"
	"os"
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
// CREATED; otherwise only --retry-unknown sends it again.
const (
	dlPlanned   = "PLANNED"
	dlRequested = "REQUESTED"
	dlCreated   = "CREATED"
	dlMissing   = "MISSING"
	dlFailed    = "FAILED"
	dlUnknown   = "UNKNOWN"
	dlSkipped   = "SKIPPED"
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

// reconcileBatch lists the portal and marks each requested row CREATED or
// MISSING, matching on entity, sub-source and snapshot timestamp.
func reconcileBatch(ctx context.Context, sess *portal.Session, batch *downloadBatch, createdDelta, requested int) error {
	jobs, err := sess.DownloadJobs(ctx)
	if err != nil {
		return err
	}
	// The dashboard counts every job, including ones that finished with
	// nothing to download; the listing below only carries jobs that are
	// running or have files. When the dashboard grew by as much as was
	// requested, nothing was lost, so an unmatched row is EMPTY rather
	// than MISSING.
	allAccountedFor := requested > 0 && createdDelta >= requested
	found := portalJobIndex(jobs)
	now := isoUTC(time.Now())
	for i := range batch.Rows {
		r := &batch.Rows[i]
		if r.Status != dlRequested && r.Status != dlMissing && r.Status != dlCreated && r.Status != dlUnknown {
			continue
		}
		r.ReconciledAt = now
		if id, ok := found[jobKey(r.Entity, r.SubSource, r.SnapshotRaw)]; ok {
			r.JobID = id
			r.Status = dlCreated
			r.Detail = ""
			continue
		}
		// An UNKNOWN create is never settled by absence: a job takes
		// minutes to appear, and an empty one never appears at all, so
		// not finding it proves nothing either way.
		if r.Status == dlUnknown {
			continue
		}
		if allAccountedFor {
			r.Status = dlEmpty
			r.Detail = "created and finished with nothing to download"
			continue
		}
		r.Status = dlMissing
		r.Detail = "requested, but no matching job appeared on the portal"
	}
	return nil
}

// portalJobIndex maps each listed job to its id by (mailbox, sub-source,
// snapshot), the identity a batch row carries.
func portalJobIndex(jobs []portal.DownloadJob) map[string]string {
	found := map[string]string{}
	for _, j := range jobs {
		found[jobKey(j.Account, j.SubSource, j.SnapshotDate)] = j.JobID
	}
	return found
}

// reconcileBeforeResend lists the portal and marks every UNKNOWN or MISSING
// row whose job is now there as CREATED, so a resume never sends a create
// for a job that already exists. It only ever promotes a row, so it is safe
// to run at any time, however recently the rows were sent.
func reconcileBeforeResend(ctx context.Context, sess *portal.Session, batch *downloadBatch) (int, error) {
	jobs, err := sess.DownloadJobs(ctx)
	if err != nil {
		return 0, err
	}
	found := portalJobIndex(jobs)
	now := isoUTC(time.Now())
	promoted := 0
	for i := range batch.Rows {
		r := &batch.Rows[i]
		if r.Status != dlUnknown && r.Status != dlMissing {
			continue
		}
		r.ReconciledAt = now
		if id, ok := found[jobKey(r.Entity, r.SubSource, r.SnapshotRaw)]; ok {
			r.JobID, r.Status, r.Detail = id, dlCreated, ""
			promoted++
		}
	}
	return promoted, nil
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
--retry-unknown is given; even then it looks first. A resume sends every row
with the batch's recorded --format, --include-deleted and --include-legal-hold.

The batch key is printed, and the ledger saved, before the first create.
Running the same command again without --resume is refused while the earlier
batch has archives that were created, may have been created, or are being
sent; --new-batch overrides that and creates every export again. Two runs
never work on one batch at once.

--timeout bounds each API call, not the run: archives stream for as long as
they keep arriving, and the reconcile wait is not cut short.

Exit codes: 0 every row settled; 2 nothing sent (usage, a refused re-run, a
batch in use); 6 rows remain FAILED, UNKNOWN or MISSING.

Nothing is created without --yes.
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
				if err := checkResumeSettings(cmd, b, format, flagIncludeDeleted, flagIncludeLegalHold); err != nil {
					return err
				}
				batch = b
				scope = scopeFromKind(b.Scope, b.AccountID)
				flagTask, format = b.TaskID, b.Format
				// Shown in the plan only; the ledger is changed under the
				// batch lock below.
				recoverInterruptedRows(batch)
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

			if dryRunOK(flags) && batch == nil {
				// A dry run still needs the calendar to show real rows, so
				// it is only short-circuited under a harness.
				if cliutil.IsAnyHarness() {
					return writeWould(cmd.OutOrStdout(), flags, "download submit",
						fmt.Sprintf("plan year-end archives for %s on %s", flagTask, flagOnDate), nil)
				}
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
				b.RerunKey = rerunIdentity(b.Scope, b.AccountID, b.TaskID, b.OnDate, b.Format, b.EntityFilter, b.SubSourceFilter)
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
			if dryRunOK(flags) {
				return printDownloadBatchPlan(cmd, flags, batch, sendable, true)
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
			run := submitRun{
				cmd: cmd, flags: flags, sess: sess, scope: scope, concurrency: flagConcurrency,
				resume: flagResume, retryUnknown: flagRetryUnknown, noReconcile: flagNoReconcile,
			}
			if flagResume {
				key := batch.BatchKey
				return withBatchLock(key, func() error {
					// Reload under the lock: the ledger on disk is the truth.
					b, err := loadBatch(key)
					if err != nil {
						return fmt.Errorf("--batch: %w", err)
					}
					if n := recoverInterruptedRows(b); n > 0 {
						fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d row(s) in batch %s were still REQUESTED from a run that did not finish; they are now UNKNOWN and will not be sent again on their own\n", n, key)
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
			return withBatchLock(batch.BatchKey, func() error {
				if err := claimNewBatch(batch, flagNewBatch); err != nil {
					return err
				}
				path, _ := batchPath(batch.BatchKey)
				fmt.Fprintf(cmd.ErrOrStderr(), "batch %s recorded in %s; if this run stops, continue it with: cloudally-cli download submit --batch %s --resume --yes\n",
					batch.BatchKey, path, batch.BatchKey)
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
	cmd.Flags().BoolVar(&flagNewBatch, "new-batch", false, "Start a new batch even though an earlier batch for the same scope, account, task, date, filters and format already has archives; every export is created again")
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
	cmd                               *cobra.Command
	flags                             *rootFlags
	sess                              *portal.Session
	scope                             restoreScope
	concurrency                       int
	resume, retryUnknown, noReconcile bool
}

// send creates a batch's pending archives and reconciles them. The caller
// holds the batch lock and the ledger is already saved, so the batch can be
// resumed from the first create on.
func (r submitRun) send(ctx context.Context, batch *downloadBatch) error {
	stderr := r.cmd.ErrOrStderr()
	retry := r.retryUnknown
	// Before anything is sent, a resume looks for its UNKNOWN and MISSING
	// rows on the portal: any found is CREATED and is never sent again.
	// --retry-unknown always looks first, even with --no-reconcile, and
	// sends nothing UNKNOWN when the portal cannot be read.
	if r.resume && (!r.noReconcile || retry) && batch.count(dlUnknown)+batch.count(dlMissing) > 0 {
		promoted, rerr := reconcileBeforeResend(ctx, r.sess, batch)
		if rerr != nil {
			tenantf(stderr, "warning: could not list the portal before re-sending (%v); UNKNOWN rows stay held\n", rerr)
			retry = false
		} else if promoted > 0 {
			fmt.Fprintf(stderr, "%d row(s) were found on the portal and are now CREATED, so they are not sent again\n", promoted)
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
		return unsettledDownloadErr(batch)
	}
	before, cerr := r.sess.JobCounts(ctx)
	if cerr != nil {
		tenantf(stderr, "warning: could not read the job count first (%v); empty jobs may read as missing\n", cerr)
	}
	includeDeleted, includeLegalHold := boolValue(batch.IncludeDeleted), boolValue(batch.IncludeLegalHold)
	if err := runDownloadBatch(ctx, r.cmd, r.sess, batch, sendable, batch.Format, r.concurrency, includeDeleted, includeLegalHold); err != nil {
		return err
	}
	// Only rows that may have created a job count toward what the
	// dashboard should have grown by.
	wanted := 0
	for _, row := range sendable {
		if row.Status == dlRequested || row.Status == dlUnknown {
			wanted++
		}
	}
	if !r.noReconcile {
		fmt.Fprintf(stderr, "waiting %s before reconciling, because a new job takes minutes to appear (batch %s)\n", reconcileDelay, batch.BatchKey)
		select {
		case <-ctx.Done():
			return fmt.Errorf("stopped before reconciling; batch %s is saved, continue it with: cloudally-cli download submit --batch %s --resume --yes: %w", batch.BatchKey, batch.BatchKey, ctx.Err())
		case <-time.After(reconcileDelay):
		}
		delta := 0
		if after, aerr := r.sess.JobCounts(ctx); aerr == nil {
			delta = after.Total - before.Total
			fmt.Fprintf(stderr, "portal job count went from %d to %d (+%d) for %d requested\n",
				before.Total, after.Total, delta, wanted)
		}
		if err := reconcileBatch(ctx, r.sess, batch, delta, wanted); err != nil {
			tenantf(stderr, "warning: reconcile failed (%v); rows stay REQUESTED and are unverified\n", err)
		}
		if err := saveBatch(batch); err != nil {
			return fmt.Errorf("saving the batch ledger: %w", err)
		}
	}
	if err := printDownloadBatchResult(r.cmd, r.flags, batch, r.scope); err != nil {
		return err
	}
	return unsettledDownloadErr(batch)
}

func boolValue(p *bool) bool { return p != nil && *p }

// checkResumeSettings refuses to resume a batch with settings that differ
// from the ones its archives were created with. What goes into an archive
// is part of what the batch promised: a resumed legal-hold export that
// quietly left out held items would be worse than no export.
func checkResumeSettings(cmd *cobra.Command, b *downloadBatch, format string, includeDeleted, includeLegalHold bool) error {
	if b.IncludeDeleted == nil || b.IncludeLegalHold == nil {
		return usageErr(fmt.Errorf("batch %s has no record of its --include-deleted and --include-legal-hold settings, so a resumed archive could leave out items the first run included; nothing was sent. Start a separate batch with --new-batch and the flags it needs", b.BatchKey))
	}
	var conflicts []string
	f := cmd.Flags()
	if f.Changed("format") && format != b.Format {
		conflicts = append(conflicts, fmt.Sprintf("--format %s (batch recorded %s)", format, b.Format))
	}
	if f.Changed("include-deleted") && includeDeleted != *b.IncludeDeleted {
		conflicts = append(conflicts, fmt.Sprintf("--include-deleted=%t (batch recorded %t)", includeDeleted, *b.IncludeDeleted))
	}
	if f.Changed("include-legal-hold") && includeLegalHold != *b.IncludeLegalHold {
		conflicts = append(conflicts, fmt.Sprintf("--include-legal-hold=%t (batch recorded %t)", includeLegalHold, *b.IncludeLegalHold))
	}
	if len(conflicts) > 0 {
		return usageErr(fmt.Errorf("batch %s creates its archives with different settings than this run: %s; nothing was sent. Drop the flag(s) to resume with the recorded values, or start a separate batch with --new-batch instead of --resume",
			b.BatchKey, strings.Join(conflicts, ", ")))
	}
	return nil
}

// pendingDownloadRows picks the rows a run should send. REQUESTED and
// CREATED rows are never sent again, and UNKNOWN ones only with explicit
// --retry-unknown.
func pendingDownloadRows(batch *downloadBatch, resume, retryUnknown bool) []*downloadRow {
	var out []*downloadRow
	for i := range batch.Rows {
		r := &batch.Rows[i]
		switch r.Status {
		case dlPlanned:
			out = append(out, r)
		case dlMissing, dlFailed:
			if resume {
				out = append(out, r)
			}
		case dlUnknown:
			if resume && retryUnknown {
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

// unsettledDownloadErr reports a batch that still has FAILED, UNKNOWN or
// MISSING rows, so a script never reads it as finished.
func unsettledDownloadErr(batch *downloadBatch) error {
	failed, unknown, missing := batch.count(dlFailed), batch.count(dlUnknown), batch.count(dlMissing)
	if failed+unknown+missing == 0 {
		return nil
	}
	return partialFailureErr(fmt.Errorf("batch %s is not settled: %d FAILED and %d MISSING (re-send with --batch %s --resume --yes), %d UNKNOWN (check 'download list' before --retry-unknown)",
		batch.BatchKey, failed, missing, batch.BatchKey, unknown))
}

// classifyDownloadCreateFailure decides what a failed create call means for
// the ledger, the same way restore submit does. connected reports whether a
// connection to the portal was obtained during the call, which is the
// earliest point request bytes can leave this process.
//
//   - No connection: nothing was sent (a local error, DNS, a refused
//     connection, a cancel before dialing), so the row is FAILED and safe to
//     send again.
//   - A rejected session (401/403) or a demand for an interactive redirect is
//     a clean refusal: FAILED.
//   - Anything else after a connection (a timeout, a reset, a 5xx, an answer
//     that could not be read) may have created the job: UNKNOWN.
func classifyDownloadCreateFailure(err error, connected bool) string {
	switch {
	case !connected:
		return dlFailed
	case errors.Is(err, portal.ErrNoSession), errors.Is(err, portal.ErrNeedsPortalRedirect):
		return dlFailed
	default:
		return dlUnknown
	}
}

// runDownloadBatch creates each of rows. Each row is saved to the ledger as
// REQUESTED before its create call leaves, and saved again with the answer,
// so a run killed at any point leaves a ledger that says which creates may
// have reached the portal. A row whose REQUESTED state cannot be saved is
// not sent.
func runDownloadBatch(ctx context.Context, cmd *cobra.Command, sess *portal.Session, batch *downloadBatch, rows []*downloadRow, format string, concurrency int, includeDeleted, includeLegalHold bool) error {
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var saveErr error
	notSent := 0
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
			prevStatus, prevDetail, prevRequested, prevAccepted := r.Status, r.Detail, r.RequestedAt, r.AcceptedAt
			r.Status, r.RequestedAt, r.AcceptedAt, r.Detail = dlRequested, isoUTC(time.Now()), "", ""
			if err := saveBatch(batch); err != nil {
				r.Status, r.Detail, r.RequestedAt, r.AcceptedAt = prevStatus, prevDetail, prevRequested, prevAccepted
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
				Format:           format,
				IncludeDeleted:   includeDeleted,
				IncludeLegalHold: includeLegalHold,
			})
			mu.Lock()
			if err != nil {
				r.Status, r.Detail = classifyDownloadCreateFailure(err, connected.Load()), err.Error()
			} else {
				r.AcceptedAt = isoUTC(time.Now())
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
	fmt.Fprintf(cmd.ErrOrStderr(), "requested %d archive(s)\n", len(rows)-notSent)
	if err := saveBatch(batch); err != nil {
		return fmt.Errorf("saving the batch ledger: %w", err)
	}
	if notSent > 0 {
		return fmt.Errorf("saving the batch ledger: %w; %d row(s) were not sent and stay as they were", saveErr, notSent)
	}
	if saveErr != nil {
		tenantf(cmd.ErrOrStderr(), "warning: a ledger save during the run failed (%v); the final save succeeded\n", saveErr)
	}
	return nil
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
	fmt.Fprintf(w, "  unknown:     %d\n", batch.count(dlUnknown))
	fmt.Fprintf(w, "  unverified:  %d\n", batch.count(dlRequested))
	if n := batch.count(dlMissing) + batch.count(dlFailed); n > 0 {
		fmt.Fprintf(w, "\n%d row(s) never appeared or were refused. Re-send them with:\n", n)
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
