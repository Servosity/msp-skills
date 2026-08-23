// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Novel feature: cross-tenant software version spread.
// pp:data-source local

package cli

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"immybot-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

type versionBucket struct {
	Version       string   `json:"version"`
	ComputerCount int      `json:"computer_count"`
	TenantCount   int      `json:"tenant_count"`
	BelowFloor    bool     `json:"below_floor"`
	Tenants       []string `json:"tenants"`
}

type tenantLag struct {
	Tenant        string `json:"tenant"`
	LowestVersion string `json:"lowest_version"`
	ComputerCount int    `json:"computer_count"`
}

type versionSpreadView struct {
	Title            string          `json:"title"`
	MinVersion       string          `json:"min_version,omitempty"`
	MatchedInstalls  int             `json:"matched_installs"`
	ScannedInstalls  int             `json:"scanned_installs"`
	DistinctVersions int             `json:"distinct_versions"`
	BelowFloor       int             `json:"computers_below_floor"`
	Versions         []versionBucket `json:"versions"`
	TenantsBehind    []tenantLag     `json:"tenants_behind"`
	Note             string          `json:"note,omitempty"`
}

func newNovelVersionSpreadCmd(flags *rootFlags) *cobra.Command {
	var (
		flagMinVersion string
		flagTenant     string
		flagExact      bool
		dbPath         string
	)

	cmd := &cobra.Command{
		Use:   "version-spread [title]",
		Short: "Semver-ordered spread of one software title across every tenant",
		Long: "Semver-ordered distribution of one software title across every tenant, flagging " +
			"everything below a floor.\n\n" +
			"Use this command for the version distribution of one software title across every " +
			"tenant. Do NOT use this command for a whole-tenant compliance posture across all " +
			"titles; use 'drift' instead.",
		Example: strings.Trim(`
  immybot-cli version-spread "Google Chrome" --min-version 140
  immybot-cli version-spread "7-Zip" --agent
  immybot-cli version-spread "Google Chrome" --min-version 140 --tenant Contoso --agent
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
			"pp:happy-args": "title=Google Chrome",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "version-spread")
			}
			if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a software title is required"))
			}
			title := strings.TrimSpace(args[0])
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			dbPath = immyMirrorPath(dbPath)
			view := versionSpreadView{
				Title:         title,
				MinVersion:    strings.TrimSpace(flagMinVersion),
				Versions:      make([]versionBucket, 0),
				TenantsBehind: make([]tenantLag, 0),
			}
			if immyMirrorMissing(cmd, dbPath, "tenants-software-from-inventory-dx") {
				if !wantsHumanTable(cmd.OutOrStdout(), flags) {
					return printJSONFiltered(cmd.OutOrStdout(), view, flags)
				}
				return nil
			}

			db, err := store.OpenWithContext(ctx, dbPath)
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "tenants-software-from-inventory-dx") {
				hintIfStale(cmd, db, "tenants-software-from-inventory-dx", flags.maxAge)
			}

			rows, err := db.DB().QueryContext(ctx, `
				SELECT
					json_extract(data,'$.softwareName'),
					json_extract(data,'$.version'),
					json_extract(data,'$.tenantName'),
					json_extract(data,'$.computerName')
				FROM resources
				WHERE resource_type IN ('tenants-software-from-inventory-dx','computers-inventory')`)
			if err != nil {
				return fmt.Errorf("querying software inventory: %w", err)
			}

			type installRow struct{ name, version, tenant, computer string }
			installs := make([]installRow, 0)
			for rows.Next() {
				var name, version, tenant, computer sql.NullString
				if err := rows.Scan(&name, &version, &tenant, &computer); err != nil {
					_ = rows.Close()
					return fmt.Errorf("scanning software inventory: %w", err)
				}
				installs = append(installs, installRow{
					name: nullStr(name), version: nullStr(version),
					tenant: nullStr(tenant), computer: nullStr(computer),
				})
			}
			if err := rows.Err(); err != nil {
				_ = rows.Close()
				return fmt.Errorf("iterating software inventory: %w", err)
			}
			if err := rows.Close(); err != nil {
				return fmt.Errorf("closing software inventory: %w", err)
			}

			needle := strings.ToLower(title)
			tenantFilter := strings.ToLower(strings.TrimSpace(flagTenant))

			type vAgg struct {
				computers map[string]struct{}
				tenants   map[string]struct{}
			}
			byVersion := map[string]*vAgg{}
			lowestByTenant := map[string]string{}
			tenantComputers := map[string]map[string]struct{}{}

			for _, in := range installs {
				view.ScannedInstalls++
				if in.name == "" {
					continue
				}
				lname := strings.ToLower(in.name)
				if flagExact {
					if !strings.EqualFold(in.name, title) {
						continue
					}
				} else if !strings.Contains(lname, needle) {
					continue
				}
				if tenantFilter != "" && !strings.Contains(strings.ToLower(in.tenant), tenantFilter) {
					continue
				}
				view.MatchedInstalls++

				ver := strings.TrimSpace(in.version)
				if ver == "" {
					ver = "(unknown)"
				}
				a := byVersion[ver]
				if a == nil {
					a = &vAgg{computers: map[string]struct{}{}, tenants: map[string]struct{}{}}
					byVersion[ver] = a
				}
				if in.computer != "" {
					a.computers[in.computer] = struct{}{}
				}
				if in.tenant != "" {
					a.tenants[in.tenant] = struct{}{}
					if tc := tenantComputers[in.tenant]; tc == nil {
						tenantComputers[in.tenant] = map[string]struct{}{}
					}
					if in.computer != "" {
						tenantComputers[in.tenant][in.computer] = struct{}{}
					}
					cur, ok := lowestByTenant[in.tenant]
					if !ok || (ver != "(unknown)" && immyCompareVersions(ver, cur) < 0) {
						lowestByTenant[in.tenant] = ver
					}
				}
			}

			floor := strings.TrimSpace(flagMinVersion)
			for ver, a := range byVersion {
				b := versionBucket{
					Version:       ver,
					ComputerCount: len(a.computers),
					TenantCount:   len(a.tenants),
					Tenants:       sortedSetSlice(a.tenants),
				}
				if floor != "" && ver != "(unknown)" && immyCompareVersions(ver, floor) < 0 {
					b.BelowFloor = true
					view.BelowFloor += b.ComputerCount
				}
				view.Versions = append(view.Versions, b)
			}
			// Highest version first, so the newest build heads the table and
			// the laggards sort to the bottom where they are read.
			sort.Slice(view.Versions, func(i, j int) bool {
				return immyCompareVersions(view.Versions[i].Version, view.Versions[j].Version) > 0
			})
			view.DistinctVersions = len(view.Versions)

			if floor != "" {
				for tenant, low := range lowestByTenant {
					if low == "(unknown)" || immyCompareVersions(low, floor) >= 0 {
						continue
					}
					view.TenantsBehind = append(view.TenantsBehind, tenantLag{
						Tenant:        tenant,
						LowestVersion: low,
						ComputerCount: len(tenantComputers[tenant]),
					})
				}
				sort.Slice(view.TenantsBehind, func(i, j int) bool {
					return immyCompareVersions(view.TenantsBehind[i].LowestVersion, view.TenantsBehind[j].LowestVersion) < 0
				})
			}

			if view.ScannedInstalls == 0 {
				view.Note = "no software inventory in the local mirror; run 'immybot-cli sync --resources tenants-software-from-inventory-dx'"
			} else if view.MatchedInstalls == 0 {
				view.Note = fmt.Sprintf("scanned %d inventory rows without matching software title %q; "+
					"re-run 'immybot-cli sync --resources tenants-software-from-inventory-dx' if the mirror is stale",
					view.ScannedInstalls, title)
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			if len(view.Versions) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No installs of %q found (scanned %d inventory rows).\n", title, view.ScannedInstalls)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-24s %-8s %-8s %s\n", "VERSION", "MACHS", "TENANTS", "")
			for _, b := range view.Versions {
				marker := ""
				if b.BelowFloor {
					marker = "BELOW FLOOR"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-24s %-8d %-8d %s\n", immyTruncate(b.Version, 24), b.ComputerCount, b.TenantCount, marker)
			}
			if floor != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "\n%d computer(s) below %s across %d tenant(s).\n",
					view.BelowFloor, floor, len(view.TenantsBehind))
				for _, t := range view.TenantsBehind {
					fmt.Fprintf(cmd.OutOrStdout(), "  %-40s lowest %s (%d machines)\n", immyTruncate(t.Tenant, 40), t.LowestVersion, t.ComputerCount)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagMinVersion, "min-version", "", "Flag installs below this version (semver-aware comparison)")
	cmd.Flags().StringVar(&flagTenant, "tenant", "", "Only consider inventory rows whose tenant name contains this substring")
	cmd.Flags().BoolVar(&flagExact, "exact", false, "Match the software title exactly instead of by substring")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local mirror database path")
	return cmd
}
