// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"testing"
)

func TestParseReportAmount(t *testing.T) {
	cases := []struct {
		in   string
		want float64
		ok   bool
	}{
		{"1234.56", 1234.56, true},
		{"1,234.56", 1234.56, true},
		{"$1,234.50", 1234.50, true},
		{"(789.00)", -789.00, true},
		{"", 0, false},
		{"  ", 0, false},
		{"n/a", 0, false},
		{"0.00", 0, true},
	}
	for _, c := range cases {
		got, ok := parseReportAmount(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("parseReportAmount(%q) = (%v, %v), want (%v, %v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

// fixture mirrors the QBO ProfitAndLoss shape (group rows carry Header + nested
// Rows + Summary; data rows carry ColData). All figures are synthetic round
// numbers chosen to exercise the parser, tree-walk, and metric math.
const pnlFixture = `{
  "Header": {"StartPeriod":"2026-05-01","EndPeriod":"2026-05-31","ReportBasis":"Accrual","Currency":"USD","ReportName":"ProfitAndLoss"},
  "Rows": {"Row": [
    {"Header": {"ColData":[{"value":"Recurring Revenue"}]},
     "Rows": {"Row": [
        {"ColData":[{"value":"4010 Subscriptions"},{"value":"100000.00"}]},
        {"ColData":[{"value":"4020 Services"},{"value":"50000.00"}]}
     ]},
     "Summary": {"ColData":[{"value":"Total Recurring Revenue"},{"value":"150000.00"}]}},
    {"Summary": {"ColData":[{"value":"Total Income"},{"value":"150000.00"}]}},
    {"Header": {"ColData":[{"value":"Cost of Goods Sold"}]},
     "Rows": {"Row": [
        {"ColData":[{"value":"5000 Direct Costs"},{"value":"40000.00"}]}
     ]},
     "Summary": {"ColData":[{"value":"Total Cost of Goods Sold"},{"value":"60000.00"}]}},
    {"Summary": {"ColData":[{"value":"Gross Profit"},{"value":"90000.00"}]}},
    {"Summary": {"ColData":[{"value":"Net Operating Income"},{"value":"20000.00"}]}},
    {"Summary": {"ColData":[{"value":"Net Income"},{"value":"15000.00"}]}}
  ]}
}`

func TestWalkReportAndMetrics(t *testing.T) {
	var rep qbReport
	if err := json.Unmarshal([]byte(pnlFixture), &rep); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	var leaves, summaries []reportLeaf
	walkReport(rep.Rows, "", &leaves, &summaries)

	// leaf accounts with section threading
	if len(leaves) != 3 {
		t.Fatalf("want 3 leaves, got %d: %+v", len(leaves), leaves)
	}
	if leaves[0].Name != "4010 Subscriptions" || leaves[0].Amount != 100000.00 {
		t.Errorf("leaf[0] = %+v", leaves[0])
	}
	if leaves[0].Section != "Recurring Revenue" {
		t.Errorf("want section threaded onto revenue leaf, got %q", leaves[0].Section)
	}

	totalIncome, _ := summaryValue(summaries, func(s string) bool { return s == "total income" })
	recurring, _ := summaryValue(summaries, contains("recurring revenue"))
	cogs, _ := summaryValue(summaries, contains("cost of goods sold"))
	gp, _ := summaryValue(summaries, func(s string) bool { return s == "gross profit" })
	oi, _ := summaryValue(summaries, contains("net operating income"))
	ni, _ := summaryValue(summaries, func(s string) bool { return s == "net income" })

	// "Net Income" must not be captured by the "net operating income" matcher
	// and vice versa.
	if ni != 15000.00 {
		t.Errorf("net income = %v, want 15000.00", ni)
	}
	if oi != 20000.00 {
		t.Errorf("operating income = %v, want 20000.00", oi)
	}
	if totalIncome != 150000.00 || recurring != 150000.00 {
		t.Errorf("income totals = %v / %v, want 150000.00", totalIncome, recurring)
	}
	if cogs != 60000.00 || gp != 90000.00 {
		t.Errorf("cogs/gp = %v / %v", cogs, gp)
	}
	if gm := gp / totalIncome * 100; gm < 59.9 || gm > 60.1 {
		t.Errorf("gross margin pct = %v, want ~60.0", gm)
	}
}

func TestMonthBounds(t *testing.T) {
	s, e, err := monthBounds("2026-05")
	if err != nil {
		t.Fatal(err)
	}
	if s != "2026-05-01" || e != "2026-05-31" {
		t.Errorf("monthBounds(2026-05) = %s..%s, want 2026-05-01..2026-05-31", s, e)
	}
	if _, _, err := monthBounds("nonsense"); err == nil {
		t.Error("expected error for malformed month")
	}
}

func TestMatchHighlights(t *testing.T) {
	leaves := []reportLeaf{
		{Name: "Cash", Amount: 5000.00},
		{Name: "Accrued Interest", Amount: 750.00},
		{Name: "Accrued Interest - Note B", Amount: 250.00},
	}
	summaries := []reportLeaf{
		{Name: "Total Current Liabilities", Amount: 20000.00},
		{Name: "Total Deferred Revenue", Amount: 12000.00},
	}

	// A group/parent total lands in summaries, so a summary match wins and
	// reports the whole group balance (not a partial leaf sum).
	got := matchHighlights([]string{"deferred revenue"}, leaves, summaries)
	if len(got) != 1 || got[0].Source != "summary" || got[0].Amount != 12000.00 {
		t.Fatalf("deferred-revenue highlight = %+v", got)
	}
	if len(got[0].Matched) != 1 || got[0].Matched[0] != "Total Deferred Revenue" {
		t.Errorf("matched names = %v", got[0].Matched)
	}

	// A leaf-level term with no matching summary sums every matching account,
	// case-insensitively.
	got = matchHighlights([]string{"ACCRUED INTEREST"}, leaves, summaries)
	if len(got) != 1 || got[0].Source != "leaves" || got[0].Amount != 1000.00 {
		t.Fatalf("accrued-interest highlight = %+v", got)
	}
	if len(got[0].Matched) != 2 {
		t.Errorf("want 2 matched leaves, got %v", got[0].Matched)
	}

	// Unmatched and blank terms produce no output.
	if got := matchHighlights([]string{"nonexistent", "  "}, leaves, summaries); len(got) != 0 {
		t.Errorf("want no matches, got %+v", got)
	}
}
