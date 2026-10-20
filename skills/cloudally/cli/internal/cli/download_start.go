// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"cloudally-pp-cli/internal/client"
	"cloudally-pp-cli/internal/cliutil"
	"github.com/spf13/cobra"
)

// sourceForDownload picks the service to archive: an explicit sub-source
// when given, otherwise the backup task's own source.
func sourceForDownload(explicit, taskSource string) string {
	if s := strings.TrimSpace(explicit); s != "" {
		return s
	}
	return taskSource
}

// isParentMS365 reports whether a download would be asked for against an
// MS365 task, either with the parent source or with one of its sub-sources.
// Both are refused by 'download start': the parent creates no job, and a
// sub-source needs that sub-source's own timestamp, which only
// 'download submit' resolves.
func isParentMS365(source, taskSource string) bool {
	return strings.EqualFold(strings.TrimSpace(taskSource), "MS365") ||
		strings.EqualFold(strings.TrimSpace(source), "MS365")
}

// downloadStartOutput is what 'download start' prints on success.
type downloadStartOutput struct {
	Scope        string `json:"scope"`
	AccountID    string `json:"account_id,omitempty"`
	BackupTaskID string `json:"backup_task_id"`
	Entity       string `json:"entity"`
	EntityStatus string `json:"entity_status,omitempty"`
	Source       string `json:"source"`
	Requested    string `json:"requested"`
	Resolved     string `json:"resolved"`
	ResolvedRaw  int64  `json:"resolved_raw"`
	DateSource   string `json:"date_source"`
	BatchID      string `json:"batch_id,omitempty"`
	Format       string `json:"format"`
	Destination  string `json:"destination"`
	JobLabel     string `json:"job_label,omitempty"`
	// BatchKey is the one-archive batch the create is recorded under.
	BatchKey string `json:"batch_key,omitempty"`
}

// Archive formats 'download start' can ask for. CLOUDALLY_S3 is the
// storage type the portal labels "For download", which is the plain zip.
const (
	downloadFormatZip        = "zip"
	downloadFormatPST        = "pst"
	downloadFormatSinglePST  = "single-pst"
	downloadDestinationOwned = "CLOUDALLY_S3"
)

var downloadFormats = []string{downloadFormatZip, downloadFormatPST, downloadFormatSinglePST}

func newDownloadStartCmd(flags *rootFlags) *cobra.Command {
	var scope restoreScope
	var flagTask, flagEntity, flagDate, flagFormat, flagResolve, flagPortalURL string
	var flagSource, flagLabel string
	var flagLookback int
	var flagIncludeDeleted, flagIncludeLegalHold bool

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Build a downloadable archive of one mailbox from a chosen backup date",
		Long: strings.Trim(`
Creates a download (export) job for ONE mailbox from the backup taken on or
before the requested date. The portal returns no job id, so it prints the
'download list' command to find it.

The create runs as a one-archive batch through the same download guard and
ledger as 'download submit': the batch key is printed, the same start again
is refused (exit 2), and 'download submit --batch <key> --resume --yes'
confirms the job without creating another.

This is deliberately one mailbox per run: a download is a real job against a
customer's data, and nothing about this command should make it easy to fan
out across a whole task by accident.

The backup date is resolved the same way 'restore plan' resolves it, from
the portal's per-mailbox backup calendar, so a past date picks that date's
backup rather than the most recent one. A mailbox with no successful backup
at or before the date is refused instead of exported from some other date.

Nothing is created without --yes, including under --agent, --no-input, or
when output is piped. The archive lands in CloudAlly's own storage, which is
what makes it downloadable.

MS365 mailboxes are refused here: their download is one job per sub-source
(mail, calendar, contacts, tasks, notes), each with its own timestamp, so use
'download submit --entities <mailbox>' for them. The documented
'distributor-recovery export' endpoint answers 200 and creates no job, so it
is not used.
`, "\n"),
		Example: strings.Trim(`
  cloudally-cli download start --account 5f1c2a --task 64b0e9 --entity jane.doe@contoso.com --date 2025-12-31 --dry-run
  cloudally-cli download start --account 5f1c2a --task 64b0e9 --entity jane.doe@contoso.com --date 2025-12-31 --yes
  cloudally-cli download start --account 5f1c2a --task 64b0e9 --entity jane.doe@contoso.com --date 2025-12-31 --format pst --yes
`, "\n"),
		Annotations: map[string]string{
			"pp:data-source": "live",
			"pp:happy-args":  "--account=5f1c2a;--task=64b0e9;--entity=jane.doe@contoso.com;--date=2025-12-31;--yes",
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
			if strings.TrimSpace(flagTask) == "" {
				return usageErr(errors.New("--task <backup task id> is required"))
			}
			if strings.TrimSpace(flagEntity) == "" {
				return usageErr(errors.New("--entity <mailbox> is required; this command exports one mailbox per run"))
			}
			if scope.account == "" {
				return usageErr(errPortalNeedsAccount)
			}
			format := strings.ToLower(strings.TrimSpace(flagFormat))
			if format == "" {
				format = downloadFormatZip
			}
			if format != downloadFormatZip && format != downloadFormatPST && format != downloadFormatSinglePST {
				return usageErr(fmt.Errorf("--format %q is not one of %s", flagFormat, strings.Join(downloadFormats, ", ")))
			}
			resolveMode, err := validateResolveMode(flagResolve, scope)
			if err != nil {
				return err
			}
			dates, err := parseRequestedDates([]string{flagDate})
			if err != nil {
				return err
			}
			requested := dates[0]

			// A download job is a real side effect, so --dry-run and any
			// Printing Press harness stop before the network entirely.
			if dryRunOK(flags) || cliutil.IsAnyHarness() {
				return printDownloadStartDryRun(cmd, flags, scope, flagTask, flagEntity, requested, format)
			}

			c, err := flags.newClient()
			if err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			task, err := fetchTaskWithEntities(ctx, c, scope.taskPath(flagTask))
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			ent, ok := findTaskEntity(task, flagEntity)
			if !ok {
				return notFoundErr(fmt.Errorf("entity %q is not on task %s; list the task's mailboxes with 'restore plan --account %s --task %s --date %s'",
					flagEntity, flagTask, scope.account, flagTask, requested.text))
			}

			row, err := resolveDownloadDate(ctx, c, scope, task, ent, requested, resolveMode, flagPortalURL, flagLookback)
			if err != nil {
				return err
			}
			if row.Status != planOK {
				return notFoundErr(fmt.Errorf("%s has no usable backup for %s: %s", tenantText(ent.Entity), requested.text, tenantText(row.Detail)))
			}

			out := downloadStartOutput{
				Scope: scope.kind(), AccountID: scope.account, BackupTaskID: flagTask,
				Entity: ent.Entity, EntityStatus: ent.Status, Source: task.Source,
				Requested: requested.text, Resolved: row.Resolved, ResolvedRaw: row.ResolvedRaw,
				DateSource: row.DateSource, BatchID: row.BatchID,
				Format: format, Destination: downloadDestinationOwned,
			}
			if !flags.yes {
				printDownloadStartConfirmation(cmd.ErrOrStderr(), out, scope)
				return usageErr(errors.New("add --yes to create this download job"))
			}
			// An MS365 mailbox has no single archive: a job asked for with the
			// parent MS365 source is accepted by the portal and creates nothing,
			// and each sub-source needs its own timestamp. 'download submit'
			// handles both, so send the operator there instead.
			if isParentMS365(sourceForDownload(flagSource, task.Source), task.Source) {
				return usageErr(fmt.Errorf("%s is on an MS365 task, where a download is one job per sub-source (mail, calendar, contacts, tasks, notes), each with its own backup timestamp; a single job for the whole mailbox creates nothing. Use: cloudally-cli download submit --account %s --task %s --on-date %s --entities %s --yes",
					ent.Entity, scope.account, flagTask, requested.text, ent.Entity))
			}
			// The documented recovery/export endpoint answers 200 with an
			// empty body and creates nothing, so the job is created the
			// way the portal does it, through the same machinery as
			// 'download submit': a batch of one archive, claimed in the
			// download guard before the create, so running the same
			// command twice never creates a second job.
			sess, err := openPortalSession(ctx, c, scope, flagPortalURL)
			if err != nil {
				return err
			}
			batch := startBatch(scope, flagTask, task.Source, ent, sourceForDownload(flagSource, task.Source), requested.text, row,
				format, flagLabel, flagIncludeDeleted, flagIncludeLegalHold)
			guard, err := openDownloadGuard(ctx)
			if err != nil {
				return err
			}
			defer guard.Close()
			if err := guard.seedFromLedgers(ctx); err != nil {
				return usageErr(err)
			}
			leftOut, err := refuseHeldArchives(ctx, cmd, guard, batch)
			if err != nil {
				return err
			}
			if err := withBatchLock(batch.BatchKey, func() error {
				if err := claimNewBatch(batch, false); err != nil {
					return err
				}
				printLeftOut(cmd.ErrOrStderr(), leftOut)
				return runDownloadBatch(ctx, cmd, sess, guard, batch, []*downloadRow{&batch.Rows[0]}, 1, false, false)
			}); err != nil {
				return err
			}
			if err := startOutcome(batch, scope); err != nil {
				return err
			}
			out.BatchKey = batch.BatchKey
			out.Source = sourceForDownload(flagSource, task.Source)
			out.JobLabel = flagLabel
			return printDownloadStart(cmd, flags, out, scope)
		},
	}
	addRestoreScopeFlags(cmd, &scope)
	cmd.Flags().StringVar(&flagTask, "task", "", "Backup task id the mailbox belongs to")
	cmd.Flags().StringVar(&flagEntity, "entity", "", "The one mailbox (or site, or drive) to build an archive of")
	cmd.Flags().StringVar(&flagDate, "date", "", restoreDateFlagHelp)
	cmd.Flags().StringVar(&flagFormat, "format", downloadFormatZip,
		"Archive format: zip (default, one archive), pst (split into chunks), or single-pst (one PST file)")
	cmd.Flags().StringVar(&flagSource, "source", "",
		"Service to archive: the task's own source (a whole mailbox) by default, or one sub-source such as O365MAIL for mail only")
	cmd.Flags().StringVar(&flagLabel, "label", "", "Free-text label stored on the download job")
	cmd.Flags().BoolVar(&flagIncludeDeleted, "include-deleted", false, "Include deleted items in the archive")
	cmd.Flags().BoolVar(&flagIncludeLegalHold, "include-legal-hold", false, "Include items under legal hold")
	addResolveFlags(cmd, &flagResolve, &flagPortalURL, &flagLookback)
	return cmd
}

// resolveDownloadDate resolves one entity's backup for the requested date,
// reusing the same resolution 'restore plan' uses so a download and a
// restore of the same date never disagree about which backup that is.
func resolveDownloadDate(ctx context.Context, c *client.Client, scope restoreScope, task *taskWithEntities, ent taskEntity, requested requestedDate, resolveMode, portalURL string, lookback int) (planRow, error) {
	single := &taskWithEntities{ID: task.ID, Source: task.Source, Entities: []taskEntity{ent}}
	var resolver snapshotResolver
	if wantsSnapshotResolution(resolveMode, scope) {
		r, err := openPortalResolver(ctx, c, scope, portalURL, lookback)
		if err != nil {
			// A download names one backup date; guessing it from the
			// latest-backup table would be the wrong archive, so this
			// fails rather than quietly downgrading.
			return planRow{}, fmt.Errorf("opening the portal backup calendar to resolve %s: %w", requested.text, err)
		}
		resolver = r
	}
	rows, warnings := buildPlanWithResolver(ctx, single, []requestedDate{requested}, nil, "", resolver)
	if len(rows) != 1 {
		return planRow{}, fmt.Errorf("resolving %s for %s produced %d rows", requested.text, ent.Entity, len(rows))
	}
	// A per-entity warning is already recorded on the row's Detail, and
	// the caller refuses any row that is not OK, so it needs no separate
	// channel here.
	_ = warnings
	return rows[0], nil
}

// exportBody builds the ExportDTO for one mailbox. storageType is left
// unset so CloudAlly's own storage is used, which is the downloadable case.
func exportBody(taskID, source string, ent taskEntity, dateRaw int64, format string, includeDeleted, includeLegalHold bool) map[string]any {
	body := map[string]any{
		"taskId":           taskID,
		"account":          ent.Entity,
		"source":           source,
		"date":             dateRaw,
		"includeDeleted":   includeDeleted,
		"includeLegalHold": includeLegalHold,
	}
	if extID := strings.TrimSpace(string(ent.ExtID)); extID != "" {
		body["extid"] = extID
	}
	switch format {
	case downloadFormatPST:
		body["downloadToPst"] = true
	case downloadFormatSinglePST:
		body["downloadToPst"] = true
		body["preferSinglePST"] = true
	}
	return body
}

// printDownloadStartConfirmation shows what a real run would create.
func printDownloadStartConfirmation(w io.Writer, out downloadStartOutput, scope restoreScope) {
	fmt.Fprintf(w, "download start would create 1 download job; nothing has been created\n")
	tenantf(w, "  account:  %s\n", out.AccountID)
	tenantf(w, "  task:     %s (%s)\n", out.BackupTaskID, out.Source)
	tenantf(w, "  mailbox:  %s (%s)\n", out.Entity, out.EntityStatus)
	tenantf(w, "  asked:    %s\n", out.Requested)
	tenantf(w, "  backup:   %s (%d), from %s\n", out.Resolved, out.ResolvedRaw, out.DateSource)
	fmt.Fprintf(w, "  format:   %s into CloudAlly storage, for download\n", out.Format)
	fmt.Fprintf(w, "\nadd --yes to create it, then follow it with: cloudally-cli download watch --account %s --job <job id>\n", scope.account)
}

// printDownloadStart reports a created job.
func printDownloadStart(cmd *cobra.Command, flags *rootFlags, out downloadStartOutput, scope restoreScope) error {
	if !wantsHumanTable(cmd.OutOrStdout(), flags) {
		return printJSONFiltered(cmd.OutOrStdout(), out, flags)
	}
	w := cmd.OutOrStdout()
	tenantf(w, "download job created for %s (batch %s)\n", out.Entity, out.BatchKey)
	tenantf(w, "  backup:  %s (%d), from %s\n", out.Resolved, out.ResolvedRaw, out.DateSource)
	fmt.Fprintf(w, "  format:  %s\n", out.Format)
	fmt.Fprintf(w, "\nThe API does not return the job id, so find it with:\n")
	fmt.Fprintf(w, "  cloudally-cli download list --account %s\n", scope.account)
	fmt.Fprintf(w, "then follow it with 'download watch' and get the link with 'download link'.\n")
	fmt.Fprintf(w, "Running this command again will not create a second job; confirm this one with: cloudally-cli download submit --batch %s --resume --yes\n", out.BatchKey)
	return nil
}

// printDownloadStartDryRun prints the exact body a real run would send.
func printDownloadStartDryRun(cmd *cobra.Command, flags *rootFlags, scope restoreScope, taskID, entity string, requested requestedDate, format string) error {
	mode := "dry-run"
	if h := cliutil.HarnessName(); h != "" {
		mode = h + " harness"
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "%s: would resolve %s for %s then POST %s; nothing created\n",
		mode, requested.text, entity, scope.exportPath())
	return writeWould(cmd.OutOrStdout(), flags, "download start",
		fmt.Sprintf("create a %s download job for %s from the backup at or before %s", format, entity, requested.text),
		map[string]any{
			"path": scope.exportPath(), "task": taskID, "entity": entity,
			"requested": requested.text, "format": format, "destination": downloadDestinationOwned,
		})
}

// startBatch is the one-archive batch 'download start' records its create
// under.
func startBatch(scope restoreScope, taskID, taskSource string, ent taskEntity, source, requested string, row planRow, format, label string, includeDeleted, includeLegalHold bool) *downloadBatch {
	now := time.Now()
	incDel, incLH := includeDeleted, includeLegalHold
	b := &downloadBatch{
		BatchKey: newBatchKey(now, scope.account), Scope: scope.kind(), AccountID: scope.account,
		TaskID: taskID, Source: taskSource, OnDate: requested, Timezone: "UTC", Format: format,
		IncludeDeleted: &incDel, IncludeLegalHold: &incLH,
		EntityFilter:    []string{strings.ToLower(strings.TrimSpace(ent.Entity))},
		SubSourceFilter: []string{strings.ToUpper(strings.TrimSpace(source))},
		CreatedAt:       isoUTC(now),
		Rows: []downloadRow{{
			Entity: ent.Entity, EntityStatus: ent.Status, ExtID: string(ent.ExtID), SubSource: source,
			SnapshotRaw: row.ResolvedRaw, Snapshot: row.Resolved, BatchID: row.BatchID, Status: dlPlanned,
			// An operator's --label is sent as the job label, and is what
			// the job is recognised by afterwards.
			Label: strings.TrimSpace(label),
		}},
	}
	b.RerunKey = rerunIdentity(b.Scope, b.AccountID, b.TaskID, b.OnDate, b.Format, b.EntityFilter, b.SubSourceFilter, incDel, incLH)
	return b
}

// startOutcome turns the one row's result into the command's answer.
func startOutcome(b *downloadBatch, scope restoreScope) error {
	r := b.Rows[0]
	switch r.Status {
	case dlRequested:
		return nil
	case dlUnknown:
		return fmt.Errorf("the create's outcome is unknown (%s). Batch %s holds this archive, so running this again will not create a second job; check 'cloudally-cli download list --account %s', then confirm it with: cloudally-cli download submit --batch %s --resume --yes",
			tenantText(r.Detail), b.BatchKey, scope.account, b.BatchKey)
	case dlSkipped:
		return usageErr(fmt.Errorf("%s; nothing was sent", tenantText(r.Detail)))
	default:
		return fmt.Errorf("the portal did not create the download job: %s", tenantText(r.Detail))
	}
}
