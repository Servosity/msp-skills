// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// insights.go holds the reprint-added offline analytics: customer
// profitability, DSO, cash forecasting, journal-entry balance checks, the
// composed reconcile sweep, and aging-snapshot deltas. Same contract as
// analytics.go: pure functions over decoded JSON objects (map[string]any),
// json.Number-safe, fully unit-testable, no CLI/store/network dependencies.
package analytics

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// objSlice reads a []map[string]any field (e.g. a QBO Line array), tolerating
// the []any shape json decoding produces. Missing/wrong-type returns nil.
func objSlice(obj map[string]any, key string) []map[string]any {
	raw, ok := obj[key].([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(raw))
	for _, v := range raw {
		if m, ok := v.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// objField reads a nested object field, returning nil when missing.
func objField(obj map[string]any, key string) map[string]any {
	m, _ := obj[key].(map[string]any)
	return m
}

// BoolField reads a bool field; missing or non-bool returns (false, false).
func BoolField(obj map[string]any, key string) (val bool, present bool) {
	b, ok := obj[key].(bool)
	return b, ok
}

// --- Payment ↔ invoice linkage ----------------------------------------------

// payLink is one payment applied to one invoice, with the realized days-to-pay.
type payLink struct {
	CustomerID   string
	CustomerName string
	Days         float64
}

// paymentInvoiceLinks walks each payment's Line[].LinkedTxn[] entries of type
// Invoice and pairs them with the invoice's TxnDate to compute realized
// days-to-pay. Payments or invoices with unparseable dates are skipped.
func paymentInvoiceLinks(invoices, payments []map[string]any) []payLink {
	invByID := make(map[string]map[string]any, len(invoices))
	for _, inv := range invoices {
		if id := Str(inv, "Id"); id != "" {
			invByID[id] = inv
		}
	}
	var links []payLink
	for _, pay := range payments {
		payDate, ok := ParseDate(Str(pay, "TxnDate"))
		if !ok {
			continue
		}
		custID := Nested(pay, "CustomerRef", "value")
		custName := Nested(pay, "CustomerRef", "name")
		for _, line := range objSlice(pay, "Line") {
			for _, lt := range objSlice(line, "LinkedTxn") {
				if Str(lt, "TxnType") != "Invoice" {
					continue
				}
				inv, ok := invByID[Str(lt, "TxnId")]
				if !ok {
					continue
				}
				invDate, ok := ParseDate(Str(inv, "TxnDate"))
				if !ok {
					continue
				}
				days := payDate.Sub(invDate).Hours() / 24
				if days < 0 {
					days = 0
				}
				links = append(links, payLink{CustomerID: custID, CustomerName: custName, Days: days})
			}
		}
	}
	return links
}

// --- Customer profitability --------------------------------------------------

// ProfitRow is one customer's net position.
type ProfitRow struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Invoiced     float64 `json:"invoiced"`
	Paid         float64 `json:"paid"`
	Net          float64 `json:"net_outstanding"` // invoiced - paid
	OpenBalance  float64 `json:"open_balance"`    // sum of open invoice balances
	AvgDaysToPay float64 `json:"avg_days_to_pay"` // 0 when no linked payments
	PaidLinks    int     `json:"paid_links"`      // payment→invoice pairs behind the average
}

// CustomerProfitability joins invoices × payments × customers into per-customer
// net position: lifetime invoiced, payments received, open balance, and
// realized average days-to-pay from payment→invoice links. Sorted by invoiced
// total descending. limit 0 returns all.
func CustomerProfitability(invoices, payments, customers []map[string]any, limit int) []ProfitRow {
	type acc struct {
		row  ProfitRow
		days float64
	}
	byID := map[string]*acc{}
	nameByID := map[string]string{}
	for _, c := range customers {
		nameByID[Str(c, "Id")] = Str(c, "DisplayName")
	}
	get := func(id, name string) *acc {
		key := id
		if key == "" {
			key = "(unspecified)|" + name
		}
		a := byID[key]
		if a == nil {
			if name == "" {
				name = nameByID[id]
			}
			if name == "" {
				name = "(unspecified)"
			}
			a = &acc{row: ProfitRow{ID: id, Name: name}}
			byID[key] = a
		}
		return a
	}
	for _, inv := range invoices {
		a := get(Nested(inv, "CustomerRef", "value"), Nested(inv, "CustomerRef", "name"))
		a.row.Invoiced += Num(inv, "TotalAmt")
		a.row.OpenBalance += Num(inv, "Balance")
	}
	for _, pay := range payments {
		a := get(Nested(pay, "CustomerRef", "value"), Nested(pay, "CustomerRef", "name"))
		a.row.Paid += Num(pay, "TotalAmt")
	}
	for _, l := range paymentInvoiceLinks(invoices, payments) {
		a := get(l.CustomerID, l.CustomerName)
		a.days += l.Days
		a.row.PaidLinks++
	}
	rows := make([]ProfitRow, 0, len(byID))
	for _, a := range byID {
		a.row.Net = a.row.Invoiced - a.row.Paid
		if a.row.PaidLinks > 0 {
			a.row.AvgDaysToPay = round2(a.days / float64(a.row.PaidLinks))
		}
		a.row.Invoiced = round2(a.row.Invoiced)
		a.row.Paid = round2(a.row.Paid)
		a.row.Net = round2(a.row.Net)
		a.row.OpenBalance = round2(a.row.OpenBalance)
		rows = append(rows, a.row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Invoiced != rows[j].Invoiced {
			return rows[i].Invoiced > rows[j].Invoiced
		}
		return rows[i].Name < rows[j].Name
	})
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	return rows
}

func round2(f float64) float64 {
	if f < 0 {
		return float64(int64(f*100-0.5)) / 100
	}
	return float64(int64(f*100+0.5)) / 100
}

// --- Days Sales Outstanding ---------------------------------------------------

// DSOCustomerRow is one customer's realized payment speed.
type DSOCustomerRow struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	AvgDaysToPay float64 `json:"avg_days_to_pay"`
	PaidLinks    int     `json:"paid_links"`
}

// DSOReport is the collections-efficiency KPI snapshot.
type DSOReport struct {
	AsOf         string           `json:"as_of"`
	WindowDays   int              `json:"window_days"`
	ARBalance    float64          `json:"ar_balance"`
	CreditSales  float64          `json:"credit_sales_in_window"`
	DSO          float64          `json:"dso_days"`
	AvgDaysToPay float64          `json:"avg_days_to_pay"` // realized, all linked pairs
	SlowPayers   []DSOCustomerRow `json:"slow_payers"`     // realized avg days-to-pay, slowest first
}

// DSO computes Days Sales Outstanding = (open AR balance ÷ credit sales over
// the trailing window) × window days, plus realized average days-to-pay from
// payment→invoice links. Zero credit sales yields DSO 0 (reported, not NaN).
func DSO(invoices, payments []map[string]any, windowDays int, now time.Time) DSOReport {
	if windowDays <= 0 {
		windowDays = 90
	}
	rep := DSOReport{AsOf: now.Format("2006-01-02"), WindowDays: windowDays}
	cutoff := now.AddDate(0, 0, -windowDays)
	for _, inv := range invoices {
		rep.ARBalance += Num(inv, "Balance")
		if d, ok := ParseDate(Str(inv, "TxnDate")); ok && !d.Before(cutoff) && !d.After(now) {
			rep.CreditSales += Num(inv, "TotalAmt")
		}
	}
	rep.ARBalance = round2(rep.ARBalance)
	rep.CreditSales = round2(rep.CreditSales)
	if rep.CreditSales > 0 {
		rep.DSO = round2(rep.ARBalance / rep.CreditSales * float64(windowDays))
	}
	links := paymentInvoiceLinks(invoices, payments)
	type acc struct {
		name  string
		days  float64
		count int
	}
	byCust := map[string]*acc{}
	var totalDays float64
	for _, l := range links {
		totalDays += l.Days
		a := byCust[l.CustomerID]
		if a == nil {
			a = &acc{name: l.CustomerName}
			byCust[l.CustomerID] = a
		}
		a.days += l.Days
		a.count++
	}
	if len(links) > 0 {
		rep.AvgDaysToPay = round2(totalDays / float64(len(links)))
	}
	for id, a := range byCust {
		rep.SlowPayers = append(rep.SlowPayers, DSOCustomerRow{
			ID: id, Name: a.name,
			AvgDaysToPay: round2(a.days / float64(a.count)), PaidLinks: a.count,
		})
	}
	sort.Slice(rep.SlowPayers, func(i, j int) bool {
		if rep.SlowPayers[i].AvgDaysToPay != rep.SlowPayers[j].AvgDaysToPay {
			return rep.SlowPayers[i].AvgDaysToPay > rep.SlowPayers[j].AvgDaysToPay
		}
		return rep.SlowPayers[i].Name < rep.SlowPayers[j].Name
	})
	return rep
}

// --- Cash-flow due-date forecast ----------------------------------------------

// ForecastWeek is one forward week's scheduled cash movement.
type ForecastWeek struct {
	WeekStart  string  `json:"week_start"`
	WeekEnd    string  `json:"week_end"`
	Inflows    float64 `json:"inflows"`  // open invoice balances due this week
	Outflows   float64 `json:"outflows"` // open bill balances due this week
	Net        float64 `json:"net"`
	Cumulative float64 `json:"cumulative_net"`
}

// ForecastReport projects scheduled (not predicted) cash movement from due dates.
type ForecastReport struct {
	AsOf            string         `json:"as_of"`
	Weeks           int            `json:"weeks"`
	OverdueInflows  float64        `json:"overdue_inflows"`  // due before as_of, still open
	OverdueOutflows float64        `json:"overdue_outflows"` // due before as_of, still unpaid
	Rows            []ForecastWeek `json:"by_week"`
	BeyondInflows   float64        `json:"beyond_window_inflows"`
	BeyondOutflows  float64        `json:"beyond_window_outflows"`
	NetInWindow     float64        `json:"net_in_window"`
}

// CashForecast buckets open invoice balances (inflows) and open bill balances
// (outflows) by DueDate into the next N forward weeks from now. Amounts due
// before now land in the overdue fields; amounts due past the window land in
// the beyond fields. Forecast means scheduled by due date — no prediction.
func CashForecast(invoices, bills []map[string]any, weeks int, now time.Time) ForecastReport {
	if weeks <= 0 {
		weeks = 4
	}
	rep := ForecastReport{AsOf: now.Format("2006-01-02"), Weeks: weeks}
	// Floor to the caller's wall-clock day (not UTC truncation) so week
	// boundaries and the overdue/in-window split agree with as_of.
	y, m, d := now.Date()
	day := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
	rep.Rows = make([]ForecastWeek, weeks)
	for i := 0; i < weeks; i++ {
		ws := day.AddDate(0, 0, i*7)
		we := ws.AddDate(0, 0, 6)
		rep.Rows[i] = ForecastWeek{WeekStart: ws.Format("2006-01-02"), WeekEnd: we.Format("2006-01-02")}
	}
	place := func(items []map[string]any, inflow bool) {
		for _, it := range items {
			bal := Num(it, "Balance")
			if bal <= 0 {
				continue
			}
			due, ok := ParseDate(Str(it, "DueDate"))
			if !ok {
				due, ok = ParseDate(Str(it, "TxnDate"))
				if !ok {
					continue
				}
			}
			switch {
			case due.Before(day):
				if inflow {
					rep.OverdueInflows += bal
				} else {
					rep.OverdueOutflows += bal
				}
			case due.After(day.AddDate(0, 0, weeks*7-1)):
				if inflow {
					rep.BeyondInflows += bal
				} else {
					rep.BeyondOutflows += bal
				}
			default:
				idx := int(due.Sub(day).Hours() / 24 / 7)
				if idx >= weeks {
					idx = weeks - 1
				}
				if inflow {
					rep.Rows[idx].Inflows += bal
				} else {
					rep.Rows[idx].Outflows += bal
				}
			}
		}
	}
	place(invoices, true)
	place(bills, false)
	cum := 0.0
	for i := range rep.Rows {
		rep.Rows[i].Inflows = round2(rep.Rows[i].Inflows)
		rep.Rows[i].Outflows = round2(rep.Rows[i].Outflows)
		rep.Rows[i].Net = round2(rep.Rows[i].Inflows - rep.Rows[i].Outflows)
		cum += rep.Rows[i].Net
		rep.Rows[i].Cumulative = round2(cum)
	}
	rep.OverdueInflows = round2(rep.OverdueInflows)
	rep.OverdueOutflows = round2(rep.OverdueOutflows)
	rep.BeyondInflows = round2(rep.BeyondInflows)
	rep.BeyondOutflows = round2(rep.BeyondOutflows)
	rep.NetInWindow = round2(cum)
	return rep
}

// --- Journal-entry balance check ------------------------------------------------

// JEFinding is one problematic journal entry.
type JEFinding struct {
	ID               string   `json:"id"`
	DocNumber        string   `json:"doc_number"`
	TxnDate          string   `json:"txn_date"`
	Debits           float64  `json:"debits"`
	Credits          float64  `json:"credits"`
	Diff             float64  `json:"diff"`
	Unbalanced       bool     `json:"unbalanced"`
	SuspenseAccounts []string `json:"suspense_accounts,omitempty"`
}

// JECheckReport summarizes the GL hygiene scan.
type JECheckReport struct {
	Checked      int         `json:"checked"`
	Unbalanced   int         `json:"unbalanced"`
	SuspenseHits int         `json:"suspense_hits"`
	Findings     []JEFinding `json:"findings"`
}

// suspenseAccountPattern flags account names that suggest unfinished
// classification work.
var suspensePatterns = []string{"suspense", "uncategor", "ask my accountant", "opening balance equity"}

// CheckJournalEntries sums each entry's debit and credit lines and flags
// entries that don't net to zero (beyond a half-cent tolerance) or that touch
// suspense/uncategorized accounts. Only problematic entries appear in Findings.
func CheckJournalEntries(entries []map[string]any) JECheckReport {
	rep := JECheckReport{Findings: []JEFinding{}}
	for _, je := range entries {
		rep.Checked++
		var debits, credits float64
		var suspense []string
		seenSuspense := map[string]bool{}
		for _, line := range objSlice(je, "Line") {
			det := objField(line, "JournalEntryLineDetail")
			if det == nil {
				continue
			}
			amt := Num(line, "Amount")
			switch Str(det, "PostingType") {
			case "Debit":
				debits += amt
			case "Credit":
				credits += amt
			}
			acct := strings.ToLower(Nested(det, "AccountRef", "name"))
			for _, p := range suspensePatterns {
				if acct != "" && strings.Contains(acct, p) {
					if name := Nested(det, "AccountRef", "name"); !seenSuspense[name] {
						seenSuspense[name] = true
						suspense = append(suspense, name)
					}
					break
				}
			}
		}
		diff := debits - credits
		unbalanced := diff > 0.005 || diff < -0.005
		if !unbalanced && len(suspense) == 0 {
			continue
		}
		if unbalanced {
			rep.Unbalanced++
		}
		if len(suspense) > 0 {
			rep.SuspenseHits++
		}
		rep.Findings = append(rep.Findings, JEFinding{
			ID:               Str(je, "Id"),
			DocNumber:        Str(je, "DocNumber"),
			TxnDate:          Str(je, "TxnDate"),
			Debits:           round2(debits),
			Credits:          round2(credits),
			Diff:             round2(diff),
			Unbalanced:       unbalanced,
			SuspenseAccounts: suspense,
		})
	}
	return rep
}

// --- Reconcile sweep ---------------------------------------------------------

// ReconcileFinding is one category of close-hygiene problem.
type ReconcileFinding struct {
	Category string `json:"category"`
	Count    int    `json:"count"`
	Summary  string `json:"summary"`
	Detail   any    `json:"detail,omitempty"`
}

// ReconcileReport is the composed close-hygiene verdict.
type ReconcileReport struct {
	AsOf     string             `json:"as_of"`
	Clean    bool               `json:"clean"`
	Findings []ReconcileFinding `json:"findings"`
}

// InactiveRef is one open transaction referencing an inactive record.
type InactiveRef struct {
	TxnType   string `json:"txn_type"` // Invoice | Bill
	TxnID     string `json:"txn_id"`
	DocNumber string `json:"doc_number"`
	RefKind   string `json:"ref_kind"` // customer | vendor | item
	RefID     string `json:"ref_id"`
	RefName   string `json:"ref_name"`
}

// inactiveIDs collects Ids of records explicitly marked Active=false.
func inactiveIDs(items []map[string]any) map[string]string {
	out := map[string]string{}
	for _, it := range items {
		if active, present := BoolField(it, "Active"); present && !active {
			out[Str(it, "Id")] = Str(it, "DisplayName")
			if out[Str(it, "Id")] == "" {
				out[Str(it, "Id")] = Str(it, "Name")
			}
		}
	}
	return out
}

// Reconcile composes the close-hygiene scans — unapplied payments, duplicate
// customers/vendors, unbalanced or suspense journal entries, and inactive
// records still referenced by open transactions — into one findings list.
func Reconcile(payments, customers, vendors, journalEntries, invoices, bills, items []map[string]any, now time.Time) ReconcileReport {
	rep := ReconcileReport{AsOf: now.Format("2006-01-02"), Findings: []ReconcileFinding{}}

	if unapplied := UnappliedPayments(payments); len(unapplied) > 0 {
		var total float64
		for _, u := range unapplied {
			total += u.UnappliedAmt
		}
		rep.Findings = append(rep.Findings, ReconcileFinding{
			Category: "unapplied-payments", Count: len(unapplied),
			Summary: fmt.Sprintf("%d payment(s) with %.2f received but not applied to any invoice", len(unapplied), total),
			Detail:  unapplied,
		})
	}
	if d := Dupes(customers); len(d) > 0 {
		rep.Findings = append(rep.Findings, ReconcileFinding{
			Category: "duplicate-customers", Count: len(d),
			Summary: fmt.Sprintf("%d duplicate customer group(s) by normalized name/email", len(d)),
			Detail:  d,
		})
	}
	if d := Dupes(vendors); len(d) > 0 {
		rep.Findings = append(rep.Findings, ReconcileFinding{
			Category: "duplicate-vendors", Count: len(d),
			Summary: fmt.Sprintf("%d duplicate vendor group(s) by normalized name/email", len(d)),
			Detail:  d,
		})
	}
	if je := CheckJournalEntries(journalEntries); len(je.Findings) > 0 {
		rep.Findings = append(rep.Findings, ReconcileFinding{
			Category: "journal-entry-anomalies", Count: len(je.Findings),
			Summary: fmt.Sprintf("%d journal entr(ies) unbalanced or touching suspense accounts", len(je.Findings)),
			Detail:  je.Findings,
		})
	}

	inactiveCust := inactiveIDs(customers)
	inactiveVend := inactiveIDs(vendors)
	inactiveItem := inactiveIDs(items)
	var refs []InactiveRef
	for _, inv := range invoices {
		if Num(inv, "Balance") <= 0 {
			continue
		}
		if cid := Nested(inv, "CustomerRef", "value"); cid != "" {
			if name, bad := inactiveCust[cid]; bad {
				refs = append(refs, InactiveRef{TxnType: "Invoice", TxnID: Str(inv, "Id"), DocNumber: Str(inv, "DocNumber"), RefKind: "customer", RefID: cid, RefName: name})
			}
		}
		for _, line := range objSlice(inv, "Line") {
			det := objField(line, "SalesItemLineDetail")
			if det == nil {
				continue
			}
			if iid := Nested(det, "ItemRef", "value"); iid != "" {
				if name, bad := inactiveItem[iid]; bad {
					refs = append(refs, InactiveRef{TxnType: "Invoice", TxnID: Str(inv, "Id"), DocNumber: Str(inv, "DocNumber"), RefKind: "item", RefID: iid, RefName: name})
				}
			}
		}
	}
	for _, bill := range bills {
		if Num(bill, "Balance") <= 0 {
			continue
		}
		if vid := Nested(bill, "VendorRef", "value"); vid != "" {
			if name, bad := inactiveVend[vid]; bad {
				refs = append(refs, InactiveRef{TxnType: "Bill", TxnID: Str(bill, "Id"), DocNumber: Str(bill, "DocNumber"), RefKind: "vendor", RefID: vid, RefName: name})
			}
		}
	}
	if len(refs) > 0 {
		rep.Findings = append(rep.Findings, ReconcileFinding{
			Category: "inactive-on-open-transactions", Count: len(refs),
			Summary: fmt.Sprintf("%d open transaction reference(s) to inactive customers/vendors/items", len(refs)),
			Detail:  refs,
		})
	}

	rep.Clean = len(rep.Findings) == 0
	return rep
}

// --- Aging snapshot delta -------------------------------------------------------

// AgingSnapshot is one persisted AR+AP aging picture.
type AgingSnapshot struct {
	TakenAt string      `json:"taken_at"`
	AR      AgingReport `json:"ar"`
	AP      AgingReport `json:"ap"`
}

// EntityDelta is one customer/vendor whose aging position changed between
// snapshots.
type EntityDelta struct {
	Kind            string  `json:"kind"` // customer | vendor
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	PrevTotal       float64 `json:"prev_total"`
	CurrTotal       float64 `json:"curr_total"`
	Delta           float64 `json:"delta"`
	PrevWorstBucket string  `json:"prev_worst_bucket"`
	CurrWorstBucket string  `json:"curr_worst_bucket"`
	BucketSlipped   bool    `json:"bucket_slipped"` // worst bucket got older
	New             bool    `json:"new,omitempty"`
	Gone            bool    `json:"gone,omitempty"`
}

// AgingDeltaReport is the diff between two aging snapshots.
type AgingDeltaReport struct {
	PrevTakenAt  string        `json:"prev_taken_at"`
	CurrAsOf     string        `json:"curr_as_of"`
	ARTotalDelta float64       `json:"ar_total_delta"`
	APTotalDelta float64       `json:"ap_total_delta"`
	Changes      []EntityDelta `json:"changes"`
	Slipped      int           `json:"slipped_count"`
}

// worstBucket returns the oldest aging bucket holding more than half a cent.
func worstBucket(buckets map[string]float64) string {
	worst := ""
	for _, b := range AgingBuckets {
		if buckets[b] > 0.005 {
			worst = b
		}
	}
	return worst
}

func bucketIndex(b string) int {
	for i, x := range AgingBuckets {
		if x == b {
			return i
		}
	}
	return -1
}

func diffEntities(kind string, prev, curr []AgingEntityRow) []EntityDelta {
	prevByKey := map[string]AgingEntityRow{}
	for _, r := range prev {
		prevByKey[r.ID+"|"+r.Name] = r
	}
	currKeys := map[string]bool{}
	var out []EntityDelta
	for _, c := range curr {
		key := c.ID + "|" + c.Name
		currKeys[key] = true
		p, existed := prevByKey[key]
		pw, cw := worstBucket(p.Buckets), worstBucket(c.Buckets)
		d := EntityDelta{
			Kind: kind, ID: c.ID, Name: c.Name,
			PrevTotal: round2(p.Total), CurrTotal: round2(c.Total), Delta: round2(c.Total - p.Total),
			PrevWorstBucket: pw, CurrWorstBucket: cw,
			BucketSlipped: existed && bucketIndex(cw) > bucketIndex(pw),
			New:           !existed,
		}
		if d.Delta != 0 || d.BucketSlipped || d.New {
			out = append(out, d)
		}
	}
	for key, p := range prevByKey {
		if currKeys[key] {
			continue
		}
		out = append(out, EntityDelta{
			Kind: kind, ID: p.ID, Name: p.Name,
			PrevTotal: round2(p.Total), CurrTotal: 0, Delta: round2(-p.Total),
			PrevWorstBucket: worstBucket(p.Buckets), CurrWorstBucket: "",
			Gone: true,
		})
	}
	return out
}

// AgingDelta diffs two aging snapshots and surfaces every customer/vendor whose
// outstanding total or worst bucket changed, sorted by |delta| descending.
func AgingDelta(prev, curr AgingSnapshot) AgingDeltaReport {
	rep := AgingDeltaReport{
		PrevTakenAt:  prev.TakenAt,
		CurrAsOf:     curr.AR.AsOf,
		ARTotalDelta: round2(curr.AR.Total - prev.AR.Total),
		APTotalDelta: round2(curr.AP.Total - prev.AP.Total),
		Changes:      []EntityDelta{},
	}
	if rep.CurrAsOf == "" {
		rep.CurrAsOf = curr.AP.AsOf
	}
	rep.Changes = append(rep.Changes, diffEntities("customer", prev.AR.Entities, curr.AR.Entities)...)
	rep.Changes = append(rep.Changes, diffEntities("vendor", prev.AP.Entities, curr.AP.Entities)...)
	for _, c := range rep.Changes {
		if c.BucketSlipped {
			rep.Slipped++
		}
	}
	sort.Slice(rep.Changes, func(i, j int) bool {
		ai, aj := rep.Changes[i].Delta, rep.Changes[j].Delta
		if ai < 0 {
			ai = -ai
		}
		if aj < 0 {
			aj = -aj
		}
		if ai != aj {
			return ai > aj
		}
		return rep.Changes[i].Name < rep.Changes[j].Name
	})
	return rep
}
