// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source auto
// pp:client-call -- fleet reaches the API only through child zoom-phone-cli
// processes, one per saved client account (see zoom_fleet.go).

package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

const fleetMaxConcurrency = 32

func newNovelFleetCmd(flags *rootFlags) *cobra.Command {
	var flagAccounts string
	var flagConcurrency int
	var flagAllowWrites bool

	cmd := &cobra.Command{
		Use:   "fleet -- <command> [args...]",
		Short: "Run one read-only command (or sync) across every saved client account and get one merged result with an account column.",
		Long: `Run one read-only command (or sync) across every saved client account and get one merged result with an account column. Do NOT use it to add or list accounts; use 'accounts connect' or 'accounts list' instead.

fleet runs the command once per account saved with 'accounts connect' (all of
them, or the ones named in --accounts), each as a separate zoom-phone-cli
process with that account's own config, cached token and local store. Every
ZOOM_PHONE_* environment variable outside a small allowlist (learning-loop and
user-agent settings) is removed from each child, including the credential
variables and the ZOOM_PHONE_BASE_URL / ZOOM_PHONE_TOKEN_URL host overrides, so
no account can run with another account's credentials or host. --json is added
unless the command already passes --json or --agent.

Everything after the command name belongs to that command; put fleet's own
flags before it. By default only read-only commands run (plus doctor and
version), and sync and other commands that write only the account's own local
store. Anything that writes to a client account or returns secrets needs
--allow-writes (from an MCP tool call also confirm: true, otherwise fleet
returns its plan); offboard additionally needs --accounts naming exactly one
account. fleet never runs fleet, accounts or auth, and refuses per-account
flags that pick another config, home, run profile, database or local file
(--config, --home, --profile, --db, --output, --receipt-file, --audit-dir,
--deliver).

Output is one row per account, in account order:
account, account_id, exit_code, ok, result (the command's JSON output, or its
raw text) and stderr (last 2 KB). Exit code: 0 when every account succeeded, 6
when some failed, 5 when all failed.`,
		Example: strings.Trim(`
  zoom-phone-cli fleet -- doctor
  zoom-phone-cli fleet -- sync
  zoom-phone-cli fleet -- users list-phone --page-size 100
  zoom-phone-cli fleet --accounts acme,globex --concurrency 2 -- e911 audit
  zoom-phone-cli fleet --dry-run -- users list-phone`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":       "false",
			"pp:data-source":      "auto",
			"pp:happy-args":       "command=doctor",
			"pp:typed-exit-codes": "0,2,3,5,6",
		},
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if len(args) == 0 {
				if dryRunOK(flags) {
					return writeDryRun(cmd.OutOrStdout(), flags, "fleet")
				}
				_ = cmd.Usage()
				return usageErr(errors.New("fleet needs a command to run, e.g. zoom-phone-cli fleet -- doctor"))
			}
			if flagConcurrency < 1 || flagConcurrency > fleetMaxConcurrency {
				return usageErr(fmt.Errorf("--concurrency must be between 1 and %d", fleetMaxConcurrency))
			}
			root := cmd.Root()
			target, err := fleetResolveTarget(root, args, flagAllowWrites)
			if err != nil {
				return err
			}
			if err := fleetCheckSingleAccount(target, flagAccounts); err != nil {
				return err
			}
			childArgs := fleetChildArgs(args, target.parsed)

			all, err := acctList()
			if err != nil {
				return err
			}
			accounts, err := fleetSelectAccounts(all, flagAccounts)
			if err != nil {
				return err
			}
			if err := fleetVerifyAccounts(accounts); err != nil {
				return err
			}
			if len(accounts) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "no saved accounts; add one with: zoom-phone-cli accounts connect <name> --account-id <id> --client-id <id> --client-secret-stdin")
			}

			commandPath := strings.TrimSpace(strings.TrimPrefix(target.cmd.CommandPath(), root.Name()))
			// zoom-phone hand-fix: zoom-phone-mcp-write-confirm. From an MCP
			// call, a command that may write to client accounts or return
			// secrets runs only with confirm: true; otherwise fleet returns
			// its plan.
			mcpPreview := target.access == fleetAccessWrite && zoomMCPPreviewOnly(cmd)
			if dryRunOK(flags) || mcpPreview {
				plan := fleetPlan{DryRun: true, Command: commandPath, ReadOnly: target.access == fleetAccessReadOnly, Access: target.access, Accounts: make([]fleetPlanItem, 0, len(accounts))}
				if mcpPreview {
					plan.Note = zoomMCPPreviewNote
				}
				for _, a := range accounts {
					plan.Accounts = append(plan.Accounts, fleetPlanItem{Account: a.Name, AccountID: a.AccountID, Config: a.Path, Args: childArgs})
				}
				if !wantsHumanTable(cmd.OutOrStdout(), flags) {
					return printJSONFilteredKeep(cmd.OutOrStdout(), plan, flags, "dry_run", "command", "read_only", "access", "accounts", "note")
				}
				fmt.Fprintf(cmd.OutOrStdout(), "dry-run: would run %q (%s) on %d account(s); nothing was started\n", root.Name()+" "+strings.Join(childArgs, " "), target.access, len(accounts))
				for _, a := range accounts {
					fmt.Fprintf(cmd.OutOrStdout(), "  %s (%s)\n", a.Name, a.AccountID)
				}
				return nil
			}

			results := make([]fleetResult, 0)
			if len(accounts) > 0 {
				exe, err := os.Executable()
				if err != nil {
					return fmt.Errorf("fleet: locating this executable: %w", err)
				}
				results = fleetRun(cmd.Context(), exe, os.Environ(), accounts, childArgs, flagConcurrency)
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				if err := printJSONFilteredKeep(cmd.OutOrStdout(), results, flags, fleetResultKeys...); err != nil {
					return err
				}
			} else if len(results) > 0 {
				tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
				fmt.Fprintln(tw, "ACCOUNT\tACCOUNT_ID\tOK\tEXIT\tRESULT")
				for _, r := range results {
					fmt.Fprintf(tw, "%s\t%s\t%t\t%d\t%s\n", r.Account, r.AccountID, r.OK, r.ExitCode, fleetSummarize(r))
				}
				if err := tw.Flush(); err != nil {
					return err
				}
			}
			return fleetOutcomeErr(commandPath, results)
		},
	}
	cmd.Flags().SetInterspersed(false)
	cmd.Flags().StringVar(&flagAccounts, "accounts", "", "Comma-separated account names to run on (default: every saved account)")
	cmd.Flags().IntVar(&flagConcurrency, "concurrency", 4, "How many accounts to run at once (1-32)")
	cmd.Flags().BoolVar(&flagAllowWrites, "allow-writes", false, "Allow commands that write to client accounts or return secrets (they run against every selected account)")
	return cmd
}
