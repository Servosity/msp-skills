// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"

	"quickbooks-pp-cli/internal/cliutil/testenv"
)

// Every company, account name, amount and date below is invented for these
// tests (a made-up "Larkspur Widget Co"). The shapes follow Intuit's public
// Reports API documentation: Columns.Column carries ColTitle, ColType and
// MetaData (ColKey, StartDate, EndDate); a Section row carries Header, Rows,
// Summary and a group; a Data row carries ColData.

const larkspurCashFlow = `{
  "Header": {"ReportName":"CashFlow","StartPeriod":"2025-01-01","EndPeriod":"2025-02-28","SummarizeColumnsBy":"Month","Currency":"USD"},
  "Columns": {"Column": [
    {"ColTitle":"","ColType":"Account","MetaData":[{"Name":"ColKey","Value":"account"}]},
    {"ColTitle":"Jan 2025","ColType":"Money","MetaData":[{"Name":"StartDate","Value":"2025-01-01"},{"Name":"EndDate","Value":"2025-01-31"},{"Name":"ColKey","Value":"Jan 2025"}]},
    {"ColTitle":"Feb 2025","ColType":"Money","MetaData":[{"Name":"StartDate","Value":"2025-02-01"},{"Name":"EndDate","Value":"2025-02-28"},{"Name":"ColKey","Value":"Feb 2025"}]},
    {"ColTitle":"Total","ColType":"Money","MetaData":[{"Name":"ColKey","Value":"total"}]}
  ]},
  "Rows": {"Row": [
    {"type":"Section","group":"OperatingActivities",
     "Header":{"ColData":[{"value":"OPERATING ACTIVITIES"},{"value":""},{"value":""},{"value":""}]},
     "Rows":{"Row":[
       {"type":"Data","group":"NetIncome","ColData":[{"value":"Net Income"},{"value":"110.00"},{"value":"90.00"},{"value":"200.00"}]},
       {"type":"Section","group":"OperatingAdjustments",
        "Header":{"ColData":[{"value":"Adjustments to reconcile Net Income"},{"value":""},{"value":""},{"value":""}]},
        "Rows":{"Row":[
          {"type":"Data","ColData":[{"value":"Widget Receivables","id":"71"},{"value":"-20.00"},{"value":"5.00"},{"value":"-15.00"}]}
        ]},
        "Summary":{"ColData":[{"value":"Total Adjustments to reconcile Net Income"},{"value":"-20.00"},{"value":"5.00"},{"value":"-15.00"}]}}
     ]},
     "Summary":{"ColData":[{"value":"Net cash provided by operating activities"},{"value":"90.00"},{"value":"95.00"},{"value":"185.00"}]}},
    {"type":"Section","group":"InvestingActivities",
     "Header":{"ColData":[{"value":"INVESTING ACTIVITIES"},{"value":""},{"value":""},{"value":""}]},
     "Rows":{"Row":[
       {"type":"Data","ColData":[{"value":"Bench Lathe","id":"72"},{"value":"-40.00"},{"value":""},{"value":"-40.00"}]}
     ]},
     "Summary":{"ColData":[{"value":"Net cash provided by investing activities"},{"value":"-40.00"},{"value":"0.00"},{"value":"-40.00"}]}},
    {"type":"Section","group":"FinancingActivities",
     "Header":{"ColData":[{"value":"FINANCING ACTIVITIES"},{"value":""},{"value":""},{"value":""}]},
     "Rows":{"Row":[
       {"type":"Data","ColData":[{"value":"Owner Draw","id":"73"},{"value":"-10.00"},{"value":"-10.00"},{"value":"-20.00"}]}
     ]},
     "Summary":{"ColData":[{"value":"Net cash provided by financing activities"},{"value":"-10.00"},{"value":"-10.00"},{"value":"-20.00"}]}},
    {"type":"Section","group":"CashIncrease","Summary":{"ColData":[{"value":"Net cash increase for period"},{"value":"40.00"},{"value":"85.00"},{"value":"125.00"}]}},
    {"type":"Section","group":"BeginningCash","Summary":{"ColData":[{"value":"Cash at beginning of period"},{"value":"500.00"},{"value":"540.00"},{"value":"500.00"}]}},
    {"type":"Section","group":"EndingCash","Summary":{"ColData":[{"value":"Cash at end of period"},{"value":"540.00"},{"value":"625.00"},{"value":"625.00"}]}}
  ]}
}`

// larkspurPnl nests a group two levels deep (Payroll inside Expenses) and
// carries summary-only sections (Gross Profit, Net Income) the way the
// Reports API does.
const larkspurPnl = `{
  "Header": {"ReportName":"ProfitAndLoss","StartPeriod":"2025-01-01","EndPeriod":"2025-03-31","ReportBasis":"Accrual","SummarizeColumnsBy":"Quarter","Currency":"USD"},
  "Columns": {"Column": [
    {"ColTitle":"","ColType":"Account","MetaData":[{"Name":"ColKey","Value":"account"}]},
    {"ColTitle":"Q1 2025","ColType":"Money","MetaData":[{"Name":"StartDate","Value":"2025-01-01"},{"Name":"EndDate","Value":"2025-03-31"},{"Name":"ColKey","Value":"Q1 2025"}]},
    {"ColTitle":"Total","ColType":"Money","MetaData":[{"Name":"ColKey","Value":"total"}]}
  ]},
  "Rows": {"Row": [
    {"type":"Section","group":"Income",
     "Header":{"ColData":[{"value":"Income"},{"value":""},{"value":""}]},
     "Rows":{"Row":[
       {"type":"Data","ColData":[{"value":"Widget Sales","id":"11"},{"value":"1,000.00"},{"value":"1,000.00"}]},
       {"type":"Data","ColData":[{"value":"Repair Fees","id":"12"},{"value":"250.00"},{"value":"250.00"}]}
     ]},
     "Summary":{"ColData":[{"value":"Total Income"},{"value":"1,250.00"},{"value":"1,250.00"}]}},
    {"type":"Section","group":"GrossProfit","Summary":{"ColData":[{"value":"Gross Profit"},{"value":"1,250.00"},{"value":"1,250.00"}]}},
    {"type":"Section","group":"Expenses",
     "Header":{"ColData":[{"value":"Expenses"},{"value":""},{"value":""}]},
     "Rows":{"Row":[
       {"type":"Data","ColData":[{"value":"Shop Rent","id":"21"},{"value":"300.00"},{"value":"300.00"}]},
       {"type":"Section",
        "Header":{"ColData":[{"value":"Payroll","id":"22"},{"value":"(5.00)"},{"value":"(5.00)"}]},
        "Rows":{"Row":[
          {"type":"Data","ColData":[{"value":"Wages","id":"23"},{"value":"400.00"},{"value":"400.00"}]}
        ]},
        "Summary":{"ColData":[{"value":"Total Payroll"},{"value":"395.00"},{"value":"395.00"}]}}
     ]},
     "Summary":{"ColData":[{"value":"Total Expenses"},{"value":"695.00"},{"value":"695.00"}]}},
    {"type":"Section","group":"NetOperatingIncome","Summary":{"ColData":[{"value":"Net Operating Income"},{"value":"555.00"},{"value":"555.00"}]}},
    {"type":"Section","group":"NetIncome","Summary":{"ColData":[{"value":"Net Income"},{"value":"555.00"},{"value":"555.00"}]}}
  ]}
}`

const larkspurBalanceSheet = `{
  "Header": {"ReportName":"BalanceSheet","StartPeriod":"2025-01-01","EndPeriod":"2025-02-28","ReportBasis":"Accrual","SummarizeColumnsBy":"Month","Currency":"USD"},
  "Columns": {"Column": [
    {"ColTitle":"","ColType":"Account","MetaData":[{"Name":"ColKey","Value":"account"}]},
    {"ColTitle":"Jan 2025","ColType":"Money","MetaData":[{"Name":"EndDate","Value":"2025-01-31"},{"Name":"ColKey","Value":"Jan 2025"}]},
    {"ColTitle":"Feb 2025","ColType":"Money","MetaData":[{"Name":"EndDate","Value":"2025-02-28"},{"Name":"ColKey","Value":"Feb 2025"}]}
  ]},
  "Rows": {"Row": [
    {"type":"Section","group":"TotalAssets",
     "Header":{"ColData":[{"value":"ASSETS"},{"value":""},{"value":""}]},
     "Rows":{"Row":[
       {"type":"Data","ColData":[{"value":"Till Cash","id":"31"},{"value":"540.00"},{"value":"625.00"}]}
     ]},
     "Summary":{"ColData":[{"value":"TOTAL ASSETS"},{"value":"540.00"},{"value":"625.00"}]}}
  ]}
}`

// reportServer serves body for every request and records each query.
type reportServer struct {
	mu      sync.Mutex
	paths   []string
	queries []url.Values
	srv     *httptest.Server
}

func newReportServer(t *testing.T, body string) *reportServer {
	t.Helper()
	rs := &reportServer{}
	rs.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rs.mu.Lock()
		rs.paths = append(rs.paths, r.URL.Path)
		rs.queries = append(rs.queries, r.URL.Query())
		rs.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(rs.srv.Close)
	return rs
}

func (rs *reportServer) requests() ([]string, []url.Values) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return append([]string(nil), rs.paths...), append([]url.Values(nil), rs.queries...)
}

// runReport runs the CLI against srv with a throwaway token and returns
// stdout and the command error.
func runReport(t *testing.T, srv *reportServer, args ...string) (string, error) {
	t.Helper()
	testenv.Isolate(t)
	t.Setenv("QUICKBOOKS_BASE_URL", srv.srv.URL)
	t.Setenv("QUICKBOOKS_ACCESS_TOKEN", "larkspur-fixture-token")
	var out bytes.Buffer
	root := newRootCmd(&rootFlags{})
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func f64(v float64) *float64 { return &v }

func TestReportCashFlowReturnsSectionsAndCashPosition(t *testing.T) {
	srv := newReportServer(t, larkspurCashFlow)
	out, err := runReport(t, srv, "report", "cash-flow", "--start", "2025-01-01", "--end", "2025-02-28", "--columns", "month", "--json", "--no-cache")
	if err != nil {
		t.Fatalf("report cash-flow: %v", err)
	}
	paths, queries := srv.requests()
	if len(paths) != 1 || !strings.HasSuffix(paths[0], "/reports/CashFlow") {
		t.Fatalf("requested %v, want one GET of /reports/CashFlow", paths)
	}
	q := queries[0]
	for k, want := range map[string]string{"start_date": "2025-01-01", "end_date": "2025-02-28", "accounting_method": "Accrual", "summarize_column_by": "Month", "minorversion": "75"} {
		if got := q.Get(k); got != want {
			t.Errorf("query %s = %q, want %q", k, got, want)
		}
	}
	var view struct {
		Report    string `json:"report"`
		StartDate string `json:"start_date"`
		EndDate   string `json:"end_date"`
		Basis     string `json:"basis"`
		ColumnsBy string `json:"columns_by"`
		Columns   []struct {
			Key, Title, StartDate, EndDate string
		} `json:"columns"`
		Metrics         map[string]*float64 `json:"metrics"`
		MetricsByColumn []struct {
			Column  string              `json:"column"`
			Title   string              `json:"title"`
			Metrics map[string]*float64 `json:"metrics"`
		} `json:"metrics_by_column"`
		Rows []reportRow `json:"rows"`
	}
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out)
	}
	if view.Report != "CashFlow" || view.StartDate != "2025-01-01" || view.EndDate != "2025-02-28" || view.Basis != "Accrual" || view.ColumnsBy != "Month" {
		t.Fatalf("header fields = %+v", view)
	}
	wantTotal := map[string]float64{
		"operating_activities": 185, "investing_activities": -40, "financing_activities": -20,
		"net_cash_change": 125, "beginning_cash": 500, "ending_cash": 625,
	}
	for k, want := range wantTotal {
		got := view.Metrics[k]
		if got == nil || *got != want {
			t.Errorf("metrics.%s = %v, want %v", k, got, want)
		}
	}
	if len(view.MetricsByColumn) != 3 {
		t.Fatalf("metrics_by_column has %d entries, want 3 (Jan, Feb, Total)", len(view.MetricsByColumn))
	}
	feb := view.MetricsByColumn[1]
	if feb.Column != "Feb 2025" || feb.Title != "Feb 2025" {
		t.Errorf("second column = %q/%q, want Feb 2025", feb.Column, feb.Title)
	}
	for k, want := range map[string]float64{"operating_activities": 95, "investing_activities": 0, "financing_activities": -10, "net_cash_change": 85, "beginning_cash": 540, "ending_cash": 625} {
		if got := feb.Metrics[k]; got == nil || *got != want {
			t.Errorf("Feb metrics.%s = %v, want %v", k, got, want)
		}
	}
	if len(view.Columns) != 3 || view.Columns[0].Title != "Jan 2025" || view.Columns[2].Key != "total" {
		t.Errorf("columns = %+v", view.Columns)
	}
	if len(view.Rows) != 15 {
		t.Errorf("rows = %d, want 15 (every header, line and total the report carries)", len(view.Rows))
	}
}

// A section QuickBooks does not return is null, not zero: the report did not
// say it was zero.
func TestReportCashFlowMissingSectionIsNull(t *testing.T) {
	body := strings.Replace(larkspurCashFlow, `"group":"InvestingActivities"`, `"group":"SomethingElse"`, 1)
	body = strings.Replace(body, "investing activities", "other activities", 1)
	srv := newReportServer(t, body)
	out, err := runReport(t, srv, "report", "cash-flow", "--start", "2025-01-01", "--end", "2025-02-28", "--json", "--no-cache")
	if err != nil {
		t.Fatalf("report cash-flow: %v", err)
	}
	var view struct {
		Metrics map[string]*float64 `json:"metrics"`
	}
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if v, ok := view.Metrics["investing_activities"]; !ok || v != nil {
		t.Fatalf("investing_activities = %v (present %v), want null", v, ok)
	}
}

func TestReportCashFlowMonthAndCashFlags(t *testing.T) {
	srv := newReportServer(t, larkspurCashFlow)
	if _, err := runReport(t, srv, "report", "cash-flow", "--month", "2025-02", "--cash", "--json", "--no-cache"); err != nil {
		t.Fatalf("report cash-flow: %v", err)
	}
	_, queries := srv.requests()
	q := queries[0]
	if q.Get("start_date") != "2025-02-01" || q.Get("end_date") != "2025-02-28" || q.Get("accounting_method") != "Cash" {
		t.Fatalf("query = %v, want February on the cash basis", q)
	}
	if _, set := q["summarize_column_by"]; set {
		t.Fatalf("summarize_column_by sent without --columns: %v", q)
	}
}

func TestReportColumnsFlagMapsToSummarizeColumnBy(t *testing.T) {
	cases := map[string]struct {
		body string
		args []string
	}{
		"pnl":           {larkspurPnl, []string{"report", "pnl", "--start", "2025-01-01", "--end", "2025-03-31"}},
		"balance-sheet": {larkspurBalanceSheet, []string{"report", "balance-sheet", "--start", "2025-01-01", "--as-of", "2025-02-28"}},
		"cash-flow":     {larkspurCashFlow, []string{"report", "cash-flow", "--start", "2025-01-01", "--end", "2025-02-28"}},
	}
	for name, tc := range cases {
		for flag, want := range map[string]string{"total": "Total", "month": "Month", "quarter": "Quarter", "MONTH": "Month"} {
			t.Run(name+"/"+flag, func(t *testing.T) {
				srv := newReportServer(t, tc.body)
				args := append(append([]string(nil), tc.args...), "--columns", flag, "--json", "--no-cache")
				if _, err := runReport(t, srv, args...); err != nil {
					t.Fatalf("%v: %v", args, err)
				}
				_, queries := srv.requests()
				if len(queries) != 1 || queries[0].Get("summarize_column_by") != want {
					t.Fatalf("summarize_column_by = %v, want %q", queries, want)
				}
			})
		}
		t.Run(name+"/invalid", func(t *testing.T) {
			srv := newReportServer(t, tc.body)
			args := append(append([]string(nil), tc.args...), "--columns", "week", "--json", "--no-cache")
			_, err := runReport(t, srv, args...)
			if err == nil || ExitCode(err) != 2 {
				t.Fatalf("--columns week: err %v (exit %d), want a usage error", err, ExitCode(err))
			}
			if paths, _ := srv.requests(); len(paths) != 0 {
				t.Fatalf("an invalid --columns still called the API: %v", paths)
			}
		})
	}
}

func TestReportBalanceSheetStartSpansColumns(t *testing.T) {
	srv := newReportServer(t, larkspurBalanceSheet)
	out, err := runReport(t, srv, "report", "balance-sheet", "--start", "2025-01-01", "--as-of", "2025-02-28", "--columns", "month", "--json", "--no-cache")
	if err != nil {
		t.Fatalf("report balance-sheet: %v", err)
	}
	_, queries := srv.requests()
	if q := queries[0]; q.Get("start_date") != "2025-01-01" || q.Get("end_date") != "2025-02-28" {
		t.Fatalf("query = %v, want start 2025-01-01 and end 2025-02-28", q)
	}
	var view struct {
		AsOf     string       `json:"as_of"`
		Accounts []reportLeaf `json:"accounts"`
		Columns  []struct{ Key, Title string }
		Rows     []reportRow `json:"rows"`
	}
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if view.AsOf != "2025-02-28" || len(view.Columns) != 2 || view.Columns[1].Title != "Feb 2025" {
		t.Fatalf("view = %+v", view)
	}
	// The existing flat field keeps its meaning: the last column's amount.
	if len(view.Accounts) != 1 || view.Accounts[0].Amount != 625 {
		t.Fatalf("accounts = %+v, want the latest balance", view.Accounts)
	}
	if got := view.Rows[1].Amounts; len(got) != 2 || *got[0] != 540 || *got[1] != 625 {
		t.Fatalf("per-column amounts = %v", got)
	}
}

// TestReportRowTreeKeepsTheLayout pins the row tree on a report with a group
// nested two levels deep and summary-only sections: every header, line and
// total in the order QuickBooks shows them, each with its depth, the header
// of the group it belongs to, and its per-column amounts.
func TestReportRowTreeKeepsTheLayout(t *testing.T) {
	srv := newReportServer(t, larkspurPnl)
	out, err := runReport(t, srv, "report", "pnl", "--start", "2025-01-01", "--end", "2025-03-31", "--columns", "quarter", "--json", "--no-cache")
	if err != nil {
		t.Fatalf("report pnl: %v", err)
	}
	var view struct {
		Columns []reportColumn `json:"columns"`
		Rows    []reportRow    `json:"rows"`
		Metrics pnlMetrics     `json:"metrics"`
	}
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("decode: %v\n%s", err, out)
	}
	wantCols := []reportColumn{
		{Key: "Q1 2025", Title: "Q1 2025", StartDate: "2025-01-01", EndDate: "2025-03-31"},
		{Key: "total", Title: "Total"},
	}
	if !reflect.DeepEqual(view.Columns, wantCols) {
		t.Fatalf("columns = %+v, want %+v", view.Columns, wantCols)
	}
	p := func(i int) *int { return &i }
	amts := func(v ...float64) []*float64 {
		out := make([]*float64, len(v))
		for i := range v {
			out[i] = f64(v[i])
		}
		return out
	}
	blank := []*float64{nil, nil}
	want := []reportRow{
		{ID: 0, Kind: "header", Depth: 0, Parent: nil, Name: "Income", Group: "Income", Amounts: blank},
		{ID: 1, Kind: "account", Depth: 1, Parent: p(0), Name: "Widget Sales", AccountID: "11", Amount: f64(1000), Amounts: amts(1000, 1000)},
		{ID: 2, Kind: "account", Depth: 1, Parent: p(0), Name: "Repair Fees", AccountID: "12", Amount: f64(250), Amounts: amts(250, 250)},
		{ID: 3, Kind: "total", Depth: 0, Parent: p(0), Name: "Total Income", Group: "Income", Amount: f64(1250), Amounts: amts(1250, 1250)},
		{ID: 4, Kind: "total", Depth: 0, Parent: nil, Name: "Gross Profit", Group: "GrossProfit", Amount: f64(1250), Amounts: amts(1250, 1250)},
		{ID: 5, Kind: "header", Depth: 0, Parent: nil, Name: "Expenses", Group: "Expenses", Amounts: blank},
		{ID: 6, Kind: "account", Depth: 1, Parent: p(5), Name: "Shop Rent", AccountID: "21", Amount: f64(300), Amounts: amts(300, 300)},
		{ID: 7, Kind: "header", Depth: 1, Parent: p(5), Name: "Payroll", AccountID: "22", Amount: f64(-5), Amounts: amts(-5, -5)},
		{ID: 8, Kind: "account", Depth: 2, Parent: p(7), Name: "Wages", AccountID: "23", Amount: f64(400), Amounts: amts(400, 400)},
		{ID: 9, Kind: "total", Depth: 1, Parent: p(7), Name: "Total Payroll", Amount: f64(395), Amounts: amts(395, 395)},
		{ID: 10, Kind: "total", Depth: 0, Parent: p(5), Name: "Total Expenses", Group: "Expenses", Amount: f64(695), Amounts: amts(695, 695)},
		{ID: 11, Kind: "total", Depth: 0, Parent: nil, Name: "Net Operating Income", Group: "NetOperatingIncome", Amount: f64(555), Amounts: amts(555, 555)},
		{ID: 12, Kind: "total", Depth: 0, Parent: nil, Name: "Net Income", Group: "NetIncome", Amount: f64(555), Amounts: amts(555, 555)},
	}
	if !reflect.DeepEqual(view.Rows, want) {
		got, _ := json.MarshalIndent(view.Rows, "", " ")
		t.Fatalf("rows differ from the expected tree:\n%s", got)
	}
	// The existing metrics keep working from the total column.
	if view.Metrics.TotalIncome != 1250 || view.Metrics.NetIncome != 555 || view.Metrics.OperatingIncome != 555 {
		t.Fatalf("metrics = %+v", view.Metrics)
	}
}

// A report with no Columns (an older answer, or a test double) still gets one
// total column, and the existing fields are unchanged.
func TestReportWithoutColumnsKeepsExistingFields(t *testing.T) {
	srv := newReportServer(t, pnlFixture)
	out, err := runReport(t, srv, "report", "pnl", "--month", "2026-05", "--json", "--no-cache")
	if err != nil {
		t.Fatalf("report pnl: %v", err)
	}
	var view struct {
		Accounts  []reportLeaf   `json:"accounts"`
		Summaries []reportLeaf   `json:"summaries"`
		Metrics   pnlMetrics     `json:"metrics"`
		Columns   []reportColumn `json:"columns"`
		Rows      []reportRow    `json:"rows"`
	}
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(view.Accounts) != 3 || view.Accounts[0].Section != "Recurring Revenue" || len(view.Summaries) != 6 {
		t.Fatalf("flat fields changed: %+v", view)
	}
	if view.Metrics.NetIncome != 15000 || view.Metrics.GrossProfit != 90000 {
		t.Fatalf("metrics changed: %+v", view.Metrics)
	}
	if !reflect.DeepEqual(view.Columns, []reportColumn{{Key: "total", Title: "Total"}}) {
		t.Fatalf("columns = %+v, want one total column", view.Columns)
	}
	if len(view.Rows) != 11 {
		t.Fatalf("rows = %d, want 11", len(view.Rows))
	}
}

func TestReportRawPassesTheAnswerThrough(t *testing.T) {
	cases := map[string]struct {
		body string
		args []string
	}{
		"pnl":           {larkspurPnl, []string{"report", "pnl", "--month", "2025-01"}},
		"balance-sheet": {larkspurBalanceSheet, []string{"report", "balance-sheet", "--as-of", "2025-02-28"}},
		"cash-flow":     {larkspurCashFlow, []string{"report", "cash-flow", "--month", "2025-01", "--columns", "month"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := newReportServer(t, tc.body)
			out, err := runReport(t, srv, append(append([]string(nil), tc.args...), "--raw", "--json", "--no-cache")...)
			if err != nil {
				t.Fatalf("--raw: %v", err)
			}
			var got, want any
			if err := json.Unmarshal([]byte(out), &got); err != nil {
				t.Fatalf("decode --raw output: %v\n%s", err, out)
			}
			_ = json.Unmarshal([]byte(tc.body), &want)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("--raw changed the answer:\n%s", out)
			}
		})
	}
}
