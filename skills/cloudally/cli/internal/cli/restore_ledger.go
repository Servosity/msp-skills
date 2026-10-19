// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"crypto/rand"
	"crypto/sha1" // #nosec G505 -- batch key is a dedupe fingerprint, not a security primitive.
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"cloudally-pp-cli/internal/store"
)

// batchIDRandomBytes is the length of the random suffix on a batch id.
const batchIDRandomBytes = 2

// recoveredInFlightError is written on rows a previous run left SUBMITTING.
const recoveredInFlightError = "a previous run stopped while this restore was being sent; it may have been accepted"

// restoreBatch is one row of restore_batches.
type restoreBatch struct {
	BatchID       string         `json:"batch_id"`
	BatchKey      string         `json:"batch_key"`
	ResumeKey     string         `json:"resume_key,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	Scope         string         `json:"scope"`
	AccountID     string         `json:"account_id,omitempty"`
	AuthScope     string         `json:"auth_scope,omitempty"`
	TaskID        string         `json:"task_id"`
	Source        string         `json:"source"`
	Label         string         `json:"label,omitempty"`
	Dates         []string       `json:"dates"`
	Options       map[string]any `json:"options"`
	LastRefreshAt *time.Time     `json:"last_refresh_at,omitempty"`
}

// restoreJob is one row of restore_jobs. Status is what the row shows: the
// submission outcome until 'restore watch' replaces it with the RESTORE
// task's own status. SubmitStatus is the submission outcome alone and is
// what decides whether a row may be sent again; see submitOutcome.
type restoreJob struct {
	ID           int64      `json:"id"`
	BatchID      string     `json:"batch_id"`
	Entity       string     `json:"entity"`
	ExtID        string     `json:"ext_id,omitempty"`
	Requested    string     `json:"requested"`
	ResolvedRaw  int64      `json:"resolved_raw"`
	Resolved     string     `json:"resolved"`
	SubSource    string     `json:"sub_source,omitempty"`
	Status       string     `json:"status"`
	SubmitStatus string     `json:"submit_status"`
	HTTPStatus   int        `json:"http_status,omitempty"`
	Response     string     `json:"response,omitempty"`
	Error        string     `json:"error,omitempty"`
	TaskID       string     `json:"task_id,omitempty"`
	Progress     string     `json:"progress,omitempty"`
	SubmittedAt  *time.Time `json:"submitted_at,omitempty"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// submitOutcome is the row's submission outcome: PLANNED, SUBMITTING,
// SUBMITTED, FAILED (rejected before it ran), FAILED_REDIRECT, or UNKNOWN.
// It never takes a RESTORE task status, so a task that ran and then
// reported FAILED still reads SUBMITTED here. Rows read from the ledger
// always carry it; a row built without it is derived with the same rule
// the ledger migration uses (store.LegacySubmitStatusSQL).
func (j restoreJob) submitOutcome() string {
	if s := strings.ToUpper(strings.TrimSpace(j.SubmitStatus)); s != "" {
		return s
	}
	switch {
	case strings.TrimSpace(j.TaskID) != "":
		return jobSubmitted
	case j.Status == jobPlanned || j.Status == jobSubmitting || j.Status == jobSubmitted || j.Status == jobUnknown || j.Status == jobFailedRedirect:
		return j.Status
	case j.Status == jobFailed && j.HTTPStatus >= 200 && j.HTTPStatus <= 299:
		return jobSubmitted
	case j.Status == jobFailed:
		return jobFailed
	default:
		return jobSubmitted
	}
}

// jobEverAccepted reports whether the API accepted this row's restore, or
// may have. Such a row is never sent again automatically: an UNKNOWN row
// only with --retry-unknown, and a row with a RESTORE task id never.
func jobEverAccepted(j restoreJob) bool {
	switch j.submitOutcome() {
	case jobSubmitted, jobSubmitting, jobUnknown:
		return true
	}
	return strings.TrimSpace(j.TaskID) != ""
}

// batchSummary is the per-batch aggregate 'restore history' prints.
type batchSummary struct {
	BatchID       string     `json:"batch_id"`
	CreatedAt     time.Time  `json:"created_at"`
	Scope         string     `json:"scope"`
	AccountID     string     `json:"account_id,omitempty"`
	AuthScope     string     `json:"auth_scope,omitempty"`
	TaskID        string     `json:"task_id"`
	Source        string     `json:"source"`
	Label         string     `json:"label,omitempty"`
	Rows          int        `json:"rows"`
	Submitted     int        `json:"submitted"`
	Failed        int        `json:"failed"`
	Unknown       int        `json:"unknown"`
	LastRefreshAt *time.Time `json:"last_refresh_at,omitempty"`
}

// openLedger opens the local store read-write so the ledger tables exist.
func openLedger(ctx context.Context) (*store.Store, error) {
	return store.OpenWithContext(ctx, defaultDBPath(restoreCLIName))
}

// openLedgerRead opens the store read-only. ok is false when no store file
// exists yet, which the caller reports as an empty ledger.
func openLedgerRead(ctx context.Context) (db *store.Store, ok bool, err error) {
	path := defaultDBPath(restoreCLIName)
	if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
		return nil, false, nil
	}
	db, err = store.OpenReadOnlyContext(ctx, path)
	if err != nil {
		return nil, false, err
	}
	return db, true, nil
}

func isMissingTableErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no such table")
}

// newBatchID returns YYYYMMDD-HHMMSS-<4 hex>.
func newBatchID(now time.Time) string {
	buf := make([]byte, batchIDRandomBytes)
	// crypto/rand.Read never returns an error since Go 1.24 (it crashes
	// the program instead), so there is no fallback to write.
	_, _ = rand.Read(buf)
	return now.UTC().Format("20060102-150405") + "-" + hex.EncodeToString(buf)
}

func normalizedSorted(values []string, lower bool) []string {
	out := append([]string(nil), values...)
	for i := range out {
		out[i] = strings.TrimSpace(out[i])
		if lower {
			out[i] = strings.ToLower(out[i])
		}
	}
	sort.Strings(out)
	return out
}

// batchKey is the deterministic fingerprint stored on a batch: scope, the
// recorded account, task, the sorted entity filter (empty means every entity
// on the task), and the sorted requested dates. Batches written before the
// account was recorded used the resolved entity list here; --resume still
// looks those up by the same formula.
func batchKey(scopeKind, account, taskID string, entities, dates []string) string {
	h := sha1.New() // #nosec G401 -- fingerprint only.
	fmt.Fprintf(h, "%s|%s|%s|%s|%s", scopeKind, account, taskID,
		strings.Join(normalizedSorted(entities, true), ","), strings.Join(normalizedSorted(dates, false), ","))
	return hex.EncodeToString(h.Sum(nil))
}

// resumeKey is the account-free fingerprint --resume searches by, so a
// batch recorded under a different account is found and refused rather
// than silently replaced by a new batch.
func resumeKey(scopeKind, taskID string, entities, dates []string) string {
	h := sha1.New() // #nosec G401 -- fingerprint only.
	fmt.Fprintf(h, "resume|%s|%s|%s|%s", scopeKind, taskID,
		strings.Join(normalizedSorted(entities, true), ","), strings.Join(normalizedSorted(dates, false), ","))
	return hex.EncodeToString(h.Sum(nil))
}

func ledgerInsertBatch(db *sql.DB, b restoreBatch) error {
	dates, _ := json.Marshal(b.Dates)
	opts, _ := json.Marshal(b.Options)
	_, err := db.Exec(`INSERT INTO restore_batches
		(batch_id, batch_key, resume_key, created_at, scope, account_id, auth_scope, task_id, source, label, dates, options)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		b.BatchID, b.BatchKey, b.ResumeKey, isoUTC(b.CreatedAt), b.Scope, b.AccountID, b.AuthScope, b.TaskID, b.Source, b.Label, string(dates), string(opts))
	return err
}

// batchColumnList returns the SELECT list for restore_batches. A ledger
// opened read-only before its first read-write open has not gained the
// newer columns yet, so those read as empty strings.
func batchColumnList(db *sql.DB, alias string) string {
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}
	col := func(name string) string {
		if ledgerHasColumn(db, "restore_batches", name) {
			return prefix + name
		}
		return "'' AS " + name
	}
	return strings.Join([]string{
		prefix + "batch_id", prefix + "batch_key", col("resume_key"), prefix + "created_at", prefix + "scope",
		prefix + "account_id", col("auth_scope"), prefix + "task_id", prefix + "source", prefix + "label",
		prefix + "dates", prefix + "options", prefix + "last_refresh_at",
	}, ", ")
}

func ledgerHasColumn(db *sql.DB, table, column string) bool {
	var name string
	err := db.QueryRow(`SELECT name FROM pragma_table_info(?) WHERE name = ?`, table, column).Scan(&name)
	return err == nil
}

func scanBatch(row interface{ Scan(dest ...any) error }) (*restoreBatch, error) {
	var b restoreBatch
	var created, dates, opts string
	var refresh sql.NullString
	if err := row.Scan(&b.BatchID, &b.BatchKey, &b.ResumeKey, &created, &b.Scope, &b.AccountID, &b.AuthScope, &b.TaskID, &b.Source, &b.Label, &dates, &opts, &refresh); err != nil {
		return nil, err
	}
	b.CreatedAt, _ = time.Parse(time.RFC3339, created)
	_ = json.Unmarshal([]byte(dates), &b.Dates)
	_ = json.Unmarshal([]byte(opts), &b.Options)
	if refresh.Valid && refresh.String != "" {
		if t, err := time.Parse(time.RFC3339, refresh.String); err == nil {
			b.LastRefreshAt = &t
		}
	}
	return &b, nil
}

// ledgerGetBatch returns nil, nil when the batch id is unknown.
func ledgerGetBatch(db *sql.DB, batchID string) (*restoreBatch, error) {
	b, err := scanBatch(db.QueryRow(`SELECT `+batchColumnList(db, "")+` FROM restore_batches WHERE batch_id = ?`, batchID))
	if errors.Is(err, sql.ErrNoRows) || isMissingTableErr(err) {
		return nil, nil
	}
	return b, err
}

// ledgerFindBatchByKey returns the newest batch with the given batch key.
func ledgerFindBatchByKey(db *sql.DB, key string) (*restoreBatch, error) {
	return ledgerFindBatchBy(db, "batch_key", key)
}

// ledgerFindBatchByResumeKey returns the newest batch with the given
// resume key.
func ledgerFindBatchByResumeKey(db *sql.DB, key string) (*restoreBatch, error) {
	return ledgerFindBatchBy(db, "resume_key", key)
}

func ledgerFindBatchBy(db *sql.DB, column, key string) (*restoreBatch, error) {
	if key == "" {
		return nil, nil
	}
	b, err := scanBatch(db.QueryRow(`SELECT `+batchColumnList(db, "")+` FROM restore_batches WHERE `+column+` = ? ORDER BY created_at DESC LIMIT 1`, key))
	if errors.Is(err, sql.ErrNoRows) || isMissingTableErr(err) {
		return nil, nil
	}
	return b, err
}

func ledgerTouchBatch(db *sql.DB, batchID string, at time.Time) error {
	_, err := db.Exec(`UPDATE restore_batches SET last_refresh_at = ? WHERE batch_id = ?`, isoUTC(at), batchID)
	return err
}

// ledgerInsertJob inserts a PLANNED row. An existing (batch, entity,
// requested) row is left untouched so --resume keeps its status and its
// stored resolved backup; the older (batch, entity, resolved) constraint
// still folds two dates that resolve to one backup into a single row.
func ledgerInsertJob(db *sql.DB, j restoreJob) error {
	_, err := db.Exec(`INSERT OR IGNORE INTO restore_jobs
		(batch_id, entity, ext_id, requested, resolved_raw, resolved, sub_source, status, submit_status, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		j.BatchID, j.Entity, j.ExtID, j.Requested, j.ResolvedRaw, j.Resolved, j.SubSource, jobPlanned, jobPlanned, isoUTC(time.Now()))
	return err
}

const jobBaseColumns = `id, batch_id, entity, ext_id, requested, resolved_raw, resolved, sub_source, status,
	http_status, response, error, task_id, progress, submitted_at, updated_at`

// jobColumnList returns the SELECT list scanJob reads. A ledger opened
// read-only before its first read-write open has no submit_status column
// yet, and a row an older binary wrote since has it empty; both derive the
// outcome from the older columns.
func jobColumnList(db *sql.DB) string {
	return jobBaseColumns + ", " + submitStatusExpr(db, "") + " AS submit_status"
}

// submitStatusExpr is the SQL for a restore_jobs row's submission outcome.
func submitStatusExpr(db *sql.DB, prefix string) string {
	if !ledgerHasColumn(db, "restore_jobs", "submit_status") {
		return store.LegacySubmitStatusSQL(prefix)
	}
	return "COALESCE(NULLIF(" + prefix + "submit_status, ''), " + store.LegacySubmitStatusSQL(prefix) + ")"
}

func scanJob(rows *sql.Rows) (restoreJob, error) {
	var j restoreJob
	var httpStatus sql.NullInt64
	var response, errText, taskID, progress, submitted sql.NullString
	var updated string
	if err := rows.Scan(&j.ID, &j.BatchID, &j.Entity, &j.ExtID, &j.Requested, &j.ResolvedRaw, &j.Resolved, &j.SubSource, &j.Status,
		&httpStatus, &response, &errText, &taskID, &progress, &submitted, &updated, &j.SubmitStatus); err != nil {
		return j, err
	}
	j.HTTPStatus = int(httpStatus.Int64)
	j.Response, j.Error, j.TaskID, j.Progress = response.String, errText.String, taskID.String, progress.String
	if submitted.Valid && submitted.String != "" {
		if t, err := time.Parse(time.RFC3339, submitted.String); err == nil {
			j.SubmittedAt = &t
		}
	}
	j.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return j, nil
}

// ledgerListJobs returns a batch's rows in insertion order, optionally
// filtered by status (case-insensitive).
func ledgerListJobs(db *sql.DB, batchID, status string) ([]restoreJob, error) {
	query := `SELECT ` + jobColumnList(db) + ` FROM restore_jobs WHERE batch_id = ?`
	args := []any{batchID}
	if status != "" {
		query += ` AND UPPER(status) = ?`
		args = append(args, strings.ToUpper(status))
	}
	query += ` ORDER BY id ASC`
	rows, err := db.Query(query, args...)
	if err != nil {
		if isMissingTableErr(err) {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []restoreJob
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// errRowNotSendable reports that a row changed status after it was chosen
// for sending, for example because another run already claimed it.
var errRowNotSendable = errors.New("ledger row is no longer PLANNED, FAILED, or UNKNOWN, or has a RESTORE task; not sending it")

// ledgerMarkSubmitting moves a sendable row to SUBMITTING and commits it
// before the request goes out. The guard on the submission outcome makes
// the update a compare-and-swap, so two runs of the same batch cannot both
// send a row, and a row with a recorded RESTORE task is never sent again
// whatever its outcome reads.
func ledgerMarkSubmitting(db *sql.DB, id int64, at time.Time) error {
	res, err := db.Exec(`UPDATE restore_jobs SET status = ?, submit_status = ?, submitted_at = ?, updated_at = ?
		WHERE id = ? AND submit_status IN (?, ?, ?) AND COALESCE(task_id, '') = ''`,
		jobSubmitting, jobSubmitting, isoUTC(at), isoUTC(time.Now()), id, jobPlanned, jobFailed, jobUnknown)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n != 1 {
		return errRowNotSendable
	}
	return nil
}

// ledgerRecoverInFlight turns rows a crashed run left SUBMITTING into
// UNKNOWN and reports how many it changed.
func ledgerRecoverInFlight(db *sql.DB, batchID string) (int64, error) {
	res, err := db.Exec(`UPDATE restore_jobs SET status = ?, submit_status = ?, error = ?, updated_at = ? WHERE batch_id = ? AND submit_status = ?`,
		jobUnknown, jobUnknown, recoveredInFlightError, isoUTC(time.Now()), batchID, jobSubmitting)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func ledgerUpdateJobSubmit(db *sql.DB, id int64, status string, httpStatus int, response, errText string, submittedAt time.Time) error {
	_, err := db.Exec(`UPDATE restore_jobs SET status = ?, submit_status = ?, http_status = ?, response = ?, error = ?, submitted_at = ?, updated_at = ? WHERE id = ?`,
		status, status, httpStatus, response, errText, isoUTC(submittedAt), isoUTC(time.Now()), id)
	return err
}

// ledgerUpdateJobWatch records what 'restore watch' saw. status is the
// RESTORE task status (or UNMATCHED) and only ever lands in the status
// column. matched means a RESTORE task was found for the row, which proves
// the API accepted it, so an UNKNOWN or SUBMITTING outcome is settled as
// SUBMITTED; no other outcome is changed.
func ledgerUpdateJobWatch(db *sql.DB, id int64, status, taskID, progress string, matched bool) error {
	_, err := db.Exec(`UPDATE restore_jobs SET status = ?, task_id = ?, progress = ?, updated_at = ?,
		submit_status = CASE WHEN ? AND submit_status IN (?, ?) THEN ? ELSE submit_status END
		WHERE id = ?`,
		status, taskID, progress, isoUTC(time.Now()), matched, jobUnknown, jobSubmitting, jobSubmitted, id)
	return err
}

// ledgerAcceptedRowsByKey finds the newest batch recorded under any of the
// given batch keys that holds rows the API accepted or may have accepted
// (SUBMITTED, SUBMITTING, UNKNOWN, or a recorded RESTORE task) and counts
// those rows. It returns "", 0 when there is none.
func ledgerAcceptedRowsByKey(db *sql.DB, keys ...string) (string, int, error) {
	var nonEmpty []any
	for _, k := range keys {
		if k != "" {
			nonEmpty = append(nonEmpty, k)
		}
	}
	if len(nonEmpty) == 0 {
		return "", 0, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?, ", len(nonEmpty)), ", ")
	outcome := submitStatusExpr(db, "j.")
	query := `SELECT b.batch_id, COUNT(j.id) FROM restore_batches b JOIN restore_jobs j ON j.batch_id = b.batch_id
		WHERE b.batch_key IN (` + placeholders + `) AND (` + outcome + ` IN (?, ?, ?) OR COALESCE(j.task_id, '') <> '')
		GROUP BY b.batch_id ORDER BY b.created_at DESC LIMIT 1`
	args := append(nonEmpty, jobSubmitted, jobSubmitting, jobUnknown)
	var batchID string
	var n int
	err := db.QueryRow(query, args...).Scan(&batchID, &n)
	if errors.Is(err, sql.ErrNoRows) || isMissingTableErr(err) {
		return "", 0, nil
	}
	return batchID, n, err
}

// historyFilter narrows 'restore history' output.
type historyFilter struct {
	account string
	status  string
	since   time.Time
	limit   int
}

// ledgerListBatches aggregates per-batch counts newest-first. submitted
// counts rows the API accepted, whatever their task did afterwards; failed
// counts rows that show FAILED or FAILED_REDIRECT now, whether they were
// rejected before running or their RESTORE task failed after being
// accepted (so such a row is counted in both); unknown counts rows whose
// submission was never confirmed.
func ledgerListBatches(db *sql.DB, f historyFilter) ([]batchSummary, error) {
	authScope := "b.auth_scope"
	if !ledgerHasColumn(db, "restore_batches", "auth_scope") {
		authScope = "''"
	}
	outcome := submitStatusExpr(db, "j.")
	query := `SELECT b.batch_id, b.created_at, b.scope, b.account_id, ` + authScope + `, b.task_id, b.source, b.label, b.last_refresh_at,
			COUNT(j.id),
			COALESCE(SUM(CASE WHEN j.id IS NOT NULL AND ` + outcome + ` = ? THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN j.status IN (?, ?) THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN j.id IS NOT NULL AND ` + outcome + ` IN (?, ?) THEN 1 ELSE 0 END), 0)
		FROM restore_batches b LEFT JOIN restore_jobs j ON j.batch_id = b.batch_id WHERE 1=1`
	args := []any{jobSubmitted, jobFailed, jobFailedRedirect, jobUnknown, jobSubmitting}
	if f.account != "" {
		query += ` AND b.account_id = ?`
		args = append(args, f.account)
	}
	if !f.since.IsZero() {
		query += ` AND b.created_at >= ?`
		args = append(args, isoUTC(f.since))
	}
	query += ` GROUP BY b.batch_id`
	if f.status != "" {
		query += ` HAVING SUM(CASE WHEN UPPER(j.status) = ? THEN 1 ELSE 0 END) > 0`
		args = append(args, strings.ToUpper(f.status))
	}
	query += ` ORDER BY b.created_at DESC`
	if f.limit > 0 {
		query += ` LIMIT ?`
		args = append(args, f.limit)
	}
	rows, err := db.Query(query, args...)
	if err != nil {
		if isMissingTableErr(err) {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []batchSummary
	for rows.Next() {
		var s batchSummary
		var created string
		var refresh sql.NullString
		if err := rows.Scan(&s.BatchID, &created, &s.Scope, &s.AccountID, &s.AuthScope, &s.TaskID, &s.Source, &s.Label, &refresh, &s.Rows, &s.Submitted, &s.Failed, &s.Unknown); err != nil {
			return nil, err
		}
		s.CreatedAt, _ = time.Parse(time.RFC3339, created)
		if refresh.Valid && refresh.String != "" {
			if t, err := time.Parse(time.RFC3339, refresh.String); err == nil {
				s.LastRefreshAt = &t
			}
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
