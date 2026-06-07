// Package insights powers knowbe4-cli's transcendence commands: cross-entity
// joins and time-windowed aggregations over the local SQLite store that no single
// KnowBe4 Reporting API call provides (repeat clickers, untrained clickers, risk
// drift, coverage gaps, phish-prone trend, risk leaderboard, group risk
// contribution, and the composed QBR pack).
//
// The package reads the generator-emitted typed tables (users, groups,
// phishing_tests, training_enrollments, account) and two additional tables it
// owns and populates itself: pst_recipients (per-user phishing results, fanned
// out over every PST — a sub-resource the generated sync does not walk) and
// risk_snapshots (current_risk_score per entity stamped at each sync, so drift
// can diff across time even before risk_score_history is pulled).
//
// All outbound HTTP goes through the generated client (the Getter interface);
// this package never constructs its own http.Client.
package insights

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Getter is the minimal slice of the generated *client.Client the sync path
// needs. Reusing the generated client keeps rate limiting, auth, caching, and
// region templating in one place rather than re-implementing an HTTP client.
type Getter interface {
	Get(ctx context.Context, path string, params map[string]string) (json.RawMessage, error)
}

// EnsureSchema creates the insights-owned tables if they do not exist. It is
// idempotent and safe to call before every read or write.
func EnsureSchema(ctx context.Context, db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS "pst_recipients" (
			"pst_id" INTEGER NOT NULL,
			"recipient_id" INTEGER NOT NULL,
			"user_id" INTEGER,
			"user_email" TEXT,
			"campaign_id" INTEGER,
			"scheduled_at" TEXT,
			"delivered_at" TEXT,
			"opened_at" TEXT,
			"clicked_at" TEXT,
			"replied_at" TEXT,
			"attachment_opened_at" TEXT,
			"macro_enabled_at" TEXT,
			"data_entered_at" TEXT,
			"reported_at" TEXT,
			"bounced_at" TEXT,
			"ip_location" TEXT,
			"browser" TEXT,
			"os" TEXT,
			"synced_at" DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY ("pst_id", "recipient_id")
		)`,
		`CREATE INDEX IF NOT EXISTS idx_pst_recipients_email ON "pst_recipients"("user_email")`,
		`CREATE TABLE IF NOT EXISTS "risk_snapshots" (
			"entity_type" TEXT NOT NULL,
			"entity_id" TEXT NOT NULL,
			"entity_name" TEXT,
			"risk_score" REAL,
			"member_count" INTEGER,
			"snapshot_at" TEXT NOT NULL,
			PRIMARY KEY ("entity_type", "entity_id", "snapshot_at")
		)`,
	}
	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("insights schema: %w", err)
		}
	}
	return nil
}

// apiRecipient mirrors the KnowBe4 /phishing/security_tests/{pst_id}/recipients
// item shape. Timestamp fields are plain strings; a JSON null decodes to "".
type apiRecipient struct {
	RecipientID int64 `json:"recipient_id"`
	PstID       int64 `json:"pst_id"`
	User        struct {
		ID    int64  `json:"id"`
		Email string `json:"email"`
	} `json:"user"`
	ScheduledAt        string `json:"scheduled_at"`
	DeliveredAt        string `json:"delivered_at"`
	OpenedAt           string `json:"opened_at"`
	ClickedAt          string `json:"clicked_at"`
	RepliedAt          string `json:"replied_at"`
	AttachmentOpenedAt string `json:"attachment_opened_at"`
	MacroEnabledAt     string `json:"macro_enabled_at"`
	DataEnteredAt      string `json:"data_entered_at"`
	ReportedAt         string `json:"reported_at"`
	BouncedAt          string `json:"bounced_at"`
	IPLocation         string `json:"ip_location"`
	Browser            string `json:"browser"`
	OS                 string `json:"os"`
}

// pstRef is a (pst_id, campaign_id) pair read from the synced phishing_tests table.
type pstRef struct {
	PstID      int64
	CampaignID sql.NullInt64
}

// SyncExtras populates the insights-owned tables from live data: it fans out over
// every synced PST to pull recipient-level results, then captures a risk-score
// snapshot for every user, group, and the account. It is best-effort: a failure
// on one PST is recorded and the walk continues. Returns the number of recipient
// rows upserted and the number of risk snapshots captured.
func SyncExtras(ctx context.Context, g Getter, db *sql.DB, perPage int) (recipients int, snapshots int, err error) {
	if err = EnsureSchema(ctx, db); err != nil {
		return 0, 0, err
	}
	recipients, err = SyncRecipients(ctx, g, db, perPage)
	if err != nil {
		return recipients, 0, err
	}
	snapshots, err = SnapshotRisk(ctx, db, time.Now().UTC())
	return recipients, snapshots, err
}

// SyncRecipients walks every PST in the local phishing_tests table and upserts
// its recipient results into pst_recipients.
func SyncRecipients(ctx context.Context, g Getter, db *sql.DB, perPage int) (int, error) {
	if perPage <= 0 {
		perPage = 100
	}
	refs, err := loadPSTRefs(ctx, db)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, ref := range refs {
		n, err := syncOnePST(ctx, g, db, ref, perPage)
		if err != nil {
			// best-effort: skip this PST, keep going
			continue
		}
		total += n
	}
	return total, nil
}

func loadPSTRefs(ctx context.Context, db *sql.DB) ([]pstRef, error) {
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT pst_id, campaign_id FROM "phishing_tests" WHERE pst_id IS NOT NULL AND pst_id > 0`)
	if err != nil {
		// phishing_tests not synced yet -> nothing to walk
		return nil, nil
	}
	defer rows.Close()
	var refs []pstRef
	for rows.Next() {
		var r pstRef
		if err := rows.Scan(&r.PstID, &r.CampaignID); err != nil {
			continue
		}
		refs = append(refs, r)
	}
	return refs, rows.Err()
}

func syncOnePST(ctx context.Context, g Getter, db *sql.DB, ref pstRef, perPage int) (int, error) {
	count := 0
	for page := 1; page <= 1000; page++ {
		params := map[string]string{
			"page":     fmt.Sprintf("%d", page),
			"per_page": fmt.Sprintf("%d", perPage),
		}
		raw, err := g.Get(ctx, fmt.Sprintf("/phishing/security_tests/%d/recipients", ref.PstID), params)
		if err != nil {
			return count, err
		}
		var batch []apiRecipient
		if err := json.Unmarshal(raw, &batch); err != nil {
			return count, fmt.Errorf("decode recipients for pst %d: %w", ref.PstID, err)
		}
		if len(batch) == 0 {
			break
		}
		if err := upsertRecipients(ctx, db, ref, batch); err != nil {
			return count, err
		}
		count += len(batch)
		if len(batch) < perPage {
			break
		}
	}
	return count, nil
}

func upsertRecipients(ctx context.Context, db *sql.DB, ref pstRef, batch []apiRecipient) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO "pst_recipients"
		("pst_id","recipient_id","user_id","user_email","campaign_id",
		 "scheduled_at","delivered_at","opened_at","clicked_at","replied_at",
		 "attachment_opened_at","macro_enabled_at","data_entered_at","reported_at","bounced_at",
		 "ip_location","browser","os","synced_at")
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,CURRENT_TIMESTAMP)
		ON CONFLICT("pst_id","recipient_id") DO UPDATE SET
		 "user_id"=excluded."user_id","user_email"=excluded."user_email","campaign_id"=excluded."campaign_id",
		 "scheduled_at"=excluded."scheduled_at","delivered_at"=excluded."delivered_at","opened_at"=excluded."opened_at",
		 "clicked_at"=excluded."clicked_at","replied_at"=excluded."replied_at","attachment_opened_at"=excluded."attachment_opened_at",
		 "macro_enabled_at"=excluded."macro_enabled_at","data_entered_at"=excluded."data_entered_at","reported_at"=excluded."reported_at",
		 "bounced_at"=excluded."bounced_at","ip_location"=excluded."ip_location","browser"=excluded."browser","os"=excluded."os",
		 "synced_at"=CURRENT_TIMESTAMP`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	var campaignID any
	if ref.CampaignID.Valid {
		campaignID = ref.CampaignID.Int64
	}
	for _, r := range batch {
		pst := r.PstID
		if pst == 0 {
			pst = ref.PstID
		}
		if _, err := stmt.ExecContext(ctx,
			pst, r.RecipientID, r.User.ID, r.User.Email, campaignID,
			r.ScheduledAt, r.DeliveredAt, r.OpenedAt, r.ClickedAt, r.RepliedAt,
			r.AttachmentOpenedAt, r.MacroEnabledAt, r.DataEnteredAt, r.ReportedAt, r.BouncedAt,
			r.IPLocation, r.Browser, r.OS,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SnapshotRisk captures the current risk score for every user and group (and the
// account) into risk_snapshots, stamped with `at`. Repeated calls over time build
// the history that risk-drift and group-risk-contribution diff against.
func SnapshotRisk(ctx context.Context, db *sql.DB, at time.Time) (int, error) {
	if err := EnsureSchema(ctx, db); err != nil {
		return 0, err
	}
	stamp := at.UTC().Format("2006-01-02T15:04:05.000Z")
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() //nolint:errcheck

	ins, err := tx.PrepareContext(ctx, `INSERT INTO "risk_snapshots"
		("entity_type","entity_id","entity_name","risk_score","member_count","snapshot_at")
		VALUES (?,?,?,?,?,?)
		ON CONFLICT("entity_type","entity_id","snapshot_at") DO UPDATE SET
		 "risk_score"=excluded."risk_score","entity_name"=excluded."entity_name","member_count"=excluded."member_count"`)
	if err != nil {
		return 0, err
	}
	defer ins.Close()

	n := 0
	snap := func(kind, query string, withMembers bool) error {
		rows, qerr := tx.QueryContext(ctx, query)
		if qerr != nil {
			if strings.Contains(qerr.Error(), "no such table") {
				return nil // typed table absent -> skip kind
			}
			return qerr // real failure (locked, I/O, cancelled) must surface
		}
		defer rows.Close()
		for rows.Next() {
			var id, name sql.NullString
			var score sql.NullFloat64
			var members sql.NullInt64
			if withMembers {
				if err := rows.Scan(&id, &name, &score, &members); err != nil {
					continue
				}
			} else {
				if err := rows.Scan(&id, &name, &score); err != nil {
					continue
				}
			}
			if !id.Valid {
				continue
			}
			var mc any
			if members.Valid {
				mc = members.Int64
			}
			var sc any
			if score.Valid {
				sc = score.Float64
			}
			if _, err := ins.ExecContext(ctx, kind, id.String, name.String, sc, mc, stamp); err != nil {
				return err
			}
			n++
		}
		return rows.Err()
	}

	if err := snap("user", `SELECT id, COALESCE(NULLIF(TRIM(COALESCE(first_name,'')||' '||COALESCE(last_name,'')),''), email) AS name, current_risk_score FROM "users"`, false); err != nil {
		return n, err
	}
	if err := snap("group", `SELECT id, name, current_risk_score, member_count FROM "groups"`, true); err != nil {
		return n, err
	}
	if err := snap("account", `SELECT id, name, current_risk_score FROM "account"`, false); err != nil {
		return n, err
	}
	return n, tx.Commit()
}

// --- duration / time helpers -------------------------------------------------

// ParseWindow parses durations including KnowBe4-friendly day/week/month/year
// suffixes that Go's time.ParseDuration rejects: "90d", "12w", "6mo", "1y", plus
// standard "h"/"m"/"s". Returns the duration; an empty string yields 0.
func ParseWindow(s string) (time.Duration, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, nil
	}
	mult := map[string]time.Duration{
		"d":  24 * time.Hour,
		"w":  7 * 24 * time.Hour,
		"mo": 30 * 24 * time.Hour,
		"y":  365 * 24 * time.Hour,
	}
	for _, suf := range []string{"mo", "d", "w", "y"} {
		if strings.HasSuffix(s, suf) {
			num := strings.TrimSuffix(s, suf)
			var n float64
			if _, err := fmt.Sscanf(num, "%g", &n); err != nil {
				return 0, fmt.Errorf("invalid window %q", s)
			}
			return time.Duration(float64(mult[suf]) * n), nil
		}
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid window %q (use forms like 90d, 12w, 6mo, 1y, or 24h)", s)
	}
	return d, nil
}

// parseTS parses a KnowBe4 timestamp leniently. Returns zero time + false when
// the value is empty/null/unparseable (i.e. "the event did not occur").
func parseTS(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" || s == "null" {
		return time.Time{}, false
	}
	for _, layout := range []string{
		"2006-01-02T15:04:05.000Z07:00",
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05Z",
		"2006-01-02",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// occurred reports whether a timestamp field marks an event that happened
// (non-empty) and, when `since` is non-zero, happened at or after it.
func occurred(ts string, since time.Time) bool {
	t, ok := parseTS(ts)
	if !ok {
		return false
	}
	if since.IsZero() {
		return true
	}
	return !t.Before(since)
}
