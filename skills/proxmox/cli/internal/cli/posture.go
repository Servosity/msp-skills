// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: fleet backup posture.
// pp:data-source live

package cli

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"proxmox-pp-cli/internal/client"
)

// postureRow is vendor-neutral on purpose: vendor, tenant, asset_id,
// asset_name, asset_type, covered_by_job, last_success_at, age_hours, stale,
// verify_state and size_bytes line up with the staleness rows other backup
// connectors emit, so the outputs can be merged by tenant + asset name.
type postureRow struct {
	Vendor               string   `json:"vendor"`
	Tenant               string   `json:"tenant"`
	Cluster              string   `json:"cluster"`
	AssetID              string   `json:"asset_id"`
	AssetName            string   `json:"asset_name"`
	AssetType            string   `json:"asset_type"`
	Node                 string   `json:"node"`
	VMID                 int64    `json:"vmid"`
	Verdict              string   `json:"verdict"`
	Severity             string   `json:"severity"`
	Reason               string   `json:"reason"`
	CoveredByJob         bool     `json:"covered_by_job"`
	Jobs                 []string `json:"jobs"`
	LastSuccessAt        string   `json:"last_success_at,omitempty"`
	AgeHours             float64  `json:"age_hours,omitempty"`
	Stale                bool     `json:"stale"`
	RestorePoints        int      `json:"restore_points"`
	LastSource           string   `json:"last_source,omitempty"`
	VerifyState          string   `json:"verify_state"`
	LastVerifiedOKAt     string   `json:"last_verified_ok_at,omitempty"`
	SizeBytes            int64    `json:"size_bytes,omitempty"`
	Protected            bool     `json:"protected"`
	Encrypted            bool     `json:"encrypted"`
	LastAttemptStatus    string   `json:"last_attempt_status,omitempty"`
	LastAttemptAt        string   `json:"last_attempt_at,omitempty"`
	OffsiteLastSuccessAt string   `json:"offsite_last_success_at,omitempty"`
}

type postureClientRow struct {
	Tenant             string `json:"tenant"`
	Clusters           int    `json:"clusters"`
	Guests             int    `json:"guests"`
	Red                int    `json:"red"`
	Amber              int    `json:"amber"`
	Fresh              int    `json:"fresh"`
	Stale              int    `json:"stale"`
	Never              int    `json:"never"`
	Uncovered          int    `json:"uncovered"`
	VerifyFailed       int    `json:"verify_failed"`
	Unverified         int    `json:"unverified"`
	LastAttemptFailed  int    `json:"last_attempt_failed"`
	AttemptUnknown     int    `json:"attempt_unknown"`
	ArchivesIncomplete int    `json:"archives_incomplete"`
	ClockSkew          int    `json:"clock_skew"`
	ProtectedElsewhere int    `json:"protected_elsewhere"`
	Unproven           int    `json:"unproven"`
	// FetchFailures counts what was not read on this client's clusters (see
	// fetch_failures): non-zero means every other count may be incomplete.
	FetchFailures int `json:"fetch_failures"`
	// ClustersUnreachable counts this client's clusters that could not be
	// read at all: their guests are missing from every other column.
	ClustersUnreachable int `json:"clusters_unreachable"`
}

type postureReport struct {
	GeneratedAt   string             `json:"generated_at"`
	StaleAfterHrs float64            `json:"stale_after_hours"`
	Clusters      []string           `json:"clusters"`
	Red           int                `json:"red"`
	Amber         int                `json:"amber"`
	Rows          []postureRow       `json:"rows"`
	Clients       []postureClientRow `json:"clients,omitempty"`
	Warnings      []string           `json:"warnings"`
	FetchFailures []fetchFailure     `json:"fetch_failures"`
}

type postureOpts struct {
	staleAfter      time.Duration
	attemptWindow   time.Duration
	elsewhereTag    string
	includeTemplate bool
	maxLogs         int
	now             time.Time
}

type clusterPosture struct {
	rows     []postureRow
	warnings []string
	failures []fetchFailure
}

func newNovelPostureCmd(flags *rootFlags) *cobra.Command {
	var staleAfter, attemptWindow, clientFilter, elsewhereTag, only string
	var byClient, includeTemplates, failOnRed bool
	var maxLogs int
	cmd := &cobra.Command{
		Use:   "posture",
		Short: "One backup verdict per guest across every cluster: enabled-job coverage, newest restore point age, verify state",
		Long: "Use this command for the one-row-per-guest backup verdict across every configured cluster: job coverage, newest restore point age, verify state, last attempt. " +
			"Do NOT use this command to explain why a guest or disk is uncovered; use 'backup coverage' instead. " +
			"Do NOT use it to list one guest's individual restore points; use 'restore-points' instead. " +
			"Do NOT use it for last night's failed tasks and log lines; use 'failures' instead.\n\n" +
			"Verdicts: uncovered (no ENABLED job includes the guest), never (covered, every source that may hold its restore points was read, none exists), " +
			"unproven (covered, no readable restore point or only an old one, while something that may hold one was not proven: a storage or PBS namespace " +
			"not read, a backup listed without a readable manifest, a PBS namespace another configured cluster writes to or may write to, or a backup under its VMID that " +
			"may be an earlier guest's with the same VMID because the VM's config (which records its creation time) could not be read or the backup is dated within an hour before that time; " +
			"see reason and fetch_failures), " +
			"stale (newest restore point older than --stale-after), verify-failed (newest PBS restore point failed verification, including one dated in the future) are red; " +
			"unverified (newest PBS restore point not verified yet), last-attempt-failed (latest vzdump run failed but an older point is still fresh), " +
			"attempt-unknown (fresh restore point, but the latest vzdump log could not be read; raise --max-logs or fix the reported fetch failure), " +
			"archives-incomplete (fresh restore point, but some backup storage could not be read, or a newer backup is listed without a readable manifest), " +
			"and clock-skew (fresh restore point, but another is dated in the future, so a node clock ran ahead; a future-dated point never counts toward age) are amber; " +
			"protected-elsewhere (uncovered but tagged with --protected-elsewhere-tag) and fresh are green. " +
			"Freshness comes from real archives (PVE backup storages, including PBS storages, and the PBS API when configured), never from a job having run.",
		Example: strings.Trim(`
  proxmox-cli posture --agent --stale-after 26h
  proxmox-cli posture --only red --agent --select rows.cluster,rows.asset_name,rows.verdict,rows.age_hours
  proxmox-cli posture --by-client --csv
  proxmox-cli posture --cluster acme --protected-elsewhere-tag veeam`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "posture")
			}
			opts := postureOpts{elsewhereTag: elsewhereTag, includeTemplate: includeTemplates, maxLogs: maxLogs, now: time.Now()}
			var err error
			if opts.staleAfter, err = parseWindow(staleAfter, 26*time.Hour); err != nil {
				return err
			}
			if opts.attemptWindow, err = parseWindow(attemptWindow, 48*time.Hour); err != nil {
				return err
			}
			switch only {
			case "", "red", "amber", "green", "problems":
			default:
				return usageErr(fmt.Errorf("--only must be red, amber, green, or problems (red+amber), got %q", only))
			}
			ctx := cmd.Context() // each request is bounded by --timeout; a fleet run may take longer
			targets, err := fleetTargets(flags, clientFilter)
			if err != nil {
				return err
			}
			markSharedPBS(ctx, flags, targets)
			results, failures := runFleet(ctx, targets, func(ctx context.Context, t fleetTarget) (clusterPosture, error) {
				return collectPosture(ctx, t, opts)
			})
			report := postureReport{
				GeneratedAt: opts.now.UTC().Format(time.RFC3339), StaleAfterHrs: opts.staleAfter.Hours(),
				Rows: make([]postureRow, 0), Warnings: make([]string, 0), FetchFailures: failures,
			}
			for _, t := range targets {
				report.Clusters = append(report.Clusters, t.Name)
			}
			for _, r := range results {
				report.Rows = append(report.Rows, r.Value.rows...)
				report.Warnings = append(report.Warnings, r.Value.warnings...)
				report.FetchFailures = append(report.FetchFailures, r.Value.failures...)
			}
			sortPostureRows(report.Rows)
			for _, r := range report.Rows {
				switch r.Severity {
				case "red":
					report.Red++
				case "amber":
					report.Amber++
				}
			}
			report.Clients = rollupByClient(report.Rows, targets, report.FetchFailures)
			guestsSeen := len(report.Rows)
			if only != "" {
				report.Rows = filterSeverity(report.Rows, only)
			}
			printFetchFailures(cmd.ErrOrStderr(), report.FetchFailures)
			allFailed, outcomeErr := fleetOutcome(cmd.ErrOrStderr(), len(targets), report.FetchFailures)
			for _, w := range report.Warnings {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", w)
			}
			var out any = report
			if byClient {
				out = report.Clients
			} else if flags.csv {
				out = report.Rows
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				if err := printJSONFilteredKeep(cmd.OutOrStdout(), out, flags, postureKeep...); err != nil {
					return err
				}
			} else if !allFailed {
				if only != "" && !byClient && len(report.Rows) == 0 && guestsSeen > 0 {
					// Filtered to nothing is not "no guests": say what was seen.
					fmt.Fprintf(cmd.OutOrStdout(), "No %s guests: %d guest(s) across %d cluster(s) that answered, none matched --only %s.\n", only, guestsSeen, answered(report.Clusters, report.FetchFailures), only)
				} else if err := printPostureTable(cmd, flags, report, byClient); err != nil {
					return err
				}
			}
			if outcomeErr != nil {
				return outcomeErr
			}
			if failOnRed && (report.Red > 0 || len(report.FetchFailures) > 0) {
				// A cluster that could not be read is unverified, which a gate must not pass.
				return &cliError{code: 1, err: fmt.Errorf("%d red guest(s), %d fetch failure(s)", report.Red, len(report.FetchFailures))}
			}
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&staleAfter, "stale-after", "26h", "Restore points older than this are stale (e.g. 26h, 2d, 1w)")
	f.StringVar(&attemptWindow, "attempt-window", "48h", "How far back to read vzdump task logs for the last attempt status")
	f.StringVar(&clientFilter, "client", "", "Only clusters whose client name (or cluster name) matches")
	f.StringVar(&elsewhereTag, "protected-elsewhere-tag", "", "PVE guest tag meaning 'backed up by another product' (e.g. veeam); such uncovered guests are green")
	f.StringVar(&only, "only", "", "Show only rows of this severity: red, amber, green, or problems (red+amber)")
	f.BoolVar(&byClient, "by-client", false, "Emit one summary row per client instead of per guest")
	f.IntVar(&maxLogs, "max-logs", 50, "Maximum vzdump task logs to read per cluster for last-attempt status")
	f.BoolVar(&includeTemplates, "include-templates", false, "Include VM/CT templates (skipped by default)")
	f.BoolVar(&failOnRed, "fail-on-red", false, "Exit 1 when any guest has a red verdict or any cluster could not be fully read (for cron and CI)")
	return cmd
}

func collectPosture(ctx context.Context, t fleetTarget, o postureOpts) (clusterPosture, error) {
	var out clusterPosture
	guests, err := fetchGuests(ctx, t.C)
	if err != nil {
		return out, err
	}
	if len(guests) == 0 {
		if err := requireVisibility(ctx, t.C, false, client.PrivVMAudit); err != nil {
			return out, err
		}
	}
	if err := guestListHidden(ctx, t.C); err != nil {
		out.failures = append(out.failures, fetchFailure{Cluster: t.Name, Scope: "guests", Error: err.Error()})
	}
	cov, err := fetchCoverage(ctx, t.C)
	if err != nil {
		return out, err
	}
	nodes, offline, err := fetchNodes(ctx, t.C)
	if err != nil {
		return out, err
	}
	out.failures = append(out.failures, offlineFailures(t.Name, offline)...)
	if len(guests) == 0 {
		out.warnings = append(out.warnings, fmt.Sprintf("%s: the token sees no guests; privilege-separated tokens need their own ACL (check 'proxmox-cli --cluster %s access permissions')", t.Name, t.Name))
	}

	refs, err := fetchPBSStorageRefs(ctx, t.C)
	if err != nil {
		out.failures = append(out.failures, fetchFailure{Cluster: t.Name, Scope: "storage config", Error: err.Error()})
	}
	refs = t.withShared(refs)
	// Restore points from every backup-content storage (PBS storages carry verification).
	points := map[int64][]restorePoint{}
	// archivesIncomplete: some archive source was not read, so a newer (or
	// failed) restore point may be unseen and "fresh" cannot be asserted.
	storages, readers, sfails, disabled := backupStorages(ctx, t.C, nodes)
	archivesIncomplete := len(sfails) > 0 || len(offline) > 0
	readStorage := map[string]bool{} // storage IDs whose archives were read
	storageUnread := false           // a non-PBS storage that is no job's target here could not be read
	pveOnlyPBS := false              // a PBS storage was read only through PVE (see pveOnlyPBSNote)
	for _, f := range sfails {
		f.Cluster = t.Name
		out.failures = append(out.failures, f)
	}
	for _, key := range sortedKeys(disabled) {
		// It takes no new backups, but may hold older points (and a PBS one
		// verify state).
		out.failures = append(out.failures, disabledFailure(t.Name, key))
		if disabled[key].Type == "pbs" {
			archivesIncomplete = true
		} else {
			storageUnread = true
		}
	}

	// Proxmox VE lists a storage only to a token that may see it: a hidden one
	// may hold this cluster's archives (and verify state), so nothing is green.
	if err := storageListHidden(ctx, t.C); err != nil {
		archivesIncomplete = true
		out.failures = append(out.failures, fetchFailure{Cluster: t.Name, Scope: "backup storage", Error: err.Error()})
	}
	// PBS API (when configured): snapshots this cluster provably owns, plus push
	// sync jobs. Attributed first, so a PBS storage the PBS API reads directly
	// does not depend on the PVE content list (which hides backups from
	// audit-only tokens).
	var syncJobs []pbsSyncJob
	var nsByStore map[string][]string
	pbsHost := t.pbsEndpoint()
	if t.C.PBSConfigured() {
		var warning string
		nsByStore, warning, err = pbsAttribution(ctx, t, refs)
		if err != nil {
			archivesIncomplete = true
			out.failures = append(out.failures, fetchFailure{Cluster: t.Name, Scope: "pbs", Error: err.Error()})
		}
		if warning != "" {
			out.warnings = append(out.warnings, warning)
		}
	}
	standIn := map[string]pbsStorageRef{} // storages the PBS API reads instead of their PVE list
	pbsUnread := map[string]bool{}        // PBS sources (pbsSourceLabel) whose snapshot list failed
	for _, key := range sortedKeys(storages) {
		s := storages[key]
		covered := pbsCovers(refs, s.Storage, pbsHost, nsByStore)
		rps, err := readStorageArchives(ctx, t.C, readers[key], s, 0, refs, covered)
		if merr := t.storageUnattributed(s, refs); merr != nil {
			rps, err = nil, merr
		}
		if err != nil && covered {
			standIn[key] = refs[s.Storage]
			if !errors.Is(err, errCoveredByPBS) {
				out.failures = append(out.failures, fetchFailure{Cluster: t.Name, Scope: "storage " + key, Error: err.Error() + " (its datastore is read through the PBS API instead)"})
			}
			continue
		}
		if err != nil {
			// An unread PBS storage may hide a newer point that failed
			// verification, so nothing on the cluster is green. Any other
			// unread storage carries no verify state and can only add newer
			// points; a guest whose job writes there is caught by its gap.
			if s.Type == "pbs" {
				archivesIncomplete = true
			} else {
				storageUnread = true
			}
			out.failures = append(out.failures, fetchFailure{Cluster: t.Name, Scope: "storage " + key, Error: err.Error()})
			continue
		}
		readStorage[key] = true
		if s.Type == "pbs" && !covered {
			pveOnlyPBS = true
			out.warnings = append(out.warnings, t.Name+": "+pveOnlyPBSNote(key))
		}
		for _, rp := range rps {
			points[rp.VMID] = append(points[rp.VMID], rp)
		}
	}
	if t.C.PBSConfigured() {
		for _, store := range sortedKeys(nsByStore) {
			for _, ns := range nsByStore[store] {
				snaps, err := pbsSnapshots(ctx, t.C, store, ns, nil)
				if err != nil {
					archivesIncomplete = true
					pbsUnread[pbsSourceLabel(store, ns)] = true
					out.failures = append(out.failures, fetchFailure{Cluster: t.Name, Scope: pbsSourceLabel(store, ns), Error: err.Error()})
					continue
				}
				for _, s := range snaps {
					if s.BackupType != "vm" && s.BackupType != "ct" {
						continue
					}
					rp := s.restorePoint(pbsHost, store, ns)
					points[rp.VMID] = append(points[rp.VMID], rp)
				}
			}
		}
		// The default lists only pull jobs; push jobs are the offsite copies.
		if err := getJSON(ctx, t.C, pbsPath("/admin/sync"), map[string]string{"sync-direction": "all"}, &syncJobs); err != nil {
			out.failures = append(out.failures, fetchFailure{Cluster: t.Name, Scope: "pbs sync jobs", Error: err.Error()})
		}
		for _, w := range loadSyncEvidence(ctx, t.C, syncJobs) {
			out.warnings = append(out.warnings, t.Name+": "+w)
		}
	}

	pbsStandIns(standIn, pbsUnread, readStorage)
	targets := newJobTargets(ctx, t.C, cov)
	allGaps := map[storageGap]bool{}
	attempts, blindSince, aborted := lastVzdumpAttempts(ctx, t, nodes, o.now.Add(-o.attemptWindow), o.maxLogs, &out)
	if len(offline) > 0 {
		blindSince = math.MaxInt64 // an offline node's vzdump tasks are unread
	}

	births := newGuestBirths(t, o.now, offline)
	for _, g := range guests {
		if bool(g.Template) && !o.includeTemplate {
			continue
		}
		vmid := int64(g.VMID)
		row := postureRow{
			Vendor: "proxmox", Tenant: t.Tenant, Cluster: t.Name, AssetID: fmt.Sprintf("%s:%d", t.Name, vmid),
			AssetName: g.Name, AssetType: g.Type, Node: g.Node, VMID: vmid,
			Jobs: append([]string{}, cov.EnabledFor[vmid]...), VerifyState: "none",
		}
		row.CoveredByJob = len(row.Jobs) > 0
		// Only this guest's type: a VMID reused by a guest of the other type
		// (CT 100 replaced by VM 100) is a different backup group.
		// A backup from before this VM was created is an earlier guest's that
		// had the same VMID (deleted, its backups retained).
		var birth guestBirth
		if _, tried := attempts[vmid]; tried || len(points[vmid]) > 0 {
			birth = births.read(ctx, g)
		}
		predated := 0
		var rps, unreadable, either []restorePoint
		for _, rp := range dedupeRestorePoints(points[vmid]) {
			if rp.GuestType != "" && rp.GuestType != g.Type {
				continue
			}
			switch birth.of(rp.Time) {
			case ofEarlierGuest:
				predated++
				continue
			case ofEitherGuest:
				either = append(either, rp)
				continue
			}
			if rp.unreadable {
				unreadable = append(unreadable, rp)
			} else {
				rps = append(rps, rp)
			}
		}
		// A point dated past now (the backing-up node's clock ran ahead: vzdump
		// names the archive and PBS takes backup-time from it) has no judgeable
		// age; counting it would keep a guest fresh after backups stop.
		// Its verify result still counts: the newest of them failing is a bad
		// newest backup.
		var future int
		var newestFuture *restorePoint
		judged := rps[:0:0]
		for i, rp := range rps {
			if rp.Time > o.now.Add(clockSkewTolerance).Unix() {
				future++
				if newestFuture == nil || rp.Time > newestFuture.Time {
					newestFuture = &rps[i]
				}
			} else {
				judged = append(judged, rp)
			}
		}
		rps = judged
		row.RestorePoints = len(rps)
		var newest *restorePoint
		for i := range rps {
			if newest == nil || rps[i].Time > newest.Time {
				newest = &rps[i]
			}
			if rps[i].VerifyState == "ok" && (row.LastVerifiedOKAt == "" || unixRFC3339(rps[i].Time) > row.LastVerifiedOKAt) {
				row.LastVerifiedOKAt = unixRFC3339(rps[i].Time)
			}
		}
		if newest != nil {
			row.LastSuccessAt = newest.TimeRFC3339
			row.AgeHours = hoursSince(o.now, newest.Time)
			row.Stale = o.now.Sub(time.Unix(newest.Time, 0)) > o.staleAfter
			row.LastSource = newest.Source
			row.VerifyState = newest.VerifyState
			row.SizeBytes = newest.Size
			row.Protected = newest.Protected
			row.Encrypted = newest.Encrypted
			row.OffsiteLastSuccessAt = offsiteFor(rps, pbsHost, syncJobs)
		}
		a, ok := attempts[vmid]
		attemptOf := ofThisGuest
		if ok {
			attemptOf = birth.of(a.atUnix)
		}
		if attemptOf == ofEarlierGuest {
			a, ok = vzdumpAttempt{}, false // an earlier guest's run
		}
		superseded := supersededByAbort(aborted, vmid, max(a.atUnix, birth.at))
		switch {
		case ok && a.atUnix > blindSince && !superseded && attemptOf == ofThisGuest:
			row.LastAttemptStatus, row.LastAttemptAt = a.status, a.at
		case ok || blindSince > 0 || superseded:
			// An unread task (log cap, failed log or task-list read) may hold a newer attempt.
			row.LastAttemptStatus = "unknown"
		}
		classifyPosture(&row, g, cov, newest, o.elsewhereTag)
		// A snapshot listed without a readable manifest may be a backup still
		// running or a damaged one; one that may be an earlier guest's may be
		// this one's: no restore point proven, none ruled out. An either-guest
		// point is older than every point of this guest, so it can only change
		// a verdict when this guest has none.
		unreadableCounts := row.CoveredByJob && len(unreadable) > 0 && (row.Verdict == "never" || row.Verdict == "stale")
		eitherCounts := row.CoveredByJob && len(either) > 0 && row.Verdict == "never"
		if unreadableCounts || eitherCounts {
			row.Verdict = "unproven"
			var why []string
			if unreadableCounts {
				why = append(why, fmt.Sprintf("%d backup(s) of it are listed without a readable manifest (still running, or damaged)", len(unreadable)))
			}
			if eitherCounts {
				why = append(why, eitherGuestWhy(either, birth))
			}
			row.Reason = strings.Join(why, "; ") + "; the newest restore point proven to be this VM's is " + orNone(row.LastSuccessAt)
		}
		if row.Severity == "green" && newerThan(unreadable, newest) {
			// The newest backup cannot be read: it may be running, or it failed
			// and left debris. The fresh point is real; the latest run is unproven.
			row.Verdict, row.Severity = "archives-incomplete", "amber"
			row.Reason = "newest readable restore point is fresh, but a newer backup is listed without a readable manifest (still running, or damaged)"
		}
		gaps := jobStorageGaps(cov, vmid, g.Node, readStorage, targets)
		for _, g := range gaps {
			allGaps[g] = true
		}
		if (archivesIncomplete || storageUnread || pveOnlyPBS || len(gaps) > 0) && row.CoveredByJob {
			why := "some backup storage could not be read (see fetch_failures)"
			if pveOnlyPBS && !archivesIncomplete && !storageUnread {
				why = "a PBS storage was listed only through the PBS credential saved in it, which may not see every backup group (see warnings)"
			}
			if len(gaps) > 0 {
				why = "the backup target of " + joinGaps(gaps) + " was not read (see fetch_failures)"
			}
			switch {
			case row.Verdict == "never":
				// Something that may hold its restore points (the job's target,
				// a former target, a hidden storage) was not read: no restore
				// point was SEEN, which is not the same as none existing.
				row.Verdict = "unproven"
				row.Reason = "covered by " + strings.Join(row.Jobs, ", ") + ", but " + why + "; no restore point could be read, which does not mean none exists"
			case row.Verdict == "stale":
				row.Verdict = "unproven"
				row.Reason = fmt.Sprintf("newest readable restore point is %.1fh old, but %s; a newer one may be there", row.AgeHours, why)
			case row.Severity == "green" && (archivesIncomplete || len(gaps) > 0):
				row.Verdict, row.Severity = "archives-incomplete", "amber"
				row.Reason = "newest readable restore point is fresh, but " + why + "; a newer or failed archive may be unseen"
			case row.Severity == "red":
				row.Reason += " (" + why + ")"
			}
		}
		if future > 0 {
			note := fmt.Sprintf("%d restore point(s) dated in the future (the backing-up node's clock was ahead) not counted toward its age", future)
			futureFailed := newestFuture.VerifyState == "failed"
			if futureFailed {
				note += "; the newest of them failed verification"
			}
			switch {
			case futureFailed && row.CoveredByJob:
				// The newest backup is known and bad: that is the verdict, whatever
				// the older points say (stale, never, unproven).
				prior := ""
				if row.Severity == "red" && row.Verdict != "never" {
					prior = "; also " + row.Verdict + ": " + row.Reason
				}
				row.Verdict, row.Severity, row.VerifyState = "verify-failed", "red", "failed"
				row.Reason = "the newest restore point failed verification; " + note + prior
			case row.Verdict == "never":
				row.Verdict, row.Reason = "unproven", note+"; no other restore point exists"
			case row.Severity == "green" && row.CoveredByJob:
				row.Verdict, row.Severity = "clock-skew", "amber"
				row.Reason += ", but " + note
			default:
				row.Reason += "; " + note
			}
		}
		if predated > 0 {
			row.Reason += fmt.Sprintf("; %d older backup(s) of VMID %d predate this VM's creation (an earlier guest with the same VMID) and were not counted", predated, vmid)
		}
		if len(either) > 0 && !eitherCounts {
			row.Reason += "; " + eitherGuestWhy(either, birth) + "; they were not counted"
		}
		out.rows = append(out.rows, row)
	}
	out.failures = append(out.failures, gapFailures(t.Name, allGaps)...)
	out.failures = append(out.failures, births.fails...)
	return out, nil
}

// classifyPosture applies the verdict rules in severity order.
func classifyPosture(row *postureRow, g pveGuest, cov *coverage, newest *restorePoint, elsewhereTag string) {
	set := func(verdict, severity, reason string) {
		row.Verdict, row.Severity, row.Reason = verdict, severity, reason
	}
	switch {
	case !row.CoveredByJob && g.hasTag(elsewhereTag):
		set("protected-elsewhere", "green", fmt.Sprintf("no enabled job includes it; tagged %q", elsewhereTag))
	case !row.CoveredByJob:
		reason := "no enabled backup job includes this guest"
		if len(cov.DisabledFor[row.VMID]) > 0 {
			reason = "only included by disabled job(s): " + strings.Join(cov.DisabledFor[row.VMID], ", ")
		}
		set("uncovered", "red", reason)
	case newest == nil:
		set("never", "red", "covered by "+strings.Join(row.Jobs, ", ")+" but no restore point exists")
	case row.Stale:
		set("stale", "red", fmt.Sprintf("newest restore point is %.1fh old", row.AgeHours))
	case newest.VerifyState == "failed":
		set("verify-failed", "red", "newest restore point failed verification")
	case row.LastAttemptStatus == "failed" || row.LastAttemptStatus == "incomplete":
		set("last-attempt-failed", "amber", "latest vzdump run "+row.LastAttemptStatus+"; an older restore point is still fresh")
	case newest.PBSBacked && newest.VerifyState != "ok":
		set("unverified", "amber", "newest restore point has not been verified")
	case row.LastAttemptStatus == "unknown":
		set("attempt-unknown", "amber", "restore point is fresh, but the latest vzdump run could not be read (see fetch_failures)")
	default:
		set("fresh", "green", fmt.Sprintf("restore point %.1fh old", row.AgeHours))
	}
}

type vzdumpAttempt struct {
	status, at string
	atUnix     int64
}

// lastVzdumpAttempts reads vzdump task logs newest first across all nodes (so
// --max-logs drops only the oldest) and keeps each guest's most recent
// per-guest outcome. blindSince is the start time of the newest vzdump task
// whose log was not read (math.MaxInt64 when a node's task list itself could
// not be read): an attempt no newer than that may be superseded by the
// unread one, so the caller reports it as unknown.
func lastVzdumpAttempts(ctx context.Context, t fleetTarget, nodes []string, since time.Time, maxLogs int, out *clusterPosture) (map[int64]vzdumpAttempt, int64, []abortedRun) {
	var blindSince int64
	blind := func(at int64) {
		if at > blindSince {
			blindSince = at
		}
	}
	perms, privErr := fetchPrivs(ctx, t.C, false)
	type nodeTask struct {
		node string
		task pveTask
	}
	var all []nodeTask
	for _, node := range nodes {
		scope := "node " + node + " tasks"
		err := privErr
		if err == nil {
			err = taskListVisibility(perms, false, node)
		}
		var tasks []pveTask
		capped := false
		if err == nil {
			tasks, capped, err = fetchTasks(ctx, t.C, false, node, map[string]string{"typefilter": "vzdump", "since": fmt.Sprint(since.Unix())})
		}
		if err != nil {
			out.failures = append(out.failures, fetchFailure{Cluster: t.Name, Scope: scope, Error: err.Error()})
			blind(math.MaxInt64)
			continue
		}
		if capped && len(tasks) > 0 {
			blind(int64(tasks[len(tasks)-1].StartTime)) // older tasks were not listed
			out.failures = append(out.failures, fetchFailure{Cluster: t.Name, Scope: scope,
				Error: fmt.Sprintf("more than %d vzdump tasks in the attempt window; older ones not listed", taskListPage*taskListMaxPages)})
		}
		for _, task := range tasks {
			if task.EndTime != 0 { // still running: no outcome yet
				all = append(all, nodeTask{node, task})
			}
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].task.StartTime > all[j].task.StartTime })
	latest := map[int64]attemptSeen{}
	var aborted []abortedRun
	unread := 0
	for i, nt := range all {
		if i >= maxLogs {
			unread = len(all) - i
			blind(int64(nt.task.StartTime))
			break
		}
		lines, err := fetchTaskLog(ctx, t.C, false, nt.node, nt.task.UPID)
		if err != nil {
			out.failures = append(out.failures, fetchFailure{Cluster: t.Name, Scope: "task log " + nt.task.UPID, Error: err.Error()})
			blind(int64(nt.task.StartTime))
			continue
		}
		results := parseVzdumpLog(lines)
		for _, r := range results {
			if prev, ok := latest[r.VMID]; !ok || int64(nt.task.StartTime) > prev.at {
				latest[r.VMID] = attemptSeen{at: int64(nt.task.StartTime), res: r.Result}
			}
		}
		// A failed run that processed no guest (global lock, job-start hook) or
		// died part-way (no "Backup job finished" line) has no outcome for the
		// guests it did not reach. Name them when the job's own guest list (or a
		// single-guest task id) says who they are; otherwise older attempts of
		// every guest it did not mention are unknown.
		if !taskOK(nt.task.Status) && (len(results) == 0 || !vzdumpJobFinished(lines)) {
			start := int64(nt.task.StartTime)
			seen := map[int64]bool{}
			for _, r := range results {
				seen[r.VMID] = true
			}
			ids := vzdumpJobVMIDs(lines)
			if ids == nil {
				if vmid, err := strconv.ParseInt(nt.task.ID, 10, 64); err == nil && vmid > 0 {
					ids = []int64{vmid}
				}
			}
			outcome := "incomplete"
			if len(results) == 0 {
				outcome = "failed"
			}
			switch {
			case ids != nil:
				for _, id := range ids {
					if prev, ok := latest[id]; !seen[id] && (!ok || start > prev.at) {
						latest[id] = attemptSeen{at: start, res: outcome}
					}
				}
			case len(results) == 0:
				blind(start)
			default:
				aborted = append(aborted, abortedRun{at: start, seen: seen})
			}
			out.failures = append(out.failures, fetchFailure{Cluster: t.Name, Scope: "task " + nt.task.UPID,
				Error: "vzdump job failed before finishing (" + nt.task.Status + "); guests it did not reach have no outcome from this run"})
		}
	}
	if unread > 0 {
		out.failures = append(out.failures, fetchFailure{Cluster: t.Name, Scope: "vzdump logs",
			Error: fmt.Sprintf("read the newest %d of %d vzdump task logs (--max-logs); guests not seen in them show last_attempt_status unknown", maxLogs, len(all))})
	}
	res := make(map[int64]vzdumpAttempt, len(latest))
	for vmid, s := range latest {
		res[vmid] = vzdumpAttempt{status: s.res, at: unixRFC3339(s.at), atUnix: s.at}
	}
	return res, blindSince, aborted
}

// abortedRun is a --all/--pool vzdump job that died part-way: any guest it did
// not reach, whose last known attempt is older, has an unknown last attempt.
type abortedRun struct {
	at   int64
	seen map[int64]bool
}

func supersededByAbort(runs []abortedRun, vmid, attemptAt int64) bool {
	for _, r := range runs {
		if r.at > attemptAt && !r.seen[vmid] {
			return true
		}
	}
	return false
}

type attemptSeen struct {
	at  int64
	res string
}

// offsiteFor is the end time of the last successful push sync run that
// copied one of these restore points offsite: that run's own log shows it
// pushed that very snapshot, the job's namespace and group filter include the
// archive, its encrypted-only / verified-only / transfer-last restrictions
// admit it, and the run ended after the archive existed. Pull jobs (this PBS
// is the target) are not evidence.
func offsiteFor(rps []restorePoint, pbsHost string, jobs []pbsSyncJob) string {
	if pbsHost == "" {
		return ""
	}
	best := ""
	for _, rp := range rps {
		if !strings.HasPrefix(rp.canonical, "pbs:"+pbsHost+"/") {
			continue // archive is not on the PBS whose sync jobs we read
		}
		typ := map[string]string{"qemu": "vm", "lxc": "ct"}[rp.GuestType]
		for _, j := range jobs {
			end := int64(j.LastRunEnd)
			if !strings.EqualFold(j.LastRunState, "ok") || end < rp.Time || !j.evidenced[syncEvidenceKey(rp.pbsStore, rp.pbsNS, typ+"/"+strconv.FormatInt(rp.VMID, 10), rp.Time)] || !j.admits(rp, rps, end) ||
				!j.covers(rp.pbsStore, rp.pbsNS, typ, strconv.FormatInt(rp.VMID, 10)) {
				continue
			}
			if ts := unixRFC3339(end); ts > best {
				best = ts
			}
		}
	}
	return best
}

// dedupeRestorePoints merges views of the same archive (a PVE storage listing,
// storage aliases, and the PBS API can all show one snapshot). Identity is the
// canonical server/datastore/namespace/guest/time, so copies on different
// datastores or namespaces stay distinct. Verification is reduced over all
// views at once (see reduceVerification), so the result never depends on order.
func dedupeRestorePoints(in []restorePoint) []restorePoint {
	sort.SliceStable(in, func(i, j int) bool {
		return strings.HasPrefix(in[i].Source, "pve-storage:") && !strings.HasPrefix(in[j].Source, "pve-storage:")
	})
	var order []string
	groups := map[string][]restorePoint{}
	for _, rp := range in {
		key := rp.canonical
		if key == "" {
			key = fmt.Sprintf("%s|%s/%d/%d", rp.Source, rp.GuestType, rp.VMID, rp.Time)
		}
		if _, seen := groups[key]; !seen {
			order = append(order, key)
		}
		groups[key] = append(groups[key], rp)
	}
	out := make([]restorePoint, 0, len(order))
	for _, key := range order {
		views := groups[key]
		rp := views[0]
		for _, v := range views[1:] {
			rp.Protected = rp.Protected || v.Protected
			rp.Encrypted = rp.Encrypted || v.Encrypted
			rp.unreadable = rp.unreadable && v.unreadable // one readable view is a manifest
		}
		rp.VerifyState, rp.VerifyUPID = reduceVerification(views)
		out = append(out, rp)
	}
	return out
}

// reduceVerification picks one verification for an archive seen in several
// views: the state of the newest dated verify task. It is "failed" whenever the
// views disagree in a way time cannot settle (an undated state that differs
// from the winner, or different states at the newest time), so a failure is
// never resolved away as verified.
func reduceVerification(views []restorePoint) (string, string) {
	unset := func(state string) bool { return state == "" || state == "none" || state == "n/a" }
	var best *restorePoint
	var bestT int64
	for i := range views {
		if unset(views[i].VerifyState) {
			continue
		}
		if t := upidStartTime(views[i].VerifyUPID); best == nil || t > bestT {
			best, bestT = &views[i], t
		}
	}
	if best == nil {
		return views[0].VerifyState, views[0].VerifyUPID
	}
	for _, v := range views {
		if unset(v.VerifyState) || v.VerifyState == best.VerifyState {
			continue
		}
		if t := upidStartTime(v.VerifyUPID); t == 0 || t == bestT {
			return "failed", best.VerifyUPID
		}
	}
	return best.VerifyState, best.VerifyUPID
}

var severityRank = map[string]int{"red": 0, "amber": 1, "green": 2}

func sortPostureRows(rows []postureRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		if severityRank[rows[i].Severity] != severityRank[rows[j].Severity] {
			return severityRank[rows[i].Severity] < severityRank[rows[j].Severity]
		}
		if rows[i].Cluster != rows[j].Cluster {
			return rows[i].Cluster < rows[j].Cluster
		}
		return rows[i].VMID < rows[j].VMID
	})
}

func filterSeverity(rows []postureRow, only string) []postureRow {
	out := make([]postureRow, 0, len(rows))
	for _, r := range rows {
		if r.Severity == only || (only == "problems" && r.Severity != "green") {
			out = append(out, r)
		}
	}
	return out
}

func rollupByClient(rows []postureRow, targets []fleetTarget, failures []fetchFailure) []postureClientRow {
	unreachable, scoped := tenantFailures(targets, failures)
	byTenant := map[string]*postureClientRow{}
	clusters := map[string]map[string]bool{}
	// Every cluster that answered counts for its client, guests or not: a
	// client whose clusters list no included guest must not vanish.
	down := downClusters(failures)
	for _, t := range targets {
		if down[t.Name] {
			continue
		}
		if byTenant[t.Tenant] == nil {
			byTenant[t.Tenant] = &postureClientRow{Tenant: t.Tenant}
			clusters[t.Tenant] = map[string]bool{}
		}
		clusters[t.Tenant][t.Name] = true
	}
	for _, r := range rows {
		c := byTenant[r.Tenant]
		if c == nil {
			c = &postureClientRow{Tenant: r.Tenant}
			byTenant[r.Tenant] = c
			clusters[r.Tenant] = map[string]bool{}
		}
		clusters[r.Tenant][r.Cluster] = true
		c.Guests++
		switch r.Severity {
		case "red":
			c.Red++
		case "amber":
			c.Amber++
		}
		switch r.Verdict {
		case "fresh":
			c.Fresh++
		case "stale":
			c.Stale++
		case "never":
			c.Never++
		case "uncovered":
			c.Uncovered++
		case "verify-failed":
			c.VerifyFailed++
		case "unverified":
			c.Unverified++
		case "last-attempt-failed":
			c.LastAttemptFailed++
		case "attempt-unknown":
			c.AttemptUnknown++
		case "archives-incomplete":
			c.ArchivesIncomplete++
		case "clock-skew":
			c.ClockSkew++
		case "protected-elsewhere":
			c.ProtectedElsewhere++
		case "unproven":
			c.Unproven++
		}
	}
	for tenant, n := range unreachable {
		if byTenant[tenant] == nil {
			byTenant[tenant] = &postureClientRow{Tenant: tenant}
		}
		byTenant[tenant].ClustersUnreachable = n
	}
	for tenant, n := range scoped {
		if byTenant[tenant] == nil {
			byTenant[tenant] = &postureClientRow{Tenant: tenant}
		}
		byTenant[tenant].FetchFailures = n
	}
	out := make([]postureClientRow, 0, len(byTenant))
	for tenant, c := range byTenant {
		c.Clusters = len(clusters[tenant]) + c.ClustersUnreachable
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool {
		if (out[i].ClustersUnreachable > 0) != (out[j].ClustersUnreachable > 0) {
			return out[i].ClustersUnreachable > 0
		}
		if out[i].Red != out[j].Red {
			return out[i].Red > out[j].Red
		}
		return out[i].Tenant < out[j].Tenant
	})
	return out
}

func printPostureTable(cmd *cobra.Command, flags *rootFlags, rep postureReport, byClient bool) error {
	if byClient {
		rows := make([][]string, 0, len(rep.Clients))
		for _, c := range rep.Clients {
			rows = append(rows, []string{c.Tenant, fmt.Sprint(c.Guests), fmt.Sprint(c.Red), fmt.Sprint(c.Amber), fmt.Sprint(c.Uncovered), fmt.Sprint(c.Stale + c.Never), fmt.Sprint(c.Unproven), fmt.Sprint(c.VerifyFailed), fmt.Sprint(c.ClustersUnreachable), fmt.Sprint(c.FetchFailures)})
		}
		return flags.printTable(cmd, []string{"CLIENT", "GUESTS", "RED", "AMBER", "UNCOVERED", "STALE/NEVER", "UNPROVEN", "VERIFY-FAILED", "UNREACHABLE", "NOT READ"}, rows)
	}
	if len(rep.Rows) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No guests found on the configured clusters.")
		return nil
	}
	rows := make([][]string, 0, len(rep.Rows))
	for _, r := range rep.Rows {
		age := "-"
		if r.LastSuccessAt != "" {
			age = fmt.Sprintf("%.1fh", r.AgeHours)
		}
		rows = append(rows, []string{r.Cluster, fmt.Sprint(r.VMID), r.AssetName, r.Verdict, age, r.VerifyState, r.Reason})
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%d red, %d amber across %d cluster(s) that answered\n", rep.Red, rep.Amber, answered(rep.Clusters, rep.FetchFailures))
	return flags.printTable(cmd, []string{"CLUSTER", "VMID", "NAME", "VERDICT", "AGE", "VERIFY", "REASON"}, rows)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// newerThan reports whether any of rps is newer than newest (nil: any at all).
func newerThan(rps []restorePoint, newest *restorePoint) bool {
	for _, rp := range rps {
		if newest == nil || rp.Time > newest.Time {
			return true
		}
	}
	return false
}

// answered counts the clusters that answered.
func answered(clusters []string, failures []fetchFailure) int {
	return len(clusters) - len(downClusters(failures))
}
