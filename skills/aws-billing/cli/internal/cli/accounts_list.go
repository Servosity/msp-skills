// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// `accounts list`: list the AWS Organizations accounts of the credentials'
// account: the complete list a sync stored for that account, else a live
// Organizations call. An organization with no accounts prints [] (exit 0);
// a list that cannot be read (a member account, a refused call) is an error.
//
// pp:client-call
package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"aws-billing-pp-cli/internal/awsx"
	"aws-billing-pp-cli/internal/cliutil"
	"aws-billing-pp-cli/internal/store"
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

// accountsForList returns the Organizations accounts of the credentials'
// account: its complete stored list (aws-billing handfix
// cost-cache-answers-only-its-request), else (and with --data-source live) a
// live Organizations call. A refused or failed call is an error, never an
// empty organization (Codex r1 class failure-read-as-an-empty-result: a
// member account cannot list the organization, which is not "no accounts").
func accountsForList(cmd *cobra.Command, o awsReadOpts, accountID string) ([]awsx.Account, error) {
	ctx := cmd.Context()
	if !o.live {
		db, err := store.OpenWithContext(ctx, o.db())
		if err != nil && o.localOnly {
			return nil, err
		}
		if err == nil {
			accts, ok, lerr := storedAccounts(ctx, o, db, accountID)
			_ = db.Close()
			if lerr != nil {
				return nil, lerr
			}
			if ok {
				return accts, nil
			}
		}
	}
	client, err := o.awsClient(ctx)
	if err != nil {
		return nil, err
	}
	accts, err := client.ListAccounts(ctx)
	if err != nil {
		if awsx.IsAccessDenied(err) {
			return nil, fmt.Errorf("organizations:ListAccounts denied: only the organization's management account (or a delegated administrator) can list its accounts; run from that account's profile: %w", err)
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

// storedAccounts reads the account's complete stored list; ok only when one
// is recorded. --data-source local without one is an error.
func storedAccounts(ctx context.Context, o awsReadOpts, db *store.Store, accountID string) ([]awsx.Account, bool, error) {
	scope, err := o.readScope(ctx, db)
	if err != nil {
		return nil, false, err
	}
	rows, recorded, err := db.AccountListIfRecorded(ctx, scope, accountID) // one read transaction
	if err != nil {
		return nil, false, err
	}
	if !recorded {
		if o.localOnly {
			return nil, false, fmt.Errorf("the local store holds no Organizations account list of AWS account %s (run 'aws-billing-cli sync' from the management account's profile)", scope)
		}
		return nil, false, nil
	}
	accts := make([]awsx.Account, 0, len(rows))
	for _, r := range rows {
		accts = append(accts, awsx.Account{ID: r.AccountID, AccountID: r.AccountID, Name: r.Name, Status: r.Status, Email: r.Email})
	}
	return accts, true, nil
}
