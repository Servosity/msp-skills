// Copyright 2026 Abhi Saini and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Absorbed feature (manifest row 38): tenant compliance posture across all titles.
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

type driftTitle struct {
	Title         string `json:"title"`
	TenantVersion string `json:"tenant_version"`
	FleetLatest   string `json:"fleet_latest"`
	ComputerCount int    `json:"computer_count"`
}

type driftTenant struct {
	Tenant        string       `json:"tenant"`
	TitlesTracked int          `json:"titles_tracked"`
	TitlesBehind  int          `json:"titles_behind"`
	Behind        []driftTitle `json:"behind"`
}

type driftView struct {
	ScannedInstalls int           `json:"scanned_installs"`
	TitlesTracked   int           `json:"titles_tracked"`
	Tenants         []driftTenant `json:"tenants"`
	Note            string        `json:"note,omitempty"`
}

func newNovelDriftCmd(flags *rootFlags) *cobra.Command {
	var (
		flagTenant string
		flagLimit  int
		dbPath     string
	)
	cmd := &cobra.Command{
		Use:   "drift",
		Short: "Per-tenant compliance posture across every software title",
		Long: "Rank tenants by how far their installed software has drifted behind the newest " +
			"version seen anywhere in the fleet.\n\n" +
			"Use this command for a whole-tenant compliance posture across all titles. Do NOT " +
			"use this command for the version distribution of one title; use 'version-spread' instead.",
		Example: strings.Trim(`
  immybot-pp-cli drift
  immybot-pp-cli drift --tenant Contoso --agent
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
			"pp:happy-args": "--limit=25",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "drift")
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			dbPath = immyMirrorPath(dbPath)
			view := driftView{Tenants: make([]driftTenant, 0)}
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
				SELECT json_extract(data,'$.softwareName'), json_extract(data,'$.version'),
				       json_extract(data,'$.tenantName'), json_extract(data,'$.computerName')
				FROM resources WHERE resource_type = 'tenants-software-from-inventory-dx'`)
			if err != nil {
				return fmt.Errorf("querying software inventory: %w", err)
			}
			type row struct{ title, version, tenant, computer string }
			all := make([]row, 0)
			for rows.Next() {
				var title, version, tenant, computer sql.NullString
				if err := rows.Scan(&title, &version, &tenant, &computer); err != nil {
					_ = rows.Close()
					return fmt.Errorf("scanning inventory: %w", err)
				}
				all = append(all, row{nullStr(title), nullStr(version), nullStr(tenant), nullStr(computer)})
			}
			if err := rows.Err(); err != nil {
				_ = rows.Close()
				return fmt.Errorf("iterating inventory: %w", err)
			}
			if err := rows.Close(); err != nil {
				return fmt.Errorf("closing inventory: %w", err)
			}

			fleetLatest := map[string]string{}
			type tk struct{ tenant, title string }
			tenantHighest := map[tk]string{}
			tenantComputers := map[tk]map[string]struct{}{}
			tenantTitles := map[string]map[string]struct{}{}

			for _, r := range all {
				view.ScannedInstalls++
				if r.title == "" || r.version == "" || r.tenant == "" {
					continue
				}
				if cur, ok := fleetLatest[r.title]; !ok || immyCompareVersions(r.version, cur) > 0 {
					fleetLatest[r.title] = r.version
				}
				k := tk{r.tenant, r.title}
				if cur, ok := tenantHighest[k]; !ok || immyCompareVersions(r.version, cur) > 0 {
					tenantHighest[k] = r.version
				}
				if tenantComputers[k] == nil {
					tenantComputers[k] = map[string]struct{}{}
				}
				if r.computer != "" {
					tenantComputers[k][r.computer] = struct{}{}
				}
				if tenantTitles[r.tenant] == nil {
					tenantTitles[r.tenant] = map[string]struct{}{}
				}
				tenantTitles[r.tenant][r.title] = struct{}{}
			}
			view.TitlesTracked = len(fleetLatest)

			filter := strings.ToLower(strings.TrimSpace(flagTenant))
			for tenant, titles := range tenantTitles {
				if filter != "" && !strings.Contains(strings.ToLower(tenant), filter) {
					continue
				}
				dt := driftTenant{Tenant: tenant, TitlesTracked: len(titles), Behind: make([]driftTitle, 0)}
				for title := range titles {
					k := tk{tenant, title}
					have := tenantHighest[k]
					want := fleetLatest[title]
					if want == "" || have == "" || immyCompareVersions(have, want) >= 0 {
						continue
					}
					dt.Behind = append(dt.Behind, driftTitle{
						Title: title, TenantVersion: have, FleetLatest: want,
						ComputerCount: len(tenantComputers[k]),
					})
				}
				dt.TitlesBehind = len(dt.Behind)
				sort.Slice(dt.Behind, func(i, j int) bool { return dt.Behind[i].Title < dt.Behind[j].Title })
				view.Tenants = append(view.Tenants, dt)
			}
			sort.Slice(view.Tenants, func(i, j int) bool {
				if view.Tenants[i].TitlesBehind != view.Tenants[j].TitlesBehind {
					return view.Tenants[i].TitlesBehind > view.Tenants[j].TitlesBehind
				}
				return view.Tenants[i].Tenant < view.Tenants[j].Tenant
			})
			if flagLimit > 0 && len(view.Tenants) > flagLimit {
				view.Tenants = view.Tenants[:flagLimit]
			}
			if view.ScannedInstalls == 0 {
				view.Note = "no software inventory in the local mirror; run 'immybot-pp-cli sync --resources tenants-software-from-inventory-dx'"
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			if len(view.Tenants) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), firstNonEmpty(view.Note, "No tenants matched."))
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-34s %-8s %s\n", "TENANT", "BEHIND", "TRACKED")
			for _, t := range view.Tenants {
				fmt.Fprintf(cmd.OutOrStdout(), "%-34s %-8d %d\n", immyTruncate(t.Tenant, 34), t.TitlesBehind, t.TitlesTracked)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagTenant, "tenant", "", "Only report tenants whose name contains this substring")
	cmd.Flags().IntVar(&flagLimit, "limit", 50, "Maximum tenants to return (0 for all)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local mirror database path")
	return cmd
}

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		addNovelCommandIfAbsent(root, newNovelDriftCmd(flags))
	})
}
