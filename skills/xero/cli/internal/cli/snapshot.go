// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"xero-pp-cli/internal/store"
	"xero-pp-cli/internal/xfin"
)

var snapshotResourceTypes = []string{"accounts", "bank-transactions", "contacts", "invoices", "items", "journals", "payments"}

// pp:data-source local
func newNovelSnapshotCmd(flags *rootFlags) *cobra.Command {
	var dbPath, asOf string

	cmd := &cobra.Command{
		Use:   "snapshot",
		Short: "One offline call returning receivable/payable outstanding, overdue and unreconciled counts, and sync staleness",
		Long: "Compose a one-shot offline summary of the org: total receivable and payable outstanding, overdue\n" +
			"receivable count, unreconciled bank-transaction count, per-entity row counts, and per-table sync\n" +
			"staleness — one structured object an agent can read instead of fanning out across list endpoints.",
		Example:     "  xero-cli snapshot --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			as, err := parseAsOf(asOf)
			if err != nil {
				return err
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			db, err := openXeroStore(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening store: %w", err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "") {
				hintIfStale(cmd, db, "", flags.maxAge)
			}

			counts, err := resourceCounts(db)
			if err != nil {
				return fmt.Errorf("counting resources: %w", err)
			}
			lastSynced := map[string]string{}
			for _, rt := range snapshotResourceTypes {
				if ls := db.GetLastSyncedAt(rt); ls != "" {
					lastSynced[rt] = ls
				}
			}
			invRows, err := loadAllRows(db, "invoices")
			if err != nil {
				return fmt.Errorf("loading invoices: %w", err)
			}
			txnRows, err := loadAllRows(db, "bank-transactions")
			if err != nil {
				return fmt.Errorf("loading bank transactions: %w", err)
			}
			snap := xfin.ComputeSnapshot(xfin.DecodeInvoices(invRows), xfin.DecodeBankTransactions(txnRows), as, counts, lastSynced)

			if flags.asJSON || (!isTerminal(cmd.OutOrStdout()) && !humanFriendly) {
				return printJSONFiltered(cmd.OutOrStdout(), snap, flags)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Receivable outstanding: %s\nPayable outstanding:    %s\nOverdue receivables:    %d\nUnreconciled bank txns: %d\n",
				money(snap.ReceivableOutstanding), money(snap.PayableOutstanding), snap.OverdueReceivable, snap.UnreconciledBankTxns)
			out := [][]string{}
			for _, rt := range snapshotResourceTypes {
				out = append(out, []string{rt, strconv.Itoa(snap.Counts[rt]), snap.SyncStaleness[rt]})
			}
			return flags.printTable(cmd, []string{"resource", "rows", "last synced"}, out)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/xero-cli/data.db)")
	cmd.Flags().StringVar(&asOf, "as-of", "", "Overdue cutoff date (YYYY-MM-DD); defaults to today")
	return cmd
}

// resourceCounts returns the row count per resource_type in the store.
func resourceCounts(db *store.Store) (map[string]int, error) {
	rows, err := db.Query(`SELECT resource_type, COUNT(*) FROM resources GROUP BY resource_type`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var rt string
		var n int
		if err := rows.Scan(&rt, &n); err != nil {
			return nil, err
		}
		counts[rt] = n
	}
	return counts, rows.Err()
}
