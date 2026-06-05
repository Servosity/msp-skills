// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// pp:data-source local

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"mspbots-pp-cli/internal/store"
)

// diffView reports row-level movement between two snapshots of one resource.
type diffView struct {
	Alias        string             `json:"alias"`
	FromSnapshot store.SnapshotMeta `json:"from_snapshot"`
	ToSnapshot   store.SnapshotMeta `json:"to_snapshot"`
	AddedCount   int                `json:"added_count"`
	RemovedCount int                `json:"removed_count"`
	ChangedCount int                `json:"changed_count"`
	Added        []json.RawMessage  `json:"added"`
	Removed      []json.RawMessage  `json:"removed"`
	Changed      []json.RawMessage  `json:"changed"`
	Note         string             `json:"note,omitempty"`
}

func newNovelDiffCmd(flags *rootFlags) *cobra.Command {
	var fromID int64
	var toID int64
	var dbPath string

	cmd := &cobra.Command{
		Use:   "diff [alias]",
		Short: "Row-level added/removed/changed comparison between two stored snapshots of the same resource",
		Long: strings.Trim(`
Use this command to compare two stored snapshots of the same resource row-by-row.
Do NOT use it for numeric KPI movement over time; use 'trend' instead.

By default compares the two most recent snapshots of the alias. Row identity
prefers an id-shaped field when rows carry one (enabling changed-row
detection); rows without ids are matched by content hash (added/removed only).
Capture snapshots first with 'snapshot'.`, "\n"),
		Example: strings.Trim(`
  mspbots-cli diff open-tickets --json
  mspbots-cli diff open-tickets --from 3 --to 7 --json`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
			"pp:happy-args": "alias=example-alias",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if len(args) < 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("diff needs <alias>"))
			}
			if dryRunOK(flags) {
				fmt.Fprintf(cmd.OutOrStdout(), "would diff the two most recent snapshots of %q\n", args[0])
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return usageErr(err)
			}
			db, err := openMspbotsStore(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			alias := args[0]
			snaps, err := db.SnapshotsForAlias(cmd.Context(), alias, 100)
			if err != nil {
				return fmt.Errorf("listing snapshots: %w", err)
			}
			var fromMeta, toMeta *store.SnapshotMeta
			if fromID > 0 || toID > 0 {
				if fromID <= 0 || toID <= 0 {
					return usageErr(fmt.Errorf("--from and --to must be passed together (snapshot IDs from prior runs)"))
				}
				for i := range snaps {
					if snaps[i].ID == fromID {
						fromMeta = &snaps[i]
					}
					if snaps[i].ID == toID {
						toMeta = &snaps[i]
					}
				}
				if fromMeta == nil || toMeta == nil {
					return notFoundErr(fmt.Errorf("snapshot id %d or %d not found for alias %q", fromID, toID, alias))
				}
			} else {
				if len(snaps) < 2 {
					return notFoundErr(fmt.Errorf("alias %q has %d snapshot(s); need at least 2 — run 'mspbots-cli snapshot %s' again later", alias, len(snaps), alias))
				}
				toMeta = &snaps[0]
				fromMeta = &snaps[1]
			}
			fromKeys, fromRows, err := db.SnapshotRows(cmd.Context(), fromMeta.ID)
			if err != nil {
				return fmt.Errorf("loading snapshot %d: %w", fromMeta.ID, err)
			}
			toKeys, toRows, err := db.SnapshotRows(cmd.Context(), toMeta.ID)
			if err != nil {
				return fmt.Errorf("loading snapshot %d: %w", toMeta.ID, err)
			}
			added, removed, changed := diffSnapshots(fromKeys, fromRows, toKeys, toRows)
			view := diffView{
				Alias:        alias,
				FromSnapshot: *fromMeta,
				ToSnapshot:   *toMeta,
				AddedCount:   len(added),
				RemovedCount: len(removed),
				ChangedCount: len(changed),
				Added:        added,
				Removed:      removed,
				Changed:      changed,
			}
			if view.AddedCount == 0 && view.RemovedCount == 0 && view.ChangedCount == 0 {
				view.Note = "no row-level differences between the two snapshots"
			}
			return flags.printJSON(cmd, view)
		},
	}
	cmd.Flags().Int64Var(&fromID, "from", 0, "Older snapshot ID (default: second-most-recent)")
	cmd.Flags().Int64Var(&toID, "to", 0, "Newer snapshot ID (default: most recent)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (defaults to the CLI's local store)")
	return cmd
}

// diffSnapshots computes row movement by identity key. id-keyed rows present
// on both sides with different content are "changed"; hash-keyed rows can
// only ever be added or removed (the hash IS the content).
func diffSnapshots(fromKeys []string, fromRows []json.RawMessage, toKeys []string, toRows []json.RawMessage) (added, removed, changed []json.RawMessage) {
	added = make([]json.RawMessage, 0)
	removed = make([]json.RawMessage, 0)
	changed = make([]json.RawMessage, 0)
	fromByKey := make(map[string]json.RawMessage, len(fromKeys))
	for i, k := range fromKeys {
		fromByKey[k] = fromRows[i]
	}
	seen := make(map[string]bool, len(toKeys))
	for i, k := range toKeys {
		seen[k] = true
		old, ok := fromByKey[k]
		switch {
		case !ok:
			added = append(added, toRows[i])
		case strings.HasPrefix(k, "id:") && !bytes.Equal(canonicalRowJSON(old), canonicalRowJSON(toRows[i])):
			changed = append(changed, toRows[i])
		}
	}
	for i, k := range fromKeys {
		if !seen[k] {
			removed = append(removed, fromRows[i])
		}
	}
	return added, removed, changed
}
