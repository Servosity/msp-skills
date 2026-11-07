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
// (/reports/ProfitAndLoss, /reports/BalanceSheet, /reports/CashFlow), which
// the /query endpoint the rest of this CLI rides cannot reach. These commands
// pull the recognized-revenue, margin, balance-sheet and cash-flow figures
// financial reporting needs straight from the books, instead of
// reconstructing them from raw JournalEntry rows.

func newReportCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Financial statements from the QBO Reports API (P&L, balance sheet, cash flow)",
		Long: "Pull QuickBooks financial statements directly from the Reports API.\n" +
			"Use this for recognized revenue, gross/operating margin, balance-sheet\n" +
			"balances (deferred revenue, accrued interest, debt) and the statement of\n" +
			"cash flows: figures the /query endpoint cannot aggregate. Every report\n" +
			"takes --columns total|month|quarter and --raw. Read-only.",
		RunE: parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newReportPnlCmd(flags))
	cmd.AddCommand(newReportBalanceSheetCmd(flags))
	cmd.AddCommand(newReportCashFlowCmd(flags))
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
	Type    string     `json:"type"`
	Group   string     `json:"group"`
}

type qbMetaData struct {
	Name  string `json:"Name"`
	Value string `json:"Value"`
}

type qbColumn struct {
	ColTitle string       `json:"ColTitle"`
	ColType  string       `json:"ColType"`
	MetaData []qbMetaData `json:"MetaData"`
}

type qbReport struct {
	Header struct {
		StartPeriod        string `json:"StartPeriod"`
		EndPeriod          string `json:"EndPeriod"`
		ReportBasis        string `json:"ReportBasis"`
		Currency           string `json:"Currency"`
		ReportName         string `json:"ReportName"`
		SummarizeColumnsBy string `json:"SummarizeColumnsBy"`
	} `json:"Header"`
	Columns struct {
		Column []qbColumn `json:"Column"`
	} `json:"Columns"`
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

// --- columns, row tree and the shared fetch ---

// reportColumn is one amount column of a report, in order: a month, a
// quarter, or the total. Key and dates come from the column's MetaData
// (ColKey, StartDate, EndDate) and Title from its ColTitle.
type reportColumn struct {
	Key       string `json:"key"`
	Title     string `json:"title"`
	StartDate string `json:"start_date,omitempty"`
	EndDate   string `json:"end_date,omitempty"`
}

// reportRow is one line of a report as QuickBooks lays it out. Kind is
// "header" (a group's title row), "account" (a line inside a group, usually
// an account) or "total" (a group's total, or a summary line such as Net
// Income). Depth is the indent QuickBooks shows: a group's total sits at its
// header's depth. Parent is the ID of the header of the group the row
// belongs to (a total's parent is the header it closes), or null at the top
// level. Amounts holds one value per entry in the report's columns (null
// where the cell is blank); Amount is the last of them.
type reportRow struct {
	ID        int        `json:"id"`
	Kind      string     `json:"kind"`
	Depth     int        `json:"depth"`
	Parent    *int       `json:"parent"`
	Name      string     `json:"name"`
	Group     string     `json:"group,omitempty"`
	AccountID string     `json:"account_id,omitempty"`
	Amount    *float64   `json:"amount"`
	Amounts   []*float64 `json:"amounts"`
}

const (
	rowKindHeader  = "header"
	rowKindAccount = "account"
	rowKindTotal   = "total"
)

// summarizeColumnBy maps --columns to the Reports API's summarize_column_by.
// An empty value sends nothing, so QuickBooks' default (one total column)
// applies, as it did before the flag existed.
func summarizeColumnBy(v string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "":
		return "", nil
	case "total":
		return "Total", nil
	case "month":
		return "Month", nil
	case "quarter":
		return "Quarter", nil
	}
	return "", usageErr(fmt.Errorf("invalid --columns %q: expected total, month or quarter", v))
}

func metaValue(md []qbMetaData, name string) string {
	for _, m := range md {
		if strings.EqualFold(m.Name, name) {
			return strings.TrimSpace(m.Value)
		}
	}
	return ""
}

// reportColumns returns the amount columns: every column after the first,
// which holds the row labels. An answer without Columns gets one column per
// amount cell, named "total" when there is only one.
func reportColumns(rep qbReport) []reportColumn {
	cols := rep.Columns.Column
	if len(cols) > 1 {
		out := make([]reportColumn, 0, len(cols)-1)
		for i, c := range cols[1:] {
			key := metaValue(c.MetaData, "ColKey")
			title := strings.TrimSpace(c.ColTitle)
			if key == "" {
				key = strings.ToLower(title)
			}
			if key == "" {
				key = fmt.Sprintf("col%d", i+1)
			}
			if title == "" {
				title = key
			}
			out = append(out, reportColumn{
				Key:       key,
				Title:     title,
				StartDate: metaValue(c.MetaData, "StartDate"),
				EndDate:   metaValue(c.MetaData, "EndDate"),
			})
		}
		return out
	}
	n := maxAmountCells(rep.Rows)
	if n <= 1 {
		return []reportColumn{{Key: "total", Title: "Total"}}
	}
	out := make([]reportColumn, n)
	for i := range out {
		key := fmt.Sprintf("col%d", i+1)
		out[i] = reportColumn{Key: key, Title: key}
	}
	return out
}

func maxAmountCells(rows qbRows) int {
	most := 0
	cells := func(cd []qbCol) {
		if len(cd)-1 > most {
			most = len(cd) - 1
		}
	}
	for _, r := range rows.Row {
		cells(r.ColData)
		if r.Header != nil {
			cells(r.Header.ColData)
		}
		if r.Summary != nil {
			cells(r.Summary.ColData)
		}
		if r.Rows != nil {
			if n := maxAmountCells(*r.Rows); n > most {
				most = n
			}
		}
	}
	return most
}

// flattenReport walks the report tree in the order QuickBooks shows it and
// returns every header, line and total. A section with a Header opens a
// group one level deeper; a section without one (a summary line such as
// Gross Profit, or a bare total) stays at the level it appears in.
func flattenReport(rows qbRows, ncols int) []reportRow {
	var out []reportRow
	add := func(kind string, depth int, parent *int, group string, cd []qbCol) int {
		id := len(out)
		row := reportRow{ID: id, Kind: kind, Depth: depth, Parent: parent, Group: strings.TrimSpace(group)}
		if len(cd) > 0 {
			row.Name = strings.TrimSpace(cd[0].Value)
			row.AccountID = strings.TrimSpace(cd[0].ID)
		}
		row.Amounts = make([]*float64, 0, ncols)
		for i := 1; i < len(cd); i++ {
			var v *float64
			if f, ok := parseReportAmount(cd[i].Value); ok {
				v = &f
			}
			row.Amounts = append(row.Amounts, v)
		}
		for len(row.Amounts) < ncols {
			row.Amounts = append(row.Amounts, nil)
		}
		if len(row.Amounts) > 0 {
			row.Amount = row.Amounts[len(row.Amounts)-1]
		}
		out = append(out, row)
		return id
	}
	var walk func(rows qbRows, parent *int, depth int)
	walk = func(rows qbRows, parent *int, depth int) {
		for _, r := range rows.Row {
			if r.Header != nil {
				header := add(rowKindHeader, depth, parent, r.Group, r.Header.ColData)
				if r.Rows != nil {
					walk(*r.Rows, &header, depth+1)
				}
				if r.Summary != nil {
					add(rowKindTotal, depth, &header, r.Group, r.Summary.ColData)
				}
				continue
			}
			if len(r.ColData) > 0 {
				add(rowKindAccount, depth, parent, r.Group, r.ColData)
			}
			if r.Rows != nil {
				walk(*r.Rows, parent, depth)
			}
			if r.Summary != nil {
				add(rowKindTotal, depth, parent, r.Group, r.Summary.ColData)
			}
		}
	}
	walk(rows, nil, 0)
	return out
}

// reportFlags are the flags every report command shares.
type reportFlags struct {
	columns string
	raw     bool
}

func (rf *reportFlags) register(cmd *cobra.Command) {
	cmd.Flags().StringVar(&rf.columns, "columns", "", "Columns: total, month or quarter (default: one total column)")
	cmd.Flags().BoolVar(&rf.raw, "raw", false, "Print QuickBooks' report JSON (Header, Columns, Rows) as returned")
}

// fetchReport calls one Reports API endpoint. With --raw it prints the answer
// as returned (through the same client and output path as every other
// command) and returns handled=true.
func fetchReport(cmd *cobra.Command, flags *rootFlags, rf *reportFlags, path string, params map[string]string) (qbReport, bool, error) {
	var rep qbReport
	c, err := flags.newClient()
	if err != nil {
		return rep, false, err
	}
	data, err := c.Get(cmd.Context(), path, params)
	if err != nil {
		return rep, false, classifyAPIError(cmd.OutOrStdout(), err, flags)
	}
	if rf.raw {
		return rep, true, flags.printJSON(cmd, json.RawMessage(data))
	}
	if err := json.Unmarshal(data, &rep); err != nil {
		return rep, false, apiErr(fmt.Errorf("parsing %s report: %w", strings.TrimPrefix(path, "/reports/"), err))
	}
	return rep, false, nil
}

// reportParams builds the shared query. summarize_column_by is sent only when
// --columns was given.
func reportParams(start, end string, cash bool, columnsBy string) map[string]string {
	method := "Accrual"
	if cash {
		method = "Cash"
	}
	params := map[string]string{
		"start_date":        start,
		"end_date":          end,
		"accounting_method": method,
		"minorversion":      "75",
	}
	if columnsBy != "" {
		params["summarize_column_by"] = columnsBy
	}
	return params
}

func reportBasis(rep qbReport, cash bool) string {
	if b := strings.TrimSpace(rep.Header.ReportBasis); b != "" {
		return b
	}
	if cash {
		return "Cash"
	}
	return "Accrual"
}

// periodBounds resolves --start/--end, falling back to --month (default:
// the last closed month) for whichever is unset.
func periodBounds(month, start, end string) (string, string, error) {
	if start != "" && end != "" {
		return start, end, nil
	}
	s, e, err := monthBounds(month)
	if err != nil {
		return "", "", err
	}
	if start == "" {
		start = s
	}
	if end == "" {
		end = e
	}
	return start, end, nil
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
	Report    string         `json:"report"`
	StartDate string         `json:"start_date"`
	EndDate   string         `json:"end_date"`
	Basis     string         `json:"basis"`
	Currency  string         `json:"currency"`
	ColumnsBy string         `json:"columns_by,omitempty"`
	Metrics   pnlMetrics     `json:"metrics"`
	Accounts  []reportLeaf   `json:"accounts"`
	Summaries []reportLeaf   `json:"summaries"`
	Columns   []reportColumn `json:"columns"`
	Rows      []reportRow    `json:"rows"`
}

func newReportPnlCmd(flags *rootFlags) *cobra.Command {
	var month, start, end string
	var cash bool
	var rf reportFlags
	cmd := &cobra.Command{
		Use:   "pnl",
		Short: "Profit & Loss: recognized revenue, COGS, gross/operating margin, net income",
		Long: "Pull the QBO Profit & Loss report for a period (default: last closed month).\n" +
			"Returns recognized revenue by account, COGS, gross profit/margin, operating\n" +
			"income/margin, and net income: the closed-books numbers, accrual basis.\n" +
			"rows[] keeps the full layout (kind, depth, parent, per-column amounts);\n" +
			"--columns month|quarter adds a column per period; --raw prints QuickBooks' JSON.",
		Example: "  quickbooks-cli report pnl --month 2026-05 --json\n" +
			"  quickbooks-cli report pnl --start 2026-01-01 --end 2026-03-31 --columns month --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			columnsBy, err := summarizeColumnBy(rf.columns)
			if err != nil {
				return err
			}
			if dryRunOK(flags) {
				return nil
			}
			start, end, err := periodBounds(month, start, end)
			if err != nil {
				return err
			}
			rep, handled, err := fetchReport(cmd, flags, &rf, "/reports/ProfitAndLoss", reportParams(start, end, cash, columnsBy))
			if err != nil || handled {
				return err
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

			columns := reportColumns(rep)
			view := pnlView{
				Report:    "ProfitAndLoss",
				StartDate: rep.Header.StartPeriod,
				EndDate:   rep.Header.EndPeriod,
				Basis:     reportBasis(rep, cash),
				Currency:  rep.Header.Currency,
				ColumnsBy: rep.Header.SummarizeColumnsBy,
				Metrics:   m,
				Accounts:  leaves,
				Summaries: summaries,
				Columns:   columns,
				Rows:      flattenReport(rep.Rows, len(columns)),
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
	rf.register(cmd)
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
	StartDate  string           `json:"start_date,omitempty"`
	Basis      string           `json:"basis"`
	Currency   string           `json:"currency"`
	ColumnsBy  string           `json:"columns_by,omitempty"`
	Highlights []highlightMatch `json:"highlights,omitempty"`
	Accounts   []reportLeaf     `json:"accounts"`
	Summaries  []reportLeaf     `json:"summaries"`
	Columns    []reportColumn   `json:"columns"`
	Rows       []reportRow      `json:"rows"`
}

func newReportBalanceSheetCmd(flags *rootFlags) *cobra.Command {
	var asOf, start string
	var cash bool
	var highlight []string
	var rf reportFlags
	cmd := &cobra.Command{
		Use:   "balance-sheet",
		Short: "Balance sheet: every account + group balance, with optional --highlight totals",
		Long: "Pull the QBO Balance Sheet as of a date (default: today). Returns every\n" +
			"account leaf and summary total. Pass --highlight to total and surface any\n" +
			"account or group by name substring (repeatable), e.g. \"deferred revenue\"\n" +
			"or \"accrued interest\". rows[] keeps the full layout; --start with\n" +
			"--columns month|quarter returns a balance per period end from --start to\n" +
			"--as-of; --raw prints QuickBooks' JSON. Read-only.",
		Example: "  quickbooks-cli report balance-sheet --json\n" +
			"  quickbooks-cli report balance-sheet --highlight \"deferred revenue\" --highlight \"accrued interest\" --agent\n" +
			"  quickbooks-cli report balance-sheet --start 2026-01-01 --as-of 2026-03-31 --columns month --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			columnsBy, err := summarizeColumnBy(rf.columns)
			if err != nil {
				return err
			}
			if dryRunOK(flags) {
				return nil
			}
			asOf := asOf
			if asOf == "" {
				// The caller's calendar date, not UTC's: QBO report dates
				// are calendar dates (see analytics' calendar-day basis).
				asOf = time.Now().Format("2006-01-02")
			}
			from := start
			if from == "" {
				from = asOf
			}
			rep, handled, err := fetchReport(cmd, flags, &rf, "/reports/BalanceSheet", reportParams(from, asOf, cash, columnsBy))
			if err != nil || handled {
				return err
			}
			var leaves, summaries []reportLeaf
			walkReport(rep.Rows, "", &leaves, &summaries)

			columns := reportColumns(rep)
			view := bsView{
				Report:     "BalanceSheet",
				AsOf:       rep.Header.EndPeriod,
				Basis:      reportBasis(rep, cash),
				Currency:   rep.Header.Currency,
				ColumnsBy:  rep.Header.SummarizeColumnsBy,
				Highlights: matchHighlights(highlight, leaves, summaries),
				Accounts:   leaves,
				Summaries:  summaries,
				Columns:    columns,
				Rows:       flattenReport(rep.Rows, len(columns)),
			}
			if view.AsOf == "" {
				view.AsOf = asOf
			}
			if start != "" {
				view.StartDate = firstNonEmpty(rep.Header.StartPeriod, start)
			}
			return flags.printJSON(cmd, view)
		},
	}
	cmd.Flags().StringVar(&asOf, "as-of", "", "Balance-sheet date YYYY-MM-DD (default: today)")
	cmd.Flags().StringVar(&start, "start", "", "First period start YYYY-MM-DD for --columns month|quarter (default: --as-of)")
	cmd.Flags().BoolVar(&cash, "cash", false, "Cash basis (default: accrual)")
	cmd.Flags().StringArrayVar(&highlight, "highlight", nil, "Account or group name substring to total and highlight (repeatable, case-insensitive)")
	rf.register(cmd)
	return cmd
}

// --- report cash-flow ---

// cashFlowMetrics are the statement of cash flows' headline figures for one
// column. A figure is null when the report has no such line (for example no
// investing activity in the period, or no beginning-cash line in the answer):
// the report did not say it was zero.
type cashFlowMetrics struct {
	OperatingActivities *float64 `json:"operating_activities"`
	InvestingActivities *float64 `json:"investing_activities"`
	FinancingActivities *float64 `json:"financing_activities"`
	NetCashChange       *float64 `json:"net_cash_change"`
	BeginningCash       *float64 `json:"beginning_cash"`
	EndingCash          *float64 `json:"ending_cash"`
}

type cashFlowColumnMetrics struct {
	Column  string          `json:"column"`
	Title   string          `json:"title"`
	Metrics cashFlowMetrics `json:"metrics"`
}

type cfView struct {
	Report          string                  `json:"report"`
	StartDate       string                  `json:"start_date"`
	EndDate         string                  `json:"end_date"`
	Basis           string                  `json:"basis"`
	Currency        string                  `json:"currency"`
	ColumnsBy       string                  `json:"columns_by,omitempty"`
	Metrics         cashFlowMetrics         `json:"metrics"`
	MetricsByColumn []cashFlowColumnMetrics `json:"metrics_by_column"`
	Accounts        []reportLeaf            `json:"accounts"`
	Summaries       []reportLeaf            `json:"summaries"`
	Columns         []reportColumn          `json:"columns"`
	Rows            []reportRow             `json:"rows"`
}

// cashFlowLine names one headline line: the QuickBooks group that marks it,
// and the label to fall back on when an answer carries no group.
type cashFlowLine struct {
	group string
	label func(string) bool
}

var (
	cfOperating = cashFlowLine{"OperatingActivities", contains("operating activities")}
	cfInvesting = cashFlowLine{"InvestingActivities", contains("investing activities")}
	cfFinancing = cashFlowLine{"FinancingActivities", contains("financing activities")}
	cfNetChange = cashFlowLine{"CashIncrease", func(s string) bool {
		return strings.Contains(s, "net cash increase") || strings.Contains(s, "net cash decrease") || strings.Contains(s, "net change in cash")
	}}
	cfBeginning = cashFlowLine{"BeginningCash", contains("cash at beginning of period")}
	cfEnding    = cashFlowLine{"EndingCash", contains("cash at end of period")}
)

// find returns the row for this line: a total (or a line, for a group
// QuickBooks emits as a single data row) carrying the group, else the first
// total whose label matches.
func (l cashFlowLine) find(rows []reportRow) *reportRow {
	for i := range rows {
		if strings.EqualFold(rows[i].Group, l.group) && rows[i].Kind == rowKindTotal {
			return &rows[i]
		}
	}
	for i := range rows {
		if strings.EqualFold(rows[i].Group, l.group) && rows[i].Kind == rowKindAccount {
			return &rows[i]
		}
	}
	for i := range rows {
		if rows[i].Kind == rowKindTotal && rows[i].Group == "" && l.label(strings.ToLower(rows[i].Name)) {
			return &rows[i]
		}
	}
	return nil
}

func cashFlowMetricsAt(rows []reportRow, col int) cashFlowMetrics {
	at := func(l cashFlowLine) *float64 {
		r := l.find(rows)
		if r == nil || col < 0 || col >= len(r.Amounts) {
			return nil
		}
		return r.Amounts[col]
	}
	return cashFlowMetrics{
		OperatingActivities: at(cfOperating),
		InvestingActivities: at(cfInvesting),
		FinancingActivities: at(cfFinancing),
		NetCashChange:       at(cfNetChange),
		BeginningCash:       at(cfBeginning),
		EndingCash:          at(cfEnding),
	}
}

func newReportCashFlowCmd(flags *rootFlags) *cobra.Command {
	var month, start, end string
	var cash bool
	var rf reportFlags
	cmd := &cobra.Command{
		Use:   "cash-flow",
		Short: "Statement of cash flows: operating, investing, financing, net change, beginning and ending cash",
		Long: "Pull the QBO Statement of Cash Flows for a period (default: last closed month),\n" +
			"accrual basis unless --cash. metrics holds net cash from operating, investing and\n" +
			"financing activities, the net cash change, and cash at the beginning and end of\n" +
			"the period (null when the report has no such line); metrics_by_column repeats them\n" +
			"for each column. rows[] keeps every line of each section; --columns month|quarter\n" +
			"adds a column per period; --raw prints QuickBooks' JSON. Read-only.",
		Example: "  quickbooks-cli report cash-flow --month 2026-05 --json\n" +
			"  quickbooks-cli report cash-flow --start 2026-01-01 --end 2026-03-31 --columns month --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			columnsBy, err := summarizeColumnBy(rf.columns)
			if err != nil {
				return err
			}
			if dryRunOK(flags) {
				return nil
			}
			start, end, err := periodBounds(month, start, end)
			if err != nil {
				return err
			}
			rep, handled, err := fetchReport(cmd, flags, &rf, "/reports/CashFlow", reportParams(start, end, cash, columnsBy))
			if err != nil || handled {
				return err
			}
			var leaves, summaries []reportLeaf
			walkReport(rep.Rows, "", &leaves, &summaries)
			columns := reportColumns(rep)
			rows := flattenReport(rep.Rows, len(columns))
			byColumn := make([]cashFlowColumnMetrics, 0, len(columns))
			for i, c := range columns {
				byColumn = append(byColumn, cashFlowColumnMetrics{Column: c.Key, Title: c.Title, Metrics: cashFlowMetricsAt(rows, i)})
			}
			view := cfView{
				Report:          "CashFlow",
				StartDate:       firstNonEmpty(rep.Header.StartPeriod, start),
				EndDate:         firstNonEmpty(rep.Header.EndPeriod, end),
				Basis:           reportBasis(rep, cash),
				Currency:        rep.Header.Currency,
				ColumnsBy:       rep.Header.SummarizeColumnsBy,
				Metrics:         cashFlowMetricsAt(rows, len(columns)-1),
				MetricsByColumn: byColumn,
				Accounts:        leaves,
				Summaries:       summaries,
				Columns:         columns,
				Rows:            rows,
			}
			return flags.printJSON(cmd, view)
		},
	}
	cmd.Flags().StringVar(&month, "month", "", "Period as YYYY-MM (default: last closed month)")
	cmd.Flags().StringVar(&start, "start", "", "Start date YYYY-MM-DD (overrides --month)")
	cmd.Flags().StringVar(&end, "end", "", "End date YYYY-MM-DD (overrides --month)")
	cmd.Flags().BoolVar(&cash, "cash", false, "Cash basis (default: accrual)")
	rf.register(cmd)
	return cmd
}
