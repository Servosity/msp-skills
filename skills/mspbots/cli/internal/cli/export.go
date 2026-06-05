// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// pp:data-source live

package cli

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"mspbots-pp-cli/internal/cliutil"
)

// exportSummary is printed to stderr (or as JSON with --out) after a stream
// completes, so pipes receive only data rows on stdout.
type exportSummary struct {
	Alias        string `json:"alias,omitempty"`
	ResourceID   string `json:"resource_id"`
	ResourceType string `json:"resource_type"`
	Format       string `json:"format"`
	Rows         int    `json:"rows"`
	Pages        int    `json:"pages"`
	MaxPagesHit  bool   `json:"max_pages_hit"`
	Out          string `json:"out,omitempty"`
	Note         string `json:"note,omitempty"`
}

func newNovelExportCmd(flags *rootFlags) *cobra.Command {
	var flagFormat string
	var flagWhere []string
	var outPath string
	var maxPages int
	var pageSize int
	var typeOverride string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "export [alias-or-resourceId]",
		Short: "Dump an entire dataset or widget to CSV or JSONL, walking every page automatically",
		Long: strings.Trim(`
Use this command to dump an entire dataset/widget to a file or stdout.
Do NOT use it for a single page or quick peek; use 'pull' with --page/--page-size instead.

Walks the Public API's current/size pagination until a short or empty page,
streaming rows as JSONL (default) or CSV. Scan effort is bounded separately
from output: --max-pages caps how many pages are fetched, and the summary
reports whether the cap was hit so an incomplete dump is never mistaken for
a complete one.`, "\n"),
		Example: strings.Trim(`
  mspbots-cli export open-tickets --format csv > tickets.csv
  mspbots-cli export open-tickets --format jsonl --out tickets.jsonl
  mspbots-cli export open-tickets --where "Status = Open" --format csv --max-pages 10`, "\n"),
		Annotations: map[string]string{
			"pp:happy-args": "alias-or-resourceId=1534956341424005122;--format=jsonl;--max-pages=1",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if len(args) < 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("export needs <alias-or-resourceId>"))
			}
			if dryRunOK(flags) {
				fmt.Fprintf(cmd.OutOrStdout(), "would export %q as %s (up to %d pages of %d rows)\n", args[0], flagFormat, maxPages, pageSize)
				return nil
			}
			if err := validateDataSourceStrategy(flags, "live"); err != nil {
				return usageErr(err)
			}
			if flagFormat != "jsonl" && flagFormat != "csv" {
				return usageErr(fmt.Errorf("--format must be jsonl or csv, got %q", flagFormat))
			}
			if err := validateResourceType(typeOverride, true); err != nil {
				return usageErr(err)
			}
			filters, err := compileWhere(flagWhere)
			if err != nil {
				return usageErr(err)
			}
			if cliutil.IsDogfoodEnv() && maxPages > 1 {
				maxPages = 1
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

			var w io.Writer = cmd.OutOrStdout()
			if outPath != "" {
				// #nosec G304 -- outPath is the user's explicit --out destination; writing the export to a caller-chosen file is this flag's whole purpose.
				f, err := os.Create(outPath)
				if err != nil {
					return fmt.Errorf("creating %s: %w", outPath, err)
				}
				defer f.Close()
				w = f
			}

			summary := exportSummary{
				Alias:        res.Alias,
				ResourceID:   res.ResourceID,
				ResourceType: res.ResourceType,
				Format:       flagFormat,
				Out:          outPath,
			}
			var csvW *csv.Writer
			var csvCols []string
			pages, maxPagesHit, err := walkResourcePages(cmd.Context(), c, res, maxPages, pageSize, filters, func(rows []json.RawMessage) error {
				for _, row := range rows {
					switch flagFormat {
					case "jsonl":
						if _, err := w.Write(append(canonicalRowJSON(row), '\n')); err != nil {
							return fmt.Errorf("writing row: %w", err)
						}
					case "csv":
						if csvW == nil {
							csvW = csv.NewWriter(w)
							csvCols = csvColumnsFromRow(row)
							if err := csvW.Write(csvCols); err != nil {
								return fmt.Errorf("writing CSV header: %w", err)
							}
						}
						if err := csvW.Write(csvRecordForRow(row, csvCols)); err != nil {
							return fmt.Errorf("writing CSV row: %w", err)
						}
					}
					summary.Rows++
				}
				return nil
			})
			if err != nil {
				return err
			}
			summary.Pages = pages
			if csvW != nil {
				csvW.Flush()
				if err := csvW.Error(); err != nil {
					return fmt.Errorf("flushing CSV: %w", err)
				}
			}
			summary.MaxPagesHit = maxPagesHit
			if maxPagesHit {
				summary.Note = fmt.Sprintf("stopped at --max-pages %d with full pages still coming; raise --max-pages for a complete dump", maxPages)
			}
			if outPath != "" {
				return flags.printJSON(cmd, summary)
			}
			payload, err := json.Marshal(summary)
			if err != nil {
				return fmt.Errorf("marshaling summary: %w", err)
			}
			fmt.Fprintln(cmd.ErrOrStderr(), "export summary: "+string(payload))
			return nil
		},
	}
	cmd.Flags().StringVar(&flagFormat, "format", "jsonl", "Output format: jsonl or csv")
	// StringArrayVar, not StringSliceVar: slice mode comma-splits a single
	// value, which would shred "Col between A,B" into two broken predicates.
	cmd.Flags().StringArrayVar(&flagWhere, "where", nil, `Readable filter predicate, repeatable (e.g. --where "Status = Open")`)
	cmd.Flags().StringVar(&outPath, "out", "", "Write to a file instead of stdout (summary then prints as JSON on stdout)")
	cmd.Flags().IntVar(&maxPages, "max-pages", 40, "Maximum pages to fetch before returning a partial dump")
	cmd.Flags().IntVar(&pageSize, "page-size", 200, "Rows per page request (wire key: size)")
	cmd.Flags().StringVar(&typeOverride, "type", "", "Resource type when passing a raw ID: dataset or widget (default dataset)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path for alias resolution (defaults to the CLI's local store)")
	return cmd
}

// csvColumnsFromRow derives the header from the first row's keys, sorted for
// determinism. Later rows with extra keys keep only the header's columns —
// the alternative (rescanning all pages first) would defeat streaming.
func csvColumnsFromRow(row json.RawMessage) []string {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(row, &obj); err != nil {
		return []string{"_raw"}
	}
	cols := make([]string, 0, len(obj))
	for k := range obj {
		cols = append(cols, k)
	}
	sort.Strings(cols)
	return cols
}

func csvRecordForRow(row json.RawMessage, cols []string) []string {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(row, &obj); err != nil {
		return []string{string(row)}
	}
	rec := make([]string, len(cols))
	for i, c := range cols {
		v, ok := obj[c]
		if !ok {
			continue
		}
		s := strings.TrimSpace(string(v))
		if s == "null" {
			continue
		}
		// Unquote plain strings; keep nested JSON verbatim.
		var asString string
		if err := json.Unmarshal(v, &asString); err == nil {
			rec[i] = asString
		} else {
			rec[i] = s
		}
	}
	return rec
}
