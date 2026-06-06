package xfin

import (
	"testing"
	"time"
)

func TestParseXeroDate(t *testing.T) {
	cases := []struct {
		in   string
		ok   bool
		year int
	}{
		{"/Date(1714003200000)/", true, 2024},
		{"/Date(1714003200000+1200)/", true, 2024},
		{"2024-04-25T00:00:00", true, 2024},
		{"2024-04-25", true, 2024},
		{"", false, 0},
		{"not-a-date", false, 0},
	}
	for _, c := range cases {
		got, ok := ParseXeroDate(c.in)
		if ok != c.ok {
			t.Errorf("ParseXeroDate(%q) ok=%v, want %v", c.in, ok, c.ok)
			continue
		}
		if ok && got.Year() != c.year {
			t.Errorf("ParseXeroDate(%q) year=%d, want %d", c.in, got.Year(), c.year)
		}
	}
}

func ms(t time.Time) string {
	return "/Date(" + itoa(t.UnixMilli()) + ")/"
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func TestComputeAging(t *testing.T) {
	asOf := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)
	invoices := []Invoice{
		{InvoiceID: "i1", Type: "ACCREC", Status: "AUTHORISED", AmountDue: 100, DueDate: ms(asOf.AddDate(0, 0, 10))},   // current (due in future)
		{InvoiceID: "i2", Type: "ACCREC", Status: "AUTHORISED", AmountDue: 200, DueDate: ms(asOf.AddDate(0, 0, -15))},  // 1-30
		{InvoiceID: "i3", Type: "ACCREC", Status: "AUTHORISED", AmountDue: 300, DueDate: ms(asOf.AddDate(0, 0, -100))}, // 90+
		{InvoiceID: "i4", Type: "ACCREC", Status: "PAID", AmountDue: 0, DueDate: ms(asOf.AddDate(0, 0, -100))},         // excluded (paid)
		{InvoiceID: "i5", Type: "ACCPAY", Status: "AUTHORISED", AmountDue: 999, DueDate: ms(asOf.AddDate(0, 0, -5))},   // excluded (payable)
	}
	rep := ComputeAging(invoices, asOf, false)
	if rep.Kind != "receivable" {
		t.Fatalf("kind = %q", rep.Kind)
	}
	if rep.InvoiceCount != 3 {
		t.Fatalf("invoice count = %d, want 3 (paid + payable excluded)", rep.InvoiceCount)
	}
	if rep.TotalDue != 600 {
		t.Fatalf("total due = %v, want 600", rep.TotalDue)
	}
	want := map[string]float64{"current": 100, "1-30": 200, "90+": 300, "31-60": 0, "61-90": 0}
	for _, b := range rep.Buckets {
		if b.TotalDue != want[b.Label] {
			t.Errorf("bucket %q total = %v, want %v", b.Label, b.TotalDue, want[b.Label])
		}
	}

	// payable view picks up only the ACCPAY invoice
	pay := ComputeAging(invoices, asOf, true)
	if pay.Kind != "payable" || pay.InvoiceCount != 1 || pay.TotalDue != 999 {
		t.Fatalf("payable aging = %+v, want 1 invoice / 999", pay)
	}
}

func TestComputeReconcile(t *testing.T) {
	invoices := []Invoice{
		{InvoiceID: "i1", Type: "ACCREC", Status: "AUTHORISED", AmountDue: 150, AmountPaid: 0, Contact: ContactRef{ContactID: "c1", Name: "Acme"}},
		{InvoiceID: "i2", Type: "ACCREC", Status: "PAID", AmountDue: 0, AmountPaid: 300},
	}
	payments := []Payment{
		{PaymentID: "p1", Amount: 300, Invoice: InvoiceRef{InvoiceID: "i2"}},
		{PaymentID: "p2", Amount: 75, Invoice: InvoiceRef{InvoiceID: "missing"}, Contact: ContactRef{Name: "Ghost"}}, // orphan
	}
	rep := ComputeReconcile(invoices, payments)
	if rep.OutstandingCount != 1 || rep.TotalOutstanding != 150 {
		t.Fatalf("outstanding = %d / %v, want 1 / 150", rep.OutstandingCount, rep.TotalOutstanding)
	}
	if rep.OrphanCount != 1 || rep.OrphanPayments[0].PaymentID != "p2" {
		t.Fatalf("orphan payments = %+v, want p2", rep.OrphanPayments)
	}
}

func TestComputeBankRecon(t *testing.T) {
	txns := []BankTransaction{
		{BankTransactionID: "b1", Type: "SPEND", Total: 50, IsReconciled: false, Contact: ContactRef{ContactID: "c1", Name: "Acme"}},
		{BankTransactionID: "b2", Type: "RECEIVE", Total: 999, IsReconciled: true}, // excluded (reconciled)
	}
	invoices := []Invoice{
		{InvoiceID: "i1", InvoiceNumber: "INV-1", Total: 50, Contact: ContactRef{ContactID: "c1", Name: "Acme"}},
	}
	payments := []Payment{
		{PaymentID: "p1", Amount: 50, Contact: ContactRef{ContactID: "c1", Name: "Acme"}},
	}
	rep := ComputeBankRecon(txns, invoices, payments)
	if rep.UnreconciledCount != 1 {
		t.Fatalf("unreconciled count = %d, want 1", rep.UnreconciledCount)
	}
	if got := len(rep.Unreconciled[0].Candidates); got != 2 {
		t.Fatalf("candidates = %d, want 2 (one invoice + one payment matched on contact+amount)", got)
	}
}

func TestComputeTieOut_TiesAndVariance(t *testing.T) {
	accounts := []Account{
		{AccountID: "ar", Code: "610", Name: "Accounts Receivable", SystemAccount: "DEBTORS"},
		{AccountID: "ap", Code: "800", Name: "Accounts Payable", SystemAccount: "CREDITORS"},
	}
	journals := []Journal{
		{JournalNumber: 1, JournalLines: []JournalLine{{AccountCode: "610", NetAmount: 150}}},
		{JournalNumber: 2, JournalLines: []JournalLine{{AccountCode: "800", NetAmount: 40}}},
	}
	invoices := []Invoice{
		{InvoiceID: "i1", Type: "ACCREC", Status: "AUTHORISED", AmountDue: 150},
		{InvoiceID: "i2", Type: "ACCPAY", Status: "AUTHORISED", AmountDue: 100}, // mismatch vs GL 40 -> variance -60
	}
	rep := ComputeTieOut(journals, invoices, accounts)
	if len(rep.Lines) != 2 {
		t.Fatalf("lines = %d, want 2", len(rep.Lines))
	}
	var ar, ap TieOutLine
	for _, l := range rep.Lines {
		if l.Control == "receivable" {
			ar = l
		} else {
			ap = l
		}
	}
	if !ar.Ties || ar.Variance != 0 {
		t.Errorf("AR should tie: %+v", ar)
	}
	if ap.Ties || ap.Variance != -60 {
		t.Errorf("AP variance = %v ties=%v, want -60 / false", ap.Variance, ap.Ties)
	}
	if rep.AllTie {
		t.Errorf("AllTie should be false when AP doesn't tie")
	}
}

func TestComputeTieOut_NameFallbackWhenNoSystemAccount(t *testing.T) {
	accounts := []Account{{AccountID: "ar", Code: "610", Name: "Trade Receivables"}}
	journals := []Journal{{JournalNumber: 1, JournalLines: []JournalLine{{AccountCode: "610", NetAmount: 80}}}}
	invoices := []Invoice{{Type: "ACCREC", Status: "AUTHORISED", AmountDue: 80}}
	rep := ComputeTieOut(journals, invoices, accounts)
	if len(rep.Lines) != 1 || !rep.Lines[0].Ties {
		t.Fatalf("name-fallback AR tie failed: %+v unmatched=%v", rep.Lines, rep.Unmatched)
	}
}

func TestComputeLedger_RunningBalance(t *testing.T) {
	accounts := []Account{{Code: "610", Name: "Accounts Receivable"}}
	journals := []Journal{
		{JournalNumber: 2, JournalDate: "/Date(1714003200000)/", JournalLines: []JournalLine{{AccountCode: "610", NetAmount: -50, Description: "payment"}}},
		{JournalNumber: 1, JournalDate: "/Date(1711000000000)/", JournalLines: []JournalLine{{AccountCode: "610", NetAmount: 150, Description: "invoice"}}},
		{JournalNumber: 3, JournalLines: []JournalLine{{AccountCode: "999", NetAmount: 7}}}, // other account, excluded
	}
	stmt := ComputeLedger(journals, "610", accounts)
	if stmt.EntryCount != 2 {
		t.Fatalf("entries = %d, want 2", stmt.EntryCount)
	}
	// Ordered by journal number: 1 (+150) then 2 (-50) -> running 150, 100
	if stmt.Entries[0].JournalNumber != 1 || stmt.Entries[0].RunningBalance != 150 {
		t.Errorf("entry0 = %+v, want jn1 balance 150", stmt.Entries[0])
	}
	if stmt.Entries[1].RunningBalance != 100 {
		t.Errorf("entry1 running = %v, want 100", stmt.Entries[1].RunningBalance)
	}
	if stmt.FinalBalance != 100 || stmt.AccountName != "Accounts Receivable" {
		t.Errorf("final = %v name=%q", stmt.FinalBalance, stmt.AccountName)
	}
}

func TestComputeExposure_RankedAndOverdue(t *testing.T) {
	asOf := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)
	invoices := []Invoice{
		{Type: "ACCREC", Status: "AUTHORISED", AmountDue: 100, DueDate: ms(asOf.AddDate(0, 0, -10)), Contact: ContactRef{ContactID: "c1", Name: "Acme"}},
		{Type: "ACCREC", Status: "AUTHORISED", AmountDue: 50, DueDate: ms(asOf.AddDate(0, 0, 10)), Contact: ContactRef{ContactID: "c1", Name: "Acme"}},
		{Type: "ACCREC", Status: "AUTHORISED", AmountDue: 500, DueDate: ms(asOf.AddDate(0, 0, -5)), Contact: ContactRef{ContactID: "c2", Name: "Big Co"}},
	}
	contacts := []Contact{{ContactID: "c1", Name: "Acme"}, {ContactID: "c2", Name: "Big Co"}}
	exp := ComputeExposure(invoices, contacts, asOf, "")
	if len(exp) != 2 {
		t.Fatalf("exposure rows = %d, want 2", len(exp))
	}
	if exp[0].ContactID != "c2" || exp[0].TotalDue != 500 {
		t.Errorf("top exposure = %+v, want Big Co 500", exp[0])
	}
	if exp[1].TotalDue != 150 || exp[1].OverdueDue != 100 || exp[1].OverdueCount != 1 {
		t.Errorf("Acme exposure = %+v, want total 150 overdue 100/1", exp[1])
	}
	// filter to one contact
	one := ComputeExposure(invoices, contacts, asOf, "c1")
	if len(one) != 1 || one[0].ContactID != "c1" {
		t.Errorf("filtered exposure = %+v, want only c1", one)
	}
}

func TestComputeSnapshot(t *testing.T) {
	asOf := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)
	invoices := []Invoice{
		{Type: "ACCREC", Status: "AUTHORISED", AmountDue: 150, DueDate: ms(asOf.AddDate(0, 0, -3))},
		{Type: "ACCPAY", Status: "AUTHORISED", AmountDue: 60, DueDate: ms(asOf.AddDate(0, 0, 5))},
	}
	bank := []BankTransaction{{IsReconciled: false}, {IsReconciled: true}}
	s := ComputeSnapshot(invoices, bank, asOf, map[string]int{"invoices": 2}, map[string]string{"invoices": "2024-05-01"})
	if s.ReceivableOutstanding != 150 || s.PayableOutstanding != 60 {
		t.Fatalf("AR/AP = %v/%v, want 150/60", s.ReceivableOutstanding, s.PayableOutstanding)
	}
	if s.OverdueReceivable != 1 || s.UnreconciledBankTxns != 1 {
		t.Fatalf("overdue/unreconciled = %d/%d, want 1/1", s.OverdueReceivable, s.UnreconciledBankTxns)
	}
}

// A control account that resolves by SystemAccount/name but carries no
// user-facing Code must be reported unmatched — matching on "" would sum
// every code-less journal line into the control balance (silent wrong
// variance). Pins the guard added in the 4.22 reprint review.
func TestComputeTieOutEmptyControlCodeIsUnmatched(t *testing.T) {
	accounts := []Account{{AccountID: "a1", Name: "Accounts Receivable", SystemAccount: "DEBTORS", Code: ""}}
	journals := []Journal{{JournalID: "j1", JournalLines: []JournalLine{
		{AccountCode: "", NetAmount: 500},
		{AccountCode: "200", NetAmount: 100},
	}}}
	rep := ComputeTieOut(journals, nil, accounts)
	if rep.AllTie {
		t.Fatalf("AllTie = true, want false when the control account has no Code")
	}
	found := false
	for _, u := range rep.Unmatched {
		if u == "receivable" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Unmatched = %v, want to contain %q", rep.Unmatched, "receivable")
	}
	for _, l := range rep.Lines {
		if l.Control == "receivable" {
			t.Fatalf("receivable produced a tie-out line %+v despite empty control code", l)
		}
	}
}
