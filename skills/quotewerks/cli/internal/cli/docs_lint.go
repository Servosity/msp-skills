// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source auto

package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"quotewerks-pp-cli/internal/client"
	"quotewerks-pp-cli/internal/qw"
)

type lintInput struct {
	header          map[string]any
	lines           []map[string]any
	idx             *qw.ProductIndex
	source          string
	productsChecked bool
	partial         []string
}

func lintLive(ctx context.Context, c *client.Client, ref, productDB string, noProducts bool) (lintInput, error) {
	doc, err := resolveDoc(ctx, c, ref)
	if err != nil {
		return lintInput{}, err
	}
	lines, err := fetchItems(ctx, c, doc.ID)
	if err != nil {
		return lintInput{}, err
	}
	in := lintInput{header: doc.Flat(), lines: lines, source: "live"}
	if noProducts {
		return in, nil
	}
	tables, err := productTables(ctx, c, productDB)
	if err != nil {
		return lintInput{}, err
	}
	products, err := catalogLookup(ctx, c, tables, lineParts(lines))
	if err != nil {
		return lintInput{}, err
	}
	in.idx = qw.NewProductIndex(products)
	in.productsChecked = len(tables) > 0
	return in, nil
}

// lintLocal audits the mirrored copy. found is false when the document is not
// in the mirror.
func lintLocal(ctx context.Context, cmd *cobra.Command, flags *rootFlags, dbPath, ref, productDB string, noProducts bool) (in lintInput, found bool, err error) {
	resources := []string{qwResDocuments, qwResLineItems}
	if !noProducts {
		resources = append(resources, qwResProducts)
	}
	db, ok, err := openMirror(ctx, cmd, flags, dbPath, resources...)
	if err != nil || !ok {
		return lintInput{}, false, err
	}
	defer db.Close()
	docs, err := loadMirror(db, qwResDocuments)
	if err != nil {
		return lintInput{}, false, err
	}
	guid := strings.ToUpper(strings.Trim(ref, "{}"))
	var matches []map[string]any
	for _, d := range docs {
		if strings.ToUpper(qw.Str(d["id"])) == guid || qw.Str(d["DocNo"]) == ref {
			matches = append(matches, d)
		}
	}
	switch len(matches) {
	case 0:
		return lintInput{}, false, nil
	case 1:
	default:
		return lintInput{}, false, usageErr(fmt.Errorf("DocNo %q matches %d mirrored documents; pass the RecGUID instead", ref, len(matches)))
	}
	header := matches[0]
	docID := qw.Str(header["id"])
	// Only audit lines the mirror actually read for this document; an empty
	// line list after 'mirror --only documents' must not read as a clean quote.
	coverage, err := loadMirror(db, qwResItemCoverage)
	if err != nil {
		return lintInput{}, false, err
	}
	covered := false
	for _, cv := range coverage {
		if qw.Str(cv["id"]) == docID {
			covered = true
			break
		}
	}
	if !covered {
		return lintInput{}, false, notFoundErr(fmt.Errorf("the lines of document %s are not in the local mirror; run '%s mirror' (without --only documents) or drop --data-source local", ref, qwCLIName))
	}
	items, err := loadMirror(db, qwResLineItems)
	if err != nil {
		return lintInput{}, false, err
	}
	lines := make([]map[string]any, 0)
	for _, it := range items {
		if qw.Str(it["DocRecGUID"]) == docID {
			lines = append(lines, it)
		}
	}
	sortLines(lines)
	in = lintInput{header: header, lines: lines, source: "local", partial: partialResources(db, resources...)}
	if !noProducts {
		if in.idx, err = mirroredProductIndex(db, productDB); err != nil {
			return lintInput{}, false, err
		}
		in.productsChecked = in.idx != nil
	}
	return in, true, nil
}

func newNovelDocsLintCmd(flags *rootFlags) *cobra.Command {
	var minMargin float64
	var productDB, dbPath string
	var noProducts, strict bool

	cmd := &cobra.Command{
		Use:   "lint [DocNo-or-RecGUID]",
		Short: "Audit one quote before sending: price below cost, zero cost, bad quantity, low margin",
		Long: `Audit one document's header and product lines and report findings.

Errors: unit price below unit cost, missing unit cost when the product database has one,
quantity of zero or less, SoldToCompany longer than 50 characters.
Warnings: margin below --min-margin, missing unit cost, missing part number, quoted cost
different from the product database.
Info: price different from the product database, SubTotal different from the sum of
product-line ExtendedPrice.

Findings are data: the command exits 0 and reports error and warning counts. Add
--strict to exit 1 when any error is found (for scripts and CI).

Reads live by default; with --data-source local (or when the API is unreachable) it
audits the copy in the local mirror.

Use this command to audit one document's lines before sending it. Do NOT use it to scan
all open quotes for distributor cost changes; use 'cost-drift' instead.`,
		Example: strings.Trim(`
  quotewerks-cli docs lint 1042 --min-margin 10 --json
  quotewerks-cli docs lint 1042 --strict --data-source local --agent`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto", "pp:typed-exit-codes": "0,1", "pp:happy-args": "doc=1042"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "docs lint")
			}
			if len(args) == 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a document number or RecGUID is required"))
			}
			ref := args[0]
			ctx := cmd.Context()
			var in lintInput
			if flags.dataSource == "local" {
				got, ok, err := lintLocal(ctx, cmd, flags, dbPath, ref, productDB, noProducts)
				if err != nil {
					return err
				}
				if !ok {
					return notFoundErr(fmt.Errorf("document %s is not in the local mirror; run '%s mirror' or drop --data-source local", ref, qwCLIName))
				}
				in = got
			} else {
				c, err := flags.newClient()
				if err != nil {
					return err
				}
				got, liveErr := lintLive(ctx, c, ref, productDB, noProducts)
				if liveErr != nil {
					if flags.dataSource != "auto" || !isNetworkError(liveErr) {
						return liveErr
					}
					local, ok, err := lintLocal(ctx, cmd, flags, dbPath, ref, productDB, noProducts)
					if err != nil || !ok {
						return localFallbackError(liveErr, err)
					}
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: live read failed (%v); auditing the local mirror copy\n", liveErr)
					got = local
				}
				in = got
			}
			findings := qw.Lint(in.header, in.lines, in.idx, minMargin)
			nErr := qw.CountSeverity(findings, qw.SevError)
			view := map[string]any{
				"document":         project(in.header, []string{"id", "DocNo", "DocType", "DocName", "SoldToCompany"}),
				"source":           in.source,
				"errors":           nErr,
				"warnings":         qw.CountSeverity(findings, qw.SevWarning),
				"findings":         findings,
				"products_checked": in.productsChecked,
			}
			if len(in.partial) > 0 {
				view["partial_resources"] = in.partial // the comparison may have missed rows
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				if err := printJSONFiltered(cmd.OutOrStdout(), view, flags); err != nil {
					return err
				}
			} else {
				w := cmd.OutOrStdout()
				if len(findings) == 0 {
					fmt.Fprintf(w, "%s: no findings\n", sc(in.header["DocNo"]))
				}
				for _, f := range findings {
					fmt.Fprintf(w, "%-7s %-26s %s\n", f.Severity, f.Rule, sc(f.Message))
				}
				if !in.productsChecked {
					fmt.Fprintln(w, "note: no product database was compared")
				}
			}
			if strict && nErr > 0 {
				return &cliError{code: 1, err: fmt.Errorf("%d error finding(s) on document %s", nErr, sc(in.header["DocNo"]))}
			}
			return nil
		},
	}
	cmd.Flags().Float64Var(&minMargin, "min-margin", 10, "Warn on product lines whose margin percent is below this")
	cmd.Flags().StringVar(&productDB, "product-db", "", "Compare against one product database only (name from 'products search --dbs')")
	cmd.Flags().BoolVar(&noProducts, "no-products", false, "Skip the product-database comparison")
	cmd.Flags().BoolVar(&strict, "strict", false, "Exit 1 when any error-severity finding is reported")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local mirror database path (for --data-source local)")
	return cmd
}
