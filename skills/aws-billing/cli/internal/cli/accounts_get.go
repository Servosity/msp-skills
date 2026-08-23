// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// `accounts get <account_id>` — show one synced AWS account. Empty-tolerant:
// reports a clear not-found (exit 0) rather than erroring on an unsynced store.
//
// pp:client-call
package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"aws-billing-pp-cli/internal/cliutil"
)

func newAccountsGetCmd(flags *rootFlags) *cobra.Command {
	var dbPath, profile, region string

	cmd := &cobra.Command{
		Use:         "get <account_id>",
		Short:       "Get one synced AWS account",
		Example:     "  aws-billing-cli accounts get 123456789012",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) || cliutil.IsVerifyEnv() {
				return nil
			}
			o := awsReadOptsFromFlags(flags, dbPath, profile, region)
			accts, err := accountsForList(cmd, o, args[0])
			if err != nil {
				return err
			}
			if flags.asJSON {
				if len(accts) == 0 {
					return flags.printJSON(cmd, map[string]any{"account_id": args[0], "found": false})
				}
				return flags.printJSON(cmd, accts[0])
			}
			w := cmd.OutOrStdout()
			if len(accts) == 0 {
				fmt.Fprintf(w, "account %s not found in the local store (run 'aws-billing-cli sync' from a management-account profile)\n", args[0])
				return nil
			}
			a := accts[0]
			fmt.Fprintf(w, "Account:  %s\nName:     %s\nStatus:   %s\nEmail:    %s\n", a.AccountID, a.Name, a.Status, a.Email)
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: per-user cache)")
	cmd.Flags().StringVar(&profile, "profile-aws", "", "AWS shared-config profile for a live fetch")
	cmd.Flags().StringVar(&region, "region", "", "AWS region for a live fetch")
	return cmd
}
