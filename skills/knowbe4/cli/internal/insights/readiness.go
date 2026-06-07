// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored insights: report-vs-click behavior and store freshness /
// input-readiness. New in the 4.22 reprint (run 20260606-175947).

package insights

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// ---- report-rate -------------------------------------------------------------

// ReportRateRow is one entity (user or group) ranked by report-vs-click
// behavior across every synced PST recipient row.
type ReportRateRow struct {
	Entity        string  `json:"entity"`
	Name          string  `json:"name,omitempty"`
	Delivered     int     `json:"delivered"`
	Clicked       int     `json:"clicked"`
	Reported      int     `json:"reported"`
	ReportRatePct float64 `json:"report_rate_pct"`
	ClickRatePct  float64 `json:"click_rate_pct"`
	NeverReported bool    `json:"never_reported"`
}

// ReportRate ranks users (or groups with byGroup) by how often they reported
// simulated phish versus falling for them. limit bounds the result; worst=true
// sorts worst reporters first (lowest report rate, most clicks), worst=false
// sorts best reporters first. minDelivered drops entities with too few
// delivered phish for the ratio to mean anything.
func ReportRate(ctx context.Context, db *sql.DB, byGroup bool, limit int, worst bool, since string, minDelivered int) ([]ReportRateRow, error) {
	cut, err := cutoff(since)
	if err != nil {
		return nil, err
	}
	recips, err := loadRecipients(ctx, db)
	if err != nil {
		return nil, err
	}
	if minDelivered <= 0 {
		minDelivered = 1
	}

	type tally struct {
		delivered, clicked, reported int
	}
	byUser := map[string]*tally{}
	for _, r := range recips {
		if !occurred(r.DeliveredAt, cut) {
			continue
		}
		t := byUser[r.Email]
		if t == nil {
			t = &tally{}
			byUser[r.Email] = t
		}
		t.delivered++
		if occurred(r.ClickedAt, cut) {
			t.clicked++
		}
		if occurred(r.ReportedAt, cut) {
			t.reported++
		}
	}

	users, _, err := loadUsers(ctx, db)
	if err != nil {
		return nil, err
	}

	rows := []ReportRateRow{}
	if byGroup {
		emailGroups, groupNames, gerr := loadUserGroups(ctx, db)
		if gerr != nil {
			return nil, gerr
		}
		byGrp := map[int64]*tally{}
		for email, t := range byUser {
			for _, gid := range emailGroups[email] {
				g := byGrp[gid]
				if g == nil {
					g = &tally{}
					byGrp[gid] = g
				}
				g.delivered += t.delivered
				g.clicked += t.clicked
				g.reported += t.reported
			}
		}
		for gid, t := range byGrp {
			if t.delivered < minDelivered {
				continue
			}
			name := groupNames[gid]
			if name == "" {
				name = fmt.Sprintf("group %d", gid)
			}
			rows = append(rows, buildReportRateRow(name, "", t.delivered, t.clicked, t.reported))
		}
	} else {
		for email, t := range byUser {
			if t.delivered < minDelivered {
				continue
			}
			name := ""
			if u, ok := users[email]; ok {
				name = u.Name
			}
			rows = append(rows, buildReportRateRow(email, name, t.delivered, t.clicked, t.reported))
		}
	}

	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if worst {
			if a.ReportRatePct != b.ReportRatePct {
				return a.ReportRatePct < b.ReportRatePct
			}
			if a.Clicked != b.Clicked {
				return a.Clicked > b.Clicked
			}
		} else {
			if a.ReportRatePct != b.ReportRatePct {
				return a.ReportRatePct > b.ReportRatePct
			}
			if a.Clicked != b.Clicked {
				return a.Clicked < b.Clicked
			}
		}
		if a.Delivered != b.Delivered {
			return a.Delivered > b.Delivered
		}
		return a.Entity < b.Entity
	})
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	return rows, nil
}

func buildReportRateRow(entity, name string, delivered, clicked, reported int) ReportRateRow {
	row := ReportRateRow{
		Entity:        entity,
		Name:          name,
		Delivered:     delivered,
		Clicked:       clicked,
		Reported:      reported,
		NeverReported: reported == 0,
	}
	if delivered > 0 {
		row.ReportRatePct = round1(float64(reported) / float64(delivered) * 100)
		row.ClickRatePct = round1(float64(clicked) / float64(delivered) * 100)
	}
	return row
}

func round1(f float64) float64 {
	return math.Round(f*10) / 10
}

// loadUserGroups maps each user email to its group ids (from the synced users
// table's data JSON) and returns the group id -> name map from the groups table.
func loadUserGroups(ctx context.Context, db *sql.DB) (map[string][]int64, map[int64]string, error) {
	emailGroups := map[string][]int64{}
	rows, err := db.QueryContext(ctx, `SELECT COALESCE(email,''), COALESCE(data,'') FROM "users"`)
	if err != nil {
		// users not synced
		return emailGroups, map[int64]string{}, nil
	}
	defer rows.Close()
	for rows.Next() {
		var email, data string
		if err := rows.Scan(&email, &data); err != nil {
			continue
		}
		email = strings.ToLower(strings.TrimSpace(email))
		if email == "" || strings.TrimSpace(data) == "" {
			continue
		}
		var parsed struct {
			Groups []int64 `json:"groups"`
		}
		if err := json.Unmarshal([]byte(data), &parsed); err != nil {
			continue
		}
		emailGroups[email] = parsed.Groups
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	groupNames := map[int64]string{}
	grows, err := db.QueryContext(ctx, `SELECT id, COALESCE(name,'') FROM "groups"`)
	if err != nil {
		// groups not synced — ids render as "group <id>"
		return emailGroups, groupNames, nil
	}
	defer grows.Close()
	for grows.Next() {
		var id sql.NullString
		var name string
		if err := grows.Scan(&id, &name); err != nil {
			continue
		}
		if !id.Valid {
			continue
		}
		var gid int64
		if _, err := fmt.Sscanf(strings.TrimSpace(id.String), "%d", &gid); err != nil {
			continue
		}
		groupNames[gid] = name
	}
	return emailGroups, groupNames, grows.Err()
}

// parseStoreTS parses a timestamp from either the KnowBe4 API shapes (via
// parseTS) or SQLite's space-separated DATETIME default ("2006-01-02 15:04:05"),
// which the store's synced_at columns use.
func parseStoreTS(s string) (time.Time, bool) {
	if t, ok := parseTS(s); ok {
		return t, true
	}
	s = strings.TrimSpace(s)
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02 15:04:05.999999999-07:00"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// ---- freshness ----------------------------------------------------------------

// TableFreshness reports one local table's sync state.
type TableFreshness struct {
	Table      string  `json:"table"`
	Rows       int     `json:"rows"`
	LastSynced string  `json:"last_synced,omitempty"`
	AgeHours   float64 `json:"age_hours,omitempty"`
	Stale      bool    `json:"stale"`
}

// CommandReadiness reports whether one transcendence command has the local
// inputs it needs.
type CommandReadiness struct {
	Command  string   `json:"command"`
	Requires []string `json:"requires"`
	Ready    bool     `json:"ready"`
	Missing  []string `json:"missing,omitempty"`
	Hint     string   `json:"hint,omitempty"`
}

// FreshnessReport is the full store freshness / input-readiness view.
type FreshnessReport struct {
	GeneratedAt     string             `json:"generated_at"`
	StaleAfterHours float64            `json:"stale_after_hours"`
	Resources       []TableFreshness   `json:"resources"`
	Commands        []CommandReadiness `json:"commands"`
}

// Freshness reads per-table sync watermarks and row counts from the local
// store (including the insights-owned pst_recipients and risk_snapshots
// tables) and flags each transcendence command whose inputs are stale or
// never synced.
func Freshness(ctx context.Context, db *sql.DB, staleAfter time.Duration, now time.Time) (FreshnessReport, error) {
	if err := EnsureSchema(ctx, db); err != nil {
		return FreshnessReport{}, err
	}
	rep := FreshnessReport{
		GeneratedAt:     now.UTC().Format(time.RFC3339),
		StaleAfterHours: staleAfter.Hours(),
		Resources:       []TableFreshness{},
		Commands:        []CommandReadiness{},
	}

	counts := map[string]int{}
	addTable := func(name string, rowsN int, last string) {
		tf := TableFreshness{Table: name, Rows: rowsN, LastSynced: last}
		if t, ok := parseStoreTS(last); ok {
			tf.AgeHours = round1(now.UTC().Sub(t).Hours())
			tf.Stale = staleAfter > 0 && now.UTC().Sub(t) > staleAfter
		} else if rowsN == 0 {
			tf.Stale = true
		}
		counts[name] = rowsN
		rep.Resources = append(rep.Resources, tf)
	}

	// Generic synced resources ledger.
	seen := map[string]bool{}
	rows, err := db.QueryContext(ctx, `SELECT resource_type, COUNT(*), COALESCE(MAX(synced_at),'') FROM resources GROUP BY resource_type ORDER BY resource_type`)
	if err == nil {
		func() {
			defer rows.Close()
			for rows.Next() {
				var rt, last string
				var n int
				if err := rows.Scan(&rt, &n, &last); err != nil {
					continue
				}
				seen[rt] = true
				addTable(rt, n, last)
			}
		}()
	}
	// Resources the sync covers but the store has never seen.
	for _, rt := range []string{"account", "groups", "phishing-campaigns", "phishing-tests", "policies", "store-purchases", "training-campaigns", "training-enrollments", "users"} {
		if !seen[rt] {
			addTable(rt, 0, "")
		}
	}

	// Insights-owned tables.
	var prRows int
	var prLast string
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(MAX(synced_at),'') FROM "pst_recipients"`).Scan(&prRows, &prLast)
	addTable("pst_recipients", prRows, prLast)
	var rsRows, rsDistinct int
	var rsLast string
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*), COUNT(DISTINCT snapshot_at), COALESCE(MAX(snapshot_at),'') FROM "risk_snapshots"`).Scan(&rsRows, &rsDistinct, &rsLast)
	addTable("risk_snapshots", rsRows, rsLast)

	// Readiness must reflect the TYPED tables the insight commands actually
	// read (loadUsers reads "users", trainingEmailSets reads
	// "training_enrollments", ...), not just the generic resources ledger —
	// the two can diverge on partial or abnormal sync state. Table names are
	// static literals, never user input.
	for rt, tbl := range map[string]string{
		"users":                "users",
		"groups":               "groups",
		"phishing-tests":       "phishing_tests",
		"training-enrollments": "training_enrollments",
	} {
		var n int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM "`+tbl+`"`).Scan(&n); err == nil && n > counts[rt] {
			counts[rt] = n
		}
	}

	// Per-command input readiness.
	syncHint := "run `knowbe4-cli sync` to populate the local store"
	reqs := []struct {
		command  string
		requires []string
		hint     string
	}{
		{"repeat-clickers", []string{"pst_recipients"}, syncHint},
		{"untrained-clickers", []string{"pst_recipients", "training-enrollments"}, syncHint},
		{"coverage-gaps", []string{"users"}, syncHint},
		{"phish-prone-trend", []string{"phishing-tests"}, syncHint},
		{"risk-leaderboard", []string{"users"}, syncHint},
		{"report-rate", []string{"pst_recipients"}, syncHint},
		{"risk-drift", []string{"risk_snapshots"}, "snapshots accumulate once per sync; run sync on two different days to get a delta"},
		{"group-risk-contribution", []string{"risk_snapshots", "groups"}, "snapshots accumulate once per sync; run sync on two different days to get a delta"},
		{"qbr", []string{"users", "phishing-tests", "pst_recipients"}, syncHint},
	}
	for _, r := range reqs {
		cr := CommandReadiness{Command: r.command, Requires: r.requires, Ready: true}
		for _, t := range r.requires {
			if counts[t] == 0 {
				cr.Ready = false
				cr.Missing = append(cr.Missing, t)
			}
		}
		// Drift commands need two distinct snapshots to compute a delta.
		if (r.command == "risk-drift" || r.command == "group-risk-contribution") && rsDistinct < 2 && cr.Ready {
			cr.Ready = false
			cr.Missing = append(cr.Missing, "risk_snapshots (need 2+ distinct snapshot times)")
		}
		if !cr.Ready {
			cr.Hint = r.hint
		}
		rep.Commands = append(rep.Commands, cr)
	}
	return rep, nil
}
