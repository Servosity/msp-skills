// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"

	"sherweb-pp-cli/internal/insights"

	"github.com/spf13/cobra"
)

// pp:data-source local
func newNovelOrphansCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "orphans",
		Short: "List active subscriptions with zero receivable charges — zombie subs you pay for but don't bill on.",
		Long: `Anti-join the local subscriptions against receivable charges: any active
subscription whose customer billed zero receivable charges in the latest period
is an orphan you pay Sherweb for but are not billing the customer for.

Run 'sherweb-cli sync' and 'sherweb-cli deep-sync' first.`,
		Example:     "  sherweb-cli orphans --agent",
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
			receivable, err := loadReceivableCharges(s)
			if err != nil {
				return fmt.Errorf("loading receivable charges: %w", err)
			}
			result := insights.FindOrphans(subs, receivable)
			if len(subs) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "no subscriptions in the local store — run 'sherweb-cli deep-sync' first")
			}
			return flags.printJSON(cmd, result)
		},
	}
	return cmd
}
