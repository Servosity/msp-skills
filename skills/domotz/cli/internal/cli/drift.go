// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel feature. Config/inventory drift for a site: diffs the
// current synced device state against the previously-captured snapshot to
// surface what changed (added/removed devices, status changes) over time — a
// cross-time compare the API cannot do in a single call.

package cli

import (
	"context"
	"domotz-pp-cli/internal/store"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

// driftDeviceState is the per-device snapshot record. Status (and the
// display-name label) drive the diff today; Type and FirstSeenOn are stored
// for forward-compat so adding diff dimensions later won't break old snapshots.
type driftDeviceState struct {
	DisplayName string `json:"display_name"`
	Status      string `json:"status"`
	Type        string `json:"type"`
	FirstSeenOn string `json:"first_seen_on"`
}

type driftChange struct {
	DeviceID    string `json:"device_id"`
	DisplayName string `json:"display_name"`
	From        string `json:"from,omitempty"`
	To          string `json:"to,omitempty"`
}

type driftReport struct {
	AgentID       string        `json:"agent_id"`
	Baseline      bool          `json:"baseline"`
	Message       string        `json:"message,omitempty"`
	Devices       int           `json:"devices"`
	Added         []driftChange `json:"added"`
	Removed       []driftChange `json:"removed"`
	StatusChanged []driftChange `json:"status_changed"`
}

const driftSnapshotDDL = `
CREATE TABLE IF NOT EXISTS "drift_snapshot" (
	"agent_id"    TEXT NOT NULL,
	"captured_at" DATETIME DEFAULT CURRENT_TIMESTAMP,
	"snapshot"    JSON NOT NULL
)`

// pp:data-source local
func newNovelDriftCmd(flags *rootFlags) *cobra.Command {
	var flagAgent string
	var dbPath string
	var baseline bool

	cmd := &cobra.Command{
		Use:   "drift [agent-id]",
		Short: "Diff a site's devices against its previous snapshot (what changed over time)",
		Long: "Compare the current synced device state for a site against the last captured snapshot " +
			"and report added/removed devices and status changes — useful after a maintenance window. " +
			"The first run captures a baseline; each run updates the snapshot. Reads the local store; " +
			"run 'domotz-cli sync --full' first. Identify the site with a positional agent id or --agent-id.",
		Example:     "  domotz-cli drift --agent-id 12345 --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			agentID := flagAgent
			if agentID == "" && len(args) > 0 {
				agentID = args[0]
			}
			if dryRunOK(flags) {
				return nil
			}
			if agentID == "" {
				return usageErr(fmt.Errorf("an agent id is required (positional <agent-id> or --agent-id)"))
			}

			db, err := novelOpenStoreChecked(cmd, flags, dbPath, "device")
			if err != nil {
				return err
			}
			defer db.Close()
			if _, err := db.DB().ExecContext(cmd.Context(), driftSnapshotDDL); err != nil {
				return fmt.Errorf("preparing drift snapshot table: %w", err)
			}

			current, err := loadDriftState(cmd.Context(), db, agentID)
			if err != nil {
				return err
			}
			prior, hadPrior, err := loadLatestDriftSnapshot(cmd.Context(), db, agentID)
			if err != nil {
				return err
			}

			report := driftReport{
				AgentID:       agentID,
				Devices:       len(current),
				Added:         []driftChange{},
				Removed:       []driftChange{},
				StatusChanged: []driftChange{},
			}

			if !hadPrior || baseline {
				if err := saveDriftSnapshot(cmd.Context(), db, agentID, current); err != nil {
					return err
				}
				report.Baseline = true
				report.Message = fmt.Sprintf("baseline captured for agent %s (%d devices); run again later to see drift", agentID, len(current))
				return printJSONFiltered(cmd.OutOrStdout(), report, flags)
			}

			diffDriftStates(prior, current, &report)
			if err := saveDriftSnapshot(cmd.Context(), db, agentID, current); err != nil {
				return err
			}
			return printJSONFiltered(cmd.OutOrStdout(), report, flags)
		},
	}
	cmd.Flags().StringVar(&flagAgent, "agent-id", "", "Agent (Collector) id to check for drift")
	cmd.Flags().BoolVar(&baseline, "baseline", false, "Capture a fresh baseline snapshot without reporting drift")
	addFleetDBFlag(cmd, &dbPath)
	return cmd
}

// loadDriftState builds the current per-device fingerprint map for an agent.
func loadDriftState(ctx context.Context, db *store.Store, agentID string) (map[string]driftDeviceState, error) {
	rows, err := queryFleetRows(ctx, db, `
SELECT
  id,
  json_extract(data, '$.display_name') AS display_name,
  json_extract(data, '$.status')       AS status,
  json_extract(data, '$.type.label')   AS type,
  json_extract(data, '$.first_seen_on') AS first_seen_on
FROM "device" WHERE agent_id = ?`, agentID)
	if err != nil {
		return nil, err
	}
	state := make(map[string]driftDeviceState, len(rows))
	for _, r := range rows {
		state[asString(r["id"])] = driftDeviceState{
			DisplayName: asString(r["display_name"]),
			Status:      asString(r["status"]),
			Type:        asString(r["type"]),
			FirstSeenOn: asString(r["first_seen_on"]),
		}
	}
	return state, nil
}

// loadLatestDriftSnapshot returns the most recent stored snapshot for an agent.
func loadLatestDriftSnapshot(ctx context.Context, db *store.Store, agentID string) (map[string]driftDeviceState, bool, error) {
	var raw string
	err := db.DB().QueryRowContext(ctx,
		`SELECT snapshot FROM "drift_snapshot" WHERE agent_id = ? ORDER BY captured_at DESC, rowid DESC LIMIT 1`, agentID).Scan(&raw)
	if err != nil {
		// No rows is the baseline case, not an error.
		return nil, false, nil
	}
	var state map[string]driftDeviceState
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return nil, false, fmt.Errorf("parsing stored snapshot: %w", err)
	}
	return state, true, nil
}

// saveDriftSnapshot persists the current state as the new latest snapshot.
func saveDriftSnapshot(ctx context.Context, db *store.Store, agentID string, state map[string]driftDeviceState) error {
	blob, err := json.Marshal(state)
	if err != nil {
		return err
	}
	_, err = db.DB().ExecContext(ctx,
		`INSERT INTO "drift_snapshot" (agent_id, snapshot) VALUES (?, ?)`, agentID, string(blob))
	return err
}

// diffDriftStates fills the report's added/removed/status_changed sets.
func diffDriftStates(prior, current map[string]driftDeviceState, report *driftReport) {
	for id, cur := range current {
		old, ok := prior[id]
		if !ok {
			report.Added = append(report.Added, driftChange{DeviceID: id, DisplayName: cur.DisplayName})
			continue
		}
		if old.Status != cur.Status {
			report.StatusChanged = append(report.StatusChanged, driftChange{
				DeviceID: id, DisplayName: cur.DisplayName, From: old.Status, To: cur.Status,
			})
		}
	}
	for id, old := range prior {
		if _, ok := current[id]; !ok {
			report.Removed = append(report.Removed, driftChange{DeviceID: id, DisplayName: old.DisplayName})
		}
	}
	sortDriftChanges(report.Added)
	sortDriftChanges(report.Removed)
	sortDriftChanges(report.StatusChanged)
}

func sortDriftChanges(c []driftChange) {
	sort.Slice(c, func(i, j int) bool { return c[i].DeviceID < c[j].DeviceID })
}
