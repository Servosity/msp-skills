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
	// job's target, a former target, a hidden storage) could not be read, so
	// the missed runs and the restore-point count are unproven, not measured.
	PointsUnread bool   `json:"points_unread,omitempty"`
	Note         string `json:"note,omitempty"`
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
			"reported as missed.",
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
				if err != nil {
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
				fmt.Fprintf(cmd.OutOrStdout(), "%d guest(s) with partial evidence: their missed runs are unproven (see NOTE)\n", partial)
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
	allGaps := map[storageGap]bool{}
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
		var mine []restorePoint
		for _, rp := range dedupeRestorePoints(points[vmid]) {
			if rp.GuestType == "" || rp.GuestType == g.Type {
				mine = append(mine, rp)
			}
		}
		mine, unreadable := splitUnreadable(mine)
		row := evaluateRPO(mine, runs, start, now)
		if scheduleUnknown {
			row.MetSchedule, row.Evidence = false, "partial"
			row.Note = strings.TrimSpace(row.Note + " a covering job's schedule could not be expanded; compliance unproven")
		}
		gaps := jobStorageGaps(cov, vmid, g.Node, readStorage, targets)
		for _, g := range gaps {
			allGaps[g] = true
		}
		if pointsIncomplete || len(gaps) > 0 || len(unreadable) > 0 {
			why := "some backup storage could not be read (see fetch_failures)"
			switch {
			case len(gaps) > 0:
				why = "the backup target of " + joinGaps(gaps) + " was not read"
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
				row.Note = strings.TrimSpace(row.Note + " " + why + "; missed runs may be unread restore points")
			} else {
				row.Note = strings.TrimSpace(row.Note + " " + why + "; gaps and verified days may be understated")
			}
		}
		row.Tenant, row.Cluster, row.VMID, row.Name = t.Tenant, t.Name, vmid, g.Name
		row.Jobs = append([]string{}, cov.EnabledFor[vmid]...)
		rows = append(rows, row)
	}
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

// evaluateRPO scores one guest's restore points against its scheduled runs.
func evaluateRPO(points []restorePoint, runs []int64, start, now time.Time) rpoRow {
	row := rpoRow{WindowStart: start.UTC().Format(time.RFC3339), MissedRunTimes: []string{}}
	// A point dated past now (a node clock ran ahead) proves no run and must
	// not anchor the retention cap either.
	points = slices.DeleteFunc(slices.Clone(points), func(p restorePoint) bool { return p.Time > now.Unix() })
	sort.Slice(points, func(i, j int) bool { return points[i].Time < points[j].Time })
	runs = dedupeRuns(runs)
	// Effective window: earlier runs may have had points that retention pruned.
	effStart := start.Unix()
	if len(points) > 0 && points[0].Time > effStart {
		effStart = points[0].Time
		row.WindowCapped, row.CappedAt = true, unixRFC3339(points[0].Time)
	}
	var inWindow []int64
	okDays := map[string]bool{}
	for _, p := range points {
		if p.Time < start.Unix() || p.Time > now.Unix() {
			continue
		}
		inWindow = append(inWindow, p.Time)
		if p.VerifyState == "ok" {
			okDays[time.Unix(p.Time, 0).UTC().Format("2006-01-02")] = true
		}
	}
	row.RestorePoints = len(inWindow)
	row.VerifiedOKDays = len(okDays)
	for i, run := range runs {
		// Only the newest run can still be in progress: an older one's window
		// closed when the next run was due, so it is judged whatever its age.
		if run < effStart || (i == len(runs)-1 && now.Sub(time.Unix(run, 0)) < rpoInProgressGrace) {
			continue
		}
		row.ScheduledRuns++
		until := now.Unix()
		if i+1 < len(runs) {
			until = runs[i+1]
		}
		hit := false
		for _, p := range inWindow {
			if p >= run && p < until {
				hit = true
				break
			}
		}
		if !hit {
			row.MissedRuns++
			row.MissedRunTimes = append(row.MissedRunTimes, unixRFC3339(run))
		}
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
	if len(points) == 0 {
		row.Note = "no restore points retained; every scheduled run in the window is missed"
	}
	row.Evidence = "full"
	if row.WindowCapped {
		// Retention pruned the start of the window, so runs before capped_at
		// (and any that failed there) cannot be checked.
		row.Evidence = "partial"
	}
	// Met only when some scheduled run was judged and none was missed: with
	// no run judged (the only one still within the in-progress grace, or a
	// window shorter than the job's interval) there is nothing to have met.
	row.MetSchedule = row.ScheduledRuns > 0 && row.MissedRuns == 0 && row.Evidence == "full"
	if row.ScheduledRuns == 0 && len(runs) > 0 && row.Note == "" {
		row.Note = "no scheduled run in the window can be judged yet (the only one is still within the in-progress grace, or the window is shorter than the job's interval); widen --days"
	}
	return row
}

// dedupeRuns merges only identical timestamps (two jobs firing at the same
// moment); runs at different times each need their own restore point.
func dedupeRuns(runs []int64) []int64 {
	sort.Slice(runs, func(i, j int) bool { return runs[i] < runs[j] })
	out := runs[:0]
	for _, r := range runs {
		if len(out) > 0 && r == out[len(out)-1] {
			continue
		}
		out = append(out, r)
	}
	return out
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }

func rollupRPO(rows []rpoRow, targets []fleetTarget, failures []fetchFailure) []rpoClientRow {
	unreachable, scoped := tenantFailures(targets, failures)
	by := map[string]*rpoClientRow{}
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
			if r.RestorePoints == 0 {
				c.GuestsWithNoData++
			}
			c.MissedRuns += r.MissedRuns
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
