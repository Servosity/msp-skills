// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"xero-pp-cli/internal/xfin"
)

// pp:data-source local
func newNovelLedgerCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "ledger <account-code>",
		Short: "Replay the immutable journals feed for one account code as an ordered running-balance statement",
		Long: "Replay the immutable general-ledger journal feed for a single account code as an ordered\n" +
			"running-balance statement (date, journal number, description, net, running balance), computed\n" +
			"locally from the synced journals + accounts. Run `sync` first.",
		Example:     "  xero-cli ledger 200 --json\n  xero-cli ledger 610",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			if len(args) == 0 || args[0] == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("<account-code> is required"))
			}
			accountCode := args[0]
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			db, err := openXeroStore(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening store: %w", err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "journals") {
				hintIfStale(cmd, db, "journals", flags.maxAge)
			}
			jRows, err := loadAllRows(db, "journals")
			if err != nil {
				return fmt.Errorf("loading journals: %w", err)
			}
			aRows, err := loadAllRows(db, "accounts")
			if err != nil {
				return fmt.Errorf("loading accounts: %w", err)
			}
			stmt := xfin.ComputeLedger(xfin.DecodeJournals(jRows), accountCode, xfin.DecodeAccounts(aRows))

			if flags.asJSON || (!isTerminal(cmd.OutOrStdout()) && !humanFriendly) {
				return printJSONFiltered(cmd.OutOrStdout(), stmt, flags)
			}
			out := [][]string{}
			for _, e := range stmt.Entries {
				out = append(out, []string{strconv.FormatInt(e.JournalNumber, 10), e.Date, e.Description, money(e.NetAmount), money(e.RunningBalance)})
			}
			name := stmt.AccountName
			if name == "" {
				name = "(unknown account)"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Account %s %s — %d entries, final balance %s\n", stmt.AccountCode, name, stmt.EntryCount, money(stmt.FinalBalance))
			return flags.printTable(cmd, []string{"journal #", "date", "description", "net", "running balance"}, out)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/xero-cli/data.db)")
	return cmd
}
