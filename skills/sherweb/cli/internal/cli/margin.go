// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"

	"sherweb-pp-cli/internal/insights"

	"github.com/spf13/cobra"
)

// pp:data-source local
func newNovelMarginCmd(flags *rootFlags) *cobra.Command {
	var flagMonth string
	var flagCustomer string

	cmd := &cobra.Command{
		Use:   "margin",
		Short: "See net margin per customer (receivable minus payable) from the local store.",
		Long: `Compute net margin per customer = what the customer owes you (receivable charges)
minus what you owe Sherweb (payable charges), from the local SQLite store.

Run 'sherweb-cli sync' (payable charges + customers) and 'sherweb-cli deep-sync'
(per-customer receivable charges) first. Rows sort worst-margin-first so leaks surface at the top.`,
		Example:     "  sherweb-cli margin --month 2026-04 --agent --select customerName,payable,receivable,margin",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			s, err := openInsightStore(cmd.Context())
			if err != nil {
				return err
			}
			defer s.Close()

			if !hintIfUnsynced(cmd, s, "") {
				hintIfStale(cmd, s, "", flags.maxAge)
			}

			payable, err := loadPayableCharges(s, flagMonth)
			if err != nil {
				return fmt.Errorf("loading payable charges: %w", err)
			}
			receivable, err := loadReceivableCharges(s)
			if err != nil {
				return fmt.Errorf("loading receivable charges: %w", err)
			}
			result := insights.ComputeMargin(payable, receivable)
			if flagCustomer != "" {
				filtered := make([]insights.CustomerMargin, 0, len(result))
				for _, r := range result {
					if r.CustomerID == flagCustomer || r.CustomerName == flagCustomer {
						filtered = append(filtered, r)
					}
				}
				result = filtered
			}
			if len(result) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "no margin data — run 'sherweb-cli sync' then 'sherweb-cli deep-sync' to populate payable + receivable charges")
			}
			return flags.printJSON(cmd, result)
		},
	}
	cmd.Flags().StringVar(&flagMonth, "month", "", "Restrict payable charges to a billing month (YYYY-MM)")
	cmd.Flags().StringVar(&flagCustomer, "customer", "", "Limit to a single customer (id or display name)")
	return cmd
}
