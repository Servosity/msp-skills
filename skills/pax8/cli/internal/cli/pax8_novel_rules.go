// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// pax8 handfix (novel-documented-shapes, Codex r1 4.34.2 wrong-result
// classes): the one reading of each documented Pax8 shape the novel commands
// share, so no command can read it differently.
//
//   - Subscription.status (spec.json#/components/schemas/Subscription): one
//     classifier over the whole documented enum (pax8SubscriptionBills).
//     Active, Activated ("Requires Activation and has been activated"),
//     Trial and PendingCancel (a deprovisioning request is pending: still
//     provisioned, and billed until the cancellation completes) bill;
//     Cancelled, the provisioning states
//     (PendingManual, PendingAutomated, WaitingForDetails, PendingActivation:
//     "Billing services do not begin ... until activation is complete") and
//     Converted (the trial a paid subscription replaced) do not. A status
//     outside the enum, or none, is unclassified: never counted as billing,
//     and reported.
//   - Subscription.billingTerm: Monthly is monthly recurring revenue; Annual,
//     2-Year and 3-Year recur on another cadence and are reported beside MRR
//     as per-term amounts, never summed into it; One-Time, Activation and
//     Trial do not recur. A term outside the enum, or none, is unclassified.
//   - Invoice status (the /invoices status enum in spec.json): one rule
//     (pax8InvoiceOwed). Unpaid, Paid and Carried were billed; Void, Nothing Due
//     and Credited are not owed. A status outside the enum, or none, is
//     unclassified: never summed, and reported with its amount.
//   - currencyCode (Subscription, Invoice): money is never added across it
//     (pax8Currency). A row with no currency is its own group, since it may be
//     any of them.
//   - Timestamps (examples/subscription-history writes "-0700" offsets):
//     pax8ParseTime reads RFC 3339, numeric offsets with or without a colon,
//     no zone (UTC) and dates; a value it cannot read is never inside a
//     window.
//   - Collections: each novel reads a collection under every name sync or a
//     live read stores it as (pax8Collection), so a renamed dependent (4.34.2
//     stores subscription usage summaries as subscriptions_usage_summaries)
//     cannot silently empty a command.

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"pax8-pp-cli/internal/store"
)

// pax8StatusBills is the documented Subscription.status enum, lowercased:
// true when the subscription bills.
var pax8StatusBills = map[string]bool{
	"active":            true,
	"activated":         true,
	"trial":             true,
	"cancelled":         false,
	"pendingcancel":     true,
	"pendingmanual":     false,
	"pendingautomated":  false,
	"waitingfordetails": false,
	"pendingactivation": false,
	"converted":         false,
}

// pax8SubscriptionBills reports whether a subscription with this status bills
// (known is false for a status outside the documented enum).
func pax8SubscriptionBills(status string) (bills, known bool) {
	bills, known = pax8StatusBills[strings.ToLower(strings.TrimSpace(status))]
	return bills, known
}

// pax8InvoiceStatusOwed is the documented /invoices status enum, lowercased: true
// when the invoice was billed, so its total is spend. The spec names these values
// and does not define them, so this is their plain reading: a Carried invoice was
// billed and rolled onto a later one, and a Paid one was billed and settled.
var pax8InvoiceStatusOwed = map[string]bool{
	"unpaid":      true,
	"paid":        true,
	"carried":     true,
	"void":        false,
	"nothing due": false,
	"credited":    false,
}

// pax8InvoiceOwed reports whether an invoice with this status counts as spend
// (known is false for a status outside the documented enum, or none).
func pax8InvoiceOwed(status string) (owed, known bool) {
	owed, known = pax8InvoiceStatusOwed[strings.ToLower(strings.TrimSpace(status))]
	return owed, known
}

// pax8Currency is a row's currencyCode, so that "usd" and "USD" are one currency;
// "" when the row has none.
func pax8Currency(row map[string]any) string {
	return strings.ToUpper(strings.TrimSpace(pax8FieldStr(row, "currencyCode", "currency_code")))
}

// pax8Money writes an amount in a currency, or alone when there is none.
func pax8Money(amount float64, currency string) string {
	if currency == "" {
		return fmt.Sprintf("%.2f", amount)
	}
	return fmt.Sprintf("%.2f %s", amount, currency)
}

// Billing-term classes.
const (
	termMonthly      = "monthly"
	termOtherCadence = "other-cadence"
	termNonRecurring = "non-recurring"
	termUnclassified = "unclassified"
)

var pax8BillingTerms = map[string]string{
	"monthly":    termMonthly,
	"annual":     termOtherCadence,
	"2-year":     termOtherCadence,
	"3-year":     termOtherCadence,
	"one-time":   termNonRecurring,
	"activation": termNonRecurring,
	"trial":      termNonRecurring,
}

// pax8ExpectsInvoiceLine reports a subscription an invoice should carry a
// line for: it bills, and, for the next (draft) invoice, its term is not
// billed on another cadence or once (an Annual subscription is on the invoice
// of its renewal month only). A trial (Trial status or term: "free or
// discounted access") is expected only when it is priced above zero
// (Subscription.price): a discounted trial bills. A term outside the enum is
// expected (fail closed: it is surfaced).
func pax8ExpectsInvoiceLine(sub map[string]any, draft bool) bool {
	status := pax8FieldStr(sub, "status")
	if bills, _ := pax8SubscriptionBills(status); !bills {
		return false
	}
	term := pax8FieldStr(sub, "billingTerm", "billing_term")
	if strings.EqualFold(strings.TrimSpace(status), "trial") || strings.EqualFold(strings.TrimSpace(term), "trial") {
		if price, ok := pax8FieldNum(sub, "price"); !ok || price <= 0 {
			return false
		}
	}
	if draft {
		switch pax8BillingTermClass(term) {
		case termOtherCadence, termNonRecurring:
			return false
		}
	}
	return true
}

// pax8CreditLineTypes are the documented InvoiceItem.type values that credit
// the partner rather than charge (spec.json#/components/schemas/InvoiceItem).
var pax8CreditLineTypes = map[string]bool{
	"rebate":         true,
	"payment-credit": true,
	"service-credit": true,
	"invoice-credit": true,
}

// pax8LineIsCharge reports an invoice line that bills: not a documented
// credit type, and not a negative amount (a prorated refund carries a charge
// type with a negative total). A line with an undocumented type is a charge
// (fail closed: it is surfaced).
func pax8LineIsCharge(line map[string]any) bool {
	if pax8CreditLineTypes[strings.ToLower(strings.TrimSpace(pax8FieldStr(line, "type")))] {
		return false
	}
	if amount, ok := pax8FieldNum(line, "total", "subTotal", "amountDue"); ok && amount < 0 {
		return false
	}
	return true
}

// pax8LineBeforeEnd reports an invoice line (InvoiceItem.startPeriod) whose
// period began before the subscription ended (Subscription.endDate); false
// when either date is missing or unreadable.
func pax8LineBeforeEnd(line, sub map[string]any) bool {
	start, ok := pax8ParseTime(pax8FieldStr(line, "startPeriod", "start_period"))
	if !ok {
		return false
	}
	end, ok := pax8ParseTime(pax8FieldStr(sub, "endDate", "end_date"))
	return ok && start.Before(end)
}

// pax8BillingTermClass classifies a documented Subscription.billingTerm.
func pax8BillingTermClass(term string) string {
	if c, ok := pax8BillingTerms[strings.ToLower(strings.TrimSpace(term))]; ok {
		return c
	}
	return termUnclassified
}

var pax8TimeLayouts = []string{
	time.RFC3339Nano,
	"2006-01-02T15:04:05.999999999Z0700",
	"2006-01-02T15:04:05.999999999Z07",
	"2006-01-02T15:04:05.999999999",
	"2006-01-02 15:04:05.999999999Z07:00",
	"2006-01-02 15:04:05.999999999Z0700",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02",
}

// pax8ParseTime reads every timestamp form Pax8 documents; ok is false for
// anything else.
func pax8ParseTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range pax8TimeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// Collections the novels read, each under every name it is stored as
// (sync's name first).
var (
	pax8Companies      = []string{"companies"}
	pax8Products       = []string{"products"}
	pax8Subscriptions  = []string{"subscriptions"}
	pax8Invoices       = []string{"invoices"}
	pax8InvoiceLines   = []string{"items"}                // GET /invoices/{invoiceId}/items, a sync dependent of invoices
	pax8DraftLines     = []string{"invoices-draft-items"} // GET /invoices/draftItems
	pax8Contacts       = []string{"contacts"}             // a sync dependent of companies
	pax8UsageSummaries = []string{"subscriptions_usage_summaries", "usage-summaries", "usage_summaries"}
	pax8History        = []string{"history"} // filled by the live `subscriptions history` read (write-through), not by sync
)

// pax8NovelSyncedCollections names every collection above that sync writes
// (the class test checks each against sync's resource list).
var pax8NovelSyncedCollections = [][]string{pax8Companies, pax8Products, pax8Subscriptions, pax8Invoices, pax8InvoiceLines, pax8DraftLines, pax8Contacts, pax8UsageSummaries}

// pax8Collection reads every row stored under any of names. A record stored
// under more than one name (a sync dependent and a live write-through of the
// same id) is returned once: the row written last (updated_at, then the rowid,
// which the store advances on every write of a row), so a fresh read of either
// name is never hidden behind an older one, within one second included. Rows
// under a single name are all kept (history snapshots share their subscription's
// id).
func pax8Collection(db *store.Store, names []string) ([]map[string]any, error) {
	if len(names) == 1 {
		return pax8ListObjects(db, names[0])
	}
	type stamped struct {
		row     map[string]any
		updated string
		seq     int64
	}
	out := []stamped{}
	at := map[string]int{}
	for _, name := range names {
		rows, err := db.DB().Query(`SELECT data, COALESCE(updated_at, ''), rowid FROM resources WHERE resource_type = ? ORDER BY rowid`, name)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var data, updated string
			var seq int64
			if err := rows.Scan(&data, &updated, &seq); err != nil {
				_ = rows.Close()
				return nil, err
			}
			var obj map[string]any
			if json.Unmarshal([]byte(data), &obj) != nil {
				continue
			}
			s := stamped{row: obj, updated: updated, seq: seq}
			id := pax8FieldStr(obj, "id")
			if id == "" {
				out = append(out, s)
				continue
			}
			if i, dup := at[id]; dup {
				if prev := out[i]; s.updated > prev.updated || (s.updated == prev.updated && s.seq > prev.seq) {
					out[i] = s
				}
				continue
			}
			at[id] = len(out)
			out = append(out, s)
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return nil, err
		}
	}
	result := make([]map[string]any, 0, len(out))
	for _, s := range out {
		result = append(result, s.row)
	}
	return result, nil
}

// pax8Where keeps the rows whose value under any of keys equals want.
func pax8Where(rows []map[string]any, want string, keys ...string) []map[string]any {
	out := []map[string]any{}
	for _, r := range rows {
		for _, k := range keys {
			if v := pax8FieldStr(r, k); v != "" && v == want {
				out = append(out, r)
				break
			}
		}
	}
	return out
}
