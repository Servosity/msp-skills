// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (NOT generated): tactical-rmm handfix alerts-sync-reads-alert-list.

package cli

// Tactical RMM lists alerts with PATCH /alerts/ (amidaware/tacticalrmm
// alerts/views.py GetAddAlerts.patch: an empty body returns every non-hidden
// alert in AlertSerializer shape: id, severity, resolved, snoozed, alert_type,
// alert_time, client, site, hostname, agent_id). The generated sync walks GET
// lists only, so its "alerts" partition holds GET /alerts/templates/, the alert
// TEMPLATES. This step runs after sync and stores the real alerts as resource
// type alerts-list, which the alert reports read.

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"tactical-rmm-pp-cli/internal/store"
)

const trmmAlertsResource = "alerts-list"

// trmmAlertActiveSQL is the vendor's own "active alert" (the dashboard count in
// GetAddAlerts.patch: resolved=False, snoozed=False). Every report uses it.
const trmmAlertActiveSQL = "COALESCE(json_extract(data,'$.resolved'),0)=0 AND COALESCE(json_extract(data,'$.snoozed'),0)=0"

func init() { registerNovelCommand(wrapSyncWithAlertList) }

func wrapSyncWithAlertList(root *cobra.Command, flags *rootFlags) {
	cmd, _, err := root.Find([]string{"sync"})
	if err != nil || cmd == root || cmd.Name() != "sync" || cmd.RunE == nil {
		return
	}
	next := cmd.RunE
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if err := next(cmd, args); err != nil {
			return err
		}
		if dryRunActive(flags, cmd.Root()) || requiredInputWaived() || !syncWantsAlertList(cmd) {
			return nil
		}
		n, err := syncAlertList(cmd, flags)
		if err != nil {
			return apiErr(fmt.Errorf("syncing %s (PATCH /alerts/): %w", trmmAlertsResource, err))
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "synced %d alerts into %s\n", n, trmmAlertsResource)
		return nil
	}
}

// syncWantsAlertList: a plain sync, or one whose --resources names alerts.
func syncWantsAlertList(cmd *cobra.Command) bool {
	resources, err := cmd.Flags().GetStringSlice("resources")
	if err != nil || len(resources) == 0 {
		return true
	}
	for _, r := range resources {
		if strings.TrimSpace(r) == "alerts" {
			return true
		}
	}
	return false
}

// syncAlertList replaces the stored alert list with the server's. An answer
// that is not a list of objects with an id is an error, never an empty list.
func syncAlertList(cmd *cobra.Command, flags *rootFlags) (int, error) {
	c, err := flags.newClient()
	if err != nil {
		return 0, err
	}
	c.NoCache = true
	data, _, err := c.PatchQueryWithParams(cmd.Context(), "/alerts/", nil, map[string]any{})
	if err != nil {
		return 0, err
	}
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(data, &rows); err != nil || rows == nil {
		return 0, fmt.Errorf("the alert list is not a JSON array of alerts: %v", err)
	}
	items := make([]json.RawMessage, 0, len(rows))
	for i, r := range rows {
		if id := strings.TrimSpace(string(r["id"])); id == "" || id == "null" {
			return 0, fmt.Errorf("alert %d has no id", i)
		}
		b, _ := json.Marshal(r)
		items = append(items, b)
	}
	dbPath, _ := cmd.Flags().GetString("db")
	if dbPath == "" {
		dbPath = defaultDBPath("tactical-rmm-cli")
	}
	db, err := store.OpenWithContext(cmd.Context(), dbPath)
	if err != nil {
		return 0, fmt.Errorf("opening local database: %w", err)
	}
	defer db.Close()
	tx, err := db.DB().BeginTx(cmd.Context(), nil)
	if err != nil {
		return 0, err
	}
	for _, q := range []string{`DELETE FROM resources_fts WHERE resource_type = ?`, `DELETE FROM resources WHERE resource_type = ?`} {
		if _, err := tx.ExecContext(cmd.Context(), q, trmmAlertsResource); err != nil {
			_ = tx.Rollback()
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	stored, extractFailures, _, err := db.UpsertBatchDetailed(trmmAlertsResource, items)
	if err != nil {
		return 0, err
	}
	if extractFailures > 0 || stored != len(items) {
		return 0, fmt.Errorf("stored %d of %d alerts", stored, len(items))
	}
	if err := db.SaveSyncState(trmmAlertsResource, "", stored); err != nil {
		return 0, err
	}
	return stored, nil
}
