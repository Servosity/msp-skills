// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

// aws-billing handfix (cost-cache-answers-only-its-request; Codex r1 classes
// cache-answer-not-the-request and failure-read-as-an-empty-result): the local
// store answers a read only with what AWS answered for the same AWS account
// (the scope: the account the credentials resolve to, sts:GetCallerIdentity),
// and, for Cost Explorer, for the same dimension pair, granularity and time
// period. The cache records what each answer covered:
//
//   - awsbilling_cost_coverage: one row per (scope, kind, granularity,
//     period start, period end) of a complete GetCostAndUsage answer (every
//     page read, every row stored), a period with no spend included, and
//     whether AWS marked it estimated (not final: reused only by an explicit
//     --data-source local read, never by an automatic one). A
//     period's cost rows carry its key as their storage-ID prefix
//     (CostRowPrefix) and are replaced whole when the period is answered
//     again, so a group AWS no longer returns does not stay.
//   - awsbilling_inventory_scans: one row per (scope, region) inventory scan,
//     complete or not (a failed CloudWatch or S3 call, a row not stored), so a
//     stored incomplete scan is never read as "no waste".
//   - awsbilling_account_lists: the scopes whose complete Organizations
//     account list is stored (rows prefixed scope|).
//
// Codex r2 class cache-answer-not-the-request (fixed after the stop rule): an
// answer is replaced in ONE transaction (its old rows and record removed, its
// rows stored, its record written) and read in ONE transaction (the record
// checked and the rows read together), so a reader sees a whole old answer or
// a whole new one, never the rows of half a replacement under a record, and a
// replacement that fails leaves the old answer as it was. A row whose storage
// ID the store would withhold, or that lacks its answer's key, makes the
// answer not storable (Codex r2 P2: its amount would vanish under a record).
//   - awsbilling_scope_labels: the account an AWS profile name resolved to,
//     for --data-source local (which reads without asking AWS).

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

var awsBillingScopeDDL = []string{
	`CREATE TABLE IF NOT EXISTS awsbilling_cost_coverage (
		scope TEXT NOT NULL, kind TEXT NOT NULL, granularity TEXT NOT NULL,
		period_start TEXT NOT NULL, period_end TEXT NOT NULL, estimated INTEGER NOT NULL, answered_at TEXT NOT NULL,
		PRIMARY KEY (scope, kind, granularity, period_start, period_end))`,
	`CREATE TABLE IF NOT EXISTS awsbilling_inventory_scans (
		scope TEXT NOT NULL, region TEXT NOT NULL, complete INTEGER NOT NULL, detail TEXT NOT NULL, scanned_at TEXT NOT NULL,
		PRIMARY KEY (scope, region))`,
	`CREATE TABLE IF NOT EXISTS awsbilling_account_lists (scope TEXT PRIMARY KEY, listed_at TEXT NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS awsbilling_scope_labels (label TEXT PRIMARY KEY, scope TEXT NOT NULL, seen_at TEXT NOT NULL)`,
}

func (s *Store) ensureScopeTables() error {
	for _, q := range awsBillingScopeDDL {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("creating the cache scope tables: %w", err)
		}
	}
	return nil
}

// CostBucket is one Cost Explorer time period, [Start, End); Estimated as
// Cost Explorer marked it.
type CostBucket struct {
	Start     string
	End       string
	Estimated bool
}

// CostRowPrefix is the storage-ID prefix of the cost rows of one answered
// period: kind|scope|granularity|start|end|.
func CostRowPrefix(kind, scope, granularity string, b CostBucket) string {
	return strings.Join([]string{kind, scope, granularity, b.Start, b.End, ""}, "|") // Estimated is not part of the key
}

// AccountRowPrefix is the storage-ID prefix of one scope's account rows.
func AccountRowPrefix(scope string) string { return scope + "|" }

func scopeNow() string { return time.Now().UTC().Format(time.RFC3339) }

// ReplaceCostBuckets stores one complete Cost Explorer answer for scope, kind
// and granularity in one transaction: the stored rows and records of every
// period it answered for are removed, rows are stored (each storage ID must
// carry one of those periods' CostRowPrefix, as written), and the periods are
// recorded as covered. Any failure rolls all of it back.
func (s *Store) ReplaceCostBuckets(scope, kind, granularity string, buckets []CostBucket, rows []json.RawMessage) error {
	if scope == "" || kind == "" || granularity == "" {
		return fmt.Errorf("storing a cost answer: no scope, kind or granularity")
	}
	if err := s.ensureScopeTables(); err != nil {
		return err
	}
	prefixes := make([]string, 0, len(buckets))
	for _, b := range buckets {
		prefixes = append(prefixes, CostRowPrefix(kind, scope, granularity, b))
	}
	return s.writeTx(func(tx *sql.Tx) error {
		for i, b := range buckets {
			if _, err := tx.Exec(`DELETE FROM awsbilling_cost_coverage WHERE scope = ? AND kind = ? AND granularity = ? AND period_start = ? AND period_end = ?`,
				scope, kind, granularity, b.Start, b.End); err != nil {
				return err
			}
			if err := deleteRowsWithPrefix(tx, "costs", prefixes[i]); err != nil {
				return err
			}
		}
		for _, r := range rows {
			if err := s.upsertKeyedTx(tx, "costs", r, prefixes, s.upsertCostsTx); err != nil {
				return err
			}
		}
		for _, b := range buckets {
			est := 0
			if b.Estimated {
				est = 1
			}
			if _, err := tx.Exec(`INSERT OR REPLACE INTO awsbilling_cost_coverage (scope, kind, granularity, period_start, period_end, estimated, answered_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
				scope, kind, granularity, b.Start, b.End, est, scopeNow()); err != nil {
				return err
			}
		}
		return nil
	})
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// RecordInventoryScan records the outcome of an inventory scan.
func (s *Store) RecordInventoryScan(scope, region string, complete bool, detail string) error {
	if scope == "" {
		return fmt.Errorf("recording an inventory scan: no account")
	}
	if err := s.ensureScopeTables(); err != nil {
		return err
	}
	return s.writeTx(func(tx *sql.Tx) error { return recordInventoryScanTx(tx, scope, region, complete, detail) })
}

func recordInventoryScanTx(tx *sql.Tx, scope, region string, complete bool, detail string) error {
	_, err := tx.Exec(`INSERT OR REPLACE INTO awsbilling_inventory_scans (scope, region, complete, detail, scanned_at) VALUES (?, ?, ?, ?, ?)`,
		scope, region, boolInt(complete), detail, scopeNow())
	return err
}

// CostRow is one stored cost line.
type CostRow struct {
	PeriodStart, PeriodEnd, Granularity, AccountID, AccountName, Service, Region, UsageType, Unit string
	AmountUSD                                                                                     float64
}

// CostRowsIfCovered reads, in one transaction, whether every period is
// covered for scope, kind and granularity (an estimated one only when
// allowEstimated) and, when it is, the periods' rows.
func (s *Store) CostRowsIfCovered(ctx context.Context, scope, kind, granularity string, buckets []CostBucket, allowEstimated bool) ([]CostRow, bool, error) {
	if len(buckets) == 0 {
		return nil, false, nil
	}
	if err := s.ensureScopeTables(); err != nil {
		return nil, false, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()
	var out []CostRow
	for _, b := range buckets {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM awsbilling_cost_coverage WHERE scope = ? AND kind = ? AND granularity = ? AND period_start = ? AND period_end = ? AND (estimated = 0 OR ? = 1)`,
			scope, kind, granularity, b.Start, b.End, boolInt(allowEstimated)).Scan(&n); err != nil {
			return nil, false, err
		}
		if n == 0 {
			return nil, false, nil
		}
		prefix := CostRowPrefix(kind, scope, granularity, b)
		rows, err := tx.QueryContext(ctx, `SELECT COALESCE(period_start, ''), COALESCE(period_end, ''), COALESCE(granularity, ''), COALESCE(account_id, ''),
			COALESCE(account_name, ''), COALESCE(service, ''), COALESCE(region, ''), COALESCE(usage_type, ''), COALESCE(amount_usd, 0), COALESCE(unit, '')
			FROM "costs" WHERE substr("id", 1, ?) = ?`, len(prefix), prefix)
		if err != nil {
			return nil, false, err
		}
		for rows.Next() {
			var r CostRow
			if err := rows.Scan(&r.PeriodStart, &r.PeriodEnd, &r.Granularity, &r.AccountID, &r.AccountName, &r.Service, &r.Region, &r.UsageType, &r.AmountUSD, &r.Unit); err != nil {
				_ = rows.Close()
				return nil, false, err
			}
			out = append(out, r)
		}
		if err := rows.Close(); err != nil {
			return nil, false, err
		}
	}
	return out, true, tx.Commit()
}

// InventoryRow is one stored inventory resource.
type InventoryRow struct {
	ResourceType, ResourceID, AccountID, Region, State, WasteReason, Attrs string
	MonthlyCostUSD, MonthlyWasteUSD                                        float64
}

// InventoryIfScanned reads, in one transaction, scope's inventory scan of
// region and, when it was complete, the account's rows of that region (and
// its S3 buckets, listed account-wide). ok is false when there is no such
// scan; incomplete names an incomplete one. A read always names its region
// (r1b-inventory-region-resolved): a live scan covers one region, so an
// answer from every scanned region is never the answer to a read.
func (s *Store) InventoryIfScanned(ctx context.Context, scope, region, typeFilter, account string, wasteOnly bool) (rows []InventoryRow, ok bool, incomplete string, err error) {
	if region == "" {
		return nil, false, "", fmt.Errorf("an inventory read names no AWS region: a scan covers one region")
	}
	if err := s.ensureScopeTables(); err != nil {
		return nil, false, "", err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, "", err
	}
	defer tx.Rollback()
	scans, err := tx.QueryContext(ctx, `SELECT region, complete, detail, scanned_at FROM awsbilling_inventory_scans WHERE scope = ? ORDER BY region`, scope)
	if err != nil {
		return nil, false, "", err
	}
	var regions []string
	for scans.Next() {
		var r, detail, at string
		var c int
		if err := scans.Scan(&r, &c, &detail, &at); err != nil {
			_ = scans.Close()
			return nil, false, "", err
		}
		if r != region {
			continue
		}
		if c != 1 && incomplete == "" {
			incomplete = fmt.Sprintf("the last inventory scan of AWS account %s in %s (%s) was incomplete: %s", scope, r, at, detail)
		}
		regions = append(regions, r)
	}
	if err := scans.Close(); err != nil {
		return nil, false, "", err
	}
	if incomplete != "" || len(regions) == 0 {
		return nil, false, incomplete, nil
	}
	inRegion := map[string]bool{}
	for _, r := range regions {
		inRegion[r] = true
	}
	// Constant SQL: the filters are parameters, the scanned regions are
	// applied as the rows are read (S3 buckets are listed account-wide).
	res, err := tx.QueryContext(ctx, `SELECT COALESCE(resource_type, ''), COALESCE(resource_id, ''), COALESCE(account_id, ''), COALESCE(region, ''), COALESCE(state, ''),
	      COALESCE(monthly_cost_usd, 0), COALESCE(monthly_waste_usd, 0), COALESCE(waste_reason, ''), COALESCE(attrs, '')
	      FROM "inventory" WHERE account_id = ? AND (? = '' OR resource_type = ?) AND (? = '' OR account_id = ?) AND (? = 0 OR monthly_waste_usd > 0)
	      ORDER BY monthly_waste_usd DESC, monthly_cost_usd DESC`,
		scope, typeFilter, typeFilter, account, account, boolInt(wasteOnly))
	if err != nil {
		return nil, false, "", err
	}
	for res.Next() {
		var r InventoryRow
		if err := res.Scan(&r.ResourceType, &r.ResourceID, &r.AccountID, &r.Region, &r.State, &r.MonthlyCostUSD, &r.MonthlyWasteUSD, &r.WasteReason, &r.Attrs); err != nil {
			_ = res.Close()
			return nil, false, "", err
		}
		if r.ResourceType == "s3" || inRegion[r.Region] {
			rows = append(rows, r)
		}
	}
	if err := res.Close(); err != nil {
		return nil, false, "", err
	}
	return rows, true, "", tx.Commit()
}

// AccountRow is one stored Organizations account.
type AccountRow struct{ AccountID, Name, Status, Email string }

// AccountListIfRecorded reads, in one transaction, whether scope's complete
// account list is stored and, when it is, its rows (one account ID, or all).
func (s *Store) AccountListIfRecorded(ctx context.Context, scope, accountID string) ([]AccountRow, bool, error) {
	if err := s.ensureScopeTables(); err != nil {
		return nil, false, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM awsbilling_account_lists WHERE scope = ?`, scope).Scan(&n); err != nil {
		return nil, false, err
	}
	if n == 0 {
		return nil, false, nil
	}
	prefix := AccountRowPrefix(scope)
	rows, err := tx.QueryContext(ctx, `SELECT COALESCE(account_id, ''), COALESCE(name, ''), COALESCE(status, ''), COALESCE(email, '')
		FROM "accounts" WHERE substr("id", 1, ?) = ? AND (? = '' OR account_id = ?) ORDER BY account_id`, len(prefix), prefix, accountID, accountID)
	if err != nil {
		return nil, false, err
	}
	var out []AccountRow
	for rows.Next() {
		var a AccountRow
		if err := rows.Scan(&a.AccountID, &a.Name, &a.Status, &a.Email); err != nil {
			_ = rows.Close()
			return nil, false, err
		}
		out = append(out, a)
	}
	if err := rows.Close(); err != nil {
		return nil, false, err
	}
	return out, true, tx.Commit()
}

// ReplaceAccountList stores the complete Organizations account list of scope
// (each row ID prefixed AccountRowPrefix(scope)) in place of the stored one,
// and records it, in one transaction.
func (s *Store) ReplaceAccountList(scope string, rows []json.RawMessage) error {
	if scope == "" {
		return fmt.Errorf("storing an account list: no account")
	}
	if err := s.ensureScopeTables(); err != nil {
		return err
	}
	prefixes := []string{AccountRowPrefix(scope)}
	return s.writeTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`DELETE FROM awsbilling_account_lists WHERE scope = ?`, scope); err != nil {
			return err
		}
		if err := deleteRowsWithPrefix(tx, "accounts", prefixes[0]); err != nil {
			return err
		}
		for _, r := range rows {
			if err := s.upsertKeyedTx(tx, "accounts", r, prefixes, s.upsertAccountsTx); err != nil {
				return err
			}
		}
		_, err := tx.Exec(`INSERT OR REPLACE INTO awsbilling_account_lists (scope, listed_at) VALUES (?, ?)`, scope, scopeNow())
		return err
	})
}

// ReplaceInventoryScan stores one inventory scan of scope and region in one
// transaction: its rows, the prune of the rows of that account and region it
// did not return (only when complete), and the scan's record. When a row
// cannot be stored, nothing of the scan is, and the scan is recorded as
// incomplete. It returns how many rows it stored.
func (s *Store) ReplaceInventoryScan(scope, region string, rows []json.RawMessage, seen map[string]bool, complete bool, detail string) (int, error) {
	if scope == "" {
		return 0, fmt.Errorf("storing an inventory scan: no account")
	}
	if err := s.ensureScopeTables(); err != nil {
		return 0, err
	}
	err := s.writeTx(func(tx *sql.Tx) error {
		for _, r := range rows {
			if err := s.upsertKeyedTx(tx, "inventory", r, nil, s.upsertInventoryTx); err != nil {
				return err
			}
		}
		if complete {
			if _, err := pruneInventoryTx(tx, scope, region, seen); err != nil {
				return fmt.Errorf("pruning resources no longer in AWS: %w", err)
			}
		}
		return recordInventoryScanTx(tx, scope, region, complete, detail)
	})
	if err != nil {
		_ = s.RecordInventoryScan(scope, region, false, "storing the scan: "+err.Error())
		return 0, err
	}
	return len(rows), nil
}

// errRowNotStorable marks a row the store cannot keep as part of a whole
// answer: it lacks the answer's key. A row that repeats a held credential is
// refused by the credential kit's SQLite driver (credsql), which fails the
// whole replacement the same way.
var errRowNotStorable = fmt.Errorf("a row of the answer cannot be stored under its own key (it lacks the answer's key); the answer is not kept")

// upsertKeyedTx stores one row inside tx as the generated Upsert<Type> does
// (storage ID, merge with the stored row, generic resources row, typed row),
// refusing a row that carries none of prefixes (when given).
func (s *Store) upsertKeyedTx(tx *sql.Tx, resourceType string, data json.RawMessage, prefixes []string, typed func(*sql.Tx, string, map[string]any, json.RawMessage) error) error {
	obj, err := DecodeJSONObject(data)
	if err != nil {
		return fmt.Errorf("unmarshaling %s: %w", resourceType, err)
	}
	id := ResolveStorageID(resourceType, obj)
	if id == "" {
		return fmt.Errorf("missing id for %s", resourceType)
	}
	storageID := resourceStorageID(resourceType, id, obj)
	if len(prefixes) > 0 {
		keyed := false
		for _, p := range prefixes {
			keyed = keyed || strings.HasPrefix(storageID, p)
		}
		if !keyed {
			return errRowNotStorable
		}
	}
	data = s.mergeIncomingResourceData(tx, resourceType, storageID, data)
	if merged, err := DecodeJSONObject(data); err == nil {
		obj = merged
	}
	if err := s.upsertGenericResourceTx(tx, resourceType, storageID, data); err != nil {
		return err
	}
	return typed(tx, storageID, obj, data)
}

// RecordScopeLabel records the account an AWS profile name resolved to.
func (s *Store) RecordScopeLabel(label, scope string) error {
	if label == "" || scope == "" {
		return nil
	}
	if err := s.ensureScopeTables(); err != nil {
		return err
	}
	return s.writeTx(func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT OR REPLACE INTO awsbilling_scope_labels (label, scope, seen_at) VALUES (?, ?, ?)`, label, scope, scopeNow())
		return err
	})
}

// ScopeForLabel returns the account an AWS profile name last resolved to.
func (s *Store) ScopeForLabel(label string) (string, bool, error) {
	if err := s.ensureScopeTables(); err != nil {
		return "", false, err
	}
	var scope string
	err := s.db.QueryRow(`SELECT scope FROM awsbilling_scope_labels WHERE label = ?`, label).Scan(&scope)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	return scope, err == nil, err
}

// DataScopes returns every account the store holds answered data for.
func (s *Store) DataScopes() ([]string, error) {
	if err := s.ensureScopeTables(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT scope FROM awsbilling_cost_coverage UNION SELECT scope FROM awsbilling_inventory_scans
		UNION SELECT scope FROM awsbilling_account_lists ORDER BY 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var sc string
		if err := rows.Scan(&sc); err != nil {
			return nil, err
		}
		out = append(out, sc)
	}
	return out, rows.Err()
}

// writeTx runs fn in one write transaction under the store's write lock.
func (s *Store) writeTx(fn func(tx *sql.Tx) error) error {
	s.lockForWrite()
	defer s.unlockAfterWrite()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// prefixedIDsSQL and deleteRowSQL are the statements of the typed tables
// whose rows are replaced whole (constant SQL: no table name is spliced in).
var prefixedIDsSQL = map[string]string{
	"costs":    `SELECT "id" FROM "costs" WHERE substr("id", 1, ?) = ?`,
	"accounts": `SELECT "id" FROM "accounts" WHERE substr("id", 1, ?) = ?`,
}

var deleteRowSQL = map[string]string{
	"costs":     `DELETE FROM "costs" WHERE "id" = ?`,
	"accounts":  `DELETE FROM "accounts" WHERE "id" = ?`,
	"inventory": `DELETE FROM "inventory" WHERE "id" = ?`,
}

// deleteRowsWithPrefix removes the rows of a typed table whose storage ID
// starts with prefix, with their generic resources rows and search index
// entries.
func deleteRowsWithPrefix(tx *sql.Tx, table, prefix string) error {
	q, ok := prefixedIDsSQL[table]
	if !ok {
		return fmt.Errorf("no prefixed delete for table %q", table)
	}
	rows, err := tx.Query(q, len(prefix), prefix)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	return deleteResourceRows(tx, table, ids)
}

// deleteResourceRows removes ids from a typed table, the generic resources
// table and the search index.
func deleteResourceRows(tx *sql.Tx, table string, ids []string) error {
	del, ok := deleteRowSQL[table]
	if !ok {
		return fmt.Errorf("no row delete for table %q", table)
	}
	var fts int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'resources_fts'`).Scan(&fts); err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := tx.Exec(del, id); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM resources WHERE resource_type = ? AND id = ?`, table, id); err != nil {
			return err
		}
		if fts > 0 {
			if _, err := tx.Exec(`DELETE FROM resources_fts WHERE rowid = ?`, ftsRowID(table, id)); err != nil {
				return err
			}
		}
	}
	return nil
}
