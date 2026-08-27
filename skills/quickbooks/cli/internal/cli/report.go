// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// QuickBooks Online's financial statements live behind the Reports API
// (/reports/ProfitAndLoss, /reports/BalanceSheet), which the /query endpoint
// the rest of this CLI rides cannot reach. These commands pull the
// recognized-revenue, margin, and balance-sheet figures financial reporting
// needs straight from the books — instead of reconstructing them from raw
// JournalEntry rows.

func newReportCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Financial statements from the QBO Reports API (P&L, balance sheet)",
		Long: "Pull QuickBooks financial statements directly from the Reports API.\n" +
			"Use this for recognized revenue, gross/operating margin, and balance-sheet\n" +
			"balances (deferred revenue, accrued interest, debt) — figures the /query\n" +
			"endpoint cannot aggregate. Read-only.",
		RunE: parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newReportPnlCmd(flags))
	cmd.AddCommand(newReportBalanceSheetCmd(flags))
	return cmd
}

// --- shared report-tree parsing ---

type reportLeaf struct {
	Name    string  `json:"name"`
	Amount  float64 `json:"amount"`
	Section string  `json:"section,omitempty"`
}

type qbCol struct {
	Value string `json:"value"`
	ID    string `json:"id"`
}

type qbColData struct {
	ColData []qbCol `json:"ColData"`
}

type qbRows struct {
	Row []qbRow `json:"Row"`
}

type qbRow struct {
	Header  *qbColData `json:"Header"`
	Rows    *qbRows    `json:"Rows"`
	Summary *qbColData `json:"Summary"`
	ColData []qbCol    `json:"ColData"`
}

type qbReport struct {
	Header struct {
		StartPeriod string `json:"StartPeriod"`
		EndPeriod   string `json:"EndPeriod"`
		ReportBasis string `json:"ReportBasis"`
		Currency    string `json:"Currency"`
		ReportName  string `json:"ReportName"`
	} `json:"Header"`
	Rows qbRows `json:"Rows"`
}

// parseReportAmount tolerates the formats QBO emits: plain "1234.56",
// thousands-separated "1,234.56", parenthesised negatives "(789.00)", and
// empty cells. The bool reports whether a numeric value was present at all.
func parseReportAmount(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	neg := false
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		neg = true
		s = s[1 : len(s)-1]
	}
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, "$", "")
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	if neg {
		f = -f
	}
	return f, true
}

// walkReport flattens the QBO report tree into leaf account rows and section
// summary rows. `section` threads the enclosing group label (from a row's
// Header) down to its leaves so callers can tell an Income account from a COGS
// account without re-deriving the hierarchy.
func walkReport(rows qbRows, section string, leaves, summaries *[]reportLeaf) {
	for _, r := range rows.Row {
		groupLabel := section
		if r.Header != nil && len(r.Header.ColData) > 0 {
			if name := strings.TrimSpace(r.Header.ColData[0].Value); name != "" {
				groupLabel = name
			}
		}
		// A data row carries its account name + amount directly in ColData.
		if len(r.ColData) >= 2 {
			name := strings.TrimSpace(r.ColData[0].Value)
			if name != "" {
				amt, ok := parseReportAmount(r.ColData[len(r.ColData)-1].Value)
				if ok {
					*leaves = append(*leaves, reportLeaf{Name: name, Amount: amt, Section: section})
				}
			}
		}
		if r.Rows != nil {
			walkReport(*r.Rows, groupLabel, leaves, summaries)
		}
		if r.Summary != nil && len(r.Summary.ColData) >= 2 {
			name := strings.TrimSpace(r.Summary.ColData[0].Value)
			if amt, ok := parseReportAmount(r.Summary.ColData[len(r.Summary.ColData)-1].Value); ok && name != "" {
				*summaries = append(*summaries, reportLeaf{Name: name, Amount: amt})
			}
		}
	}
}

// summaryValue returns the amount of the last summary row whose lowercased
// name satisfies match. Last-wins because report grand totals appear once and
// after any same-named subtotal.
func summaryValue(summaries []reportLeaf, match func(string) bool) (float64, bool) {
	out, found := 0.0, false
	for _, s := range summaries {
		if match(strings.ToLower(s.Name)) {
			out, found = s.Amount, true
		}
	}
	return out, found
}

func contains(sub string) func(string) bool {
	return func(s string) bool { return strings.Contains(s, sub) }
}

// monthBounds returns the first and last day (YYYY-MM-DD) of the given
// YYYY-MM. When month is empty it defaults to the previous calendar month
// (the most recently closed period).
func monthBounds(month string) (string, string, error) {
	var first time.Time
	if month == "" {
		now := time.Now()
		first = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -1, 0)
	} else {
		t, err := time.Parse("2006-01", month)
		if err != nil {
			return "", "", usageErr(fmt.Errorf("invalid --month %q: expected YYYY-MM", month))
		}
		first = t
	}
	last := first.AddDate(0, 1, -1)
	return first.Format("2006-01-02"), last.Format("2006-01-02"), nil
}

// --- report pnl ---

type pnlMetrics struct {
	TotalIncome        float64  `json:"total_income"`
	RecurringRevenue   float64  `json:"recurring_revenue"`
	COGS               float64  `json:"cogs"`
	GrossProfit        float64  `json:"gross_profit"`
	GrossMarginPct     *float64 `json:"gross_margin_pct,omitempty"`
	OperatingIncome    float64  `json:"operating_income"`
	OperatingMarginPct *float64 `json:"operating_margin_pct,omitempty"`
	NetIncome          float64  `json:"net_income"`
}

type pnlView struct {
	Report    string       `json:"report"`
	StartDate string       `json:"start_date"`
	EndDate   string       `json:"end_date"`
	Basis     string       `json:"basis"`
	Currency  string       `json:"currency"`
	Metrics   pnlMetrics   `json:"metrics"`
	Accounts  []reportLeaf `json:"accounts"`
	Summaries []reportLeaf `json:"summaries"`
}

func newReportPnlCmd(flags *rootFlags) *cobra.Command {
	var month, start, end string
	var cash bool
	cmd := &cobra.Command{
		Use:   "pnl",
		Short: "Profit & Loss: recognized revenue, COGS, gross/operating margin, net income",
		Long: "Pull the QBO Profit & Loss report for a period (default: last closed month).\n" +
			"Returns recognized revenue by account, COGS, gross profit/margin, operating\n" +
			"income/margin, and net income — the closed-books numbers, accrual basis.",
		Example: "  quickbooks-cli report pnl --month 2026-05 --json\n" +
			"  quickbooks-cli report pnl --start 2026-01-01 --end 2026-03-31 --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if start == "" || end == "" {
				s, e, err := monthBounds(month)
				if err != nil {
					return err
				}
				if start == "" {
					start = s
				}
				if end == "" {
					end = e
				}
			}
			method := "Accrual"
			if cash {
				method = "Cash"
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			params := map[string]string{
				"start_date":        start,
				"end_date":          end,
				"accounting_method": method,
				"minorversion":      "75",
			}
			data, err := c.Get(cmd.Context(), "/reports/ProfitAndLoss", params)
			if err != nil {
				return classifyAPIError(err, flags)
			}
			var rep qbReport
			if err := json.Unmarshal(data, &rep); err != nil {
				return apiErr(fmt.Errorf("parsing ProfitAndLoss report: %w", err))
			}
			var leaves, summaries []reportLeaf
			walkReport(rep.Rows, "", &leaves, &summaries)

			m := pnlMetrics{}
			m.TotalIncome, _ = summaryValue(summaries, func(s string) bool { return s == "total income" })
			if v, ok := summaryValue(summaries, contains("recurring revenue")); ok {
				m.RecurringRevenue = v
			} else {
				m.RecurringRevenue = m.TotalIncome
			}
			m.COGS, _ = summaryValue(summaries, contains("cost of goods sold"))
			m.GrossProfit, _ = summaryValue(summaries, func(s string) bool { return s == "gross profit" })
			m.OperatingIncome, _ = summaryValue(summaries, contains("net operating income"))
			m.NetIncome, _ = summaryValue(summaries, func(s string) bool { return s == "net income" })
			if m.TotalIncome != 0 {
				gm := m.GrossProfit / m.TotalIncome * 100
				om := m.OperatingIncome / m.TotalIncome * 100
				m.GrossMarginPct = &gm
				m.OperatingMarginPct = &om
			}

			view := pnlView{
				Report:    "ProfitAndLoss",
				StartDate: rep.Header.StartPeriod,
				EndDate:   rep.Header.EndPeriod,
				Basis:     rep.Header.ReportBasis,
				Currency:  rep.Header.Currency,
				Metrics:   m,
				Accounts:  leaves,
				Summaries: summaries,
			}
			if view.StartDate == "" {
				view.StartDate = start
			}
			if view.EndDate == "" {
				view.EndDate = end
			}
			return flags.printJSON(cmd, view)
		},
	}
	cmd.Flags().StringVar(&month, "month", "", "Period as YYYY-MM (default: last closed month)")
	cmd.Flags().StringVar(&start, "start", "", "Start date YYYY-MM-DD (overrides --month)")
	cmd.Flags().StringVar(&end, "end", "", "End date YYYY-MM-DD (overrides --month)")
	cmd.Flags().BoolVar(&cash, "cash", false, "Cash basis (default: accrual)")
	return cmd
}

// --- report balance-sheet ---

// highlightMatch is one resolved --highlight term: the total found for it and
// where it came from ("summary" for a group/parent total, "leaves" for a sum
// of matching account rows).
type highlightMatch struct {
	Match   string   `json:"match"`
	Amount  float64  `json:"amount"`
	Source  string   `json:"source"`
	Matched []string `json:"matched,omitempty"`
}

// matchHighlights resolves each --highlight substring against the report. A
// term first tries the summary rows: a group/parent total (QBO emits these as
// "Total ... <name>" summary rows, e.g. "Total ... Deferred Revenue") lands in
// summaries, not leaves, so summing the partial child leaves would undercount.
// If no summary matches, it sums the leaf accounts whose name contains the term
// (e.g. "accrued interest", a leaf-level line). Last-wins on summaries so a
// grand total beats an earlier subtotal. Matching is case-insensitive.
func matchHighlights(terms []string, leaves, summaries []reportLeaf) []highlightMatch {
	var out []highlightMatch
	for _, t := range terms {
		term := strings.ToLower(strings.TrimSpace(t))
		if term == "" {
			continue
		}
		amt, name, ok := 0.0, "", false
		for _, s := range summaries {
			if strings.Contains(strings.ToLower(s.Name), term) {
				amt, name, ok = s.Amount, s.Name, true
			}
		}
		if ok {
			out = append(out, highlightMatch{Match: t, Amount: amt, Source: "summary", Matched: []string{name}})
			continue
		}
		var names []string
		var total float64
		for _, l := range leaves {
			if strings.Contains(strings.ToLower(l.Name), term) {
				total += l.Amount
				names = append(names, l.Name)
			}
		}
		if len(names) > 0 {
			out = append(out, highlightMatch{Match: t, Amount: total, Source: "leaves", Matched: names})
		}
	}
	return out
}

type bsView struct {
	Report     string           `json:"report"`
	AsOf       string           `json:"as_of"`
	Basis      string           `json:"basis"`
	Currency   string           `json:"currency"`
	Highlights []highlightMatch `json:"highlights,omitempty"`
	Accounts   []reportLeaf     `json:"accounts"`
	Summaries  []reportLeaf     `json:"summaries"`
}

func newReportBalanceSheetCmd(flags *rootFlags) *cobra.Command {
	var asOf string
	var cash bool
	var highlight []string
	cmd := &cobra.Command{
		Use:   "balance-sheet",
		Short: "Balance sheet: every account + group balance, with optional --highlight totals",
		Long: "Pull the QBO Balance Sheet as of a date (default: today). Returns every\n" +
			"account leaf and summary total. Pass --highlight to total and surface any\n" +
			"account or group by name substring (repeatable), e.g. \"deferred revenue\"\n" +
			"or \"accrued interest\". Read-only.",
		Example: "  quickbooks-cli report balance-sheet --json\n" +
			"  quickbooks-cli report balance-sheet --highlight \"deferred revenue\" --highlight \"accrued interest\" --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if asOf == "" {
				asOf = time.Now().UTC().Format("2006-01-02")
			}
			method := "Accrual"
			if cash {
				method = "Cash"
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			params := map[string]string{
				"start_date":        asOf,
				"end_date":          asOf,
				"accounting_method": method,
				"minorversion":      "75",
			}
			data, err := c.Get(cmd.Context(), "/reports/BalanceSheet", params)
			if err != nil {
				return classifyAPIError(err, flags)
			}
			var rep qbReport
			if err := json.Unmarshal(data, &rep); err != nil {
				return apiErr(fmt.Errorf("parsing BalanceSheet report: %w", err))
			}
			var leaves, summaries []reportLeaf
			walkReport(rep.Rows, "", &leaves, &summaries)

			view := bsView{
				Report:     "BalanceSheet",
				AsOf:       rep.Header.EndPeriod,
				Basis:      rep.Header.ReportBasis,
				Currency:   rep.Header.Currency,
				Highlights: matchHighlights(highlight, leaves, summaries),
				Accounts:   leaves,
				Summaries:  summaries,
			}
			if view.AsOf == "" {
				view.AsOf = asOf
			}
			return flags.printJSON(cmd, view)
		},
	}
	cmd.Flags().StringVar(&asOf, "as-of", "", "Balance-sheet date YYYY-MM-DD (default: today)")
	cmd.Flags().BoolVar(&cash, "cash", false, "Cash basis (default: accrual)")
	cmd.Flags().StringArrayVar(&highlight, "highlight", nil, "Account or group name substring to total and highlight (repeatable, case-insensitive)")
	return cmd
}
