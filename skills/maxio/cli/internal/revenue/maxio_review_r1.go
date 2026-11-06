// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package revenue

// Codex review round 1 (maxio 4.34.2 reprint): the shared readings every
// revenue command uses, one per wrong-result class, so no command keeps its
// own narrower copy.
//
//   - current-states-narrowed-to-active: CurrentStates is the one list of
//     subscription states that are still a customer relationship (spec.json
//     SubscriptionState: the live and the problem states), bound into every
//     query as CurrentStatesArg.
//   - unbilled-invoice-status-counted: BilledInvoiceStatuses is the one list of
//     invoice statuses that are billed revenue (InvoiceStatus: issued and not
//     voided or canceled; an unknown status is not counted), bound as
//     BilledInvoiceStatusesArg.
//   - currency-dropped: Money renders cents in the site's currency.
//   - movement-category-ignored: Bucket is the one classifier for a movement
//     category, at the movement and at the line-item level.
//   - movement-identity-incomplete: movementIdentity keys a movement on every
//     member Maxio sends except its display renderings.
//   - incomplete-backfill-read-as-complete: the movement backfill records
//     whether the stored history reaches the first page.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// CurrentStates are the SubscriptionState values (spec.json) of a
// subscription that is still a customer relationship: the Live States
// (active, assessing, pending, trialing, paused) and the Problem States
// (past_due, soft_failure, unpaid). The End of Life states (canceled,
// expired, failed_to_create, on_hold, suspended, trial_ended) and any state
// not listed are not current.
var CurrentStates = []string{"active", "assessing", "pending", "trialing", "paused", "past_due", "soft_failure", "unpaid"}

// ProblemStates are the documented Problem States: payment has failed or the
// subscription is in dunning.
var ProblemStates = []string{"past_due", "soft_failure", "unpaid"}

// BilledInvoiceStatuses are the InvoiceStatus values (spec.json) of an
// invoice that bills its amount: issued (open, pending, processing) or paid.
// draft, voided, canceled and any status not listed bill nothing.
var BilledInvoiceStatuses = []string{"open", "paid", "pending", "processing"}

func jsonList(values []string) string {
	b, err := json.Marshal(values)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// CurrentStatesArg is CurrentStates as a JSON array, the one bound argument of
//
//	LOWER(COALESCE(<state>,'')) IN (SELECT value FROM json_each(?))
//
// so every query reads the same list without building SQL text.
func CurrentStatesArg() string { return jsonList(CurrentStates) }

// BilledInvoiceStatusesArg is BilledInvoiceStatuses as a JSON array, bound
// the same way to an invoice's status.
func BilledInvoiceStatusesArg() string { return jsonList(BilledInvoiceStatuses) }

// SupersededParentsArg is the UIDs of the consolidated parents whose child
// segment is stored, as a JSON array, the one bound argument of
//
//	COALESCE(json_extract(inv.data,'$.uid'),'') NOT IN (SELECT value FROM json_each(?))
//
// on every billed read (Codex r2 class consolidated-invoice-double-counted).
// InvoiceConsolidationLevel: a parent has no lines of its own and totals that
// aggregate its segments, so the child already bills that amount. A parent with
// no stored child is the only record of the bill and counts; a parent with only
// some children stored counts only those children (a capped sync, not a shape
// Maxio answers on its own).
func SupersededParentsArg(ctx context.Context, db *sql.DB) (string, error) {
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT json_extract(data,'$.parent_invoice_uid') FROM resources
		WHERE resource_type='invoices-json' AND COALESCE(json_extract(data,'$.parent_invoice_uid'),'') <> ''`)
	if err != nil {
		return "", fmt.Errorf("reading consolidated invoices: %w", err)
	}
	defer rows.Close()
	uids := []string{}
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			return "", fmt.Errorf("reading consolidated invoices: %w", err)
		}
		uids = append(uids, uid)
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("reading consolidated invoices: %w", err)
	}
	return jsonList(uids), nil
}

// An amount in another currency (Invoice.currency, ISO 4217 per invoice) is
// never added to a site-currency total (Codex r1 class currency-dropped): an
// invoice query reads
//
//	(COALESCE(<currency>,'') = '' OR UPPER(<currency>) = UPPER(?))
//
// bound to SiteCurrency.

// IsProblemState reports whether a subscription state is a Problem State.
func IsProblemState(state string) bool {
	s := strings.ToLower(strings.TrimSpace(state))
	for _, p := range ProblemStates {
		if s == p {
			return true
		}
	}
	return false
}

// Bucket classifies a Maxio MRR movement category with its signed amount, at
// the movement level and at the line-item level (MRRMovement.category) alike.
func Bucket(category string, amountCents int64) string {
	return classifyBucket(category, amountCents)
}

// Money renders an integer cents value in an ISO 4217 currency: "$1,234.56"
// for USD, "1,234.56 EUR" for any other code, and the bare amount when the
// currency is unknown (never a dollar sign it cannot vouch for).
func Money(c int64, currency string) string {
	cur := strings.ToUpper(strings.TrimSpace(currency))
	if cur == "USD" {
		return Cents(c)
	}
	neg := c < 0
	if neg {
		c = -c
	}
	s := fmt.Sprintf("%s.%02d", withThousands(strconv.FormatInt(c/100, 10)), c%100)
	if neg {
		s = "-" + s
	}
	if cur == "" {
		return s
	}
	return s + " " + cur
}

// SiteCurrency is the site's currency as the store knows it: the latest MRR
// snapshot's (/mrr.json currency), else the latest synced invoice's, else "".
func SiteCurrency(ctx context.Context, db *sql.DB) string {
	var cur sql.NullString
	_ = db.QueryRowContext(ctx, `SELECT currency FROM rev_site_snapshots
		WHERE COALESCE(currency,'') <> '' ORDER BY snapshot_at DESC LIMIT 1`).Scan(&cur)
	if strings.TrimSpace(cur.String) != "" {
		return strings.TrimSpace(cur.String)
	}
	cur = sql.NullString{}
	_ = db.QueryRowContext(ctx, `SELECT json_extract(data,'$.currency') FROM resources
		WHERE resource_type='invoices-json' AND COALESCE(json_extract(data,'$.currency'),'') <> ''
		ORDER BY json_extract(data,'$.issue_date') DESC LIMIT 1`).Scan(&cur)
	return strings.TrimSpace(cur.String)
}

// movementIdentity keys one movement on every member Maxio sent, at every
// depth, except its display text (any *_formatted rendering, subscriber_name,
// the description sentence and a line item's name, which follow a customer or
// product rename): subscription_id, timestamp, category, amount, breakouts
// and every line item's ids, quantities and MRR movements distinguish
// movements, and a member Maxio adds later distinguishes them too.
// occurrence numbers records identical in every member across one walk of the
// log, so identical records either side of a page boundary stay distinct.
func movementIdentity(raw json.RawMessage, occurrence int) string {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		v = string(raw)
	}
	b, err := json.Marshal(withoutDisplayMembers(v))
	if err != nil {
		b = raw
	}
	h := sha256.Sum256(append(b, []byte("#"+strconv.Itoa(occurrence))...))
	return "m2:" + hex.EncodeToString(h[:16])
}

func withoutDisplayMembers(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, c := range t {
			lk := strings.ToLower(k)
			if strings.HasSuffix(lk, "_formatted") || lk == "subscriber_name" || lk == "description" || lk == "name" {
				continue
			}
			out[k] = withoutDisplayMembers(c)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, c := range t {
			out[i] = withoutDisplayMembers(c)
		}
		return out
	}
	return v
}

// movementsBackfillKey records whether the stored movement history is the
// whole log: "complete" once a walk reached page 1 (or caught up with a
// history that was complete), "partial" while a walk runs or after one was
// capped or failed.
const movementsBackfillKey = "movements_backfill"

func ensureStateTable(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS rev_sync_state (key TEXT PRIMARY KEY, value TEXT)`)
	return err
}

func syncState(ctx context.Context, db *sql.DB, key string) string {
	var v sql.NullString
	_ = db.QueryRowContext(ctx, `SELECT value FROM rev_sync_state WHERE key = ?`, key).Scan(&v)
	return v.String
}

func setSyncState(ctx context.Context, db *sql.DB, key, value string) error {
	_, err := db.ExecContext(ctx, `INSERT OR REPLACE INTO rev_sync_state (key, value) VALUES (?,?)`, key, value)
	return err
}

// MovementsComplete reports whether the stored movement history was
// backfilled to its first page by mrr sync.
func MovementsComplete(ctx context.Context, db *sql.DB) bool {
	if err := EnsureSchema(db); err != nil {
		return false
	}
	return syncState(ctx, db, movementsBackfillKey) == "complete"
}
