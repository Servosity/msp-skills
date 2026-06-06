// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"time"

	"acronis-pp-cli/internal/cliutil"
	"acronis-pp-cli/internal/store"
	"github.com/spf13/cobra"
)

type staleAgentRow struct {
	TenantID   string `json:"tenant_id"`
	TenantName string `json:"tenant_name"`
	AgentID    string `json:"agent_id"`
	Hostname   string `json:"hostname"`
	LastSeen   string `json:"last_seen"`
	Status     string `json:"status"`
	Version    string `json:"version"`
}

// computeStaleAgents returns agents whose last_seen is older than the cutoff
// (now - olderThan) OR whose status indicates offline.
func computeStaleAgents(db *store.Store, olderThan time.Duration, now time.Time) ([]staleAgentRow, error) {
	names := tenantNames(db)
	cutoff := now.Add(-olderThan)

	rows, err := db.Query(`SELECT id, tenant_id, hostname, last_seen, status, version FROM agent_manager`)
	if err != nil {
		return nil, fmt.Errorf("querying agent_manager: %w", err)
	}
	defer rows.Close()

	var out []staleAgentRow
	for rows.Next() {
		var id string
		var tenantID, hostname, lastSeen, status, version *string
		if err := rows.Scan(&id, &tenantID, &hostname, &lastSeen, &status, &version); err != nil {
			continue
		}
		ls := deref(lastSeen)
		stale := false
		if t, ok := parseTime(ls); ok {
			stale = t.Before(cutoff)
		} else if ls == "" {
			// No last_seen at all is treated as stale.
			stale = true
		}
		if agentOffline(deref(status)) {
			stale = true
		}
		if !stale {
			continue
		}
		tid := deref(tenantID)
		out = append(out, staleAgentRow{
			TenantID:   tid,
			TenantName: names[tid],
			AgentID:    id,
			Hostname:   deref(hostname),
			LastSeen:   ls,
			Status:     deref(status),
			Version:    deref(version),
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].TenantID != out[j].TenantID {
			return out[i].TenantID < out[j].TenantID
		}
		return out[i].LastSeen < out[j].LastSeen
	})
	return out, nil
}

// pp:data-source local
func newNovelAgentsStaleCmd(flags *rootFlags) *cobra.Command {
	var flagOlderThan string
	var dbPath string
	var limit int

	cmd := &cobra.Command{
		Use:         "stale",
		Short:       "List backup agents that haven't checked in within a threshold, across every tenant, sorted by customer.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			olderThan := 7 * 24 * time.Hour
			if flagOlderThan != "" {
				d, err := cliutil.ParseDurationLoose(flagOlderThan)
				if err != nil {
					return fmt.Errorf("invalid --older-than %q: %w", flagOlderThan, err)
				}
				olderThan = d
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

			rowsOut, err := computeStaleAgents(db, olderThan, time.Now())
			if err != nil {
				return err
			}
			if limit > 0 && len(rowsOut) > limit {
				rowsOut = rowsOut[:limit]
			}

			if wantJSON(flags, cmd) {
				if rowsOut == nil {
					rowsOut = []staleAgentRow{}
				}
				return encodeJSON(cmd, flags, rowsOut)
			}
			if len(rowsOut) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No stale agents — run 'acronis-cli sync' first if you expected data.")
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-24s %-20s %-20s %-24s %s\n", "TENANT", "AGENT_ID", "HOSTNAME", "LAST_SEEN", "VERSION")
			for _, r := range rowsOut {
				label := r.TenantName
				if label == "" {
					label = r.TenantID
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-24s %-20s %-20s %-24s %s\n", truncate(label, 24), truncate(r.AgentID, 20), truncate(r.Hostname, 20), r.LastSeen, r.Version)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagOlderThan, "older-than", "7d", "Flag agents not seen within this window (e.g. 7d, 72h)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/acronis-cli/data.db)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum agents to show (0 = all)")
	return cmd
}
