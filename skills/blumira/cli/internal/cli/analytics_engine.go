// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-built analytics engine shared by the novel cross-account / over-time
// commands (triage, drift, velocity, sla, coverage, exposure, audit, recurring).
//
// Design: pure computation functions take plain Go slices so they are unit
// testable offline (no DB, no network); the command RunE bodies load records
// from the local SQLite store and call into these. The over-time commands read
// from a finding_history snapshot table this file owns, captured per sync.

package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"blumira-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

// jsonMarshalRows marshals a typed row slice to JSON, normalizing a nil slice
// to an empty array so consumers never see `null`.
func jsonMarshalRows(rows any) (json.RawMessage, error) {
	b, err := json.Marshal(rows)
	if err != nil {
		return nil, err
	}
	if string(b) == "null" {
		return json.RawMessage("[]"), nil
	}
	return b, nil
}

// ---- normalized records ---------------------------------------------------

// findingRec is the normalized shape of a Blumira findings_data_record used by
// every analytics command. Fields not present in a given payload stay zero.
type findingRec struct {
	ID          string   `json:"finding_id"`
	ShortID     string   `json:"short_id,omitempty"`
	Name        string   `json:"name"`
	Account     string   `json:"account,omitempty"`      // org_id
	AccountName string   `json:"account_name,omitempty"` // org_name
	Status      int      `json:"status"`
	StatusName  string   `json:"status_name,omitempty"`
	Priority    int      `json:"priority"`
	Category    int      `json:"category,omitempty"`
	Type        int      `json:"type,omitempty"`
	Resolution  int      `json:"resolution,omitempty"`
	Created     string   `json:"created,omitempty"`
	Modified    string   `json:"modified,omitempty"`
	Owners      []string `json:"owners,omitempty"`
}

// resolvedStatusCode is the Blumira finding status code for "Resolved".
const resolvedStatusCode = 40

func (f findingRec) IsResolved() bool {
	return f.Status == resolvedStatusCode || strings.EqualFold(strings.TrimSpace(f.StatusName), "Resolved")
}

func (f findingRec) IsOpen() bool { return !f.IsResolved() }

func (f findingRec) CreatedTime() (time.Time, bool) { return parseBlumiraTime(f.Created) }

// deviceRec is the normalized shape of an agents_devices_data_record.
type deviceRec struct {
	DeviceID string `json:"device_id"`
	Hostname string `json:"hostname"`
	Account  string `json:"account,omitempty"` // org_id
	IsDC     bool   `json:"is_domain_controller"`
	Isolated bool   `json:"is_isolated,omitempty"`
	Sleeping bool   `json:"is_sleeping,omitempty"`
	Excluded bool   `json:"is_excluded,omitempty"`
	Alive    string `json:"alive,omitempty"` // last seen
	Platform string `json:"plat,omitempty"`
}

func (d deviceRec) AliveTime() (time.Time, bool) { return parseBlumiraTime(d.Alive) }

// ruleRec is the normalized shape of a detection_rule_record.
type ruleRec struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Account     string `json:"account,omitempty"` // org_id (absent on basis rules)
	Status      int    `json:"status"`            // 0=not_deployed,1=enabled,5=disabled
	StatusLabel string `json:"status_label,omitempty"`
	Priority    int    `json:"priority,omitempty"`
}

// ruleStatusEnabled is the detection_rule_record status code for "enabled".
const ruleStatusEnabled = 1

func (r ruleRec) Enabled() bool {
	return r.Status == ruleStatusEnabled || strings.EqualFold(strings.TrimSpace(r.StatusLabel), "enabled")
}

// ---- field / time parsing -------------------------------------------------

func parseBlumiraTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.999999",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05.999999",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

func mapString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			switch t := v.(type) {
			case string:
				if t != "" {
					return t
				}
			case fmt.Stringer:
				return t.String()
			default:
				return fmt.Sprintf("%v", v)
			}
		}
	}
	return ""
}

func mapInt(m map[string]any, keys ...string) int {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			switch t := v.(type) {
			case json.Number:
				if n, err := t.Int64(); err == nil {
					return int(n)
				}
				if fv, err := t.Float64(); err == nil {
					return int(fv)
				}
			case float64:
				return int(t)
			case int:
				return t
			case int64:
				return int(t)
			case string:
				if n, err := strconv.Atoi(strings.TrimSpace(t)); err == nil {
					return n
				}
				if fv, err := strconv.ParseFloat(strings.TrimSpace(t), 64); err == nil {
					return int(fv)
				}
			}
		}
	}
	return 0
}

func mapBool(m map[string]any, keys ...string) bool {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			switch t := v.(type) {
			case bool:
				return t
			case string:
				b, _ := strconv.ParseBool(strings.TrimSpace(t))
				return b
			}
		}
	}
	return false
}

// ---- store loaders --------------------------------------------------------

const analyticsListLimit = 100000

// findingResourceTypes are the store resource_types that hold findings_data_record.
var findingResourceTypes = []string{"msp-accounts-findings", "org-findings"}

func decodeFinding(m map[string]any) findingRec {
	return findingRec{
		ID:          mapString(m, "finding_id", "id"),
		ShortID:     mapString(m, "short_id"),
		Name:        mapString(m, "name"),
		Account:     mapString(m, "org_id", "account_id"),
		AccountName: mapString(m, "org_name"),
		Status:      mapInt(m, "status"),
		StatusName:  mapString(m, "status_name"),
		Priority:    mapInt(m, "priority"),
		Category:    mapInt(m, "category"),
		Type:        mapInt(m, "type"),
		Resolution:  mapInt(m, "resolution"),
		Created:     mapString(m, "created"),
		Modified:    mapString(m, "modified"),
		Owners:      decodeOwners(m),
	}
}

// loadFindings reads every finding from the local store, de-duplicated by
// finding_id (the cross-account MSP feed and the direct-org feed overlap).
func loadFindings(s *store.Store) ([]findingRec, error) {
	seen := map[string]bool{}
	var out []findingRec
	for _, rt := range findingResourceTypes {
		rows, err := s.List(rt, analyticsListLimit)
		if err != nil {
			return nil, err
		}
		for _, raw := range rows {
			m, err := store.DecodeJSONObject(raw)
			if err != nil {
				continue
			}
			f := decodeFinding(m)
			if f.ID == "" || seen[f.ID] {
				continue
			}
			seen[f.ID] = true
			out = append(out, f)
		}
	}
	return out, nil
}

func loadDevices(s *store.Store) ([]deviceRec, error) {
	rows, err := s.List("org-agents-devices", analyticsListLimit)
	if err != nil {
		return nil, err
	}
	var out []deviceRec
	for _, raw := range rows {
		m, err := store.DecodeJSONObject(raw)
		if err != nil {
			continue
		}
		out = append(out, deviceRec{
			DeviceID: mapString(m, "device_id", "id"),
			Hostname: mapString(m, "hostname"),
			Account:  mapString(m, "org_id"),
			IsDC:     mapBool(m, "is_domain_controller"),
			Isolated: mapBool(m, "is_isolated"),
			Sleeping: mapBool(m, "is_sleeping"),
			Excluded: mapBool(m, "is_excluded"),
			Alive:    mapString(m, "alive"),
			Platform: mapString(m, "plat"),
		})
	}
	return out, nil
}

func loadRules(s *store.Store, resourceType string) ([]ruleRec, error) {
	rows, err := s.List(resourceType, analyticsListLimit)
	if err != nil {
		return nil, err
	}
	var out []ruleRec
	for _, raw := range rows {
		m, err := store.DecodeJSONObject(raw)
		if err != nil {
			continue
		}
		out = append(out, ruleRec{
			ID:          mapString(m, "id"),
			Name:        mapString(m, "name"),
			Account:     mapString(m, "org_id"),
			Status:      mapInt(m, "status"),
			StatusLabel: mapString(m, "status_label"),
			Priority:    mapInt(m, "priority"),
		})
	}
	return out, nil
}

// ---- snapshot history (over-time axis) ------------------------------------

const findingHistoryDDL = `CREATE TABLE IF NOT EXISTS finding_history (
	synced_at   TEXT NOT NULL,
	finding_id  TEXT NOT NULL,
	account     TEXT,
	name        TEXT,
	status      INTEGER,
	status_name TEXT,
	priority    INTEGER,
	created     TEXT,
	modified    TEXT,
	captured_at TEXT,
	PRIMARY KEY (synced_at, finding_id)
)`

func ensureFindingHistory(db *sql.DB) error {
	_, err := db.Exec(findingHistoryDDL)
	return err
}

// historySnapshot is one captured batch of findings keyed by the sync that
// produced it.
type historySnapshot struct {
	SyncedAt time.Time
	Key      string
	Findings []findingRec
}

// captureFindingHistory records the current findings as a snapshot batch keyed
// by the store's last sync time for findings. Idempotent: re-running between
// syncs (same key) inserts nothing new thanks to the (synced_at, finding_id)
// primary key. Returns the batch key used.
func captureFindingHistory(db *sql.DB, s *store.Store, findings []findingRec, now time.Time) (string, error) {
	if err := ensureFindingHistory(db); err != nil {
		return "", err
	}
	key := ""
	for _, rt := range findingResourceTypes {
		if ls := s.GetLastSyncedAt(rt); ls != "" {
			key = ls
			break
		}
	}
	if key == "" {
		// No sync state recorded; fall back to a coarse minute bucket so
		// repeated runs in the same minute don't double-count.
		key = now.UTC().Format("2006-01-02T15:04")
	}
	if len(findings) == 0 {
		return key, nil
	}
	tx, err := db.Begin()
	if err != nil {
		return key, err
	}
	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO finding_history
		(synced_at, finding_id, account, name, status, status_name, priority, created, modified, captured_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		_ = tx.Rollback()
		return key, err
	}
	defer stmt.Close()
	capturedAt := now.UTC().Format(time.RFC3339)
	for _, f := range findings {
		if _, err := stmt.Exec(key, f.ID, f.Account, f.Name, f.Status, f.StatusName, f.Priority, f.Created, f.Modified, capturedAt); err != nil {
			_ = tx.Rollback()
			return key, err
		}
	}
	if err := tx.Commit(); err != nil {
		return key, err
	}
	return key, nil
}

// loadHistorySnapshots reads all snapshot batches, ordered oldest-first.
func loadHistorySnapshots(db *sql.DB) ([]historySnapshot, error) {
	if err := ensureFindingHistory(db); err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT synced_at, finding_id, account, name, status, status_name, priority, created, modified
		FROM finding_history ORDER BY synced_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byKey := map[string]*historySnapshot{}
	var order []string
	for rows.Next() {
		var key string
		var f findingRec
		var account, name, statusName, created, modified sql.NullString
		var status, priority sql.NullInt64
		if err := rows.Scan(&key, &f.ID, &account, &name, &status, &statusName, &priority, &created, &modified); err != nil {
			return nil, err
		}
		f.Account = account.String
		f.Name = name.String
		f.StatusName = statusName.String
		f.Created = created.String
		f.Modified = modified.String
		f.Status = int(status.Int64)
		f.Priority = int(priority.Int64)
		snap, ok := byKey[key]
		if !ok {
			t, _ := parseBlumiraTime(key)
			snap = &historySnapshot{Key: key, SyncedAt: t}
			byKey[key] = snap
			order = append(order, key)
		}
		snap.Findings = append(snap.Findings, f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]historySnapshot, 0, len(order))
	for _, k := range order {
		out = append(out, *byKey[k])
	}
	return out, nil
}

// ---- pure compute: triage -------------------------------------------------

type triageRow struct {
	Account     string `json:"account"`
	AccountName string `json:"account_name,omitempty"`
	FindingID   string `json:"finding_id"`
	ShortID     string `json:"short_id,omitempty"`
	Name        string `json:"name"`
	Priority    int    `json:"priority"`
	Status      string `json:"status"`
	Created     string `json:"created,omitempty"`
	AgeHours    int    `json:"age_hours"`
}

type triageOpts struct {
	priorityMatch func(int) bool
	statusMatch   func(findingRec) bool
	limit         int
}

// computeTriage ranks findings into one cross-account queue: open (by default)
// findings sorted by priority (1=highest first) then age (oldest first).
func computeTriage(findings []findingRec, now time.Time, opts triageOpts) []triageRow {
	var rows []triageRow
	for _, f := range findings {
		if opts.statusMatch != nil && !opts.statusMatch(f) {
			continue
		}
		if opts.priorityMatch != nil && !opts.priorityMatch(f.Priority) {
			continue
		}
		age := 0
		if t, ok := f.CreatedTime(); ok {
			age = int(now.Sub(t).Hours())
			if age < 0 {
				age = 0
			}
		}
		status := f.StatusName
		if status == "" {
			status = fmt.Sprintf("status_%d", f.Status)
		}
		rows = append(rows, triageRow{
			Account: f.Account, AccountName: f.AccountName,
			FindingID: f.ID, ShortID: f.ShortID, Name: f.Name,
			Priority: f.Priority, Status: status, Created: f.Created, AgeHours: age,
		})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		pi, pj := normPriority(rows[i].Priority), normPriority(rows[j].Priority)
		if pi != pj {
			return pi < pj
		}
		return rows[i].AgeHours > rows[j].AgeHours
	})
	if opts.limit > 0 && len(rows) > opts.limit {
		rows = rows[:opts.limit]
	}
	return rows
}

// normPriority maps the 1-5 scale so that unknown/zero priority sorts last.
func normPriority(p int) int {
	if p <= 0 {
		return 999
	}
	return p
}

// parsePriorityFilter turns "high"/"critical"/"low"/"p1"/"3" into a predicate.
// Empty string matches everything.
func parsePriorityFilter(s string) (func(int) bool, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "":
		return func(int) bool { return true }, nil
	case "critical", "p1":
		return func(p int) bool { return p == 1 }, nil
	case "high":
		return func(p int) bool { return p >= 1 && p <= 2 }, nil
	case "medium", "med", "p3":
		return func(p int) bool { return p == 3 }, nil
	case "low":
		return func(p int) bool { return p >= 4 }, nil
	}
	if strings.HasPrefix(s, "p") {
		if n, err := strconv.Atoi(s[1:]); err == nil {
			return func(p int) bool { return p == n }, nil
		}
	}
	if n, err := strconv.Atoi(s); err == nil {
		return func(p int) bool { return p == n }, nil
	}
	return nil, fmt.Errorf("invalid --priority %q (use high, critical, medium, low, p1..p5, or a number 1-5)", s)
}

// parseStatusFilter turns "open"/"resolved"/"all" into a predicate.
func parseStatusFilter(s string) (func(findingRec) bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "open":
		return func(f findingRec) bool { return f.IsOpen() }, nil
	case "resolved", "closed":
		return func(f findingRec) bool { return f.IsResolved() }, nil
	case "all", "any":
		return func(findingRec) bool { return true }, nil
	}
	return nil, fmt.Errorf("invalid --status %q (use open, resolved, or all)", s)
}

// ---- pure compute: sla ----------------------------------------------------

// defaultSLA maps finding priority (1-5) to an age threshold. Sensible MSP
// defaults; overridable per command via --sla.
var defaultSLA = map[int]time.Duration{
	1: 4 * time.Hour,
	2: 8 * time.Hour,
	3: 24 * time.Hour,
	4: 72 * time.Hour,
	5: 168 * time.Hour,
}

const fallbackSLA = 24 * time.Hour

type slaRow struct {
	Account     string `json:"account"`
	FindingID   string `json:"finding_id"`
	Name        string `json:"name"`
	Priority    int    `json:"priority"`
	AgeHours    int    `json:"age_hours"`
	SLAHours    int    `json:"sla_hours"`
	BreachInHrs int    `json:"breach_in_hours"` // negative => already breached
	Breached    bool   `json:"breached"`
}

type slaOpts struct {
	override time.Duration // 0 => per-priority defaultSLA
	breachIn time.Duration // only surface items breaching within this window (0 => all open)
	priority func(int) bool
}

// computeSLA returns open findings ranked by time-to-breach (most urgent
// first). Breached findings sort first with a negative breach_in.
func computeSLA(findings []findingRec, now time.Time, opts slaOpts) []slaRow {
	var rows []slaRow
	for _, f := range findings {
		if !f.IsOpen() {
			continue
		}
		if opts.priority != nil && !opts.priority(f.Priority) {
			continue
		}
		created, ok := f.CreatedTime()
		if !ok {
			continue
		}
		sla := opts.override
		if sla <= 0 {
			var has bool
			sla, has = defaultSLA[f.Priority]
			if !has {
				sla = fallbackSLA
			}
		}
		age := now.Sub(created)
		breachIn := sla - age // time remaining until breach
		breached := breachIn <= 0
		if opts.breachIn > 0 && !breached && breachIn > opts.breachIn {
			continue // not imminent enough
		}
		rows = append(rows, slaRow{
			Account: f.Account, FindingID: f.ID, Name: f.Name, Priority: f.Priority,
			AgeHours: int(age.Hours()), SLAHours: int(sla.Hours()),
			BreachInHrs: int(breachIn.Hours()), Breached: breached,
		})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].BreachInHrs < rows[j].BreachInHrs
	})
	return rows
}

// ---- pure compute: coverage ----------------------------------------------

type coverageRow struct {
	RuleID   string `json:"rule_id,omitempty"`
	RuleName string `json:"rule_name"`
	Gap      string `json:"gap"` // "missing" | "disabled"
	Priority int    `json:"priority,omitempty"`
}

// computeCoverage diffs the MSP basis ruleset against the org's deployed rules,
// matched by rule name. A basis rule with no org match is "missing"; one whose
// org match is not enabled is "disabled".
func computeCoverage(basis, orgRules []ruleRec) []coverageRow {
	byName := map[string]ruleRec{}
	for _, r := range orgRules {
		byName[strings.ToLower(strings.TrimSpace(r.Name))] = r
	}
	var rows []coverageRow
	for _, b := range basis {
		key := strings.ToLower(strings.TrimSpace(b.Name))
		if key == "" {
			continue
		}
		org, ok := byName[key]
		switch {
		case !ok:
			rows = append(rows, coverageRow{RuleID: b.ID, RuleName: b.Name, Gap: "missing", Priority: b.Priority})
		case !org.Enabled():
			rows = append(rows, coverageRow{RuleID: org.ID, RuleName: b.Name, Gap: "disabled", Priority: b.Priority})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Gap != rows[j].Gap {
			return rows[i].Gap < rows[j].Gap // "disabled" < "missing"
		}
		return rows[i].RuleName < rows[j].RuleName
	})
	return rows
}

// ---- pure compute: exposure ----------------------------------------------

type exposureRow struct {
	Account     string `json:"account,omitempty"`
	DeviceID    string `json:"device_id"`
	Hostname    string `json:"hostname"`
	IsDC        bool   `json:"is_domain_controller"`
	Reason      string `json:"reason"`
	LastSeenHrs int    `json:"last_seen_hours"` // -1 when unknown
}

type exposureOpts struct {
	staleAfter time.Duration
	onlyDCs    bool // --flag-dc-stale: restrict to domain controllers
}

// computeExposure flags concerning agent devices: domain controllers (and, when
// not onlyDCs, any device) that are stale, isolated, sleeping, or excluded.
func computeExposure(devices []deviceRec, now time.Time, opts exposureOpts) []exposureRow {
	stale := opts.staleAfter
	if stale <= 0 {
		stale = 24 * time.Hour
	}
	var rows []exposureRow
	for _, d := range devices {
		if opts.onlyDCs && !d.IsDC {
			continue
		}
		var reasons []string
		lastSeenHrs := -1
		if t, ok := d.AliveTime(); ok {
			gap := now.Sub(t)
			lastSeenHrs = int(gap.Hours())
			if gap > stale {
				reasons = append(reasons, "stale")
			}
		} else {
			reasons = append(reasons, "never-checked-in")
		}
		if d.Isolated {
			reasons = append(reasons, "isolated")
		}
		if d.Excluded {
			reasons = append(reasons, "excluded")
		}
		if d.Sleeping {
			reasons = append(reasons, "sleeping")
		}
		if len(reasons) == 0 {
			continue
		}
		rows = append(rows, exposureRow{
			Account: d.Account, DeviceID: d.DeviceID, Hostname: d.Hostname,
			IsDC: d.IsDC, Reason: strings.Join(reasons, ","), LastSeenHrs: lastSeenHrs,
		})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].IsDC != rows[j].IsDC {
			return rows[i].IsDC // DCs first
		}
		return rows[i].LastSeenHrs > rows[j].LastSeenHrs
	})
	return rows
}

// ---- pure compute: drift --------------------------------------------------

type driftRow struct {
	Account   string `json:"account,omitempty"`
	FindingID string `json:"finding_id"`
	Name      string `json:"name"`
	Change    string `json:"change"` // "new" | "status-change" | "resolved"
	From      string `json:"from,omitempty"`
	To        string `json:"to,omitempty"`
}

// computeDrift diffs a previous snapshot against the current one: findings that
// are new, changed status, or were newly resolved.
func computeDrift(prev, cur []findingRec) []driftRow {
	prevByID := map[string]findingRec{}
	for _, f := range prev {
		prevByID[f.ID] = f
	}
	var rows []driftRow
	for _, c := range cur {
		p, existed := prevByID[c.ID]
		switch {
		case !existed:
			rows = append(rows, driftRow{Account: c.Account, FindingID: c.ID, Name: c.Name, Change: "new", To: statusLabel(c)})
		case !p.IsResolved() && c.IsResolved():
			rows = append(rows, driftRow{Account: c.Account, FindingID: c.ID, Name: c.Name, Change: "resolved", From: statusLabel(p), To: statusLabel(c)})
		case p.Status != c.Status || !strings.EqualFold(p.StatusName, c.StatusName):
			rows = append(rows, driftRow{Account: c.Account, FindingID: c.ID, Name: c.Name, Change: "status-change", From: statusLabel(p), To: statusLabel(c)})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Change != rows[j].Change {
			return rows[i].Change < rows[j].Change
		}
		return rows[i].Name < rows[j].Name
	})
	return rows
}

func statusLabel(f findingRec) string {
	if f.StatusName != "" {
		return f.StatusName
	}
	return fmt.Sprintf("status_%d", f.Status)
}

// ---- pure compute: velocity ----------------------------------------------

type velocityRow struct {
	Key         string  `json:"key"` // account id (or "all")
	Resolved    int     `json:"resolved"`
	Open        int     `json:"open"`
	MTTRHours   float64 `json:"mttr_hours"`
	OpenRatePct float64 `json:"open_rate_pct"`
}

// computeVelocity derives MTTR and open-rate per group (account) from the
// snapshot history within [since, now]. MTTR for a finding = time between its
// first appearance and the first snapshot in which it is resolved.
func computeVelocity(snaps []historySnapshot, since time.Time, groupByAccount bool) []velocityRow {
	type track struct {
		first    time.Time
		resolved time.Time
		isRes    bool
		account  string
		open     bool
	}
	tracks := map[string]*track{}
	for _, snap := range snaps {
		if !snap.SyncedAt.IsZero() && snap.SyncedAt.Before(since) {
			continue
		}
		for _, f := range snap.Findings {
			t := tracks[f.ID]
			if t == nil {
				t = &track{first: snap.SyncedAt, account: f.Account, open: true}
				tracks[f.ID] = t
			}
			if f.IsResolved() && !t.isRes {
				t.isRes = true
				t.resolved = snap.SyncedAt
				t.open = false
			}
			if !f.IsResolved() {
				t.open = true
			}
		}
	}
	agg := map[string]*velocityRow{}
	durs := map[string][]float64{}
	keyOf := func(acct string) string {
		if groupByAccount {
			if acct == "" {
				return "(unknown)"
			}
			return acct
		}
		return "all"
	}
	for _, t := range tracks {
		k := keyOf(t.account)
		row := agg[k]
		if row == nil {
			row = &velocityRow{Key: k}
			agg[k] = row
		}
		if t.isRes {
			row.Resolved++
			if !t.first.IsZero() && !t.resolved.IsZero() && !t.resolved.Before(t.first) {
				durs[k] = append(durs[k], t.resolved.Sub(t.first).Hours())
			}
		} else {
			row.Open++
		}
	}
	var out []velocityRow
	for k, row := range agg {
		total := row.Resolved + row.Open
		if total > 0 {
			row.OpenRatePct = round1(float64(row.Open) / float64(total) * 100)
		}
		if ds := durs[k]; len(ds) > 0 {
			var sum float64
			for _, d := range ds {
				sum += d
			}
			row.MTTRHours = round1(sum / float64(len(ds)))
		}
		out = append(out, *row)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func round1(f float64) float64 { return float64(int(f*10+0.5)) / 10 }

// ---- pure compute: audit (re-fire) ---------------------------------------

type auditRow struct {
	FindingID string `json:"finding_id"`
	Account   string `json:"account,omitempty"`
	Name      string `json:"name"`
	Reopens   int    `json:"reopens"`
}

// computeAudit finds findings that were resolved in one snapshot and then
// non-resolved (re-fired) in a later snapshot, counting how many times.
func computeAudit(snaps []historySnapshot) []auditRow {
	type state struct {
		account     string
		name        string
		wasResolved bool
		reopens     int
	}
	st := map[string]*state{}
	for _, snap := range snaps {
		for _, f := range snap.Findings {
			s := st[f.ID]
			if s == nil {
				s = &state{account: f.Account, name: f.Name}
				st[f.ID] = s
			}
			if f.Name != "" {
				s.name = f.Name
			}
			if f.IsResolved() {
				s.wasResolved = true
			} else if s.wasResolved {
				s.reopens++
				s.wasResolved = false
			}
		}
	}
	var out []auditRow
	for id, s := range st {
		if s.reopens > 0 {
			out = append(out, auditRow{FindingID: id, Account: s.account, Name: s.name, Reopens: s.reopens})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Reopens != out[j].Reopens {
			return out[i].Reopens > out[j].Reopens
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// ---- pure compute: recurring ----------------------------------------------

type recurringRow struct {
	Name     string `json:"name"`
	Count    int    `json:"count"`
	Accounts int    `json:"accounts"`
}

// computeRecurring counts distinct findings (by name) seen across snapshots in
// the window, returning names that recur at least minCount times.
func computeRecurring(snaps []historySnapshot, since time.Time, minCount int) []recurringRow {
	type agg struct {
		ids      map[string]bool
		accounts map[string]bool
	}
	byName := map[string]*agg{}
	for _, snap := range snaps {
		if !snap.SyncedAt.IsZero() && snap.SyncedAt.Before(since) {
			continue
		}
		for _, f := range snap.Findings {
			name := strings.TrimSpace(f.Name)
			if name == "" {
				continue
			}
			a := byName[name]
			if a == nil {
				a = &agg{ids: map[string]bool{}, accounts: map[string]bool{}}
				byName[name] = a
			}
			a.ids[f.ID] = true
			if f.Account != "" {
				a.accounts[f.Account] = true
			}
		}
	}
	var out []recurringRow
	for name, a := range byName {
		if len(a.ids) >= minCount {
			out = append(out, recurringRow{Name: name, Count: len(a.ids), Accounts: len(a.accounts)})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// ---- shared command helpers ----------------------------------------------

// openAnalyticsStore opens the local SQLite store at the default path.
func openAnalyticsStore(ctx context.Context) (*store.Store, error) {
	return store.OpenWithContext(ctx, defaultDBPath("blumira-cli"))
}

// emptyStoreHint is the honest message emitted when no findings are synced yet.
const emptyStoreHint = "no findings in the local store yet — run 'blumira-cli auth login' then 'blumira-cli sync --full' first"

// parseWindow parses a duration that may use day (Nd) or week (Nw) suffixes in
// addition to Go's standard units (h, m, s). Empty string returns 0, nil.
func parseWindow(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	if strings.HasSuffix(s, "d") {
		if n, err := strconv.Atoi(strings.TrimSuffix(s, "d")); err == nil {
			return time.Duration(n) * 24 * time.Hour, nil
		}
	}
	if strings.HasSuffix(s, "w") {
		if n, err := strconv.Atoi(strings.TrimSuffix(s, "w")); err == nil {
			return time.Duration(n) * 7 * 24 * time.Hour, nil
		}
	}
	return time.ParseDuration(s)
}

// emitAnalyticsRows renders a slice of typed rows through the standard output
// pipeline (JSON, CSV, table, --select, --compact). A nil/empty slice renders
// as an empty JSON array plus a stderr hint when hintOnEmpty is set.
func emitAnalyticsRows(cmd *cobra.Command, flags *rootFlags, rows any, count int, hintOnEmpty string) error {
	data, err := jsonMarshalRows(rows)
	if err != nil {
		return err
	}
	if count == 0 {
		if hintOnEmpty != "" && !flags.quiet {
			fmt.Fprintln(cmd.ErrOrStderr(), hintOnEmpty)
		}
		return printOutputWithFlags(cmd.OutOrStdout(), []byte("[]"), flags)
	}
	if flags.selectFields != "" {
		data = filterFields(data, flags.selectFields)
	} else if flags.compact {
		data = compactFields(data)
	}
	return printOutputWithFlags(cmd.OutOrStdout(), data, flags)
}
