// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"

	"sherweb-pp-cli/internal/insights"

	"github.com/spf13/cobra"
)

// pp:data-source local
func newNovelRightSizeCmd(flags *rootFlags) *cobra.Command {
	var flagCustomer string

	cmd := &cobra.Command{
		Use:   "right-size",
		Short: "Flag subscriptions where seats paid differ from metered seats used.",
		Long: `Join local subscriptions (seats paid) against platform meter-usage (seats used)
and flag over- and under-provisioned subscriptions. Only subscriptions with a
usage signal are returned; rows sort by absolute seat delta descending.

Run 'sherweb-cli deep-sync' first to populate subscriptions + usage.`,
		Example:     "  sherweb-cli right-size --customer 00000000-0000-0000-0000-000000000000 --agent",
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
			usage, err := loadUsage(s)
			if err != nil {
				return fmt.Errorf("loading usage: %w", err)
			}
			if flagCustomer != "" {
				kept := subs[:0]
				for _, sub := range subs {
					if sub.CustomerID == flagCustomer || sub.CustomerName == flagCustomer {
						kept = append(kept, sub)
					}
				}
				subs = kept
			}
			result := insights.RightSize(subs, usage)
			if len(usage) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "no usage data in the local store — run 'sherweb-cli deep-sync' first")
			}
			return flags.printJSON(cmd, result)
		},
	}
	cmd.Flags().StringVar(&flagCustomer, "customer", "", "Limit to a single customer (id or display name)")
	return cmd
}
