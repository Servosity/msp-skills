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
	Baseline      string        `json:"baseline_snapshot,omitempty"`
	Compared      string        `json:"compared_against"`
	Since         string        `json:"since,omitempty"`
	TrackedTypes  []string      `json:"tracked_resource_types"`
	CurrentCount  int           `json:"current_objects"`
	BaselineCount int           `json:"baseline_objects"`
	Added         []fleetChange `json:"added"`
	Removed       []fleetChange `json:"removed"`
	Changed       []fleetChange `json:"changed"`
	SnapshotTaken string        `json:"snapshot_taken,omitempty"`
	Note          string        `json:"note,omitempty"`
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
}{
	{
		resourceTypes: []string{"computers", "computers-paged", "computers-dx"},
		kind:          "computer",
		labelField:    "$.name",
		fields:        []string{"$.name", "$.tenantId", "$.online", "$.excludeFromMaintenance"},
	},
	{
		resourceTypes: []string{"tenants-software-from-inventory-dx"},
		kind:          "software-install",
		labelField:    "$.softwareName",
		fields:        []string{"$.softwareName", "$.version", "$.computerName", "$.tenantName"},
	},
	{
		resourceTypes: []string{"target-assignments", "target-assignments-global"},
		kind:          "deployment",
		labelField:    "$.targetName",
		fields:        []string{"$.maintenanceIdentifier", "$.targetName", "$.targetText", "$.excluded", "$.targetEnforcement"},
	},
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
			"runs against it.\n\n" +
			"Use this command for what changed in the fleet between two syncs. Do NOT use this " +
			"command for grouping the failures a maintenance window produced; use " +
			"'session-triage' instead.",
		Example: strings.Trim(`
  immybot-cli fleet-diff --snapshot
  immybot-cli fleet-diff --since 24h
  immybot-cli fleet-diff --since 7d --agent
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
			"pp:happy-args": "--since=24h",
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
			db, err := store.OpenWithContext(ctx, dbPath)
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "computers") {
				hintIfStale(cmd, db, "computers", flags.maxAge)
			}

			current, err := collectFleetState(ctx, db)
			if err != nil {
				return err
			}
			view.CurrentCount = len(current)

			if flagSnapshot {
				takenAt := time.Now().UTC().Format(time.RFC3339)
				rows := make([]store.ImmyFleetRow, 0, len(current))
				for _, r := range current {
					rows = append(rows, r)
				}
				if err := db.WriteImmyFleetSnapshot(ctx, takenAt, rows); err != nil {
					return fmt.Errorf("writing fleet snapshot: %w", err)
				}
				view.SnapshotTaken = takenAt
				view.Compared = "none (snapshot only)"
				view.Note = fmt.Sprintf("recorded snapshot of %d object(s) at %s", len(rows), takenAt)
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

			// Prefer the newest snapshot at least --since old; fall back to the
			// oldest available so a too-recent baseline still produces output.
			baseline := times[len(times)-1]
			if !cutoff.IsZero() {
				for _, t := range times {
					if ts, err := time.Parse(time.RFC3339, t); err == nil && ts.Before(cutoff) {
						baseline = t
						break
					}
				}
			} else {
				baseline = times[0]
			}
			view.Baseline = baseline
			view.Compared = "current local mirror"

			prev, err := db.ReadImmyFleetSnapshot(ctx, baseline)
			if err != nil {
				return fmt.Errorf("reading fleet snapshot %s: %w", baseline, err)
			}
			view.BaselineCount = len(prev)

			for key, cur := range current {
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
			return nil
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "", "Compare against the newest snapshot at least this old (e.g. 24h, 7d)")
	cmd.Flags().BoolVar(&flagSnapshot, "snapshot", false, "Record the current mirror state as a new baseline instead of diffing")
	cmd.Flags().IntVar(&flagLimit, "limit", 200, "Maximum rows per change list (0 for all)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local mirror database path")
	return cmd
}

// collectFleetState fingerprints every tracked object in the current mirror.
// Each resource group is drained fully before the next query runs.
func collectFleetState(ctx context.Context, db *store.Store) (map[string]store.ImmyFleetRow, error) {
	out := map[string]store.ImmyFleetRow{}
	for _, tracked := range fleetTracked {
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
			return nil, fmt.Errorf("querying %s state: %w", tracked.kind, err)
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
				return nil, fmt.Errorf("scanning %s state: %w", tracked.kind, err)
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
			return nil, fmt.Errorf("iterating %s state: %w", tracked.kind, err)
		}
		if err := rows.Close(); err != nil {
			return nil, fmt.Errorf("closing %s state: %w", tracked.kind, err)
		}
	}
	return out, nil
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
