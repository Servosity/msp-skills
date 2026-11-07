// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Codex r1 (pax8 4.34.2 reprint) wrong-result classes in the novel commands,
// each driven through the real command against a store seeded the way sync
// stores rows (upsertResourceBatch, with the parent fields a dependent sync
// adds), on documented Pax8 shapes (spec.json#/components/schemas/...):
//   - billing-term-ignored (Subscription.billingTerm): MRR is the monthly
//     billing terms only; other terms are reported beside it, never summed in.
//   - documented-status-misread (Subscription.status): one classifier over
//     the whole documented enum; Activated bills.
//   - novel-reads-unsynced-collection: overage and company show read the
//     collections sync writes (usage summaries are a subscription dependent).
//   - documented-timestamp-rejected (examples/subscription-history uses
//     "-0700" offsets): one shared parser; an unreadable date is never in a
//     window.
//   - company-level-join (InvoiceItem.subscriptionId): reconcile joins
//     invoice lines to subscriptions by subscription id.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pax8-pp-cli/internal/cliutil/testenv"
	"pax8-pp-cli/internal/store"
)

// seedAsSync stores rows the way sync does; a dependent's rows carry the
// parent fields sync adds (parent_id and <parent>_id).
func seedAsSync(t *testing.T, db *store.Store, resource, parentTable, parentID string, rows ...string) {
	t.Helper()
	items := make([]json.RawMessage, 0, len(rows))
	for _, r := range rows {
		if parentTable != "" {
			var obj map[string]any
			if err := json.Unmarshal([]byte(r), &obj); err != nil {
				t.Fatalf("fixture %s: %v", r, err)
			}
			obj["parent_id"] = parentID
			if _, ok := obj[parentFKColumnName(parentTable)]; !ok {
				obj[parentFKColumnName(parentTable)] = parentID
			}
			b, _ := json.Marshal(obj)
			r = string(b)
		}
		items = append(items, json.RawMessage(r))
	}
	if stored, _, _, err := upsertResourceBatch(db, resource, items); err != nil || stored != len(items) {
		t.Fatalf("seeding %s: stored %d of %d: %v", resource, stored, len(items), err)
	}
}

func novelStore(t *testing.T) (string, *store.Store) {
	t.Helper()
	home := testenv.Isolate(t)
	path := filepath.Join(home, "novel.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return path, db
}

func runNovelJSON(t *testing.T, dbPath string, out any, args ...string) {
	t.Helper()
	root := RootCmd()
	var buf, errb bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&errb)
	root.SetArgs(append(args, "--json", "--db", dbPath))
	if err := root.Execute(); err != nil {
		t.Fatalf("%v: %v\n%s", args, err, errb.String())
	}
	if err := json.Unmarshal(buf.Bytes(), out); err != nil {
		t.Fatalf("%v: output is not JSON: %v\n%s", args, err, buf.String())
	}
}

const (
	cid1 = "11111111-1111-4111-8111-111111111111"
	pid1 = "22222222-2222-4222-8222-222222222222"
	sidA = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	sidB = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	sidC = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
)

type mrrOut struct {
	Totals struct {
		MRR                 float64 `json:"mrr"`
		Margin              float64 `json:"margin"`
		ActiveSubscriptions int     `json:"activeSubscriptions"`
	} `json:"totals"`
}

func TestMRRCountsOnlyMonthlyBillingTerms(t *testing.T) {
	path, db := novelStore(t)
	seedAsSync(t, db, "subscriptions", "", "",
		`{"id":"`+sidA+`","companyId":"`+cid1+`","productId":"`+pid1+`","status":"Active","billingTerm":"Monthly","quantity":2,"price":10,"partnerCost":6,"currencyCode":"USD"}`,
		`{"id":"`+sidB+`","companyId":"`+cid1+`","productId":"`+pid1+`","status":"Active","billingTerm":"One-Time","quantity":1,"price":1200,"partnerCost":600,"currencyCode":"USD"}`,
		`{"id":"`+sidC+`","companyId":"`+cid1+`","productId":"`+pid1+`","status":"Active","billingTerm":"Activation","quantity":1,"price":50,"partnerCost":40,"currencyCode":"USD"}`)
	var got mrrOut
	runNovelJSON(t, path, &got, "mrr")
	if got.Totals.MRR != 20 || got.Totals.Margin != 8 || got.Totals.ActiveSubscriptions != 1 {
		t.Fatalf("mrr = %+v, want MRR 20, margin 8 from the one Monthly subscription", got.Totals)
	}
}

func TestDocumentedBillingStatusesCount(t *testing.T) {
	path, db := novelStore(t)
	seedAsSync(t, db, "subscriptions", "", "",
		`{"id":"`+sidA+`","companyId":"`+cid1+`","productId":"`+pid1+`","status":"Activated","billingTerm":"Monthly","quantity":2,"price":12,"partnerCost":8,"currencyCode":"USD"}`)
	var got mrrOut
	runNovelJSON(t, path, &got, "mrr")
	if got.Totals.MRR != 24 || got.Totals.Margin != 8 || got.Totals.ActiveSubscriptions != 1 {
		t.Fatalf("mrr = %+v, want MRR 24, margin 8, 1 subscription (status Activated bills)", got.Totals)
	}
	var rec reconcileResult
	runNovelJSON(t, path, &rec, "reconcile")
	if len(rec.ActiveWithoutInvoice) != 1 || rec.ActiveWithoutInvoice[0].SubscriptionID != sidA {
		t.Fatalf("reconcile did not treat the Activated subscription as active: %+v", rec)
	}
}

func TestNovelsReadTheCollectionsSyncWrites(t *testing.T) {
	path, db := novelStore(t)
	seedAsSync(t, db, "companies", "", "", `{"id":"`+cid1+`","name":"Acme"}`)
	seedAsSync(t, db, "subscriptions", "", "",
		`{"id":"`+sidA+`","companyId":"`+cid1+`","productId":"`+pid1+`","status":"Active","billingTerm":"Monthly","quantity":1,"price":5}`)
	seedAsSync(t, db, "subscriptions_usage_summaries", "subscriptions", sidA,
		`{"id":"`+sidB+`","companyId":"`+cid1+`","productId":"`+pid1+`","vendorName":"Microsoft","currentCharges":10,"currencyCode":"USD"}`,
		`{"id":"`+sidC+`","companyId":"`+cid1+`","productId":"`+pid1+`","vendorName":"Microsoft","currentCharges":90,"currencyCode":"USD"}`)
	seedAsSync(t, db, "contacts", "companies", cid1, `{"id":"33333333-3333-4333-8333-333333333333","firstName":"Ann","email":"ann@example.com"}`)
	var flagged []overageRow
	runNovelJSON(t, path, &flagged, "overage")
	if len(flagged) != 1 || flagged[0].UsageSummaryID != sidC {
		t.Fatalf("overage = %+v, want the 90-charge summary flagged (1.8x the product average)", flagged)
	}
	var show companyShowResult
	runNovelJSON(t, path, &show, "company", "show", cid1)
	if len(show.UsageSummaries) != 2 || len(show.Contacts) != 1 || len(show.Subscriptions) != 1 {
		t.Fatalf("company show = %d usage summaries, %d contacts, %d subscriptions; want 2, 1, 1", len(show.UsageSummaries), len(show.Contacts), len(show.Subscriptions))
	}
	if show.Invoices == nil {
		t.Fatalf("company show printed invoices as null on a company with none")
	}
}

func TestSinceReadsDocumentedTimestampOffsets(t *testing.T) {
	path, db := novelStore(t)
	recent := time.Now().Add(-24 * time.Hour).Format("2006-01-02T15:04:05.000-0700")
	seedAsSync(t, db, "subscriptions", "", "",
		`{"id":"`+sidA+`","companyId":"`+cid1+`","productId":"`+pid1+`","status":"Active","createdDate":"`+recent+`","updatedDate":"`+recent+`"}`)
	if err := db.UpsertHistory(json.RawMessage(`{"id":"` + sidB + `","subscriptions_id":"` + sidB + `","subscriptionId":"` + sidB + `","status":"Active","createdDate":"not a date"}`)); err != nil {
		t.Fatalf("seed history: %v", err)
	}
	var changes []subChange
	runNovelJSON(t, path, &changes, "since", "7d")
	var sawRecent bool
	for _, c := range changes {
		if c.SubscriptionID == sidA {
			sawRecent = true
		}
		if c.SubscriptionID == sidB {
			t.Errorf("a history row with an unreadable date was reported inside the window: %+v", c)
		}
	}
	if !sawRecent {
		t.Fatalf("since 7d omitted a subscription created %s: %+v", recent, changes)
	}
}

func TestReconcileJoinsInvoiceLinesBySubscription(t *testing.T) {
	for _, mode := range []string{"draft", "posted"} {
		t.Run(mode, func(t *testing.T) {
			path, db := novelStore(t)
			seedAsSync(t, db, "companies", "", "", `{"id":"`+cid1+`","name":"Acme"}`)
			seedAsSync(t, db, "subscriptions", "", "",
				`{"id":"`+sidA+`","companyId":"`+cid1+`","productId":"`+pid1+`","status":"Active","billingTerm":"Monthly","quantity":1,"price":5}`,
				`{"id":"`+sidB+`","companyId":"`+cid1+`","productId":"`+pid1+`","status":"Cancelled","billingTerm":"Monthly","quantity":1,"price":5}`)
			line := `{"id":"44444444-4444-4444-8444-444444444444","type":"subscription","companyId":"` + cid1 + `","subscriptionId":"` + sidB + `","productId":"` + pid1 + `","total":50,"currencyCode":"USD"}`
			args := []string{"reconcile"}
			if mode == "draft" {
				seedAsSync(t, db, "invoices-draft-items", "", "", line)
				args = append(args, "--draft")
			} else {
				seedAsSync(t, db, "invoices", "", "", `{"id":"55555555-5555-4555-8555-555555555555","companyId":"`+cid1+`","total":50,"invoiceDate":"2026-10-01"}`)
				seedAsSync(t, db, "items", "invoices", "55555555-5555-4555-8555-555555555555", line)
			}
			var got reconcileResult
			runNovelJSON(t, path, &got, args...)
			var billedB, unbilledA bool
			for _, f := range got.BilledWithoutActiveSubscription {
				billedB = billedB || f.SubscriptionID == sidB
			}
			for _, f := range got.ActiveWithoutInvoice {
				unbilledA = unbilledA || f.SubscriptionID == sidA
			}
			if !billedB || !unbilledA {
				t.Fatalf("reconcile %s: billed-after-cancel B found=%v, unbilled active A found=%v: %+v", mode, billedB, unbilledA, got)
			}
		})
	}
}

// TestNovelJSONIsNeverNull guards the empty-array contract on company show's
// child lists too (Codex r1 P2: pax8QueryObjects returned nil).
func TestNovelJSONIsNeverNull(t *testing.T) {
	path, db := novelStore(t)
	seedAsSync(t, db, "companies", "", "", `{"id":"`+cid1+`","name":"Acme"}`)
	root := RootCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"company", "show", cid1, "--json", "--db", path})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "null") {
		t.Fatalf("company show printed null:\n%s", buf.String())
	}
}

// TestEveryNovelCollectionIsOneSyncWrites: every collection a novel reads
// that sync fills names a resource sync writes (its first name), and the
// history collection is the resource the live history read writes through.
func TestEveryNovelCollectionIsOneSyncWrites(t *testing.T) {
	known := map[string]bool{}
	for _, n := range knownSyncResourceNames() {
		known[n] = true
	}
	for _, names := range pax8NovelSyncedCollections {
		if !known[names[0]] {
			t.Errorf("novels read %q, which sync never writes (sync writes %v)", names[0], knownSyncResourceNames())
		}
	}
	src, err := os.ReadFile("subscriptions_history_find-subscription-by-subscription-id.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), `"live", "`+pax8History[0]+`"`) {
		t.Errorf("the live history read no longer writes through as %q", pax8History[0])
	}
}

// TestReconcileReadsLinePeriodsAgainstTheSubscriptionEnd: posted lines hold
// the whole billing history, so a line for a period before a cancelled
// subscription ended is history, not leakage; a line for a period after it
// is (InvoiceItem.startPeriod, Subscription.endDate).
func TestReconcileReadsLinePeriodsAgainstTheSubscriptionEnd(t *testing.T) {
	path, db := novelStore(t)
	seedAsSync(t, db, "subscriptions", "", "",
		`{"id":"`+sidA+`","companyId":"`+cid1+`","productId":"`+pid1+`","status":"Cancelled","billingTerm":"Monthly","endDate":"2026-09-15T00:00:00Z","quantity":1,"price":5}`,
		`{"id":"`+sidB+`","companyId":"`+cid1+`","productId":"`+pid1+`","status":"Cancelled","billingTerm":"Monthly","endDate":"2026-09-15T00:00:00Z","quantity":1,"price":5}`)
	seedAsSync(t, db, "invoices", "", "", `{"id":"55555555-5555-4555-8555-555555555555","companyId":"`+cid1+`","total":10}`)
	seedAsSync(t, db, "items", "invoices", "55555555-5555-4555-8555-555555555555",
		`{"id":"44444444-4444-4444-8444-444444444441","type":"subscription","companyId":"`+cid1+`","subscriptionId":"`+sidA+`","startPeriod":"2026-09-01","endPeriod":"2026-09-30","total":5}`,
		`{"id":"44444444-4444-4444-8444-444444444442","type":"subscription","companyId":"`+cid1+`","subscriptionId":"`+sidB+`","startPeriod":"2026-10-01","endPeriod":"2026-10-31","total":5}`)
	var got reconcileResult
	runNovelJSON(t, path, &got, "reconcile")
	if len(got.BilledWithoutActiveSubscription) != 1 || got.BilledWithoutActiveSubscription[0].SubscriptionID != sidB {
		t.Fatalf("want only B (billed for October after ending in September): %+v", got.BilledWithoutActiveSubscription)
	}
}

// TestReconcileExpectsLinesOnlyWhereAnInvoiceCarriesThem: a trial, an Annual
// subscription on the next (draft) invoice, and a line for a subscription
// whose cancellation is pending are not mismatches.
func TestReconcileExpectsLinesOnlyWhereAnInvoiceCarriesThem(t *testing.T) {
	path, db := novelStore(t)
	seedAsSync(t, db, "subscriptions", "", "",
		`{"id":"`+sidA+`","companyId":"`+cid1+`","productId":"`+pid1+`","status":"Trial","billingTerm":"Monthly","quantity":1,"price":0}`,
		`{"id":"`+sidB+`","companyId":"`+cid1+`","productId":"`+pid1+`","status":"Active","billingTerm":"Annual","quantity":1,"price":120}`,
		`{"id":"`+sidC+`","companyId":"`+cid1+`","productId":"`+pid1+`","status":"PendingCancel","billingTerm":"Monthly","quantity":1,"price":5}`)
	seedAsSync(t, db, "invoices-draft-items", "", "",
		`{"id":"44444444-4444-4444-8444-444444444444","type":"subscription","companyId":"`+cid1+`","subscriptionId":"`+sidC+`","total":5}`)
	var got reconcileResult
	runNovelJSON(t, path, &got, "reconcile", "--draft")
	if len(got.BilledWithoutActiveSubscription) != 0 || len(got.ActiveWithoutInvoice) != 0 {
		t.Fatalf("reconcile --draft reported mismatches for a trial, an Annual subscription and a pending cancellation: %+v", got)
	}
}
