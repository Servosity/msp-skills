package insights

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ErrInputNotSynced marks an answer whose input the local store does not
// hold. A novel never reads a missing input as "none", "everyone" or 0: a list
// that needs it fails with this error and a figure that needs it is null
// (knowbe4 handfix r1-missing-input-read-as-zero, Codex r1 knowbe4 4.34.2:
// the QBR read the never-fetched account risk score as 0).
var ErrInputNotSynced = errors.New("not in the local store")

func notSynced(what, endpoint string) error {
	return fmt.Errorf("%s: %w; run `knowbe4-cli sync` (it reads %s)", what, ErrInputNotSynced, endpoint)
}

// RecipientsResource is the sync_state resource under which the phishing-test
// recipient walk (SyncRecipients, the insights half of `sync`) records its own
// completion: the walk is no generated list resource, so sync_state holds no
// other record of it.
const RecipientsResource = "pst-recipients"

// walkedFrom names, for a collection walked out of another one, the
// collections it was walked from: the recipients are fetched per synced
// phishing test, so they are complete only while the phishing tests they were
// walked from are, and only when no later sync of the tests (which may have
// added a test the walk never saw) finished after the walk.
var walkedFrom = map[string][]string{RecipientsResource: {"phishing-tests"}}

// CollectionSynced is the one predicate every novel answers through before it
// reads a collection as complete: an absence claim (untrained, never clicked,
// no enrollment, a zero count) or "no row, so none" over that collection. It
// is true only when the collection's own sync completion record says its last
// walk finished (sync_state last_attempt_complete = 1, written only by a list
// walk that reached its end), and, for a collection walked from others, those
// are complete too and did not complete after it. A row in the collection's
// table is never the proof: a detail read (`training-enrollments get 1`)
// caches one row of a collection nobody listed (knowbe4 handfix
// r2-missing-input-read-as-zero, Codex r2 knowbe4 4.34.2: one cached
// enrollment made the enrollments read as held, and a user whose enrollment
// was never fetched was listed as untrained).
func CollectionSynced(ctx context.Context, db *sql.DB, resource string) bool {
	at, ok := syncCompletedAt(ctx, db, resource)
	if !ok {
		return false
	}
	for _, parent := range walkedFrom[resource] {
		pat, pok := syncCompletedAt(ctx, db, parent)
		if !pok || at.IsZero() || pat.IsZero() || pat.After(at) {
			return false
		}
	}
	return true
}

// syncCompletedAt is when resource's last completed sync recorded its
// watermark; ok is false when no sync of it is recorded or its last attempt
// did not complete. The time is zero when the record carries none it can
// parse.
func syncCompletedAt(ctx context.Context, db *sql.DB, resource string) (time.Time, bool) {
	var last string
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(CAST(last_synced_at AS TEXT), '') FROM sync_state
		WHERE resource_type = ? AND last_attempt_complete = 1`, resource).Scan(&last); err != nil {
		return time.Time{}, false
	}
	t, _ := parseStoreTS(last)
	return t, true
}

func usersHeld(ctx context.Context, db *sql.DB) bool { return CollectionSynced(ctx, db, "users") }

func enrollmentsHeld(ctx context.Context, db *sql.DB) bool {
	return CollectionSynced(ctx, db, "training-enrollments")
}

func phishingTestsHeld(ctx context.Context, db *sql.DB) bool {
	return CollectionSynced(ctx, db, "phishing-tests")
}

// recipientsHeld: the recipient walk over a completely synced set of phishing
// tests finished without a failed test (a partial walk leaves who never
// clicked unknown).
func recipientsHeld(ctx context.Context, db *sql.DB) bool {
	return CollectionSynced(ctx, db, RecipientsResource)
}

const (
	recipientsInput  = "phishing-test recipients"
	recipientsSource = "GET /v1/phishing/security_tests/{pst_id}/recipients"
	enrollmentsInput = "training enrollments"
	enrollmentsSrc   = "GET /v1/training/enrollments"
	usersInput       = "users"
	usersSource      = "GET /v1/users"
)

func floatPtr(f float64) *float64 { return &f }
func intPtr(n int) *int           { return &n }

func derefF(p *float64) float64 {
	if p == nil {
		return -1
	}
	return *p
}

func derefI(p *int) int {
	if p == nil {
		return -1
	}
	return *p
}

// groupNamesFromData extracts the group names from a phishing_tests `data` JSON
// blob (its groups array is [{group_id, name}, ...]). Returns a space-joined
// lowercase-friendly string for substring matching.
func groupNamesFromData(data string) string {
	if strings.TrimSpace(data) == "" {
		return ""
	}
	var parsed struct {
		Groups []struct {
			Name string `json:"name"`
		} `json:"groups"`
	}
	if err := json.Unmarshal([]byte(data), &parsed); err != nil {
		return ""
	}
	names := make([]string, 0, len(parsed.Groups))
	for _, g := range parsed.Groups {
		names = append(names, g.Name)
	}
	return strings.Join(names, " | ")
}

// ---- shared loaders ---------------------------------------------------------

type recipientRow struct {
	Email         string
	PstID         int64
	ClickedAt     string
	ReportedAt    string
	DeliveredAt   string
	OpenedAt      string
	DataEnteredAt string
}

func loadRecipients(ctx context.Context, db *sql.DB) ([]recipientRow, error) {
	if err := EnsureSchema(ctx, db); err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT COALESCE(user_email,''), pst_id,
		COALESCE(clicked_at,''), COALESCE(reported_at,''), COALESCE(delivered_at,''),
		COALESCE(opened_at,''), COALESCE(data_entered_at,'') FROM "pst_recipients"`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []recipientRow{}
	for rows.Next() {
		var r recipientRow
		if err := rows.Scan(&r.Email, &r.PstID, &r.ClickedAt, &r.ReportedAt, &r.DeliveredAt, &r.OpenedAt, &r.DataEnteredAt); err != nil {
			continue
		}
		r.Email = strings.ToLower(strings.TrimSpace(r.Email))
		if r.Email != "" {
			out = append(out, r)
		}
	}
	return out, rows.Err()
}

type userInfo struct {
	Email      string  `json:"email"`
	Name       string  `json:"name"`
	RiskScore  float64 `json:"current_risk_score"`
	PhishProne float64 `json:"phish_prone_percentage"`
	JoinedOn   string  `json:"joined_on"`
	Status     string  `json:"status"`
}

func loadUsers(ctx context.Context, db *sql.DB) (map[string]userInfo, []userInfo, error) {
	rows, err := db.QueryContext(ctx, `SELECT COALESCE(email,''),
		COALESCE(NULLIF(TRIM(COALESCE(first_name,'')||' '||COALESCE(last_name,'')),''), email),
		COALESCE(current_risk_score,0), COALESCE(phish_prone_percentage,0),
		COALESCE(joined_on,''), COALESCE(status,'') FROM "users"`)
	if err != nil {
		// users not synced
		return map[string]userInfo{}, []userInfo{}, nil
	}
	defer rows.Close()
	byEmail := map[string]userInfo{}
	all := []userInfo{}
	for rows.Next() {
		var u userInfo
		if err := rows.Scan(&u.Email, &u.Name, &u.RiskScore, &u.PhishProne, &u.JoinedOn, &u.Status); err != nil {
			continue
		}
		u.Email = strings.ToLower(strings.TrimSpace(u.Email))
		if u.Email == "" {
			continue
		}
		byEmail[u.Email] = u
		all = append(all, u)
	}
	return byEmail, all, rows.Err()
}

// trainedEmails returns the set of lowercased emails with at least one enrollment
// whose status is Passed/Completed, plus the set of all enrolled emails.
func trainingEmailSets(ctx context.Context, db *sql.DB) (passed map[string]bool, enrolled map[string]bool) {
	passed, enrolled = map[string]bool{}, map[string]bool{}
	rows, err := db.QueryContext(ctx, `SELECT lower(COALESCE(json_extract(data,'$.user.email'),'')), COALESCE(status,'') FROM "training_enrollments"`)
	if err != nil {
		return passed, enrolled
	}
	defer rows.Close()
	for rows.Next() {
		var email, status string
		if err := rows.Scan(&email, &status); err != nil {
			continue
		}
		email = strings.TrimSpace(email)
		if email == "" {
			continue
		}
		enrolled[email] = true
		switch strings.ToLower(strings.TrimSpace(status)) {
		case "passed", "completed":
			passed[email] = true
		}
	}
	return passed, enrolled
}

// notPassedCount returns email -> count of enrollments not in a Passed/Completed state.
func notPassedCounts(ctx context.Context, db *sql.DB) map[string]int {
	out := map[string]int{}
	rows, err := db.QueryContext(ctx, `SELECT lower(COALESCE(json_extract(data,'$.user.email'),'')), COALESCE(status,'') FROM "training_enrollments"`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var email, status string
		if err := rows.Scan(&email, &status); err != nil {
			continue
		}
		email = strings.TrimSpace(email)
		if email == "" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(status)) {
		case "passed", "completed":
		default:
			out[email]++
		}
	}
	return out
}

func cutoff(window string) (time.Time, error) {
	d, err := ParseWindow(window)
	if err != nil {
		return time.Time{}, err
	}
	if d == 0 {
		return time.Time{}, nil
	}
	return time.Now().UTC().Add(-d), nil
}

// ---- 1. repeat-clickers -----------------------------------------------------

// ClickerRow is one clicker. The risk figures come from the synced user
// record and are null when the store holds no user with that email (never 0).
type ClickerRow struct {
	Email         string   `json:"user_email"`
	Name          string   `json:"name,omitempty"`
	ClickedPSTs   int      `json:"clicked_psts"`
	ReportedPSTs  int      `json:"reported_psts"`
	CurrentRisk   *float64 `json:"current_risk_score"`
	PhishPronePct *float64 `json:"phish_prone_percentage"`
}

func RepeatClickers(ctx context.Context, db *sql.DB, minClicks, top int, since string) ([]ClickerRow, error) {
	cut, err := cutoff(since)
	if err != nil {
		return nil, err
	}
	if !recipientsHeld(ctx, db) {
		return nil, notSynced(recipientsInput, recipientsSource)
	}
	recips, err := loadRecipients(ctx, db)
	if err != nil {
		return nil, err
	}
	users, _, _ := loadUsers(ctx, db)
	clicked := map[string]map[int64]bool{}
	reported := map[string]map[int64]bool{}
	for _, r := range recips {
		if occurred(r.ClickedAt, cut) {
			if clicked[r.Email] == nil {
				clicked[r.Email] = map[int64]bool{}
			}
			clicked[r.Email][r.PstID] = true
		}
		if occurred(r.ReportedAt, cut) {
			if reported[r.Email] == nil {
				reported[r.Email] = map[int64]bool{}
			}
			reported[r.Email][r.PstID] = true
		}
	}
	if minClicks < 1 {
		minClicks = 2
	}
	out := []ClickerRow{}
	for email, psts := range clicked {
		if len(psts) < minClicks {
			continue
		}
		row := ClickerRow{Email: email, ClickedPSTs: len(psts), ReportedPSTs: len(reported[email])}
		if u, ok := users[email]; ok {
			row.Name = u.Name
			row.CurrentRisk = floatPtr(u.RiskScore)
			row.PhishPronePct = floatPtr(u.PhishProne)
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ClickedPSTs != out[j].ClickedPSTs {
			return out[i].ClickedPSTs > out[j].ClickedPSTs
		}
		return derefF(out[i].CurrentRisk) > derefF(out[j].CurrentRisk)
	})
	return limitClickers(out, top), nil
}

// ---- 2. untrained-clickers --------------------------------------------------

func UntrainedClickers(ctx context.Context, db *sql.DB, top int, since string) ([]ClickerRow, error) {
	cut, err := cutoff(since)
	if err != nil {
		return nil, err
	}
	if !recipientsHeld(ctx, db) {
		return nil, notSynced(recipientsInput, recipientsSource)
	}
	if !enrollmentsHeld(ctx, db) {
		return nil, notSynced(enrollmentsInput, enrollmentsSrc)
	}
	recips, err := loadRecipients(ctx, db)
	if err != nil {
		return nil, err
	}
	users, _, _ := loadUsers(ctx, db)
	passed, _ := trainingEmailSets(ctx, db)
	clicked := map[string]map[int64]bool{}
	for _, r := range recips {
		if occurred(r.ClickedAt, cut) {
			if clicked[r.Email] == nil {
				clicked[r.Email] = map[int64]bool{}
			}
			clicked[r.Email][r.PstID] = true
		}
	}
	out := []ClickerRow{}
	for email, psts := range clicked {
		if passed[email] {
			continue // they clicked, but they have completed training
		}
		row := ClickerRow{Email: email, ClickedPSTs: len(psts)}
		if u, ok := users[email]; ok {
			row.Name = u.Name
			row.CurrentRisk = floatPtr(u.RiskScore)
			row.PhishPronePct = floatPtr(u.PhishProne)
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		if a, b := derefF(out[i].CurrentRisk), derefF(out[j].CurrentRisk); a != b {
			return a > b
		}
		return out[i].ClickedPSTs > out[j].ClickedPSTs
	})
	return limitClickers(out, top), nil
}

func limitClickers(in []ClickerRow, top int) []ClickerRow {
	if top > 0 && len(in) > top {
		return in[:top]
	}
	return in
}

// ---- 3. coverage-gaps -------------------------------------------------------

type CoverageRow struct {
	Email       string  `json:"email"`
	Name        string  `json:"name,omitempty"`
	CurrentRisk float64 `json:"current_risk_score"`
	JoinedOn    string  `json:"joined_on,omitempty"`
	GapType     string  `json:"gap_type"`
}

// CoverageGaps finds active users not covered by training and/or phishing. gapType
// is "training", "phishing", or "" (both). joinedWithin filters to recently-joined
// users when non-empty (e.g. "30d").
func CoverageGaps(ctx context.Context, db *sql.DB, gapType, joinedWithin string, top int) ([]CoverageRow, error) {
	joinCut, err := cutoff(joinedWithin)
	if err != nil {
		return nil, err
	}
	gapType = strings.ToLower(strings.TrimSpace(gapType))
	// A gap is the absence of a record: it is only an answer when the
	// records it is judged against are in the store.
	if !usersHeld(ctx, db) {
		return nil, notSynced(usersInput, usersSource)
	}
	if gapType != "phishing" && !enrollmentsHeld(ctx, db) {
		return nil, notSynced(enrollmentsInput, enrollmentsSrc)
	}
	if gapType != "training" && !recipientsHeld(ctx, db) {
		return nil, notSynced(recipientsInput, recipientsSource)
	}
	_, users, err := loadUsers(ctx, db)
	if err != nil {
		return nil, err
	}
	_, enrolled := trainingEmailSets(ctx, db)
	phished := map[string]bool{}
	recips, _ := loadRecipients(ctx, db)
	for _, r := range recips {
		phished[r.Email] = true
	}
	out := []CoverageRow{}
	for _, u := range users {
		if strings.EqualFold(u.Status, "archived") {
			continue
		}
		if !joinCut.IsZero() {
			if jt, ok := parseTS(u.JoinedOn); !ok || jt.Before(joinCut) {
				continue
			}
		}
		missTraining := !enrolled[u.Email]
		missPhishing := !phished[u.Email]
		var gap string
		switch gapType {
		case "training":
			if !missTraining {
				continue
			}
			gap = "training"
		case "phishing":
			if !missPhishing {
				continue
			}
			gap = "phishing"
		default:
			switch {
			case missTraining && missPhishing:
				gap = "training+phishing"
			case missTraining:
				gap = "training"
			case missPhishing:
				gap = "phishing"
			default:
				continue
			}
		}
		out = append(out, CoverageRow{Email: u.Email, Name: u.Name, CurrentRisk: u.RiskScore, JoinedOn: u.JoinedOn, GapType: gap})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CurrentRisk > out[j].CurrentRisk })
	if top > 0 && len(out) > top {
		out = out[:top]
	}
	return out, nil
}

// ---- 4. risk-leaderboard ----------------------------------------------------

// LeaderboardRow is one user. The phishing counts are null when the store
// holds no phishing-test recipients, open_trainings when it holds no training
// enrollments (never 0).
type LeaderboardRow struct {
	Email         string  `json:"email"`
	Name          string  `json:"name,omitempty"`
	CurrentRisk   float64 `json:"current_risk_score"`
	PhishPronePct float64 `json:"phish_prone_percentage"`
	ClickedPSTs   *int    `json:"clicked_psts"`
	ReportedPSTs  *int    `json:"reported_psts"`
	OpenTrainings *int    `json:"open_trainings"`
}

func RiskLeaderboard(ctx context.Context, db *sql.DB, top int) ([]LeaderboardRow, error) {
	if !usersHeld(ctx, db) {
		return nil, notSynced(usersInput, usersSource)
	}
	haveRecipients, haveEnrollments := recipientsHeld(ctx, db), enrollmentsHeld(ctx, db)
	_, users, err := loadUsers(ctx, db)
	if err != nil {
		return nil, err
	}
	recips, _ := loadRecipients(ctx, db)
	clicked := map[string]map[int64]bool{}
	reported := map[string]map[int64]bool{}
	for _, r := range recips {
		if occurred(r.ClickedAt, time.Time{}) {
			if clicked[r.Email] == nil {
				clicked[r.Email] = map[int64]bool{}
			}
			clicked[r.Email][r.PstID] = true
		}
		if occurred(r.ReportedAt, time.Time{}) {
			if reported[r.Email] == nil {
				reported[r.Email] = map[int64]bool{}
			}
			reported[r.Email][r.PstID] = true
		}
	}
	openT := notPassedCounts(ctx, db)
	out := []LeaderboardRow{}
	for _, u := range users {
		row := LeaderboardRow{
			Email:         u.Email,
			Name:          u.Name,
			CurrentRisk:   u.RiskScore,
			PhishPronePct: u.PhishProne,
		}
		if haveRecipients {
			row.ClickedPSTs = intPtr(len(clicked[u.Email]))
			row.ReportedPSTs = intPtr(len(reported[u.Email]))
		}
		if haveEnrollments {
			row.OpenTrainings = intPtr(openT[u.Email])
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CurrentRisk != out[j].CurrentRisk {
			return out[i].CurrentRisk > out[j].CurrentRisk
		}
		return derefI(out[i].ClickedPSTs) > derefI(out[j].ClickedPSTs)
	})
	if top > 0 && len(out) > top {
		out = out[:top]
	}
	return out, nil
}

// ---- 5. risk-drift ----------------------------------------------------------

type DriftRow struct {
	EntityType string  `json:"entity_type"`
	EntityID   string  `json:"entity_id"`
	Name       string  `json:"name,omitempty"`
	From       float64 `json:"from_risk_score"`
	To         float64 `json:"to_risk_score"`
	Delta      float64 `json:"delta"`
	FromDate   string  `json:"from_date"`
	ToDate     string  `json:"to_date"`
}

type snapPoint struct {
	score float64
	at    time.Time
	name  string
}

// RiskDrift ranks entities by how much their risk score moved between the latest
// snapshot and the snapshot nearest (latest - window). entityType filters to
// "user"/"group"/"account" or "" for all. worsenedOnly keeps positive deltas.
func RiskDrift(ctx context.Context, db *sql.DB, window, entityType string, worsenedOnly bool, top int) ([]DriftRow, error) {
	if err := EnsureSchema(ctx, db); err != nil {
		return nil, err
	}
	win, err := ParseWindow(window)
	if err != nil {
		return nil, err
	}
	if win == 0 {
		win = 90 * 24 * time.Hour
	}
	// Drift is a difference between two readings of one entity: with no
	// entity read at two different times there is no answer, never "nothing
	// moved".
	if !driftHeld(ctx, db, entityType) {
		return nil, fmt.Errorf("risk snapshots from two different syncs: %w; each `knowbe4-cli sync` takes one (run it again later)", ErrInputNotSynced)
	}
	// A snapshot without a score is no point on the series (never a 0).
	q := `SELECT entity_type, entity_id, COALESCE(entity_name,''), risk_score, snapshot_at FROM "risk_snapshots" WHERE risk_score IS NOT NULL`
	args := []any{}
	if et := strings.ToLower(strings.TrimSpace(entityType)); et != "" {
		q += ` AND entity_type = ?`
		args = append(args, et)
	}
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return []DriftRow{}, nil
	}
	defer rows.Close()
	series := map[string][]snapPoint{}
	keyType := map[string]string{}
	keyID := map[string]string{}
	for rows.Next() {
		var et, id, name, at string
		var score float64
		if err := rows.Scan(&et, &id, &name, &score, &at); err != nil {
			continue
		}
		t, ok := parseTS(at)
		if !ok {
			continue
		}
		// The account is one entity: every reading of its score (a sync's
		// GET /v1/account, an `account info` row) is a point on one series,
		// never two series the largest move is picked from.
		if et == "account" {
			id = AccountEntityID
		}
		k := et + "\x00" + id
		series[k] = append(series[k], snapPoint{score: score, at: t, name: name})
		keyType[k] = et
		keyID[k] = id
	}
	out := []DriftRow{}
	for k, pts := range series {
		if len(pts) < 2 {
			continue
		}
		sort.Slice(pts, func(i, j int) bool { return pts[i].at.Before(pts[j].at) })
		latest := pts[len(pts)-1]
		name := "" // the newest reading's name, never an older one's
		for i := len(pts) - 1; i >= 0 && name == ""; i-- {
			name = pts[i].name
		}
		target := latest.at.Add(-win)
		// baseline = latest snapshot at or before target; else the earliest.
		baseline := pts[0]
		for _, p := range pts {
			if !p.at.After(target) {
				baseline = p
			}
		}
		if baseline.at.Equal(latest.at) {
			continue
		}
		delta := latest.score - baseline.score
		if worsenedOnly && delta <= 0 {
			continue
		}
		out = append(out, DriftRow{
			EntityType: keyType[k], EntityID: keyID[k], Name: name,
			From: baseline.score, To: latest.score, Delta: round2(delta),
			FromDate: baseline.at.Format("2006-01-02"), ToDate: latest.at.Format("2006-01-02"),
		})
	}
	sort.Slice(out, func(i, j int) bool { return abs(out[i].Delta) > abs(out[j].Delta) })
	if top > 0 && len(out) > top {
		out = out[:top]
	}
	return out, nil
}

// driftHeld reports whether some entity of entityType ("" for every type)
// has risk readings at two different times, the input a drift needs. The
// count is per entity (the account is one entity, as RiskDrift reads it),
// never across entities: snapshots carry the time each row was read
// (SnapshotRisk), so users read at one time and the account at another are
// two snapshot times and no drift of either.
func driftHeld(ctx context.Context, db *sql.DB, entityType string) bool {
	var times int
	tq := `SELECT COALESCE(MAX(n), 0) FROM (SELECT COUNT(DISTINCT snapshot_at) AS n FROM "risk_snapshots" WHERE risk_score IS NOT NULL`
	targs := []any{}
	if et := strings.ToLower(strings.TrimSpace(entityType)); et != "" {
		tq += ` AND entity_type = ?`
		targs = append(targs, et)
	}
	tq += ` GROUP BY entity_type, CASE WHEN entity_type = 'account' THEN '' ELSE entity_id END)`
	if err := db.QueryRowContext(ctx, tq, targs...).Scan(&times); err != nil || times < 2 {
		return false
	}
	return true
}

// ---- 6. phish-prone-trend ---------------------------------------------------

type TrendPoint struct {
	PstID         int64   `json:"pst_id"`
	Name          string  `json:"name"`
	StartedAt     string  `json:"started_at"`
	PhishPronePct float64 `json:"phish_prone_percentage"`
}

// PhishProneTrend is the phish-prone percentage across the phishing tests in
// the window; the summary figures are null when the window holds no test.
type PhishProneTrend struct {
	Group     string       `json:"group,omitempty"`
	Points    []TrendPoint `json:"points"`
	FirstPct  *float64     `json:"first_phish_prone_percentage"`
	LastPct   *float64     `json:"last_phish_prone_percentage"`
	DeltaPct  *float64     `json:"delta_phish_prone_percentage"`
	Improving *bool        `json:"improving"`
}

func PhishProneTrendQuery(ctx context.Context, db *sql.DB, group, since string) (PhishProneTrend, error) {
	cut, err := cutoff(since)
	if err != nil {
		return PhishProneTrend{}, err
	}
	out := PhishProneTrend{Group: group, Points: []TrendPoint{}}
	if !phishingTestsHeld(ctx, db) {
		return out, notSynced("phishing tests", "GET /v1/phishing/security_tests")
	}
	rows, err := db.QueryContext(ctx, `SELECT pst_id, COALESCE(name,''), COALESCE(started_at,''), COALESCE(phish_prone_percentage,0), COALESCE(data,'') FROM "phishing_tests" WHERE pst_id IS NOT NULL`)
	if err != nil {
		return out, nil
	}
	defer rows.Close()
	groupLower := strings.ToLower(strings.TrimSpace(group))
	for rows.Next() {
		var p TrendPoint
		var data string
		if err := rows.Scan(&p.PstID, &p.Name, &p.StartedAt, &p.PhishPronePct, &data); err != nil {
			continue
		}
		if !cut.IsZero() {
			if t, ok := parseTS(p.StartedAt); !ok || t.Before(cut) {
				continue
			}
		}
		if groupLower != "" && !strings.Contains(strings.ToLower(groupNamesFromData(data)), groupLower) {
			continue
		}
		out.Points = append(out.Points, p)
	}
	sort.Slice(out.Points, func(i, j int) bool {
		ti, _ := parseTS(out.Points[i].StartedAt)
		tj, _ := parseTS(out.Points[j].StartedAt)
		return ti.Before(tj)
	})
	if len(out.Points) > 0 {
		first, last := out.Points[0].PhishPronePct, out.Points[len(out.Points)-1].PhishPronePct
		improving := last <= first
		out.FirstPct, out.LastPct, out.DeltaPct, out.Improving = floatPtr(first), floatPtr(last), floatPtr(round2(last-first)), &improving
	}
	return out, nil
}

// ---- 7. group-risk-contribution ---------------------------------------------

// ContributionRow is one group's weighted risk movement. The member count is
// the newer of the groups table's and the group's risk snapshots' (newer);
// with neither it is null, and so is the weighted contribution (never a
// weight of 0 or 1 members).
type ContributionRow struct {
	GroupID      string   `json:"group_id"`
	Name         string   `json:"name,omitempty"`
	From         float64  `json:"from_risk_score"`
	To           float64  `json:"to_risk_score"`
	Delta        float64  `json:"delta"`
	MemberCount  *int     `json:"member_count"`
	Contribution *float64 `json:"weighted_contribution"`
}

func GroupRiskContribution(ctx context.Context, db *sql.DB, window string, top int) ([]ContributionRow, error) {
	drift, err := RiskDrift(ctx, db, window, "group", false, 0)
	if err != nil {
		return nil, err
	}
	out := []ContributionRow{}
	for _, d := range drift {
		row := ContributionRow{GroupID: d.EntityID, Name: d.Name, From: d.From, To: d.To, Delta: d.Delta}
		mc := newer(
			newestReading[int](ctx, db, `SELECT member_count, CAST(synced_at AS TEXT) FROM "groups" WHERE id = ? AND member_count IS NOT NULL`, d.EntityID),
			newestReading[int](ctx, db, `SELECT member_count, CAST(snapshot_at AS TEXT) FROM "risk_snapshots" WHERE entity_type = 'group' AND entity_id = ? AND member_count IS NOT NULL`, d.EntityID),
		)
		if mc.ok {
			row.MemberCount = intPtr(mc.v)
			row.Contribution = floatPtr(round2(d.Delta * float64(maxInt(mc.v, 1))))
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return absOrNone(out[i].Contribution) > absOrNone(out[j].Contribution) })
	if top > 0 && len(out) > top {
		out = out[:top]
	}
	return out, nil
}

// absOrNone is |*p|, or -1 (after every figure) when p is null.
func absOrNone(p *float64) float64 {
	if p == nil {
		return -1
	}
	return abs(*p)
}

// ---- 8. qbr -----------------------------------------------------------------

// QBRReport is the composed quarterly review over the local store.
// Every figure whose input the local store does not hold is null and named in
// Unavailable, never a measured 0 (knowbe4 handfix
// r1-missing-input-read-as-zero, Codex r1 knowbe4 4.34.2: a tenant whose
// documented GET /v1/account answers current_risk_score 65 got
// account_risk_score 0 after `sync`, which never fetched the account).
type QBRReport struct {
	GeneratedAt        string           `json:"generated_at"`
	Window             string           `json:"window"`
	AccountRiskScore   *float64         `json:"account_risk_score"`
	AccountRiskDelta   *float64         `json:"account_risk_delta,omitempty"`
	PhishProneTrend    PhishProneTrend  `json:"phish_prone_trend"`
	TrainingCompletion *float64         `json:"training_completion_pct"`
	RepeatClickerCount *int             `json:"repeat_clicker_count"`
	UntrainedClickers  *int             `json:"untrained_clicker_count"`
	TopRiskUsers       []LeaderboardRow `json:"top_risk_users"`
	Unavailable        []string         `json:"unavailable,omitempty"`
}

func QBR(ctx context.Context, db *sql.DB, since string) (QBRReport, error) {
	rep := QBRReport{GeneratedAt: time.Now().UTC().Format(time.RFC3339), Window: since}
	if s, ok := accountRiskScore(ctx, db); ok {
		rep.AccountRiskScore = &s
	} else {
		rep.Unavailable = append(rep.Unavailable, "account_risk_score: "+notSynced("the account risk score", "GET /v1/account").Error())
	}
	if drift, err := RiskDrift(ctx, db, since, "account", false, 1); err == nil && len(drift) > 0 {
		d := drift[0].Delta
		rep.AccountRiskDelta = &d
	}
	if t, err := PhishProneTrendQuery(ctx, db, "", since); err == nil {
		rep.PhishProneTrend = t
	} else {
		rep.PhishProneTrend = PhishProneTrend{Points: []TrendPoint{}}
		rep.Unavailable = append(rep.Unavailable, "phish_prone_trend: "+err.Error())
	}
	if pct, err := trainingCompletionPct(ctx, db); err == nil {
		rep.TrainingCompletion = &pct
	} else {
		rep.Unavailable = append(rep.Unavailable, "training_completion_pct: "+err.Error())
	}
	if rc, err := RepeatClickers(ctx, db, 2, 0, since); err == nil {
		rep.RepeatClickerCount = intPtr(len(rc))
	} else {
		rep.Unavailable = append(rep.Unavailable, "repeat_clicker_count: "+err.Error())
	}
	if uc, err := UntrainedClickers(ctx, db, 0, since); err == nil {
		rep.UntrainedClickers = intPtr(len(uc))
	} else {
		rep.Unavailable = append(rep.Unavailable, "untrained_clicker_count: "+err.Error())
	}
	if lb, err := RiskLeaderboard(ctx, db, 10); err == nil {
		rep.TopRiskUsers = lb
	} else {
		rep.Unavailable = append(rep.Unavailable, "top_risk_users: "+err.Error())
	}
	if rep.TopRiskUsers == nil {
		rep.TopRiskUsers = []LeaderboardRow{}
	}
	return rep, nil
}

// accountRiskScore is the newest account risk score the store holds. Two
// sources hold it: the account risk snapshots (a sync reads GET /v1/account)
// and the account table (an `account info` read stores the same object), and
// the reading taken from the API more recently wins, whichever source holds it
// (newer). ok is false when neither holds one.
func accountRiskScore(ctx context.Context, db *sql.DB) (float64, bool) {
	r := newer(
		newestReading[float64](ctx, db, `SELECT risk_score, CAST(snapshot_at AS TEXT) FROM "risk_snapshots" WHERE entity_type = 'account' AND risk_score IS NOT NULL`),
		newestReading[float64](ctx, db, `SELECT current_risk_score, CAST(synced_at AS TEXT) FROM "account" WHERE current_risk_score IS NOT NULL`),
	)
	return r.v, r.ok
}

// stamped is one value the store holds and when it was read from the API.
type stamped[T any] struct {
	v  T
	at time.Time
	ok bool
}

// newer is the one choice between two local readings of the same value
// (knowbe4 handfix r2-stale-source-preferred, Codex r2 knowbe4 4.34.2: the
// QBR always read the account risk score from the sync snapshot, so a newer
// `account info` read of 90 lost to a snapshot of 65 taken before it). The
// reading taken from the API more recently wins, whichever source holds it;
// a reading without a recorded time loses to one with a time; a tie keeps a.
// ok is false when neither is a reading.
func newer[T any](a, b stamped[T]) stamped[T] {
	switch {
	case !a.ok:
		return b
	case !b.ok:
		return a
	case b.at.After(a.at):
		return b
	default:
		return a
	}
}

// newestReading folds every reading query returns (a value, then the time it
// was read as text) through newer. A query that fails, or rows that do not
// scan, are no reading.
func newestReading[T any](ctx context.Context, db *sql.DB, query string, args ...any) stamped[T] {
	var best stamped[T]
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return best
	}
	defer rows.Close()
	for rows.Next() {
		var v T
		var at string
		if err := rows.Scan(&v, &at); err != nil {
			continue
		}
		t, _ := parseStoreTS(at)
		best = newer(best, stamped[T]{v: v, at: t, ok: true})
	}
	if rows.Err() != nil {
		return stamped[T]{}
	}
	return best
}

// trainingCompletionPct is the share of the synced enrollments passed or
// completed. It is an error, never 0, when the enrollments are not synced
// (CollectionSynced) or the synced tenant has none.
func trainingCompletionPct(ctx context.Context, db *sql.DB) (float64, error) {
	if !enrollmentsHeld(ctx, db) {
		return 0, notSynced(enrollmentsInput, enrollmentsSrc)
	}
	var total, passed int
	rows, err := db.QueryContext(ctx, `SELECT COALESCE(status,'') FROM "training_enrollments"`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return 0, err
		}
		total++
		switch strings.ToLower(strings.TrimSpace(s)) {
		case "passed", "completed":
			passed++
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if total == 0 {
		return 0, errors.New("the synced tenant has no training enrollments, so there is no completion share")
	}
	return round2(float64(passed) / float64(total) * 100), nil
}

// ---- small numeric helpers --------------------------------------------------

func round2(f float64) float64 {
	return float64(int64(f*100+sign(f)*0.5)) / 100
}
func sign(f float64) float64 {
	if f < 0 {
		return -1
	}
	return 1
}
func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
