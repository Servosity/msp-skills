// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (NOT generated): Codex round-1 classes, each driven with the
// response shape Xero's OpenAPI documents (spec.yaml components/schemas).

package xfin

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func raws(t *testing.T, docs ...string) []json.RawMessage {
	t.Helper()
	out := make([]json.RawMessage, len(docs))
	for i, d := range docs {
		if !json.Valid([]byte(d)) {
			t.Fatalf("fixture %d is not JSON: %s", i, d)
		}
		out[i] = json.RawMessage(d)
	}
	return out
}

func asJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// JournalLine.NetAmount: "a positive value for a debit and negative for a
// credit". Accounts payable carries a credit balance, so an outstanding bill of
// 100 sits on CREDITORS as -100 and must tie.
func TestTieOutPayableControlIsACredit(t *testing.T) {
	accounts := DecodeAccounts(raws(t, `{"AccountID":"ap","Code":"800","Name":"Accounts Payable","SystemAccount":"CREDITORS"}`))
	journals := DecodeJournals(raws(t, `{"JournalID":"j1","JournalNumber":1,"JournalLines":[{"AccountCode":"800","NetAmount":-100.0}]}`))
	invoices := DecodeInvoices(raws(t, `{"InvoiceID":"b1","Type":"ACCPAY","Status":"AUTHORISED","AmountDue":100.0,"CurrencyCode":"NZD","CurrencyRate":1.0}`))
	for _, l := range ComputeTieOut(journals, invoices, accounts).Lines {
		if l.Control == "payable" && !l.Ties {
			t.Fatalf("payable control holding the bill as a credit does not tie: %s", asJSON(t, l))
		}
	}
}

// Invoice.CurrencyCode / CurrencyRate: a multi-currency org holds invoices in
// several currencies. No total may add amounts in different currencies.
func TestTotalsNeverAddAcrossCurrencies(t *testing.T) {
	asOf := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)
	invoices := DecodeInvoices(raws(t,
		`{"InvoiceID":"u","Type":"ACCREC","Status":"AUTHORISED","AmountDue":100.0,"Total":100.0,"CurrencyCode":"USD","CurrencyRate":0.65,"DueDate":"/Date(1704067200000+0000)/","Contact":{"ContactID":"c1","Name":"Acme"}}`,
		`{"InvoiceID":"e","Type":"ACCREC","Status":"AUTHORISED","AmountDue":100.0,"Total":100.0,"CurrencyCode":"EUR","CurrencyRate":0.55,"DueDate":"/Date(1704067200000+0000)/","Contact":{"ContactID":"c1","Name":"Acme"}}`))
	txns := DecodeBankTransactions(raws(t,
		`{"BankTransactionID":"b1","Type":"RECEIVE","Status":"AUTHORISED","Total":100.0,"IsReconciled":false,"CurrencyCode":"USD"}`,
		`{"BankTransactionID":"b2","Type":"RECEIVE","Status":"AUTHORISED","Total":100.0,"IsReconciled":false,"CurrencyCode":"EUR"}`))
	outputs := map[string]any{
		"aging":      ComputeAging(invoices, asOf, false),
		"reconcile":  ComputeReconcile(invoices, nil),
		"exposure":   ComputeExposure(invoices, nil, asOf, ""),
		"snapshot":   ComputeSnapshot(invoices, txns, asOf, map[string]int{}, map[string]string{}),
		"bank-recon": ComputeBankRecon(txns, nil, nil),
	}
	for name, v := range outputs {
		out := asJSON(t, v)
		if strings.Contains(out, ":200") {
			t.Errorf("%s adds USD 100 and EUR 100 into 200: %s", name, out)
		}
		if !strings.Contains(out, "USD") || !strings.Contains(out, "EUR") {
			t.Errorf("%s does not say which currency its amounts are in: %s", name, out)
		}
	}

	// The general ledger is kept in base currency: foreign-currency invoices
	// cannot be summed against it, so a raw sum that happens to equal the GL
	// must not be reported as tied.
	accounts := DecodeAccounts(raws(t, `{"AccountID":"ar","Code":"610","Name":"Accounts Receivable","SystemAccount":"DEBTORS"}`))
	journals := DecodeJournals(raws(t, `{"JournalID":"j1","JournalNumber":1,"JournalLines":[{"AccountCode":"610","NetAmount":200.0}]}`))
	for _, l := range ComputeTieOut(journals, invoices, accounts).Lines {
		if l.Control == "receivable" && l.Ties {
			t.Fatalf("tie-out reports USD+EUR invoices as tying to the base-currency ledger: %s", asJSON(t, l))
		}
	}
}

// Payment.Status is AUTHORISED or DELETED; BankTransaction.Status is
// AUTHORISED, DELETED or VOIDED. A cancelled record is not cash.
func TestCancelledRecordsAreNotCash(t *testing.T) {
	invoices := DecodeInvoices(raws(t, `{"InvoiceID":"i1","Type":"ACCREC","Status":"AUTHORISED","AmountDue":100.0,"AmountPaid":0.0,"Total":100.0,"CurrencyCode":"NZD","Contact":{"ContactID":"c1","Name":"Acme"}}`))
	payments := DecodePayments(raws(t,
		`{"PaymentID":"p1","Status":"DELETED","Amount":100.0,"Invoice":{"InvoiceID":"i1","Contact":{"ContactID":"c1","Name":"Acme"}}}`,
		`{"PaymentID":"p2","Status":"DELETED","Amount":40.0,"Invoice":{"InvoiceID":"gone"}}`))
	rec := ComputeReconcile(invoices, payments)
	if len(rec.OutstandingInvoices) != 1 || rec.OutstandingInvoices[0].PaymentsSeen != 0 || rec.OutstandingInvoices[0].AppliedGap != 0 {
		t.Fatalf("reconcile counts a DELETED payment as applied cash: %s", asJSON(t, rec))
	}
	if rec.OrphanCount != 0 {
		t.Fatalf("reconcile reports a DELETED payment as orphan cash: %s", asJSON(t, rec))
	}

	txns := DecodeBankTransactions(raws(t,
		`{"BankTransactionID":"d","Type":"RECEIVE","Status":"DELETED","Total":100.0,"IsReconciled":false,"Contact":{"ContactID":"c1"}}`,
		`{"BankTransactionID":"v","Type":"RECEIVE","Status":"VOIDED","Total":100.0,"IsReconciled":false,"Contact":{"ContactID":"c1"}}`))
	if br := ComputeBankRecon(txns, invoices, payments); br.UnreconciledCount != 0 {
		t.Fatalf("bank-recon lists cancelled bank transactions as unreconciled: %s", asJSON(t, br))
	}
	if s := ComputeSnapshot(invoices, txns, time.Now(), map[string]int{}, map[string]string{}); s.UnreconciledBankTxns != 0 {
		t.Fatalf("snapshot counts cancelled bank transactions as unreconciled: %d", s.UnreconciledBankTxns)
	}

	live := DecodeBankTransactions(raws(t, `{"BankTransactionID":"b","Type":"RECEIVE","Status":"AUTHORISED","Total":100.0,"IsReconciled":false,"Contact":{"ContactID":"c1"}}`))
	for _, c := range ComputeBankRecon(live, nil, payments).Unreconciled[0].Candidates {
		t.Fatalf("bank-recon suggests a DELETED payment as a match: %+v", c)
	}
}

// Payment carries no top-level Contact: GET /Payments nests it under the
// document the payment applies to (Invoice, CreditNote, Prepayment, Overpayment).
func TestPaymentContactComesFromItsDocument(t *testing.T) {
	txns := DecodeBankTransactions(raws(t, `{"BankTransactionID":"b","Type":"RECEIVE","Status":"AUTHORISED","Total":100.0,"IsReconciled":false,"Contact":{"ContactID":"c1","Name":"Acme"}}`))
	payments := DecodePayments(raws(t,
		`{"PaymentID":"p1","Status":"AUTHORISED","Amount":100.0,"Invoice":{"InvoiceID":"i1","InvoiceNumber":"INV-1","Contact":{"ContactID":"c1","Name":"Acme"}}}`,
		`{"PaymentID":"p2","Status":"AUTHORISED","Amount":100.0,"CreditNote":{"CreditNoteID":"cn1","Contact":{"ContactID":"c1","Name":"Acme"}}}`))
	got := ComputeBankRecon(txns, nil, payments).Unreconciled[0].Candidates
	if len(got) != 2 {
		t.Fatalf("payment candidates for contact c1 = %+v, want p1 and p2", got)
	}
}

// Payment.BankAmount is in the currency of the payment's bank account, which
// the payment does not name. A payment is only a candidate when the document
// it applies to is in the transaction's currency (Codex round 2).
func TestBankReconNeverMatchesAcrossCurrencies(t *testing.T) {
	txns := DecodeBankTransactions(raws(t, `{"BankTransactionID":"b","Type":"RECEIVE","Status":"AUTHORISED","Total":100.0,"IsReconciled":false,"CurrencyCode":"EUR","Contact":{"ContactID":"c1"}}`))
	payments := DecodePayments(raws(t,
		`{"PaymentID":"usd","Status":"AUTHORISED","Amount":100.0,"BankAmount":100.0,"Invoice":{"InvoiceID":"i1","CurrencyCode":"USD","Contact":{"ContactID":"c1"}}}`,
		`{"PaymentID":"eur","Status":"AUTHORISED","Amount":100.0,"Invoice":{"InvoiceID":"i2","CurrencyCode":"EUR","Contact":{"ContactID":"c1"}}}`))
	got := ComputeBankRecon(txns, nil, payments).Unreconciled[0].Candidates
	if len(got) != 1 || got[0].ID != "eur" {
		t.Fatalf("EUR transaction candidates = %+v, want only the EUR payment", got)
	}
}
