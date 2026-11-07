// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package keeper

// keeper handfix posture-metrics: one company's posture from four
// allowlisted reports (enterprise-info users, user-report last login,
// security-audit-report, security-audit-report --breachwatch) plus its
// msp-info row, and the change events between two runs. Pure functions.

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Metrics is one company's posture. Pointer fields are nil when the source
// report could not say (never zero by default).
type Metrics struct {
	Plan             string   `json:"plan,omitempty"`
	SeatsAllocated   *int     `json:"seats_allocated,omitempty"` // -1 unlimited
	SeatsActive      *int     `json:"seats_active,omitempty"`
	SeatsOver        bool     `json:"seats_over_allocated"`
	Users            int      `json:"users"`
	Active           int      `json:"active"`
	Invited          int      `json:"invited"`
	Locked           int      `json:"locked"`
	TwoFAOn          int      `json:"twofa_on"`
	TwoFAOff         int      `json:"twofa_off"`
	TwoFACoverage    *float64 `json:"twofa_coverage_pct,omitempty"` // of active users
	Audited          int      `json:"audited_users"`
	AvgSecurityScore *float64 `json:"avg_security_score,omitempty"`
	UsersWithWeak    int      `json:"users_with_weak_passwords"`
	WeakTotal        int      `json:"weak_passwords"`
	ReusedTotal      int      `json:"reused_passwords"`
	BreachWatch      string   `json:"breachwatch"` // active, not_active, no_users, not_enabled, no_privilege
	AtRiskTotal      int      `json:"breachwatch_at_risk"`
	UsersAtRisk      int      `json:"users_at_risk"`
	InactiveDays     int      `json:"inactive_days"`
	Inactive         int      `json:"inactive_users"`
}

// UserFacts is one user's posture facts.
type UserFacts struct {
	UserID         string `json:"user_id,omitempty"` // enterprise user id: a deleted and re-invited email gets a new one
	Name           string `json:"name,omitempty"`
	Status         string `json:"status"`
	TransferStatus string `json:"transfer_status,omitempty"`
	Admin          bool   `json:"admin"`
	TwoFAEnforced  bool   `json:"twofa_enforced"`
	TwoFA          bool   `json:"twofa"`
	LastLogin      string `json:"last_login,omitempty"`
	Inactive       bool   `json:"inactive"`
	Weak           *int   `json:"weak,omitempty"`
	Reused         *int   `json:"reused,omitempty"`
	Score          *int   `json:"security_score,omitempty"`
	AtRisk         *int   `json:"at_risk,omitempty"`
}

// Inputs are the rows the posture fan-out read for one company.
type Inputs struct {
	Company     Row
	Users       []Row
	Roles       []Row
	LastLogin   []Row
	Security    []Row
	BreachWatch []Row
	// BreachWatchState is set when the BreachWatch report was refused
	// (BreachWatchUnavailable): not_enabled or no_privilege.
	BreachWatchState string
	InactiveDays     int
}

// BreachWatchUnavailable recognizes Commander's two refusals of a
// BreachWatch report (keepercommander/breachwatch.py validate_reporting,
// raised by security-audit-report --breachwatch, commands/security_audit.py):
// the add-on is not enabled for the enterprise, or the admin lacks the
// run_reports privilege. Such a company's other posture reads stand.
func BreachWatchUnavailable(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	switch msg := err.Error(); {
	case strings.Contains(msg, "BreachWatch is not enabled for this enterprise"):
		return "not_enabled", true
	case strings.Contains(msg, "You do not have the required privilege to run a BreachWatch report"):
		return "no_privilege", true
	}
	return "", false
}

func str(r Row, k string) string {
	switch v := r[k].(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	case bool:
		return strconv.FormatBool(v)
	}
	return ""
}

// num reads an integer field; ok is false when absent or not a number.
func num(r Row, k string) (int, bool) {
	switch v := r[k].(type) {
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return int(i), true
		}
		if f, err := v.Float64(); err == nil {
			return int(f), true
		}
	case string:
		if i, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return i, true
		}
	}
	return 0, false
}

func key(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

// Compute builds one company's metrics and per-user facts. A user whose
// email is missing from enterprise-info is skipped (no key to diff on).
func Compute(in Inputs) (Metrics, map[string]UserFacts) {
	m := Metrics{InactiveDays: in.InactiveDays, BreachWatch: "no_users"}
	if in.Company != nil {
		m.Plan = str(in.Company, "plan")
		if v, ok := num(in.Company, "allocated"); ok {
			m.SeatsAllocated = &v
		}
		if v, ok := num(in.Company, "active"); ok {
			m.SeatsActive = &v
		}
		if m.SeatsAllocated != nil && m.SeatsActive != nil && *m.SeatsAllocated >= 0 && *m.SeatsActive > *m.SeatsAllocated {
			m.SeatsOver = true
		}
	}
	roles := roleIndex(in.Roles)
	users := map[string]UserFacts{}
	for _, r := range in.Users {
		k := key(str(r, "email"))
		if k == "" {
			continue
		}
		f := UserFacts{UserID: str(r, "user_id"), Name: str(r, "name"), Status: str(r, "status"), TransferStatus: str(r, "transfer_status")}
		f.Admin, f.TwoFAEnforced = roleFacts(r["roles"], roles)
		// enterprise-info writes 2fa_enabled only when true (commands/enterprise.py).
		f.TwoFA = r["2fa_enabled"] == true || strings.EqualFold(str(r, "2fa_enabled"), "true")
		users[k] = f
		m.Users++
		switch strings.ToLower(f.Status) {
		case "active":
			m.Active++
			if f.TwoFA {
				m.TwoFAOn++
			} else {
				m.TwoFAOff++
			}
		case "invited":
			m.Invited++
		default: // Locked, Locked by IdP
			m.Locked++
		}
	}
	if m.Active > 0 {
		pct := round1(100 * float64(m.TwoFAOn) / float64(m.Active))
		m.TwoFACoverage = &pct
	}
	for _, r := range in.LastLogin {
		k := key(str(r, "email"))
		f, ok := users[k]
		if !ok {
			continue
		}
		f.LastLogin = str(r, "last_login")
		// user-report writes "> N DAYS AGO" when no login fell in the window
		// and "N/A" for invited users (commands/enterprise.py UserReportCommand).
		if strings.HasPrefix(strings.TrimSpace(f.LastLogin), ">") && !strings.EqualFold(f.Status, "invited") {
			f.Inactive = true
			m.Inactive++
		}
		users[k] = f
	}
	scoreSum, scored := 0, 0
	for _, r := range in.Security {
		k := key(str(r, "email"))
		f, ok := users[k]
		if !ok {
			continue
		}
		m.Audited++
		if v, ok := num(r, "weak"); ok {
			f.Weak = intp(v)
			m.WeakTotal += v
			if v > 0 {
				m.UsersWithWeak++
			}
		}
		if v, ok := num(r, "reused"); ok {
			f.Reused = intp(v)
			m.ReusedTotal += v
		}
		if v, ok := num(r, "securityScore"); ok {
			f.Score = intp(v)
			scoreSum += v
			scored++
		}
		users[k] = f
	}
	if scored > 0 {
		avg := round1(float64(scoreSum) / float64(scored))
		m.AvgSecurityScore = &avg
	}
	if in.BreachWatchState != "" {
		m.BreachWatch = in.BreachWatchState
	} else if len(in.BreachWatch) > 0 {
		// Rows without an at_risk column read as BreachWatch not active (a
		// refused report is BreachWatchState instead).
		m.BreachWatch = "not_active"
		for _, r := range in.BreachWatch {
			v, ok := num(r, "at_risk")
			if !ok {
				continue
			}
			m.BreachWatch = "active"
			m.AtRiskTotal += v
			if v > 0 {
				m.UsersAtRisk++
			}
			if f, ok := users[key(str(r, "email"))]; ok {
				f.AtRisk = intp(v)
				users[key(str(r, "email"))] = f
			}
		}
	}
	return m, users
}

// roleInfo is what posture needs from a role: admin, and whether it enforces
// 2FA (constants.py REQUIRE_TWO_FACTOR, key require_two_factor).
type roleInfo struct{ admin, enforces bool }

func truthy(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(t, "true") || t == "1"
	case json.Number:
		return t.String() == "1"
	}
	return false
}

// roleSet indexes roles by id (keeper handfix role-join-by-id, Codex r1
// class duplicate-role-name-misattribution: Commander allows two roles with
// one name) and, for a user row that lists names only, by name.
type roleSet struct {
	byID, byName map[string]roleInfo
}

// roleIndex maps role id and role name to the role's facts. Ids are what
// enterprise-info --users --verbose lists on a user (direct and team roles,
// commands/enterprise.py). Duplicate names merge conservatively: admin if any
// is admin, enforcing only if every one enforces; the name index is used
// only for a user row without role ids.
func roleIndex(rows []Row) roleSet {
	out := roleSet{byID: map[string]roleInfo{}, byName: map[string]roleInfo{}}
	for _, r := range rows {
		info := roleInfo{admin: truthy(r["admin"])}
		if enf, ok := r["enforcements"].(map[string]any); ok {
			info.enforces = truthy(enf["require_two_factor"])
		}
		if id := str(r, "role_id"); id != "" {
			out.byID[id] = info
		}
		name := str(r, "name")
		if name == "" {
			continue
		}
		if prev, ok := out.byName[name]; ok {
			info.admin = info.admin || prev.admin
			info.enforces = info.enforces && prev.enforces
		}
		out.byName[name] = info
	}
	return out
}

// roleFacts says whether a user's roles make them an admin and enforce 2FA.
// A role listed as {role_id, role_name} is joined by id; a role id the role
// report does not list counts as an admin role that does not enforce 2FA
// (fail closed for both: such a user is reported, never hidden); a role
// listed by name alone is joined by the conservative name index.
func roleFacts(v any, roles roleSet) (admin, enforced bool) {
	list, _ := v.([]any)
	for _, e := range list {
		var info roleInfo
		var ok bool
		switch t := e.(type) {
		case map[string]any:
			id := str(Row(t), "role_id")
			if info, ok = roles.byID[id]; !ok {
				info, ok = roleInfo{admin: true}, true
			}
		case string:
			info, ok = roles.byName[t]
		}
		if ok {
			admin = admin || info.admin
			enforced = enforced || info.enforces
		}
	}
	return admin, enforced
}

func intp(v int) *int { return &v }

func round1(f float64) float64 { return math.Round(f*10) / 10 }

// Event is one change between two runs.
type Event struct {
	Type       string `json:"type"`
	Resource   string `json:"resource"` // company id, or company id/email
	ResourceTy string `json:"resource_type"`
	Before     any    `json:"before"`
	After      any    `json:"after"`
}

// EventTypes are every event 'posture run' can raise.
var EventTypes = []string{
	"posture.twofa_coverage_dropped",
	"posture.security_score_dropped",
	"posture.weak_passwords_increased",
	"posture.reused_passwords_increased",
	"posture.breachwatch_at_risk_increased",
	"posture.inactive_users_increased",
	"posture.seats_over_allocated",
	"user.added",
	"user.removed",
	"user.twofa_disabled",
	"user.locked",
	"user.became_inactive",
	"user.breachwatch_at_risk",
	"company.security_audit_error",
}

func fptr(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

// Diff returns the events between a company's previous good run and this one.
func Diff(company string, prev, cur Metrics, prevUsers, curUsers map[string]UserFacts) []Event {
	var ev []Event
	add := func(t string, before, after any) {
		ev = append(ev, Event{Type: t, Resource: company, ResourceTy: "company", Before: before, After: after})
	}
	if prev.TwoFACoverage != nil && cur.TwoFACoverage != nil && *cur.TwoFACoverage < *prev.TwoFACoverage {
		add("posture.twofa_coverage_dropped", *prev.TwoFACoverage, *cur.TwoFACoverage)
	}
	if prev.AvgSecurityScore != nil && cur.AvgSecurityScore != nil && *cur.AvgSecurityScore < *prev.AvgSecurityScore {
		add("posture.security_score_dropped", fptr(prev.AvgSecurityScore), fptr(cur.AvgSecurityScore))
	}
	if prev.Audited > 0 && cur.Audited > 0 && cur.WeakTotal > prev.WeakTotal {
		add("posture.weak_passwords_increased", prev.WeakTotal, cur.WeakTotal)
	}
	if prev.Audited > 0 && cur.Audited > 0 && cur.ReusedTotal > prev.ReusedTotal {
		add("posture.reused_passwords_increased", prev.ReusedTotal, cur.ReusedTotal)
	}
	if prev.BreachWatch == "active" && cur.BreachWatch == "active" && cur.AtRiskTotal > prev.AtRiskTotal {
		add("posture.breachwatch_at_risk_increased", prev.AtRiskTotal, cur.AtRiskTotal)
	}
	if prev.InactiveDays == cur.InactiveDays && cur.Inactive > prev.Inactive {
		add("posture.inactive_users_increased", prev.Inactive, cur.Inactive)
	}
	if cur.SeatsOver && !prev.SeatsOver {
		add("posture.seats_over_allocated", map[string]any{"allocated": prev.SeatsAllocated, "active": prev.SeatsActive},
			map[string]any{"allocated": cur.SeatsAllocated, "active": cur.SeatsActive})
	}
	emails := make([]string, 0, len(curUsers)+len(prevUsers))
	for e := range curUsers {
		emails = append(emails, e)
	}
	for e := range prevUsers {
		if _, ok := curUsers[e]; !ok {
			emails = append(emails, e)
		}
	}
	sort.Strings(emails)
	for _, e := range emails {
		p, had := prevUsers[e]
		c, has := curUsers[e]
		res := company + "/" + e
		uadd := func(t string, before, after any) {
			ev = append(ev, Event{Type: t, Resource: res, ResourceTy: "user", Before: before, After: after})
		}
		switch {
		case !had && has:
			uadd("user.added", nil, c.Status)
			continue
		case had && !has:
			uadd("user.removed", p.Status, nil)
			continue
		}
		if p.TwoFA && !c.TwoFA && strings.EqualFold(c.Status, "active") {
			uadd("user.twofa_disabled", true, false)
		}
		if !strings.HasPrefix(strings.ToLower(p.Status), "locked") && strings.HasPrefix(strings.ToLower(c.Status), "locked") {
			uadd("user.locked", p.Status, c.Status)
		}
		if prev.InactiveDays == cur.InactiveDays && !p.Inactive && c.Inactive {
			uadd("user.became_inactive", p.LastLogin, c.LastLogin)
		}
		if c.AtRisk != nil && *c.AtRisk > 0 && (p.AtRisk == nil || *p.AtRisk == 0) && prev.BreachWatch == "active" {
			uadd("user.breachwatch_at_risk", 0, *c.AtRisk)
		}
	}
	return ev
}

// EventID is the stable id of an event raised by run runID.
func EventID(runID int64, e Event) string {
	return fmt.Sprintf("keeper:%s:%s:run%d", e.Type, e.Resource, runID)
}
