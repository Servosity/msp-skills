// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// pullView is the stable JSON envelope pull emits. Rows are extracted from
// the (undocumented) upstream envelope when a known shape matches; otherwise
// the raw payload is preserved under `data` so nothing is silently dropped.
type pullView struct {
	Alias        string            `json:"alias,omitempty"`
	ResourceID   string            `json:"resource_id"`
	ResourceType string            `json:"resource_type"`
	Page         int               `json:"page"`
	PageSize     int               `json:"page_size"`
	Filters      map[string]string `json:"filters,omitempty"`
	RowCount     int               `json:"row_count"`
	Rows         []json.RawMessage `json:"rows,omitempty"`
	Data         json.RawMessage   `json:"data,omitempty"`
}

func newNovelPullCmd(flags *rootFlags) *cobra.Command {
	var flagWhere []string
	var page int
	var pageSize int
	var typeOverride string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "pull [alias-or-resourceId]",
		Short: "Fetch one page of a dataset or widget with readable --where filters",
		Long: strings.Trim(`
Fetch one page of rows from a registered alias (or raw resource ID), compiling
readable --where predicates into MSPbots' comma-encoded operator DSL:

  "Status = Open"               equals / contains (server decides by type)
  "Update Date >= 2026-05-01"   later than
  "Price <= 56.3"               earlier than / at most
  "Price between 12.6,56.3"     inclusive range (A,B or A,B,C,D intervals)
  "Name contains Tod"           text contains
  "Status is empty"             empty value

Column names may contain spaces — quote the whole predicate. Use 'describe'
first to discover column names and types. For full-table dumps use 'export';
for stored history use 'snapshot'/'trend'.`, "\n"),
		Example: strings.Trim(`
  mspbots-cli pull open-tickets --page-size 10 --json
  mspbots-cli pull open-tickets --where "Update Date >= 2026-05-01" --where "Status = Open" --json
  mspbots-cli pull 1534956341424005122 --type widget --json
  mspbots-cli pull open-tickets --agent --select row_count,rows`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
			"pp:happy-args": "alias-or-resourceId=1534956341424005122;--type=dataset",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if len(args) < 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("pull needs <alias-or-resourceId>"))
			}
			if dryRunOK(flags) {
				fmt.Fprintf(cmd.OutOrStdout(), "would pull page %d of %q\n", page, args[0])
				return nil
			}
			if err := validateDataSourceStrategy(flags, "live"); err != nil {
				return usageErr(err)
			}
			if err := validateResourceType(typeOverride, true); err != nil {
				return usageErr(err)
			}
			filters, err := compileWhere(flagWhere)
			if err != nil {
				return usageErr(err)
			}
			db, err := openMspbotsStore(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			res, err := resolveResource(cmd.Context(), db, args[0], typeOverride)
			if err != nil {
				return err
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			raw, err := fetchResourcePage(cmd.Context(), c, res, page, pageSize, filters)
			if err != nil {
				return fmt.Errorf("fetching %s %s: %w", res.ResourceType, res.ResourceID, err)
			}
			view := pullView{
				Alias:        res.Alias,
				ResourceID:   res.ResourceID,
				ResourceType: res.ResourceType,
				Page:         page,
				PageSize:     pageSize,
				Filters:      filters,
			}
			if rows, ok := extractRows(raw); ok {
				view.Rows = rows
				view.RowCount = len(rows)
			} else {
				view.Data = raw
			}
			return flags.printJSON(cmd, view)
		},
	}
	// StringArrayVar, not StringSliceVar: slice mode comma-splits a single
	// value, which would shred "Col between A,B" into two broken predicates.
	cmd.Flags().StringArrayVar(&flagWhere, "where", nil, `Readable filter predicate, repeatable (e.g. --where "Update Date >= 2026-05-01")`)
	cmd.Flags().IntVar(&page, "page", 1, "Page number, 1-based (wire key: current)")
	cmd.Flags().IntVar(&pageSize, "page-size", 50, "Rows per page (wire key: size)")
	cmd.Flags().StringVar(&typeOverride, "type", "", "Resource type when passing a raw ID: dataset or widget (default dataset)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path for alias resolution (defaults to the CLI's local store)")
	return cmd
}
