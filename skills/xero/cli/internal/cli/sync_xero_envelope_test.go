package cli

import (
	"encoding/json"
	"testing"
)

// Xero list responses wrap the resource array in a PascalCase key alongside
// metadata siblings (Id, Status, ProviderName, DateTimeUTC). This pins that
// extractPageItems unwraps the resource array for every synced entity rather
// than aborting on the scalar metadata siblings (which would zero out sync).
func TestExtractPageItems_XeroEnvelope(t *testing.T) {
	cases := []struct {
		name string
		body string
		want int
	}{
		{"invoices", `{"Id":"req-1","Status":"OK","ProviderName":"Xero API","DateTimeUTC":"/Date(1700000000000)/","Invoices":[{"InvoiceID":"i1"},{"InvoiceID":"i2"}]}`, 2},
		{"accounts", `{"Id":"req-2","Status":"OK","ProviderName":"Xero API","Accounts":[{"AccountID":"a1"}]}`, 1},
		{"contacts", `{"Id":"req-3","Status":"OK","Contacts":[{"ContactID":"c1"},{"ContactID":"c2"},{"ContactID":"c3"}]}`, 3},
		{"payments", `{"Status":"OK","Payments":[{"PaymentID":"p1"}]}`, 1},
		{"bank-transactions", `{"Status":"OK","BankTransactions":[{"BankTransactionID":"b1"},{"BankTransactionID":"b2"}]}`, 2},
		{"items", `{"Status":"OK","Items":[{"ItemID":"it1"}]}`, 1},
		{"journals", `{"Status":"OK","Journals":[{"JournalID":"j1"},{"JournalID":"j2"}]}`, 2},
	}
	for _, tc := range cases {
		items, _, _ := extractPageItems(json.RawMessage(tc.body), "")
		if len(items) != tc.want {
			t.Errorf("%s: extractPageItems returned %d items, want %d (Xero envelope not unwrapped)", tc.name, len(items), tc.want)
		}
	}
}
