// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

type softwareRollupRow struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Publisher string `json:"publisher"`
	Installs  int    `json:"installs"`
	Orgs      int    `json:"organizations"`
	Endpoints int    `json:"endpoints"`
}

// pp:data-source local
func newNovelFleetSoftwareRollupCmd(flags *rootFlags) *cobra.Command {
	var dbPath, nameFilter, orgFilter string
	var limit int

	cmd := &cobra.Command{
		Use:         "software-rollup",
		Short:       "Roll up installed software across the whole fleet by name and version.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Long: `Fleet-wide installed-software inventory. Action1 reports installed software
per endpoint within one organization. This deduplicates locally synced
installed-software records across every organization into name x version rows
with install counts and the number of organizations and endpoints affected —
the asset-management view the org-scoped API cannot produce.`,
		Example: `  action1-cli fleet software-rollup --agent --limit 50
  action1-cli fleet software-rollup --name "Google Chrome"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			db, err := fleetOpenStore(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			maybeEmitSyncHints(cmd, db, "installed_software_data", flags.maxAge)

			apps, err := fleetLoadAll(cmd.Context(), db, "installed_software_data")
			if err != nil {
				return err
			}

			type agg struct {
				row       softwareRollupRow
				orgs      map[string]bool
				endpoints map[string]bool
			}
			byKey := map[string]*agg{}
			nf := strings.ToLower(strings.TrimSpace(nameFilter))
			for _, a := range apps {
				name := firstNonEmpty(a, "name", "app_name", "application", "title", "product", "software")
				if name == "" {
					continue
				}
				if nf != "" && !strings.Contains(strings.ToLower(name), nf) {
					continue
				}
				org := fleetOrgID(a)
				if orgFilter != "" && org != orgFilter {
					continue
				}
				version := firstNonEmpty(a, "version", "app_version", "product_version")
				publisher := firstNonEmpty(a, "publisher", "vendor", "manufacturer", "company")
				endpoint := firstNonEmpty(a, "endpoint_id", "endpoint", "endpoint_name", "device_name", "computer")

				key := strings.ToLower(name) + "\x00" + strings.ToLower(version)
				g := byKey[key]
				if g == nil {
					g = &agg{
						row:       softwareRollupRow{Name: name, Version: version, Publisher: publisher},
						orgs:      map[string]bool{},
						endpoints: map[string]bool{},
					}
					byKey[key] = g
				}
				g.row.Installs++
				if org != "" {
					g.orgs[org] = true
				}
				if endpoint != "" {
					g.endpoints[endpoint] = true
				}
			}

			rows := make([]softwareRollupRow, 0, len(byKey))
			for _, g := range byKey {
				g.row.Orgs = len(g.orgs)
				g.row.Endpoints = len(g.endpoints)
				rows = append(rows, g.row)
			}
			sort.SliceStable(rows, func(i, j int) bool {
				if rows[i].Installs != rows[j].Installs {
					return rows[i].Installs > rows[j].Installs
				}
				return rows[i].Name < rows[j].Name
			})
			if limit > 0 && len(rows) > limit {
				rows = rows[:limit]
			}

			header := []string{"NAME", "VERSION", "PUBLISHER", "INSTALLS", "ORGS", "ENDPOINTS"}
			matrix := make([][]string, 0, len(rows))
			for _, r := range rows {
				matrix = append(matrix, []string{r.Name, r.Version, r.Publisher,
					fleetItoa(float64(r.Installs)), fleetItoa(float64(r.Orgs)), fleetItoa(float64(r.Endpoints))})
			}
			return fleetEmit(cmd, flags, rows, header, matrix)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Local database path (default: standard cache location)")
	cmd.Flags().StringVar(&nameFilter, "name", "", "Only software whose name contains this text (case-insensitive)")
	cmd.Flags().StringVar(&orgFilter, "org", fleetEnvOrg(), "Limit to one organization ID (default: $ACTION1_ORG_ID)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum rows to return (0 = all)")
	return cmd
}

// firstNonEmpty returns the first non-empty field among the candidate keys.
func firstNonEmpty(obj map[string]any, keys ...string) string {
	for _, k := range keys {
		if v := fleetStrField(obj, k); v != "" {
			return v
		}
	}
	return ""
}
