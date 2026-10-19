// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"cloudally-pp-cli/internal/cadate"
	"cloudally-pp-cli/internal/portal"
	"github.com/spf13/cobra"
)

// Layout of the delivery drive: one folder per year, one per mailbox.
//
//	<dest>/<year>/<mailbox>/<archive>.zip
const (
	fetchDirPerm  = 0o755
	fetchFilePerm = 0o644
)

// partialSuffix marks an in-progress download. A file is renamed into place
// only once it is complete and its size checks out, so an interrupted run
// never leaves a short archive looking finished.
const partialSuffix = ".partial"

// freeSpaceMargin is how much room to leave rather than filling a drive to
// the last byte.
const freeSpaceMargin = 2 << 30 // 2 GiB

// unsafeNameChars matches anything not safe in a folder name on Windows or
// exFAT. A mailbox address is otherwise used verbatim so the folders are
// obviously identifiable.
var unsafeNameChars = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)

// safeFolderName makes a mailbox address safe as a folder name without
// making it unrecognisable.
func safeFolderName(s string) string {
	s = strings.TrimSpace(s)
	s = unsafeNameChars.ReplaceAllString(s, "_")
	s = strings.Trim(s, " .")
	if s == "" {
		return "_unnamed"
	}
	return s
}

// jobKey identifies one archive across the ledger and the portal.
func jobKey(entity, subSource string, snapshot int64) string {
	return fmt.Sprintf("%s|%s|%d",
		strings.ToLower(strings.TrimSpace(entity)),
		strings.ToUpper(strings.TrimSpace(subSource)), snapshot)
}

// fetchOutcome is what happened to one archive.
type fetchOutcome struct {
	Entity    string `json:"entity"`
	SubSource string `json:"sub_source"`
	JobID     string `json:"job_id,omitempty"`
	FileName  string `json:"file_name,omitempty"`
	Path      string `json:"path,omitempty"`
	Bytes     int64  `json:"bytes,omitempty"`
	Reported  string `json:"reported_size,omitempty"`
	Status    string `json:"status"`
	Detail    string `json:"detail,omitempty"`
}

// Fetch outcomes.
const (
	fetchDownloaded = "DOWNLOADED"
	fetchSkipped    = "ALREADY_PRESENT"
	fetchEmpty      = "EMPTY"
	fetchNotReady   = "NOT_READY"
	fetchFailed     = "FAILED"
)

func newDownloadFetchCmd(flags *rootFlags) *cobra.Command {
	var scope restoreScope
	var flagDest, flagPortalURL, flagBatch, flagYear, flagOnDate string
	var flagEntities []string
	var flagDryRun bool

	cmd := &cobra.Command{
		Use:   "fetch",
		Short: "Download every ready archive to disk, one folder per year and mailbox",
		Long: strings.Trim(`
Downloads the archives of finished download jobs into

    <dest>/<year>/<mailbox>/<archive>.zip

Each link is minted and streamed in one step, because a link is valid for
only about a minute. Object storage authorises at request time, so a large
archive keeps transferring well past that.

A file is written to a .partial name and renamed only once the whole stream
has arrived, so an interrupted run never leaves a truncated archive looking
complete. Re-running skips archives already present at the expected size, so
it is safe to run repeatedly and to resume after a failure.

Jobs that finished with nothing to download are reported as EMPTY rather
than failed: an empty mailbox is a fact about the mailbox, not an error.
`, "\n"),
		Example: strings.Trim(`
  cloudally-cli download fetch --account 5f1c2a --dest ./archives --year 2022 --on-date 2022-12-31 --dry-run
  cloudally-cli download fetch --account 5f1c2a --dest ./archives --year 2022 --on-date 2022-12-31
`, "\n"),
		Annotations: map[string]string{"pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if !hasChangedLocalFlags(cmd) && len(args) == 0 {
				return cmd.Help()
			}
			if err := scope.validate(); err != nil {
				return err
			}
			if scope.account == "" {
				return usageErr(errPortalNeedsAccount)
			}
			if strings.TrimSpace(flagDest) == "" {
				return usageErr(errors.New("--dest <directory> is required"))
			}
			if strings.TrimSpace(flagYear) == "" {
				return usageErr(errors.New("--year <folder name> is required, for example 2022"))
			}
			if restoreShortCircuit(flags) {
				return writeWould(cmd.OutOrStdout(), flags, "download fetch",
					fmt.Sprintf("download ready archives into %s", filepath.Join(flagDest, flagYear)), nil)
			}
			// The destination must already exist: creating a drive root by
			// accident because a letter was mistyped is worse than failing.
			info, err := os.Stat(flagDest)
			if err != nil {
				return fmt.Errorf("--dest %s: %w", flagDest, err)
			}
			if !info.IsDir() {
				return usageErr(fmt.Errorf("--dest %s is not a directory", flagDest))
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
			jobs, err := sess.DownloadJobs(ctx)
			if err != nil {
				return err
			}
			keep := map[string]bool{}
			for _, e := range flagEntities {
				for _, part := range strings.Split(e, ",") {
					if p := strings.ToLower(strings.TrimSpace(part)); p != "" {
						keep[p] = true
					}
				}
			}

			// A batch ledger names exactly which (mailbox, sub-source,
			// snapshot) belong to this set. That is the only workable
			// filter for the --latest set, where every mailbox has its own
			// date, and it is more precise than a date filter for a year.
			var want map[string]bool
			if key := strings.TrimSpace(flagBatch); key != "" {
				b, berr := loadBatch(key)
				if berr != nil {
					return usageErr(fmt.Errorf("--batch %s: %w", key, berr))
				}
				want = map[string]bool{}
				for _, r := range b.Rows {
					want[jobKey(r.Entity, r.SubSource, r.SnapshotRaw)] = true
				}
				if len(want) == 0 {
					return usageErr(fmt.Errorf("--batch %s has no rows", key))
				}
			}
			// Only archives of the requested backup date belong in this
			// year's folder. Without this, a leftover job from another
			// date would be filed under the wrong year.
			var wantDay string
			if d := strings.TrimSpace(flagOnDate); d != "" {
				t, _, derr := cadate.ParseUserDate(d)
				if derr != nil {
					return usageErr(fmt.Errorf("--on-date %w", derr))
				}
				wantDay = t.UTC().Format("20060102")
			}
			yearDir := filepath.Join(flagDest, safeFolderName(flagYear))
			var outcomes []fetchOutcome
			var total int64
			for _, job := range jobs {
				if len(keep) > 0 && !keep[strings.ToLower(job.Account)] {
					continue
				}
				if want != nil && !want[jobKey(job.Account, job.SubSource, job.SnapshotDate)] {
					continue
				}
				if wantDay != "" {
					if snap := fmt.Sprintf("%d", job.SnapshotDate); len(snap) < 8 || snap[:8] != wantDay {
						continue
					}
				}
				out := fetchOutcome{Entity: job.Account, SubSource: job.SubSource, JobID: job.JobID}
				if !job.Ready() {
					out.Status, out.Detail = fetchNotReady, "job status is "+job.Status
					outcomes = append(outcomes, out)
					continue
				}
				files, ferr := sess.DownloadFiles(ctx, job.JobID)
				if ferr != nil {
					out.Status, out.Detail = fetchFailed, ferr.Error()
					outcomes = append(outcomes, out)
					continue
				}
				if len(files) == 0 {
					out.Status, out.Detail = fetchEmpty, "the job produced no files"
					outcomes = append(outcomes, out)
					continue
				}
				boxDir := filepath.Join(yearDir, safeFolderName(job.Account))
				for _, f := range files {
					fo := fetchOutcome{
						Entity: job.Account, SubSource: job.SubSource, JobID: job.JobID,
						FileName: f.FileName, Reported: f.Size,
					}
					name := safeFolderName(f.FileName)
					if name == "" {
						name = f.ExportDataID + ".zip"
					}
					dest := filepath.Join(boxDir, name)
					fo.Path = dest
					want := portal.ParseSizeBytes(f.Size)

					// Already there at the expected size: leave it alone.
					if st, serr := os.Stat(dest); serr == nil && st.Size() > 0 {
						if want < 0 || closeEnough(st.Size(), want) {
							fo.Status, fo.Bytes = fetchSkipped, st.Size()
							outcomes = append(outcomes, fo)
							continue
						}
					}
					if flagDryRun {
						fo.Status, fo.Detail = "WOULD_DOWNLOAD", "dry run"
						outcomes = append(outcomes, fo)
						continue
					}
					if ferr := ensureSpace(flagDest, want); ferr != nil {
						fo.Status, fo.Detail = fetchFailed, ferr.Error()
						outcomes = append(outcomes, fo)
						fmt.Fprintf(cmd.ErrOrStderr(), "stopping: %v\n", ferr)
						return finishFetch(cmd, flags, outcomes, total, ferr)
					}
					n, derr := streamChunk(ctx, sess, f.ExportDataID, dest)
					fo.Bytes = n
					if derr != nil {
						fo.Status, fo.Detail = fetchFailed, derr.Error()
						outcomes = append(outcomes, fo)
						fmt.Fprintf(cmd.ErrOrStderr(), "failed %s %s: %v\n", job.Account, f.FileName, derr)
						continue
					}
					total += n
					fo.Status = fetchDownloaded
					outcomes = append(outcomes, fo)
					fmt.Fprintf(cmd.ErrOrStderr(), "ok %-38s %-13s %12d bytes  %s\n",
						truncateStr(job.Account, 36), job.SubSource, n, f.FileName)
				}
			}
			return finishFetch(cmd, flags, outcomes, total, nil)
		},
	}
	addRestoreScopeFlags(cmd, &scope)
	cmd.Flags().StringVar(&flagDest, "dest", "", "Existing directory to download into, for example ./archives")
	cmd.Flags().StringVar(&flagYear, "year", "", "Folder name for this set, for example 2022")
	cmd.Flags().StringVar(&flagOnDate, "on-date", "",
		"Only archives of this backup date, as YYYY-MM-DD. Strongly recommended: without it, a job from any other date would be filed under this year")
	cmd.Flags().StringSliceVar(&flagEntities, "entities", nil, "Only these mailboxes")
	cmd.Flags().BoolVar(&flagDryRun, "plan-only", false, "List what would be downloaded without writing anything")
	cmd.Flags().StringVar(&flagBatch, "batch", "",
		"Download only the archives named by this batch ledger. Required for the --latest set, where each mailbox has its own backup date")
	cmd.Flags().StringVar(&flagPortalURL, "portal-url", portal.DefaultBaseURL, portalURLFlagHelp)
	_ = cmd.Flags().MarkHidden("portal-url")
	return cmd
}

// streamChunk downloads one chunk to dest via a .partial file.
func streamChunk(ctx context.Context, sess *portal.Session, exportDataID, dest string) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(dest), fetchDirPerm); err != nil {
		return 0, err
	}
	tmp := dest + partialSuffix
	f, err := os.OpenFile(filepath.Clean(tmp), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fetchFilePerm)
	if err != nil {
		return 0, err
	}
	n, derr := sess.FetchChunk(ctx, exportDataID, f)
	cerr := f.Close()
	if derr != nil {
		_ = os.Remove(tmp)
		return n, derr
	}
	if cerr != nil {
		_ = os.Remove(tmp)
		return n, cerr
	}
	if n == 0 {
		_ = os.Remove(tmp)
		return 0, errors.New("the download produced no bytes")
	}
	if err := os.Rename(tmp, dest); err != nil {
		return n, err
	}
	return n, nil
}

// closeEnough compares a file size with the portal's rounded size string.
// "871.26 MB" is only accurate to a few hundred bytes, so an exact match is
// not available; anything within 1% counts as the same file.
func closeEnough(have, want int64) bool {
	if want <= 0 {
		return false
	}
	diff := have - want
	if diff < 0 {
		diff = -diff
	}
	return diff*100 <= want
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func finishFetch(cmd *cobra.Command, flags *rootFlags, outcomes []fetchOutcome, total int64, fatal error) error {
	if !wantsHumanTable(cmd.OutOrStdout(), flags) {
		if err := printJSONFiltered(cmd.OutOrStdout(), outcomes, flags); err != nil {
			return err
		}
		return fatal
	}
	w := cmd.OutOrStdout()
	counts := map[string]int{}
	for _, o := range outcomes {
		counts[o.Status]++
	}
	fmt.Fprintf(w, "\n%d archive(s) considered, %.2f GiB written this run\n",
		len(outcomes), float64(total)/(1<<30))
	for _, k := range []string{fetchDownloaded, fetchSkipped, fetchEmpty, fetchNotReady, fetchFailed, "WOULD_DOWNLOAD"} {
		if counts[k] > 0 {
			fmt.Fprintf(w, "  %-16s %d\n", k, counts[k])
		}
	}
	return fatal
}
