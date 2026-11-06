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

// RecipientWalk is what one recipients fan-out did: rows upserted, tests
// walked, tests whose fetch or decode failed, and the first such error. A walk
// where every test failed is an error, never "zero results".
type RecipientWalk struct {
	Rows     int
	Tests    int
	Failed   int
	FirstErr error
	// Complete is set when the walk recorded itself complete: every test
	// walked in full, over a completely synced set of phishing tests.
	Complete bool
	// AccountErr is why the account risk score (GET /v1/account) was not
	// taken; the QBR then reads it as unavailable, never as 0.
	AccountErr error
}

// SyncExtras populates the insights-owned tables from live data: it fans out over
// every synced PST to pull recipient-level results, then captures a risk-score
// snapshot for every user and group, and for the account from the documented
// GET /v1/account (current_risk_score; the generated sync never fetches it:
// its "account" resource reads /account/risk_score_history). A failure on one
// PST is counted and the walk continues; when every PST fetch failed the walk
// returns an error and no snapshot is taken. An account fetch that fails is
// reported in walk.AccountErr. Returns the recipient walk and the number of
// risk snapshots captured.
func SyncExtras(ctx context.Context, g Getter, db *sql.DB, perPage int) (walk RecipientWalk, snapshots int, err error) {
	if err = EnsureSchema(ctx, db); err != nil {
		return walk, 0, err
	}
	walk, err = SyncRecipients(ctx, g, db, perPage)
	if err != nil {
		return walk, 0, err
	}
	snapshots, err = SnapshotRisk(ctx, db)
	if err != nil {
		return walk, snapshots, err
	}
	at := time.Now().UTC()
	name, score, aerr := fetchAccountRisk(ctx, g)
	if aerr != nil {
		walk.AccountErr = aerr
		return walk, snapshots, nil
	}
	if err = snapshotAccount(ctx, db, name, score, at); err != nil {
		return walk, snapshots, err
	}
	return walk, snapshots + 1, nil
}

// AccountEntityID is the risk_snapshots entity id of the account risk score
// read from GET /v1/account (the documented account object has no id).
const AccountEntityID = "account"

// fetchAccountRisk reads the account name and current_risk_score from the
// documented GET /v1/account object. A body that is not an object, or an
// object without a numeric current_risk_score, is an error: the score is then
// unavailable, never 0.
func fetchAccountRisk(ctx context.Context, g Getter) (string, float64, error) {
	raw, err := g.Get(ctx, "/account", nil)
	if err != nil {
		return "", 0, err
	}
	var acct struct {
		Name             string   `json:"name"`
		CurrentRiskScore *float64 `json:"current_risk_score"`
	}
	if err := json.Unmarshal(raw, &acct); err != nil {
		return "", 0, fmt.Errorf("decode GET /account: %w", err)
	}
	if acct.CurrentRiskScore == nil {
		return "", 0, fmt.Errorf("GET /account carried no current_risk_score")
	}
	return acct.Name, *acct.CurrentRiskScore, nil
}

func snapshotAccount(ctx context.Context, db *sql.DB, name string, score float64, at time.Time) error {
	_, err := db.ExecContext(ctx, `INSERT INTO "risk_snapshots"
		("entity_type","entity_id","entity_name","risk_score","member_count","snapshot_at")
		VALUES ('account', ?, ?, ?, NULL, ?)
		ON CONFLICT("entity_type","entity_id","snapshot_at") DO UPDATE SET
		 "risk_score"=excluded."risk_score","entity_name"=excluded."entity_name"`,
		AccountEntityID, name, score, at.UTC().Format("2006-01-02T15:04:05.000Z"))
	return err
}

// SyncRecipients walks every PST in the local phishing_tests table and upserts
// its recipient results into pst_recipients. It fails when every PST failed.
// The walk keeps its own completion record (RecipientsResource in sync_state,
// read by CollectionSynced): cleared before the first request, and set only
// when every test was walked in full and the tests walked were a completely
// synced set, so a partial walk is never read as every recipient.
func SyncRecipients(ctx context.Context, g Getter, db *sql.DB, perPage int) (RecipientWalk, error) {
	var walk RecipientWalk
	if perPage <= 0 {
		perPage = 100
	}
	if err := recordRecipientWalk(ctx, db, false, 0); err != nil {
		return walk, err
	}
	testsComplete := CollectionSynced(ctx, db, "phishing-tests")
	refs, err := loadPSTRefs(ctx, db)
	if err != nil {
		return walk, err
	}
	for _, ref := range refs {
		walk.Tests++
		n, err := syncOnePST(ctx, g, db, ref, perPage)
		walk.Rows += n
		if err != nil {
			walk.Failed++
			if walk.FirstErr == nil {
				walk.FirstErr = fmt.Errorf("pst %d: %w", ref.PstID, err)
			}
		}
	}
	if walk.Tests > 0 && walk.Failed == walk.Tests {
		return walk, fmt.Errorf("all %d phishing-test recipient fetches failed: %w", walk.Tests, walk.FirstErr)
	}
	if walk.Failed == 0 && testsComplete {
		if err := recordRecipientWalk(ctx, db, true, walk.Rows); err != nil {
			return walk, err
		}
		walk.Complete = true
	}
	return walk, nil
}

// recordRecipientWalk writes the recipient walk's completion record: complete
// false marks the walk under way (a crash or a failed test then leaves it
// unset), complete true stamps a walk that finished. A store without a
// sync_state table holds no record to clear.
func recordRecipientWalk(ctx context.Context, db *sql.DB, complete bool, rows int) error {
	if !complete {
		_, err := db.ExecContext(ctx, `UPDATE sync_state SET last_attempt_complete = 0 WHERE resource_type = ?`, RecipientsResource)
		if err != nil && strings.Contains(err.Error(), "no such table") {
			return nil
		}
		return err
	}
	_, err := db.ExecContext(ctx, `INSERT INTO sync_state (resource_type, last_cursor, last_synced_at, total_count, last_attempt_complete)
		VALUES (?, '', ?, ?, 1)
		ON CONFLICT(resource_type) DO UPDATE SET last_cursor = '', last_synced_at = excluded.last_synced_at,
		 total_count = excluded.total_count, last_attempt_complete = 1`,
		RecipientsResource, time.Now().UTC().Format(time.RFC3339), rows)
	return err
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

// maxRecipientPages bounds one phishing test's recipient walk (a server that
// ignored the page parameter would otherwise be walked forever).
const maxRecipientPages = 1000

// syncOnePST walks one phishing test's recipient pages until a short or empty
// page. A walk that reaches maxRecipientPages with the last page still full
// has not read every recipient and is an error, so the test counts as failed
// and the sync says its answers are partial (knowbe4 handfix
// r1-incomplete-paging-read-as-complete, Codex r1 knowbe4 4.34.2: the walk
// stopped after 1000 pages and reported the test as complete, so a recipient
// past the bound, with a documented clicked_at, never reached repeat-clickers).
func syncOnePST(ctx context.Context, g Getter, db *sql.DB, ref pstRef, perPage int) (int, error) {
	count := 0
	for page := 1; ; page++ {
		if page > maxRecipientPages {
			return count, fmt.Errorf("recipients for pst %d run past %d pages of %d; the walk stopped before the last page", ref.PstID, maxRecipientPages, perPage)
		}
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

// SnapshotRisk copies the risk score of every user and group (and of the
// account, from an `account info` row) into risk_snapshots. Each copy is
// stamped with the time its row was read from the API (the row's synced_at),
// never the time of the copy: a sync that did not re-read a row copies an
// older reading, and stamping it with the sync's time made it outrank a newer
// one (knowbe4 handfix r2-stale-source-preferred: an `account info` read of 90
// copied into a later sync's snapshot tied with that sync's own GET /v1/account
// read of 70, and the QBR reported 90). A row whose read time does not parse
// is not copied. The account's copies join its one series (AccountEntityID).
// Repeated syncs that re-read a row build the history risk-drift and
// group-risk-contribution diff against.
func SnapshotRisk(ctx context.Context, db *sql.DB) (int, error) {
	if err := EnsureSchema(ctx, db); err != nil {
		return 0, err
	}
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
			var id, name, readAt sql.NullString
			var score sql.NullFloat64
			var members sql.NullInt64
			if withMembers {
				if err := rows.Scan(&id, &name, &score, &readAt, &members); err != nil {
					continue
				}
			} else {
				if err := rows.Scan(&id, &name, &score, &readAt); err != nil {
					continue
				}
			}
			if !id.Valid {
				continue
			}
			read, ok := parseStoreTS(readAt.String)
			if !ok {
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
			if _, err := ins.ExecContext(ctx, kind, id.String, name.String, sc, mc, read.UTC().Format("2006-01-02T15:04:05.000Z")); err != nil {
				return err
			}
			n++
		}
		return rows.Err()
	}

	if err := snap("user", `SELECT id, COALESCE(NULLIF(TRIM(COALESCE(first_name,'')||' '||COALESCE(last_name,'')),''), email) AS name, current_risk_score, CAST(synced_at AS TEXT) FROM "users"`, false); err != nil {
		return n, err
	}
	if err := snap("group", `SELECT id, name, current_risk_score, CAST(synced_at AS TEXT), member_count FROM "groups"`, true); err != nil {
		return n, err
	}
	// The generated "account" table also holds /account/risk_score_history
	// rows, whose current_risk_score is NULL: only a real score is a snapshot.
	if err := snap("account", `SELECT '`+AccountEntityID+`', name, current_risk_score, CAST(synced_at AS TEXT) FROM "account" WHERE current_risk_score IS NOT NULL`, false); err != nil {
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
