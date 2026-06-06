// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel feature. See internal/cli/itglue_records.go for shared helpers.
// New in the 2026-06-06 reprint: orphaned-record detection across the local mirror.

package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

// orphanRow is one child record whose owning organization is not in the local
// store (deleted, renamed, or never synced).
type orphanRow struct {
	ResourceType   string `json:"resource_type"`
	ID             string `json:"id"`
	Name           string `json:"name"`
	OrganizationID string `json:"organization_id"`
	UpdatedAt      string `json:"updated_at,omitempty"`
}

// orphansView is the command's JSON envelope.
type orphansView struct {
	Orphans      []orphanRow `json:"orphans"`
	ScannedTotal int         `json:"scanned_records"`
	OrgsSynced   int         `json:"organizations_synced"`
	Note         string      `json:"note,omitempty"`
}

// pp:data-source local
func newNovelOrphansCmd(flags *rootFlags) *cobra.Command {
	var flagResource string

	cmd := &cobra.Command{
		Use:   "orphans",
		Short: "Find records whose owning organization is missing from the local store",
		Long: `Find configurations, contacts, passwords, and documents whose organization-id
no longer resolves to a synced organization — the documentation left dangling
after a client is offboarded or an org record is deleted.

A local anti-join across the synced mirror; makes zero API calls. No IT Glue
endpoint, wrapper, or MCP can list records whose parent organization is gone.
Run after client offboarding or org renames to catch dangling documentation
before it misleads a technician.`,
		Example: `  # All orphaned records, grouped by resource type
  itglue-cli orphans

  # Only orphaned configurations, as JSON for an agent
  itglue-cli orphans --resource configurations --agent`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			childTypes := []string{"contacts", "configurations", "passwords", "documents"}
			if flagResource != "" {
				found := false
				for _, rt := range childTypes {
					if rt == flagResource {
						found = true
						break
					}
				}
				if !found {
					_ = cmd.Usage()
					return usageErr(fmt.Errorf("--resource must be one of contacts, configurations, passwords, documents (got %q)", flagResource))
				}
				childTypes = []string{flagResource}
			}

			db, err := openITGStore(cmd)
			if err != nil {
				return configErr(err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "") {
				hintIfStale(cmd, db, "", flags.maxAge)
			}

			orgs, err := listITGRecords(db, "organizations")
			if err != nil {
				return apiErr(fmt.Errorf("reading organizations: %w", err))
			}
			known := make(map[string]struct{}, len(orgs))
			for _, o := range orgs {
				known[o.ID] = struct{}{}
			}

			view := orphansView{Orphans: []orphanRow{}, OrgsSynced: len(orgs)}
			for _, rt := range childTypes {
				recs, err := listITGRecords(db, rt)
				if err != nil {
					return apiErr(fmt.Errorf("reading %s: %w", rt, err))
				}
				view.ScannedTotal += len(recs)
				// With zero orgs synced every child would look orphaned; that is a
				// sync gap, not data rot. Counted but reported via Note below.
				if len(orgs) == 0 {
					continue
				}
				for _, rec := range recs {
					oid := rec.orgID()
					if oid == "" {
						continue // org-less records are unassigned, not orphaned
					}
					if _, ok := known[oid]; ok {
						continue
					}
					view.Orphans = append(view.Orphans, orphanRow{
						ResourceType:   rec.Type,
						ID:             rec.ID,
						Name:           rec.displayName(),
						OrganizationID: oid,
						UpdatedAt:      rec.updatedAtString(),
					})
				}
			}
			if view.OrgsSynced == 0 && view.ScannedTotal > 0 {
				view.Note = "no organizations synced — every child record would look orphaned; run 'itglue-cli sync --full' first"
			}

			sort.Slice(view.Orphans, func(i, j int) bool {
				if view.Orphans[i].ResourceType != view.Orphans[j].ResourceType {
					return view.Orphans[i].ResourceType < view.Orphans[j].ResourceType
				}
				return view.Orphans[i].ID < view.Orphans[j].ID
			})

			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().StringVar(&flagResource, "resource", "", "Limit the scan to one resource type (contacts, configurations, passwords, documents)")
	return cmd
}
