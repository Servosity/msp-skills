// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel feature. See internal/cli/itglue_records.go for shared helpers.

package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

// coverageRow is one organization's documentation-completeness scorecard.
type coverageRow struct {
	OrganizationID string   `json:"organization_id"`
	Organization   string   `json:"organization"`
	Configurations int      `json:"configurations"`
	Contacts       int      `json:"contacts"`
	Passwords      int      `json:"passwords"`
	Documents      int      `json:"documents"`
	Total          int      `json:"total"`
	Missing        []string `json:"missing"`
}

// pp:data-source local
func newNovelCoverageCmd(flags *rootFlags) *cobra.Command {
	var flagBelow int
	var flagOrg string

	cmd := &cobra.Command{
		Use:   "coverage",
		Short: "Rank organizations by documentation completeness (offline scorecard)",
		Long: `Rank organizations by documentation completeness — per-org counts of
configurations, contacts, passwords, and documents — thinnest first.

Reads the local store only (run 'sync --full' first); makes no API calls, so it
is immune to IT Glue's 3000-requests/5-minute rate ceiling. No single API call
returns this view — it is a cross-resource join over every synced organization.`,
		Example: `  # Every organization, least-documented first
  itglue-cli coverage

  # Only organizations missing a whole documentation category
  itglue-cli coverage --below 1 --agent

  # One organization's scorecard
  itglue-cli coverage --org 12345`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}

			db, err := openITGStore(cmd)
			if err != nil {
				return configErr(err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "") {
				hintIfStale(cmd, db, "", flags.maxAge)
			}

			rows := map[string]*coverageRow{}
			ensure := func(id, name string) *coverageRow {
				if id == "" {
					return nil
				}
				r, ok := rows[id]
				if !ok {
					r = &coverageRow{OrganizationID: id, Organization: name}
					rows[id] = r
				}
				if r.Organization == "" && name != "" {
					r.Organization = name
				}
				return r
			}

			// Seed from organizations so empty clients still appear.
			orgs, err := listITGRecords(db, "organizations")
			if err != nil {
				return apiErr(fmt.Errorf("reading organizations: %w", err))
			}
			for _, o := range orgs {
				ensure(o.ID, o.displayName())
			}

			countInto := func(resource string, add func(*coverageRow)) error {
				recs, err := listITGRecords(db, resource)
				if err != nil {
					return apiErr(fmt.Errorf("reading %s: %w", resource, err))
				}
				for _, rec := range recs {
					if r := ensure(rec.orgID(), rec.orgName()); r != nil {
						add(r)
					}
				}
				return nil
			}
			if err := countInto("configurations", func(r *coverageRow) { r.Configurations++ }); err != nil {
				return err
			}
			if err := countInto("contacts", func(r *coverageRow) { r.Contacts++ }); err != nil {
				return err
			}
			if err := countInto("passwords", func(r *coverageRow) { r.Passwords++ }); err != nil {
				return err
			}
			if err := countInto("documents", func(r *coverageRow) { r.Documents++ }); err != nil {
				return err
			}

			out := make([]coverageRow, 0, len(rows))
			for _, r := range rows {
				if flagOrg != "" && r.OrganizationID != flagOrg {
					continue
				}
				r.Total = r.Configurations + r.Contacts + r.Passwords + r.Documents
				r.Missing = []string{}
				if r.Configurations == 0 {
					r.Missing = append(r.Missing, "configurations")
				}
				if r.Contacts == 0 {
					r.Missing = append(r.Missing, "contacts")
				}
				if r.Passwords == 0 {
					r.Missing = append(r.Missing, "passwords")
				}
				if r.Documents == 0 {
					r.Missing = append(r.Missing, "documents")
				}
				// --below N: keep only orgs where some category count is < N.
				if flagBelow > 0 {
					minCat := r.Configurations
					for _, c := range []int{r.Contacts, r.Passwords, r.Documents} {
						if c < minCat {
							minCat = c
						}
					}
					if minCat >= flagBelow {
						continue
					}
				}
				out = append(out, *r)
			}

			sort.Slice(out, func(i, j int) bool {
				if out[i].Total != out[j].Total {
					return out[i].Total < out[j].Total
				}
				return out[i].Organization < out[j].Organization
			})

			return printJSONFiltered(cmd.OutOrStdout(), out, flags)
		},
	}
	cmd.Flags().IntVar(&flagBelow, "below", 0, "Only show orgs where some documentation category has fewer than N records (e.g. --below 1 = missing a category)")
	cmd.Flags().StringVar(&flagOrg, "org", "", "Limit the scorecard to a single organization id")
	return cmd
}
