// Package revenue is the hand-authored recurring-revenue engine for the Maxio
// CLI. It owns its own SQLite tables (separate from the generated `resources`
// mirror) because the data it stores has no stable primary key the generic
// store can extract (MRR movements) or needs point-in-time history the generic
// upsert-by-id store would overwrite (per-subscription MRR snapshots).
//
// It reads three live Maxio Advanced Billing "Insights" surfaces — site MRR
// (/mrr.json), per-subscription MRR (/subscriptions_mrr.json), and the MRR
// movement log (/mrr_movements.json) — and persists them so the novel
// commands (mrr waterfall/client, retention, cohort, triage, usage-drivers)
// can compute offline. The movement log is server-deprecated upstream; syncing
// it into the local store is precisely the durability play.
package revenue

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"maxio-pp-cli/internal/client"
)

// Movement buckets — Maxio's five canonical MRR movement categories plus an
// "imported" bucket for the initial book-of-business import rows (which carry
// category "imported" and would otherwise pollute New Business).
const (
	BucketNew          = "new"
	BucketExpansion    = "expansion"
	BucketContraction  = "contraction"
	BucketChurn        = "churn"
	BucketReactivation = "reactivation"
	BucketImported     = "imported"
)

// Buckets is the canonical display order for waterfall output.
var Buckets = []string{BucketNew, BucketExpansion, BucketContraction, BucketChurn, BucketReactivation, BucketImported}

// EnsureSchema creates the revenue tables if they do not exist. Idempotent;
// called lazily by every revenue command before it reads or writes.
func EnsureSchema(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS rev_site_snapshots (
			snapshot_at TEXT PRIMARY KEY,
			at_time     TEXT,
			mrr_cents   INTEGER,
			currency    TEXT,
			active_subs INTEGER,
			total_subs  INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS rev_sub_mrr_snapshots (
			snapshot_at     TEXT,
			subscription_id TEXT,
			mrr_cents       INTEGER,
			plan_cents      INTEGER,
			usage_cents     INTEGER,
			PRIMARY KEY (snapshot_at, subscription_id)
		)`,
		`CREATE TABLE IF NOT EXISTS rev_mrr_movements (
			mvmt_key    TEXT PRIMARY KEY,
			ts          TEXT,
			amount_cents INTEGER,
			category    TEXT,
			bucket      TEXT,
			plan_cents  INTEGER,
			usage_cents INTEGER,
			description TEXT,
			line_items  TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_rev_mvmt_ts ON rev_mrr_movements(ts)`,
		`CREATE INDEX IF NOT EXISTS idx_rev_sub_snap_at ON rev_sub_mrr_snapshots(snapshot_at)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("revenue schema: %w", err)
		}
	}
	return nil
}

// classifyBucket maps a Maxio movement category + signed amount to one of the
// five canonical buckets (plus "imported"). Categories that can be either
// expansion or contraction (plan changes, component/usage changes, proration)
// are split by the sign of the amount.
func classifyBucket(category string, amountCents int64) string {
	c := strings.ToLower(strings.TrimSpace(category))
	switch c {
	case "imported":
		return BucketImported
	case "signup", "signup_with_trial", "trial_conversion", "new_business":
		return BucketNew
	case "reactivation":
		return BucketReactivation
	case "cancellation", "delayed_cancellation", "expiration", "churn":
		return BucketChurn
	}
	// Sign-driven categories: plan_change, component_allocation_change,
	// metered_usage, proration, downgrade, upgrade, and anything else.
	if amountCents < 0 {
		return BucketContraction
	}
	return BucketExpansion
}

// --- live response shapes -------------------------------------------------

// siteMRRResp mirrors /mrr.json, whose payload is wrapped in an "mrr" object:
// {"mrr": {"amount_in_cents": ..., "at_time": ..., "currency": ...}}.
type siteMRRResp struct {
	MRR struct {
		AmountInCents int64  `json:"amount_in_cents"`
		AtTime        string `json:"at_time"`
		Currency      string `json:"currency"`
	} `json:"mrr"`
}

type statsResp struct {
	Stats struct {
		TotalActiveSubscriptions int64 `json:"total_active_subscriptions"`
		TotalSubscriptions       int64 `json:"total_subscriptions"`
	} `json:"stats"`
}

type subMRRResp struct {
	SubscriptionsMRR []struct {
		SubscriptionID json.Number `json:"subscription_id"`
		MRRAmount      int64       `json:"mrr_amount_in_cents"`
		Breakouts      struct {
			Plan  int64 `json:"plan_amount_in_cents"`
			Usage int64 `json:"usage_amount_in_cents"`
		} `json:"breakouts"`
	} `json:"subscriptions_mrr"`
}

type movementsResp struct {
	MRR struct {
		TotalPages int           `json:"total_pages"`
		Movements  []rawMovement `json:"movements"`
	} `json:"mrr"`
}

type rawMovement struct {
	Timestamp     string `json:"timestamp"`
	AmountInCents int64  `json:"amount_in_cents"`
	Category      string `json:"category"`
	Description   string `json:"description"`
	Breakouts     struct {
		Plan  int64 `json:"plan_amount_in_cents"`
		Usage int64 `json:"usage_amount_in_cents"`
	} `json:"breakouts"`
	LineItems json.RawMessage `json:"line_items"`
}

// SyncOptions bound the work the snapshot pass does.
type SyncOptions struct {
	// MaxMovementPages caps movement backfill pages (per_page=200). 0 = unlimited.
	MaxMovementPages int
	// SnapshotAt is the timestamp key for this snapshot. Caller supplies it
	// (the CLI passes a stamp) so the package stays free of time.Now() for
	// deterministic tests.
	SnapshotAt string
}

// SyncResult reports what a snapshot pass persisted.
type SyncResult struct {
	SnapshotAt        string `json:"snapshot_at"`
	SiteMRRCents      int64  `json:"site_mrr_cents"`
	ActiveSubs        int64  `json:"active_subs"`
	SubSnapshotRows   int    `json:"sub_snapshot_rows"`
	MovementsFetched  int    `json:"movements_fetched"`
	MovementsInserted int    `json:"movements_inserted"`
	MovementPagesRead int    `json:"movement_pages_read"`
}

// Sync fetches the live Insights surfaces and persists a snapshot + incremental
// movement backfill. It is the single write path for the revenue tables.
func Sync(ctx context.Context, c *client.Client, db *sql.DB, opts SyncOptions) (*SyncResult, error) {
	if err := EnsureSchema(db); err != nil {
		return nil, err
	}
	res := &SyncResult{SnapshotAt: opts.SnapshotAt}

	// 1. Site MRR + stats -> one site snapshot row.
	var site siteMRRResp
	if data, err := c.Get(ctx, "/mrr.json", nil); err == nil {
		_ = json.Unmarshal(data, &site)
	} else {
		return nil, fmt.Errorf("fetching site MRR: %w", err)
	}
	var stats statsResp
	if data, err := c.Get(ctx, "/stats.json", nil); err == nil {
		_ = json.Unmarshal(data, &stats)
	}
	res.SiteMRRCents = site.MRR.AmountInCents
	res.ActiveSubs = stats.Stats.TotalActiveSubscriptions
	if _, err := db.ExecContext(ctx,
		`INSERT OR REPLACE INTO rev_site_snapshots (snapshot_at, at_time, mrr_cents, currency, active_subs, total_subs)
		 VALUES (?,?,?,?,?,?)`,
		opts.SnapshotAt, site.MRR.AtTime, site.MRR.AmountInCents, site.MRR.Currency,
		stats.Stats.TotalActiveSubscriptions, stats.Stats.TotalSubscriptions); err != nil {
		return nil, fmt.Errorf("writing site snapshot: %w", err)
	}

	// 2. Per-subscription MRR -> per-sub snapshot rows (paginated).
	for page := 1; ; page++ {
		data, err := c.Get(ctx, "/subscriptions_mrr.json", map[string]string{
			"page": strconv.Itoa(page), "per_page": "200",
		})
		if err != nil {
			return nil, fmt.Errorf("fetching per-subscription MRR page %d: %w", page, err)
		}
		var sm subMRRResp
		if err := json.Unmarshal(data, &sm); err != nil {
			return nil, fmt.Errorf("parsing per-subscription MRR page %d: %w", page, err)
		}
		if len(sm.SubscriptionsMRR) == 0 {
			break
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return nil, err
		}
		for _, s := range sm.SubscriptionsMRR {
			if _, err := tx.ExecContext(ctx,
				`INSERT OR REPLACE INTO rev_sub_mrr_snapshots (snapshot_at, subscription_id, mrr_cents, plan_cents, usage_cents)
				 VALUES (?,?,?,?,?)`,
				opts.SnapshotAt, s.SubscriptionID.String(), s.MRRAmount, s.Breakouts.Plan, s.Breakouts.Usage); err != nil {
				_ = tx.Rollback()
				return nil, fmt.Errorf("writing sub snapshot: %w", err)
			}
			res.SubSnapshotRows++
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		if len(sm.SubscriptionsMRR) < 200 {
			break
		}
	}

	// 3. MRR movements -> backfill from the most-recent pages BACKWARD. The
	//    endpoint is oldest-first and caps per_page at 10 (so a busy site has
	//    hundreds of pages); "recent" movements live on the LAST pages. We read
	//    total_pages from page 1, then walk from the last page down so a capped
	//    sync gets the most-relevant recent movements and an incremental re-sync
	//    stops as soon as it reaches an already-stored page. This is the
	//    deprecated surface we copy locally so it survives upstream sunset.
	firstData, ferr := c.Get(ctx, "/mrr_movements.json", map[string]string{"page": "1", "per_page": "200"})
	if ferr == nil {
		var first movementsResp
		if json.Unmarshal(firstData, &first) == nil {
			total := first.MRR.TotalPages
			if total < 1 {
				total = 1
			}
			for page := total; page >= 1; page-- {
				if opts.MaxMovementPages > 0 && res.MovementPagesRead >= opts.MaxMovementPages {
					break
				}
				data, gerr := c.Get(ctx, "/mrr_movements.json", map[string]string{
					"page": strconv.Itoa(page), "per_page": "200",
				})
				if gerr != nil {
					break
				}
				var mv movementsResp
				if json.Unmarshal(data, &mv) != nil {
					break
				}
				res.MovementPagesRead++
				res.MovementsFetched += len(mv.MRR.Movements)
				inserted, sawExisting, ierr := insertMovements(ctx, db, mv.MRR.Movements)
				if ierr != nil {
					return nil, ierr
				}
				res.MovementsInserted += inserted
				// Caught up to prior backfill: a whole page already stored.
				if sawExisting && inserted == 0 {
					break
				}
			}
		}
	}
	return res, nil
}

func movementKey(m rawMovement) string {
	h := sha256.Sum256([]byte(m.Timestamp + "|" + m.Category + "|" +
		strconv.FormatInt(m.AmountInCents, 10) + "|" + m.Description + "|" + string(m.LineItems)))
	return hex.EncodeToString(h[:16])
}

func insertMovements(ctx context.Context, db *sql.DB, movements []rawMovement) (inserted int, sawExisting bool, err error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, false, err
	}
	for _, m := range movements {
		key := movementKey(m)
		bucket := classifyBucket(m.Category, m.AmountInCents)
		li := string(m.LineItems)
		if li == "" {
			li = "[]"
		}
		r, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO rev_mrr_movements
			 (mvmt_key, ts, amount_cents, category, bucket, plan_cents, usage_cents, description, line_items)
			 VALUES (?,?,?,?,?,?,?,?,?)`,
			key, m.Timestamp, m.AmountInCents, m.Category, bucket,
			m.Breakouts.Plan, m.Breakouts.Usage, m.Description, li)
		if err != nil {
			_ = tx.Rollback()
			return inserted, sawExisting, fmt.Errorf("inserting movement: %w", err)
		}
		if n, _ := r.RowsAffected(); n > 0 {
			inserted++
		} else {
			sawExisting = true
		}
	}
	if err := tx.Commit(); err != nil {
		return inserted, sawExisting, err
	}
	return inserted, sawExisting, nil
}

// LatestSnapshotAt returns the most recent site-snapshot key, or "" if none.
func LatestSnapshotAt(ctx context.Context, db *sql.DB) (string, error) {
	if err := EnsureSchema(db); err != nil {
		return "", err
	}
	var at sql.NullString
	err := db.QueryRowContext(ctx, `SELECT MAX(snapshot_at) FROM rev_site_snapshots`).Scan(&at)
	if err != nil {
		return "", err
	}
	return at.String, nil
}

// HasMovements reports whether any movement rows are stored.
func HasMovements(ctx context.Context, db *sql.DB) (bool, error) {
	if err := EnsureSchema(db); err != nil {
		return false, err
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM rev_mrr_movements`).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

// Cents renders an integer cents value as a dollar string for human output.
func Cents(c int64) string {
	neg := c < 0
	if neg {
		c = -c
	}
	s := fmt.Sprintf("$%s.%02d", withThousands(strconv.FormatInt(c/100, 10)), c%100)
	if neg {
		return "-" + s
	}
	return s
}

// withThousands inserts comma separators into a non-negative integer string.
func withThousands(s string) string {
	n := len(s)
	if n <= 3 {
		return s
	}
	var b strings.Builder
	pre := n % 3
	if pre > 0 {
		b.WriteString(s[:pre])
		b.WriteByte(',')
	}
	for i := pre; i < n; i += 3 {
		b.WriteString(s[i : i+3])
		if i+3 < n {
			b.WriteByte(',')
		}
	}
	return b.String()
}

// MonthKey truncates an RFC3339-ish timestamp to YYYY-MM. Falls back to the
// first 7 chars when the timestamp is already date-prefixed.
func MonthKey(ts string) string {
	if len(ts) >= 7 {
		return ts[:7]
	}
	return ts
}

// ParseSince parses a --since value as an absolute date (YYYY-MM-DD) or a
// relative window (7d/4w/12m/1y). Returns the cutoff as an RFC3339 lower bound
// string (date granularity is enough for movement/snapshot filtering). now is
// injected for testability.
func ParseSince(since string, now time.Time) (string, error) {
	since = strings.TrimSpace(since)
	if since == "" {
		return "", nil
	}
	if t, err := time.Parse("2006-01-02", since); err == nil {
		return t.Format("2006-01-02"), nil
	}
	if len(since) >= 2 {
		unit := since[len(since)-1]
		nStr := since[:len(since)-1]
		// Reject a negative count: strconv.Atoi parses the sign, so "-5d" would
		// otherwise resolve to a date 5 days in the FUTURE and silently filter
		// out every movement/snapshot.
		if n, err := strconv.Atoi(nStr); err == nil && n >= 0 {
			switch unit {
			case 'd':
				return now.AddDate(0, 0, -n).Format("2006-01-02"), nil
			case 'w':
				return now.AddDate(0, 0, -7*n).Format("2006-01-02"), nil
			case 'm':
				return now.AddDate(0, -n, 0).Format("2006-01-02"), nil
			case 'y':
				return now.AddDate(-n, 0, 0).Format("2006-01-02"), nil
			}
		}
	}
	return "", fmt.Errorf("invalid --since %q: use YYYY-MM-DD or a window like 30d, 4w, 12m, 1y", since)
}
