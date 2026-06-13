// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// driftEvent is one security-relevant change, normalized across the ActionLog
// (tenant-scoped) and system-audit (portal admin) sources.
type driftEvent struct {
	Source         string `json:"source"`
	OrganizationID string `json:"organizationId"`
	Date           string `json:"date"`
	Actor          string `json:"actor"`
	Action         string `json:"action"`
	Target         string `json:"target"`
	sortKey        time.Time
	sortOK         bool
}

func newNovelAuditDriftCmd(flags *rootFlags) *cobra.Command {
	var flagSince string
	var flagAllTenants bool
	var flagLimit int
	var dbPath string

	cmd := &cobra.Command{
		Use:         "drift",
		Short:       "One ranked table of security-relevant changes (protection, policy, maintenance) across every tenant in a window",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Long: `Drift merges the locally synced ActionLog (tenant-scoped events) and the portal
System Audit (admin Create/Delete/Modify actions) into one newest-first table of
security-relevant changes across a time window — the cross-tenant "who changed
what this week" view no single Portal API call returns.

Sync first: threatlocker-cli sync --resources audit,system-audit`,
		Example: strings.Trim(`
  threatlocker-cli audit drift --since 7d --all-tenants --agent
  threatlocker-cli audit drift --since 2026-05-01 --limit 50
`, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			out := cmd.OutOrStdout()
			if dbPath == "" {
				dbPath = defaultDBPath("threatlocker-cli")
			}
			cutoff, ok := resolveSince(flagSince, time.Now())
			if !ok {
				return fmt.Errorf("invalid --since %q: use a window like 7d/12h or a date like 2026-05-01", flagSince)
			}
			db, err := tlOpenStore(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local database: %w\nRun 'threatlocker-cli sync --resources audit,system-audit' first.", err)
			}
			defer db.Close()

			var events []driftEvent

			// ActionLog (tenant-scoped).
			alQuery := `SELECT organization_id, date_created, username, action, computer_name, full_path FROM audit`
			var alArgs []any
			if !flagAllTenants && flags.orgID != "" {
				alQuery += ` WHERE organization_id = ?`
				alArgs = append(alArgs, flags.orgID)
			}
			alRows, err := db.DB().QueryContext(cmd.Context(), alQuery, alArgs...)
			if err != nil {
				// The store schema creates the audit table on open, so any
				// query error here is a real failure (locked/corrupt DB), not
				// "not synced yet". A silent skip would read as "no drift".
				return fmt.Errorf("querying audit (actionlog): %w", err)
			}
			for alRows.Next() {
				var org, date, user, action, computer, path *string
				if err := alRows.Scan(&org, &date, &user, &action, &computer, &path); err != nil {
					_ = alRows.Close()
					return fmt.Errorf("scanning audit (actionlog): %w", err)
				}
				ev := driftEvent{Source: "actionlog", OrganizationID: tlString(org), Date: tlString(date), Actor: tlString(user), Action: tlString(action)}
				ev.Target = tlString(path)
				if ev.Target == "" {
					ev.Target = tlString(computer)
				}
				if t, ok := parseTLTime(ev.Date); ok {
					if !cutoff.IsZero() && t.Before(cutoff) {
						continue
					}
					ev.sortKey, ev.sortOK = t, true
				}
				events = append(events, ev)
			}
			if err := alRows.Err(); err != nil {
				_ = alRows.Close()
				return fmt.Errorf("iterating audit (actionlog): %w", err)
			}
			_ = alRows.Close()

			// System Audit (portal admin changes; no per-row org). Restrict to
			// mutating actions — drift is about changes, not reads/logons.
			saRows, err := db.DB().QueryContext(cmd.Context(),
				`SELECT date_created, username, action, effective_action, details FROM system_audit
				 WHERE action NOT IN ('Read','Logon','')`)
			if err != nil {
				return fmt.Errorf("querying system_audit: %w", err)
			}
			for saRows.Next() {
				var date, user, action, effective, details *string
				if err := saRows.Scan(&date, &user, &action, &effective, &details); err != nil {
					_ = saRows.Close()
					return fmt.Errorf("scanning system_audit: %w", err)
				}
				act := tlString(action)
				if eff := tlString(effective); eff != "" {
					act = act + "/" + eff
				}
				ev := driftEvent{Source: "system", OrganizationID: "(portal)", Date: tlString(date), Actor: tlString(user), Action: act, Target: tlString(details)}
				if t, ok := parseTLTime(ev.Date); ok {
					if !cutoff.IsZero() && t.Before(cutoff) {
						continue
					}
					ev.sortKey, ev.sortOK = t, true
				}
				events = append(events, ev)
			}
			if err := saRows.Err(); err != nil {
				_ = saRows.Close()
				return fmt.Errorf("iterating system_audit: %w", err)
			}
			_ = saRows.Close()

			// Newest first; unparseable dates sink to the bottom.
			sort.SliceStable(events, func(i, j int) bool {
				if events[i].sortOK != events[j].sortOK {
					return events[i].sortOK
				}
				return events[i].sortKey.After(events[j].sortKey)
			})
			if flagLimit > 0 && len(events) > flagLimit {
				events = events[:flagLimit]
			}

			if flags.asJSON {
				return flags.printJSON(cmd, events)
			}
			if len(events) == 0 {
				fmt.Fprintln(out, "No drift events in the window. Sync first: threatlocker-cli sync --resources audit,system-audit")
				return nil
			}
			headers := []string{"DATE", "SOURCE", "ORG", "ACTOR", "ACTION", "TARGET"}
			tableRows := make([][]string, 0, len(events))
			for _, e := range events {
				target := e.Target
				if len(target) > 48 {
					target = target[:48] + "…"
				}
				tableRows = append(tableRows, []string{e.Date, e.Source, e.OrganizationID, e.Actor, e.Action, target})
			}
			return flags.printTable(cmd, headers, tableRows)
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "7d", "Window lower bound (e.g. 7d/24h or a date like 2026-05-01)")
	cmd.Flags().BoolVar(&flagAllTenants, "all-tenants", false, "Include ActionLog events from every synced organization (default scopes to --org when set)")
	cmd.Flags().IntVar(&flagLimit, "limit", 100, "Maximum events to return (0 = all)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
