// Package xfin holds the pure-logic Xero financial analytics that power the
// transcendence commands (aging, reconcile, bank-recon, tie-out, ledger,
// exposure, snapshot, since). Everything here is computed locally from synced
// data — no API calls, no Reports endpoint. The CLI commands open the local
// store, decode rows into these types, and call the Compute* functions.
package xfin

import "encoding/json"

// ContactRef is the embedded contact reference Xero attaches to invoices,
// payments, and bank transactions.
type ContactRef struct {
	ContactID string `json:"ContactID"`
	Name      string `json:"Name"`
}

// InvoiceRef is the minimal invoice reference embedded in a payment.
type InvoiceRef struct {
	InvoiceID     string `json:"InvoiceID"`
	InvoiceNumber string `json:"InvoiceNumber"`
}

// Invoice is the subset of Xero invoice fields the analytics need.
type Invoice struct {
	InvoiceID      string     `json:"InvoiceID"`
	InvoiceNumber  string     `json:"InvoiceNumber"`
	Type           string     `json:"Type"`   // ACCREC (receivable) | ACCPAY (payable)
	Status         string     `json:"Status"` // DRAFT|SUBMITTED|AUTHORISED|PAID|VOIDED|DELETED
	AmountDue      float64    `json:"AmountDue"`
	AmountPaid     float64    `json:"AmountPaid"`
	Total          float64    `json:"Total"`
	Date           string     `json:"Date"`
	DueDate        string     `json:"DueDate"`
	UpdatedDateUTC string     `json:"UpdatedDateUTC"`
	Contact        ContactRef `json:"Contact"`
}

// Payment is the subset of Xero payment fields the analytics need.
type Payment struct {
	PaymentID      string     `json:"PaymentID"`
	Amount         float64    `json:"Amount"`
	Date           string     `json:"Date"`
	Status         string     `json:"Status"`
	UpdatedDateUTC string     `json:"UpdatedDateUTC"`
	Invoice        InvoiceRef `json:"Invoice"`
	Contact        ContactRef `json:"Contact"`
}

// BankTransaction is the subset of Xero bank-transaction fields the analytics need.
type BankTransaction struct {
	BankTransactionID string     `json:"BankTransactionID"`
	Type              string     `json:"Type"` // SPEND | RECEIVE (and *-OVERPAYMENT/-PREPAYMENT)
	Status            string     `json:"Status"`
	Total             float64    `json:"Total"`
	IsReconciled      bool       `json:"IsReconciled"`
	Date              string     `json:"Date"`
	Reference         string     `json:"Reference"`
	UpdatedDateUTC    string     `json:"UpdatedDateUTC"`
	Contact           ContactRef `json:"Contact"`
}

// JournalLine is one debit/credit line of a general-ledger journal.
type JournalLine struct {
	AccountID   string  `json:"AccountID"`
	AccountCode string  `json:"AccountCode"`
	AccountName string  `json:"AccountName"`
	NetAmount   float64 `json:"NetAmount"`
	GrossAmount float64 `json:"GrossAmount"`
	TaxAmount   float64 `json:"TaxAmount"`
	Description string  `json:"Description"`
}

// Journal is one entry in the immutable general-ledger feed.
type Journal struct {
	JournalID      string        `json:"JournalID"`
	JournalNumber  int64         `json:"JournalNumber"`
	JournalDate    string        `json:"JournalDate"`
	Reference      string        `json:"Reference"`
	UpdatedDateUTC string        `json:"UpdatedDateUTC"`
	JournalLines   []JournalLine `json:"JournalLines"`
}

// Account is the subset of chart-of-accounts fields the analytics need.
// SystemAccount is Xero's reliable control-account marker ("DEBTORS" for the
// accounts-receivable control account, "CREDITORS" for accounts-payable); the
// tie-out falls back to a Name heuristic when it is absent.
type Account struct {
	AccountID     string `json:"AccountID"`
	Code          string `json:"Code"`
	Name          string `json:"Name"`
	Type          string `json:"Type"`
	SystemAccount string `json:"SystemAccount"`
}

// Contact is the subset of contact fields the analytics need.
type Contact struct {
	ContactID string `json:"ContactID"`
	Name      string `json:"Name"`
}

// decodeAll unmarshals a slice of raw JSON entity rows into a typed slice,
// skipping rows that fail to decode (defensive — a single malformed row must
// not abort an analytics run).
func decodeAll[T any](rows []json.RawMessage) []T {
	out := make([]T, 0, len(rows))
	for _, r := range rows {
		var v T
		if err := json.Unmarshal(r, &v); err != nil {
			continue
		}
		out = append(out, v)
	}
	return out
}

// DecodeInvoices, DecodePayments, etc. are the typed entry points the CLI uses
// after loading raw rows from the store.
func DecodeInvoices(rows []json.RawMessage) []Invoice { return decodeAll[Invoice](rows) }
func DecodePayments(rows []json.RawMessage) []Payment { return decodeAll[Payment](rows) }
func DecodeBankTransactions(rows []json.RawMessage) []BankTransaction {
	return decodeAll[BankTransaction](rows)
}
func DecodeJournals(rows []json.RawMessage) []Journal { return decodeAll[Journal](rows) }
func DecodeAccounts(rows []json.RawMessage) []Account { return decodeAll[Account](rows) }
func DecodeContacts(rows []json.RawMessage) []Contact { return decodeAll[Contact](rows) }
