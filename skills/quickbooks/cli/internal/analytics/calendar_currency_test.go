// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package analytics

import (
	"encoding/json"
	"testing"
	"time"
)

func mustZone(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Skipf("tz database unavailable: %v", err)
	}
	return loc
}

// TestQBODatesAreCalendarDaysInCallerZone pins the class "a QBO calendar date
// (TxnDate, DueDate: date-only fields in the Intuit Invoice and Bill reference)
// compared with now as if it were a UTC instant". Every date computation must
// count whole calendar days between the date as written and today in now's
// zone, so a run at 23:00 in New York or 08:00 in Sydney ages, buckets,
// forecasts and windows exactly as a run at noon does.
func TestQBODatesAreCalendarDaysInCallerZone(t *testing.T) {
	ny := time.Date(2026, 10, 24, 23, 0, 0, 0, mustZone(t, "America/New_York"))
	syd := time.Date(2026, 10, 24, 8, 0, 0, 0, mustZone(t, "Australia/Sydney"))

	for _, c := range []struct {
		due  string
		now  time.Time
		want int
	}{
		{"2026-10-24", ny, 0},
		{"2026-09-24", ny, 30},
		{"2026-10-23", syd, 1},
		{"2026-10-24", syd, 0},
		{"2026-10-24T23:30:00-07:00", ny, 0}, // a datetime keeps the date it was written on
	} {
		if got := DaysPastDue(c.due, c.now); got != c.want {
			t.Errorf("DaysPastDue(%q, %s) = %d, want %d", c.due, c.now, got, c.want)
		}
	}

	ar := Aging([]map[string]any{{"Id": "1", "Balance": 100.0, "DueDate": "2026-09-24"}}, "DueDate", "TxnDate", "Balance", "CustomerRef", ny)
	if ar.Buckets["0-30"] != 100 {
		t.Errorf("an invoice 30 calendar days past due must age 0-30 at 23:00 local, got buckets %v", ar.Buckets)
	}

	stale := StaleInvoices([]map[string]any{{"Id": "1", "Balance": 100.0, "DueDate": "2026-09-23"}}, 30, ny)
	if len(stale) != 1 || stale[0].DaysPastDue != 31 {
		t.Errorf("stale invoice days_past_due = %+v, want one row at 31", stale)
	}

	fc := CashForecast([]map[string]any{
		{"Id": "today", "Balance": 10.0, "DueDate": "2026-10-24"},
		{"Id": "next-week", "Balance": 20.0, "DueDate": "2026-10-31"},
	}, nil, 2, ny)
	if fc.OverdueInflows != 0 || fc.Rows[0].Inflows != 10 || fc.Rows[1].Inflows != 20 {
		t.Errorf("forecast at 23:00 local: overdue=%v week0=%v week1=%v, want 0/10/20", fc.OverdueInflows, fc.Rows[0].Inflows, fc.Rows[1].Inflows)
	}

	dso := DSO([]map[string]any{
		{"Id": "today", "TxnDate": "2026-10-24", "TotalAmt": 100.0, "Balance": 100.0},
		{"Id": "edge", "TxnDate": "2026-07-27", "TotalAmt": 50.0, "Balance": 0.0},  // 89 days back: the window's first day
		{"Id": "old", "TxnDate": "2026-07-26", "TotalAmt": 1000.0, "Balance": 0.0}, // 90 days back: outside
		{"Id": "future", "TxnDate": "2026-10-25", "TotalAmt": 7.0, "Balance": 0.0},
	}, nil, 90, syd)
	if dso.CreditSales != 150 {
		t.Errorf("DSO credit sales over the 90-day window at 08:00 Sydney = %v, want 150 (today + the window's first day; not 90 days back, not tomorrow)", dso.CreditSales)
	}

	due := BillsDue([]map[string]any{{"Id": "1", "Balance": 5.0, "DueDate": "2026-10-31"}}, 7, ny)
	if due.BillCount != 1 || due.Bills[0].DaysUntilDue != 7 {
		t.Errorf("bills due in 7 calendar days at 23:00 local = %+v", due.Bills)
	}
}

// usdEur is one home-currency (USD) and one foreign (EUR at 1.2 USD) record of
// the same document type, as a multicurrency QBO company returns them (Intuit
// Invoice/Bill/Payment reference: CurrencyRef, ExchangeRate, HomeBalance,
// HomeTotalAmt). The EUR record is worth 120 USD wherever it is 100 EUR.
func usdEur(t *testing.T, eurHasHomeFields bool, usd, eur string) []map[string]any {
	t.Helper()
	home := ""
	if eurHasHomeFields {
		home = `,"HomeBalance":120,"HomeTotalAmt":120`
	}
	return decodeMany(t,
		`{"Id":"U","CurrencyRef":{"value":"USD"},"ExchangeRate":1,`+usd+`}`,
		`{"Id":"E","CurrencyRef":{"value":"EUR"},"ExchangeRate":1.2`+home+`,`+eur+`}`,
	)
}

// TestMoneyAggregatesUseHomeCurrency pins the class "every money aggregate
// sums transaction amounts written in different currencies": each one must
// read amounts through the one home-currency reader, so 100 USD + 100 EUR at
// 1.2 is 220, never 200.
func TestMoneyAggregatesUseHomeCurrency(t *testing.T) {
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	for _, withHome := range []bool{true, false} {
		docs := usdEur(t, withHome,
			`"Balance":100,"TotalAmt":100,"DueDate":"2026-10-24","TxnDate":"2026-10-01","EmailStatus":"NeedToSend","VendorRef":{"value":"V1","name":"Vendor"},"CustomerRef":{"value":"C1","name":"Cust"}`,
			`"Balance":100,"TotalAmt":100,"DueDate":"2026-10-24","TxnDate":"2026-10-01","EmailStatus":"NeedToSend","VendorRef":{"value":"V1","name":"Vendor"},"CustomerRef":{"value":"C1","name":"Cust"}`,
		)
		if got := BillsDue(docs, 7, now); got.TotalDue != 220 || len(got.ByVendor) != 1 || got.ByVendor[0].Total != 220 {
			t.Errorf("home=%v bills due total=%v by_vendor=%+v, want 220", withHome, got.TotalDue, got.ByVendor)
		} else {
			for _, b := range got.Bills {
				if b.ID == "E" && (b.Balance != 120 || b.Currency != "EUR" || b.ForeignBalance != 100) {
					t.Errorf("home=%v EUR bill row = %+v, want balance 120 USD, currency EUR, foreign_balance 100", withHome, b)
				}
				if b.ID == "U" && (b.Currency != "" || b.ForeignBalance != 0) {
					t.Errorf("home=%v home-currency bill row carries foreign fields: %+v", withHome, b)
				}
			}
		}
		if got := Aging(docs, "DueDate", "TxnDate", "Balance", "CustomerRef", now); got.Total != 220 || got.Buckets["0-30"] != 220 || got.Entities[0].Total != 220 {
			t.Errorf("home=%v aging total=%v buckets=%v", withHome, got.Total, got.Buckets)
		}
		if got := UnsentInvoices(docs); got.TotalBalance != 220 {
			t.Errorf("home=%v unsent total=%v, want 220", withHome, got.TotalBalance)
		}
		if got := StaleInvoices(docs, -1, now); len(got) != 2 || got[0].Balance+got[1].Balance != 220 {
			t.Errorf("home=%v stale rows=%+v, want balances summing to 220", withHome, got)
		}
		if got := CashForecast(docs, docs, 1, now); got.Rows[0].Inflows != 220 || got.Rows[0].Outflows != 220 {
			t.Errorf("home=%v forecast week0=%+v, want 220/220", withHome, got.Rows[0])
		}
		if got := DSO(docs, nil, 90, now); got.ARBalance != 220 || got.CreditSales != 220 {
			t.Errorf("home=%v dso ar=%v sales=%v, want 220/220", withHome, got.ARBalance, got.CreditSales)
		}
		if got := CustomerProfitability(docs, nil, nil, 0); len(got) != 1 || got[0].Invoiced != 220 || got[0].OpenBalance != 220 {
			t.Errorf("home=%v profitability=%+v, want invoiced/open 220", withHome, got)
		}
		if got := SumByRef(docs, "TotalAmt", "VendorRef", "TxnDate", time.Time{}, false); len(got) != 1 || got[0].Total != 220 {
			t.Errorf("home=%v vendor spend=%+v, want 220", withHome, got)
		}
	}

	payments := usdEur(t, false, `"TotalAmt":50,"UnappliedAmt":50`, `"TotalAmt":50,"UnappliedAmt":50`)
	unapplied := UnappliedPayments(payments)
	if len(unapplied) != 2 || unapplied[0].UnappliedAmt != 60 || unapplied[0].Currency != "EUR" || unapplied[0].ForeignUnappliedAmt != 50 || unapplied[1].UnappliedAmt != 50 {
		t.Errorf("unapplied payments = %+v, want EUR 50 shown as 60 home first, then 50", unapplied)
	}
	rec := Reconcile(payments, nil, nil, nil, nil, nil, nil, now)
	if len(rec.Findings) != 1 || rec.Findings[0].Summary != "2 payment(s) with 110.00 received but not applied to any invoice" {
		t.Errorf("reconcile unapplied summary = %+v, want 110.00 home", rec.Findings)
	}
}

// TestNameListBalancesNeverSumAcrossCurrencies pins the same class for
// name-list records, which carry no exchange rate: a multicurrency company's
// Customer.Balance, Vendor.Balance and Account.CurrentBalance are each in the
// record's own CurrencyRef (Intuit Customer/Vendor/Account reference), so a
// total or ranking across two currencies is withheld, never computed.
func TestNameListBalancesNeverSumAcrossCurrencies(t *testing.T) {
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	customers := decodeMany(t,
		`{"Id":"1","DisplayName":"Yen Co","Balance":100000,"CurrencyRef":{"value":"JPY"}}`,
		`{"Id":"2","DisplayName":"Dollar Co","Balance":900,"CurrencyRef":{"value":"USD"}}`,
		`{"Id":"3","DisplayName":"Dollar Two","Balance":50,"CurrencyRef":{"value":"USD"}}`,
	)
	accounts := decodeMany(t,
		`{"Id":"10","AccountType":"Bank","CurrentBalance":500,"CurrencyRef":{"value":"USD"}}`,
		`{"Id":"11","AccountType":"Bank","CurrentBalance":700,"CurrencyRef":{"value":"EUR"}}`,
	)
	rep := Balances(accounts, customers, nil, now)
	if rep.CustomerReceivables != nil || rep.BankTotal != nil || rep.VendorPayables != nil || len(rep.AccountsByType) != 0 {
		raw, _ := json.Marshal(rep)
		t.Fatalf("balances summed across currencies: %s", raw)
	}
	if got := rep.ByCurrency["JPY"]; got == nil || got.CustomerReceivables != 100000 {
		t.Errorf("JPY receivables = %+v, want 100000", got)
	}
	if got := rep.ByCurrency["USD"]; got == nil || got.CustomerReceivables != 950 || got.BankTotal != 500 || got.AccountsByType["Bank"] != 500 {
		t.Errorf("USD balances = %+v, want receivables 950, bank 500", got)
	}
	if got := rep.ByCurrency["EUR"]; got == nil || got.BankTotal != 700 {
		t.Errorf("EUR balances = %+v, want bank 700", got)
	}

	single := Balances(accounts[:1], customers[1:], nil, now)
	if single.CustomerReceivables == nil || *single.CustomerReceivables != 950 || single.BankTotal == nil || *single.BankTotal != 500 || single.ByCurrency != nil {
		raw, _ := json.Marshal(single)
		t.Errorf("single-currency balances changed shape: %s", raw)
	}

	ranked := TopByField(customers, "Balance", "DisplayName", 1)
	if len(ranked) != 2 {
		t.Fatalf("mixed-currency ranking = %+v, want the top row of each currency", ranked)
	}
	for _, r := range ranked {
		if (r.Currency == "JPY" && r.ID != "1") || (r.Currency == "USD" && r.ID != "2") || r.Currency == "" {
			t.Errorf("mixed-currency ranking row %+v ranks across currencies", r)
		}
	}
	if one := TopByField(customers[1:], "Balance", "DisplayName", 0); len(one) != 2 || one[0].ID != "2" || one[0].Currency != "" {
		t.Errorf("single-currency ranking changed: %+v", one)
	}
}

// TestOpenRecordsAreDecidedOnTheWrittenAmount pins the class "whether a debt
// exists is decided on a converted, rounded amount": a foreign remainder whose
// home value rounds to 0.00 (0.10 INR at 0.012 USD) is still an open bill,
// invoice or unapplied payment, so it stays in every list, with its foreign
// amount shown.
func TestOpenRecordsAreDecidedOnTheWrittenAmount(t *testing.T) {
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	for _, withHome := range []bool{false, true} {
		home := ""
		if withHome {
			home = `,"HomeBalance":0,"HomeUnappliedAmt":0`
		}
		docs := decodeMany(t, `{"Id":"7","Balance":0.10,"UnappliedAmt":0.10,"TotalAmt":5,"DueDate":"2026-09-01","TxnDate":"2026-09-01","EmailStatus":"NeedToSend","CurrencyRef":{"value":"INR"},"ExchangeRate":0.012`+home+`}`)
		if got := BillsDue(docs, 7, now); got.BillCount != 1 || got.Bills[0].Currency != "INR" || got.Bills[0].ForeignBalance != 0.10 {
			t.Errorf("home=%v bills due dropped the INR remainder: %+v", withHome, got)
		}
		if got := Aging(docs, "DueDate", "TxnDate", "Balance", "VendorRef", now); got.Count != 1 {
			t.Errorf("home=%v aging open_count=%d, want 1", withHome, got.Count)
		}
		if got := StaleInvoices(docs, 30, now); len(got) != 1 || got[0].ForeignBalance != 0.10 {
			t.Errorf("home=%v stale invoices dropped the INR remainder: %+v", withHome, got)
		}
		if got := UnsentInvoices(docs); got.Count != 1 {
			t.Errorf("home=%v unsent invoices dropped the INR remainder: %+v", withHome, got)
		}
		if got := UnappliedPayments(docs); len(got) != 1 || got[0].ForeignUnappliedAmt != 0.10 {
			t.Errorf("home=%v unapplied payments dropped the INR remainder: %+v", withHome, got)
		}
		if got := CashForecast(docs, docs, 1, now); got.OverdueInflows != 0 || got.OverdueOutflows != 0 {
			// The remainder is open (counted) but worth 0.00 at home: the
			// totals are honestly zero, nothing else to assert beyond no panic.
			t.Errorf("home=%v forecast overdue = %v/%v, want 0/0 home", withHome, got.OverdueInflows, got.OverdueOutflows)
		}
	}
}
