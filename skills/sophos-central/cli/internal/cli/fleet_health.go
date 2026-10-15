// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// healthRow is one tenant in the fleet health ranking.
type healthRow struct {
	TenantID        string        `json:"tenant_id"`
	TenantName      string        `json:"tenant_name"`
	Status          string        `json:"status"` // ok | no_data | forbidden | no_api_host | error
	Overall         *int          `json:"overall"`
	ProtectionScore *int          `json:"protection_score"`
	Delta           *int          `json:"delta"`
	Worst           []healthCheck `json:"worst_checks"`
	SnoozedChecks   []healthCheck `json:"snoozed_checks,omitempty"`
	CheckedAt       time.Time     `json:"checked_at,omitzero"`
}

type healthView struct {
	Items        []healthRow   `json:"items"`
	Total        int           `json:"total"`
	BlindTenants []blindTenant `json:"blind_tenants"`
	SyncedAt     *time.Time    `json:"synced_at"`
	Note         string        `json:"note,omitempty"`
}

// tenantHealthDoc is the fleet_health row written by fleet refresh.
type tenantHealthDoc struct {
	TenantID   string          `json:"tenant_id"`
	TenantName string          `json:"tenant_name"`
	SyncedAt   time.Time       `json:"synced_at"`
	Data       json.RawMessage `json:"data"`
}

func newNovelFleetHealthCmd(flags *rootFlags) *cobra.Command {
	var flagLimit int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "health",
		Short: "Rank customers worst-first by Account Health Check score with sub-scores and the change since the previous sync.",
		Long: `Use this command to rank tenants by Sophos Account Health Check score, worst first, with sub-scores and change since the previous fleet refresh. Do NOT use this command for a per-tenant list of fixable or sellable gaps; use 'fleet gaps' instead. Do NOT use it for individual alerts or endpoints; use 'fleet triage' instead.

overall is the lowest non-snoozed check score; protection_score is the lower of the computer and server protection scores (comparable to the console's protection figure). Tenants with no health data keep their reason (forbidden, no_data, error) instead of a blank score and sort after scored tenants. Reads the store filled by 'fleet refresh --resources health'.`,
		Example: strings.Trim(`
  sophos-central-cli fleet health --limit 10
  sophos-central-cli fleet health --agent --select items.tenant_name,items.overall,items.delta`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet health")
			}
			if flagLimit <= 0 {
				return usageErr(fmt.Errorf("--limit must be positive"))
			}
			path := fleetDBPath(dbPath)
			db, exists, err := openFleetStore(cmd.Context(), path)
			if err != nil {
				return err
			}
			if !exists {
				fmt.Fprintf(cmd.ErrOrStderr(), "no local mirror at %s\nrun: sophos-central-cli fleet refresh --db %s\n", path, path)
				if wantsHumanTable(cmd.OutOrStdout(), flags) {
					fmt.Fprintln(cmd.OutOrStdout(), "No fleet refresh has run yet. Run: sophos-central-cli fleet refresh")
					return nil
				}
				return printJSONFiltered(cmd.OutOrStdout(), healthView{Items: make([]healthRow, 0), BlindTenants: make([]blindTenant, 0), Note: fleetNeverRefreshedNote}, flags)
			}
			defer db.Close()
			filter, err := fleetTenantFilter()
			if err != nil {
				return err
			}
			view, err := buildHealth(db, filter)
			if err != nil {
				return err
			}
			view.SyncedAt, view.Note = fleetFreshness(cmd.ErrOrStderr(), loadFleetMeta(db), loadFleetLedgerQuiet(db), filter, flags.maxAge, "health")
			if len(view.Items) > flagLimit {
				view.Items = view.Items[:flagLimit]
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			defer warnBlind(cmd.ErrOrStderr(), len(view.BlindTenants))
			if len(view.Items) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No health data. Run: sophos-central-cli fleet refresh")
				return nil
			}
			rows := make([]map[string]any, 0, len(view.Items))
			for _, r := range view.Items {
				worst := ""
				if len(r.Worst) > 0 {
					worst = fmt.Sprintf("%s=%d", r.Worst[0].Check, r.Worst[0].Score)
				}
				rows = append(rows, map[string]any{"tenant": r.TenantName, "status": r.Status, "overall": intOrDash(r.Overall), "protection": intOrDash(r.ProtectionScore), "delta": intOrDash(r.Delta), "worst": worst})
			}
			return printAutoTable(cmd.OutOrStdout(), rows)
		},
	}
	cmd.Flags().IntVar(&flagLimit, "limit", 10, "Maximum tenants to return")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database file path (default: resolved data directory data.db)")
	return cmd
}

func intOrDash(p *int) string {
	if p == nil {
		return "-"
	}
	return fmt.Sprint(*p)
}

func intPtr(v int) *int { return &v }

// tenantHealthChecks decodes a stored health doc into its scored checks.
func tenantHealthChecks(raw json.RawMessage) (tenantHealthDoc, []healthCheck, map[string]any, error) {
	var doc tenantHealthDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return doc, nil, nil, err
	}
	var data map[string]any
	if err := json.Unmarshal(doc.Data, &data); err != nil {
		return doc, nil, nil, err
	}
	if data == nil {
		return doc, nil, nil, errors.New("health document has no data")
	}
	var checks []healthCheck
	walkHealthChecks("", data, &checks)
	return doc, checks, data, nil
}

func buildHealth(db fleetReader, filter map[string]bool) (healthView, error) {
	ledger, err := loadFleetLedger(db)
	if err != nil {
		return healthView{}, fmt.Errorf("reading fleet ledger: %w", err)
	}
	names := tenantNames(db, ledger)
	seen := map[string]bool{}
	unreadable := unreadableRows{}
	rows := make([]healthRow, 0)
	err = db.ListScan(fleetHealthType, func(_ string, raw json.RawMessage) bool {
		doc, checks, data, derr := tenantHealthChecks(raw)
		if derr != nil || doc.TenantID == "" {
			unreadable.add(raw, "health")
			return true
		}
		if filter != nil && !filter[doc.TenantID] {
			return true
		}
		seen[doc.TenantID] = true
		row := healthRow{TenantID: doc.TenantID, TenantName: doc.TenantName, Status: "ok", CheckedAt: doc.SyncedAt, Worst: make([]healthCheck, 0)}
		if row.TenantName == "" {
			row.TenantName = names[doc.TenantID]
		}
		if overall, ok := overallHealth(checks); ok {
			row.Overall = intPtr(overall)
		} else {
			row.Status = "no_data"
		}
		row.ProtectionScore = minScore(data, []string{"endpoint", "protection", "computer", "score"}, []string{"endpoint", "protection", "server", "score"})
		active := make([]healthCheck, 0, len(checks))
		for _, c := range checks {
			if c.Snoozed {
				row.SnoozedChecks = append(row.SnoozedChecks, c)
			} else {
				active = append(active, c)
			}
		}
		sort.SliceStable(active, func(i, j int) bool { return active[i].Score < active[j].Score })
		if len(active) > 3 {
			active = active[:3]
		}
		row.Worst = active
		rows = append(rows, row)
		return true
	})
	if err != nil {
		return healthView{}, fmt.Errorf("reading health checks: %w", err)
	}
	// Change since the previous snapshot (drained above; separate scan).
	for i := range rows {
		if rows[i].Overall == nil {
			continue
		}
		prevRaw, gerr := db.Get(fleetHealthPrevType, rows[i].TenantID)
		if gerr != nil || len(prevRaw) == 0 {
			continue
		}
		if _, prevChecks, _, derr := tenantHealthChecks(prevRaw); derr == nil {
			if prev, ok := overallHealth(prevChecks); ok {
				rows[i].Delta = intPtr(*rows[i].Overall - prev)
			}
		}
	}
	// Tenants the last sync could not read: keep them, with their reason.
	for id, l := range ledger {
		if seen[id] || unreadable[id] != nil || (filter != nil && !filter[id]) {
			continue
		}
		status := "no_data"
		if o, ok := l.Resources["health"]; ok && o.Status != "ok" {
			status = o.Status
		} else if l.Status != "ok" && l.Status != "partial" {
			status = l.Status
		}
		rows = append(rows, healthRow{TenantID: id, TenantName: l.TenantName, Status: status, Worst: make([]healthCheck, 0)})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i].Overall, rows[j].Overall
		switch {
		case a != nil && b != nil:
			if *a != *b {
				return *a < *b
			}
			return rows[i].TenantName < rows[j].TenantName
		case a != nil:
			return true
		case b != nil:
			return false
		}
		return rows[i].TenantName < rows[j].TenantName
	})
	return healthView{Items: rows, Total: len(rows), BlindTenants: append(append(blindTenantsFrom(ledger, filter, "health"), unreadable.blind(names, filter)...), unrefreshedTenants(db, ledger, filter)...)}, nil
}

// minScore returns the lowest numeric value among the given paths, or nil.
func minScore(data map[string]any, paths ...[]string) *int {
	var out *int
	for _, p := range paths {
		if v, ok := jsonPath(data, p...).(float64); ok {
			if out == nil || int(v) < *out {
				out = intPtr(int(v))
			}
		}
	}
	return out
}
