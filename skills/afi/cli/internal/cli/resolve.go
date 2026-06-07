// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command: resolve.
// pp:data-source local

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

type resolveMatch struct {
	ObjectType string `json:"object_type"` // resource | tenant | org
	ID         string `json:"id"`
	ExternalID string `json:"external_id"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	TenantID   string `json:"tenant_id,omitempty"`
	OrgID      string `json:"org_id,omitempty"`
	Region     string `json:"region,omitempty"`
	MatchedOn  string `json:"matched_on"` // external_id | id | name
}

type resolveView struct {
	Query    string         `json:"query"`
	Matches  []resolveMatch `json:"matches"`
	MultiGeo bool           `json:"multi_geo"`
	Note     string         `json:"note,omitempty"`
}

func newNovelResolveCmd(flags *rootFlags) *cobra.Command {
	var flagKind string
	var flagLimit int
	var flagDB string

	cmd := &cobra.Command{
		Use:   "resolve <id-or-email-or-name>",
		Short: "Map a Microsoft 365 / Google Workspace ID, email, or name to the canonical Afi resource, tenant, or org",
		Long: strings.TrimSpace(`
Look up Afi objects in the local store by external ID (the stable M365/GWS
identifier), Afi ID, or name/email substring. Afi's docs warn that names and
emails are ambiguous keys, and one external ID can map to several per-region
Afi tenants (Multi-Geo) — every object sharing the identifier is returned.

Use this command to find the Afi resource/tenant behind an M365/GWS ID, email, or name before an offboard or audit.
Do NOT use this command to run the offboard itself; use 'offboard' instead.
Do NOT use this command for free-text search across all synced fields; use 'search' instead.`),
		Example: strings.Trim(`
  # Who is jane.doe@contoso.com in Afi?
  afi-cli resolve jane.doe@contoso.com --json

  # Resolve an M365 directory object ID, users only
  afi-cli resolve 7f3a1b2c-0000-4d5e-8aaa-9c1d2e3f4a5b --kind user --agent
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
			"pp:happy-args": "id-or-email-or-name=example-user@example.com",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would resolve the identifier against local resources, tenants, and orgs")
				return nil
			}
			if err := errNoLiveEquivalent(flags, "resolve"); err != nil {
				return err
			}
			if len(args) < 1 || strings.TrimSpace(args[0]) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("an identifier (external ID, Afi ID, email, or name) is required"))
			}
			query := strings.TrimSpace(args[0])
			db, err := openAfiStore(flagDB)
			if err != nil {
				return err
			}
			defer db.Close()
			fleetHint(cmd, db, "resources", flags)

			sqlQ := `
				SELECT resource_type, id,
				       COALESCE(json_extract(data,'$.external_id'), ''),
				       COALESCE(json_extract(data,'$.name'), ''),
				       COALESCE(json_extract(data,'$.kind'), ''),
				       COALESCE(json_extract(data,'$.tenant_id'), ''),
				       COALESCE(json_extract(data,'$.org_id'), ''),
				       COALESCE(json_extract(data,'$.region'), ''),
				       CASE
				         WHEN json_extract(data,'$.external_id') = ?1 THEN 'external_id'
				         WHEN id = ?1 THEN 'id'
				         ELSE 'name'
				       END AS matched_on
				FROM resources
				WHERE resource_type IN ('resources','tenants','orgs')
				  AND (json_extract(data,'$.external_id') = ?1
				       OR id = ?1
				       OR json_extract(data,'$.name') LIKE '%' || ?1 || '%')
				ORDER BY matched_on, resource_type, id
				LIMIT ?2`
			rows, err := db.DB().QueryContext(cmd.Context(), sqlQ, query, flagLimit)
			if err != nil {
				return fmt.Errorf("resolving %q: %w", query, err)
			}
			defer rows.Close()

			typeName := map[string]string{"resources": "resource", "tenants": "tenant", "orgs": "org"}
			matches := make([]resolveMatch, 0)
			externalTenants := map[string]map[string]bool{} // external_id -> tenant ids
			for rows.Next() {
				var m resolveMatch
				var rtype string
				if err := rows.Scan(&rtype, &m.ID, &m.ExternalID, &m.Name, &m.Kind, &m.TenantID, &m.OrgID, &m.Region, &m.MatchedOn); err != nil {
					return fmt.Errorf("scanning resolve row: %w", err)
				}
				m.ObjectType = typeName[rtype]
				if flagKind != "" && m.Kind != flagKind {
					continue
				}
				matches = append(matches, m)
				if m.ObjectType == "tenant" && m.ExternalID != "" {
					if externalTenants[m.ExternalID] == nil {
						externalTenants[m.ExternalID] = map[string]bool{}
					}
					externalTenants[m.ExternalID][m.ID] = true
				}
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("iterating resolve rows: %w", err)
			}

			view := resolveView{Query: query, Matches: matches}
			for _, tids := range externalTenants {
				if len(tids) > 1 {
					view.MultiGeo = true
					view.Note = "multiple Afi tenants share this external ID (Multi-Geo); operations must target each per-region tenant separately"
				}
			}
			if len(matches) == 0 {
				view.Note = "no local match; run 'afi-cli fleet-sync' to refresh the store, or try 'afi-cli search' for free-text matching"
			}
			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().StringVar(&flagKind, "kind", "", "Restrict matches to one kind (e.g. user, site, o365, gsuite)")
	cmd.Flags().IntVar(&flagLimit, "limit", 50, "Maximum matches to return")
	cmd.Flags().StringVar(&flagDB, "db", "", "Database path (default: the CLI's local store)")
	return cmd
}
