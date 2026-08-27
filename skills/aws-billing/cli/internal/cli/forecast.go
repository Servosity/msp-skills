// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// `forecast` — Cost Explorer next-period forecast.
//
// pp:client-call
package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"aws-billing-pp-cli/internal/awsx"
	"aws-billing-pp-cli/internal/cliutil"
)

func newForecastCmd(flags *rootFlags) *cobra.Command {
	var profile, region string

	cmd := &cobra.Command{
		Use:   "forecast",
		Short: "Forecast next month's AWS spend (Cost Explorer)",
		Long: `Ask Cost Explorer to forecast next month's total spend, with an 80%
prediction interval. Requires some billing history; AWS returns an error when
there isn't enough, which is reported as a soft note.`,
		Example: `  # Forecast next month
  aws-billing-cli forecast --profile-aws prod`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) || cliutil.IsVerifyEnv() {
				return nil
			}
			start, end := nextMonthRange()
			client, err := awsx.New(cmd.Context(), profile, region)
			if err != nil {
				return err
			}
			f, err := client.GetCostForecast(cmd.Context(), start, end, "MONTHLY")
			if err != nil {
				if awsx.IsAccessDenied(err) {
					return fmt.Errorf("forecast denied: grant ce:GetCostForecast (run 'aws-billing-cli iam-setup --tier core')")
				}
				return fmt.Errorf("forecast unavailable (often: not enough billing history yet): %w", err)
			}
			f.ID = "fc|" + f.PeriodStart
			if flags.asJSON {
				return flags.printJSON(cmd, f)
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "Forecast %s to %s\n", f.PeriodStart, f.PeriodEnd)
			fmt.Fprintf(w, "  expected:  $%.2f\n", f.MeanUSD)
			if f.LowerUSD > 0 || f.UpperUSD > 0 {
				fmt.Fprintf(w, "  range:     $%.2f – $%.2f (80%% interval)\n", f.LowerUSD, f.UpperUSD)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&profile, "profile-aws", "", "AWS shared-config profile")
	cmd.Flags().StringVar(&region, "region", "", "AWS region")
	return cmd
}
