package insights

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"strings"
	"time"
)

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

type ClickerRow struct {
	Email         string  `json:"user_email"`
	Name          string  `json:"name,omitempty"`
	ClickedPSTs   int     `json:"clicked_psts"`
	ReportedPSTs  int     `json:"reported_psts"`
	CurrentRisk   float64 `json:"current_risk_score"`
	PhishPronePct float64 `json:"phish_prone_percentage"`
}

func RepeatClickers(ctx context.Context, db *sql.DB, minClicks, top int, since string) ([]ClickerRow, error) {
	cut, err := cutoff(since)
	if err != nil {
		return nil, err
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
			row.CurrentRisk = u.RiskScore
			row.PhishPronePct = u.PhishProne
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ClickedPSTs != out[j].ClickedPSTs {
			return out[i].ClickedPSTs > out[j].ClickedPSTs
		}
		return out[i].CurrentRisk > out[j].CurrentRisk
	})
	return limitClickers(out, top), nil
}

// ---- 2. untrained-clickers --------------------------------------------------

func UntrainedClickers(ctx context.Context, db *sql.DB, top int, since string) ([]ClickerRow, error) {
	cut, err := cutoff(since)
	if err != nil {
		return nil, err
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
			row.CurrentRisk = u.RiskScore
			row.PhishPronePct = u.PhishProne
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CurrentRisk != out[j].CurrentRisk {
			return out[i].CurrentRisk > out[j].CurrentRisk
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
	gapType = strings.ToLower(strings.TrimSpace(gapType))
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

type LeaderboardRow struct {
	Email         string  `json:"email"`
	Name          string  `json:"name,omitempty"`
	CurrentRisk   float64 `json:"current_risk_score"`
	PhishPronePct float64 `json:"phish_prone_percentage"`
	ClickedPSTs   int     `json:"clicked_psts"`
	ReportedPSTs  int     `json:"reported_psts"`
	OpenTrainings int     `json:"open_trainings"`
}

func RiskLeaderboard(ctx context.Context, db *sql.DB, top int) ([]LeaderboardRow, error) {
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
		out = append(out, LeaderboardRow{
			Email:         u.Email,
			Name:          u.Name,
			CurrentRisk:   u.RiskScore,
			PhishPronePct: u.PhishProne,
			ClickedPSTs:   len(clicked[u.Email]),
			ReportedPSTs:  len(reported[u.Email]),
			OpenTrainings: openT[u.Email],
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CurrentRisk != out[j].CurrentRisk {
			return out[i].CurrentRisk > out[j].CurrentRisk
		}
		return out[i].ClickedPSTs > out[j].ClickedPSTs
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
	q := `SELECT entity_type, entity_id, COALESCE(entity_name,''), COALESCE(risk_score,0), snapshot_at FROM "risk_snapshots"`
	args := []any{}
	if et := strings.ToLower(strings.TrimSpace(entityType)); et != "" {
		q += ` WHERE entity_type = ?`
		args = append(args, et)
	}
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return []DriftRow{}, nil
	}
	defer rows.Close()
	series := map[string][]snapPoint{}
	names := map[string]string{}
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
		k := et + "\x00" + id
		series[k] = append(series[k], snapPoint{score: score, at: t, name: name})
		if name != "" {
			names[k] = name
		}
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
			EntityType: keyType[k], EntityID: keyID[k], Name: names[k],
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

// ---- 6. phish-prone-trend ---------------------------------------------------

type TrendPoint struct {
	PstID         int64   `json:"pst_id"`
	Name          string  `json:"name"`
	StartedAt     string  `json:"started_at"`
	PhishPronePct float64 `json:"phish_prone_percentage"`
}

type PhishProneTrend struct {
	Group     string       `json:"group,omitempty"`
	Points    []TrendPoint `json:"points"`
	FirstPct  float64      `json:"first_phish_prone_percentage"`
	LastPct   float64      `json:"last_phish_prone_percentage"`
	DeltaPct  float64      `json:"delta_phish_prone_percentage"`
	Improving bool         `json:"improving"`
}

func PhishProneTrendQuery(ctx context.Context, db *sql.DB, group, since string) (PhishProneTrend, error) {
	cut, err := cutoff(since)
	if err != nil {
		return PhishProneTrend{}, err
	}
	out := PhishProneTrend{Group: group, Points: []TrendPoint{}}
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
		out.FirstPct = out.Points[0].PhishPronePct
		out.LastPct = out.Points[len(out.Points)-1].PhishPronePct
		out.DeltaPct = round2(out.LastPct - out.FirstPct)
		out.Improving = out.LastPct <= out.FirstPct
	}
	return out, nil
}

// ---- 7. group-risk-contribution ---------------------------------------------

type ContributionRow struct {
	GroupID      string  `json:"group_id"`
	Name         string  `json:"name,omitempty"`
	From         float64 `json:"from_risk_score"`
	To           float64 `json:"to_risk_score"`
	Delta        float64 `json:"delta"`
	MemberCount  int     `json:"member_count"`
	Contribution float64 `json:"weighted_contribution"`
}

func GroupRiskContribution(ctx context.Context, db *sql.DB, window string, top int) ([]ContributionRow, error) {
	drift, err := RiskDrift(ctx, db, window, "group", false, 0)
	if err != nil {
		return nil, err
	}
	members := map[string]int{}
	if rows, err := db.QueryContext(ctx, `SELECT id, COALESCE(member_count,0) FROM "groups"`); err == nil {
		for rows.Next() {
			var id string
			var mc int
			if err := rows.Scan(&id, &mc); err == nil {
				members[id] = mc
			}
		}
		_ = rows.Close()
	}
	out := []ContributionRow{}
	for _, d := range drift {
		mc := members[d.EntityID]
		out = append(out, ContributionRow{
			GroupID: d.EntityID, Name: d.Name, From: d.From, To: d.To, Delta: d.Delta,
			MemberCount: mc, Contribution: round2(d.Delta * float64(maxInt(mc, 1))),
		})
	}
	sort.Slice(out, func(i, j int) bool { return abs(out[i].Contribution) > abs(out[j].Contribution) })
	if top > 0 && len(out) > top {
		out = out[:top]
	}
	return out, nil
}

// ---- 8. qbr -----------------------------------------------------------------

type QBRReport struct {
	GeneratedAt        string           `json:"generated_at"`
	Window             string           `json:"window"`
	AccountRiskScore   float64          `json:"account_risk_score"`
	AccountRiskDelta   *float64         `json:"account_risk_delta,omitempty"`
	PhishProneTrend    PhishProneTrend  `json:"phish_prone_trend"`
	TrainingCompletion float64          `json:"training_completion_pct"`
	RepeatClickerCount int              `json:"repeat_clicker_count"`
	UntrainedClickers  int              `json:"untrained_clicker_count"`
	TopRiskUsers       []LeaderboardRow `json:"top_risk_users"`
}

func QBR(ctx context.Context, db *sql.DB, since string) (QBRReport, error) {
	rep := QBRReport{GeneratedAt: time.Now().UTC().Format(time.RFC3339), Window: since}
	if rows, err := db.QueryContext(ctx, `SELECT COALESCE(current_risk_score,0) FROM "account" LIMIT 1`); err == nil {
		if rows.Next() {
			_ = rows.Scan(&rep.AccountRiskScore)
		}
		_ = rows.Close()
	}
	if drift, err := RiskDrift(ctx, db, since, "account", false, 1); err == nil && len(drift) > 0 {
		d := drift[0].Delta
		rep.AccountRiskDelta = &d
	}
	if t, err := PhishProneTrendQuery(ctx, db, "", since); err == nil {
		rep.PhishProneTrend = t
	}
	rep.TrainingCompletion = trainingCompletionPct(ctx, db)
	if rc, err := RepeatClickers(ctx, db, 2, 0, since); err == nil {
		rep.RepeatClickerCount = len(rc)
	}
	if uc, err := UntrainedClickers(ctx, db, 0, since); err == nil {
		rep.UntrainedClickers = len(uc)
	}
	if lb, err := RiskLeaderboard(ctx, db, 10); err == nil {
		rep.TopRiskUsers = lb
	}
	if rep.TopRiskUsers == nil {
		rep.TopRiskUsers = []LeaderboardRow{}
	}
	return rep, nil
}

func trainingCompletionPct(ctx context.Context, db *sql.DB) float64 {
	var total, passed int
	rows, err := db.QueryContext(ctx, `SELECT COALESCE(status,'') FROM "training_enrollments"`)
	if err != nil {
		return 0
	}
	defer rows.Close()
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			continue
		}
		total++
		switch strings.ToLower(strings.TrimSpace(s)) {
		case "passed", "completed":
			passed++
		}
	}
	if total == 0 {
		return 0
	}
	return round2(float64(passed) / float64(total) * 100)
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
