// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-built analytics engine extension for the 2026-06 reprint's new novel
// commands (overview, reconcile, dc-roster, workload, evidence-search), plus
// shared record decoders (decodeOwners) used by decodeFinding in
// analytics_engine.go. Pure computation functions take plain Go slices so they
// unit-test offline; the command RunE bodies load records from the local
// SQLite store and call in.

package cli

import (
	"database/sql"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"blumira-pp-cli/internal/store"
)

// decodeOwners extracts a finding's owner list tolerantly: Blumira payloads may
// carry owners as an array of strings, an array of objects (email/name/id), or
// a single string. Missing/unknown shapes return nil.
func decodeOwners(m map[string]any) []string {
	v, ok := m["owners"]
	if !ok || v == nil {
		v, ok = m["owner"]
		if !ok || v == nil {
			return nil
		}
	}
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	switch t := v.(type) {
	case string:
		add(t)
	case []any:
		for _, item := range t {
			switch e := item.(type) {
			case string:
				add(e)
			case map[string]any:
				add(mapString(e, "email", "name", "user", "id"))
			}
		}
	case map[string]any:
		add(mapString(t, "email", "name", "user", "id"))
	}
	return out
}

// ---- pure compute: overview ------------------------------------------------

type overviewRow struct {
	Account       string `json:"account"`
	AccountName   string `json:"account_name,omitempty"`
	OpenFindings  int    `json:"open_findings"`
	P1Open        int    `json:"p1_open"`
	P2Open        int    `json:"p2_open"`
	OldestOpenHrs int    `json:"oldest_open_hours"`
	Resolved      int    `json:"resolved"`
	Devices       int    `json:"devices"`
	DomainCtrls   int    `json:"domain_controllers"`
	StaleDevices  int    `json:"stale_devices"`
}

// computeOverview rolls findings (and any synced agent devices) up per account:
// open counts by priority bucket, oldest open age, and device health. Accounts
// sort by P1 count, then open count, so the hottest client leads.
func computeOverview(findings []findingRec, devices []deviceRec, now time.Time) []overviewRow {
	byAcct := map[string]*overviewRow{}
	order := func(acct, name string) *overviewRow {
		key := acct
		if key == "" {
			key = "(unknown)"
		}
		row := byAcct[key]
		if row == nil {
			row = &overviewRow{Account: key}
			byAcct[key] = row
		}
		if row.AccountName == "" && name != "" {
			row.AccountName = name
		}
		return row
	}
	for _, f := range findings {
		row := order(f.Account, f.AccountName)
		if f.IsResolved() {
			row.Resolved++
			continue
		}
		row.OpenFindings++
		switch f.Priority {
		case 1:
			row.P1Open++
		case 2:
			row.P2Open++
		}
		if t, ok := f.CreatedTime(); ok {
			age := int(now.Sub(t).Hours())
			if age > row.OldestOpenHrs {
				row.OldestOpenHrs = age
			}
		}
	}
	const staleAfter = 24 * time.Hour
	for _, d := range devices {
		row := order(d.Account, "")
		row.Devices++
		if d.IsDC {
			row.DomainCtrls++
		}
		if t, ok := d.AliveTime(); ok {
			if now.Sub(t) > staleAfter {
				row.StaleDevices++
			}
		} else {
			row.StaleDevices++
		}
	}
	out := make([]overviewRow, 0, len(byAcct))
	for _, row := range byAcct {
		out = append(out, *row)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].P1Open != out[j].P1Open {
			return out[i].P1Open > out[j].P1Open
		}
		if out[i].OpenFindings != out[j].OpenFindings {
			return out[i].OpenFindings > out[j].OpenFindings
		}
		return out[i].Account < out[j].Account
	})
	return out
}

// ---- pure compute: reconcile -----------------------------------------------

type reconcileRow struct {
	Account     string `json:"account"`
	AccountName string `json:"account_name,omitempty"`
	FindingID   string `json:"finding_id"`
	ShortID     string `json:"short_id,omitempty"`
	Name        string `json:"name"`
	Priority    int    `json:"priority"`
	Status      string `json:"status"`
	Assignees   string `json:"assignees,omitempty"` // comma-joined for flat CSV diffing
	Created     string `json:"created,omitempty"`
	Modified    string `json:"modified,omitempty"`
}

// computeReconcile projects findings into the flat, ticket-shaped row set used
// to diff against ConnectWise/Jira/Zendesk. Stable order: account, then
// created, then finding id, so repeated exports diff cleanly.
func computeReconcile(findings []findingRec, statusMatch func(findingRec) bool) []reconcileRow {
	var rows []reconcileRow
	for _, f := range findings {
		if statusMatch != nil && !statusMatch(f) {
			continue
		}
		rows = append(rows, reconcileRow{
			Account: f.Account, AccountName: f.AccountName,
			FindingID: f.ID, ShortID: f.ShortID, Name: f.Name,
			Priority: f.Priority, Status: statusLabel(f),
			Assignees: strings.Join(f.Owners, ","),
			Created:   f.Created, Modified: f.Modified,
		})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Account != rows[j].Account {
			return rows[i].Account < rows[j].Account
		}
		if rows[i].Created != rows[j].Created {
			return rows[i].Created < rows[j].Created
		}
		return rows[i].FindingID < rows[j].FindingID
	})
	return rows
}

// ---- pure compute: dc-roster -----------------------------------------------

type dcRosterRow struct {
	Account     string `json:"account,omitempty"`
	DeviceID    string `json:"device_id"`
	Hostname    string `json:"hostname"`
	Platform    string `json:"platform,omitempty"`
	LastSeenHrs int    `json:"last_seen_hours"` // -1 when never checked in
	State       string `json:"state"`           // "protected" | "stale" | "never-checked-in"
}

// computeDCRoster lists every domain controller across the synced fleet with
// its check-in state. Unlike `exposure` (the prioritized action list), this is
// the full inventory: healthy DCs are included.
func computeDCRoster(devices []deviceRec, now time.Time, staleAfter time.Duration) []dcRosterRow {
	if staleAfter <= 0 {
		staleAfter = 24 * time.Hour
	}
	var rows []dcRosterRow
	for _, d := range devices {
		if !d.IsDC {
			continue
		}
		row := dcRosterRow{
			Account: d.Account, DeviceID: d.DeviceID,
			Hostname: d.Hostname, Platform: d.Platform,
			LastSeenHrs: -1, State: "never-checked-in",
		}
		if t, ok := d.AliveTime(); ok {
			gap := now.Sub(t)
			row.LastSeenHrs = int(gap.Hours())
			if gap > staleAfter {
				row.State = "stale"
			} else {
				row.State = "protected"
			}
		}
		rows = append(rows, row)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Account != rows[j].Account {
			return rows[i].Account < rows[j].Account
		}
		return rows[i].Hostname < rows[j].Hostname
	})
	return rows
}

// ---- pure compute: workload ------------------------------------------------

type workloadRow struct {
	Assignee   string `json:"assignee"`
	OpenTotal  int    `json:"open_total"`
	Under24h   int    `json:"under_24h"`
	Days1to3   int    `json:"days_1_to_3"`
	Over3Days  int    `json:"over_3_days"`
	P1Open     int    `json:"p1_open"`
	Accounts   int    `json:"accounts"`
	Unassigned bool   `json:"unassigned,omitempty"`
}

// computeWorkload groups open findings by assignee across every account with
// age buckets. Findings with no owner aggregate under "(unassigned)" so triage
// gaps are visible, not hidden.
func computeWorkload(findings []findingRec, now time.Time) []workloadRow {
	type agg struct {
		row      workloadRow
		accounts map[string]bool
	}
	byOwner := map[string]*agg{}
	bucket := func(owner string) *agg {
		a := byOwner[owner]
		if a == nil {
			a = &agg{row: workloadRow{Assignee: owner, Unassigned: owner == "(unassigned)"}, accounts: map[string]bool{}}
			byOwner[owner] = a
		}
		return a
	}
	for _, f := range findings {
		if f.IsResolved() {
			continue
		}
		owners := f.Owners
		if len(owners) == 0 {
			owners = []string{"(unassigned)"}
		}
		ageHrs := -1
		if t, ok := f.CreatedTime(); ok {
			ageHrs = int(now.Sub(t).Hours())
		}
		for _, o := range owners {
			a := bucket(o)
			a.row.OpenTotal++
			if f.Priority == 1 {
				a.row.P1Open++
			}
			switch {
			case ageHrs < 0:
				// unknown age: counted in total only
			case ageHrs < 24:
				a.row.Under24h++
			case ageHrs <= 72:
				a.row.Days1to3++
			default:
				a.row.Over3Days++
			}
			if f.Account != "" {
				a.accounts[f.Account] = true
			}
		}
	}
	out := make([]workloadRow, 0, len(byOwner))
	for _, a := range byOwner {
		a.row.Accounts = len(a.accounts)
		out = append(out, a.row)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].OpenTotal != out[j].OpenTotal {
			return out[i].OpenTotal > out[j].OpenTotal
		}
		return out[i].Assignee < out[j].Assignee
	})
	return out
}

// ---- evidence cache (lazy table, owned by evidence-search) ------------------

const evidenceCacheDDL = `CREATE TABLE IF NOT EXISTS finding_evidence_cache (
	finding_id TEXT NOT NULL,
	account    TEXT,
	item_no    INTEGER NOT NULL,
	content    TEXT NOT NULL,
	fetched_at TEXT,
	PRIMARY KEY (finding_id, item_no)
)`

func ensureEvidenceCache(db *sql.DB) error {
	_, err := db.Exec(evidenceCacheDDL)
	return err
}

// storeEvidenceItems replaces the cached evidence rows for one finding.
func storeEvidenceItems(db *sql.DB, findingID, account string, items []string, now time.Time) error {
	if err := ensureEvidenceCache(db); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM finding_evidence_cache WHERE finding_id = ?`, findingID); err != nil {
		_ = tx.Rollback()
		return err
	}
	fetchedAt := now.UTC().Format(time.RFC3339)
	for i, content := range items {
		if _, err := tx.Exec(`INSERT INTO finding_evidence_cache (finding_id, account, item_no, content, fetched_at)
			VALUES (?,?,?,?,?)`, findingID, account, i, content, fetchedAt); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

type evidenceMatch struct {
	FindingID string `json:"finding_id"`
	Name      string `json:"name,omitempty"`
	Account   string `json:"account,omitempty"`
	Source    string `json:"source"` // "evidence" | "finding"
	Snippet   string `json:"snippet"`
}

// searchEvidenceCache greps the cached evidence rows case-insensitively.
func searchEvidenceCache(db *sql.DB, term string, limit int) ([]evidenceMatch, error) {
	if err := ensureEvidenceCache(db); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := db.Query(`SELECT finding_id, account, content FROM finding_evidence_cache
		WHERE content LIKE '%' || ? || '%' COLLATE NOCASE LIMIT ?`, term, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []evidenceMatch
	for rows.Next() {
		var id string
		var account, content sql.NullString
		if err := rows.Scan(&id, &account, &content); err != nil {
			return nil, err
		}
		out = append(out, evidenceMatch{
			FindingID: id, Account: account.String,
			Source: "evidence", Snippet: evidenceSnippet(content.String, term),
		})
	}
	return out, rows.Err()
}

// matchFindingsRaw greps the raw synced finding payloads for the term — catches
// indicators embedded in the finding body that the FTS name index misses.
func matchFindingsRaw(findings []findingRec, raws map[string]string, term string, limit int) []evidenceMatch {
	if limit <= 0 {
		limit = 50
	}
	lower := strings.ToLower(term)
	var out []evidenceMatch
	for _, f := range findings {
		raw, ok := raws[f.ID]
		if !ok {
			continue
		}
		if !strings.Contains(strings.ToLower(raw), lower) {
			continue
		}
		out = append(out, evidenceMatch{
			FindingID: f.ID, Name: f.Name, Account: f.Account,
			Source: "finding", Snippet: evidenceSnippet(raw, term),
		})
		if len(out) >= limit {
			break
		}
	}
	return out
}

// evidenceSnippet trims content around the first case-insensitive occurrence of
// term to a readable window.
func evidenceSnippet(content, term string) string {
	const window = 80
	idx := strings.Index(strings.ToLower(content), strings.ToLower(term))
	if idx < 0 {
		if len(content) > 2*window {
			return content[:2*window] + "…"
		}
		return content
	}
	start := idx - window
	if start < 0 {
		start = 0
	}
	end := idx + len(term) + window
	if end > len(content) {
		end = len(content)
	}
	// Snap to rune boundaries so a window edge never splits a multi-byte
	// UTF-8 sequence into a garbled partial rune.
	for start > 0 && content[start]&0xC0 == 0x80 {
		start--
	}
	for end < len(content) && content[end]&0xC0 == 0x80 {
		end++
	}
	snippet := content[start:end]
	if start > 0 {
		snippet = "…" + snippet
	}
	if end < len(content) {
		snippet += "…"
	}
	return strings.Join(strings.Fields(snippet), " ")
}

// extractEvidenceItems flattens an evidence API response into displayable
// strings: a JSON array becomes one string per element, an object with a list
// payload unwraps it, anything else is kept whole.
func extractEvidenceItems(raw json.RawMessage) []string {
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err == nil {
		items := make([]string, 0, len(arr))
		for _, e := range arr {
			items = append(items, string(e))
		}
		return items
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err == nil {
		for _, key := range []string{"data", "evidence", "items", "results"} {
			if inner, ok := obj[key]; ok {
				var innerArr []json.RawMessage
				if err := json.Unmarshal(inner, &innerArr); err == nil {
					items := make([]string, 0, len(innerArr))
					for _, e := range innerArr {
						items = append(items, string(e))
					}
					return items
				}
			}
		}
	}
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return nil
	}
	return []string{s}
}

// loadFindingRaws returns finding_id -> raw JSON payload for every synced
// finding, de-duplicated the same way loadFindings is.
func loadFindingRaws(s *store.Store) (map[string]string, error) {
	out := map[string]string{}
	for _, rt := range findingResourceTypes {
		rows, err := s.List(rt, analyticsListLimit)
		if err != nil {
			return nil, err
		}
		for _, raw := range rows {
			var probe struct {
				FindingID string `json:"finding_id"`
				ID        string `json:"id"`
			}
			if err := json.Unmarshal(raw, &probe); err != nil {
				continue
			}
			id := probe.FindingID
			if id == "" {
				id = probe.ID
			}
			if id == "" {
				continue
			}
			if _, seen := out[id]; !seen {
				out[id] = string(raw)
			}
		}
	}
	return out, nil
}

// fetchFailure records one failed live evidence fetch for the JSON envelope.
type fetchFailure struct {
	FindingID string `json:"finding_id"`
	Error     string `json:"error"`
}
