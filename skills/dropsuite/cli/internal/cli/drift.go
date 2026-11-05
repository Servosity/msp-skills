// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: diff two local health snapshots.
// pp:data-source local

package cli

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type driftChange struct {
	OrgID    string `json:"org_id"`
	OrgEmail string `json:"org_email"`
	Kind     string `json:"kind"`
	Object   string `json:"object"`
	ObjectID string `json:"object_id"`
	Change   string `json:"change"`
	Before   string `json:"before"`
	After    string `json:"after"`
}

type driftReport struct {
	From        string `json:"from_run"`
	FromAt      string `json:"from_taken_at"`
	To          string `json:"to_run"`
	ToAt        string `json:"to_taken_at"`
	Runs        int    `json:"runs_available"`
	NotCompared int    `json:"not_compared"`
	// UnreadScopes lists org/workload pairs (org_id/kind; kind * for a whole
	// organization) that either run did not read completely. Objects there
	// are never reported added or removed; an object seen in both runs is
	// still compared, since both rows were actually read.
	UnreadScopes []string       `json:"unread_scopes"`
	Counts       map[string]int `json:"counts"`
	Changes      []driftChange  `json:"changes"`
	Note         string         `json:"note,omitempty"`
}

// pickDriftRuns returns the baseline (the newest run at or before
// latest-since, else the oldest run) and the latest run.
func pickDriftRuns(runs []snapshotRun, since time.Duration) (from, to snapshotRun, ok bool) {
	if len(runs) < 2 {
		return snapshotRun{}, snapshotRun{}, false
	}
	to = runs[len(runs)-1]
	cutoff := to.TakenAt.Add(-since)
	from = runs[0]
	for _, r := range runs[:len(runs)-1] {
		if !r.TakenAt.After(cutoff) {
			from = r
		}
	}
	return from, to, true
}

// driftChanges are the change types diffSnapshots can report.
var driftChanges = []string{"added", "removed", "grade_changed", "status_changed", "backup_not_advancing", "items_dropped", "storage_dropped"}

func scopePair(org, kind string) string { return org + "\x00" + kind }

// backupStallAfter is how old an unchanged last backup must be at the later
// run before drift reports it as not advancing. Two health runs taken
// minutes apart see the same last_backup on every healthy object; that is
// not a stall.
const backupStallAfter = 24 * time.Hour

// backupStalled reports whether a last_backup that did not change between
// runs is older than backupStallAfter at the later run. A value that does
// not parse keeps the plain equality signal rather than hiding a stall.
func backupStalled(lastBackup string, at time.Time) bool {
	t, err := time.Parse(time.RFC3339, lastBackup)
	if err != nil {
		return true
	}
	return at.Sub(t) > backupStallAfter
}

// driftGap renders the time between two runs for the short-window note.
func driftGap(d time.Duration) string {
	switch {
	case d < time.Minute:
		return strconv.Itoa(int(d.Seconds())) + "s"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + "m"
	}
	return humanAge(d)
}

// readScope returns the organization/workload pairs a snapshot read
// completely. Snapshots written before scope markers existed fall back to
// every pair that has objects and no failed-read row.
func readScope(rows []snapshotRow) map[string]bool {
	read := map[string]bool{}
	hasMarkers := false
	for _, r := range rows {
		if r.Object == scopeMarker {
			hasMarkers = true
			read[scopePair(r.OrgID, r.Kind)] = true
		}
	}
	if hasMarkers {
		return read
	}
	failedOrg := map[string]bool{}
	failedPair := map[string]bool{}
	for _, r := range rows {
		if r.Object == "*" {
			if r.Kind == "*" {
				failedOrg[r.OrgID] = true
			} else {
				failedPair[scopePair(r.OrgID, r.Kind)] = true
			}
		}
	}
	for _, r := range rows {
		k := scopePair(r.OrgID, r.Kind)
		if r.Object != "*" && r.Object != scopeMarker && !failedOrg[r.OrgID] && !failedPair[k] {
			read[k] = true
		}
	}
	return read
}

// unreadScopes returns every org/workload pair seen in either run (objects,
// failed-read rows or scope markers) that is not read completely in both.
func unreadScopes(before, after []snapshotRow) []string {
	readBefore, readAfter := readScope(before), readScope(after)
	seen := map[string]string{}
	for _, rows := range [][]snapshotRow{before, after} {
		for _, r := range rows {
			seen[scopePair(r.OrgID, r.Kind)] = r.OrgID + "/" + r.Kind
		}
	}
	out := make([]string, 0)
	for k, label := range seen {
		if !readBefore[k] || !readAfter[k] {
			out = append(out, label)
		}
	}
	sort.Strings(out)
	return out
}

// diffSnapshots compares two snapshot row sets keyed by org/kind/object id. An
// object only counts as added or removed when both runs read its
// organization/workload completely; anything else is counted in notCompared
// (a scoped health run or a failed read is not a removal).
func diffSnapshots(before, after []snapshotRow) (changes []driftChange, notCompared int) {
	key := func(r snapshotRow) string {
		id := r.ObjectID
		if id == "" {
			id = r.Object
		}
		return r.OrgID + "\x00" + r.Kind + "\x00" + id
	}
	isObject := func(r snapshotRow) bool { return r.Object != "*" && r.Object != scopeMarker }
	readBefore, readAfter := readScope(before), readScope(after)
	b := map[string]snapshotRow{}
	for _, r := range before {
		if isObject(r) {
			b[key(r)] = r
		}
	}
	a := map[string]snapshotRow{}
	for _, r := range after {
		if isObject(r) {
			a[key(r)] = r
		}
	}
	changes = make([]driftChange, 0)
	add := func(r snapshotRow, change, before, after string) {
		changes = append(changes, driftChange{OrgID: r.OrgID, OrgEmail: r.OrgEmail, Kind: r.Kind, Object: r.Object, ObjectID: r.ObjectID, Change: change, Before: before, After: after})
	}
	for k, nr := range a {
		or, existed := b[k]
		if !existed {
			if p := scopePair(nr.OrgID, nr.Kind); readBefore[p] && readAfter[p] {
				add(nr, "added", "", nr.Grade)
			} else {
				notCompared++
			}
			continue
		}
		if or.Grade != nr.Grade {
			add(nr, "grade_changed", or.Grade, nr.Grade)
		}
		if or.Status != nr.Status && nr.Status != "" {
			add(nr, "status_changed", or.Status, nr.Status)
		}
		if or.LastBackup != "" && or.LastBackup == nr.LastBackup && nr.TakenAt.After(or.TakenAt) && backupStalled(nr.LastBackup, nr.TakenAt) {
			add(nr, "backup_not_advancing", or.LastBackup, nr.LastBackup)
		}
		// Counts compare only when both runs know them.
		if or.Items != nil && nr.Items != nil && *nr.Items < *or.Items {
			add(nr, "items_dropped", fmt.Sprint(*or.Items), fmt.Sprint(*nr.Items))
		}
		if or.Storage != nil && nr.Storage != nil && *nr.Storage < *or.Storage {
			add(nr, "storage_dropped", fmt.Sprint(*or.Storage), fmt.Sprint(*nr.Storage))
		}
	}
	for k, or := range b {
		if _, still := a[k]; still {
			continue
		}
		if p := scopePair(or.OrgID, or.Kind); readBefore[p] && readAfter[p] {
			add(or, "removed", or.Grade, "")
		} else {
			notCompared++
		}
	}
	sort.SliceStable(changes, func(i, j int) bool {
		x, y := changes[i], changes[j]
		if x.OrgID != y.OrgID {
			return x.OrgID < y.OrgID
		}
		if x.Kind != y.Kind {
			return x.Kind < y.Kind
		}
		if x.Object != y.Object {
			return x.Object < y.Object
		}
		return x.Change < y.Change
	})
	return changes, notCompared
}

func newNovelDriftCmd(flags *rootFlags) *cobra.Command {
	var sinceRaw, dbPath, changeFilter string
	var limit int

	cmd := &cobra.Command{
		Use:   "drift",
		Short: "See what changed between health runs: objects that left backup, backups that stopped advancing and item-count drops",
		Long: `Use this command for what changed between health runs: objects added to or removed from backup, backups that stopped advancing, and item-count drops. Do NOT use this command for current failures; use 'health' instead.

It compares the latest local health snapshot with the newest snapshot taken at
least --since earlier (or the oldest one, with a note saying the window is
shorter). Changes: added, removed, grade_changed, status_changed,
backup_not_advancing (same last_backup in both runs and that backup more than
24h old at the later run), items_dropped and storage_dropped. It reads only
the local store; run 'health' at least twice first.`,
		Example: strings.Trim(`
  dropsuite-cli drift --since 7d --agent
  dropsuite-cli drift --since 24h --change backup_not_advancing`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if err := rejectArgs(args); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "drift")
			}
			if changeFilter != "" && !slices.Contains(driftChanges, changeFilter) {
				return usageErr(fmt.Errorf("--change %q is not a change type (valid: %s)", changeFilter, strings.Join(driftChanges, ", ")))
			}
			since, err := parseDurationFlag("since", sinceRaw, "7d")
			if err != nil {
				return err
			}
			rep := driftReport{Counts: map[string]int{}, Changes: make([]driftChange, 0), UnreadScopes: make([]string, 0)}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			db, err := openFleetDBReadOnly(ctx, dbPath)
			if err != nil {
				return err
			}
			if db == nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "no local store at %s\nrun: dropsuite-cli health (twice, some time apart) to record snapshots\n", fleetDBPath(dbPath))
				rep.Note = "no local store yet; run health at least twice"
				return printDrift(cmd, flags, rep)
			}
			defer db.Close()
			runs, err := listSnapshotRuns(ctx, db.DB())
			if err != nil {
				return fmt.Errorf("reading snapshots: %w", err)
			}
			rep.Runs = len(runs)
			from, to, ok := pickDriftRuns(runs, since)
			if !ok {
				rep.Note = fmt.Sprintf("%d health snapshot(s) recorded; run health at least twice to see drift", len(runs))
				return printDrift(cmd, flags, rep)
			}
			before, err := loadSnapshotRows(ctx, db.DB(), from.RunID, "")
			if err != nil {
				return err
			}
			after, err := loadSnapshotRows(ctx, db.DB(), to.RunID, "")
			if err != nil {
				return err
			}
			rep.From, rep.FromAt = from.RunID, from.TakenAt.Format(time.RFC3339)
			rep.To, rep.ToAt = to.RunID, to.TakenAt.Format(time.RFC3339)
			changes, notCompared := diffSnapshots(before, after)
			rep.NotCompared = notCompared
			rep.UnreadScopes = unreadScopes(before, after)
			var notes []string
			if gap := to.TakenAt.Sub(from.TakenAt); gap < since {
				notes = append(notes, fmt.Sprintf("no snapshot is %s older than the latest, so this compares against the oldest run, %s earlier", sinceRaw, driftGap(gap)))
			}
			if notCompared > 0 || len(rep.UnreadScopes) > 0 {
				notes = append(notes, fmt.Sprintf("%d organization/workload scope(s) were not read completely by both health runs (failed read or scoped run), so no object there is reported added or removed; %d object(s) went uncompared", len(rep.UnreadScopes), notCompared))
			}
			rep.Note = strings.Join(notes, "; ")
			for _, c := range changes {
				if changeFilter != "" && c.Change != changeFilter {
					continue
				}
				rep.Counts[c.Change]++
				rep.Changes = append(rep.Changes, c)
			}
			if limit > 0 && len(rep.Changes) > limit {
				rep.Changes = rep.Changes[:limit]
			}
			return printDrift(cmd, flags, rep)
		},
	}
	cmd.Flags().StringVar(&sinceRaw, "since", "7d", "Compare against the newest snapshot at least this much older than the latest (e.g. 24h, 7d)")
	cmd.Flags().StringVar(&changeFilter, "change", "", "Only one change type: added, removed, grade_changed, status_changed, backup_not_advancing, items_dropped, storage_dropped")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local store path (default: the CLI's data.db)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum changes to return (0 = all)")
	return cmd
}

func printDrift(cmd *cobra.Command, flags *rootFlags, rep driftReport) error {
	if !wantsHumanTable(cmd.OutOrStdout(), flags) {
		return printJSONFiltered(cmd.OutOrStdout(), rep, flags)
	}
	w := cmd.OutOrStdout()
	if rep.To == "" {
		fmt.Fprintln(w, rep.Note)
		return nil
	}
	fmt.Fprintf(w, "Changes from %s to %s: %d\n", rep.FromAt, rep.ToAt, len(rep.Changes))
	if rep.Note != "" {
		fmt.Fprintln(w, rep.Note)
	}
	if len(rep.Changes) == 0 {
		return nil
	}
	rows := make([][]string, 0, len(rep.Changes))
	for _, c := range rep.Changes {
		rows = append(rows, []string{c.Change, c.OrgEmail, c.Kind, c.Object, c.Before, c.After})
	}
	return flags.printTable(cmd, []string{"CHANGE", "ORG", "KIND", "OBJECT", "BEFORE", "AFTER"}, rows)
}
