// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel feature (Phase 3): customer-360. Not generator-managed.

package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"pax8-pp-cli/internal/store"
)

type companyShowResult struct {
	Company        map[string]any   `json:"company"`
	Subscriptions  []map[string]any `json:"subscriptions"`
	Contacts       []map[string]any `json:"contacts"`
	Invoices       []map[string]any `json:"invoices"`
	UsageSummaries []map[string]any `json:"usageSummaries"`
}

// pp:data-source local
func newNovelCompanyShowCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:         "show [companyId]",
		Short:       "One view of a company with its subscriptions, contacts, invoices, and usage.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Long: `Customer-360: join a company to its subscriptions, contacts, invoices, and
usage from the local store in a single view. Run 'pax8-cli sync' first.

Use this command to assemble one company's full picture (its subscriptions, contacts, invoices, usage) by company ID.
Do NOT use this command to rank or total spend across all customers; use 'spend' instead.
Do NOT use this command for billing-mismatch detection; use 'reconcile' instead.`,
		Example: `  # Full picture of one company
  pax8-cli company show 0a1b2c3d

  # Narrowed for an agent
  pax8-cli company show 0a1b2c3d --agent --select subscriptions.productName,invoices.total`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if len(args) == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			companyID := args[0]
			out := cmd.OutOrStdout()
			if dbPath == "" {
				dbPath = defaultDBPath("pax8-cli")
			}
			db, err := store.OpenWithContext(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local database: %w\nRun 'pax8-cli sync' first.", err)
			}
			defer db.Close()
			maybeEmitSyncHints(cmd, db, "companies", flags.maxAge)

			ctx := cmd.Context()
			res := companyShowResult{}

			companyRows, err := pax8QueryObjects(ctx, db, `SELECT data FROM companies WHERE id = ? LIMIT 1`, companyID)
			if err != nil {
				return fmt.Errorf("reading company: %w", err)
			}
			if len(companyRows) == 0 {
				return notFoundErr(fmt.Errorf("company %q not found in local store (run 'pax8-cli sync' first)", companyID))
			}
			res.Company = companyRows[0]

			res.Subscriptions, _ = pax8QueryObjects(ctx, db, `SELECT data FROM subscriptions WHERE company_id = ?`, companyID)
			res.Invoices, _ = pax8QueryObjects(ctx, db, `SELECT data FROM invoices WHERE company_id = ?`, companyID)
			res.UsageSummaries, _ = pax8QueryObjects(ctx, db, `SELECT data FROM usage_summaries WHERE company_id = ?`, companyID)
			// Contacts are keyed by companies_id (FK column on the contacts table).
			res.Contacts, _ = pax8QueryObjects(ctx, db, `SELECT data FROM contacts WHERE companies_id = ?`, companyID)

			if flags.asJSON {
				return printJSONFiltered(out, res, flags)
			}
			name := pax8FieldStr(res.Company, "name", "companyName")
			if name == "" {
				name = companyID
			}
			fmt.Fprintf(out, "Company: %s (%s)\n", name, companyID)
			fmt.Fprintf(out, "  Subscriptions: %d\n", len(res.Subscriptions))
			fmt.Fprintf(out, "  Contacts:      %d\n", len(res.Contacts))
			fmt.Fprintf(out, "  Invoices:      %d\n", len(res.Invoices))
			fmt.Fprintf(out, "  Usage summaries: %d\n", len(res.UsageSummaries))
			if len(res.Subscriptions) > 0 {
				fmt.Fprintln(out, "\nSubscriptions:")
				for _, s := range res.Subscriptions {
					fmt.Fprintf(out, "  - %s  product=%s qty=%s status=%s\n",
						pax8FieldStr(s, "id"),
						pax8FieldStr(s, "productName", "productId", "product_id"),
						pax8FieldStr(s, "quantity"),
						pax8FieldStr(s, "status"))
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
