// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Novel feature: sync-to-sync fleet diff.
// pp:data-source local

package cli

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"immybot-pp-cli/internal/cliutil"
	"immybot-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

type fleetChange struct {
	ResourceType string `json:"resource_type"`
	ID           string `json:"id"`
	Label        string `json:"label,omitempty"`
	Change       string `json:"change"`
	Before       string `json:"before,omitempty"`
	After        string `json:"after,omitempty"`
}

type fleetDiffView struct {
	Baseline string `json:"baseline_snapshot,omitempty"`
	Compared string `json:"compared_against"`
	Since    string `json:"since,omitempty"`
	// SinceUnmet is set when no snapshot is known to be at least --since old,
	// so the baseline is the oldest dated one (or, with none dated, the oldest
	// recorded) and covers a shorter window. UndatedSnapshots counts the
	// snapshots whose taken_at does not parse; --since never picks one.
	SinceUnmet       bool `json:"since_unmet,omitempty"`
	UndatedSnapshots int  `json:"undated_snapshots,omitempty"`
	// UnidentifiedComputers counts mirrored computer rows with no computer id:
	// the merged roster cannot key them, so they are not tracked.
	UnidentifiedComputers int           `json:"unidentified_computers,omitempty"`
	TrackedTypes          []string      `json:"tracked_resource_types"`
	NotCompared           []string      `json:"not_compared,omitempty"`
	CurrentCount          int           `json:"current_objects"`
	BaselineCount         int           `json:"baseline_objects"`
	Added                 []fleetChange `json:"added"`
	Removed               []fleetChange `json:"removed"`
	Changed               []fleetChange `json:"changed"`
	SnapshotTaken         string        `json:"snapshot_taken,omitempty"`
	Note                  string        `json:"note,omitempty"`
}

// fleetTracked lists the resource types fleet-diff watches, with the fields
// that make up each object's fingerprint. Only fields whose change is
// operationally meaningful are included, so cosmetic churn does not show up as
// a diff.
var fleetTracked = []struct {
	resourceTypes []string
	kind          string
	labelField    string
	fields        []string
	// computerRow fingerprints the merged computer immyLoadComputers builds
	// from every resource type (each read with its own schema's field
	// names); fields then only names the shape.
	computerRow bool
}{
	{
		resourceTypes: immyComputerRosterTypes,
		kind:          "computer",
		fields: []string{"computer-roster:name", "computer-roster:tenantId", "computer-roster:online", "computer-roster:excludeFromMaintenance",
			"computer-roster:source-disagreements"},
		computerRow: true,
	},
	{
		resourceTypes: []string{"tenants-software-from-inventory-dx"},
		kind:          "software-install",
		labelField:    "$.softwareName",
		fields:        []string{"$.softwareName", "$.version", "$.computerName", "$.tenantName"},
	},
	// Local and global deployments number their ids separately, so they are
	// two kinds: one kind over both stores would key Local 5 and Global 5 as
	// one object and hide a change to either.
	{
		resourceTypes: []string{"target-assignments"},
		kind:          "deployment",
		labelField:    "$.targetName",
		fields:        []string{"$.maintenanceIdentifier", "$.targetName", "$.targetText", "$.excluded", "$.targetEnforcement"},
	},
	{
		resourceTypes: []string{"target-assignments-global"},
		kind:          "global-deployment",
		labelField:    "$.targetName",
		fields:        []string{"$.maintenanceIdentifier", "$.targetName", "$.targetText", "$.excluded", "$.targetEnforcement"},
	},
}

// fleetFingerprintVersion names how collectFleetState turns a kind's fields
// into a fingerprint. It is part of every recorded shape: change it whenever
// that computation changes, so older baselines stop being compared instead of
// reporting every object as changed.
const fleetFingerprintVersion = "sha256-nul-v1"

// fleetShape is what a kind's snapshot rows mean: how the fingerprint is
// computed, from which mirrored resource types (whose ids key the rows), over
// which fields. A baseline compares on a kind only when its shape matches.
func fleetShape(fingerprint string, resourceTypes, fields []string) string {
	return fingerprint + " types=" + strings.Join(slices.Sorted(slices.Values(resourceTypes)), ",") +
		" fields=" + strings.Join(fields, ",")
}

// fleetKindShapes is the shape of every kind this version tracks; --snapshot
// records it with the baseline.
func fleetKindShapes() map[string]string {
	out := make(map[string]string, len(fleetTracked))
	for _, t := range fleetTracked {
		out[t.kind] = fleetShape(fleetFingerprintVersion, t.resourceTypes, t.fields)
	}
	return out
}

// fleetUnversionedShapes is the shape of every snapshot that recorded no
// kinds. immybot v0.1.x wrote those with one "deployment" kind over both
// assignment stores, so Local 5 and Global 5 were one key and none of their
// deployment rows can be attributed to either store, whatever the rows hold.
// An unreleased build that already split deployments also wrote unversioned
// snapshots; it gets the same answer, which costs a re-baseline and never
// invents a change. Frozen: never edit it to follow fleetTracked.
var fleetUnversionedShapes = map[string]string{
	"computer": fleetShape("sha256-nul-v1", []string{"computers", "computers-paged", "computers-dx"},
		[]string{"$.name", "$.tenantId", "$.online", "$.excludeFromMaintenance"}),
	"software-install": fleetShape("sha256-nul-v1", []string{"tenants-software-from-inventory-dx"},
		[]string{"$.softwareName", "$.version", "$.computerName", "$.tenantName"}),
	"deployment": fleetShape("sha256-nul-v1", []string{"target-assignments", "target-assignments-global"},
		[]string{"$.maintenanceIdentifier", "$.targetName", "$.targetText", "$.excluded", "$.targetEnforcement"}),
}

func newNovelFleetDiffCmd(flags *rootFlags) *cobra.Command {
	var (
		flagSince    string
		flagSnapshot bool
		flagLimit    int
		dbPath       string
	)

	cmd := &cobra.Command{
		Use:   "fleet-diff",
		Short: "What changed in the fleet between two syncs",
		Long: "What actually changed between two syncs: computers added or removed, software " +
			"versions moved, deployments modified.\n\n" +
			"The ImmyBot API exposes no updated-since cursor, so change detection is only " +
			"possible against local snapshots. Record one with --snapshot, then compare later " +
			"runs against it. A baseline compares only on the types it recorded the way this " +
			"version tracks them; any other type is listed in not_compared and skipped, never " +
			"reported as added, removed or changed. A computer is fingerprinted once, from the record " +
			"merged across every computer list the mirror holds (the newest sync of each field wins), " +
			"and a change in any one list counts as a change; a row with no computer id cannot be " +
			"tracked and is counted in unidentified_computers.\n\n" +
			"--since compares against the newest snapshot known to be at least that old. When none " +
			"is, it compares against the oldest dated snapshot and sets since_unmet; a snapshot " +
			"whose time does not parse is never taken as old enough and is counted in undated_snapshots.\n\n" +
			"Use this command for what changed in the fleet between two syncs. Do NOT use this " +
			"command for grouping the failures a maintenance window produced; use " +
			"'session-triage' instead.",
		Example: strings.Trim(`
  immybot-cli fleet-diff --snapshot
  immybot-cli fleet-diff --since 24h
  immybot-cli fleet-diff --since 7d --agent
`, "\n"),
		// --snapshot writes a baseline into the CLI's own store, so this is a
		// local-write tool, never read-only; with the MCP read-only switch on
		// the walker drops --snapshot and the diff stays a pure read.
		Annotations: map[string]string{
			"mcp:local-write":            "true",
			"mcp:read-only-unless-flags": "snapshot",
			"pp:happy-args":              "--since=24h",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet-diff")
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			var cutoff time.Time
			if s := strings.TrimSpace(flagSince); s != "" {
				d, err := cliutil.ParseDurationLoose(s)
				if err != nil {
					_ = cmd.Usage()
					return usageErr(fmt.Errorf("--since %q: %w", s, err))
				}
				cutoff = time.Now().UTC().Add(-d)
			}

			kinds := make([]string, 0, len(fleetTracked))
			for _, t := range fleetTracked {
				kinds = append(kinds, t.kind)
			}
			view := fleetDiffView{
				Since:        strings.TrimSpace(flagSince),
				TrackedTypes: kinds,
				Added:        make([]fleetChange, 0),
				Removed:      make([]fleetChange, 0),
				Changed:      make([]fleetChange, 0),
			}

			dbPath = immyMirrorPath(dbPath)
			if immyMirrorMissing(cmd, dbPath, "computers,target-assignments,tenants-software-from-inventory-dx") {
				if !wantsHumanTable(cmd.OutOrStdout(), flags) {
					return printJSONFiltered(cmd.OutOrStdout(), view, flags)
				}
				return nil
			}
			// Only --snapshot writes; every other run reads the mirror read-only.
			openMirror := immyOpenMirror
			if flagSnapshot {
				openMirror = store.OpenWithContext
			}
			db, err := openMirror(ctx, dbPath)
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "computers") {
				hintIfStale(cmd, db, "computers", flags.maxAge)
			}

			current, unidentified, err := collectFleetState(ctx, db)
			if err != nil {
				return err
			}
			view.CurrentCount = len(current)
			view.UnidentifiedComputers = unidentified
			unidentifiedNote := func() {
				if unidentified > 0 {
					msg := fmt.Sprintf("%d mirrored computer row(s) carry no computer id and are not tracked", unidentified)
					view.Note = strings.TrimPrefix(view.Note+"; "+msg, "; ")
				}
			}

			if flagSnapshot {
				// Fixed-width nanoseconds: two snapshots in one second stay two
				// generations, and the keys still sort as text.
				takenAt := time.Now().UTC().Format("2006-01-02T15:04:05.000000000Z07:00")
				rows := make([]store.ImmyFleetRow, 0, len(current))
				for _, r := range current {
					rows = append(rows, r)
				}
				if err := db.WriteImmyFleetSnapshot(ctx, takenAt, fleetKindShapes(), rows); err != nil {
					return fmt.Errorf("writing fleet snapshot: %w", err)
				}
				view.SnapshotTaken = takenAt
				view.Compared = "none (snapshot only)"
				view.Note = fmt.Sprintf("recorded snapshot of %d object(s) at %s", len(rows), takenAt)
				unidentifiedNote()
				if !wantsHumanTable(cmd.OutOrStdout(), flags) {
					return printJSONFiltered(cmd.OutOrStdout(), view, flags)
				}
				fmt.Fprintln(cmd.OutOrStdout(), view.Note)
				return nil
			}

			times, err := db.ImmyFleetSnapshotTimes(ctx)
			if err != nil {
				return fmt.Errorf("reading fleet snapshots: %w", err)
			}
			if len(times) == 0 {
				view.Compared = "none"
				view.Note = "no fleet snapshots recorded yet; run 'immybot-cli fleet-diff --snapshot' after a sync to establish a baseline"
				if !wantsHumanTable(cmd.OutOrStdout(), flags) {
					return printJSONFiltered(cmd.OutOrStdout(), view, flags)
				}
				fmt.Fprintln(cmd.OutOrStdout(), view.Note)
				return nil
			}

			baseline := times[0]
			if !cutoff.IsZero() {
				var met bool
				baseline, met, view.UndatedSnapshots = fleetSinceBaseline(times, cutoff)
				view.SinceUnmet = !met
			}
			view.Baseline = baseline
			view.Compared = "current local mirror"

			prev, err := db.ReadImmyFleetSnapshot(ctx, baseline)
			if err != nil {
				return fmt.Errorf("reading fleet snapshot %s: %w", baseline, err)
			}
			view.BaselineCount = len(prev)

			// A kind compares only when the baseline recorded it in the shape
			// this version tracks. Which rows a snapshot holds says nothing
			// about what it tracked: a fleet with no global deployment yet
			// records none, exactly as a snapshot that keyed them as local.
			baseShapes, versioned, err := db.ReadImmyFleetSnapshotKinds(ctx, baseline)
			if err != nil {
				return fmt.Errorf("reading fleet snapshot %s kinds: %w", baseline, err)
			}
			if !versioned {
				baseShapes = fleetUnversionedShapes
			}
			nowShapes := fleetKindShapes()
			skipKinds := map[string]bool{}
			for _, r := range prev {
				if nowShapes[r.ResourceType] == "" || baseShapes[r.ResourceType] != nowShapes[r.ResourceType] {
					skipKinds[r.ResourceType] = true
				}
			}
			for _, kind := range kinds {
				if baseShapes[kind] != nowShapes[kind] {
					skipKinds[kind] = true
					view.NotCompared = append(view.NotCompared, kind)
				}
			}
			sort.Strings(view.NotCompared)

			for key, cur := range current {
				if skipKinds[cur.ResourceType] {
					continue
				}
				old, ok := prev[key]
				if !ok {
					view.Added = append(view.Added, fleetChange{
						ResourceType: cur.ResourceType, ID: cur.ID, Label: cur.Label, Change: "added",
					})
					continue
				}
				if old.Fingerprint != cur.Fingerprint {
					view.Changed = append(view.Changed, fleetChange{
						ResourceType: cur.ResourceType, ID: cur.ID, Label: cur.Label, Change: "changed",
						Before: old.Label, After: cur.Label,
					})
				}
			}
			for key, old := range prev {
				if skipKinds[old.ResourceType] {
					continue
				}
				if _, ok := current[key]; !ok {
					view.Removed = append(view.Removed, fleetChange{
						ResourceType: old.ResourceType, ID: old.ID, Label: old.Label, Change: "removed",
					})
				}
			}
			sortChanges(view.Added)
			sortChanges(view.Removed)
			sortChanges(view.Changed)
			if flagLimit > 0 {
				view.Added = capChanges(view.Added, flagLimit)
				view.Removed = capChanges(view.Removed, flagLimit)
				view.Changed = capChanges(view.Changed, flagLimit)
			}
			if len(view.Added)+len(view.Removed)+len(view.Changed) == 0 {
				view.Note = fmt.Sprintf("no tracked changes between snapshot %s and the current mirror", baseline)
			}
			if view.SinceUnmet {
				msg := fmt.Sprintf("no snapshot is known to be at least %s old, so this compares against %s, a shorter window than --since asked for",
					view.Since, baseline)
				view.Note = strings.TrimPrefix(view.Note+"; "+msg, "; ")
			}
			if view.UndatedSnapshots > 0 {
				msg := fmt.Sprintf("%d snapshot(s) have a taken_at that does not parse; --since never picks one", view.UndatedSnapshots)
				view.Note = strings.TrimPrefix(view.Note+"; "+msg, "; ")
			}
			if len(view.NotCompared) > 0 {
				why := "recorded them in another shape than this version tracks, or not at all"
				if !versioned {
					why = "predates recorded snapshot shapes; the versions before them keyed local and global deployments as one type"
				}
				msg := fmt.Sprintf("not compared: %s (snapshot %s %s); run 'immybot-cli fleet-diff --snapshot' to record a new baseline",
					strings.Join(view.NotCompared, ", "), baseline, why)
				view.Note = strings.TrimPrefix(view.Note+"; "+msg, "; ")
			}
			unidentifiedNote()

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Baseline %s -> current mirror\n", baseline)
			fmt.Fprintf(cmd.OutOrStdout(), "  added   %d\n  removed %d\n  changed %d\n",
				len(view.Added), len(view.Removed), len(view.Changed))
			for _, c := range view.Changed {
				fmt.Fprintf(cmd.OutOrStdout(), "  ~ %-18s %s\n", c.ResourceType, immyTruncate(firstNonEmpty(c.Label, c.ID), 60))
			}
			for _, c := range view.Added {
				fmt.Fprintf(cmd.OutOrStdout(), "  + %-18s %s\n", c.ResourceType, immyTruncate(firstNonEmpty(c.Label, c.ID), 60))
			}
			for _, c := range view.Removed {
				fmt.Fprintf(cmd.OutOrStdout(), "  - %-18s %s\n", c.ResourceType, immyTruncate(firstNonEmpty(c.Label, c.ID), 60))
			}
			if len(view.NotCompared) > 0 || view.SinceUnmet || view.UndatedSnapshots > 0 || view.UnidentifiedComputers > 0 {
				fmt.Fprintln(cmd.OutOrStdout(), view.Note)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "", "Compare against the newest snapshot at least this old (e.g. 24h, 7d); with none that old, the oldest dated one, flagged since_unmet")
	cmd.Flags().BoolVar(&flagSnapshot, "snapshot", false, "Record the current mirror state as a new baseline instead of diffing")
	cmd.Flags().IntVar(&flagLimit, "limit", 200, "Maximum rows per change list (0 for all)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local mirror database path")
	return cmd
}

// collectFleetState fingerprints every tracked object in the current mirror.
// Each resource group is drained fully before the next query runs.
func collectFleetState(ctx context.Context, db *store.Store) (map[string]store.ImmyFleetRow, int, error) {
	out := map[string]store.ImmyFleetRow{}
	unidentified := 0
	for _, tracked := range fleetTracked {
		if tracked.computerRow {
			n, err := collectFleetComputers(ctx, db, tracked.kind, tracked.resourceTypes, out)
			if err != nil {
				return nil, 0, err
			}
			unidentified += n
			continue
		}
		placeholders := make([]string, 0, len(tracked.resourceTypes))
		args := make([]any, 0, len(tracked.resourceTypes))
		for _, rt := range tracked.resourceTypes {
			placeholders = append(placeholders, "?")
			args = append(args, rt)
		}
		selects := make([]string, 0, len(tracked.fields))
		for _, f := range tracked.fields {
			selects = append(selects, fmt.Sprintf("COALESCE(json_extract(data,'%s'),'')", f))
		}
		query := fmt.Sprintf(
			"SELECT id, COALESCE(json_extract(data,'%s'),''), %s FROM resources WHERE resource_type IN (%s)",
			tracked.labelField, strings.Join(selects, ", "), strings.Join(placeholders, ","))

		rows, err := db.DB().QueryContext(ctx, query, args...)
		if err != nil {
			return nil, 0, fmt.Errorf("querying %s state: %w", tracked.kind, err)
		}
		for rows.Next() {
			cols := make([]any, 0, len(tracked.fields)+2)
			var id, label sql.NullString
			cols = append(cols, &id, &label)
			vals := make([]sql.NullString, len(tracked.fields))
			for i := range vals {
				cols = append(cols, &vals[i])
			}
			if err := rows.Scan(cols...); err != nil {
				_ = rows.Close()
				return nil, 0, fmt.Errorf("scanning %s state: %w", tracked.kind, err)
			}
			parts := make([]string, 0, len(vals))
			for _, v := range vals {
				parts = append(parts, nullStr(v))
			}
			sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
			row := store.ImmyFleetRow{
				ResourceType: tracked.kind,
				ID:           nullStr(id),
				Fingerprint:  hex.EncodeToString(sum[:]),
				Label:        nullStr(label),
			}
			out[row.ResourceType+"\x00"+row.ID] = row
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, 0, fmt.Errorf("iterating %s state: %w", tracked.kind, err)
		}
		if err := rows.Close(); err != nil {
			return nil, 0, fmt.Errorf("closing %s state: %w", tracked.kind, err)
		}
	}
	return out, unidentified, nil
}

// fleetSinceBaseline picks the --since baseline: the newest snapshot whose
// taken_at parses and is before cutoff (met). With none, it falls back to the
// oldest dated snapshot, or with none dated the oldest recorded, so a
// too-recent baseline still produces output, flagged. A snapshot whose time
// does not parse is counted in undated and never taken as old enough. times
// is newest first as text; the choice compares parsed instants.
func fleetSinceBaseline(times []string, cutoff time.Time) (baseline string, met bool, undated int) {
	var best, oldest string
	var bestAt, oldestAt time.Time
	for _, t := range times {
		at, ok := immyParseTimestamp(t)
		if !ok {
			undated++
			continue
		}
		if at.Before(cutoff) && (best == "" || at.After(bestAt)) {
			best, bestAt = t, at
		}
		if oldest == "" || at.Before(oldestAt) {
			oldest, oldestAt = t, at
		}
	}
	switch {
	case best != "":
		return best, true, undated
	case oldest != "":
		return oldest, false, undated
	}
	return times[len(times)-1], false, undated
}

func sortChanges(in []fleetChange) {
	sort.Slice(in, func(i, j int) bool {
		if in[i].ResourceType != in[j].ResourceType {
			return in[i].ResourceType < in[j].ResourceType
		}
		return in[i].ID < in[j].ID
	})
}

func capChanges(in []fleetChange, n int) []fleetChange {
	if n > 0 && len(in) > n {
		return in[:n]
	}
	return in
}

// collectFleetComputers fingerprints each computer once, from the merged
// record immyLoadComputers builds over every roster resource. Default sync
// mirrors GET /api/v1/computers (ComputerSearch.online, a boolean) and
// /computers/paged (ComputerListViewModel.isOnline, nullable) for the same
// machine; fingerprinting each row under one key let the last one read
// overwrite the other, so a null hid a machine going offline. Beside the merged
// fields the fingerprint carries every source value that disagrees with them,
// so a change in any one source is a change whichever source the merge
// prefers, while a source that only repeats the merged record changes nothing.
// It returns the count of rows that carry no computer id and so cannot be
// tracked.
func collectFleetComputers(ctx context.Context, db *store.Store, kind string, resourceTypes []string, out map[string]store.ImmyFleetRow) (int, error) {
	roster, err := immyLoadComputers(ctx, db, resourceTypes)
	if err != nil {
		return 0, fmt.Errorf("reading %s state: %w", kind, err)
	}
	for _, c := range roster.list() {
		merged := []string{c.Name, c.TenantID, c.Online, c.Excluded}
		var disagree []string
		for _, s := range roster.sources(c.ID) {
			for i, v := range []string{s.ref.Name, s.ref.TenantID, s.ref.Online, s.ref.Excluded} {
				if strings.TrimSpace(v) != "" && v != merged[i] {
					disagree = append(disagree, fmt.Sprintf("%s\x01%d\x01%s", s.source, i, v))
				}
			}
		}
		slices.Sort(disagree)
		sum := sha256.Sum256([]byte(strings.Join(append(merged, slices.Compact(disagree)...), "\x00")))
		row := store.ImmyFleetRow{ResourceType: kind, ID: c.ID, Fingerprint: hex.EncodeToString(sum[:]), Label: c.Name}
		out[row.ResourceType+"\x00"+row.ID] = row
	}
	return roster.Unidentified, nil
}
