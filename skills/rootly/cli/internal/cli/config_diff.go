// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Transcendence feature (hand-authored, Phase 3): config drift detection.
// Snapshots the synced config objects (services, escalation policies, workflows,
// severities, schedules, ...) into a local table and diffs the latest snapshot
// against the current store — added / removed / changed — without terraform plan
// or any API round-trip.

package cli

import (
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"
)

// novelConfigTypes are the config-as-code resource families config-diff tracks.
// Each entry lists tolerated resource_type spellings; the first synced one wins.
var novelConfigTypes = [][]string{
	{"services"},
	{"functionalities"},
	{"environments"},
	{"escalation-policies", "escalation_policies"},
	{"workflows"},
	{"workflow-groups", "workflow_groups"},
	{"severities"},
	{"schedules"},
	{"statuses"},
	{"sub-statuses", "sub_statuses"},
	{"causes"},
	{"slas"},
}

// pp:data-source local
func newNovelConfigDiffCmd(flags *rootFlags) *cobra.Command {
	var save bool
	var dbPath string

	cmd := &cobra.Command{
		Use:   "config-diff",
		Short: "Diff Rootly config objects against the last saved snapshot to catch drift.",
		Long: `Catch config drift without terraform plan. 'config-diff --save' records a
snapshot of the synced config objects (services, escalation policies,
workflows, severities, schedules, statuses, causes, SLAs). A later plain
'config-diff' compares the current store against the latest snapshot and
reports what was added, removed, or changed — by stable id, offline.

Run 'rootly-cli sync' before both steps so the store reflects live config.`,
		Example: `  rootly-cli config-diff --save
  rootly-cli config-diff --json`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			db, err := novelOpenStoreChecked(cmd, flags, dbPath, "")
			if err != nil {
				return err
			}
			defer db.Close()

			type objState struct {
				Type string `json:"type"`
				ID   string `json:"id"`
				Name string `json:"name,omitempty"`
				Hash string `json:"-"`
			}

			// Current state of every tracked config object.
			current := map[string]objState{} // key: type+id
			for _, candidates := range novelConfigTypes {
				rt := novelResolveType(db, candidates...)
				rows, err := novelLoad(db, rt)
				if err != nil {
					continue
				}
				for _, r := range rows {
					st := objState{Type: candidates[0], ID: r.ID, Name: recStr(r.Attrs, "name", "title"), Hash: novelConfigHash(r.Attrs)}
					current[st.Type+"\x00"+st.ID] = st
				}
			}

			if save {
				// Fixed-width nanosecond precision: keeps MAX(taken_at) string ordering
				// chronological and makes same-second --save collisions impossible.
				takenAt := time.Now().UTC().Format("2006-01-02T15:04:05.000000000Z07:00")
				for _, st := range current {
					if _, err := db.DB().Exec(
						`INSERT OR REPLACE INTO novel_config_snapshots (taken_at, resource_type, resource_id, name, hash) VALUES (?, ?, ?, ?, ?)`,
						takenAt, st.Type, st.ID, st.Name, st.Hash); err != nil {
						return fmt.Errorf("saving snapshot: %w", err)
					}
				}
				out := struct {
					Saved   int    `json:"saved_objects"`
					TakenAt string `json:"taken_at"`
				}{Saved: len(current), TakenAt: takenAt}
				return novelEmit(cmd, flags, out, func() {
					fmt.Fprintf(cmd.OutOrStdout(), "snapshot saved: %d config objects at %s\n", out.Saved, out.TakenAt)
				})
			}

			// Latest snapshot.
			var latest string
			if err := db.DB().QueryRow(`SELECT COALESCE(MAX(taken_at), '') FROM novel_config_snapshots`).Scan(&latest); err != nil {
				return fmt.Errorf("reading snapshots: %w", err)
			}
			snap := map[string]objState{}
			if latest != "" {
				rows, err := db.DB().Query(`SELECT resource_type, resource_id, COALESCE(name, ''), hash FROM novel_config_snapshots WHERE taken_at = ?`, latest)
				if err != nil {
					return fmt.Errorf("reading snapshot %s: %w", latest, err)
				}
				defer rows.Close()
				for rows.Next() {
					var st objState
					if rows.Scan(&st.Type, &st.ID, &st.Name, &st.Hash) != nil {
						continue
					}
					snap[st.Type+"\x00"+st.ID] = st
				}
			}

			var added, removed, changed []objState
			for k, st := range current {
				prev, ok := snap[k]
				switch {
				case !ok:
					added = append(added, st)
				case prev.Hash != st.Hash:
					changed = append(changed, st)
				}
			}
			for k, st := range snap {
				if _, ok := current[k]; !ok {
					removed = append(removed, st)
				}
			}
			for _, s := range [][]objState{added, removed, changed} {
				sort.Slice(s, func(i, j int) bool {
					if s[i].Type == s[j].Type {
						return s[i].ID < s[j].ID
					}
					return s[i].Type < s[j].Type
				})
			}
			ensure := func(s []objState) []objState {
				if s == nil {
					return []objState{}
				}
				return s
			}

			note := ""
			if latest == "" {
				note = "no snapshot recorded yet — run 'rootly-cli config-diff --save' after a sync to set the baseline"
			}
			out := struct {
				SnapshotTakenAt string     `json:"snapshot_taken_at,omitempty"`
				TrackedObjects  int        `json:"tracked_objects"`
				Added           []objState `json:"added"`
				Removed         []objState `json:"removed"`
				Changed         []objState `json:"changed"`
				Note            string     `json:"note,omitempty"`
			}{SnapshotTakenAt: latest, TrackedObjects: len(current), Added: ensure(added), Removed: ensure(removed), Changed: ensure(changed), Note: note}

			return novelEmit(cmd, flags, out, func() {
				w := cmd.OutOrStdout()
				if latest == "" {
					fmt.Fprintf(w, "config-diff: %s\n", note)
					return
				}
				fmt.Fprintf(w, "config drift vs snapshot %s (%d objects tracked)\n", latest, len(current))
				section := func(label string, s []objState) {
					if len(s) == 0 {
						return
					}
					fmt.Fprintf(w, "  %s (%d):\n", label, len(s))
					for _, st := range s {
						fmt.Fprintf(w, "    %-22s %-30s %s\n", st.Type, st.Name, st.ID)
					}
				}
				section("added", out.Added)
				section("removed", out.Removed)
				section("changed", out.Changed)
				if len(out.Added)+len(out.Removed)+len(out.Changed) == 0 {
					fmt.Fprintln(w, "  no drift — current config matches the snapshot")
				}
			})
		},
	}
	cmd.Flags().BoolVar(&save, "save", false, "Record the current config state as the new baseline snapshot")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/rootly-cli/data.db)")
	return cmd
}
