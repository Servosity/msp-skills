// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"cisco-duo-pp-cli/internal/client"
)

type findUserHit struct {
	AccountID     string `json:"account_id"`
	AccountName   string `json:"account_name"`
	UserID        string `json:"user_id"`
	Username      string `json:"username"`
	Realname      string `json:"realname"`
	Email         string `json:"email"`
	Status        string `json:"status"`
	LockoutReason string `json:"lockout_reason"`
	IsEnrolled    bool   `json:"is_enrolled"`
	LastLogin     string `json:"last_login"`
}

func toHits(a client.DuoSubaccount, users []duoUser) []findUserHit {
	hits := make([]findUserHit, 0, len(users))
	for _, u := range users {
		hits = append(hits, findUserHit{AccountID: a.AccountID, AccountName: a.Name, UserID: u.UserID, Username: u.Username,
			Realname: u.Realname, Email: u.Email, Status: u.Status, LockoutReason: u.lockoutReason(), IsEnrolled: u.IsEnrolled,
			LastLogin: unixPtrTime(u.LastLogin)})
	}
	return hits
}

func newNovelFleetFindUserCmd(flags *rootFlags) *cobra.Command {
	var only string
	var skipParent bool
	cmd := &cobra.Command{
		Use:   "find-user [username-or-email]",
		Short: "Find which customer tenants contain a username, alias or email",
		Long: strings.Trim(`
Use this command to locate which customer tenants contain a username or email.
It looks the value up in the MSP parent account and in every subaccount (by
username or alias, and also by email when the value contains '@') and
returns each hit with its account, status, lockout reason and last login.

Do NOT use it to diagnose why the person cannot log in; use 'lockout triage'
instead.`, "\n"),
		Example: strings.Trim(`
  cisco-duo-cli fleet find-user jdoe@example.com --agent
  cisco-duo-cli fleet find-user jdoe --only "Acme Corp,Globex"`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:happy-args": "who=jdoe@example.com"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet find-user")
			}
			if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a username or email is required"))
			}
			who := strings.TrimSpace(args[0])
			duoFleetPrep()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			accounts, err := fleetAccounts(ctx, c, only)
			if err != nil {
				return err
			}
			hits := make([]findUserHit, 0)
			failures := make([]fleetFailure, 0)
			searchParent := !skipParent && strings.TrimSpace(only) == ""
			attempted := len(accounts)
			if searchParent {
				attempted++
				users, err := duoLookupUsers(ctx, c, "", who)
				if err != nil {
					failures = append(failures, fleetFailure{Name: "(parent)", Error: firstLine(err)})
				} else {
					hits = append(hits, toHits(client.DuoSubaccount{Name: "(parent)"}, users)...)
				}
			}
			perAccount, childFailures := fanoutAccounts(ctx, accounts, func(ctx context.Context, a client.DuoSubaccount) ([]findUserHit, error) {
				users, err := duoLookupUsers(ctx, c, a.AccountID, who)
				if err != nil {
					return nil, err
				}
				return toHits(a, users), nil
			})
			for _, h := range perAccount {
				hits = append(hits, h...)
			}
			failures = append(failures, childFailures...)
			sort.Slice(hits, func(i, j int) bool {
				return strings.ToLower(hits[i].AccountName) < strings.ToLower(hits[j].AccountName)
			})
			warnFleetFailures(cmd, failures, attempted)
			view := struct {
				Query           string         `json:"query"`
				AccountsChecked int            `json:"accounts_checked"`
				Hits            []findUserHit  `json:"hits"`
				FetchFailures   []fleetFailure `json:"fetch_failures,omitempty"`
			}{Query: who, AccountsChecked: len(accounts), Hits: hits, FetchFailures: failures}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				if err := printJSONFiltered(cmd.OutOrStdout(), view, flags); err != nil {
					return err
				}
				return allFleetFailed(len(failures), attempted)
			}
			w := cmd.OutOrStdout()
			if len(hits) == 0 {
				fmt.Fprintf(w, "No user matches %q in %d subaccount(s) or the parent.\n", who, len(accounts))
			}
			for _, h := range hits {
				fmt.Fprintf(w, "%-28s %-22s %-20s %-12s %s\n", trunc(h.AccountName, 28), h.AccountID, h.Username, h.Status, h.LastLogin)
			}
			for _, f := range failures {
				fmt.Fprintf(w, "FAILED %s %s: %s\n", f.Name, f.AccountID, f.Error)
			}
			return allFleetFailed(len(failures), attempted)
		},
	}
	cmd.Flags().StringVar(&only, "only", "", "Comma-separated subaccount IDs or names to search (skips the parent)")
	cmd.Flags().BoolVar(&skipParent, "skip-parent", false, "Do not search the MSP parent account itself")
	return cmd
}
