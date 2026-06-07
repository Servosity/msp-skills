// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command (Phase 3); survives regeneration as a whole file.

// pp:data-source local

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"gradient-pp-cli/internal/ledger"
)

func newNovelUsageDriftCmd(flags *rootFlags) *cobra.Command {
	var flagService string
	var flagAccount string

	cmd := &cobra.Command{
		Use:   "drift",
		Short: "Report which counts changed between your last two pushes",
		Long: strings.Trim(`
Use this command to see which accounts' usage actually changed since the
previous push - the reconciliation pre-flight. Do NOT use this command to
send counts; use 'usage push'.

The Synthesize API has no read-back for pushed unit counts, so the local push
ledger written by 'usage push' is the only history that exists anywhere. This
command joins the two most recent push runs per account x service and emits
only the rows that moved (plus pairs added or removed between runs).
`, "\n"),
		Example: strings.Trim(`
  gradient-cli usage drift --agent
  gradient-cli usage drift --service 550e8400-e29b-41d4-a716-446655440000
  gradient-cli usage drift --account 123456789 --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("usage drift takes no positional arguments"))
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would compare the two most recent push runs in the local ledger")
				return nil
			}
			if flags.dataSource == "live" {
				return fmt.Errorf("usage drift has no live equivalent: the API has no read-back for pushed counts; this command reads the local push ledger")
			}

			dir, err := ledger.Dir()
			if err != nil {
				return err
			}
			pushes, err := ledger.ReadPushes(dir)
			if err != nil {
				return err
			}
			if flagService != "" || flagAccount != "" {
				filtered := make([]ledger.PushRecord, 0, len(pushes))
				for _, p := range pushes {
					if flagService != "" && p.ServiceID != flagService {
						continue
					}
					if flagAccount != "" && p.AccountID != flagAccount {
						continue
					}
					filtered = append(filtered, p)
				}
				pushes = filtered
			}
			report := ledger.ComputeDrift(pushes)
			return printJSONFiltered(cmd.OutOrStdout(), report, flags)
		},
	}
	cmd.Flags().StringVar(&flagService, "service", "", "Only compare counts for this serviceId")
	cmd.Flags().StringVar(&flagAccount, "account", "", "Only compare counts for this accountId")
	return cmd
}
