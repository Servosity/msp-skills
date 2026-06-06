// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"
	"syncro-pp-cli/internal/store"
)

// snapshotSources lists the tables snapshot diff captures. resources is keyed
// by resource_type; invoices and customer_assets are typed tables captured
// under a synthetic resource_type label.
var snapshotTypedSources = []struct {
	table string
	label string
}{
	{table: "invoices", label: "invoices"},
	{table: "customer_assets", label: "customer_assets"},
}

// pp:data-source local
func newNovelSnapshotDiffCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var flagSince string
	var captureOnly bool

	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Diff two retained local sync snapshots across all entities.",
		Long: `Capture a content-hash snapshot of the local store, then diff it against the
most recent prior snapshot older than --since, reporting added/removed/changed
counts per resource type. The first run records a baseline and exits.`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return nil
			}
			if dbPath == "" {
				dbPath = defaultDBPath("syncro-cli")
			}
			sinceDur := 7 * 24 * time.Hour
			if flagSince != "" {
				d, err := parseAgeDuration(flagSince)
				if err != nil {
					return usageErr(fmt.Errorf("invalid --since: %w", err))
				}
				sinceDur = d
			}
			db, err := store.OpenWithContext(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local database: %w", err)
			}
			defer db.Close()

			if !hintIfUnsynced(cmd, db, "") {
				hintIfStale(cmd, db, "", flags.maxAge)
			}

			sdb := db.DB()
			if _, err := sdb.Exec(`CREATE TABLE IF NOT EXISTS snapshots (
				captured_at DATETIME NOT NULL,
				resource_type TEXT NOT NULL,
				id TEXT NOT NULL,
				content_hash TEXT NOT NULL
			)`); err != nil {
				return fmt.Errorf("creating snapshots table: %w", err)
			}

			now := time.Now().UTC()
			nowStr := now.Format(time.RFC3339Nano)

			// Capture current state.
			current, err := captureSnapshot(sdb, nowStr)
			if err != nil {
				return fmt.Errorf("capturing snapshot: %w", err)
			}

			if captureOnly {
				if flags.asJSON {
					enc := json.NewEncoder(cmd.OutOrStdout())
					enc.SetIndent("", "  ")
					return enc.Encode(map[string]any{
						"captured_at": nowStr,
						"captured":    len(current),
						"note":        "snapshot captured (capture-only)",
					})
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Captured %d rows at %s.\n", len(current), nowStr)
				return nil
			}

			// Find the most recent prior capture older than --since.
			cutoff := now.Add(-sinceDur).Format(time.RFC3339Nano)
			var priorTS sql.NullString
			err = sdb.QueryRow(
				`SELECT captured_at FROM snapshots WHERE captured_at < ? ORDER BY captured_at DESC LIMIT 1`,
				cutoff,
			).Scan(&priorTS)
			if err != nil && err != sql.ErrNoRows {
				return fmt.Errorf("finding prior snapshot: %w", err)
			}

			if !priorTS.Valid {
				if flags.asJSON {
					enc := json.NewEncoder(cmd.OutOrStdout())
					enc.SetIndent("", "  ")
					return enc.Encode(map[string]any{
						"baseline_captured": true,
						"captured_at":       nowStr,
						"note":              "baseline captured; run again later to see a diff",
					})
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Baseline captured at %s; run again later to see a diff.\n", nowStr)
				return nil
			}

			prior, err := loadSnapshot(sdb, priorTS.String)
			if err != nil {
				return fmt.Errorf("loading prior snapshot: %w", err)
			}

			changes, totalAdded, totalRemoved, totalChanged := diffSnapshots(prior, current)

			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{
					"from":          priorTS.String,
					"to":            nowStr,
					"changes":       changes,
					"total_added":   totalAdded,
					"total_removed": totalRemoved,
					"total_changed": totalChanged,
				})
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Diff from %s to %s\n\n", priorTS.String, nowStr)
			if len(changes) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No changes.")
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-22s %-8s %-8s %s\n", "RESOURCE_TYPE", "ADDED", "REMOVED", "CHANGED")
			for _, c := range changes {
				fmt.Fprintf(cmd.OutOrStdout(), "%-22s %-8d %-8d %d\n", c.ResourceType, c.Added, c.Removed, c.Changed)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\nTotals: +%d / -%d / ~%d\n", totalAdded, totalRemoved, totalChanged)
			return nil
		},
	}

	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/syncro-cli/data.db)")
	cmd.Flags().StringVar(&flagSince, "since", "7d", "Diff against the most recent snapshot older than this (e.g. 7d, 48h)")
	cmd.Flags().BoolVar(&captureOnly, "capture-only", false, "Capture a snapshot and exit without diffing")
	return cmd
}

// snapshotEntry uniquely identifies a captured row.
type snapshotKey struct {
	resourceType string
	id           string
}

// contentHash16 returns the first 16 hex chars of the SHA-256 of data.
func contentHash16(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])[:16]
}

// captureSnapshot inserts a snapshot row for every resources row plus the
// typed invoices/customer_assets tables, stamped with capturedAt. Returns the
// in-memory map of what was captured so the caller can diff without re-reading.
func captureSnapshot(sdb *sql.DB, capturedAt string) (map[snapshotKey]string, error) {
	current := map[snapshotKey]string{}

	insert := func(resourceType, id string, data []byte) error {
		h := contentHash16(data)
		current[snapshotKey{resourceType, id}] = h
		_, err := sdb.Exec(
			`INSERT INTO snapshots (captured_at, resource_type, id, content_hash) VALUES (?, ?, ?, ?)`,
			capturedAt, resourceType, id, h,
		)
		return err
	}

	rows, err := sdb.Query(`SELECT resource_type, id, data FROM resources`)
	if err == nil {
		for rows.Next() {
			var rt, id string
			var data []byte
			if err := rows.Scan(&rt, &id, &data); err != nil {
				continue
			}
			if err := insert(rt, id, data); err != nil {
				_ = rows.Close()
				return nil, err
			}
		}
		_ = rows.Close()
	}

	for _, src := range snapshotTypedSources {
		trows, err := sdb.Query(fmt.Sprintf(`SELECT id, data FROM %s`, src.table))
		if err != nil {
			continue
		}
		for trows.Next() {
			var id string
			var data []byte
			if err := trows.Scan(&id, &data); err != nil {
				continue
			}
			if err := insert(src.label, id, data); err != nil {
				_ = trows.Close()
				return nil, err
			}
		}
		_ = trows.Close()
	}

	return current, nil
}

// loadSnapshot reads a prior snapshot identified by its captured_at timestamp.
func loadSnapshot(sdb *sql.DB, capturedAt string) (map[snapshotKey]string, error) {
	out := map[snapshotKey]string{}
	rows, err := sdb.Query(
		`SELECT resource_type, id, content_hash FROM snapshots WHERE captured_at = ?`,
		capturedAt,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var rt, id, h string
		if err := rows.Scan(&rt, &id, &h); err != nil {
			continue
		}
		out[snapshotKey{rt, id}] = h
	}
	return out, rows.Err()
}

type snapshotChange struct {
	ResourceType string `json:"resource_type"`
	Added        int    `json:"added"`
	Removed      int    `json:"removed"`
	Changed      int    `json:"changed"`
}

// diffSnapshots computes per-resource_type added/removed/changed counts.
func diffSnapshots(prior, current map[snapshotKey]string) (changes []snapshotChange, totalAdded, totalRemoved, totalChanged int) {
	perType := map[string]*snapshotChange{}
	ensure := func(rt string) *snapshotChange {
		c := perType[rt]
		if c == nil {
			c = &snapshotChange{ResourceType: rt}
			perType[rt] = c
		}
		return c
	}

	for k, curHash := range current {
		priorHash, ok := prior[k]
		if !ok {
			ensure(k.resourceType).Added++
			totalAdded++
		} else if priorHash != curHash {
			ensure(k.resourceType).Changed++
			totalChanged++
		}
	}
	for k := range prior {
		if _, ok := current[k]; !ok {
			ensure(k.resourceType).Removed++
			totalRemoved++
		}
	}

	for _, c := range perType {
		changes = append(changes, *c)
	}
	sort.Slice(changes, func(i, j int) bool {
		return changes[i].ResourceType < changes[j].ResourceType
	})
	return changes, totalAdded, totalRemoved, totalChanged
}
