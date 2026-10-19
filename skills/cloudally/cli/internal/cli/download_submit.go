// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"cloudally-pp-cli/internal/cadate"
	"cloudally-pp-cli/internal/cliutil"
	"cloudally-pp-cli/internal/portal"
	"github.com/spf13/cobra"
)

// Ledger statuses for one (mailbox, sub-source) archive.
//
// REQUESTED is written before the create call leaves. CREATED means the job
// was afterwards found on the portal, which is the only proof it exists: the
// create call answers "Your export request has been scheduled." even when it
// quietly does nothing. MISSING is a row that was requested and never
// appeared, and is what a resume re-sends.
const (
	dlPlanned   = "PLANNED"
	dlRequested = "REQUESTED"
	dlCreated   = "CREATED"
	dlMissing   = "MISSING"
	dlFailed    = "FAILED"
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
const reconcileDelay = 4 * time.Minute

// defaultDownloadConcurrency bounds parallel create calls. The portal's own
// rate limiter still paces the requests.
const defaultDownloadConcurrency = 4

// downloadRow is one archive in a batch: one mailbox, one sub-source.
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
	Timezone  string         `json:"timezone"`
	Format    string         `json:"format"`
	CreatedAt string         `json:"created_at"`
	UpdatedAt string         `json:"updated_at"`
	Rows      []downloadRow  `json:"rows"`
	Skipped   []downloadSkip `json:"skipped_entities,omitempty"`
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

// batchPath returns where a batch ledger lives.
func batchPath(key string) (string, error) {
	dir, err := cliutil.StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, downloadBatchDirName, key+".json"), nil
}

func saveBatch(b *downloadBatch) error {
	path, err := batchPath(b.BatchKey)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b.UpdatedAt = isoUTC(time.Now())
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), batchFilePerm)
}

func loadBatch(key string) (*downloadBatch, error) {
	path, err := batchPath(key)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path) // #nosec G304 -- key is a batch name from the state dir.
	if err != nil {
		return nil, err
	}
	var b downloadBatch
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("reading batch %s: %w", key, err)
	}
	return &b, nil
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
	type key struct {
		entity, sub string
		snap        int64
	}
	found := map[key]string{}
	for _, j := range jobs {
		found[key{strings.ToLower(j.Account), strings.ToUpper(j.SubSource), j.SnapshotDate}] = j.JobID
	}
	now := isoUTC(time.Now())
	for i := range batch.Rows {
		r := &batch.Rows[i]
		if r.Status != dlRequested && r.Status != dlMissing && r.Status != dlCreated {
			continue
		}
		k := key{strings.ToLower(r.Entity), strings.ToUpper(r.SubSource), r.SnapshotRaw}
		r.ReconciledAt = now
		if id, ok := found[k]; ok {
			r.JobID = id
			r.Status = dlCreated
			r.Detail = ""
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

func newDownloadSubmitCmd(flags *rootFlags) *cobra.Command {
	var scope restoreScope
	var flagTask, flagOnDate, flagFormat, flagPortalURL, flagBatch, flagPlanOut, flagMinSize string
	var flagUTCOffset float64
	var flagEntities, flagSubSources []string
	var flagEntitiesFile string
	var flagConcurrency int
	var flagIncludeDeleted, flagIncludeLegalHold, flagResume, flagNoReconcile, flagLatest bool
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
no job id. So every row is written as REQUESTED, and a reconcile pass then
lists the portal and marks each row CREATED or MISSING. Re-run with --resume
to send the MISSING ones again.

Nothing is created without --yes.
`, "\n"),
		Example: strings.Trim(`
  cloudally-cli download submit --account 5f1c2a --task 64b0e9 --on-date 2022-12-31 --dry-run
  cloudally-cli download submit --account 5f1c2a --task 64b0e9 --on-date 2022-12-31 --entities jane.doe@contoso.com --yes
  cloudally-cli download submit --account 5f1c2a --task 64b0e9 --on-date 2022-12-31 --yes
  cloudally-cli download submit --batch 20261015-201500-ab12 --resume --yes
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
				b, err := loadBatch(flagBatch)
				if err != nil {
					return usageErr(fmt.Errorf("--batch %s: %w", flagBatch, err))
				}
				batch = b
				scope = scopeFromKind(b.Scope, b.AccountID)
				flagTask, format = b.TaskID, b.Format
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
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

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
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: entity %q is not on task %s\n", m, flagTask)
				}
				// Per-entity sizes come from the portal: the documented
				// entity record carries none, and without them a run
				// creates jobs for mailboxes that can only be empty.
				sizes, serr := sess.TaskEntityInfos(ctx, flagTask)
				if serr != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: mailbox sizes unavailable (%v); no mailbox skipped for being empty\n", serr)
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
				b.BatchKey = time.Now().UTC().Format("20060102-150405") + "-" + shortKey(scope.account)
				b.CreatedAt = isoUTC(time.Now())
				batch = b
			}

			sendable := pendingDownloadRows(batch, flagResume)
			if flagPlanOut != "" {
				if werr := writeBatchTo(flagPlanOut, batch); werr != nil {
					return fmt.Errorf("writing --plan-out: %w", werr)
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "plan written to %s\n", flagPlanOut)
			}
			if dryRunOK(flags) {
				return printDownloadBatchPlan(cmd, flags, batch, sendable, true)
			}
			if len(sendable) == 0 {
				return printDownloadBatchPlan(cmd, flags, batch, sendable, false)
			}
			if !flags.yes {
				if err := printDownloadBatchPlan(cmd, flags, batch, sendable, false); err != nil {
					return err
				}
				return usageErr(fmt.Errorf("add --yes to create these %d archive(s)", len(sendable)))
			}

			if err := saveBatch(batch); err != nil {
				return fmt.Errorf("saving the batch ledger: %w", err)
			}
			before, cerr := sess.JobCounts(ctx)
			if cerr != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not read the job count first (%v); empty jobs may read as missing\n", cerr)
			}
			wanted := len(sendable)
			if err := runDownloadBatch(ctx, cmd, sess, batch, format, flagConcurrency, flagIncludeDeleted, flagIncludeLegalHold); err != nil {
				return err
			}
			if !flagNoReconcile {
				fmt.Fprintf(cmd.ErrOrStderr(), "waiting %s before reconciling, because a new job takes minutes to appear\n", reconcileDelay)
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(reconcileDelay):
				}
				delta := 0
				if after, aerr := sess.JobCounts(ctx); aerr == nil {
					delta = after.Total - before.Total
					fmt.Fprintf(cmd.ErrOrStderr(), "portal job count went from %d to %d (+%d) for %d requested\n",
						before.Total, after.Total, delta, wanted)
				}
				if err := reconcileBatch(ctx, sess, batch, delta, wanted); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: reconcile failed (%v); rows stay REQUESTED and are unverified\n", err)
				}
				if err := saveBatch(batch); err != nil {
					return fmt.Errorf("saving the batch ledger: %w", err)
				}
			}
			return printDownloadBatchResult(cmd, flags, batch, scope)
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
	cmd.Flags().BoolVar(&flagIncludeDeleted, "include-deleted", false, "Include deleted items")
	cmd.Flags().BoolVar(&flagIncludeLegalHold, "include-legal-hold", false, "Include items under legal hold")
	cmd.Flags().StringVar(&flagMinSize, "min-size", "",
		"Skip mailboxes holding less than this, for example 10MB. Mailboxes reporting 0 bytes or Not Accessible are always skipped, since they can only produce empty archives")
	cmd.Flags().StringVar(&flagBatch, "batch", "", "Batch key to resume")
	cmd.Flags().BoolVar(&flagResume, "resume", false, "Re-send only the rows that never appeared on the portal")
	cmd.Flags().BoolVar(&flagNoReconcile, "no-reconcile", false, "Skip the reconcile pass; rows stay REQUESTED and unverified")
	cmd.Flags().StringVar(&flagPlanOut, "plan-out", "", "Write the batch plan JSON to this path")
	cmd.Flags().StringVar(&flagPortalURL, "portal-url", portal.DefaultBaseURL, portalURLFlagHelp)
	cmd.Flags().IntVar(&flagLookback, "lookback-months", portal.DefaultMaxLookbackMonths, "Unused for --on-date; kept so flags match 'restore plan'")
	_ = cmd.Flags().MarkHidden("portal-url")
	_ = cmd.Flags().MarkHidden("lookback-months")
	return cmd
}

// pendingDownloadRows picks the rows a run should send.
func pendingDownloadRows(batch *downloadBatch, resume bool) []*downloadRow {
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
		}
	}
	return out
}

// runDownloadBatch creates each pending archive, writing REQUESTED before
// the call leaves so an interrupted run is recoverable.
func runDownloadBatch(ctx context.Context, cmd *cobra.Command, sess *portal.Session, batch *downloadBatch, format string, concurrency int, includeDeleted, includeLegalHold bool) error {
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	rows := pendingDownloadRows(batch, true)
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
			r.Status, r.RequestedAt = dlRequested, isoUTC(time.Now())
			mu.Unlock()
			err := sess.StartDownload(ctx, portal.DownloadRequest{
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
				r.Status, r.Detail = dlFailed, err.Error()
			}
			mu.Unlock()
		}(row)
	}
	wg.Wait()
	fmt.Fprintf(cmd.ErrOrStderr(), "requested %d archive(s)\n", len(rows))
	return saveBatch(batch)
}

func printDownloadBatchPlan(cmd *cobra.Command, flags *rootFlags, batch *downloadBatch, sendable []*downloadRow, dry bool) error {
	if !wantsHumanTable(cmd.OutOrStdout(), flags) {
		return printJSONFiltered(cmd.OutOrStdout(), batch, flags)
	}
	w := cmd.OutOrStdout()
	if len(batch.Rows) > 0 {
		if err := printRestoreTable(w, downloadRowHeaders, downloadRowsAsMaps(batch.Rows)); err != nil {
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
			fmt.Fprintf(w, "    %-46s %-11s %s\n", s.Entity, s.EntityStatus, s.Reason)
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
	fmt.Fprintf(w, "  unverified:  %d\n", batch.count(dlRequested))
	if n := batch.count(dlMissing); n > 0 {
		fmt.Fprintf(w, "\n%d row(s) never appeared. Re-send them with:\n", n)
		fmt.Fprintf(w, "  cloudally-cli download submit --batch %s --resume --yes\n", batch.BatchKey)
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

// shortKey makes a stable short suffix for a batch name.
func shortKey(s string) string {
	s = strings.ReplaceAll(strings.ToLower(strings.TrimSpace(s)), "-", "")
	if len(s) > 4 {
		return s[:4]
	}
	return s
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
