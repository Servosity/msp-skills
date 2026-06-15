// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"quickbooks-pp-cli/internal/analytics"
	"quickbooks-pp-cli/internal/store"
)

// ensureAgingSnapshotsTable creates the aging-delta snapshot table on demand.
// This table is private to the aging-delta novel command and has no store
// migration, so a fresh (or freshly reprinted) DB lacks it; without this, the
// first aging-delta run fails on "no such table: aging_snapshots" instead of
// recording a baseline. CREATE IF NOT EXISTS is idempotent and cheap. Recorded
// hand-fix `qbo-aging-snapshots-table` in handfixes.json.
func ensureAgingSnapshotsTable(ctx context.Context, db *store.Store) error {
	_, err := db.DB().ExecContext(ctx,
		`CREATE TABLE IF NOT EXISTS aging_snapshots (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			taken_at TEXT NOT NULL,
			payload TEXT NOT NULL
		)`)
	if err != nil {
		return fmt.Errorf("ensuring aging_snapshots table: %w", err)
	}
	return nil
}

// loadLatestAgingSnapshot returns the most recent persisted snapshot, or
// (nil, nil) when none exists yet (first run).
func loadLatestAgingSnapshot(ctx context.Context, db *store.Store) (*analytics.AgingSnapshot, error) {
	if err := ensureAgingSnapshotsTable(ctx, db); err != nil {
		return nil, err
	}
	var payload string
	err := db.DB().QueryRowContext(ctx,
		`SELECT payload FROM aging_snapshots ORDER BY taken_at DESC, id DESC LIMIT 1`).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("loading prior aging snapshot: %w", err)
	}
	var snap analytics.AgingSnapshot
	if err := json.Unmarshal([]byte(payload), &snap); err != nil {
		return nil, fmt.Errorf("decoding prior aging snapshot: %w", err)
	}
	return &snap, nil
}

// saveAgingSnapshot persists the current snapshot for the next delta run.
func saveAgingSnapshot(ctx context.Context, db *store.Store, snap analytics.AgingSnapshot) error {
	payload, err := json.Marshal(snap)
	if err != nil {
		return fmt.Errorf("encoding aging snapshot: %w", err)
	}
	if _, err := db.DB().ExecContext(ctx,
		`INSERT INTO aging_snapshots (taken_at, payload) VALUES (?, ?)`,
		snap.TakenAt, string(payload)); err != nil {
		return fmt.Errorf("saving aging snapshot: %w", err)
	}
	return nil
}

// agingDeltaView wraps the delta report with first-run context for agents.
type agingDeltaView struct {
	FirstRun bool                        `json:"first_run"`
	Note     string                      `json:"note,omitempty"`
	Report   *analytics.AgingDeltaReport `json:"report,omitempty"`
	Snapshot *analytics.AgingSnapshot    `json:"snapshot,omitempty"` // first run only: the baseline just recorded
}

// pp:data-source local
func newNovelAgingDeltaCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var noSave bool

	cmd := &cobra.Command{
		Use:   "aging-delta",
		Short: "What changed in AR/AP aging since the previous snapshot",
		Long: "Diff the current AR and AP aging picture against the snapshot recorded by the\n" +
			"previous aging-delta run: who slipped an aging bucket, whose balance grew or\n" +
			"shrank, who appeared or cleared. QuickBooks reports are point-in-time with no\n" +
			"memory; the snapshot history lives in the local store. Each run records a new\n" +
			"snapshot unless --no-save is set.\n" +
			"Use this command to see what changed in aging since the previous snapshot (who\n" +
			"slipped a bucket). Do NOT use it for the current point-in-time aging picture;\n" +
			"use 'ar-aging' or 'ap-aging' instead. Run `sync` first.",
		Example:     "  quickbooks-cli aging-delta --agent\n  quickbooks-cli aging-delta --no-save --json | jq '.report.changes'",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			ctx := cmd.Context()
			db, err := openLocalStore(ctx, dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "invoices") {
				hintIfStale(cmd, db, "invoices", flags.maxAge)
			}
			invoices, err := loadResources(ctx, db, "invoices")
			if err != nil {
				return err
			}
			bills, err := loadResources(ctx, db, "bills")
			if err != nil {
				return err
			}
			now := time.Now()
			curr := analytics.AgingSnapshot{
				TakenAt: now.UTC().Format(time.RFC3339),
				AR:      analytics.Aging(invoices, "DueDate", "TxnDate", "Balance", "CustomerRef", now),
				AP:      analytics.Aging(bills, "DueDate", "TxnDate", "Balance", "VendorRef", now),
			}
			prev, err := loadLatestAgingSnapshot(ctx, db)
			if err != nil {
				return err
			}
			view := agingDeltaView{}
			if prev == nil {
				view.FirstRun = true
				view.Note = "no prior snapshot — baseline recorded; run aging-delta again after the next sync to see changes"
				view.Snapshot = &curr
			} else {
				rep := analytics.AgingDelta(*prev, curr)
				view.Report = &rep
			}
			if !noSave {
				if err := saveAgingSnapshot(ctx, db, curr); err != nil {
					return err
				}
			}
			return flags.printJSON(cmd, view)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Path to the local SQLite store (default: per-CLI data dir)")
	cmd.Flags().BoolVar(&noSave, "no-save", false, "Compute the delta without recording a new snapshot")
	return cmd
}
