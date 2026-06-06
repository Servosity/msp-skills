// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"

	"sherweb-pp-cli/internal/insights"

	"github.com/spf13/cobra"
)

// pp:data-source local
func newNovelUsageLeakCmd(flags *rootFlags) *cobra.Command {
	var flagCustomer string

	cmd := &cobra.Command{
		Use:   "usage-leak",
		Short: "Flag metered platform usage with no matching receivable charge — consumption you absorb instead of billing.",
		Long: `Use this command for metered platform usage with NO matching receivable charge.
Do NOT use this command for subscriptions billed to no one; use 'orphans' instead.
Do NOT use this command for paid-vs-metered seat counts; use 'right-size' instead.

Anti-joins the usage rows against receivable charges in the local store (both
populated by 'sherweb-cli deep-sync'). A row leaks when its customer+SKU pair
has no receivable charge; unresolvable SKUs only leak when the customer has no
receivable charges at all. The biggest absorbed consumption sorts first.`,
		Example:     "  sherweb-cli usage-leak --agent\n  sherweb-cli usage-leak --customer 0aa00000-0000-0000-0000-000000000000",
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

			usage, err := loadUsage(s)
			if err != nil {
				return fmt.Errorf("loading usage: %w", err)
			}
			receivable, err := loadReceivableCharges(s)
			if err != nil {
				return fmt.Errorf("loading receivable charges: %w", err)
			}
			subs, err := loadSubscriptions(s)
			if err != nil {
				return fmt.Errorf("loading subscriptions: %w", err)
			}

			leaks := insights.FindUsageLeaks(usage, receivable, subs)
			if flagCustomer != "" {
				filtered := make([]insights.UsageLeak, 0, len(leaks))
				for _, l := range leaks {
					if l.CustomerID == flagCustomer || l.CustomerName == flagCustomer {
						filtered = append(filtered, l)
					}
				}
				leaks = filtered
			}
			if len(leaks) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "no usage leaks found — run 'sherweb-cli deep-sync' first to populate usage + receivable charges")
			}
			return flags.printJSON(cmd, leaks)
		},
	}
	cmd.Flags().StringVar(&flagCustomer, "customer", "", "Limit to a single customer (id or display name)")
	return cmd
}
