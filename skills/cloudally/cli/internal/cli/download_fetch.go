// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"cloudally-pp-cli/internal/cadate"
	"cloudally-pp-cli/internal/portal"
	"github.com/spf13/cobra"
)

// Layout of the delivery drive: one folder per year, one per mailbox, and
// one per sub-source, because an MS365 mailbox is one archive per
// sub-source and two of them can carry the same file name.
//
//	<dest>/<year>/<mailbox>/<sub-source>/<archive>
//	<dest>/<year>/<mailbox>/<sub-source>/<archive>.cloudally.json
//
// The .cloudally.json record beside each archive names the export it came
// from and its exact size. It is the only evidence a re-run accepts that a
// file is already the right archive: a matching name is not.
//
// These are customer mailboxes, so folders and files are private to the
// account running the download.
const (
	fetchDirPerm  = 0o700
	fetchFilePerm = 0o600
)

// partialSuffix marks an in-progress download. Each download is written to
// its own freshly created .partial file and moved into place only once it
// is complete and its size checks out, so an interrupted run never leaves a
// short archive looking finished.
const partialSuffix = ".partial"

// recordSuffix names the record written beside each finished archive.
const recordSuffix = ".cloudally.json"

// maxRecordBytes caps how much of a record is read back.
const maxRecordBytes = 64 << 10

// freeSpaceMargin is how much room to leave rather than filling a drive to
// the last byte.
const freeSpaceMargin = 2 << 30 // 2 GiB

// maxNameBytes is the longest file or folder name most filesystems take.
// An archive name leaves room for its record's suffix.
const (
	maxNameBytes    = 255
	maxArchiveBytes = maxNameBytes - len(recordSuffix)
	maxIDBytes      = 96
)

// unsafeNameChars matches anything not safe in a file or folder name on
// Windows or exFAT, or not safe to show: C0 and C1 controls, DEL, and the
// bidirectional controls that can make "‮ppa.exe.zip" display as
// "piz.exe.app". A mailbox address is otherwise used verbatim so the
// folders are obviously identifiable.
var unsafeNameChars = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f\x7f-\x9f\x{061c}\x{200e}\x{200f}\x{202a}-\x{202e}\x{2066}-\x{2069}]`)

// windowsReservedNames are device names Windows refuses as a file or
// folder name, with or without an extension.
var windowsReservedNames = func() map[string]bool {
	m := map[string]bool{"CON": true, "PRN": true, "AUX": true, "NUL": true, "CONIN$": true, "CONOUT$": true}
	for _, d := range []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "¹", "²", "³"} {
		m["COM"+d], m["LPT"+d] = true, true
	}
	return m
}()

// isWindowsReserved reports whether Windows would treat name as a device:
// the part before the first dot, ignoring trailing spaces, is reserved.
func isWindowsReserved(name string) bool {
	base := name
	if i := strings.IndexByte(base, '.'); i >= 0 {
		base = base[:i]
	}
	return windowsReservedNames[strings.ToUpper(strings.TrimRight(base, " "))]
}

// sanitizeName makes s safe as one file or folder name on Windows, macOS,
// Linux and exFAT without making it unrecognisable, and no longer than
// limit bytes. ok is false when nothing usable is left (an empty name, dots
// only, or nothing but replaced characters), so the caller can substitute
// a name of its own instead of a shared placeholder.
func sanitizeName(s string, limit int) (string, bool) {
	s = strings.ToValidUTF8(s, "_")
	s = strings.TrimSpace(s)
	s = unsafeNameChars.ReplaceAllString(s, "_")
	// Shorten before judging the result: cutting a long name can leave
	// "CON" or nothing at all.
	s = strings.Trim(fitName(strings.Trim(s, " ."), limit), " .")
	if s == "" || strings.Trim(s, "_") == "" {
		return "", false
	}
	if isWindowsReserved(s) {
		s = fitName("_"+s, limit)
	}
	return s, true
}

// safeFolderName makes a mailbox address or year safe as a folder name,
// using a placeholder when nothing usable is left.
func safeFolderName(s string) string {
	if name, ok := sanitizeName(s, maxNameBytes); ok {
		return name
	}
	return "_unnamed"
}

// nameExt returns a name's extension when it is a plausible one.
func nameExt(name string) string {
	ext := filepath.Ext(name)
	if ext == name || len(ext) > 16 {
		return ""
	}
	return ext
}

// fitName shortens a name to limit bytes, keeping its extension and never
// splitting a UTF-8 sequence.
func fitName(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	ext := nameExt(s)
	stem := s[:len(s)-len(ext)]
	n := limit - len(ext)
	for n > 0 && !utf8.RuneStart(stem[n]) {
		n--
	}
	return strings.TrimRight(stem[:n], " .") + ext
}

// archiveName picks the file name for one archive: the portal's own name
// when it survives sanitising, otherwise one built from the export data id.
// A name ending in the record suffix is not used, so an archive can never
// sit where another archive's record belongs.
func archiveName(f portal.DownloadFile) (string, error) {
	if name, ok := sanitizeName(f.FileName, maxArchiveBytes); ok && !strings.HasSuffix(strings.ToLower(name), recordSuffix) {
		return name, nil
	}
	id, ok := sanitizeName(f.ExportDataID, maxIDBytes)
	if !ok {
		return "", errors.New("the portal listed this file with no usable name and no export data id")
	}
	return id + ".zip", nil
}

// altArchiveName is the name used when the first choice already holds a
// different archive. The export data id goes before the extension, which
// makes the name unique to this archive.
func altArchiveName(name, exportDataID string) (string, error) {
	id, ok := sanitizeName(exportDataID, maxIDBytes)
	if !ok {
		return "", errors.New("no export data id to tell this archive apart")
	}
	ext := nameExt(name)
	stem := fitName(strings.TrimSuffix(name, ext), maxArchiveBytes-len(ext)-len(id)-1)
	return stem + "." + id + ext, nil
}

// jobKey identifies one archive across the ledger and the portal.
func jobKey(entity, subSource string, snapshot int64) string {
	return fmt.Sprintf("%s|%s|%d",
		strings.ToLower(strings.TrimSpace(entity)),
		strings.ToUpper(strings.TrimSpace(subSource)), snapshot)
}

// fetchOutcome is what happened to one archive.
type fetchOutcome struct {
	Entity       string `json:"entity"`
	SubSource    string `json:"sub_source"`
	JobID        string `json:"job_id,omitempty"`
	ExportDataID string `json:"export_data_id,omitempty"`
	FileName     string `json:"file_name,omitempty"`
	Path         string `json:"path,omitempty"`
	Bytes        int64  `json:"bytes,omitempty"`
	Reported     string `json:"reported_size,omitempty"`
	Status       string `json:"status"`
	Detail       string `json:"detail,omitempty"`
}

// Fetch outcomes.
const (
	fetchDownloaded = "DOWNLOADED"
	fetchSkipped    = "ALREADY_PRESENT"
	fetchEmpty      = "EMPTY"
	fetchNotReady   = "NOT_READY"
	fetchFailed     = "FAILED"
	// fetchMissing is an archive the run was asked for that has no job on
	// the portal: a --batch row nothing matched, or an --entities mailbox
	// with no download job.
	fetchMissing = "MISSING"
	// fetchUnlisted is an archive asked for whose job is not in a job
	// listing the portal cut short: it may exist beyond the cut, so it is
	// neither MISSING nor expired, and nothing is created for it.
	fetchUnlisted = "UNLISTED"
)

// unlistedDetail explains an UNLISTED outcome.
const unlistedDetail = "its job is not among the jobs the portal listed before the listing's page cap, so it may still exist; nothing is concluded and nothing should be created for it"

// heldArchive is where a --batch row another batch holds really stands:
// the holding batch's ledger row for the same archive. row is nil when
// that ledger cannot be read (err) or has no such row.
type heldArchive struct {
	key    string
	ledger *downloadBatch
	row    *downloadRow
	err    error
}

// resolveHeld follows each held row of b to its holder's ledger row.
func resolveHeld(b *downloadBatch) map[int]heldArchive {
	out := map[int]heldArchive{}
	ledgers := map[string]heldArchive{}
	for i, r := range b.Rows {
		if r.Status != dlSkipped {
			continue
		}
		key := strings.TrimSpace(r.HeldBy)
		h, ok := ledgers[key]
		if !ok {
			h.key = key
			if key == "" {
				h.err = errors.New("the ledger does not name the batch that holds it")
			} else {
				h.ledger, h.err = loadBatch(key)
			}
			ledgers[key] = h
		}
		if h.ledger != nil {
			for j := range h.ledger.Rows {
				hr := &h.ledger.Rows[j]
				if strings.EqualFold(hr.Entity, r.Entity) && strings.EqualFold(hr.SubSource, r.SubSource) && hr.SnapshotRaw == r.SnapshotRaw && hr.Status != dlSkipped {
					h.row = hr
					break
				}
			}
			if h.row == nil {
				h.err = fmt.Errorf("batch %s has no row for this archive", key)
			}
		}
		out[i] = h
	}
	return out
}

// heldOutcome reports a --batch row another batch holds, from the holder's
// own ledger row. Its confirmed job was in the run's job list (want), so
// when it was seen there the job loop already reported it.
func heldOutcome(r downloadRow, h heldArchive, matched map[string]bool, current func(string) string, account string, listingCut bool) (fetchOutcome, bool) {
	out := fetchOutcome{Entity: r.Entity, SubSource: r.SubSource}
	resume := fmt.Sprintf("cloudally-cli download submit --batch %s --resume --yes", h.key)
	if h.row == nil {
		out.Status = fetchMissing
		out.Detail = fmt.Sprintf("batch %s holds this archive, but its ledger cannot tell where it stands (%v)", h.key, h.err)
		return out, true
	}
	hr := *h.row
	out.JobID = hr.JobID
	switch {
	case hr.Status == dlEmpty:
		out.Status, out.Detail = fetchEmpty, "batch "+h.key+" holds this archive; its job finished with nothing to download"
	case hr.Status == dlCreated && hr.JobID != "":
		if matched[current(hr.JobID)] {
			return out, false
		}
		if listingCut {
			out.Status, out.Detail = fetchUnlisted, "held by batch "+h.key+": "+unlistedDetail
			return out, true
		}
		out.Status = fetchMissing
		out.Detail = "held by batch " + h.key + ": " + expiredArchiveAdvice(h.ledger, hr, account)
	case hr.Status == dlJobFailed || hr.Status == dlFailed:
		out.Status = fetchFailed
		out.Detail = fmt.Sprintf("batch %s holds this archive and its create %s (ledger status %s); create it once more with: %s", h.key, map[bool]string{true: "job failed", false: "was refused"}[hr.Status == dlJobFailed], hr.Status, resume)
	case hr.Status == dlUnknown && hr.Ambiguous:
		out.Status = fetchMissing
		out.Detail = fmt.Sprintf("batch %s holds this archive, but a job of this snapshot cannot be proved to be its; collect every archive of that snapshot with: cloudally-cli download fetch --account %s --dest <folder> --year <year> --on-date %s --entities %s", h.key, account, snapshotDay(r.SnapshotRaw), r.Entity)
	case hr.Status == dlMissing || hr.Status == dlPlanned:
		out.Status = fetchMissing
		what := map[bool]string{true: "its create never appeared on the portal", false: "its create was never sent"}[hr.Status == dlMissing]
		out.Detail = fmt.Sprintf("batch %s holds this archive but %s (ledger status %s); create it with: %s", h.key, what, hr.Status, resume)
	case (hr.Status == dlRequested || hr.Status == dlUnknown) && stillSettling(hr):
		out.Status = fetchNotReady
		out.Detail = fmt.Sprintf("batch %s holds this archive and has not confirmed its job yet (ledger status %s); confirm it with: %s, then run this again", h.key, hr.Status, resume)
	default:
		out.Status = fetchMissing
		out.Detail = fmt.Sprintf("batch %s holds this archive and has not confirmed its job for over an hour (ledger status %s); settle it with: %s", h.key, hr.Status, resume)
	}
	return out, true
}

// stillSettling reports whether a REQUESTED or UNKNOWN row's create is
// recent enough (staleRequestedAge) that its job may still appear. Past
// that, waiting changes nothing: only a resume of its batch settles it,
// so fetch reports it MISSING (exit 6) rather than NOT_READY (exit 8,
// "run again later"), which a script would poll forever.
func stillSettling(r downloadRow) bool {
	at := r.AcceptedAt
	if at == "" {
		at = r.RequestedAt
	}
	t, err := time.Parse(time.RFC3339, at)
	return err == nil && time.Since(t) < staleRequestedAge
}

// snapshotOnDay reports whether a compact snapshot falls on day
// (yyyyMMdd) in the account's timezone, offset from UTC the way 'download
// submit --utc-offset' reads it: the UTC instant plus the offset.
func snapshotOnDay(raw int64, day string, offset time.Duration) bool {
	if offset == 0 {
		snap := fmt.Sprintf("%d", raw)
		return len(snap) >= 8 && snap[:8] == day
	}
	t, err := cadate.ParseCompact(raw)
	if err != nil {
		return false
	}
	return t.UTC().Add(offset).Format("20060102") == day
}

// expiredArchiveAdvice explains a confirmed archive whose job the portal no
// longer lists. A resume never re-creates a confirmed archive, so a new
// export of it is a person's decision, made after looking at the job list.
func expiredArchiveAdvice(b *downloadBatch, r downloadRow, account string) string {
	msg := fmt.Sprintf("the batch's confirmed job %s is no longer listed on the portal: it finished with nothing to download, or the portal expired the archive. A resume does not create it again. Check 'cloudally-cli download list --account %s'", r.JobID, account)
	if b == nil {
		return msg
	}
	cmds := submitCommandsFor(b, []downloadRow{r})
	if len(cmds) == 0 {
		return msg
	}
	cmd := strings.TrimSuffix(cmds[0], " --yes") + " --new-batch --yes"
	return msg + "; if the archive is still needed, a person can decide to create a new export of it with: " + cmd
}

// failedJobAdvice marks a job that finished without an archive as
// JOB_FAILED in the download guard and in the ledger of the batch that
// confirmed it, and says how that batch creates it once more. A job no
// batch confirmed (one made in the portal, say) is only reported.
func failedJobAdvice(ctx context.Context, guard *downloadGuard, job portal.DownloadJob) string {
	base := fmt.Sprintf("the export job %s finished as %s and has no archive to download", job.JobID, job.Status)
	if guard == nil || !exportJobFailed(job.Status) {
		return base
	}
	c, err := guard.jobFailed(context.WithoutCancel(ctx), job.JobID)
	if err != nil || c == nil || c.BatchKey == "" {
		return base
	}
	markLedgerJobFailed(c.BatchKey, job.JobID, job.Status)
	return fmt.Sprintf("%s; batch %s creates it once more with: cloudally-cli download submit --batch %s --resume --yes", base, c.BatchKey, c.BatchKey)
}

// markLedgerJobFailed records a failed job on the row of key's ledger that
// confirmed it. When another run holds the batch, the guard alone records
// it; that run's next resume finds the failure itself.
func markLedgerJobFailed(key, jobID, status string) {
	_ = withBatchLock(key, func() error {
		b, err := loadBatch(key)
		if err != nil {
			return err
		}
		for i := range b.Rows {
			r := &b.Rows[i]
			if r.Status == dlCreated && strings.EqualFold(strings.TrimSpace(r.JobID), strings.TrimSpace(jobID)) {
				markJobFailed(r, status)
				return saveBatch(b)
			}
		}
		return nil
	})
}

// stalePartialAge is how old a leftover .partial must be before a run
// removes it. A live download writes to its .partial continuously and is
// ended after portal.DownloadStallTimeout of silence, so an hour-old one
// belongs to a run that was killed.
const stalePartialAge = time.Hour

// partialName matches the temporary files this tool creates, and nothing
// else, so the sweep never touches a file it did not make.
var partialName = regexp.MustCompile(`^\.cloudally-[0-9a-f]{16}\.partial$`)

// sweepStalePartials removes this tool's own .partial files older than
// stalePartialAge from one folder it writes to.
func sweepStalePartials(root *os.Root, dirRel string, now time.Time) {
	dir, err := root.Open(dirRel)
	if err != nil {
		return
	}
	names, err := dir.Readdirnames(-1)
	_ = dir.Close()
	if err != nil {
		return
	}
	for _, name := range names {
		if !partialName.MatchString(name) {
			continue
		}
		rel := filepath.Join(dirRel, name)
		fi, err := root.Lstat(rel)
		if err != nil || !fi.Mode().IsRegular() || now.Sub(fi.ModTime()) < stalePartialAge {
			continue
		}
		_ = root.Remove(rel)
	}
}

// archiveRecord is the .cloudally.json written beside a finished archive.
type archiveRecord struct {
	ExportDataID string `json:"export_data_id"`
	JobID        string `json:"job_id,omitempty"`
	Entity       string `json:"entity,omitempty"`
	SubSource    string `json:"sub_source,omitempty"`
	SnapshotRaw  int64  `json:"snapshot_raw,omitempty"`
	FileName     string `json:"file_name,omitempty"`
	ReportedSize string `json:"reported_size,omitempty"`
	Bytes        int64  `json:"bytes"`
	DownloadedAt string `json:"downloaded_at"`
}

func newDownloadFetchCmd(flags *rootFlags) *cobra.Command {
	var scope restoreScope
	var flagDest, flagPortalURL, flagBatch, flagYear, flagOnDate string
	var flagUTCOffset float64
	var flagEntities []string
	var flagDryRun bool

	cmd := &cobra.Command{
		Use:   "fetch",
		Short: "Download every ready archive to disk, one folder per year and mailbox",
		Long: strings.Trim(`
Downloads the archives of finished download jobs into

    <dest>/<year>/<mailbox>/<sub-source>/<archive>.zip

with a small <archive>.zip.cloudally.json record beside each one naming the
export it came from and its size. Folders and files are created readable by
the current user only on macOS and Linux; on Windows they inherit the
permissions of --dest, so choose a private folder.

Each link is minted and streamed in one step, because a link is valid for
only about a minute. Object storage authorises at request time, so a large
archive keeps transferring well past that.

A file is written to a fresh .partial name and moved into place only once
the whole stream has arrived and matches the size the portal reported, so an
interrupted run never leaves a truncated archive looking complete. An
existing file is never overwritten. Re-running skips an archive only when
its record shows it is the same export at the same size, so it is safe to
run repeatedly and to resume after a failure. If a different file already
holds an archive's name, the archive is saved with its export id added to
the name instead.

Jobs that finished with nothing to download are reported as EMPTY rather
than failed: an empty mailbox is a fact about the mailbox, not an error.

--timeout bounds each API and portal call, not the run: an archive streams
for as long as bytes keep arriving, and is ended only after five minutes
with nothing received. Leftover .partial files from a run that was killed
are removed once they are an hour old.

With --batch, only the jobs the batch confirmed as its own are downloaded, and
only with the --account the batch was recorded for (another is refused, exit
2); another export of the same snapshot is never taken in their place on a
guess. A job that ended in failure (any status naming a failure: FAILED,
CANCELLED, ABORTED, STOPPED, ERROR) is handed back to the batch that confirmed
it, and that batch's resume creates it once more; when another batch has
already created that archive again (--new-batch, say), the new job is
downloaded in its place once the download guard shows it is confirmed for
exactly that archive, and is NOT_READY until then. Under --batch every archive
asked for ends with an outcome; without --batch, a failed job that was created
again is skipped as history. A row another batch holds (a pilot run's mailbox,
say) is collected through that batch: its confirmed job, matched by job id, is
downloaded here. When that batch's job failed or its create was refused the
row is FAILED; when its create never appeared on the portal, was never sent,
or has gone unconfirmed for over an hour, or when that batch's ledger cannot
be read, it is MISSING; it is NOT_READY only while that batch's create is
under an hour old and unconfirmed, or its job is still being built. Each names
that batch's resume command. The batch's own unconfirmed rows follow the same
hour: NOT_READY while under an hour old, then MISSING.

The portal lists jobs 100 to a page. If the listing stops at its cap of 50
pages while more may exist, an archive whose job is not among those read is
UNLISTED: it may still exist, so it is never reported MISSING or expired and
nothing is suggested to create it.

Exit codes: 0 every archive asked for is on disk (or the job was EMPTY); 3 no
download job matched at all; 6 an archive FAILED, is UNLISTED, or is MISSING
(a --batch row or an --entities mailbox with no job on the portal), or the
drive is too full to continue; 8 nothing failed but some jobs are still being
built or not yet confirmed (NOT_READY): run the same command again later.
Without --batch, 0 means only that every job found was saved, not that a batch
is complete.
`, "\n"),
		Example: strings.Trim(`
  cloudally-cli download fetch --account 5f1c2a --dest ./archives --year 2025 --batch 20261015-201500-ab12 --plan-only
  cloudally-cli download fetch --account 5f1c2a --dest ./archives --year 2025 --batch 20261015-201500-ab12
  cloudally-cli download fetch --account 5f1c2a --dest ./archives --year 2025 --on-date 2025-12-31 --utc-offset -5
`, "\n"),
		Annotations: map[string]string{"mcp:hidden": "true", "pp:data-source": "live"},
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
			// Every write below goes through this root, so nothing under
			// --dest can redirect a write outside it.
			root, err := os.OpenRoot(flagDest)
			if err != nil {
				return fmt.Errorf("--dest %s: %w", flagDest, err)
			}
			defer root.Close()

			c, err := flags.newClient()
			if err != nil {
				return err
			}
			// No overall deadline: --timeout bounds each API and portal call
			// (the clients apply it per request), while an archive streams
			// until it stalls (portal.DownloadStallTimeout).
			ctx := cmd.Context()
			sess, err := openPortalSession(ctx, c, scope, flagPortalURL)
			if err != nil {
				return err
			}
			jobs, listErr := sess.DownloadJobs(ctx)
			// A listing cut at its page cap still proves every job in it;
			// it proves nothing about a job missing from it (listingCut).
			listingCut := errors.Is(listErr, portal.ErrListingIncomplete)
			if listErr != nil && !listingCut {
				return listErr
			}
			if listingCut {
				tenantf(cmd.ErrOrStderr(), "warning: %v; an archive not among the %d job(s) read is reported %s, never as missing or expired\n", listErr, len(jobs), fetchUnlisted)
			}
			// The guard says which batch confirmed a job, so a job that
			// failed is handed back to that batch to create once more.
			var guard *downloadGuard
			if !flagDryRun {
				if guard, err = openDownloadGuard(ctx); err != nil {
					return err
				}
				defer guard.Close()
			}
			keep := map[string]bool{}
			for _, e := range flagEntities {
				for _, part := range strings.Split(e, ",") {
					if p := strings.ToLower(strings.TrimSpace(part)); p != "" {
						keep[p] = true
					}
				}
			}

			// A batch ledger names exactly which jobs belong to this set, by
			// the job ids it confirmed. That is the only workable filter
			// for the --latest set, where every mailbox has its own date,
			// and it is more precise than a date filter for a year.
			var want map[string]bool
			var batchRows []downloadRow
			var batchLedger *downloadBatch
			var held map[int]heldArchive
			if key := strings.TrimSpace(flagBatch); key != "" {
				b, berr := loadBatch(key)
				if berr != nil {
					return usageErr(fmt.Errorf("--batch: %w", berr))
				}
				if err := checkBatchScope(cmd, scope, b); err != nil {
					return err
				}
				// Only a job the batch confirmed as its own is its archive.
				// Another export of the same snapshot (a zip without
				// legal-hold items, say) never stands in for it.
				want = map[string]bool{}
				for _, r := range b.Rows {
					if id := strings.ToLower(strings.TrimSpace(r.JobID)); id != "" && r.Status == dlCreated {
						want[id] = true
					}
				}
				// A row another batch holds is collected through that
				// batch's confirmed job, matched by its id, never a guess.
				held = resolveHeld(b)
				for _, h := range held {
					if h.row != nil && h.row.Status == dlCreated && h.row.JobID != "" {
						want[strings.ToLower(strings.TrimSpace(h.row.JobID))] = true
					}
				}
				batchRows, batchLedger = b.Rows, b
				if len(b.Rows) == 0 {
					return usageErr(fmt.Errorf("--batch %s has no rows", key))
				}
			}
			// Only archives of the requested backup date belong in this
			// year's folder. Without this, a leftover job from another
			// date would be filed under the wrong year.
			var wantDay string
			offset := time.Duration(flagUTCOffset * float64(time.Hour))
			if d := strings.TrimSpace(flagOnDate); d != "" {
				t, _, derr := cadate.ParseUserDate(d)
				if derr != nil {
					return usageErr(fmt.Errorf("--on-date %w", derr))
				}
				wantDay = t.UTC().Format("20060102")
			}
			year := safeFolderName(flagYear)
			stderr := cmd.ErrOrStderr()
			var outcomes []fetchOutcome
			var total int64
			matched := map[string]bool{}
			seenEntity := map[string]bool{}
			swept := map[string]bool{}
			// Under --batch every archive asked for ends with an outcome. A
			// job of the batch (or of a batch holding one of its rows) that
			// ended without an archive is followed to its replacement only
			// when the guard proves one: the current claim of exactly the
			// same archive. A confirmed replacement is downloaded in its
			// place; one not yet confirmed is NOT_READY, naming the batch
			// creating it; with none, the failure is reported.
			replaced := map[string]string{}
			pending := map[string]*downloadClaim{}
			if want != nil && guard != nil {
				for _, job := range jobs {
					id := strings.ToLower(strings.TrimSpace(job.JobID))
					if !want[id] || job.Ready() || !exportJobFinished(job.Status) {
						continue
					}
					cur, rerr := guard.replacement(ctx, job.JobID)
					switch {
					case rerr != nil || cur == nil:
					case cur.JobID != "":
						r := strings.ToLower(strings.TrimSpace(cur.JobID))
						replaced[id], want[r] = r, true
					default:
						pending[id] = cur
					}
				}
			}
			for _, job := range jobs {
				if len(keep) > 0 && !keep[strings.ToLower(job.Account)] {
					continue
				}
				id := strings.ToLower(strings.TrimSpace(job.JobID))
				if want != nil && !want[id] {
					continue
				}
				if wantDay != "" {
					if !snapshotOnDay(job.SnapshotDate, wantDay, offset) {
						continue
					}
				}
				if !job.Ready() && exportJobFinished(job.Status) {
					// Without --batch, a failed job its batch has already
					// created again (or that --new-batch replaced) is
					// history, not a failure of this run. It is not counted
					// as seen either.
					if want == nil && guard != nil {
						if gone, rerr := guard.jobReplaced(ctx, job.JobID); rerr == nil && gone {
							continue
						}
					}
					// Under --batch its replacement is reported instead.
					if _, ok := replaced[id]; ok {
						continue
					}
				}
				matched[id] = true
				seenEntity[strings.ToLower(strings.TrimSpace(job.Account))] = true
				out := fetchOutcome{Entity: job.Account, SubSource: job.SubSource, JobID: job.JobID}
				if !job.Ready() {
					// A job that has stopped without becoming ready never
					// will; only a running one is worth waiting for.
					if exportJobFinished(job.Status) {
						if cur, ok := pending[id]; ok {
							out.Status = fetchNotReady
							out.Detail = fmt.Sprintf("job %s finished as %s with no archive; batch %s is creating this archive again and has not confirmed it yet; confirm it with: cloudally-cli download submit --batch %s --resume --yes, then run this again",
								job.JobID, job.Status, cur.BatchKey, cur.BatchKey)
						} else {
							out.Status, out.Detail = fetchFailed, failedJobAdvice(ctx, guard, job)
						}
					} else {
						out.Status, out.Detail = fetchNotReady, "job status is "+job.Status
					}
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
				sub := job.SubSource
				if strings.TrimSpace(sub) == "" {
					sub = job.Source
				}
				dirs := []string{year, safeFolderName(job.Account), safeFolderName(sub)}
				dirRel := filepath.Join(dirs...)
				for _, f := range files {
					fo := fetchOutcome{
						Entity: job.Account, SubSource: job.SubSource, JobID: job.JobID,
						ExportDataID: f.ExportDataID, FileName: f.FileName, Reported: f.Size,
					}
					fail := func(detail string) {
						fo.Status, fo.Detail = fetchFailed, detail
						outcomes = append(outcomes, fo)
						tenantf(stderr, "failed %s %s: %s\n", job.Account, f.FileName, detail)
					}
					if strings.TrimSpace(f.ExportDataID) == "" {
						fail("the portal listed this file with no export data id, so no link can be minted for it")
						continue
					}
					name, nerr := archiveName(f)
					if nerr != nil {
						fail(nerr.Error())
						continue
					}
					// A plan never creates folders; a real run creates them
					// private, one at a time, refusing any that is a link.
					dirsExist, derr := ensureFetchDirs(root, !flagDryRun, dirs...)
					if derr != nil {
						fail(derr.Error())
						continue
					}
					rel, state, have := filepath.Join(dirRel, name), slotFree, int64(0)
					if dirsExist {
						var perr error
						rel, state, have, perr = placeArchive(root, dirRel, name, f.ExportDataID)
						if perr != nil {
							fail(perr.Error())
							continue
						}
					}
					fo.Path = filepath.Join(flagDest, rel)
					switch state {
					case slotOurs:
						fo.Status, fo.Bytes = fetchSkipped, have
						outcomes = append(outcomes, fo)
						continue
					case slotMismatch:
						fail(fmt.Sprintf("an earlier download of this archive is at this path but is %d bytes, not the size recorded beside it; it was left untouched: move it aside and run again", have))
						continue
					case slotTaken:
						fail("both this archive's name and its export-id name are taken by other files; they were left untouched: move them aside and run again")
						continue
					}
					if flagDryRun {
						fo.Status, fo.Detail = "WOULD_DOWNLOAD", "dry run"
						outcomes = append(outcomes, fo)
						continue
					}
					if !swept[dirRel] {
						swept[dirRel] = true
						sweepStalePartials(root, dirRel, time.Now())
					}
					hint, _ := portal.ParseSizeHint(f.Size)
					if serr := ensureSpace(flagDest, hint.Bytes); serr != nil {
						fo.Status, fo.Detail = fetchFailed, serr.Error()
						outcomes = append(outcomes, fo)
						tenantf(stderr, "stopping: %s\n", serr)
						return finishFetch(cmd, flags, outcomes, total, partialFailureErr(serr))
					}
					n, derr2 := streamChunk(ctx, sess, root, rel, f.ExportDataID, hint)
					fo.Bytes = n
					if derr2 != nil {
						fail(derr2.Error())
						continue
					}
					total += n
					fo.Status = fetchDownloaded
					rec := archiveRecord{
						ExportDataID: f.ExportDataID, JobID: job.JobID, Entity: job.Account,
						SubSource: job.SubSource, SnapshotRaw: job.SnapshotDate, FileName: f.FileName,
						ReportedSize: f.Size, Bytes: n, DownloadedAt: isoUTC(time.Now()),
					}
					if werr := writeArchiveRecord(root, rel+recordSuffix, rec); werr != nil {
						// The archive itself is complete and in place. Without
						// its record a re-run cannot prove that, and saves a
						// second copy under the export-id name instead.
						fo.Detail = "the archive is complete, but its record could not be written: " + werr.Error()
						tenantf(stderr, "warning: %s %s: %s\n", job.Account, f.FileName, fo.Detail)
					}
					outcomes = append(outcomes, fo)
					tenantf(stderr, "ok %-38s %-13s %12d bytes  %s\n",
						truncateStr(job.Account, 36), job.SubSource, n, f.FileName)
				}
			}
			// What was asked for but never seen on the portal is reported,
			// so a run that collected only part of a set says so.
			batchKey := strings.TrimSpace(flagBatch)
			// A row's job is the one that now holds its archive.
			current := func(jobID string) string {
				id := strings.ToLower(strings.TrimSpace(jobID))
				if r, ok := replaced[id]; ok {
					return r
				}
				return id
			}
			for i, r := range batchRows {
				id := current(r.JobID)
				if (r.Status == dlCreated && matched[id]) || (len(keep) > 0 && !keep[strings.ToLower(r.Entity)]) {
					continue
				}
				if wantDay != "" && !snapshotOnDay(r.SnapshotRaw, wantDay, offset) {
					continue
				}
				if h, ok := held[i]; ok {
					if out, report := heldOutcome(r, h, matched, current, scope.account, listingCut); report {
						outcomes = append(outcomes, out)
					}
					continue
				}
				out := fetchOutcome{Entity: r.Entity, SubSource: r.SubSource, JobID: r.JobID}
				switch {
				case r.Status == dlEmpty:
					// Every row asked for gets an outcome, so a batch of
					// empty mailboxes is not "no download jobs matched".
					out.Status, out.Detail = fetchEmpty, "the job finished with nothing to download"
				case r.Status == dlJobFailed:
					out.Status = fetchFailed
					out.Detail = fmt.Sprintf("its job %s finished as %s with no archive; create it once more with: cloudally-cli download submit --batch %s --resume --yes", r.JobID, r.JobStatus, batchKey)
				case r.Status == dlUnknown && r.Ambiguous:
					out.Status = fetchMissing
					out.Detail = fmt.Sprintf("a job of this mailbox, sub-source and snapshot is listed but cannot be proved to be this batch's; collect every archive of that snapshot with: cloudally-cli download fetch --account %s --dest <folder> --year <year> --on-date %s --entities %s", scope.account, snapshotDay(r.SnapshotRaw), r.Entity)
				case r.Status == dlCreated && listingCut:
					out.Status, out.Detail = fetchUnlisted, unlistedDetail
				case r.Status == dlCreated:
					// --resume never re-creates a confirmed archive, so the
					// way back, if one is wanted, is a person's decision.
					out.Status = fetchMissing
					out.Detail = expiredArchiveAdvice(batchLedger, r, scope.account)
				case (r.Status == dlRequested || r.Status == dlUnknown) && !stillSettling(r):
					out.Status = fetchMissing
					out.Detail = fmt.Sprintf("the batch has not confirmed this job for over an hour (ledger status %s); settle it with: cloudally-cli download submit --batch %s --resume --yes", r.Status, batchKey)
				case r.Status == dlRequested || r.Status == dlUnknown:
					// Not confirmed yet. Confirming sends nothing that may
					// already exist.
					out.Status = fetchNotReady
					out.Detail = fmt.Sprintf("the batch has not confirmed this job yet (ledger status %s); confirm it with: cloudally-cli download submit --batch %s --resume --yes, then run this again", r.Status, batchKey)
				default:
					out.Status = fetchMissing
					out.Detail = fmt.Sprintf("this archive was never created (ledger status %s); create it with: cloudally-cli download submit --batch %s --resume --yes", r.Status, batchKey)
				}
				outcomes = append(outcomes, out)
			}
			for _, e := range sortedKeys(keep) {
				if !seenEntity[e] && batchRows == nil {
					if listingCut {
						outcomes = append(outcomes, fetchOutcome{Entity: e, Status: fetchUnlisted, Detail: unlistedDetail})
						continue
					}
					outcomes = append(outcomes, fetchOutcome{Entity: e, Status: fetchMissing, Detail: "no download job on the portal for this mailbox matches this run's filters"})
				}
			}
			if len(outcomes) == 0 && listingCut {
				return partialFailureErr(fmt.Errorf("no download job matched among the %d the portal listed, but the listing was cut short (%v), so a matching job may exist beyond it", len(jobs), listErr))
			}
			return finishFetch(cmd, flags, outcomes, total, nil)
		},
	}
	addRestoreScopeFlags(cmd, &scope)
	cmd.Flags().StringVar(&flagDest, "dest", "", "Existing directory to download into, for example ./archives")
	cmd.Flags().StringVar(&flagYear, "year", "", "Folder name for this set, for example 2022")
	cmd.Flags().StringVar(&flagOnDate, "on-date", "",
		"Only archives of this backup date, as YYYY-MM-DD (the UTC day unless --utc-offset is given). Collect a 'download submit' set with --batch instead: without it an archive that was never created is not reported")
	cmd.Flags().Float64Var(&flagUTCOffset, "utc-offset", 0,
		"The account's offset from UTC in hours, as given to 'download submit --utc-offset'. --on-date is then that calendar day in the account's own timezone; 0 reads it as the UTC day")
	cmd.Flags().StringSliceVar(&flagEntities, "entities", nil, "Only these mailboxes")
	cmd.Flags().BoolVar(&flagDryRun, "plan-only", false, "List what would be downloaded without writing anything")
	cmd.Flags().StringVar(&flagBatch, "batch", "",
		"Download only the archives this batch ledger confirmed as its own. Required for the --latest set, where each mailbox has its own backup date")
	cmd.Flags().StringVar(&flagPortalURL, "portal-url", portal.DefaultBaseURL, portalURLFlagHelp)
	_ = cmd.Flags().MarkHidden("portal-url")
	return cmd
}

// ensureFetchDirs walks the folders the layout needs under --dest, one
// component at a time. Each must be a plain folder: a symbolic link, a
// junction or a file in the way is refused rather than written through.
// --dest itself may be a link, since the operator chose it. With create
// set, a missing folder is made private; without it, the walk stops at the
// first missing one and reports false.
func ensureFetchDirs(root *os.Root, create bool, comps ...string) (bool, error) {
	rel := ""
	for _, c := range comps {
		rel = filepath.Join(rel, c)
		fi, err := root.Lstat(rel)
		if errors.Is(err, fs.ErrNotExist) {
			if !create {
				return false, nil
			}
			if merr := root.Mkdir(rel, fetchDirPerm); merr != nil && !errors.Is(merr, fs.ErrExist) {
				return false, merr
			}
			fi, err = root.Lstat(rel)
		}
		if err != nil {
			return false, err
		}
		if fi.Mode()&(fs.ModeSymlink|fs.ModeIrregular) != 0 || !fi.IsDir() {
			return false, fmt.Errorf("%s under --dest is not a plain folder (a link, junction or file is in the way); refusing to write through it", rel)
		}
	}
	return true, nil
}

// slotState is what a candidate path for an archive currently holds.
type slotState int

const (
	// slotFree: nothing is there.
	slotFree slotState = iota
	// slotOurs: this archive, complete, with a record that says so.
	slotOurs
	// slotMismatch: a record for this archive, but the file does not match it.
	slotMismatch
	// slotTaken: something else.
	slotTaken
)

// inspectSlot reports what rel holds, judged against exportDataID.
func inspectSlot(root *os.Root, rel, exportDataID string) (slotState, int64, error) {
	fi, err := root.Lstat(rel)
	if errors.Is(err, fs.ErrNotExist) {
		return slotFree, 0, nil
	}
	if err != nil {
		return slotTaken, 0, err
	}
	if !fi.Mode().IsRegular() {
		return slotTaken, 0, nil
	}
	rec, ok := readArchiveRecord(root, rel+recordSuffix)
	if !ok || rec.ExportDataID != exportDataID {
		return slotTaken, fi.Size(), nil
	}
	if fi.Size() <= 0 || fi.Size() != rec.Bytes {
		return slotMismatch, fi.Size(), nil
	}
	return slotOurs, fi.Size(), nil
}

// placeArchive picks where one archive goes: its own name when that is free
// or already holds it, otherwise the export-id name. An archive saved under
// its export-id name on an earlier run is found there even after the file
// that pushed it aside is gone, so it is not downloaded a second time.
func placeArchive(root *os.Root, dirRel, name, exportDataID string) (string, slotState, int64, error) {
	primary := filepath.Join(dirRel, name)
	state, have, err := inspectSlot(root, primary, exportDataID)
	if err != nil || state == slotOurs || state == slotMismatch {
		return primary, state, have, err
	}
	alt, aerr := altArchiveName(name, exportDataID)
	if aerr != nil {
		return primary, state, have, nil
	}
	altRel := filepath.Join(dirRel, alt)
	altState, altHave, err := inspectSlot(root, altRel, exportDataID)
	if err != nil {
		return altRel, altState, altHave, err
	}
	if state == slotFree && altState != slotOurs && altState != slotMismatch {
		return primary, slotFree, 0, nil
	}
	return altRel, altState, altHave, nil
}

// readArchiveRecord reads the record beside an archive. A record that is
// missing, not a plain file, oversized or unreadable is no evidence at all.
func readArchiveRecord(root *os.Root, rel string) (archiveRecord, bool) {
	fi, err := root.Lstat(rel)
	if err != nil || !fi.Mode().IsRegular() || fi.Size() > maxRecordBytes {
		return archiveRecord{}, false
	}
	f, err := root.Open(rel)
	if err != nil {
		return archiveRecord{}, false
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxRecordBytes))
	if err != nil {
		return archiveRecord{}, false
	}
	var rec archiveRecord
	if json.Unmarshal(data, &rec) != nil || strings.TrimSpace(rec.ExportDataID) == "" {
		return archiveRecord{}, false
	}
	return rec, true
}

// writeArchiveRecord writes a record beside a finished archive, through a
// fresh temporary file so a reader never sees half of one. It replaces only
// an older record, never some other file that happens to have the name.
func writeArchiveRecord(root *os.Root, rel string, rec archiveRecord) error {
	if _, err := root.Lstat(rel); err == nil {
		if _, ok := readArchiveRecord(root, rel); !ok {
			return errors.New("a file that is not a download record is in the way; it was left untouched")
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	tmp, f, err := createPartial(root, filepath.Dir(rel))
	if err != nil {
		return err
	}
	_, werr := f.Write(append(data, '\n'))
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		_ = root.Remove(tmp)
		return werr
	}
	if err := root.Rename(tmp, rel); err != nil {
		_ = root.Remove(tmp)
		return err
	}
	return nil
}

// createPartial creates a new, private, uniquely named .partial file in dir.
// The name is random and the create is exclusive, so a file or link planted
// in advance can never be opened in its place.
func createPartial(root *os.Root, dir string) (string, *os.File, error) {
	for range 8 {
		var b [8]byte
		if _, err := rand.Read(b[:]); err != nil {
			return "", nil, err
		}
		rel := filepath.Join(dir, ".cloudally-"+hex.EncodeToString(b[:])+partialSuffix)
		f, err := root.OpenFile(rel, os.O_RDWR|os.O_CREATE|os.O_EXCL, fetchFilePerm)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		return rel, f, err
	}
	return "", nil, errors.New("could not pick an unused temporary file name")
}

// publishNoReplace moves a finished download to its final name and never
// replaces a file it did not create. In order of preference:
//
//   - a hard link, which fails when the name is taken;
//   - the operating system's no-replace rename, for drives without hard
//     links (exFAT, FAT32, some network shares);
//   - where neither exists (exFAT on macOS has neither), claiming the final
//     name with an exclusive create and renaming the download over that
//     claim. Two fetches can never both claim one name, so neither can
//     replace the other's archive.
func publishNoReplace(root *os.Root, tmp, final string) error {
	lerr := root.Link(tmp, final)
	if lerr == nil {
		_ = root.Remove(tmp)
		return nil
	}
	if errors.Is(lerr, fs.ErrExist) {
		return errArchiveNameTaken
	}
	dirRel := filepath.Dir(final)
	if filepath.Dir(tmp) != dirRel {
		return fmt.Errorf("internal: temporary file %s is not beside %s", tmp, final)
	}
	dir, err := root.Open(dirRel)
	if err != nil {
		return err
	}
	rerr := renameNoReplace(root, dir, dirRel, filepath.Base(tmp), filepath.Base(final))
	_ = dir.Close()
	switch {
	case rerr == nil:
		return nil
	case errors.Is(rerr, fs.ErrExist):
		return errArchiveNameTaken
	case !errors.Is(rerr, errors.ErrUnsupported):
		return rerr
	}
	return claimAndRename(root, tmp, final)
}

// claimAndRename claims final with an exclusive create, which every
// filesystem supports, then renames tmp over that claim. Only the process
// that won the claim renames onto the name, so nothing it did not create is
// ever replaced.
func claimAndRename(root *os.Root, tmp, final string) error {
	claim, err := root.OpenFile(final, os.O_WRONLY|os.O_CREATE|os.O_EXCL, fetchFilePerm)
	if errors.Is(err, fs.ErrExist) {
		return errArchiveNameTaken
	}
	if err != nil {
		return err
	}
	_ = claim.Close()
	if err := root.Rename(tmp, final); err != nil {
		_ = root.Remove(final)
		return err
	}
	return nil
}

// errArchiveNameTaken is a final name that filled while its archive was
// downloading.
var errArchiveNameTaken = errors.New("a file appeared at the archive's name while it was downloading; it was left untouched")

// streamChunk downloads one chunk to rel under root via a fresh .partial
// file. Nothing reaches rel unless the whole stream arrived and matched the
// reported size.
func streamChunk(ctx context.Context, sess *portal.Session, root *os.Root, rel, exportDataID string, hint portal.SizeHint) (int64, error) {
	tmp, f, err := createPartial(root, filepath.Dir(rel))
	if err != nil {
		return 0, err
	}
	n, derr := sess.FetchChunk(ctx, exportDataID, f, hint)
	cerr := f.Close()
	if derr == nil {
		derr = cerr
	}
	if derr == nil && n == 0 {
		derr = errors.New("the download produced no bytes")
	}
	if derr == nil {
		derr = publishNoReplace(root, tmp, rel)
	}
	if derr != nil {
		_ = root.Remove(tmp)
		return n, derr
	}
	return n, nil
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func finishFetch(cmd *cobra.Command, flags *rootFlags, outcomes []fetchOutcome, total int64, fatal error) error {
	counts := map[string]int{}
	for _, o := range outcomes {
		counts[o.Status]++
	}
	if !wantsHumanTable(cmd.OutOrStdout(), flags) {
		if err := printJSONFiltered(cmd.OutOrStdout(), outcomes, flags); err != nil {
			return err
		}
	} else {
		w := cmd.OutOrStdout()
		fmt.Fprintf(w, "\n%d archive(s) considered, %.2f GiB written this run\n",
			len(outcomes), float64(total)/(1<<30))
		for _, k := range []string{fetchDownloaded, fetchSkipped, fetchEmpty, fetchNotReady, fetchFailed, fetchMissing, fetchUnlisted, "WOULD_DOWNLOAD"} {
			if counts[k] > 0 {
				fmt.Fprintf(w, "  %-16s %d\n", k, counts[k])
			}
		}
	}
	if fatal != nil {
		return fatal
	}
	if len(outcomes) == 0 {
		return notFoundErr(errors.New("no download jobs matched: check --account, --on-date and --entities, or list the jobs with 'download list'; a job created minutes ago may not be listed yet"))
	}
	if n := counts[fetchFailed] + counts[fetchMissing] + counts[fetchUnlisted]; n > 0 {
		return partialFailureErr(fmt.Errorf("%d of %d archive(s) are not on disk (%d FAILED, %d MISSING, %d UNLISTED); nothing was overwritten, so the same command can be run again once the cause is fixed",
			n, len(outcomes), counts[fetchFailed], counts[fetchMissing], counts[fetchUnlisted]))
	}
	if n := counts[fetchNotReady]; n > 0 {
		return &cliError{code: ExitJobPending, err: fmt.Errorf("%d archive(s) are still being built (NOT_READY); run the same command again later to collect them", n)}
	}
	return nil
}
