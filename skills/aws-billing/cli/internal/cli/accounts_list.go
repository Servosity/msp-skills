// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// `accounts list` — list synced AWS Organizations accounts from the local
// store. Empty-tolerant: returns [] (exit 0) with a hint when nothing is
// synced, rather than erroring (the accounts table only fills from a
// management-account sync).
//
// pp:client-call
package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"aws-billing-pp-cli/internal/awsx"
	"aws-billing-pp-cli/internal/cliutil"
)

func newAccountsListCmd(flags *rootFlags) *cobra.Command {
	var dbPath, profile, region string

	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List synced AWS accounts in the organization",
		Example:     "  aws-billing-cli accounts list",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) || cliutil.IsVerifyEnv() {
				return nil
			}
			o := awsReadOptsFromFlags(flags, dbPath, profile, region)
			accts, err := accountsForList(cmd, o, "")
			if err != nil {
				return err
			}
			if flags.asJSON {
				if accts == nil {
					accts = []awsx.Account{}
				}
				return flags.printJSON(cmd, accts)
			}
			w := cmd.OutOrStdout()
			if len(accts) == 0 {
				fmt.Fprintln(w, "no synced accounts (run 'aws-billing-cli sync' from a management-account profile; member accounts can't list the org)")
				return nil
			}
			fmt.Fprintf(w, "%-16s %-30s %s\n", "ACCOUNT", "NAME", "STATUS")
			for _, a := range accts {
				fmt.Fprintf(w, "%-16s %-30s %s\n", a.AccountID, truncate(a.Name, 30), a.Status)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: per-user cache)")
	cmd.Flags().StringVar(&profile, "profile-aws", "", "AWS shared-config profile for a live fetch")
	cmd.Flags().StringVar(&region, "region", "", "AWS region for a live fetch")
	return cmd
}

// accountsForList returns accounts from the store, or (when --data-source live)
// from a live Organizations call. Empty is a normal result, never an error.
func accountsForList(cmd *cobra.Command, o awsReadOpts, accountID string) ([]awsx.Account, error) {
	if !o.live {
		accts, err := loadAccounts(cmd.Context(), o.db(), accountID)
		if err == nil && (len(accts) > 0 || o.localOnly) {
			return accts, nil
		}
	}
	if o.localOnly {
		return nil, nil
	}
	// Live fetch (member accounts get AccessDenied → return empty, not error).
	client, err := awsx.New(cmd.Context(), o.profile, o.region)
	if err != nil {
		return nil, err
	}
	accts, err := client.ListAccounts(cmd.Context())
	if err != nil {
		if awsx.IsAccessDenied(err) {
			return nil, nil
		}
		return nil, err
	}
	if accountID != "" {
		var filtered []awsx.Account
		for _, a := range accts {
			if a.AccountID == accountID {
				filtered = append(filtered, a)
			}
		}
		return filtered, nil
	}
	return accts, nil
}
