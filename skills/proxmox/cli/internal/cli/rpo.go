// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: RPO evidence over a past window.
// pp:data-source live

package cli

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"proxmox-pp-cli/internal/client"
)

type rpoRow struct {
	Tenant          string   `json:"tenant"`
	Cluster         string   `json:"cluster"`
	VMID            int64    `json:"vmid"`
	Name            string   `json:"name"`
	Jobs            []string `json:"jobs"`
	WindowStart     string   `json:"window_start"`
	WindowCapped    bool     `json:"window_capped"`
	CappedAt        string   `json:"capped_at,omitempty"`
	RestorePoints   int      `json:"restore_points"`
	ScheduledRuns   int      `json:"scheduled_runs"`
	MissedRuns      int      `json:"missed_runs"`
	MissedRunTimes  []string `json:"missed_run_times"`
	LargestGapHours float64  `json:"largest_gap_hours"`
	VerifiedOKDays  int      `json:"verified_ok_days"`
	MetSchedule     bool     `json:"met_schedule"`
	// Evidence is "full" when the whole window was checked; "partial" when
	// retention or a failed schedule expansion leaves part of it unproven.
	Evidence string `json:"evidence"`
	// PointsUnread: a storage that may hold this guest's restore points (its
	// job's target, a former target, a hidden storage) could not be read, or
	// some are dated in the future (a node clock ran ahead), so the missed
	// runs and the restore-point count are unproven, not measured.
	PointsUnread bool `json:"points_unread,omitempty"`
	// MissedRunsUnproven: missed runs retention may have thinned (counted in
	// missed_runs too).
	MissedRunsUnproven int `json:"missed_runs_unproven,omitempty"`
	// MayBeEarlierGuest: backups under the VMID that may be an earlier
	// guest's (see restore-points), not counted in restore_points.
	MayBeEarlierGuest int    `json:"may_be_earlier_guest,omitempty"`
	Note              string `json:"note,omitempty"`
}

type rpoClientRow struct {
	Tenant           string  `json:"tenant"`
	Guests           int     `json:"guests"`
	GuestsMet        int     `json:"guests_met_schedule"`
	MissedRuns       int     `json:"missed_runs"`
	WorstGapHours    float64 `json:"worst_gap_hours"`
	VerifiedOKDays   int     `json:"verified_ok_days"`
	GuestsWithNoData int     `json:"guests_without_restore_points"`
	GuestsPartial    int     `json:"guests_partial_evidence"`
	// Guests whose job target was not read are counted here instead of in
	// guests_without_restore_points / missed_runs: their points went unseen.
	GuestsUnproven      int `json:"guests_unproven"`
	MissedRunsUnproven  int `json:"missed_runs_unproven"`
	ClustersUnreachable int `json:"clusters_unreachable"`
	FetchFailures       int `json:"fetch_failures"`
}

type rpoReport struct {
	Days          int            `json:"days"`
	WindowStart   string         `json:"window_start"`
	Clusters      []string       `json:"clusters"`
	Rows          []rpoRow       `json:"rows"`
	Clients       []rpoClientRow `json:"clients"`
	FetchFailures []fetchFailure `json:"fetch_failures"`
}

// rpoInProgressGrace keeps a run that started moments ago from counting as missed.
const rpoInProgressGrace = 6 * time.Hour

func newNovelRpoCmd(flags *rootFlags) *cobra.Command {
	var days int
	var clientFilter, guest string
	var byClient bool
	cmd := &cobra.Command{
		Use:   "rpo",
		Short: "Per guest over a past window: largest restore-point gap, scheduled runs with no restore point, verified days",
		Long: "Use this command for restore-point gaps and missed scheduled runs per guest over a past window (SLA and compliance evidence). " +
			"Do NOT use this command for the current age or verify verdict; use 'posture' instead. " +
			"Do NOT use it to list one guest's individual restore points; use 'restore-points' instead.\n\n" +
			"Each enabled job's schedule is expanded server-side (cluster jobs schedule-analyze) into the past runs inside the window. " +
			"A run is missed when no restore point exists between it and the next scheduled run. Retention prunes old points, so when a " +
			"guest's oldest retained point is newer than the window start, earlier runs are not counted (window_capped) rather than " +
			"reported as missed. Runs scheduled before a VM was created are not counted either. Jobs that run at the same time each need " +
			"their own restore point. The newest run is not judged while it may still be running (6h), but the schedule is not met until it " +
			"has a restore point. Each job's runs are judged against restore points on that job's own destination, and one backup pays " +
			"one run. When retention may delete older backups (any covering job's prune-backups policy, from the job, its storage or " +
			"vzdump.conf with remove on, a PBS prune job on the datastore, or one that cannot be read), a missed run older than a " +
			"restore point that was kept may have been pruned, and one with a restore point elsewhere inside its window (a job moved " +
			"storages) may be that point: both count in missed_runs_unproven and evidence is partial. may_be_earlier_guest counts backups under " +
			"the VMID that may be an earlier guest's with the same VMID (see restore-points); they are not counted in restore_points.",
		Example: strings.Trim(`
  proxmox-cli rpo --days 7 --agent
  proxmox-cli rpo --days 30 --by-client --csv
  proxmox-cli rpo --guest 101 --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "rpo")
			}
			if days < 1 || days > 366 {
				return usageErr(errors.New("--days must be between 1 and 366"))
			}
			var guestID int64
			if guest != "" {
				id, err := strconv.ParseInt(guest, 10, 64)
				if err != nil || id <= 0 {
					return usageErr(fmt.Errorf("--guest %q must be a vmid", guest))
				}
				guestID = id
			}
			ctx := cmd.Context() // each request is bounded by --timeout; a fleet run may take longer
			targets, err := fleetTargets(flags, clientFilter)
			if err != nil {
				return err
			}
			markSharedPBS(ctx, flags, targets)
			now := time.Now()
			start := now.Add(-time.Duration(days) * 24 * time.Hour)
			type result struct {
				rows  []rpoRow
				fails []fetchFailure
			}
			results, failures := runFleet(ctx, targets, func(ctx context.Context, t fleetTarget) (result, error) {
				var r result
				var err error
				r.rows, r.fails, err = collectRPO(ctx, t, start, now, guestID)
				return r, err
			})
			rep := rpoReport{Days: days, WindowStart: start.UTC().Format(time.RFC3339), Rows: make([]rpoRow, 0), FetchFailures: failures}
			for _, t := range targets {
				rep.Clusters = append(rep.Clusters, t.Name)
			}
			for _, r := range results {
				rep.Rows = append(rep.Rows, r.Value.rows...)
				rep.FetchFailures = append(rep.FetchFailures, r.Value.fails...)
			}
			sort.SliceStable(rep.Rows, func(i, j int) bool {
				if rep.Rows[i].MissedRuns != rep.Rows[j].MissedRuns {
					return rep.Rows[i].MissedRuns > rep.Rows[j].MissedRuns
				}
				return rep.Rows[i].LargestGapHours > rep.Rows[j].LargestGapHours
			})
			rep.Clients = rollupRPO(rep.Rows, targets, rep.FetchFailures)
			printFetchFailures(cmd.ErrOrStderr(), rep.FetchFailures)
			allFailed, outcomeErr := fleetOutcome(cmd.ErrOrStderr(), len(targets), rep.FetchFailures)
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				var out any = rep
				switch {
				case byClient:
					out = rep.Clients
				case flags.csv:
					out = rep.Rows
				}
				if err := printJSONFilteredKeep(cmd.OutOrStdout(), out, flags, rpoKeep...); err != nil {
					return err
				}
				return outcomeErr
			}
			if allFailed {
				return outcomeErr
			}
			if byClient {
				rows := make([][]string, 0, len(rep.Clients))
				for _, c := range rep.Clients {
					rows = append(rows, []string{c.Tenant, fmt.Sprint(c.Guests), fmt.Sprint(c.GuestsMet), fmt.Sprint(c.MissedRuns), fmt.Sprint(c.GuestsUnproven), fmt.Sprint(c.MissedRunsUnproven), fmt.Sprintf("%.1f", c.WorstGapHours), fmt.Sprint(c.ClustersUnreachable), fmt.Sprint(c.FetchFailures)})
				}
				return flags.printTable(cmd, []string{"CLIENT", "GUESTS", "MET SCHEDULE", "MISSED RUNS", "UNPROVEN", "MISSED (UNPROVEN)", "WORST GAP (H)", "UNREACHABLE", "NOT READ"}, rows)
			}
			if len(rep.Rows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No guests covered by an enabled backup job.")
				return nil
			}
			rows := make([][]string, 0, len(rep.Rows))
			partial := 0
			for _, r := range rep.Rows {
				if r.Evidence == "partial" {
					partial++
				}
				rows = append(rows, []string{r.Cluster, fmt.Sprint(r.VMID), r.Name, fmt.Sprint(r.ScheduledRuns), fmt.Sprint(r.MissedRuns), fmt.Sprintf("%.1f", r.LargestGapHours), fmt.Sprint(r.VerifiedOKDays), strconv.FormatBool(r.WindowCapped), r.Evidence, r.Note})
			}
			if partial > 0 {
				// Misses on a partial row may be restore points that were not read.
				fmt.Fprintf(cmd.OutOrStdout(), "%d guest(s) with partial evidence: some of their missed runs or met schedules are unproven (see NOTE and --json missed_runs_unproven)\n", partial)
			}
			return flags.printTable(cmd, []string{"CLUSTER", "VMID", "NAME", "RUNS", "MISSED", "LARGEST GAP (H)", "VERIFIED DAYS", "CAPPED", "EVIDENCE", "NOTE"}, rows)
		},
	}
	f := cmd.Flags()
	f.IntVar(&days, "days", 7, "Window length in days")
	f.StringVar(&guest, "guest", "", "Only this vmid")
	f.StringVar(&clientFilter, "client", "", "Only clusters whose client name (or cluster name) matches")
	f.BoolVar(&byClient, "by-client", false, "Emit one rollup row per client")
	return cmd
}

func collectRPO(ctx context.Context, t fleetTarget, start, now time.Time, onlyVMID int64) ([]rpoRow, []fetchFailure, error) {
	var fails []fetchFailure
	guests, err := fetchGuests(ctx, t.C)
	if err != nil {
		return nil, nil, err
	}
	if len(guests) == 0 {
		if err := requireVisibility(ctx, t.C, false, client.PrivVMAudit); err != nil {
			return nil, nil, err
		}
	}
	if err := guestListHidden(ctx, t.C); err != nil {
		fails = append(fails, fetchFailure{Cluster: t.Name, Scope: "guests", Error: err.Error()})
	}
	cov, err := fetchCoverage(ctx, t.C)
	if err != nil {
		return nil, nil, err
	}
	nodes, offline, err := fetchNodes(ctx, t.C)
	if err != nil {
		return nil, nil, err
	}
	fails = append(fails, offlineFailures(t.Name, offline)...)
	points := map[int64][]restorePoint{}
	refs, refsErr := fetchPBSStorageRefs(ctx, t.C)
	refs = t.withShared(refs)
	if refsErr != nil {
		fails = append(fails, fetchFailure{Cluster: t.Name, Scope: "storage config", Error: refsErr.Error()})
	}
	storages, readers, sfails, disabled := backupStorages(ctx, t.C, nodes)
	// A storage or snapshot list that could not be read may hold the "missed"
	// runs, so a miss is then unproven, not a fact.
	pointsIncomplete := len(sfails) > 0 || len(offline) > 0 || len(disabled) > 0
	readStorage := map[string]bool{}
	for _, f := range sfails {
		f.Cluster = t.Name
		fails = append(fails, f)
	}
	for _, key := range sortedKeys(disabled) {
		fails = append(fails, disabledFailure(t.Name, key))
	}

	if err := storageListHidden(ctx, t.C); err != nil {
		pointsIncomplete = true
		fails = append(fails, fetchFailure{Cluster: t.Name, Scope: "backup storage", Error: err.Error()})
	}
	// PBS attribution first, so a PBS storage the PBS API reads directly does
	// not depend on the PVE content list (which hides backups from audit-only
	// tokens).
	var nsByStore map[string][]string
	if t.C.PBSConfigured() {
		var warning string
		var err error
		nsByStore, warning, err = pbsAttribution(ctx, t, refs)
		if err != nil {
			pointsIncomplete = true
			fails = append(fails, fetchFailure{Cluster: t.Name, Scope: "pbs", Error: err.Error()})
		} else if warning != "" {
			fails = append(fails, fetchFailure{Cluster: t.Name, Scope: "pbs attribution", Error: warning})
		}
	}
	standIn := map[string]pbsStorageRef{} // storages the PBS API reads instead of their PVE list
	pbsUnread := map[string]bool{}        // PBS sources (pbsSourceLabel) whose snapshot list failed
	for _, key := range sortedKeys(storages) {
		s := storages[key]
		covered := pbsCovers(refs, s.Storage, t.pbsEndpoint(), nsByStore)
		rps, err := readStorageArchives(ctx, t.C, readers[key], s, onlyVMID, refs, covered)
		if merr := t.storageUnattributed(s, refs); merr != nil {
			rps, err = nil, merr
		}
		if err != nil && covered {
			standIn[key] = refs[s.Storage]
			if !errors.Is(err, errCoveredByPBS) {
				fails = append(fails, fetchFailure{Cluster: t.Name, Scope: "storage " + key, Error: err.Error() + " (its datastore is read through the PBS API instead)"})
			}
			continue
		}
		if err != nil {
			pointsIncomplete = true
			fails = append(fails, fetchFailure{Cluster: t.Name, Scope: "storage " + key, Error: err.Error()})
			continue
		}
		readStorage[key] = true
		if s.Type == "pbs" && !covered {
			pointsIncomplete = true // its points are real; a miss there is unproven
			fails = append(fails, fetchFailure{Cluster: t.Name, Scope: "storage " + key, Error: pveOnlyPBSNote(key)})
		}
		for _, rp := range rps {
			points[rp.VMID] = append(points[rp.VMID], rp)
		}
	}
	if t.C.PBSConfigured() {
		for _, store := range sortedKeys(nsByStore) {
			for _, ns := range nsByStore[store] {
				params := map[string]string(nil)
				if onlyVMID > 0 {
					params = map[string]string{"backup-id": strconv.FormatInt(onlyVMID, 10)}
				}
				snaps, err := pbsSnapshots(ctx, t.C, store, ns, params)
				if err != nil {
					pointsIncomplete = true
					pbsUnread[pbsSourceLabel(store, ns)] = true
					fails = append(fails, fetchFailure{Cluster: t.Name, Scope: pbsSourceLabel(store, ns), Error: err.Error()})
					continue
				}
				for _, s := range snaps {
					if s.BackupType == "vm" || s.BackupType == "ct" {
						rp := s.restorePoint(t.pbsEndpoint(), store, ns)
						points[rp.VMID] = append(points[rp.VMID], rp)
					}
				}
			}
		}
	}
	// Expand each enabled job's schedule once per cluster.
	runsByJob := map[string][]int64{}
	unknownJob := map[string]bool{}
	for _, job := range cov.Jobs {
		if !job.enabled() || job.Schedule == "" {
			continue
		}
		runs, err := scheduledRuns(ctx, t, job.Schedule, start, now)
		if err != nil {
			unknownJob[job.ID] = true
			fails = append(fails, fetchFailure{Cluster: t.Name, Scope: "schedule " + job.ID, Error: err.Error()})
			continue
		}
		runsByJob[job.ID] = runs
	}
	pbsStandIns(standIn, pbsUnread, readStorage)
	targets := newJobTargets(ctx, t.C, cov)
	retention := newRPORetention(ctx, t.C)
	allGaps := map[storageGap]bool{}
	births := newGuestBirths(t, now, offline)
	var rows []rpoRow
	for _, g := range guests {
		vmid := int64(g.VMID)
		if (onlyVMID > 0 && vmid != onlyVMID) || bool(g.Template) || len(cov.EnabledFor[vmid]) == 0 {
			continue
		}
		var runs []int64
		scheduleUnknown := false
		for _, id := range cov.EnabledFor[vmid] {
			runs = append(runs, runsByJob[id]...)
			scheduleUnknown = scheduleUnknown || unknownJob[id]
		}
		// An earlier guest's backups under this VMID are not this one's, and a
		// run scheduled before this guest existed is not one it missed.
		birth := births.read(ctx, g)
		var mine, either []restorePoint
		for _, rp := range dedupeRestorePoints(points[vmid]) {
			if rp.GuestType != "" && rp.GuestType != g.Type {
				continue
			}
			switch birth.of(rp.Time) {
			case ofThisGuest:
				mine = append(mine, rp)
			case ofEitherGuest:
				either = append(either, rp)
			}
		}
		mine, unreadable := splitUnreadable(mine)
		// A VM created inside the window: its window starts at creation, so a
		// first restore point at creation is no sign that retention pruned runs.
		from := start
		if birth.at > start.Unix() {
			from = time.Unix(min(birth.at, now.Unix()), 0)
		}
		// The covering jobs' runs against the points on their own destination
		// (a PBS datastore namespace is one destination however many storages
		// name it); a run scheduled before this guest existed is not one it missed.
		byTarget := map[string]*rpoTarget{}
		var order []string
		for _, id := range cov.EnabledFor[vmid] {
			target := targets.target(id, g.Node)
			var ref *pbsStorageRef
			if r, ok := refs[target]; ok {
				ref = &r
			}
			key := destinationKey(target, ref)
			tg := byTarget[key]
			if tg == nil {
				tg = &rpoTarget{}
				for _, p := range mine {
					if onDestination(p, key) {
						tg.points = append(tg.points, p)
					}
				}
				byTarget[key], order = tg, append(order, key)
			}
			tg.jobRuns = append(tg.jobRuns, slices.DeleteFunc(slices.Clone(runsByJob[id]), func(r int64) bool { return r < birth.at }))
			tg.prunes = tg.prunes || retention.mayPrune(targets.jobs[id], g.Node, target, ref)
		}
		var dests []rpoTarget
		for _, key := range order {
			dests = append(dests, *byTarget[key])
		}
		row := evaluateJobsRPO(mine, dests, from, now)
		row.WindowStart = start.UTC().Format(time.RFC3339)
		if birth.at > start.Unix() && row.ScheduledRuns == 0 && !row.WindowCapped {
			// Created inside the window: no wider window judges runs from before then.
			row.Note = strings.TrimSpace(strings.TrimSuffix(strings.Replace(row.Note, rpoNoJudgedRunNote, "", 1), "; ") + "; the VM was created " + unixRFC3339(birth.at) + ", inside the window, and no scheduled run since then can be judged yet")
			row.Note = strings.TrimPrefix(row.Note, "; ")
		}
		if scheduleUnknown {
			row.MetSchedule, row.Evidence = false, "partial"
			row.Note = joinNote(row.Note, "a covering job's schedule could not be expanded; compliance unproven")
		}
		gaps := jobStorageGaps(cov, vmid, g.Node, readStorage, targets)
		for _, g := range gaps {
			allGaps[g] = true
		}
		// Backups that may be an earlier guest's exist and were read: never "no
		// restore points retained". One predates every judged run (all are at or
		// after creation), so it can neither meet nor excuse one; only an unread
		// config, which also hides when the guest was created, makes misses
		// unproven.
		row.MayBeEarlierGuest = len(either)
		if len(either) > 0 {
			rest := strings.TrimPrefix(strings.TrimPrefix(row.Note, rpoNoPointsNote), "; ")
			row.Note = strings.TrimSuffix(eitherGuestWhy(either, birth)+"; "+rest, "; ")
		}
		if pointsIncomplete || len(gaps) > 0 || len(unreadable) > 0 || birth.unread {
			// Something that may hold restore points was not read: "none" is
			// only "none read".
			row.Note = strings.NewReplacer(rpoNoPointsNote+"; every scheduled run in the window is missed", "no restore point could be read, so the scheduled runs have none that was read",
				rpoNoPointsNote, "no restore point could be read",
				"no restore point inside the window", "no readable restore point inside the window").Replace(row.Note)
			why := "some backup storage could not be read (see fetch_failures)"
			missed := "missed runs may be unread restore points"
			switch {
			case len(gaps) > 0:
				why = "the backup target of " + joinGaps(gaps) + " was not read"
			case birth.unread && !pointsIncomplete && len(unreadable) == 0:
				why = "the VM's creation time could not be read (see fetch_failures)"
				missed = "missed runs may predate its creation"
				if len(either) > 0 {
					missed += " or be among those backups"
				}
			case len(unreadable) > 0 && !pointsIncomplete:
				why = fmt.Sprintf("%d backup(s) are listed without a readable manifest (still running, or damaged)", len(unreadable))
			}
			// Evidence is partial either way. A met schedule stays met (each run
			// has a readable restore point; unread storage can only add points),
			// while missed runs become unproven.
			row.Evidence = "partial"
			row.PointsUnread = true
			if row.MissedRuns > 0 {
				row.MetSchedule = false
				row.Note = joinNote(row.Note, why+"; "+missed)
			} else {
				row.Note = joinNote(row.Note, why+"; gaps and verified days may be understated")
			}
		}
		row.Tenant, row.Cluster, row.VMID, row.Name = t.Tenant, t.Name, vmid, g.Name
		row.Jobs = append([]string{}, cov.EnabledFor[vmid]...)
		rows = append(rows, row)
	}
	fails = append(fails, births.fails...)
	return rows, append(fails, gapFailures(t.Name, allGaps)...), nil
}

// scheduleAnalyzeMax is the API's cap on iterations per call.
const scheduleAnalyzeMax = 100

// scheduledRuns returns the job's run times inside [start, now], computed by
// Proxmox's own calendar-event parser, paging in calls of at most 100 events.
func scheduledRuns(ctx context.Context, t fleetTarget, schedule string, start, now time.Time) ([]int64, error) {
	var out []int64
	from := start.Unix()
	for page := 0; page < 50; page++ { // 5,000 runs: hourly schedules for ~200 days
		var events []struct {
			Timestamp flexInt `json:"timestamp"`
		}
		params := map[string]string{"schedule": schedule, "starttime": strconv.FormatInt(from, 10), "iterations": strconv.Itoa(scheduleAnalyzeMax)}
		if err := getJSON(ctx, t.C, "/cluster/jobs/schedule-analyze", params, &events); err != nil {
			return nil, err
		}
		last := int64(0)
		for _, e := range events {
			ts := int64(e.Timestamp)
			if ts >= start.Unix() && ts <= now.Unix() {
				out = append(out, ts)
			}
			last = ts
		}
		if len(events) < scheduleAnalyzeMax || last > now.Unix() {
			return out, nil
		}
		from = last + 1
	}
	return nil, fmt.Errorf("schedule %q expands to more than 5000 runs in the window", schedule)
}

const rpoNoPointsNote = "no restore points retained"

// joinNote appends one clause to an rpo note.
func joinNote(note, clause string) string {
	if note == "" {
		return clause
	}
	return note + "; " + clause
}

const rpoNoJudgedRunNote = "no scheduled run in the window can be judged yet (the only one is still within the in-progress grace, or the window is shorter than the job's interval); widen --days"

// evaluateRPO scores one guest's restore points against its scheduled runs.
func evaluateRPO(points []restorePoint, runs []int64, start, now time.Time) rpoRow {
	return evaluateJobsRPO(points, []rpoTarget{{jobRuns: [][]int64{runs}, points: points}}, start, now)
}

// rpoTarget is one backup destination of a guest (a storage, or a PBS
// datastore namespace however many storages name it): the scheduled runs of
// each covering job that writes there, the guest's restore points there, and
// whether retention may have deleted older ones (any prune-backups policy on
// a job, its storage or vzdump.conf, a PBS prune job, or one not read).
type rpoTarget struct {
	jobRuns [][]int64
	points  []restorePoint
	prunes  bool
}

// evaluateJobsRPO scores each destination's runs against its own points:
// every job's run owes one point before that job's own next run, and one
// backup time pays one run in all (one archive can show under two storage
// names). Judged runs are paid first (windows by deadline, each taking the
// earliest free point inside it), then in-grace ones. A missed run is
// unproven when retention may have deleted its point (a newer one was kept),
// or when any point not at its destination falls inside its window (a job
// moved storages, the same archive under another name, a manual backup).
func evaluateJobsRPO(all []restorePoint, targets []rpoTarget, start, now time.Time) rpoRow {
	row := rpoRow{WindowStart: start.UTC().Format(time.RFC3339), MissedRunTimes: []string{}}
	// A point dated past the clock-skew tolerance (a node clock ran ahead) has
	// no judgeable time: it proves no run and must not anchor the retention
	// cap, but it is a real backup, so the runs it may cover are unproven.
	cutoff := now.Add(clockSkewTolerance).Unix()
	judgeable := func(pts []restorePoint) []restorePoint {
		out := slices.DeleteFunc(slices.Clone(pts), func(p restorePoint) bool { return p.Time > cutoff })
		sort.Slice(out, func(i, j int) bool { return out[i].Time < out[j].Time })
		return out
	}
	points := judgeable(all)
	future := len(all) - len(points)
	var inWindow []int64
	okDays := map[string]bool{}
	for _, p := range points {
		if p.Time < start.Unix() {
			continue
		}
		inWindow = append(inWindow, p.Time)
		if p.VerifyState == "ok" {
			okDays[time.Unix(p.Time, 0).UTC().Format("2006-01-02")] = true
		}
	}
	row.RestorePoints = len(inWindow)
	row.VerifiedOKDays = len(okDays)
	type window struct {
		tgt         int
		from, until int64
		inGrace     bool
		paid        bool
	}
	var wins []window
	dpts := make([][]restorePoint, len(targets))
	newest := make([]int64, len(targets))
	onTgt := make([]map[string]bool, len(targets))
	capHidesRun, anyRuns := false, false
	for ti, tg := range targets {
		pts := judgeable(tg.points)
		dpts[ti], onTgt[ti] = pts, map[string]bool{}
		for _, p := range tg.points {
			onTgt[ti][p.canonical] = true
		}
		// Effective window: earlier runs may have had points that retention pruned.
		effStart, capped := start.Unix(), false
		if len(pts) > 0 {
			newest[ti] = pts[len(pts)-1].Time
			if pts[0].Time > effStart {
				effStart, capped = pts[0].Time, true
				row.WindowCapped = true
				if c := unixRFC3339(pts[0].Time); c > row.CappedAt {
					row.CappedAt = c
				}
			}
		}
		for _, jr := range tg.jobRuns {
			runs, _ := dedupeRuns(slices.Clone(jr))
			anyRuns = anyRuns || len(runs) > 0
			for i, run := range runs {
				until := now.Unix()
				if i+1 < len(runs) {
					until = runs[i+1]
				}
				// A run whose whole window precedes the oldest retained point may
				// have had a point that retention pruned: not judged. Only the
				// newest run can still be in progress: an older one's window closed
				// when the job's next run was due, so it is judged whatever its age.
				if until <= effStart {
					capHidesRun = capHidesRun || capped
					continue
				}
				wins = append(wins, window{tgt: ti, from: run, until: until, inGrace: i == len(runs)-1 && now.Sub(time.Unix(run, 0)) < rpoInProgressGrace})
			}
		}
	}
	// One backup time pays one run: an exact maximum matching of windows to
	// times (Kuhn's augmenting paths), judged windows first so a run still in
	// its grace never takes a point a judged run needs.
	times := map[int64]int{} // backup time -> index
	var timeAt []int64
	adj := make([][]int, len(wins))
	for w := range wins {
		for _, p := range dpts[wins[w].tgt] {
			if p.Time < wins[w].from || p.Time >= wins[w].until {
				continue
			}
			ti, ok := times[p.Time]
			if !ok {
				ti, times[p.Time], timeAt = len(timeAt), len(timeAt), append(timeAt, p.Time)
			}
			if !slices.Contains(adj[w], ti) {
				adj[w] = append(adj[w], ti)
			}
		}
	}
	owner := make([]int, len(timeAt))
	for i := range owner {
		owner[i] = -1
	}
	var augment func(w int, seen []bool) bool
	augment = func(w int, seen []bool) bool {
		for _, ti := range adj[w] {
			if seen[ti] {
				continue
			}
			seen[ti] = true
			if owner[ti] < 0 || augment(owner[ti], seen) {
				owner[ti] = w
				return true
			}
		}
		return false
	}
	for _, grace := range []bool{false, true} {
		for w := range wins {
			if wins[w].inGrace == grace {
				augment(w, make([]bool, len(timeAt)))
			}
		}
	}
	usedTime := map[int64]bool{}
	for ti, w := range owner {
		if w >= 0 {
			wins[w].paid, usedTime[timeAt[ti]] = true, true
		}
	}
	prunes := slices.ContainsFunc(targets, func(tg rpoTarget) bool { return tg.prunes })
	pending := ""
	var missed []int64
	for _, w := range wins {
		if w.inGrace {
			// It may still be running: not judged, but not met either.
			if !w.paid {
				pending = unixRFC3339(w.from)
			}
			continue
		}
		row.ScheduledRuns++
		if w.paid {
			continue
		}
		missed = append(missed, w.from)
		// Retention never deletes the newest backup: with a newer one kept, this
		// run's may have been pruned (by any covering job's policy: two storage
		// ids can name one directory); with none, it never existed. A point at
		// another destination inside the window that paid no run may be this
		// run's (a job moved storages, a manual backup).
		elsewhereIn := slices.ContainsFunc(points, func(p restorePoint) bool {
			return !onTgt[w.tgt][p.canonical] && !usedTime[p.Time] && p.Time >= w.from && p.Time < w.until
		})
		if (prunes && newest[w.tgt] > w.from) || elsewhereIn {
			row.MissedRunsUnproven++
		}
	}
	slices.Sort(missed)
	row.MissedRuns = len(missed)
	for _, run := range missed {
		row.MissedRunTimes = append(row.MissedRunTimes, unixRFC3339(run))
	}
	// Largest gap: between consecutive points, and from the newest point to now.
	prev := start.Unix()
	if len(inWindow) == 0 {
		row.LargestGapHours = round1(now.Sub(start).Hours())
		row.Note = "no restore point inside the window"
	} else {
		if row.WindowCapped {
			prev = inWindow[0]
		}
		for _, p := range inWindow {
			row.LargestGapHours = math.Max(row.LargestGapHours, round1(float64(p-prev)/3600))
			prev = p
		}
		row.LargestGapHours = math.Max(row.LargestGapHours, round1(float64(now.Unix()-prev)/3600))
	}
	switch {
	case future > 0 && row.MissedRuns > 0:
		// Real backups with unjudgeable dates may be the missed runs: unproven,
		// like a point on unread storage. With no miss they can only add cover.
		row.Note = fmt.Sprintf("%d restore point(s) dated in the future (a node clock ran ahead) were not counted; the missed runs may be among them, so they are unproven", future)
		row.PointsUnread = true
	case future > 0 && len(points) == 0:
		// It has backups, none with a judgeable date: not "no restore points".
		row.Note = fmt.Sprintf("its only %d restore point(s) are dated in the future (a node clock ran ahead) and were not counted", future)
		row.PointsUnread = true
	case len(points) == 0 && future == 0 && row.ScheduledRuns > 0:
		row.Note = rpoNoPointsNote + "; every scheduled run in the window is missed"
	case len(points) == 0 && future == 0:
		row.Note = rpoNoPointsNote
	}
	if row.MissedRunsUnproven > 0 {
		row.Note = joinNote(row.Note, fmt.Sprintf("%d missed run(s) may have restore points that retention pruned (a newer one was kept) or that another storage holds inside the run's window; they are unproven", row.MissedRunsUnproven))
	}
	row.Evidence = "full"
	// Retention may have pruned the start of the window, so a run whose
	// window ended before capped_at cannot be checked; when none did, the cap
	// hides nothing.
	if capHidesRun || row.PointsUnread || row.MissedRunsUnproven > 0 {
		row.Evidence = "partial"
	}
	// Met only when some scheduled run was judged and none was missed: with
	// no run judged (the only one still within the in-progress grace, or a
	// window shorter than the job's interval) there is nothing to have met.
	row.MetSchedule = row.ScheduledRuns > 0 && row.MissedRuns == 0 && row.Evidence == "full" && pending == ""
	if pending != "" {
		row.Note = joinNote(row.Note, "the newest scheduled run ("+pending+") has no restore point yet; it may still be running")
	}
	if row.ScheduledRuns == 0 && anyRuns && row.Note == "" {
		row.Note = rpoNoJudgedRunNote
	}
	return row
}

// dedupeRuns merges only identical timestamps (two jobs firing at the same
// moment); runs at different times each need their own restore point.
// dedupeRuns sorts runs and merges equal times, counting how many jobs run
// at each.
func dedupeRuns(runs []int64) ([]int64, []int) {
	sort.Slice(runs, func(i, j int) bool { return runs[i] < runs[j] })
	out, n := runs[:0], []int{}
	for _, r := range runs {
		if len(out) > 0 && r == out[len(out)-1] {
			n[len(n)-1]++
			continue
		}
		out, n = append(out, r), append(n, 1)
	}
	return out, n
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }

func rollupRPO(rows []rpoRow, targets []fleetTarget, failures []fetchFailure) []rpoClientRow {
	unreachable, scoped := tenantFailures(targets, failures)
	by := map[string]*rpoClientRow{}
	// Every client with a cluster that answered gets a row, guests or not.
	down := downClusters(failures)
	for _, t := range targets {
		if !down[t.Name] && by[t.Tenant] == nil {
			by[t.Tenant] = &rpoClientRow{Tenant: t.Tenant}
		}
	}
	for _, r := range rows {
		c := by[r.Tenant]
		if c == nil {
			c = &rpoClientRow{Tenant: r.Tenant}
			by[r.Tenant] = c
		}
		c.Guests++
		if r.MetSchedule {
			c.GuestsMet++
		}
		if r.Evidence == "partial" {
			c.GuestsPartial++
		}
		switch {
		case r.MetSchedule:
			// Every run has a readable point; unread storage can only add more.
		case r.PointsUnread:
			c.GuestsUnproven++
			c.MissedRunsUnproven += r.MissedRuns
		default:
			if r.RestorePoints == 0 && r.MayBeEarlierGuest == 0 {
				c.GuestsWithNoData++
			}
			certain := r.MissedRuns - r.MissedRunsUnproven
			if certain == 0 && r.MissedRunsUnproven > 0 {
				c.GuestsUnproven++
			}
			c.MissedRuns += certain
			c.MissedRunsUnproven += r.MissedRunsUnproven
		}
		c.WorstGapHours = math.Max(c.WorstGapHours, r.LargestGapHours)
		c.VerifiedOKDays += r.VerifiedOKDays
	}
	for tenant, n := range unreachable {
		if by[tenant] == nil {
			by[tenant] = &rpoClientRow{Tenant: tenant}
		}
		by[tenant].ClustersUnreachable = n
	}
	for tenant, n := range scoped {
		if by[tenant] == nil {
			by[tenant] = &rpoClientRow{Tenant: tenant}
		}
		by[tenant].FetchFailures = n
	}
	out := make([]rpoClientRow, 0, len(by))
	for _, k := range sortedKeys(by) {
		out = append(out, *by[k])
	}
	return out
}
