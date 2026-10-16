// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"jira-service-management-pp-cli/internal/store"
)

// Local-store reads shared by sla-report and client. Synced requests carry
// their SLA inline (sync asks for expand=sla).

type localRequest struct {
	IssueKey      string     `json:"issueKey"`
	ServiceDeskID flexString `json:"serviceDeskId"`
	RequestTypeID flexString `json:"requestTypeId"`
	CreatedDate   jsmDate    `json:"createdDate"`
	Reporter      jsmUser    `json:"reporter"`
	CurrentStatus struct {
		Status         string `json:"status"`
		StatusCategory string `json:"statusCategory"`
	} `json:"currentStatus"`
	SLA struct {
		Values []jsmSLA `json:"values"`
	} `json:"sla"`
}

func (r localRequest) open() bool { return !strings.EqualFold(r.CurrentStatus.StatusCategory, "DONE") }

// openMirror opens the local store read-only (dbPath "" means the default).
// A missing store returns a nil db and the sync hint to show instead.
func openMirror(ctx context.Context, dbPath, syncResources string) (*store.Store, string, error) {
	if dbPath == "" {
		dbPath = defaultDBPath("jira-service-management-cli")
	}
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil, fmt.Sprintf("no local mirror at %s; run: jira-service-management-cli sync --resources %s --db %s", dbPath, syncResources, dbPath), nil
	}
	db, err := store.OpenReadOnlyContext(ctx, dbPath)
	if err != nil {
		return nil, "", fmt.Errorf("opening database: %w", err)
	}
	return db, "", nil
}

// eachResource streams every data blob for the given resource types, so a
// large table is never held in memory as raw JSON.
func eachResource(ctx context.Context, db *store.Store, fn func(json.RawMessage), types ...string) error {
	ph := strings.TrimSuffix(strings.Repeat("?,", len(types)), ",")
	args := make([]any, len(types))
	for i, t := range types {
		args[i] = t
	}
	rows, err := db.DB().QueryContext(ctx, `SELECT data FROM resources WHERE resource_type IN (`+ph+`)`, args...)
	if err != nil {
		return fmt.Errorf("query: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return fmt.Errorf("scan: %w", err)
		}
		fn(data)
	}
	return rows.Err()
}

// resourceData collects every data blob for the given (small) resource types.
func resourceData(ctx context.Context, db *store.Store, types ...string) ([]json.RawMessage, error) {
	out := make([]json.RawMessage, 0)
	err := eachResource(ctx, db, func(raw json.RawMessage) { out = append(out, raw) }, types...)
	return out, err
}

// localRequests decodes synced requests row by row; rows that do not decode
// or carry no issueKey are skipped.
func localRequests(ctx context.Context, db *store.Store) ([]localRequest, error) {
	out := make([]localRequest, 0)
	err := eachResource(ctx, db, func(raw json.RawMessage) {
		var r localRequest
		if json.Unmarshal(raw, &r) == nil && r.IssueKey != "" {
			out = append(out, r)
		}
	}, "request")
	return out, err
}

// nameIndex maps id -> display name for synced desks and request types.
func nameIndex(ctx context.Context, db *store.Store) (map[string]string, map[string]string) {
	desks, types := map[string]string{}, map[string]string{}
	if raws, err := resourceData(ctx, db, "servicedesk"); err == nil {
		for _, raw := range raws {
			var d struct {
				ID          flexString `json:"id"`
				ProjectName string     `json:"projectName"`
			}
			if json.Unmarshal(raw, &d) == nil {
				desks[string(d.ID)] = d.ProjectName
			}
		}
	}
	if raws, err := resourceData(ctx, db, "requesttype", "servicedesk_requesttype"); err == nil {
		for _, raw := range raws {
			var t struct {
				ID   flexString `json:"id"`
				Name string     `json:"name"`
			}
			if json.Unmarshal(raw, &t) == nil {
				types[string(t.ID)] = t.Name
			}
		}
	}
	return desks, types
}

type slaGroup struct {
	Group           string  `json:"group"`
	Name            string  `json:"name,omitempty"`
	Cycles          int     `json:"cycles"`
	Breached        int     `json:"breached"`
	BreachRate      float64 `json:"breach_rate"`
	MedianElapsedMS int64   `json:"median_elapsed_ms"`
	MedianGoalMS    int64   `json:"median_goal_ms"`
}

// slaRollup aggregates completed SLA cycles that stopped at or after cutoffMS.
// groupBy is desk, requesttype or metric.
func slaRollup(reqs []localRequest, cutoffMS int64, groupBy string, names map[string]string) []slaGroup {
	type acc struct {
		cycles, breached int
		elapsed, goal    []int64
	}
	groups := map[string]*acc{}
	for _, r := range reqs {
		for _, s := range r.SLA.Values {
			for _, c := range s.CompletedCycles {
				if c.StopTime == nil || c.StopTime.EpochMillis < cutoffMS {
					continue
				}
				var key string
				switch groupBy {
				case "requesttype":
					key = string(r.RequestTypeID)
				case "metric":
					key = s.Name
				default:
					key = string(r.ServiceDeskID)
				}
				a := groups[key]
				if a == nil {
					a = &acc{}
					groups[key] = a
				}
				a.cycles++
				if c.Breached {
					a.breached++
				}
				a.elapsed = append(a.elapsed, c.ElapsedTime.Millis)
				a.goal = append(a.goal, c.GoalDuration.Millis)
			}
		}
	}
	out := make([]slaGroup, 0, len(groups))
	for key, a := range groups {
		g := slaGroup{Group: key, Name: names[key], Cycles: a.cycles, Breached: a.breached,
			MedianElapsedMS: medianInt64(a.elapsed), MedianGoalMS: medianInt64(a.goal)}
		if a.cycles > 0 {
			g.BreachRate = float64(a.breached) / float64(a.cycles)
		}
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].BreachRate != out[j].BreachRate {
			return out[i].BreachRate > out[j].BreachRate
		}
		// Equal rates: more breached cycles first, then more cycles, so a
		// group with 3/3 breached outranks one with 1/1.
		if out[i].Breached != out[j].Breached {
			return out[i].Breached > out[j].Breached
		}
		if out[i].Cycles != out[j].Cycles {
			return out[i].Cycles > out[j].Cycles
		}
		return out[i].Group < out[j].Group
	})
	return out
}

// humanDuration renders a duration for human tables as days+hours, hours+minutes
// or minutes ("4d4h", "9d", "3h20m", "45m") instead of Go's "100h0m0s".
// JSON output keeps the raw millisecond/hour fields.
func humanDuration(d time.Duration) string {
	if d < 0 {
		if d == math.MinInt64 {
			d++ // -MinInt64 overflows back to itself
		}
		return "-" + humanDuration(-d)
	}
	if d < time.Minute {
		return "<1m"
	}
	days := int64(d / (24 * time.Hour))
	hours := int64((d % (24 * time.Hour)) / time.Hour)
	mins := int64((d % time.Hour) / time.Minute)
	switch {
	case days > 0 && hours > 0:
		return fmt.Sprintf("%dd%dh", days, hours)
	case days > 0:
		return fmt.Sprintf("%dd", days)
	case hours > 0 && mins > 0:
		return fmt.Sprintf("%dh%dm", hours, mins)
	case hours > 0:
		return fmt.Sprintf("%dh", hours)
	default:
		return fmt.Sprintf("%dm", mins)
	}
}

// plural returns "<n> <word>" with an "s" added when n != 1.
func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func medianInt64(v []int64) int64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]int64(nil), v...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	m := len(s) / 2
	if len(s)%2 == 1 {
		return s[m]
	}
	return (s[m-1] + s[m]) / 2
}

type clientPicture struct {
	Found           bool           `json:"found"`
	OrgID           string         `json:"org_id"`
	Org             string         `json:"org"`
	Members         int            `json:"members"`
	Requests        int            `json:"requests"`
	Open            int            `json:"open"`
	ByStatus        map[string]int `json:"open_by_status"`
	ByRequestType   map[string]int `json:"open_by_request_type"`
	OldestOpen      string         `json:"oldest_open,omitempty"`
	OldestOpenAgeH  int64          `json:"oldest_open_age_hours,omitempty"`
	OngoingBreached int            `json:"ongoing_breached"`
	BreachedKeys    []string       `json:"breached_keys"`
}

// clientRollup joins org members (accountIds) to request reporters.
func clientRollup(orgID, orgName string, members map[string]bool, reqs []localRequest, typeNames map[string]string, nowMS int64) clientPicture {
	p := clientPicture{Found: true, OrgID: orgID, Org: orgName, Members: len(members), ByStatus: map[string]int{}, ByRequestType: map[string]int{}, BreachedKeys: make([]string, 0)}
	var oldest int64
	for _, r := range reqs {
		if !members[r.Reporter.AccountID] {
			continue
		}
		p.Requests++
		if !r.open() {
			continue
		}
		p.Open++
		p.ByStatus[r.CurrentStatus.Status]++
		rt := typeNames[string(r.RequestTypeID)]
		if rt == "" {
			rt = string(r.RequestTypeID)
		}
		p.ByRequestType[rt]++
		if c := r.CreatedDate.EpochMillis; c > 0 && (oldest == 0 || c < oldest) {
			oldest = c
			p.OldestOpen = r.IssueKey
		}
		if risk, ok := mostUrgent(r.SLA.Values); ok && risk.Breached {
			p.OngoingBreached++
			p.BreachedKeys = append(p.BreachedKeys, r.IssueKey)
		}
	}
	if oldest > 0 && nowMS > oldest {
		p.OldestOpenAgeH = (nowMS - oldest) / 3_600_000
	}
	sort.Strings(p.BreachedKeys)
	return p
}

// orgMembers reads synced organization members (the dependent "user" table).
func orgMembers(ctx context.Context, db *store.Store, orgID string) (map[string]bool, error) {
	members := map[string]bool{}
	var exists int
	if err := db.DB().QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type='table' AND name='user'`).Scan(&exists); err != nil || exists == 0 {
		return members, err
	}
	rows, err := db.DB().QueryContext(ctx, `SELECT data FROM "user" WHERE organization_id = ?`, orgID)
	if err != nil {
		return nil, err
	}
	raws := make([]string, 0)
	for rows.Next() {
		var data sql.NullString
		if err := rows.Scan(&data); err != nil {
			_ = rows.Close()
			return nil, err
		}
		raws = append(raws, data.String)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for _, d := range raws {
		var u jsmUser
		if json.Unmarshal([]byte(d), &u) == nil && u.AccountID != "" {
			members[u.AccountID] = true
		}
	}
	return members, nil
}
