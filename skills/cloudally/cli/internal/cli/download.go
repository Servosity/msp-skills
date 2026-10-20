// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"cloudally-pp-cli/internal/cadate"
	"cloudally-pp-cli/internal/client"
	"cloudally-pp-cli/internal/portal"
	"github.com/spf13/cobra"
)

// exportTaskListType is the task type filter for export (download) jobs.
// CloudAlly calls the job EXPORT on the API and "Download" in the portal.
const exportTaskListType = "EXPORT"

// Terminal states for an export job. READY_FOR_DOWNLOAD is the success
// state: the archive is built and a link can be minted.
var exportFinishedStatuses = map[string]bool{
	portal.StatusReadyForDownload: true,
	"COMPLETE":                    true,
	"COMPLETED":                   true,
	"SUCCEED":                     true,
	"SUCCEEDED":                   true,
	"FAILED":                      true,
	"PARTIALLY_SUCCEEDED":         true,
	"CANCELLED":                   true,
	"CANCELED":                    true,
}

// exportFailureWords mark a status as a terminal failure even when it is
// not one of the names above (ABORTED, STOPPED, ERROR, EXPORT_FAILED, ...).
// Without them an unlisted failure counted as still running: download
// watch polled forever, and fetch kept the job NOT_READY.
var exportFailureWords = []string{"FAIL", "ERROR", "CANCEL", "ABORT", "STOP"}

// exportJobFailed is the one terminal-failure classifier: a job whose
// status carries a failure word ended without an archive and never will
// produce one. Watch, fetch, fetch's advice and batch reconciliation all
// use it, so a job one of them calls failed is JOB_FAILED in all of them.
func exportJobFailed(status string) bool {
	s := strings.ToUpper(strings.TrimSpace(status))
	for _, w := range exportFailureWords {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

// exportJobFinished reports whether an export job has stopped moving. A
// status that is neither listed nor a failure is treated as still running.
func exportJobFinished(status string) bool {
	return exportFinishedStatuses[strings.ToUpper(strings.TrimSpace(status))] || exportJobFailed(status)
}

// exportJobReady reports whether an export job's archive can be downloaded.
func exportJobReady(status string) bool {
	return strings.EqualFold(strings.TrimSpace(status), portal.StatusReadyForDownload)
}

// downloadJobRow is one export job as this CLI reports it.
type downloadJobRow struct {
	JobID        string `json:"job_id"`
	Status       string `json:"status"`
	Entity       string `json:"entity"`
	BackupTaskID string `json:"backup_task_id,omitempty"`
	Source       string `json:"source,omitempty"`
	SubSource    string `json:"sub_source,omitempty"`
	SnapshotDate string `json:"snapshot_date,omitempty"`
	SnapshotRaw  int64  `json:"snapshot_raw,omitempty"`
	CreatedAt    string `json:"created_at,omitempty"`
	JobLabel     string `json:"job_label,omitempty"`
	Files        int    `json:"files,omitempty"`
	Ready        bool   `json:"ready"`
}

var downloadJobHeaders = []string{"job_id", "status", "entity", "sub_source", "snapshot_date", "files", "ready"}

func downloadJobRowsAsMaps(rows []downloadJobRow) [][]string {
	out := make([][]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, []string{
			r.JobID, r.Status, r.Entity, r.SubSource, r.SnapshotDate,
			strconv.Itoa(r.Files), fmt.Sprintf("%t", r.Ready),
		})
	}
	return out
}

// downloadJobRowFrom converts a portal job into a report row.
func downloadJobRowFrom(j portal.DownloadJob) downloadJobRow {
	row := downloadJobRow{
		JobID:       j.JobID,
		Status:      j.Status,
		Entity:      j.Account,
		Source:      j.Source,
		SubSource:   j.SubSource,
		SnapshotRaw: j.SnapshotDate,
		JobLabel:    j.JobLabel,
		Files:       len(j.Chunks),
		Ready:       j.Ready(),
	}
	if ts, err := j.SnapshotTime(); err == nil {
		row.SnapshotDate = isoUTC(ts)
	}
	if ts, err := cadate.ParseCompact(j.InitDate); err == nil {
		row.CreatedAt = isoUTC(ts)
	}
	return row
}

// fetchExportJobs lists every download job for the account.
//
// This reads the portal rather than the documented backup-tasks list: a
// running export disappears from that list within a couple of minutes, so it
// cannot be used to follow a job to completion.
func fetchExportJobs(ctx context.Context, c *client.Client, scope restoreScope, portalURL string) ([]portal.DownloadJob, error) {
	sess, err := openPortalSession(ctx, c, scope, portalURL)
	if err != nil {
		return nil, err
	}
	return sess.DownloadJobs(ctx)
}

// findExportJob looks up one download job by id.
func findExportJob(ctx context.Context, c *client.Client, scope restoreScope, portalURL, jobID string) (portal.DownloadJob, bool, error) {
	jobs, err := fetchExportJobs(ctx, c, scope, portalURL)
	if err != nil {
		return portal.DownloadJob{}, false, err
	}
	for _, j := range jobs {
		if strings.EqualFold(strings.TrimSpace(j.JobID), strings.TrimSpace(jobID)) {
			return j, true, nil
		}
	}
	return portal.DownloadJob{}, false, nil
}

func newDownloadCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "download",
		Short: "Create, follow, and fetch links for mailbox download (export) jobs",
		Long: strings.Trim(`
Builds a downloadable archive of one mailbox from a chosen backup date and
hands back the link to give the customer.

CloudAlly calls this an EXPORT on its API and a "Download" in its portal;
they are the same job. 'start' creates it, 'watch' follows it to
READY_FOR_DOWNLOAD, and 'link' mints the download URL.

The link is the one piece the documented API cannot provide: it comes from
the customer portal's own service layer, reached with the same partner
handoff the backup calendar uses, so 'link' needs a partner login and
--account. Links are time limited; 'link' reports the expiry it reads from
the URL and can be run again to mint a fresh one.
`, "\n"),
		Example: strings.Trim(`
  cloudally-cli download start --account 5f1c2a --task 64b0e9 --entity jane.doe@contoso.com --date 2025-12-31 --yes
  cloudally-cli download list --account 5f1c2a
  cloudally-cli download watch --account 5f1c2a --job 9f2b1c
  cloudally-cli download link --account 5f1c2a --job 9f2b1c
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:typed-exit-codes": "0,2"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newDownloadStartCmd(flags))
	cmd.AddCommand(newDownloadSubmitCmd(flags))
	cmd.AddCommand(newDownloadFetchCmd(flags))
	cmd.AddCommand(newDownloadListCmd(flags))
	cmd.AddCommand(newDownloadWatchCmd(flags))
	cmd.AddCommand(newDownloadLinkCmd(flags))
	// Errors reach the terminal through the root's printer, so each
	// subcommand's error text is scrubbed on the way out.
	for _, sub := range cmd.Commands() {
		if sub.RunE != nil {
			sub.RunE = scrubRunE(sub.RunE)
		}
	}
	return cmd
}

// newDownloadListCmd lists export jobs for an account.
func newDownloadListCmd(flags *rootFlags) *cobra.Command {
	var scope restoreScope
	var flagPortalURL string
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List download (export) jobs for an account",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := scope.validate(); err != nil {
				return err
			}
			if scope.account == "" {
				return usageErr(errPortalNeedsAccount)
			}
			if restoreShortCircuit(flags) {
				return writeWould(cmd.OutOrStdout(), flags, "download list",
					"read the portal's download job list", nil)
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			jobs, err := fetchExportJobs(ctx, c, scope, flagPortalURL)
			if err != nil {
				return err
			}
			rows := make([]downloadJobRow, 0, len(jobs))
			for _, j := range jobs {
				rows = append(rows, downloadJobRowFrom(j))
			}
			if wantsHumanTable(cmd.OutOrStdout(), flags) {
				if len(rows) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "no download (export) jobs on this account")
					return nil
				}
				return printRestoreTable(cmd.OutOrStdout(), downloadJobHeaders, tenantCells(downloadJobRowsAsMaps(rows)))
			}
			return printJSONFiltered(cmd.OutOrStdout(), rows, flags)
		},
	}
	addRestoreScopeFlags(cmd, &scope)
	cmd.Flags().StringVar(&flagPortalURL, "portal-url", portal.DefaultBaseURL, portalURLFlagHelp)
	_ = cmd.Flags().MarkHidden("portal-url")
	return cmd
}

// newDownloadLinkCmd mints the download URL for a finished export job.
func newDownloadLinkCmd(flags *rootFlags) *cobra.Command {
	var scope restoreScope
	var flagJob, flagPortalURL string
	var flagFiles, flagSkipStatusCheck bool

	cmd := &cobra.Command{
		Use:   "link",
		Short: "Mint the download link for a finished download (export) job",
		Long: strings.Trim(`
Mints a download link for each file of an export job and reports when each
link expires. A zip export is one file, so it gets one link; a PST export
split into chunks gets one link per chunk.

Links are minted per file, from the file's export data id, the same way
'download fetch' mints them: the portal refuses a link asked for by the job
id. The link comes from the customer portal's service layer, so this needs a
partner login and --account. The portal mints a fresh link on every call, so
running this again after a link expires produces a working one for as long as
CloudAlly retains the export itself.

The expiry is read from the link, not guessed. If the URL states no expiry,
the command says so rather than implying the link is durable.
`, "\n"),
		Example: strings.Trim(`
  cloudally-cli download link --account 5f1c2a --job 9f2b1c
  cloudally-cli download link --account 5f1c2a --job 9f2b1c --files
  cloudally-cli download link --account 5f1c2a --job 9f2b1c --json
`, "\n"),
		Annotations: map[string]string{"pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if !hasChangedLocalFlags(cmd) && len(args) == 0 {
				return cmd.Help()
			}
			if err := scope.validate(); err != nil {
				return err
			}
			if strings.TrimSpace(flagJob) == "" {
				return usageErr(errors.New("--job <export job id> is required; find it with 'download list'"))
			}
			if scope.account == "" {
				return usageErr(errPortalNeedsAccount)
			}
			if restoreShortCircuit(flags) {
				return writeWould(cmd.OutOrStdout(), flags, "download link",
					fmt.Sprintf("mint a download link for export job %s", flagJob),
					map[string]any{"job": flagJob})
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			// Check the job is actually ready first: minting a link for a
			// job still running returns the same "expired" failure as an
			// unknown job, which is a confusing thing to hand an operator.
			var job portal.DownloadJob
			if !flagSkipStatusCheck {
				j, found, ferr := findExportJob(ctx, c, scope, flagPortalURL, flagJob)
				if ferr != nil {
					return ferr
				}
				if !found {
					return notFoundErr(fmt.Errorf("export job %s is not on account %s; list them with 'download list --account %s'", flagJob, scope.account, scope.account))
				}
				if !j.Ready() {
					return fmt.Errorf("export job %s is %s, not %s; follow it with 'download watch --account %s --job %s'",
						flagJob, tenantText(j.Status), portal.StatusReadyForDownload, scope.account, flagJob)
				}
				job = j
			}

			sess, err := openPortalSession(ctx, c, scope, flagPortalURL)
			if err != nil {
				return err
			}
			out := downloadLinkOutput{JobID: flagJob, Entity: job.Account, Status: job.Status}
			// Both modes list the job's files and mint each link from that
			// file's export data id. A listing row carries no link, and a
			// link asked for with the job id fails as "expired".
			files, ferr := mintDownloadLinks(ctx, sess, flagJob)
			if ferr != nil {
				return ferr
			}
			for _, f := range files {
				out.Files = append(out.Files, downloadFileOut(f))
			}
			if len(out.Files) == 0 {
				return notFoundErr(fmt.Errorf("export job %s produced no downloadable files", flagJob))
			}
			return printDownloadLink(cmd, flags, out)
		},
	}
	addRestoreScopeFlags(cmd, &scope)
	cmd.Flags().StringVar(&flagJob, "job", "", "Export job id to mint a link for (from 'download list')")
	cmd.Flags().BoolVar(&flagFiles, "files", false, "Accepted for compatibility: every file the job produced is always listed with a link each")
	cmd.Flags().BoolVar(&flagSkipStatusCheck, "skip-status-check", false, "Mint the link without first confirming the job is ready")
	cmd.Flags().StringVar(&flagPortalURL, "portal-url", portal.DefaultBaseURL, portalURLFlagHelp)
	_ = cmd.Flags().MarkHidden("portal-url")
	return cmd
}

// mintDownloadLinks lists an export job's files and mints a link for each
// from its export data id, the path 'download fetch' uses against the live
// portal.
func mintDownloadLinks(ctx context.Context, sess *portal.Session, jobID string) ([]portal.DownloadFile, error) {
	files, err := sess.DownloadFiles(ctx, jobID)
	if err != nil {
		return nil, err
	}
	out := make([]portal.DownloadFile, 0, len(files))
	for _, f := range files {
		if strings.TrimSpace(f.ExportDataID) == "" {
			return nil, fmt.Errorf("export job %s lists %q with no export data id, so no link can be minted for it", jobID, tenantText(f.FileName))
		}
		link, err := sess.DownloadURL(ctx, f.ExportDataID)
		if err != nil {
			return nil, err
		}
		f.DownloadURL = link
		out = append(out, f)
	}
	return out, nil
}

// downloadLinkOutput is what 'download link' prints.
type downloadLinkOutput struct {
	JobID  string             `json:"job_id"`
	Entity string             `json:"entity,omitempty"`
	Status string             `json:"status,omitempty"`
	Files  []downloadFileInfo `json:"files"`
}

// downloadFileInfo is one link with its expiry.
type downloadFileInfo struct {
	FileName string `json:"file_name,omitempty"`
	// Size is the portal's own human string, for example "871.26 MB".
	Size string `json:"size,omitempty"`
	URL  string `json:"url"`
	// ExpiresAt is empty when the link states no expiry.
	ExpiresAt string `json:"expires_at,omitempty"`
	// ValidFor is the link's stated lifetime, when it states one.
	ValidFor string `json:"valid_for,omitempty"`
	// ExpiryKnown is false when the URL carries no readable expiry.
	ExpiryKnown bool `json:"expiry_known"`
}

func downloadFileOut(f portal.DownloadFile) downloadFileInfo {
	info := downloadFileInfo{FileName: f.FileName, Size: f.Size, URL: f.DownloadURL}
	if exp, ok := portal.LinkExpiry(f.DownloadURL); ok {
		info.ExpiresAt = isoUTC(exp)
		info.ExpiryKnown = true
	}
	if life, ok := portal.LinkLifetime(f.DownloadURL); ok {
		info.ValidFor = life.String()
	}
	return info
}

// printDownloadLink renders the link, its expiry, and the caveat an
// operator needs before pasting it into a ticket.
func printDownloadLink(cmd *cobra.Command, flags *rootFlags, out downloadLinkOutput) error {
	if !wantsHumanTable(cmd.OutOrStdout(), flags) {
		return printJSONFiltered(cmd.OutOrStdout(), out, flags)
	}
	w := cmd.OutOrStdout()
	tenantf(w, "export job %s", out.JobID)
	if out.Entity != "" {
		tenantf(w, " (%s)", out.Entity)
	}
	fmt.Fprintln(w)
	for _, f := range out.Files {
		fmt.Fprintln(w)
		if f.FileName != "" {
			tenantf(w, "  file:    %s", f.FileName)
			if f.Size != "" {
				tenantf(w, "  (%s)", f.Size)
			}
			fmt.Fprintln(w)
		}
		tenantf(w, "  link:    %s\n", f.URL)
		switch {
		case f.ExpiryKnown && f.ValidFor != "":
			tenantf(w, "  expires: %s (valid for %s from when it was minted)\n", f.ExpiresAt, f.ValidFor)
		case f.ExpiryKnown:
			tenantf(w, "  expires: %s\n", f.ExpiresAt)
		default:
			fmt.Fprintln(w, "  expires: not stated in the link; treat it as short lived and confirm before quoting a deadline")
		}
	}
	fmt.Fprintf(w, "\nRun this command again to mint a fresh link once this one expires.\n")
	return nil
}

// watchInterruptedErr is what download watch returns when its context is
// cancelled (Ctrl-C, SIGTERM, a closed terminal). Watching changes nothing,
// so it exits like --once on a job that is not ready yet (ExitJobPending)
// and names the command that picks the watch up again.
func watchInterruptedErr(cmd *cobra.Command, scope restoreScope, job string) error {
	again := []string{"cloudally-cli", "download", "watch", "--account", shellQuote(scope.account)}
	if scope.reseller {
		again = append(again, "--reseller")
	}
	again = append(again, "--job", shellQuote(job))
	for _, name := range []string{"interval", "portal-url"} {
		if f := cmd.Flags().Lookup(name); f != nil && f.Changed {
			again = append(again, "--"+name, shellQuote(f.Value.String()))
		}
	}
	if f := cmd.Flags().Lookup("once"); f != nil && f.Changed && f.Value.String() == "true" {
		again = append(again, "--once")
	}
	return &cliError{code: ExitJobPending, err: fmt.Errorf("interrupted while watching export job %s; nothing was changed. Run this to watch it again: %s",
		tenantText(job), tenantText(strings.Join(again, " ")))}
}

// newDownloadWatchCmd follows an export job to a terminal state.
func newDownloadWatchCmd(flags *rootFlags) *cobra.Command {
	var scope restoreScope
	var flagJob, flagPortalURL string
	var flagInterval time.Duration
	var flagOnce bool

	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Follow a download (export) job until its archive is ready",
		Long: strings.Trim(`
Polls one export job until it reaches a terminal state, so a script can gate
the link step on it. Exits 0 when the job is READY_FOR_DOWNLOAD, 6 when it
finished any other way (any status naming a failure: failed, cancelled,
aborted, stopped, error), and with --once 8 when it is still being built.
--timeout bounds each status check, not the wait: an export can take hours to
build.
`, "\n"),
		Example: strings.Trim(`
  cloudally-cli download watch --account 5f1c2a --job 9f2b1c
  cloudally-cli download watch --account 5f1c2a --job 9f2b1c --once
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if !hasChangedLocalFlags(cmd) && len(args) == 0 {
				return cmd.Help()
			}
			if err := scope.validate(); err != nil {
				return err
			}
			if strings.TrimSpace(flagJob) == "" {
				return usageErr(errors.New("--job <export job id> is required; find it with 'download list'"))
			}
			if flagInterval <= 0 {
				return usageErr(errors.New("--interval must be greater than 0"))
			}
			if scope.account == "" {
				return usageErr(errPortalNeedsAccount)
			}
			if restoreShortCircuit(flags) {
				return writeWould(cmd.OutOrStdout(), flags, "download watch",
					fmt.Sprintf("poll export job %s until it is ready", flagJob), map[string]any{"job": flagJob})
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			// No overall deadline: each status check is bounded per request
			// by the clients, and the job may take hours to build.
			ctx := cmd.Context()
			for {
				job, found, ferr := findExportJob(ctx, c, scope, flagPortalURL, flagJob)
				if ferr != nil {
					if ctx.Err() != nil {
						return watchInterruptedErr(cmd, scope, flagJob)
					}
					return ferr
				}
				if !found {
					return notFoundErr(fmt.Errorf("export job %s is not on account %s", flagJob, scope.account))
				}
				row := downloadJobRowFrom(job)
				if wantsHumanTable(cmd.OutOrStdout(), flags) {
					tenantf(cmd.OutOrStdout(), "%s  %s  %s\n", isoUTC(time.Now()), row.Status, row.Entity)
				}
				if exportJobFinished(job.Status) || flagOnce {
					if !wantsHumanTable(cmd.OutOrStdout(), flags) {
						if perr := printJSONFiltered(cmd.OutOrStdout(), row, flags); perr != nil {
							return perr
						}
					}
					// A terminal failure is reported as one whether or not
					// this was a single check.
					switch {
					case job.Ready():
						if wantsHumanTable(cmd.OutOrStdout(), flags) {
							fmt.Fprintf(cmd.OutOrStdout(), "\nready. Get the link with: cloudally-cli download link --account %s --job %s\n", scope.account, flagJob)
						}
						return nil
					case exportJobFinished(job.Status):
						return partialFailureErr(fmt.Errorf("export job %s finished as %s, not %s", flagJob, tenantText(job.Status), portal.StatusReadyForDownload))
					default:
						return &cliError{code: ExitJobPending, err: fmt.Errorf("export job %s is still %s; check again later", flagJob, tenantText(job.Status))}
					}
				}
				select {
				case <-ctx.Done():
					return watchInterruptedErr(cmd, scope, flagJob)
				case <-time.After(flagInterval):
				}
			}
		},
	}
	addRestoreScopeFlags(cmd, &scope)
	cmd.Flags().StringVar(&flagJob, "job", "", "Export job id to follow (from 'download list')")
	cmd.Flags().DurationVar(&flagInterval, "interval", 30*time.Second, "How long to wait between status checks")
	cmd.Flags().BoolVar(&flagOnce, "once", false, "Check once and exit instead of polling")
	cmd.Flags().StringVar(&flagPortalURL, "portal-url", portal.DefaultBaseURL, portalURLFlagHelp)
	_ = cmd.Flags().MarkHidden("portal-url")
	return cmd
}
