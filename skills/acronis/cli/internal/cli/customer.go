// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"time"

	"acronis-pp-cli/internal/store"
	"github.com/spf13/cobra"
)

type customerCard struct {
	TenantID      string `json:"tenant_id"`
	Name          string `json:"name,omitempty"`
	Kind          string `json:"kind,omitempty"`
	Enabled       bool   `json:"enabled"`
	ParentID      string `json:"parent_id,omitempty"`
	ChildTenants  int    `json:"child_tenants"`
	Users         int    `json:"users"`
	OfferingItems int    `json:"offering_items"`
	UsageRecords  int    `json:"usage_records"`
	OAuthClients  int    `json:"oauth_clients"`
	AgentsTotal   int    `json:"agents_total"`
	AgentsOnline  int    `json:"agents_online"`
	LastAgentSeen string `json:"last_agent_seen,omitempty"`
	TasksTotal7d  int    `json:"tasks_total_7d"`
	TasksOK7d     int    `json:"tasks_ok_7d"`
	TasksFailed7d int    `json:"tasks_failed_7d"`
	TasksUndated  int    `json:"tasks_undated,omitempty"`
	LastSuccess   string `json:"last_success,omitempty"`
}

func countWhere(db *store.Store, query string, args ...any) int {
	var n int
	if err := db.DB().QueryRow(query, args...).Scan(&n); err != nil {
		return 0
	}
	return n
}

// computeCustomerCard joins every local table for a single tenant into one
// cross-resource snapshot card.
func computeCustomerCard(db *store.Store, tenantID string, now time.Time) (customerCard, error) {
	card := customerCard{TenantID: tenantID}

	var name, kind, parentID *string
	var enabled *int
	err := db.DB().QueryRow(`SELECT name, kind, enabled, parent_id FROM tenants WHERE id = ?`, tenantID).
		Scan(&name, &kind, &enabled, &parentID)
	if err != nil {
		return card, fmt.Errorf("tenant %q not found in local store (run 'acronis-cli sync' first): %w", tenantID, err)
	}
	card.Name = deref(name)
	card.Kind = deref(kind)
	card.ParentID = deref(parentID)
	card.Enabled = enabled != nil && *enabled != 0

	card.ChildTenants = countWhere(db, `SELECT COUNT(*) FROM tenants WHERE parent_id = ?`, tenantID)
	card.Users = countWhere(db, `SELECT COUNT(*) FROM users WHERE tenants_id = ?`, tenantID)
	offerings, err := countActiveOfferings(db, tenantID)
	if err != nil {
		return card, err
	}
	card.OfferingItems = offerings
	card.UsageRecords = countWhere(db, `SELECT COUNT(*) FROM usages WHERE tenants_id = ?`, tenantID)
	card.OAuthClients = countWhere(db, `SELECT COUNT(*) FROM clients WHERE tenant_id = ?`, tenantID)

	// Agents: total, online (status proves it), most recent check-in.
	aRows, err := db.Query(`SELECT id, status, last_seen FROM agent_manager WHERE tenant_id = ?`, tenantID)
	if err != nil {
		return card, fmt.Errorf("querying agent_manager: %w", err)
	}
	var lastSeen time.Time
	for aRows.Next() {
		var id string
		var status, seen *string
		if err := aRows.Scan(&id, &status, &seen); err != nil {
			aRows.Close()
			return card, fmt.Errorf("reading agent_manager: %w", err)
		}
		card.AgentsTotal++
		if agentOnline(deref(status)) {
			card.AgentsOnline++
		}
		t, known, err := rollupTime(deref(seen))
		if err != nil {
			aRows.Close()
			return card, fmt.Errorf("agent %s: %w", id, err)
		}
		if known && t.After(lastSeen) {
			lastSeen = t
		}
	}
	err = aRows.Err()
	aRows.Close()
	if err != nil {
		return card, fmt.Errorf("reading agent_manager: %w", err)
	}
	if !lastSeen.IsZero() {
		card.LastAgentSeen = lastSeen.UTC().Format(time.RFC3339)
	}

	// Backup tasks: 7-day window stats plus all-time last success. Only an
	// explicit backup task can be a success; an untyped task still counts, and
	// a task the mirror cannot date is counted (never as ok) in tasks_undated.
	cutoff := now.Add(-7 * 24 * time.Hour)
	tRows, err := db.Query(`SELECT id, type, state, result_code, `+taskTimeColumns+` FROM task_manager WHERE tenant_id = ?`, tenantID)
	if err != nil {
		return card, fmt.Errorf("querying task_manager: %w", err)
	}
	var lastSuccess time.Time
	for tRows.Next() {
		var id string
		var typ, state, rc, completedAt, updatedAt, startedAt, enqueuedAt, createdAt *string
		if err := tRows.Scan(&id, &typ, &state, &rc, &completedAt, &updatedAt, &startedAt, &enqueuedAt, &createdAt); err != nil {
			tRows.Close()
			return card, fmt.Errorf("reading task_manager: %w", err)
		}
		ts, known, err := rollupTime(deref(completedAt), deref(updatedAt), deref(startedAt), deref(enqueuedAt), deref(createdAt))
		if err != nil {
			tRows.Close()
			return card, fmt.Errorf("task %s: %w", id, err)
		}
		kind := taskBackupKind(deref(typ))
		if kind == "other" {
			continue
		}
		outcome := taskOutcome(deref(state), deref(rc))
		if outcome == "ok" && kind == "backup" && known && ts.After(lastSuccess) {
			lastSuccess = ts
		}
		if known && ts.Before(cutoff) {
			continue
		}
		card.TasksTotal7d++
		if !known {
			card.TasksUndated++
		}
		switch {
		case outcome == "ok" && kind == "backup" && known:
			card.TasksOK7d++
		case outcome == "failed":
			card.TasksFailed7d++
		}
	}
	err = tRows.Err()
	tRows.Close()
	if err != nil {
		return card, fmt.Errorf("reading task_manager: %w", err)
	}
	if !lastSuccess.IsZero() {
		card.LastSuccess = lastSuccess.UTC().Format(time.RFC3339)
	}

	return card, nil
}

// pp:data-source local
func newNovelCustomerCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "customer [tenant_id]",
		Short: "One cross-resource snapshot of a single customer: tenants, users, licenses, agents, and backup status joined.",
		Long: `Join every locally synced table for one tenant into a single card: tenant
record, child tenants, users, offering items, usage records, OAuth clients,
agent fleet (online/total, last check-in), and 7-day task pass/fail with the
all-time last successful backup.

Use this for the cross-resource snapshot of one customer (agents + usage +
users + backup status joined). Do NOT use it for the raw tenant record
fields; use 'tenants get'.

Reads the local store; run 'acronis-cli sync' first.`,
		Example: `  # Full 360 card for one tenant
  acronis-cli customer 8b7e0e9d-0b5a-4f0e-9c1d-1234567890ab

  # Agent JSON, narrowed fields
  acronis-cli customer TENANT_ID --agent --select tenant_id,agents_online,tasks_failed_7d`,
		Annotations: map[string]string{
			"mcp:read-only": "true",
			"pp:happy-args": "tenant_id=8b7e0e9d-0b5a-4f0e-9c1d-1234567890ab",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			if len(args) < 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("tenant_id is required"))
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			db, err := openNovelDB(cmd, dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			maybeEmitSyncHints(cmd, db, "", flags.maxAge)

			card, err := computeCustomerCard(db, args[0], time.Now())
			if err != nil {
				return err
			}

			if wantJSON(flags, cmd) {
				return encodeJSON(cmd, flags, card)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Tenant:        %s (%s)\n", card.Name, card.TenantID)
			fmt.Fprintf(cmd.OutOrStdout(), "Kind/Enabled:  %s / %t\n", card.Kind, card.Enabled)
			if card.ParentID != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Parent:        %s\n", card.ParentID)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Children:      %d\n", card.ChildTenants)
			fmt.Fprintf(cmd.OutOrStdout(), "Users:         %d\n", card.Users)
			fmt.Fprintf(cmd.OutOrStdout(), "Offerings:     %d\n", card.OfferingItems)
			fmt.Fprintf(cmd.OutOrStdout(), "Usage rows:    %d\n", card.UsageRecords)
			fmt.Fprintf(cmd.OutOrStdout(), "OAuth clients: %d\n", card.OAuthClients)
			fmt.Fprintf(cmd.OutOrStdout(), "Agents:        %d online / %d total", card.AgentsOnline, card.AgentsTotal)
			if card.LastAgentSeen != "" {
				fmt.Fprintf(cmd.OutOrStdout(), " (last seen %s)", card.LastAgentSeen)
			}
			fmt.Fprintln(cmd.OutOrStdout())
			fmt.Fprintf(cmd.OutOrStdout(), "Tasks (7d):    %d total, %d ok, %d failed, %d undated\n", card.TasksTotal7d, card.TasksOK7d, card.TasksFailed7d, card.TasksUndated)
			last := card.LastSuccess
			if last == "" {
				last = "(never)"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Last success:  %s\n", last)
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/acronis-cli/data.db)")
	return cmd
}
