package xfin

import (
	"sort"
	"strings"
	"time"
)

// round2 rounds a money amount to 2 decimal places to keep JSON output clean
// after float accumulation.
func round2(f float64) float64 {
	return float64(int64(f*100+sign(f)*0.5)) / 100
}

func sign(f float64) float64 {
	if f < 0 {
		return -1
	}
	return 1
}

// outstanding reports whether an invoice is approved and still owed.
func outstanding(inv Invoice) bool {
	return inv.Status == "AUTHORISED" && inv.AmountDue > 0
}

// ---------------------------------------------------------------------------
// Aging
// ---------------------------------------------------------------------------

// AgingBucket is one overdue band.
type AgingBucket struct {
	Label    string  `json:"bucket"`
	Count    int     `json:"count"`
	TotalDue float64 `json:"total_due"`
}

// AgingReport is the result of ComputeAging.
type AgingReport struct {
	AsOf         string        `json:"as_of"`
	Kind         string        `json:"kind"` // "receivable" | "payable"
	Buckets      []AgingBucket `json:"buckets"`
	TotalDue     float64       `json:"total_due"`
	InvoiceCount int           `json:"invoice_count"`
}

var agingBands = []string{"current", "1-30", "31-60", "61-90", "90+"}

func agingBandFor(daysOverdue int) string {
	switch {
	case daysOverdue <= 0:
		return "current"
	case daysOverdue <= 30:
		return "1-30"
	case daysOverdue <= 60:
		return "31-60"
	case daysOverdue <= 90:
		return "61-90"
	default:
		return "90+"
	}
}

// ComputeAging buckets outstanding invoices by how overdue they are relative to
// asOf. payable=false buckets receivables (ACCREC); payable=true buckets
// payables (ACCPAY). Only AUTHORISED invoices with an amount still due count.
func ComputeAging(invoices []Invoice, asOf time.Time, payable bool) AgingReport {
	wantType := "ACCREC"
	kind := "receivable"
	if payable {
		wantType = "ACCPAY"
		kind = "payable"
	}
	idx := map[string]*AgingBucket{}
	report := AgingReport{AsOf: asOf.UTC().Format("2006-01-02"), Kind: kind}
	// Pre-size the slice so the &report.Buckets[i] pointers stay valid — an
	// append-and-take-pointer loop would reallocate and invalidate earlier
	// pointers, silently dropping their bucket totals.
	report.Buckets = make([]AgingBucket, len(agingBands))
	for i, b := range agingBands {
		report.Buckets[i].Label = b
		idx[b] = &report.Buckets[i]
	}
	for _, inv := range invoices {
		if inv.Type != wantType || !outstanding(inv) {
			continue
		}
		band := "current"
		if due, ok := ParseXeroDate(inv.DueDate); ok {
			band = agingBandFor(daysBetween(due, asOf))
		}
		b := idx[band]
		b.Count++
		b.TotalDue = round2(b.TotalDue + inv.AmountDue)
		report.TotalDue = round2(report.TotalDue + inv.AmountDue)
		report.InvoiceCount++
	}
	return report
}

// ---------------------------------------------------------------------------
// Reconcile — payment vs invoice gap
// ---------------------------------------------------------------------------

// OutstandingInvoice is an approved invoice still owed, annotated with how much
// payment the local store can actually see applied to it.
type OutstandingInvoice struct {
	InvoiceID     string  `json:"invoice_id"`
	InvoiceNumber string  `json:"invoice_number"`
	Contact       string  `json:"contact"`
	AmountDue     float64 `json:"amount_due"`
	AmountPaid    float64 `json:"amount_paid"`
	PaymentsSeen  float64 `json:"payments_seen"` // sum of synced payments referencing this invoice
	AppliedGap    float64 `json:"applied_gap"`   // AmountPaid - PaymentsSeen (nonzero = payments not visible locally)
}

// OrphanPayment is a payment that references an invoice the local store has not
// synced (or has no invoice link at all) — cash we can't tie to a document.
type OrphanPayment struct {
	PaymentID string  `json:"payment_id"`
	Amount    float64 `json:"amount"`
	InvoiceID string  `json:"invoice_id"`
	Contact   string  `json:"contact"`
}

// ReconcileReport is the result of ComputeReconcile.
type ReconcileReport struct {
	OutstandingInvoices []OutstandingInvoice `json:"outstanding_invoices"`
	OrphanPayments      []OrphanPayment      `json:"orphan_payments"`
	TotalOutstanding    float64              `json:"total_outstanding"`
	OutstandingCount    int                  `json:"outstanding_count"`
	OrphanCount         int                  `json:"orphan_payment_count"`
}

// ComputeReconcile surfaces the gap between the invoice ledger and applied cash:
// AUTHORISED receivable invoices still owed (with the payments the store can see
// applied), and payments that reference an invoice not present locally.
func ComputeReconcile(invoices []Invoice, payments []Payment) ReconcileReport {
	invByID := map[string]Invoice{}
	for _, inv := range invoices {
		invByID[inv.InvoiceID] = inv
	}
	paymentsByInvoice := map[string]float64{}
	for _, p := range payments {
		if p.Invoice.InvoiceID != "" {
			paymentsByInvoice[p.Invoice.InvoiceID] = round2(paymentsByInvoice[p.Invoice.InvoiceID] + p.Amount)
		}
	}
	rep := ReconcileReport{
		OutstandingInvoices: []OutstandingInvoice{},
		OrphanPayments:      []OrphanPayment{},
	}
	for _, inv := range invoices {
		if inv.Type != "ACCREC" || !outstanding(inv) {
			continue
		}
		seen := paymentsByInvoice[inv.InvoiceID]
		rep.OutstandingInvoices = append(rep.OutstandingInvoices, OutstandingInvoice{
			InvoiceID:     inv.InvoiceID,
			InvoiceNumber: inv.InvoiceNumber,
			Contact:       inv.Contact.Name,
			AmountDue:     round2(inv.AmountDue),
			AmountPaid:    round2(inv.AmountPaid),
			PaymentsSeen:  seen,
			AppliedGap:    round2(inv.AmountPaid - seen),
		})
		rep.TotalOutstanding = round2(rep.TotalOutstanding + inv.AmountDue)
		rep.OutstandingCount++
	}
	for _, p := range payments {
		if p.Invoice.InvoiceID == "" {
			rep.OrphanPayments = append(rep.OrphanPayments, OrphanPayment{p.PaymentID, round2(p.Amount), "", p.Contact.Name})
			rep.OrphanCount++
			continue
		}
		if _, ok := invByID[p.Invoice.InvoiceID]; !ok {
			rep.OrphanPayments = append(rep.OrphanPayments, OrphanPayment{p.PaymentID, round2(p.Amount), p.Invoice.InvoiceID, p.Contact.Name})
			rep.OrphanCount++
		}
	}
	sort.SliceStable(rep.OutstandingInvoices, func(i, j int) bool {
		return rep.OutstandingInvoices[i].AmountDue > rep.OutstandingInvoices[j].AmountDue
	})
	return rep
}

// ---------------------------------------------------------------------------
// Bank reconciliation prep
// ---------------------------------------------------------------------------

// BankMatchCandidate is a suggested invoice/payment match for an unreconciled
// bank transaction.
type BankMatchCandidate struct {
	Kind   string  `json:"kind"` // "invoice" | "payment"
	ID     string  `json:"id"`
	Amount float64 `json:"amount"`
	Ref    string  `json:"ref"`
}

// UnreconciledTxn is a bank transaction not yet reconciled, with suggested matches.
type UnreconciledTxn struct {
	BankTransactionID string               `json:"bank_transaction_id"`
	Type              string               `json:"type"`
	Total             float64              `json:"total"`
	Contact           string               `json:"contact"`
	Candidates        []BankMatchCandidate `json:"candidates"`
}

// BankReconReport is the result of ComputeBankRecon.
type BankReconReport struct {
	Unreconciled      []UnreconciledTxn `json:"unreconciled"`
	UnreconciledCount int               `json:"unreconciled_count"`
	UnreconciledTotal float64           `json:"unreconciled_total"`
}

func amountsClose(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 0.005
}

// ComputeBankRecon lists unreconciled bank transactions and suggests matches
// against invoices and payments by same contact + equal amount.
func ComputeBankRecon(txns []BankTransaction, invoices []Invoice, payments []Payment) BankReconReport {
	rep := BankReconReport{Unreconciled: []UnreconciledTxn{}}
	for _, t := range txns {
		if t.IsReconciled {
			continue
		}
		ut := UnreconciledTxn{
			BankTransactionID: t.BankTransactionID,
			Type:              t.Type,
			Total:             round2(t.Total),
			Contact:           t.Contact.Name,
			Candidates:        []BankMatchCandidate{},
		}
		for _, inv := range invoices {
			if sameContact(inv.Contact, t.Contact) && amountsClose(inv.Total, t.Total) {
				ut.Candidates = append(ut.Candidates, BankMatchCandidate{"invoice", inv.InvoiceID, round2(inv.Total), inv.InvoiceNumber})
			}
		}
		for _, p := range payments {
			if sameContact(p.Contact, t.Contact) && amountsClose(p.Amount, t.Total) {
				ut.Candidates = append(ut.Candidates, BankMatchCandidate{"payment", p.PaymentID, round2(p.Amount), p.Invoice.InvoiceNumber})
			}
		}
		rep.Unreconciled = append(rep.Unreconciled, ut)
		rep.UnreconciledCount++
		rep.UnreconciledTotal = round2(rep.UnreconciledTotal + t.Total)
	}
	return rep
}

func sameContact(a, b ContactRef) bool {
	if a.ContactID != "" && b.ContactID != "" {
		return a.ContactID == b.ContactID
	}
	return a.Name != "" && strings.EqualFold(a.Name, b.Name)
}

// ---------------------------------------------------------------------------
// GL tie-out
// ---------------------------------------------------------------------------

// TieOutLine reports one control-account tie-out (AR or AP).
type TieOutLine struct {
	Control       string  `json:"control"` // "receivable" | "payable"
	AccountCode   string  `json:"account_code"`
	AccountName   string  `json:"account_name"`
	LedgerBalance float64 `json:"ledger_balance"` // net of GL journal lines on the control account
	InvoiceTotal  float64 `json:"invoice_total"`  // sum of outstanding invoice AmountDue
	Variance      float64 `json:"variance"`       // ledger - invoice
	Ties          bool    `json:"ties"`           // |variance| < 0.01
}

// TieOutReport is the result of ComputeTieOut.
type TieOutReport struct {
	Lines     []TieOutLine `json:"lines"`
	AllTie    bool         `json:"all_tie"`
	Unmatched []string     `json:"unmatched"` // controls with no resolvable account
}

func findControlAccount(accounts []Account, system string, namePart string) (Account, bool) {
	for _, a := range accounts {
		if a.SystemAccount != "" && strings.EqualFold(a.SystemAccount, system) {
			return a, true
		}
	}
	for _, a := range accounts {
		if strings.Contains(strings.ToLower(a.Name), namePart) {
			return a, true
		}
	}
	return Account{}, false
}

func glBalanceForCode(journals []Journal, code string) float64 {
	// An empty code must never match: Xero journal lines with no AccountCode
	// would all sum into the control balance and silently skew the variance.
	if code == "" {
		return 0
	}
	var sum float64
	for _, j := range journals {
		for _, l := range j.JournalLines {
			if l.AccountCode == code {
				sum += l.NetAmount
			}
		}
	}
	return round2(sum)
}

func outstandingTotal(invoices []Invoice, invType string) float64 {
	var sum float64
	for _, inv := range invoices {
		if inv.Type == invType && outstanding(inv) {
			sum += inv.AmountDue
		}
	}
	return round2(sum)
}

// ComputeTieOut sums the immutable GL control accounts and compares them to the
// outstanding invoice totals, reporting the variance. The GL is the independent
// oracle the documents are checked against.
func ComputeTieOut(journals []Journal, invoices []Invoice, accounts []Account) TieOutReport {
	rep := TieOutReport{AllTie: true, Lines: []TieOutLine{}, Unmatched: []string{}}
	specs := []struct {
		control  string
		system   string
		namePart string
		invType  string
	}{
		{"receivable", "DEBTORS", "receivable", "ACCREC"},
		{"payable", "CREDITORS", "payable", "ACCPAY"},
	}
	for _, s := range specs {
		acct, ok := findControlAccount(accounts, s.system, s.namePart)
		if !ok || acct.Code == "" {
			// A control account without a user-facing Code can't be tied:
			// matching on "" would sum every code-less journal line into the
			// control balance. Treat it as unresolved instead.
			rep.Unmatched = append(rep.Unmatched, s.control)
			rep.AllTie = false
			continue
		}
		ledger := glBalanceForCode(journals, acct.Code)
		invTotal := outstandingTotal(invoices, s.invType)
		variance := round2(ledger - invTotal)
		ties := variance < 0.01 && variance > -0.01
		if !ties {
			rep.AllTie = false
		}
		rep.Lines = append(rep.Lines, TieOutLine{
			Control:       s.control,
			AccountCode:   acct.Code,
			AccountName:   acct.Name,
			LedgerBalance: ledger,
			InvoiceTotal:  invTotal,
			Variance:      variance,
			Ties:          ties,
		})
	}
	return rep
}

// ---------------------------------------------------------------------------
// Ledger walk
// ---------------------------------------------------------------------------

// LedgerEntry is one posting to an account with a running balance.
type LedgerEntry struct {
	JournalNumber  int64   `json:"journal_number"`
	Date           string  `json:"date"`
	Description    string  `json:"description"`
	NetAmount      float64 `json:"net_amount"`
	RunningBalance float64 `json:"running_balance"`
}

// LedgerStatement is the result of ComputeLedger for one account code.
type LedgerStatement struct {
	AccountCode  string        `json:"account_code"`
	AccountName  string        `json:"account_name"`
	Entries      []LedgerEntry `json:"entries"`
	FinalBalance float64       `json:"final_balance"`
	EntryCount   int           `json:"entry_count"`
}

// ComputeLedger replays the immutable journal feed for one account code as an
// ordered running-balance statement.
func ComputeLedger(journals []Journal, accountCode string, accounts []Account) LedgerStatement {
	stmt := LedgerStatement{AccountCode: accountCode, Entries: []LedgerEntry{}}
	for _, a := range accounts {
		if a.Code == accountCode {
			stmt.AccountName = a.Name
			break
		}
	}
	type lineWithMeta struct {
		num  int64
		date string
		line JournalLine
	}
	var collected []lineWithMeta
	for _, j := range journals {
		for _, l := range j.JournalLines {
			if l.AccountCode == accountCode {
				if stmt.AccountName == "" && l.AccountName != "" {
					stmt.AccountName = l.AccountName
				}
				collected = append(collected, lineWithMeta{j.JournalNumber, j.JournalDate, l})
			}
		}
	}
	sort.SliceStable(collected, func(i, j int) bool {
		return collected[i].num < collected[j].num
	})
	var running float64
	for _, c := range collected {
		running = round2(running + c.line.NetAmount)
		date := c.date
		if t, ok := ParseXeroDate(c.date); ok {
			date = t.Format("2006-01-02")
		}
		stmt.Entries = append(stmt.Entries, LedgerEntry{
			JournalNumber:  c.num,
			Date:           date,
			Description:    c.line.Description,
			NetAmount:      round2(c.line.NetAmount),
			RunningBalance: running,
		})
	}
	stmt.FinalBalance = running
	stmt.EntryCount = len(stmt.Entries)
	return stmt
}

// ---------------------------------------------------------------------------
// Contact exposure
// ---------------------------------------------------------------------------

// ContactExposure ranks one contact's outstanding receivable exposure.
type ContactExposure struct {
	ContactID    string  `json:"contact_id"`
	Contact      string  `json:"contact"`
	TotalDue     float64 `json:"total_due"`
	OverdueDue   float64 `json:"overdue_due"`
	InvoiceCount int     `json:"invoice_count"`
	OverdueCount int     `json:"overdue_count"`
}

// ComputeExposure ranks contacts by total outstanding receivable amount, with an
// overdue split (relative to asOf). When contactFilter is non-empty, only that
// contact id is returned (the per-contact statement drill-down).
func ComputeExposure(invoices []Invoice, contacts []Contact, asOf time.Time, contactFilter string) []ContactExposure {
	names := map[string]string{}
	for _, c := range contacts {
		names[c.ContactID] = c.Name
	}
	byContact := map[string]*ContactExposure{}
	order := []string{}
	for _, inv := range invoices {
		if inv.Type != "ACCREC" || !outstanding(inv) {
			continue
		}
		cid := inv.Contact.ContactID
		if contactFilter != "" && cid != contactFilter {
			continue
		}
		e, ok := byContact[cid]
		if !ok {
			name := names[cid]
			if name == "" {
				name = inv.Contact.Name
			}
			e = &ContactExposure{ContactID: cid, Contact: name}
			byContact[cid] = e
			order = append(order, cid)
		}
		e.TotalDue = round2(e.TotalDue + inv.AmountDue)
		e.InvoiceCount++
		if due, ok := ParseXeroDate(inv.DueDate); ok && daysBetween(due, asOf) > 0 {
			e.OverdueDue = round2(e.OverdueDue + inv.AmountDue)
			e.OverdueCount++
		}
	}
	out := make([]ContactExposure, 0, len(order))
	for _, cid := range order {
		out = append(out, *byContact[cid])
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].TotalDue > out[j].TotalDue })
	return out
}

// ---------------------------------------------------------------------------
// Snapshot
// ---------------------------------------------------------------------------

// Snapshot is a one-shot offline summary of the org's accounting state.
type Snapshot struct {
	ReceivableOutstanding float64           `json:"receivable_outstanding"`
	PayableOutstanding    float64           `json:"payable_outstanding"`
	OverdueReceivable     int               `json:"overdue_receivable_count"`
	UnreconciledBankTxns  int               `json:"unreconciled_bank_txns"`
	Counts                map[string]int    `json:"counts"`
	SyncStaleness         map[string]string `json:"last_synced"`
}

// ComputeSnapshot composes the org-state numbers from the synced entities.
// counts and lastSynced are supplied by the caller (from the store).
func ComputeSnapshot(invoices []Invoice, bankTxns []BankTransaction, asOf time.Time, counts map[string]int, lastSynced map[string]string) Snapshot {
	s := Snapshot{Counts: counts, SyncStaleness: lastSynced}
	s.ReceivableOutstanding = outstandingTotal(invoices, "ACCREC")
	s.PayableOutstanding = outstandingTotal(invoices, "ACCPAY")
	for _, inv := range invoices {
		if inv.Type == "ACCREC" && outstanding(inv) {
			if due, ok := ParseXeroDate(inv.DueDate); ok && daysBetween(due, asOf) > 0 {
				s.OverdueReceivable++
			}
		}
	}
	for _, t := range bankTxns {
		if !t.IsReconciled {
			s.UnreconciledBankTxns++
		}
	}
	return s
}
