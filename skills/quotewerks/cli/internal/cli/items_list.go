// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"quotewerks-pp-cli/internal/qw"
)

func newItemsListCmd(flags *rootFlags) *cobra.Command {
	var doc, part, text string
	var limit int
	var allFields bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "Find quote lines by document, part number or description",
		Long: `Find DocumentItems lines on one document (--doc), by manufacturer part number
(--part) across all documents, or by description text (--text). Lines carry their
DocRecGUID so 'docs get' can open the parent document.`,
		Example: strings.Trim(`
  quotewerks-cli items list --doc 1042 --json
  quotewerks-cli items list --part 20UES0X100 --limit 20 --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "items list")
			}
			if doc == "" && part == "" && text == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("pass --doc, --part or --text"))
			}
			if limit <= 0 {
				return usageErr(fmt.Errorf("--limit must be positive"))
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			filters := make([]map[string]any, 0)
			if doc != "" {
				d, err := resolveDoc(ctx, c, doc)
				if err != nil {
					return err
				}
				filters = append(filters, qw.Cond("DocRecGUID", "eq", d.ID))
			}
			if part != "" {
				filters = append(filters, qw.Cond("ManufacturerPartNumber", "eq", part))
			}
			if text != "" {
				filters = append(filters, qw.Cond("Description", "contains", text))
			}
			rows, _, err := qwSearch(ctx, c, qw.TableItems, qwSearchOpts{filters: filters, limit: limit, maxPages: qwAllPages})
			if err != nil {
				return asUsage(err)
			}
			lines := make([]map[string]any, 0, len(rows))
			for _, r := range rows {
				lines = append(lines, r.Flat())
			}
			sortLines(lines)
			out := make([]map[string]any, 0, len(lines))
			for _, l := range lines {
				out = append(out, decorateLine(l, allFields))
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), out, flags)
			}
			if len(out) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No matching lines.")
				return nil
			}
			table := make([]map[string]any, 0, len(out))
			for _, l := range out {
				table = append(table, project(l, []string{"line_type", "ManufacturerPartNumber", "Description", "QtyBase", "UnitCost", "UnitPrice", "margin_pct"}))
			}
			return printAutoTable(cmd.OutOrStdout(), table)
		},
	}
	cmd.Flags().StringVar(&doc, "doc", "", "Lines on this document (DocNo or RecGUID)")
	cmd.Flags().StringVar(&part, "part", "", "Exact manufacturer part number (ManufacturerPartNumber)")
	cmd.Flags().StringVar(&text, "text", "", "Description contains this text")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum lines to return")
	cmd.Flags().BoolVar(&allFields, "all-fields", false, "Return every DocumentItems column instead of the key fields")
	return cmd
}
