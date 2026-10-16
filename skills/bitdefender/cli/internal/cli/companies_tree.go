// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

type companiesTreeView struct {
	Companies     []gzCompany    `json:"companies"`
	Total         int            `json:"total"`
	Partners      int            `json:"partners"`
	Customers     int            `json:"customers"`
	FetchFailures []fetchFailure `json:"fetch_failures"`
	Note          string         `json:"note,omitempty"`
}

func newNovelCompaniesTreeCmd(flags *rootFlags) *cobra.Command {
	var maxCompanies int
	cmd := &cobra.Command{
		Use:   "tree",
		Short: "List your own company plus every child company and sub-partner as one flat list with id, type, parent and depth",
		Long: `Walks the partner company tree breadth-first (getCompaniesList answers one level per
call) across sub-partners, customers and company folders, and returns one flat list with
id, name, type (0 partner, 1 customer), parent id, depth and path. The first row (depth 0)
is the API key's own company. The tree is cached
locally so the fleet commands reuse it within --max-age.

Use this command to list every child company and sub-partner under your partner account
with ids, type, parent and depth. Do NOT use this command for endpoint counts or protection
status per company; use 'fleet health' instead.`,
		Example:     "  bitdefender-cli companies tree --agent\n  bitdefender-cli companies tree --csv",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "companies tree")
			}
			// No whole-command deadline: a fan-out over many companies runs far
			// longer than --timeout, which the generated client applies per request.
			ctx := cmd.Context()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			s, err := openGZStore(ctx)
			if err != nil {
				return err
			}
			defer s.Close()
			tree, failures, err := walkCompanyTree(ctx, c, maxCompanies)
			if err != nil && len(tree) == 0 {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			view := companiesTreeView{Companies: tree, Total: len(tree), FetchFailures: make([]fetchFailure, 0)}
			view.FetchFailures = append(view.FetchFailures, failures...)
			switch {
			case err != nil:
				view.Note = err.Error()
			case len(failures) > 0:
				view.Note = "tree incomplete (see fetch_failures); not cached for the fleet commands"
			default:
				if serr := saveCompanyTree(ctx, s.DB(), tree); serr != nil {
					view.Note = "tree not cached: " + serr.Error()
				}
			}
			for _, co := range tree {
				if co.Type == 0 {
					view.Partners++
				} else {
					view.Customers++
				}
			}
			if len(view.FetchFailures) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d tree calls failed; the list may be missing companies below them\n", len(view.FetchFailures))
			}
			if wantsHumanTable(cmd.OutOrStdout(), flags) {
				humanNote(cmd, view.Note)
				rows := make([]map[string]any, 0, len(tree))
				for _, co := range tree {
					kind := "customer"
					if co.Type == 0 {
						kind = "partner"
					}
					rows = append(rows, map[string]any{"id": co.ID, "type": kind, "depth": co.Depth, "path": co.Path})
				}
				return printAutoTable(cmd.OutOrStdout(), rows)
			}
			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().IntVar(&maxCompanies, "max-companies", 5000, "Stop walking (with an error note) after this many companies")
	return cmd
}
