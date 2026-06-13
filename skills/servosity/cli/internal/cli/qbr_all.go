// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"servosity-msp-pp-cli/internal/qbrreport"
	"servosity-msp-pp-cli/internal/store"
)

type qbrAllFailure struct {
	CompanyID   int64  `json:"company_id"`
	CompanyName string `json:"company_name"`
	Error       string `json:"error"`
}

type qbrAllView struct {
	Quarter  string          `json:"quarter"`
	OutDir   string          `json:"out_dir"`
	Format   string          `json:"format"`
	Written  []string        `json:"written"`
	Failures []qbrAllFailure `json:"failures"`
}

var qbrAllSlugRE = regexp.MustCompile(`[^a-z0-9]+`)

func qbrAllSlug(name string, id int64) string {
	s := qbrAllSlugRE.ReplaceAllString(strings.ToLower(name), "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return strconv.FormatInt(id, 10)
	}
	return s
}

// pp:data-source local
func newNovelQbrAllCmd(flags *rootFlags) *cobra.Command {
	var quarter string
	var outDir string
	var format string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "qbr-all",
		Short: "Generate every client's QBR backup report in one pass, one file per company",
		Long: `Loop the QBR assembly over every company in the local store and write one
report file per client into --out.

Use this command to generate every client's QBR section in one pass.
Do NOT use it for a single client; use 'qbr' instead.

Reads from the local sync store. Run 'servosity-cli sync' first.`,
		Example: `  # Quarter-end: the whole book as Markdown files
  servosity-cli qbr-all --quarter 2026-Q1 --out ./qbrs/

  # Current quarter, HTML
  servosity-cli qbr-all --format html --out ./qbrs/`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintf(cmd.OutOrStdout(), "would write one %s QBR report per company to %s\n", format, outDir)
				return nil
			}
			ctx := cmd.Context()

			if quarter == "" {
				quarter = qbrreport.CurrentQuarter(time.Now())
			}
			if _, _, err := qbrreport.ParseQuarter(quarter); err != nil {
				_ = cmd.Usage()
				return usageErr(err)
			}
			switch format {
			case "md", "html", "pdf":
			default:
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("invalid --format %q: must be md, html, or pdf", format))
			}

			if dbPath == "" {
				dbPath = defaultDBPath("servosity-cli")
			}
			db, err := store.OpenWithContext(ctx, dbPath)
			if err != nil {
				return fmt.Errorf("opening local database: %w", err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "companies") {
				hintIfStale(cmd, db, "companies", flags.maxAge)
			}

			rows, err := db.DB().QueryContext(ctx, `SELECT id, COALESCE(name,'') FROM companies`)
			if err != nil {
				return fmt.Errorf("listing companies from local store (run 'servosity-cli sync' first): %w", err)
			}
			type co struct {
				id   int64
				name string
			}
			cos := make([]co, 0, 64)
			for rows.Next() {
				var idStr, name string
				if err := rows.Scan(&idStr, &name); err != nil {
					continue
				}
				id, perr := strconv.ParseInt(idStr, 10, 64)
				if perr != nil {
					continue
				}
				cos = append(cos, co{id: id, name: name})
			}
			_ = rows.Close()
			if len(cos) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "hint: no companies in the local store — run 'servosity-cli sync --resources companies' first")
				view := qbrAllView{Quarter: quarter, OutDir: outDir, Format: format, Written: []string{}, Failures: []qbrAllFailure{}}
				if !wantsHumanTable(cmd.OutOrStdout(), flags) {
					return printJSONFiltered(cmd.OutOrStdout(), view, flags)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Wrote 0 reports — no companies in the local store (run sync first)\n")
				return nil
			}

			if err := os.MkdirAll(outDir, 0o750); err != nil {
				return fmt.Errorf("creating --out directory: %w", err)
			}

			view := qbrAllView{
				Quarter:  quarter,
				OutDir:   outDir,
				Format:   format,
				Written:  make([]string, 0, len(cos)),
				Failures: make([]qbrAllFailure, 0),
			}
			for _, c := range cos {
				report, aerr := qbrreport.Assemble(ctx, db.DB(), c.id, quarter)
				if aerr != nil {
					view.Failures = append(view.Failures, qbrAllFailure{CompanyID: c.id, CompanyName: c.name, Error: aerr.Error()})
					continue
				}
				name := fmt.Sprintf("%s-%s.%s", qbrAllSlug(c.name, c.id), quarter, format)
				outPath := filepath.Join(outDir, name)
				var werr error
				switch format {
				case "md":
					// outPath is built from the operator's --out dir plus a
					// sanitized company slug; a local report write to a
					// user-chosen path, not attacker-controlled inclusion.
					f, ferr := os.Create(outPath) // #nosec G304 -- operator-controlled --out path
					if ferr == nil {
						werr = qbrreport.RenderMarkdown(report, f)
						_ = f.Close()
					} else {
						werr = ferr
					}
				case "html":
					f, ferr := os.Create(outPath) // #nosec G304 -- operator-controlled --out path
					if ferr == nil {
						werr = qbrreport.RenderHTML(report, f)
						_ = f.Close()
					} else {
						werr = ferr
					}
				case "pdf":
					werr = qbrreport.RenderPDF(report, outPath)
				}
				if werr != nil {
					view.Failures = append(view.Failures, qbrAllFailure{CompanyID: c.id, CompanyName: c.name, Error: werr.Error()})
					continue
				}
				view.Written = append(view.Written, outPath)
			}

			if len(view.Failures) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of %d reports failed; %d written\n", len(view.Failures), len(cos), len(view.Written))
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Wrote %d %s report(s) for %s to %s\n", len(view.Written), format, quarter, outDir)
			for _, f := range view.Failures {
				fmt.Fprintf(out, "  FAILED %s (%d): %s\n", coalesceCompany(f.CompanyName), f.CompanyID, f.Error)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&quarter, "quarter", "", "Quarter like 2026-Q1 (default: current quarter)")
	cmd.Flags().StringVar(&outDir, "out", "./qbrs", "Output directory (one file per company)")
	cmd.Flags().StringVar(&format, "format", "md", "Report format: md, html, or pdf (pdf needs local Chrome)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/servosity-cli/data.db)")
	return cmd
}
