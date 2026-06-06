package store

import "testing"

// Xero entities key on a PascalCase <Entity>ID field. The generic id/ID/name
// fallbacks would either fail (no match) or wrongly key contacts on Name, which
// silently drops or collides synced rows. This pins the resourceIDFieldOverrides
// entries against the exact resourceType strings the sync layer passes
// (defaultSyncResources — note the hyphen in "bank-transactions").
func TestExtractResourceID_Xero(t *testing.T) {
	cases := []struct {
		resourceType string
		obj          map[string]any
		want         string
	}{
		{"accounts", map[string]any{"AccountID": "acc-1", "Name": "Sales"}, "acc-1"},
		{"bank-transactions", map[string]any{"BankTransactionID": "bt-1", "Reference": "x"}, "bt-1"},
		{"contacts", map[string]any{"ContactID": "c-1", "Name": "Acme Ltd"}, "c-1"},
		{"invoices", map[string]any{"InvoiceID": "inv-1", "InvoiceNumber": "INV-001"}, "inv-1"},
		{"items", map[string]any{"ItemID": "it-1", "Code": "WIDGET"}, "it-1"},
		{"journals", map[string]any{"JournalID": "j-1", "JournalNumber": float64(42)}, "j-1"},
		{"payments", map[string]any{"PaymentID": "p-1", "Amount": float64(100)}, "p-1"},
	}
	for _, tc := range cases {
		got := ExtractResourceID(tc.resourceType, tc.obj)
		if got != tc.want {
			t.Errorf("ExtractResourceID(%q) = %q, want %q — sync would drop or mis-key these rows", tc.resourceType, got, tc.want)
		}
	}
}

// Guard against the regression where contacts (which carry a Name) fall through
// to the generic `name` fallback and upsert on Name instead of ContactID.
func TestExtractResourceID_ContactNotKeyedOnName(t *testing.T) {
	obj := map[string]any{"ContactID": "c-99", "Name": "Duplicate Name Co"}
	if got := ExtractResourceID("contacts", obj); got != "c-99" {
		t.Fatalf("contacts must key on ContactID, got %q", got)
	}
}
