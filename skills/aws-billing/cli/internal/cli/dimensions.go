// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// `dimensions` — list the distinct values of a Cost Explorer dimension.
//
// pp:client-call
package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"aws-billing-pp-cli/internal/awsx"
	"aws-billing-pp-cli/internal/cliutil"
)

func newDimensionsCmd(flags *rootFlags) *cobra.Command {
	var period, profile, region string

	cmd := &cobra.Command{
		Use:   "dimensions <SERVICE|LINKED_ACCOUNT|REGION|USAGE_TYPE|INSTANCE_TYPE>",
		Short: "List the distinct values of a Cost Explorer dimension",
		Long: `List the distinct values present for a Cost Explorer dimension over a
period — useful for discovering what services, accounts, regions, or usage
types exist before filtering 'bill' or 'compare'.`,
		Example: `  # What services are on the bill this month?
  aws-billing-cli dimensions SERVICE

  # What linked accounts exist?
  aws-billing-cli dimensions LINKED_ACCOUNT --agent`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) || cliutil.IsVerifyEnv() {
				return nil
			}
			dim := strings.ToUpper(args[0])
			pr, err := resolvePeriod(period)
			if err != nil {
				return usageErr(err)
			}
			client, err := awsx.New(cmd.Context(), profile, region)
			if err != nil {
				return err
			}
			vals, err := client.GetDimensionValues(cmd.Context(), pr.Start, pr.End, dim)
			if err != nil {
				if awsx.IsAccessDenied(err) {
					return fmt.Errorf("denied: grant ce:GetDimensionValues (run 'aws-billing-cli iam-setup --tier core')")
				}
				return err
			}
			if flags.asJSON {
				return flags.printJSON(cmd, map[string]any{"dimension": dim, "period": pr.Label, "values": vals})
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "%s values (%s): %d\n", dim, pr.Label, len(vals))
			for _, v := range vals {
				fmt.Fprintf(w, "  %s\n", v.Value)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&period, "period", "this-month", "Period to inspect")
	cmd.Flags().StringVar(&profile, "profile-aws", "", "AWS shared-config profile")
	cmd.Flags().StringVar(&region, "region", "", "AWS region")
	return cmd
}
