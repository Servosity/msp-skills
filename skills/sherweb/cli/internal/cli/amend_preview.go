// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"strconv"

	"sherweb-pp-cli/internal/insights"

	"github.com/spf13/cobra"
)

// pp:data-source local
func newNovelAmendPreviewCmd(flags *rootFlags) *cobra.Command {
	var flagCustomer string
	var flagSub string
	var flagQty string

	cmd := &cobra.Command{
		Use:   "amend-preview",
		Short: "Preview the monthly cost change of a seat amendment before committing it.",
		Long: `Compute the projected monthly cost delta of changing a subscription to a new
seat quantity, using the unit price stored from the subscription pricing-information
endpoint. Read-only — it never sends an amendment. Use 'service-provider
amend-subscriptions' to actually apply the change.

Run 'sherweb-cli deep-sync' first so the subscription and its unit price are in the store.`,
		Example:     "  sherweb-cli amend-preview --customer 00000000-0000-0000-0000-000000000000 --sub SUB123 --qty 25",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			// Bare invocation prints help rather than pflag's terse required-flag error.
			if cmd.Flags().NFlag() == 0 && len(args) == 0 && !flags.dryRun {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			if flagSub == "" {
				return fmt.Errorf("--sub is required: the subscription id to preview")
			}
			if flagQty == "" {
				return fmt.Errorf("--qty is required: the new seat quantity")
			}
			newQty, err := strconv.ParseFloat(flagQty, 64)
			if err != nil {
				return fmt.Errorf("--qty must be a number, got %q", flagQty)
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
			var found *insights.Subscription
			for i := range subs {
				if subs[i].ID == flagSub {
					if flagCustomer == "" || subs[i].CustomerID == flagCustomer {
						found = &subs[i]
						break
					}
				}
			}
			if found == nil {
				return fmt.Errorf("subscription %q not found in local store — run 'sherweb-cli deep-sync' first, or check the id (exit 4)", flagSub)
			}
			preview := insights.PreviewAmendment(*found, newQty)
			if found.UnitPrice == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "warning: no unit price stored for this subscription — cost deltas will be 0; ensure deep-sync captured pricing-information")
			}
			return flags.printJSON(cmd, preview)
		},
	}
	cmd.Flags().StringVar(&flagCustomer, "customer", "", "Customer id (optional disambiguation)")
	cmd.Flags().StringVar(&flagSub, "sub", "", "Subscription id to preview (required)")
	cmd.Flags().StringVar(&flagQty, "qty", "", "New seat quantity (required)")
	return cmd
}
