// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// whitespaceProduct is one catalog product a company has never been quoted.
type whitespaceProduct struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	MPN   string  `json:"mpn,omitempty"`
	Price float64 `json:"price"`
}

type whitespaceReport struct {
	Company        string              `json:"company"`
	CompanyID      string              `json:"companyId"`
	QuotesScanned  int                 `json:"quotesScanned"`
	ProductsQuoted int                 `json:"productsQuoted"`
	CatalogSize    int                 `json:"catalogSize"`
	Whitespace     []whitespaceProduct `json:"whitespace"`
	Note           string              `json:"note,omitempty"`
}

// newNovelCompanyWhitespaceCmd implements the "company whitespace"
// transcendence command: catalog products a company has never been quoted.
// pp:data-source local
func newNovelCompanyWhitespaceCmd(flags *rootFlags) *cobra.Command {
	var flagLimit int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "whitespace [company]",
		Short: "Show catalog products a company has never been quoted — the cross-sell gaps",
		Long: "Use this command to find products a company has never been quoted. Do NOT use this command for\n" +
			"the most-quoted products overall; use 'product velocity' instead.\n\n" +
			"Resolves the company by id, external identifier, or name, collects every product that has ever\n" +
			"appeared on one of its quotes, and reports the catalog products NOT in that set — the\n" +
			"cross-sell gaps. A set-difference only the local store can compute. Run `sync` first.",
		Example: "  salesbuildr-cli company whitespace \"Acme Managed IT\"\n" +
			"  salesbuildr-cli company whitespace 64f1c0… --limit 25 --agent",
		Annotations: map[string]string{
			"mcp:read-only": "true",
			"pp:happy-args": "<company>=example-company",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if len(args) < 1 || strings.TrimSpace(args[0]) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("company id, external identifier, or name is required"))
			}
			if flagLimit <= 0 {
				return usageErr(fmt.Errorf("--limit must be > 0"))
			}
			target := strings.TrimSpace(args[0])

			db, err := openNovelStore(cmd, flags, dbPath, "company")
			if err != nil {
				return err
			}
			companies, err := loadCompaniesFromStore(db)
			if err != nil {
				_ = db.Close()
				return err
			}
			products, err := loadProductsFromStore(db)
			_ = db.Close()
			if err != nil {
				return err
			}
			quotes, err := loadQuotesHinted(cmd, flags, dbPath)
			if err != nil {
				return err
			}

			company, err := resolveCompany(companies, target)
			if err != nil {
				return notFoundErr(err)
			}

			// Collect product identity keys quoted to this company.
			quoted := map[string]bool{}
			quotesScanned := 0
			nameLower := strings.ToLower(company.Name)
			for _, q := range quotes {
				// Prefer ID matching; fall back to name equality only when the
				// quote document carries no company id, so two companies that
				// share a name cannot cross-attribute each other's quotes.
				var match bool
				if q.CompanyID != "" && company.ID != "" {
					match = q.CompanyID == company.ID
				} else {
					match = nameLower != "" && strings.ToLower(q.Company) == nameLower
				}
				if !match {
					continue
				}
				quotesScanned++
				for _, item := range q.Items {
					if key := item.productKey(); key != "" {
						quoted[key] = true
					}
				}
			}

			report := whitespaceReport{
				Company:        company.Name,
				CompanyID:      company.ID,
				QuotesScanned:  quotesScanned,
				ProductsQuoted: len(quoted),
				CatalogSize:    len(products),
				Whitespace:     make([]whitespaceProduct, 0),
			}
			for _, p := range products {
				seen := false
				for _, key := range p.productKeys() {
					if quoted[key] {
						seen = true
						break
					}
				}
				if !seen {
					report.Whitespace = append(report.Whitespace, whitespaceProduct{ID: p.ID, Name: p.Name, MPN: p.MPN, Price: p.Price})
				}
			}
			sort.SliceStable(report.Whitespace, func(i, j int) bool {
				if report.Whitespace[i].Price != report.Whitespace[j].Price {
					return report.Whitespace[i].Price > report.Whitespace[j].Price
				}
				return report.Whitespace[i].ID < report.Whitespace[j].ID
			})
			if len(report.Whitespace) > flagLimit {
				report.Whitespace = report.Whitespace[:flagLimit]
				report.Note = fmt.Sprintf("showing top %d by price; raise --limit for more", flagLimit)
			}
			if len(products) == 0 {
				report.Note = "no products in local store — run `salesbuildr-cli sync` first"
			} else if quotesScanned == 0 {
				report.Note = fmt.Sprintf("no quotes found for %q in the local store — whitespace equals the whole catalog", company.Name)
			}

			w := cmd.OutOrStdout()
			if flags.asJSON || flags.agent || !isTerminal(w) {
				return printJSONFiltered(w, report, flags)
			}
			fmt.Fprintf(w, "%s — %d quotes scanned, %d distinct products quoted, catalog %d\n\n",
				report.Company, report.QuotesScanned, report.ProductsQuoted, report.CatalogSize)
			tw := newTabWriter(w)
			fmt.Fprintln(tw, "PRODUCT\tMPN\tPRICE")
			for _, p := range report.Whitespace {
				fmt.Fprintf(tw, "%s\t%s\t%.2f\n", truncate(p.Name, 40), orDash(p.MPN), p.Price)
			}
			if err := tw.Flush(); err != nil {
				return err
			}
			if report.Note != "" {
				fmt.Fprintf(w, "\n%s\n", report.Note)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&flagLimit, "limit", 50, "Maximum whitespace products to return (ranked by price)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (defaults to standard location)")
	return cmd
}

// resolveCompany finds a company by exact id, exact external identifier,
// case-insensitive exact name, or unique case-insensitive substring match.
func resolveCompany(companies []novelCompany, target string) (novelCompany, error) {
	lower := strings.ToLower(target)
	var substringMatches []novelCompany
	for _, c := range companies {
		if c.ID == target || (c.ExternalIdentifier != "" && c.ExternalIdentifier == target) {
			return c, nil
		}
		name := strings.ToLower(c.Name)
		if name == lower {
			return c, nil
		}
		if name != "" && strings.Contains(name, lower) {
			substringMatches = append(substringMatches, c)
		}
	}
	switch len(substringMatches) {
	case 1:
		return substringMatches[0], nil
	case 0:
		return novelCompany{}, fmt.Errorf("no company matching %q in local store (run `salesbuildr-cli sync` first?)", target)
	default:
		names := make([]string, 0, len(substringMatches))
		for i, c := range substringMatches {
			if i >= 5 {
				names = append(names, "…")
				break
			}
			names = append(names, c.Name)
		}
		return novelCompany{}, fmt.Errorf("company %q is ambiguous: %s", target, strings.Join(names, ", "))
	}
}
