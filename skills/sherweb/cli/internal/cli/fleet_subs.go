// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"

	"sherweb-pp-cli/internal/insights"

	"github.com/spf13/cobra"
)

// pp:data-source local
func newNovelFleetSubsCmd(flags *rootFlags) *cobra.Command {
	var flagProduct string
	var flagSKU string

	cmd := &cobra.Command{
		Use:   "fleet-subs",
		Short: "Roll up subscriptions across ALL customers by product/SKU (total seats, customer counts).",
		Long: `The Service Provider API returns subscriptions one customer at a time. This
command aggregates the whole book from the local store: total seats, customer
count, and subscription count per product/SKU, sorted by total seats.

Run 'sherweb-cli deep-sync' first to populate per-customer subscriptions.`,
		Example:     "  sherweb-cli fleet-subs --product \"Microsoft 365\" --agent",
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

			subs, err := loadSubscriptions(s)
			if err != nil {
				return fmt.Errorf("loading subscriptions: %w", err)
			}
			result := insights.RollupFleet(subs, flagProduct, flagSKU)
			if len(subs) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "no subscriptions in the local store — run 'sherweb-cli deep-sync' first")
			}
			return flags.printJSON(cmd, result)
		},
	}
	cmd.Flags().StringVar(&flagProduct, "product", "", "Filter to products whose name contains this text (case-insensitive)")
	cmd.Flags().StringVar(&flagSKU, "sku", "", "Filter to SKUs containing this text (case-insensitive)")
	return cmd
}
