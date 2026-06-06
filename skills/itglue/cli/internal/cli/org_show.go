// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel feature. See internal/cli/itglue_records.go for shared helpers.
// Reprint 2026-06-06: reframed from `organizations brief` to `org show` so the
// command reads as the tech's verb ("show me this org") instead of an IT Glue
// document type.

package cli

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

// pp:data-source local
func newNovelOrgShowCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show [organization-id]",
		Short: "Assemble everything known about one client in a single offline read",
		Long: `Assemble a one-shot picture of a client from the local store: its contacts,
configurations, password metadata, and documents — in a single offline read.

Replaces the IT Glue API fan-out (org, then its configurations, then contacts,
then passwords, then documents) with one local multi-table query that makes zero
API calls. Password secret values are never read.

Use this command to pull the full local picture for ONE known org id. Do NOT use
it to find which org owns an identifier; use 'search' instead. Do NOT use it to
compare completeness across many orgs; use 'coverage' instead.`,
		Example: `  # Everything known about one client
  itglue-cli org show 12345

  # As JSON for an agent
  itglue-cli org show 12345 --agent`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if len(args) == 0 {
				return cmd.Help()
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			orgID := args[0]

			db, err := openITGStore(cmd)
			if err != nil {
				return configErr(err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "") {
				hintIfStale(cmd, db, "", flags.maxAge)
			}

			header := map[string]any{"id": orgID, "found": false}
			if raw, err := db.Get("organizations", orgID); err == nil {
				if rec, ok := parseITGRecord(raw); ok {
					header["found"] = true
					header["name"] = rec.displayName()
					itgAddIf(header, "organization_type", rec.attr("organization-type-name"))
					itgAddIf(header, "organization_status", rec.attr("organization-status-name"))
				}
			} else if !errors.Is(err, sql.ErrNoRows) {
				return apiErr(fmt.Errorf("reading organization %s: %w", orgID, err))
			}

			sections := map[string][]map[string]any{
				"contacts":       {},
				"configurations": {},
				"passwords":      {},
				"documents":      {},
			}
			for _, rt := range []string{"contacts", "configurations", "passwords", "documents"} {
				recs, err := listITGRecords(db, rt)
				if err != nil {
					return apiErr(fmt.Errorf("reading %s: %w", rt, err))
				}
				for _, rec := range recs {
					if rec.orgID() == orgID {
						sections[rt] = append(sections[rt], rec.summary())
					}
				}
			}

			brief := map[string]any{
				"organization": header,
				"counts": map[string]int{
					"contacts":       len(sections["contacts"]),
					"configurations": len(sections["configurations"]),
					"passwords":      len(sections["passwords"]),
					"documents":      len(sections["documents"]),
				},
				"contacts":       sections["contacts"],
				"configurations": sections["configurations"],
				"passwords":      sections["passwords"],
				"documents":      sections["documents"],
			}
			return printJSONFiltered(cmd.OutOrStdout(), brief, flags)
		},
	}
	return cmd
}
