// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"comet-backup-pp-cli/internal/comet"
	"comet-backup-pp-cli/internal/store"
)

type changesView struct {
	FromSnapshot int64          `json:"from_snapshot,omitempty"`
	ToSnapshot   int64          `json:"to_snapshot,omitempty"`
	FromTakenAt  string         `json:"from_taken_at,omitempty"`
	ToTakenAt    string         `json:"to_taken_at,omitempty"`
	Changes      []comet.Change `json:"changes"`
	Note         string         `json:"note,omitempty"`
}

func newNovelChangesCmd(flags *rootFlags) *cobra.Command {
	var flagSince, flagOnly, flagDB string

	cmd := &cobra.Command{
		Use:   "changes",
		Short: "Compare the two newest local snapshots: problems opened or resolved, devices and items added or removed",
		Long: `Compare the two newest local snapshots: problems opened or resolved, devices and items added or removed.

Use this command for what changed between two local fleet snapshots: problems opened or resolved, devices and protected items added or removed, storage growth. Do NOT use this command for the current state; use 'health' instead.

Compares the newest snapshot with the previous one, or with the newest snapshot at least --since old. Keys are stable (tenant/item-id, tenant/device-id, tenant/vault-id), so a ticketing script can open a ticket on problem_opened and close it on problem_resolved. Reads only the local store; run 'snapshot' first.`,
		Example: strings.Trim(`
  comet-backup-cli changes --only health --agent
  comet-backup-cli changes --since 7d --only inventory --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: guardRunE(flags, func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "changes")
			}
			if flagOnly != "" && flagOnly != "health" && flagOnly != "inventory" {
				return usageErr(fmt.Errorf("--only must be health or inventory, got %q", flagOnly))
			}
			var cutoff int64
			if flagSince != "" {
				since, err := durationFlag("since", flagSince)
				if err != nil {
					return err
				}
				cutoff = time.Now().Add(-since).Unix()
			}
			dbPath := flagDB
			if dbPath == "" {
				dbPath = defaultDBPath(cometDBName)
			}
			view := changesView{Changes: make([]comet.Change, 0)}
			if _, statErr := os.Stat(dbPath); os.IsNotExist(statErr) {
				fmt.Fprintf(cmd.ErrOrStderr(), "no local snapshots at %s\nrun: comet-backup-cli snapshot\n", dbPath)
				view.Note = "no snapshots yet; run 'comet-backup-cli snapshot' twice (for example on a schedule) to compare"
				return printChanges(cmd, flags, view)
			}
			db, err := store.OpenReadOnlyContext(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local store %s: %w", dbPath, err)
			}
			defer db.Close()
			latest, base, shortHistory, err := loadSnapshots(cmd.Context(), db.DB(), cutoff)
			if err != nil {
				return err
			}
			switch {
			case latest == nil:
				view.Note = "no snapshots yet; run 'comet-backup-cli snapshot' to start the history"
			case base == nil:
				view.ToSnapshot, view.ToTakenAt = latest.ID, fmtTime(latest.Snap.TakenAt)
				view.Note = "only one snapshot; run 'comet-backup-cli snapshot' again to compare"
			default:
				view.FromSnapshot, view.FromTakenAt = base.ID, fmtTime(base.Snap.TakenAt)
				view.ToSnapshot, view.ToTakenAt = latest.ID, fmtTime(latest.Snap.TakenAt)
				if shortHistory {
					view.Note = fmt.Sprintf("no snapshot is %s old yet; compared with the oldest snapshot (%s), so this covers a shorter window", flagSince, view.FromTakenAt)
					fmt.Fprintln(cmd.ErrOrStderr(), "warning: "+view.Note)
				}
				for _, ch := range comet.Diff(base.Snap, latest.Snap) {
					if flagOnly == "" || ch.Category == flagOnly {
						view.Changes = append(view.Changes, ch)
					}
				}
			}
			if latest != nil && flags.maxAge > 0 {
				if age := time.Since(time.Unix(latest.Snap.TakenAt, 0)); age > flags.maxAge {
					fmt.Fprintf(cmd.ErrOrStderr(), "hint: newest snapshot is %s old; run: comet-backup-cli snapshot\n", age.Round(time.Minute))
				}
			}
			return printChanges(cmd, flags, view)
		}),
	}
	cmd.Flags().StringVar(&flagSince, "since", "", "Compare against the newest snapshot at least this old (e.g. 24h, 7d); default is the previous snapshot")
	cmd.Flags().StringVar(&flagOnly, "only", "", "Only health changes (problems, online state) or inventory changes (tenants, devices, items, storage)")
	cmd.Flags().StringVar(&flagDB, "db", "", "SQLite database path (default: the CLI's local data directory)")
	return cmd
}

func printChanges(cmd *cobra.Command, flags *rootFlags, view changesView) error {
	if !wantsHumanTable(cmd.OutOrStdout(), flags) {
		return printJSONFilteredKeep(cmd.OutOrStdout(), view, flags, changeAgentKeep...)
	}
	if len(view.Changes) == 0 {
		msg := "No changes."
		if view.Note != "" {
			msg = view.Note
		}
		fmt.Fprintln(cmd.OutOrStdout(), msg)
		return nil
	}
	rows := make([][]string, 0, len(view.Changes))
	for _, ch := range view.Changes {
		before, after := ch.Before, ch.After
		if ch.Kind == "vault_size_changed" {
			before, after = humanByteString(before), humanByteString(after)
		}
		rows = append(rows, []string{ch.Kind, ch.Tenant, ch.Device, ch.Item, ch.Vault, before, after, ch.Detail})
	}
	return printCleanTable(cmd, flags, []string{"KIND", "TENANT", "DEVICE", "ITEM", "VAULT", "BEFORE", "AFTER", "DETAIL"}, rows)
}

// humanByteString renders a decimal byte count for the human table.
func humanByteString(v string) string {
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return v
	}
	return humanBytes(n)
}
