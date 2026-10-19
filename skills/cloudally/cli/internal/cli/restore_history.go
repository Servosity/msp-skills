// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"
	"strings"
	"time"

	"cloudally-pp-cli/internal/cadate"
	"github.com/spf13/cobra"
)

const defaultHistoryLimit = 100

const emptyLedgerHint = "hint: the restore ledger is empty. Run 'cloudally-cli restore submit' to record a batch."

func newRestoreHistoryCmd(flags *rootFlags) *cobra.Command {
	var flagAccount, flagBatch, flagStatus, flagSince string
	var flagLimit int

	cmd := &cobra.Command{
		Use:   "history",
		Short: "List past restore batches, or one batch's rows, from the local ledger",
		Long:  "Use this command to see what was submitted in past batches from the local ledger. Do NOT use it to refresh statuses; use 'restore watch' instead.",
		Example: strings.Trim(`
  cloudally-cli restore history
  cloudally-cli restore history --account 5f1c2a --since 2026-09-01
  cloudally-cli restore history --batch 20261015-120000-ab12
  cloudally-cli restore history --batch 20261015-120000-ab12 --status FAILED --json
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":  "true",
			"pp:data-source": "local",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return usageErr(err)
			}
			if flagLimit < 0 {
				return usageErr(fmt.Errorf("--limit must be 0 or more"))
			}
			var since time.Time
			if flagSince != "" {
				t, dateOnly, err := cadate.ParseUserDate(flagSince)
				if err != nil {
					return usageErr(fmt.Errorf("--since %w", err))
				}
				since = t
				if dateOnly {
					since = t.Truncate(24 * time.Hour)
				}
			}
			if dryRunOK(flags) {
				return writeWould(cmd.OutOrStdout(), flags, "restore history", "read restore batches from the local ledger", nil)
			}
			ctx := cmd.Context()
			db, ok, err := openLedgerRead(ctx)
			if err != nil {
				return fmt.Errorf("opening ledger: %w", err)
			}
			human := wantsHumanTable(cmd.OutOrStdout(), flags)
			if !ok {
				fmt.Fprintln(cmd.ErrOrStderr(), emptyLedgerHint)
				if !human {
					return printJSONFiltered(cmd.OutOrStdout(), []any{}, flags)
				}
				return nil
			}
			defer db.Close()
			sqlDB := db.DB()
			if flagBatch != "" {
				batch, err := ledgerGetBatch(sqlDB, flagBatch)
				if err != nil {
					return fmt.Errorf("reading ledger: %w", err)
				}
				if batch == nil {
					return notFoundErr(fmt.Errorf("unknown batch %q", flagBatch))
				}
				jobs, err := ledgerListJobs(sqlDB, flagBatch, flagStatus)
				if err != nil {
					return fmt.Errorf("reading ledger: %w", err)
				}
				if flagLimit > 0 && len(jobs) > flagLimit {
					jobs = jobs[:flagLimit]
				}
				if !human {
					return printJSONFiltered(cmd.OutOrStdout(), jobs, flags)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "batch %s  task %s (%s)  scope %s  account %s  created %s\n\n", batch.BatchID, batch.TaskID, batch.Source, batch.Scope, historyAccountLabel(batch.AccountID, batch.AuthScope), isoUTC(batch.CreatedAt))
				rows := make([][]string, 0, len(jobs))
				for _, j := range jobs {
					rows = append(rows, []string{j.Entity, j.Requested, j.Resolved, j.Status, j.TaskID, strings.TrimSpace(truncate(j.Error, 50))})
				}
				return printRestoreTable(cmd.OutOrStdout(), []string{"entity", "requested", "resolved", "status", "task_id", "error"}, rows)
			}
			batches, err := ledgerListBatches(sqlDB, historyFilter{account: flagAccount, status: flagStatus, since: since, limit: flagLimit})
			if err != nil {
				return fmt.Errorf("reading ledger: %w", err)
			}
			if len(batches) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), emptyLedgerHint)
			}
			if !human {
				if batches == nil {
					batches = []batchSummary{}
				}
				return printJSONFiltered(cmd.OutOrStdout(), batches, flags)
			}
			rows := make([][]string, 0, len(batches))
			for _, b := range batches {
				refresh := ""
				if b.LastRefreshAt != nil {
					refresh = isoUTC(*b.LastRefreshAt)
				}
				rows = append(rows, []string{b.BatchID, isoUTC(b.CreatedAt), b.Scope, historyAccountLabel(b.AccountID, b.AuthScope), b.TaskID,
					fmt.Sprint(b.Rows), fmt.Sprint(b.Submitted), fmt.Sprint(b.Failed), fmt.Sprint(b.Unknown), refresh})
			}
			return printRestoreTable(cmd.OutOrStdout(), []string{"batch", "created", "scope", "account", "task", "rows", "submitted", "failed", "unknown", "last_refresh"}, rows)
		},
	}
	cmd.Flags().StringVar(&flagAccount, "account", "", "Only batches for this customer account id")
	cmd.Flags().StringVar(&flagBatch, "batch", "", "Show the rows of one batch instead of the batch list")
	cmd.Flags().StringVar(&flagStatus, "status", "", "Only rows (or batches containing rows) with this status, for example FAILED")
	cmd.Flags().StringVar(&flagSince, "since", "", "Only batches created on or after this date (YYYY-MM-DD or RFC3339)")
	cmd.Flags().IntVar(&flagLimit, "limit", defaultHistoryLimit, "Maximum batches or rows to print (0 for all)")
	return cmd
}

// historyAccountLabel renders the account a batch was recorded for, with
// how the token was obtained when known.
func historyAccountLabel(accountID, authScope string) string {
	switch {
	case accountID != "" && authScope != "":
		return accountID + " (" + authScope + ")"
	case accountID != "":
		return accountID
	case authScope != "":
		return "(" + authScope + ")"
	default:
		return "-"
	}
}
