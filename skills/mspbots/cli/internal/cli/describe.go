// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// pp:data-source live

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

type describeView struct {
	Alias        string       `json:"alias,omitempty"`
	ResourceID   string       `json:"resource_id"`
	ResourceType string       `json:"resource_type"`
	SampledRows  int          `json:"sampled_rows"`
	Columns      []columnInfo `json:"columns"`
	Note         string       `json:"note,omitempty"`
}

func newNovelDescribeCmd(flags *rootFlags) *cobra.Command {
	var sampleSize int
	var typeOverride string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "describe [alias-or-resourceId]",
		Short: "Sample live rows and infer the column names and types of a dataset or widget",
		Long: strings.Trim(`
Samples one page of live rows and mechanically infers each column's name,
type (number/date/boolean/text/object/array), null rate, and an example
value — the Public API has no metadata endpoint, so this is how --where
targets become discoverable. Run it before building 'pull' filters.`, "\n"),
		Example: strings.Trim(`
  mspbots-cli describe open-tickets --json
  mspbots-cli describe 1534956341424005122 --type widget --json`, "\n"),
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
				return usageErr(fmt.Errorf("describe needs <alias-or-resourceId>"))
			}
			if dryRunOK(flags) {
				fmt.Fprintf(cmd.OutOrStdout(), "would sample %d rows of %q and infer the schema\n", sampleSize, args[0])
				return nil
			}
			if err := validateDataSourceStrategy(flags, "live"); err != nil {
				return usageErr(err)
			}
			if err := validateResourceType(typeOverride, true); err != nil {
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
			raw, err := fetchResourcePage(cmd.Context(), c, res, 1, sampleSize, nil)
			if err != nil {
				return fmt.Errorf("sampling %s %s: %w", res.ResourceType, res.ResourceID, err)
			}
			view := describeView{
				Alias:        res.Alias,
				ResourceID:   res.ResourceID,
				ResourceType: res.ResourceType,
			}
			rows, ok := extractRows(raw)
			if !ok {
				view.Note = "response shape not recognized as rows; raw payload may be a non-tabular widget — inspect with 'pull'"
				return flags.printJSON(cmd, view)
			}
			view.SampledRows = len(rows)
			view.Columns = inferColumns(rows)
			if len(rows) == sampleSize {
				view.Note = fmt.Sprintf("types inferred from the first %d rows; rare variants past the sample may differ", sampleSize)
			}
			return flags.printJSON(cmd, view)
		},
	}
	cmd.Flags().IntVar(&sampleSize, "sample", 50, "Rows to sample for inference (one page)")
	cmd.Flags().StringVar(&typeOverride, "type", "", "Resource type when passing a raw ID: dataset or widget (default dataset)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path for alias resolution (defaults to the CLI's local store)")
	return cmd
}
