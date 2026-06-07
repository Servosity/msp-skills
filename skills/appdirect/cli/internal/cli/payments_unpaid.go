// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-implemented novel feature: cross-company failed/overdue payment radar
// over the local payments table. Originally scaffolded by the CLI Printing
// Press; body is hand-authored.

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"appdirect-pp-cli/internal/cliutil"
	"appdirect-pp-cli/internal/store"
)

// pp:data-source local

type unpaidPayment struct {
	PaymentID     string  `json:"paymentId"`
	CompanyID     string  `json:"companyId,omitempty"`
	CompanyName   string  `json:"companyName,omitempty"`
	Amount        float64 `json:"amount"`
	Currency      string  `json:"currency,omitempty"`
	Result        string  `json:"result"`
	Method        string  `json:"method,omitempty"`
	Date          string  `json:"date,omitempty"`
	AgeDays       int     `json:"ageDays"`
	TransactionID string  `json:"transactionId,omitempty"`
}

type unpaidView struct {
	Since           string          `json:"since"`
	Payments        []unpaidPayment `json:"payments"`
	ScannedPayments int             `json:"scanned_payments"`
	Note            string          `json:"note,omitempty"`
}

func newNovelPaymentsUnpaidCmd(flags *rootFlags) *cobra.Command {
	var flagSince string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "unpaid",
		Short: "List failed or unavailable-gateway payments across all companies, sorted by amount and age",
		Long: strings.TrimSpace(`
Use this command to list failed or overdue payments across all companies.
It scans the locally synced billing-v1-payments table for payments whose
result is FAILED or GATEWAY_NOT_AVAILABLE inside the window, sorted by
amount (largest first) then age.

Do NOT use this command for full invoice-to-subscription matching; use
'reconcile' instead. Run 'sync --resources billing-v1-payments' first to
populate the local store.`),
		Example: strings.Trim(`
  # This week's failed payments, largest first
  appdirect-cli payments unpaid --since 7d --json

  # Month-scale view for close, narrowed for agents
  appdirect-cli payments unpaid --since 30d --agent --select payments.companyName,payments.amount,payments.result
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would scan local payments for FAILED / GATEWAY_NOT_AVAILABLE results")
				return nil
			}
			if flagSince == "" {
				flagSince = "30d"
			}
			sinceDur, err := cliutil.ParseDurationLoose(flagSince)
			if err != nil {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--since: %w", err))
			}
			now := time.Now().UTC()
			cutoff := now.Add(-sinceDur)

			if dbPath == "" {
				dbPath = defaultDBPath("appdirect-cli")
			}
			db, err := store.OpenWithContext(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			defer db.Close()

			if !hintIfUnsynced(cmd, db, rtPayments) {
				hintIfStale(cmd, db, rtPayments, flags.maxAge)
			}

			payments, err := loadResourceObjects(cmd.Context(), db, rtPayments)
			if err != nil {
				return err
			}
			companies, err := loadResourceObjects(cmd.Context(), db, rtCompanies)
			if err != nil {
				return err
			}
			names := companyNameIndex(companies)

			items := make([]unpaidPayment, 0)
			for _, pay := range payments {
				result := strings.ToUpper(novelStr(pay, "result"))
				if result != "FAILED" && result != "GATEWAY_NOT_AVAILABLE" {
					continue
				}
				date, _ := novelEpochMS(pay, "date")
				if !date.IsZero() && date.Before(cutoff) {
					continue
				}
				amount, _ := novelNum(pay, "amount")
				companyID := novelRefID(pay, "company")
				paymentID := novelStr(pay, "id")
				if paymentID == "" {
					paymentID = novelStr(pay, "paymentNumber")
				}
				items = append(items, unpaidPayment{
					PaymentID:     paymentID,
					CompanyID:     companyID,
					CompanyName:   names[companyID],
					Amount:        amount,
					Currency:      novelStr(pay, "currency"),
					Result:        result,
					Method:        novelStr(pay, "method"),
					Date:          isoOrEmpty(date),
					AgeDays:       ageDays(date, now),
					TransactionID: novelStr(pay, "transactionId"),
				})
			}
			sort.SliceStable(items, func(i, j int) bool {
				if items[i].Amount != items[j].Amount {
					return items[i].Amount > items[j].Amount
				}
				return items[i].AgeDays > items[j].AgeDays
			})

			view := unpaidView{
				Since:           flagSince,
				Payments:        items,
				ScannedPayments: len(payments),
			}
			if len(items) == 0 {
				view.Note = fmt.Sprintf("scanned %d payments with no FAILED or GATEWAY_NOT_AVAILABLE results in the %s window; widen with --since or refresh with 'sync --resources billing-v1-payments'", len(payments), flagSince)
			}
			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "30d", "Window for payment date filtering (e.g. 7d, 30d, 12h)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
