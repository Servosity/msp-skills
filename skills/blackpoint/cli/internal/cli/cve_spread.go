// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: cve-spread.
// pp:data-source local

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

type cveSpreadRow struct {
	CVE          string   `json:"cve"`
	Tenants      int      `json:"tenants"`
	Assets       float64  `json:"assets"`
	MaxBaseScore float64  `json:"max_base_score"`
	Severity     string   `json:"severity"`
	Exploited    bool     `json:"exploited"`
	Prioritized  bool     `json:"prioritized"`
	TenantNames  []string `json:"tenant_names"`
}

var severityRank = map[string]int{"low": 1, "medium": 2, "moderate": 2, "high": 3, "critical": 4}

func newNovelCveSpreadCmd(flags *rootFlags) *cobra.Command {
	var exploited, prioritized bool
	var minTenants, limit int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "cve-spread",
		Short: "Rank exploited or prioritized CVEs by how many tenants and assets they affect",
		Long: `Groups the unresolved vulnerability findings fleet-sync stored for every tenant
by CVE, then ranks by distinct tenants affected and then by affected assets.
CompassOne has no cross-tenant vulnerability rollup; this answers which patch
campaign protects the most customers first.`,
		Example: strings.Trim(`
  blackpoint-cli cve-spread --exploited --agent
  blackpoint-cli cve-spread --prioritized --min-tenants 3`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "cve-spread: group local vulnerability findings by CVE across tenants")
			}
			if limit < 0 || minTenants < 1 {
				return usageErr(fmt.Errorf("--limit must be zero or more and --min-tenants at least 1"))
			}
			db, ok, err := openFleetForRead(cmd, flags, &dbPath)
			if err != nil {
				return err
			}
			if !ok {
				return emitRows(cmd, flags, []cveSpreadRow{})
			}
			defer db.Close()
			if err := checkFleetKind(cmd, flags, db.DB(), fleetKindVuln); err != nil {
				return err
			}
			idx, _, err := fleetTenantIndex(db.DB())
			if err != nil {
				return err
			}
			recs, err := loadCleanFleetRows(db.DB(), fleetKindVuln, fleetKindVuln)
			if err != nil {
				return err
			}
			rows := buildCveSpreadRows(recs, idx, exploited, prioritized, minTenants)
			if limit > 0 && len(rows) > limit {
				rows = rows[:limit]
			}
			return emitRows(cmd, flags, rows, "cve", "tenants", "assets", "max_base_score", "severity", "exploited", "prioritized")
		},
	}
	cmd.Flags().BoolVar(&exploited, "exploited", false, "Only CVEs known exploited in the wild (exploitability Attacked)")
	cmd.Flags().BoolVar(&prioritized, "prioritized", false, "Only vendor-prioritized findings")
	cmd.Flags().IntVar(&minTenants, "min-tenants", 1, "Only CVEs present in at least this many tenants")
	cmd.Flags().IntVar(&limit, "limit", 25, "Maximum rows to return (0 = all)")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database path (default: the CLI's local data.db)")
	return cmd
}

func buildCveSpreadRows(recs []fleetRecord, idx map[string]fleetTenant, exploited, prioritized bool, minTenants int) []cveSpreadRow {
	type agg struct {
		row     cveSpreadRow
		tenants map[string]bool
	}
	groups := map[string]*agg{}
	for _, r := range recs {
		o := r.Obj
		isExploited := strings.EqualFold(fieldStr(o, "exploitability"), "Attacked")
		isPrioritized, _ := fieldBool(o, "prioritized")
		if (exploited && !isExploited) || (prioritized && !isPrioritized) {
			continue
		}
		key := strings.ToUpper(fieldStr(o, "cveId", "cve", "name", "id"))
		if key == "" {
			key = r.ID
		}
		g := groups[key]
		if g == nil {
			g = &agg{row: cveSpreadRow{CVE: key}, tenants: map[string]bool{}}
			groups[key] = g
		}
		g.tenants[r.Scope] = true
		if n, ok := fieldNum(o, "assetsAmount", "affectedAssets"); ok {
			g.row.Assets += n
		}
		if s, ok := fieldNum(o, "baseScore", "cvssScore"); ok && s > g.row.MaxBaseScore {
			g.row.MaxBaseScore = s
		}
		if sev := fieldStr(o, "severity"); severityRank[strings.ToLower(sev)] > severityRank[strings.ToLower(g.row.Severity)] {
			g.row.Severity = sev
		}
		g.row.Exploited = g.row.Exploited || isExploited
		g.row.Prioritized = g.row.Prioritized || isPrioritized
	}
	rows := []cveSpreadRow{}
	for _, g := range groups {
		if len(g.tenants) < minTenants {
			continue
		}
		g.row.Tenants = len(g.tenants)
		names := make([]string, 0, len(g.tenants))
		for id := range g.tenants {
			names = append(names, tenantName(idx, id))
		}
		sort.Strings(names)
		if len(names) > 10 {
			names = names[:10]
		}
		g.row.TenantNames = names
		rows = append(rows, g.row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Tenants != rows[j].Tenants {
			return rows[i].Tenants > rows[j].Tenants
		}
		if rows[i].Assets != rows[j].Assets {
			return rows[i].Assets > rows[j].Assets
		}
		return rows[i].CVE < rows[j].CVE
	})
	return rows
}
