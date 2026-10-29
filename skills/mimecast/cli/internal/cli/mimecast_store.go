// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: Mimecast row types and the local tables the hold-queue
// workflow commands keep (triage snapshot, release ledger, release-log cache).
// The classic Mimecast endpoints are POST-only, so generated sync cannot
// mirror them; these tables are owned by the novel commands.

package cli

import (
	"cmp"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"mimecast-pp-cli/internal/client"
	"mimecast-pp-cli/internal/cliutil"
	"mimecast-pp-cli/internal/config"
	"mimecast-pp-cli/internal/store"
)

// heldMessage is one row of find-held-messages data[].heldEmails[].
type heldMessage struct {
	ID             string   `json:"id"`
	Subject        string   `json:"subject,omitempty"`
	FromEnv        mcAddr   `json:"fromEnv"`
	FromHdr        mcAddr   `json:"fromHdr"`
	To             []mcAddr `json:"to,omitempty"`
	HeldReason     string   `json:"heldReason,omitempty"`
	HeldGroup      string   `json:"heldGroup,omitempty"`
	PolicyInfo     string   `json:"policyInfo,omitempty"`
	MessageInfo    string   `json:"messageInfo,omitempty"`
	HeldSince      string   `json:"heldSince,omitempty"`
	Received       string   `json:"received,omitempty"`
	Route          string   `json:"route,omitempty"`
	SpamScore      *int     `json:"spamScore,omitempty"`
	SpamVerdict    string   `json:"spamVerdict,omitempty"`
	DetectionLevel string   `json:"detectionLevel,omitempty"`
	Status         string   `json:"status,omitempty"`
	SenderIP       string   `json:"senderIP,omitempty"`
	Attachments    bool     `json:"attachments,omitempty"`
}

func (h heldMessage) sender() string {
	return cmp.Or(h.FromEnv.EmailAddress, h.FromHdr.EmailAddress)
}

func (h heldMessage) recipients() []string {
	out := make([]string, 0, len(h.To))
	for _, t := range h.To {
		if t.EmailAddress != "" {
			out = append(out, t.EmailAddress)
		}
	}
	return out
}

func (h heldMessage) spamScore() int {
	if h.SpamScore == nil {
		return -1
	}
	return *h.SpamScore
}

func (h heldMessage) risk() (string, string) {
	return classifyHold(h.HeldReason, h.PolicyInfo, h.SpamVerdict, h.spamScore())
}

func parseHeld(data []json.RawMessage) []heldMessage {
	out := make([]heldMessage, 0)
	seen := map[string]bool{}
	for _, raw := range unwrapList(data, "heldEmails") {
		var h heldMessage
		if json.Unmarshal(raw, &h) == nil && h.ID != "" && !seen[h.ID] {
			seen[h.ID] = true
			out = append(out, h)
		}
	}
	return out
}

// spamCheck is one SPF/DKIM/DMARC/RBL/managed/permitted sender result.
type spamCheck struct {
	Allow *bool  `json:"allow,omitempty"`
	Info  string `json:"info,omitempty"`
}

type spamDetail struct {
	SPF             *spamCheck `json:"spf,omitempty"`
	DKIM            *spamCheck `json:"dkim,omitempty"`
	DMARC           *spamCheck `json:"dmarc,omitempty"`
	RBL             *spamCheck `json:"rbl,omitempty"`
	ManagedSender   *spamCheck `json:"managedSender,omitempty"`
	PermittedSender *spamCheck `json:"permittedSender,omitempty"`
	GreyEmail       bool       `json:"greyEmail,omitempty"`
	Verdict         *struct {
		Decision    string `json:"decision,omitempty"`
		Description string `json:"description,omitempty"`
		Risk        string `json:"risk,omitempty"`
	} `json:"verdict,omitempty"`
}

// failedChecks lists SPF/DKIM/DMARC/RBL checks reported as not allowed.
func (d *spamDetail) failedChecks() []string {
	if d == nil {
		return nil
	}
	var out []string
	for name, c := range map[string]*spamCheck{"spf": d.SPF, "dkim": d.DKIM, "dmarc": d.DMARC, "rbl": d.RBL} {
		if c != nil && c.Allow != nil && !*c.Allow {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// releaseLog is one row of get-held-release-logs data[].heldReleaseLogs[].
type releaseLog struct {
	ID                   string      `json:"id"`
	Subject              string      `json:"subject,omitempty"`
	FromEnv              mcAddr      `json:"fromEnv"`
	FromHdr              mcAddr      `json:"fromHdr"`
	To                   []mcAddr    `json:"to,omitempty"`
	HeldReason           string      `json:"heldReason,omitempty"`
	HeldGroup            string      `json:"heldGroup,omitempty"`
	Policy               string      `json:"policy,omitempty"`
	MessageInfo          string      `json:"messageInfo,omitempty"`
	Operator             mcAddr      `json:"operator"`
	Released             string      `json:"released,omitempty"`
	Status               string      `json:"status,omitempty"`
	RejectReason         string      `json:"rejectReason,omitempty"`
	Route                string      `json:"route,omitempty"`
	SpamScore            *int        `json:"spamScore,omitempty"`
	DetectionLevel       string      `json:"detectionLevel,omitempty"`
	SpamProcessingDetail *spamDetail `json:"spamProcessingDetail,omitempty"`
}

func (r releaseLog) sender() string {
	return cmp.Or(r.FromEnv.EmailAddress, r.FromHdr.EmailAddress)
}

func (r releaseLog) spamScore() int {
	if r.SpamScore == nil {
		return -1
	}
	return *r.SpamScore
}

func parseReleaseLogs(data []json.RawMessage) []releaseLog {
	out := make([]releaseLog, 0)
	for _, raw := range unwrapList(data, "heldReleaseLogs") {
		var r releaseLog
		if json.Unmarshal(raw, &r) == nil && (r.ID != "" || r.Released != "") {
			out = append(out, r)
		}
	}
	return out
}

// openMimecastStore opens the CLI's SQLite store and ensures the
// hand-owned tables exist.
// Delegated customer accounts get their own database file so one
// customer's ledger and sender history never mix with another's.
//
// owner is the database's tenant identity (see mcStoreOwner); it is checked
// even for an explicit --db.
func openMimecastStore(ctx context.Context, dbPath, accountCode, owner string) (*store.Store, error) {
	dbPath = workflowDBPath(dbPath, accountCode)
	db, err := store.OpenWithContext(ctx, dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening local store: %w", err)
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS mimecast_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS mimecast_held_seen (
			id TEXT PRIMARY KEY, first_seen TEXT NOT NULL, last_seen TEXT NOT NULL, reason TEXT)`,
		`CREATE TABLE IF NOT EXISTS mimecast_release_ledger (
			seq INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, held_id TEXT NOT NULL,
			ticket TEXT, action TEXT NOT NULL, sender TEXT, subject TEXT, hold_reason TEXT,
			risk_class TEXT, override TEXT, outcome TEXT NOT NULL, error TEXT)`,
		`CREATE TABLE IF NOT EXISTS mimecast_release_logs (
			id TEXT PRIMARY KEY, released TEXT, status TEXT, sender TEXT, recipients TEXT,
			subject TEXT, held_reason TEXT, policy TEXT, operator TEXT, reject_reason TEXT,
			spam_score INTEGER, data TEXT NOT NULL, fetched_at TEXT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS mimecast_release_logs_released ON mimecast_release_logs (released)`,
	}
	for _, s := range stmts {
		if _, err := db.DB().ExecContext(ctx, s); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("preparing local tables: %w", err)
		}
	}
	// A database belongs to one account (empty = the API app's own account),
	// including an explicit --db, so customer histories never mix. Only an
	// empty database is stamped automatically; populated rows with no owner
	// record could be anyone's.
	var recorded string
	err = db.DB().QueryRowContext(ctx, `SELECT value FROM mimecast_meta WHERE key = 'owner'`).Scan(&recorded)
	switch {
	case err == sql.ErrNoRows:
		var rows int
		if err := db.DB().QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM mimecast_held_seen) + (SELECT COUNT(*) FROM mimecast_release_ledger) + (SELECT COUNT(*) FROM mimecast_release_logs)`).Scan(&rows); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("checking local database: %w", err)
		}
		if rows > 0 {
			_ = db.Close()
			return nil, configErr(fmt.Errorf("local database %s holds Mimecast workflow data with no recorded account owner; use a new --db", dbPath))
		}
		if _, err := db.DB().ExecContext(ctx, `INSERT INTO mimecast_meta (key, value) VALUES ('owner', ?)`, owner); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("recording database owner: %w", err)
		}
		recorded = owner
	case err != nil:
		_ = db.Close()
		return nil, fmt.Errorf("reading database owner: %w", err)
	}
	if recorded != owner {
		_ = db.Close()
		return nil, configErr(fmt.Errorf("local database %s belongs to a different Mimecast tenant or account code; use a different --db (owner %s, current %s)", dbPath, recorded, owner))
	}
	return db, nil
}

// ledgerRow is one release/reject decision taken through this CLI.
type ledgerRow struct {
	At         string `json:"at"`
	HeldID     string `json:"held_id"`
	Ticket     string `json:"ticket,omitempty"`
	Action     string `json:"action"`
	Sender     string `json:"sender,omitempty"`
	Subject    string `json:"subject,omitempty"`
	HoldReason string `json:"hold_reason,omitempty"`
	RiskClass  string `json:"risk_class,omitempty"`
	Override   string `json:"override,omitempty"`
	Outcome    string `json:"outcome"`
	Error      string `json:"error,omitempty"`
}

func writeLedger(ctx context.Context, db *store.Store, rows []ledgerRow) error {
	if len(rows) == 0 {
		return nil
	}
	tx, err := db.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if _, err := tx.ExecContext(ctx, `INSERT INTO mimecast_release_ledger
			(at, held_id, ticket, action, sender, subject, hold_reason, risk_class, override, outcome, error)
			VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
			r.At, r.HeldID, r.Ticket, r.Action, r.Sender, r.Subject, r.HoldReason, r.RiskClass, r.Override, r.Outcome, r.Error); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func readLedger(ctx context.Context, db *store.Store, since time.Time) ([]ledgerRow, error) {
	if db == nil {
		return nil, nil
	}
	rows, err := db.DB().QueryContext(ctx, `SELECT at, held_id, COALESCE(ticket,''), action, COALESCE(sender,''),
		COALESCE(subject,''), COALESCE(hold_reason,''), COALESCE(risk_class,''), COALESCE(override,''), outcome, COALESCE(error,'')
		FROM mimecast_release_ledger WHERE at >= ? ORDER BY seq`, since.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ledgerRow, 0)
	for rows.Next() {
		var r ledgerRow
		if err := rows.Scan(&r.At, &r.HeldID, &r.Ticket, &r.Action, &r.Sender, &r.Subject, &r.HoldReason, &r.RiskClass, &r.Override, &r.Outcome, &r.Error); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// cacheReleaseLogs upserts release-log rows so history accumulates beyond
// any single API lookback window.
func cacheReleaseLogs(ctx context.Context, db *store.Store, logs []releaseLog) error {
	if len(logs) == 0 {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := db.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for _, l := range logs {
		data, _ := json.Marshal(l)
		c := cachedFromLog(l)
		var score sql.NullInt64
		if l.SpamScore != nil {
			score = sql.NullInt64{Int64: int64(*l.SpamScore), Valid: true}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO mimecast_release_logs
			(id, released, status, sender, recipients, subject, held_reason, policy, operator, reject_reason, spam_score, data, fetched_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(id) DO UPDATE SET released=excluded.released, status=excluded.status, data=excluded.data, fetched_at=excluded.fetched_at`,
			c.ID, c.Released, c.Status, c.Sender, strings.Join(c.Recipients, ","), l.Subject,
			l.HeldReason, l.Policy, c.Operator, l.RejectReason, score, string(data), now); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// cachedRelease is the slice of a cached release-log row the aggregations need.
type cachedRelease struct {
	ID         string
	Released   string
	Status     string
	Sender     string
	Operator   string
	Recipients []string
	// Risky marks a release that held release-audit would flag (never-release
	// hold, high spam-verdict risk, or failed SPF/DKIM/DMARC); such pairs are
	// never permit candidates.
	Risky bool
}

// cachedFromLog is a release-log row as the cache stores it: one
// normalization for a cached row and a live one.
func cachedFromLog(l releaseLog) cachedRelease {
	c := cachedRelease{
		ID:       l.ID,
		Released: l.Released,
		Status:   strings.ToLower(l.Status),
		Sender:   strings.ToLower(l.sender()),
		Operator: strings.ToLower(l.Operator.EmailAddress),
		Risky:    len(releaseRiskFlags(l)) > 0,
	}
	if c.ID == "" {
		c.ID = l.Released + "|" + c.Sender + "|" + l.Subject
	}
	if t, ok := parseMCTime(c.Released); ok {
		c.Released = t.UTC().Format(time.RFC3339)
	}
	for _, t := range l.To {
		if x := strings.ToLower(strings.TrimSpace(t.EmailAddress)); x != "" {
			c.Recipients = append(c.Recipients, x)
		}
	}
	return c
}

func readCachedReleases(ctx context.Context, db *store.Store, since time.Time) ([]cachedRelease, error) {
	if db == nil {
		return nil, nil
	}
	rows, err := db.DB().QueryContext(ctx, `SELECT id, COALESCE(released,''), COALESCE(status,''), COALESCE(sender,''), COALESCE(operator,''), COALESCE(recipients,''), data
		FROM mimecast_release_logs WHERE released >= ?`, since.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	out := make([]cachedRelease, 0)
	for rows.Next() {
		var r cachedRelease
		var recips, data string
		if err := rows.Scan(&r.ID, &r.Released, &r.Status, &r.Sender, &r.Operator, &recips, &data); err != nil {
			_ = rows.Close()
			return nil, err
		}
		var l releaseLog
		if json.Unmarshal([]byte(data), &l) == nil {
			r.Risky = len(releaseRiskFlags(l)) > 0 // the same rule held release-audit flags
		}
		for _, x := range strings.Split(recips, ",") {
			if x = strings.TrimSpace(x); x != "" {
				r.Recipients = append(r.Recipients, x)
			}
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	return out, rows.Close()
}

// fetchReleaseLogs pages get-held-release-logs for [start, end].
func fetchReleaseLogs(ctx context.Context, c *client.Client, start, end time.Time, accountCode string, maxPages int, searchBy map[string]any) ([]releaseLog, mcPage, error) {
	item := map[string]any{"start": mcTime(start), "end": mcTime(end)}
	if searchBy != nil {
		item["searchBy"] = searchBy
	}
	page, err := mcReadAll(ctx, c, "/api/gateway/get-held-release-logs", []any{item}, accountCode, mcMaxPageSize, maxPages)
	if err != nil {
		return nil, page, err
	}
	return parseReleaseLogs(page.Data), page, nil
}

// fetchHeld pages find-held-messages (admin scope) for [start, end].
func fetchHeld(ctx context.Context, c *client.Client, start, end time.Time, accountCode string, maxPages int, searchBy []map[string]any) ([]heldMessage, mcPage, error) {
	item := map[string]any{"admin": true, "start": mcTime(start), "end": mcTime(end)}
	if len(searchBy) > 0 {
		item["searchBy"] = searchBy
	}
	page, err := mcReadAll(ctx, c, "/api/gateway/find-held-messages", []any{item}, accountCode, mcMaxPageSize, maxPages)
	if err != nil {
		return nil, page, err
	}
	return parseHeld(page.Data), page, nil
}

var unsafeFileChars = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

// safeFileToken makes an account code safe to embed in a file name.
func safeFileToken(s string) string {
	return unsafeFileChars.ReplaceAllString(strings.TrimSpace(s), "_")
}

// mcStoreOwner identifies whose data a local database holds: the delegated
// account code plus a hash of the API application's client ID and base URL,
// so two tenants (or two customer accounts) can never share one --db. Only
// the hash is stored; the client ID itself is not written to disk here.
func mcStoreOwner(flags *rootFlags, accountCode string) string {
	clientID, baseURL := "", ""
	if cfg, err := config.Load(flags.configPath); err == nil {
		clientID, baseURL = cfg.ClientID, cfg.BaseURL
	}
	sum := sha256.Sum256([]byte(clientID + "\n" + strings.TrimRight(baseURL, "/")))
	return "app:" + hex.EncodeToString(sum[:8]) + " account:" + accountCode
}

// openAccountStore opens the workflow store for the given account code with
// the matching tenant owner, so the two can never disagree.
func openAccountStore(ctx context.Context, flags *rootFlags, dbPath, accountCode string) (*store.Store, error) {
	return openMimecastStore(ctx, dbPath, accountCode, mcStoreOwner(flags, accountCode))
}

// workflowDBPath is the workflow database for an account code: --db when
// given, else the default store, one file per delegated account.
func workflowDBPath(dbPath, accountCode string) string {
	if dbPath != "" {
		return dbPath
	}
	dbPath = defaultDBPath("mimecast-cli")
	if accountCode != "" {
		dbPath = strings.TrimSuffix(dbPath, filepath.Ext(dbPath)) + "-account-" + safeFileToken(accountCode) + filepath.Ext(dbPath)
	}
	return dbPath
}

// openReadOnlyWorkflowStore opens the workflow store for a command the MCP
// server exposes as read-only (handfixes.json
// mimecast-read-only-workflows-write-nothing). Such a tool runs without
// confirm, so under the MCP server, and with MIMECAST_NO_LOCAL_DATA=1, it
// writes nothing: an existing store is opened read-only (its owner still
// checked) and a store with no workflow data yet reads as empty (nil db).
// From the CLI, with local data on, it is openAccountStore and writable.
func openReadOnlyWorkflowStore(ctx context.Context, flags *rootFlags, dbPath, accountCode string) (db *store.Store, writable bool, err error) {
	if !cliutil.IsMCPChildEnv() && !localDataDisabled() {
		db, err = openAccountStore(ctx, flags, dbPath, accountCode)
		return db, err == nil, err
	}
	dbPath = workflowDBPath(dbPath, accountCode)
	if _, statErr := os.Stat(dbPath); errors.Is(statErr, fs.ErrNotExist) {
		return nil, false, nil
	}
	db, err = store.OpenReadOnlyContext(ctx, dbPath)
	if err != nil {
		return nil, false, fmt.Errorf("opening local store: %w", err)
	}
	var recorded string
	err = db.DB().QueryRowContext(ctx, `SELECT value FROM mimecast_meta WHERE key = 'owner'`).Scan(&recorded)
	switch {
	case errors.Is(err, sql.ErrNoRows) || err != nil && strings.Contains(err.Error(), "no such table"):
		_ = db.Close()
		return nil, false, nil // no workflow data recorded yet
	case err != nil:
		_ = db.Close()
		return nil, false, fmt.Errorf("reading database owner: %w", err)
	}
	if owner := mcStoreOwner(flags, accountCode); recorded != owner {
		_ = db.Close()
		return nil, false, configErr(fmt.Errorf("local database %s belongs to a different Mimecast tenant or account code; use a different --db (owner %s, current %s)", dbPath, recorded, owner))
	}
	return db, false, nil
}

// mergeLiveReleases adds the live window's rows a store that was not written
// does not hold yet, so a read-only run answers from the live API too.
func mergeLiveReleases(cached []cachedRelease, logs []releaseLog) []cachedRelease {
	have := make(map[string]bool, len(cached))
	for _, c := range cached {
		have[c.ID] = true
	}
	for _, l := range logs {
		if c := cachedFromLog(l); !have[c.ID] {
			have[c.ID] = true
			cached = append(cached, c)
		}
	}
	return cached
}

// fetchHeldUntil is fetchHeld that stops paging once done reports the
// messages read so far are enough (done may be nil).
func fetchHeldUntil(ctx context.Context, c *client.Client, start, end time.Time, accountCode string, maxPages int, done func([]heldMessage) bool) ([]heldMessage, mcPage, error) {
	item := map[string]any{"admin": true, "start": mcTime(start), "end": mcTime(end)}
	var stop func([]json.RawMessage) bool
	if done != nil {
		stop = func(data []json.RawMessage) bool { return done(parseHeld(data)) }
	}
	page, err := mcReadUntil(ctx, c, "/api/gateway/find-held-messages", []any{item}, accountCode, mcMaxPageSize, maxPages, stop)
	if err != nil {
		return nil, page, err
	}
	return parseHeld(page.Data), page, nil
}
