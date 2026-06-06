// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Shared logic for SuperOps transcendence commands. These commands answer
// cross-entity questions over the locally synced SQLite mirror that no single
// SuperOps GraphQL call can. The compute functions here are pure (they take
// already-parsed resource records and return result rows) so they are unit
// tested without a live API or a database.
//
// Records are the decoded `data` JSON objects from the generic `resources`
// table, keyed by resource_type ("tickets", "worklogs", "assets", "alerts",
// "clients", "invoices", "contracts", "sites", "users"). Nested relations
// (ticket.client, worklog.ticket, alert.asset, ...) are GraphQL sub-objects,
// so the helpers below resolve a nested object's id/name fields.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"superops-pp-cli/internal/store"
)

// rec is one decoded resource `data` object.
type rec map[string]any

// queryRecs returns the decoded `data` objects for a resource type from the
// local store's generic resources table.
func queryRecs(db *store.Store, resourceType string) ([]rec, error) {
	rows, err := db.Query(`SELECT data FROM resources WHERE resource_type = ?`, resourceType)
	if err != nil {
		return nil, fmt.Errorf("querying %s: %w", resourceType, err)
	}
	defer rows.Close()
	var out []rec
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			continue
		}
		var r rec
		if json.Unmarshal(data, &r) != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// openStoreForNovel opens the local store, returning a friendly error that
// points at `sync` when the database is missing.
func openStoreForNovel(ctx context.Context, dbPath string) (*store.Store, error) {
	if dbPath == "" {
		dbPath = defaultDBPath("superops-cli")
	}
	db, err := store.OpenWithContext(ctx, dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening local database: %w\nRun 'superops-cli sync' first.", err)
	}
	return db, nil
}

// --- scalar / nested accessors -------------------------------------------

func recStr(r rec, key string) string {
	v, ok := r[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case float64:
		// SuperOps ids arrive as strings, but be tolerant of numerics.
		return strings.TrimSuffix(fmt.Sprintf("%v", t), ".0")
	case bool:
		return fmt.Sprintf("%v", t)
	default:
		return fmt.Sprintf("%v", t)
	}
}

// nestedField returns the string value of sub inside a nested object field.
// e.g. nestedField(ticket, "client", "name") -> ticket.client.name.
func nestedField(r rec, key, sub string) string {
	v, ok := r[key]
	if !ok || v == nil {
		return ""
	}
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	sv, ok := m[sub]
	if !ok || sv == nil {
		return ""
	}
	return fmt.Sprintf("%v", sv)
}

func recBool(r rec, key string) bool {
	v, ok := r[key]
	if !ok || v == nil {
		return false
	}
	switch t := v.(type) {
	case bool:
		return t
	case float64:
		return t != 0
	case string:
		return strings.EqualFold(t, "true") || t == "1"
	}
	return false
}

func recInt(r rec, key string) int {
	v, ok := r[key]
	if !ok || v == nil {
		return 0
	}
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case string:
		var n int
		fmt.Sscanf(t, "%d", &n)
		return n
	}
	return 0
}

// parseSuperopsTime parses SuperOps timestamps. The API documents UTC ISO-8601
// without a zone suffix (e.g. "2022-04-10T10:15:30") and also emits RFC3339.
func parseSuperopsTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02 15:04:05",
		"2006-01-02",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	// Epoch millis as a numeric string.
	var ms int64
	if _, err := fmt.Sscanf(s, "%d", &ms); err == nil && ms > 1_000_000_000_000 {
		return time.UnixMilli(ms).UTC(), true
	}
	return time.Time{}, false
}

// ticketIsOpen reports whether a ticket is unresolved (no resolutionTime).
func ticketIsOpen(t rec) bool {
	return strings.TrimSpace(recStr(t, "resolutionTime")) == ""
}

// --- sla-watch ------------------------------------------------------------

// SLAWatchRow is one ticket on the SLA watch board.
type SLAWatchRow struct {
	TicketID     string `json:"ticketId"`
	DisplayID    string `json:"displayId"`
	Subject      string `json:"subject"`
	GroupBy      string `json:"groupBy"`           // technician or client name
	SLA          string `json:"sla,omitempty"`     // SLA policy name
	DueTime      string `json:"resolutionDueTime"` // ISO due time
	State        string `json:"state"`             // "breached" | "at-risk"
	MinutesToDue int    `json:"minutesToDue"`      // negative if already breached
}

// computeSLAWatch returns open tickets that are breached or at risk (due within
// window), grouped by technician or client. now and window are injected for
// testability.
func computeSLAWatch(tickets []rec, by string, window time.Duration, now time.Time) []SLAWatchRow {
	var rows []SLAWatchRow
	for _, t := range tickets {
		if !ticketIsOpen(t) {
			continue
		}
		due, ok := parseSuperopsTime(recStr(t, "resolutionDueTime"))
		if !ok {
			continue
		}
		delta := due.Sub(now)
		state := ""
		switch {
		case delta < 0:
			state = "breached"
		case delta <= window:
			state = "at-risk"
		default:
			continue
		}
		group := ""
		if by == "client" {
			group = nestedField(t, "client", "name")
		} else {
			group = nestedField(t, "technician", "name")
		}
		if group == "" {
			group = "(unassigned)"
		}
		rows = append(rows, SLAWatchRow{
			TicketID:     recStr(t, "ticketId"),
			DisplayID:    recStr(t, "displayId"),
			Subject:      recStr(t, "subject"),
			GroupBy:      group,
			SLA:          nestedField(t, "sla", "name"),
			DueTime:      recStr(t, "resolutionDueTime"),
			State:        state,
			MinutesToDue: int(delta.Minutes()),
		})
	}
	// Most-overdue first.
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].MinutesToDue < rows[j].MinutesToDue })
	return rows
}

// --- unbilled -------------------------------------------------------------

// UnbilledRow aggregates billable worklog time per client.
type UnbilledRow struct {
	Client          string  `json:"client"`
	BillableEntries int     `json:"billableEntries"`
	BillableMinutes int     `json:"billableMinutes"`
	BillableHours   float64 `json:"billableHours"`
}

// computeUnbilled sums billable worklog time per client by resolving each
// worklog's ticket to its client. since (zero = no filter) bounds by worklog
// start time. SuperOps' list API does not expose a per-entry "billed" flag, so
// this surfaces billable logged time per client to reconcile against invoices.
func computeUnbilled(worklogs, tickets []rec, clientFilter string, since time.Time) []UnbilledRow {
	// ticketId -> client name
	ticketClient := map[string]string{}
	for _, t := range tickets {
		id := recStr(t, "ticketId")
		if id == "" {
			id = recStr(t, "displayId")
		}
		if id != "" {
			ticketClient[id] = nestedField(t, "client", "name")
		}
	}
	type agg struct {
		entries int
		minutes int
	}
	byClient := map[string]*agg{}
	for _, w := range worklogs {
		if !recBool(w, "billable") {
			continue
		}
		if !since.IsZero() {
			if ts, ok := parseSuperopsTime(recStr(w, "startTime")); ok && ts.Before(since) {
				continue
			}
		}
		tid := nestedField(w, "ticket", "ticketId")
		if tid == "" {
			tid = nestedField(w, "ticket", "displayId")
		}
		client := ticketClient[tid]
		if client == "" {
			client = "(unknown client)"
		}
		if clientFilter != "" && !strings.EqualFold(client, clientFilter) {
			continue
		}
		a := byClient[client]
		if a == nil {
			a = &agg{}
			byClient[client] = a
		}
		a.entries++
		a.minutes += recInt(w, "timespent")
	}
	var rows []UnbilledRow
	for c, a := range byClient {
		rows = append(rows, UnbilledRow{
			Client:          c,
			BillableEntries: a.entries,
			BillableMinutes: a.minutes,
			BillableHours:   float64(a.minutes) / 60.0,
		})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].BillableMinutes > rows[j].BillableMinutes })
	return rows
}

// --- at-risk-assets -------------------------------------------------------

// AtRiskAssetRow is an asset that is both patch-risky and has an active alert.
type AtRiskAssetRow struct {
	AssetID     string `json:"assetId"`
	Name        string `json:"name"`
	HostName    string `json:"hostName"`
	Client      string `json:"client"`
	PatchStatus string `json:"patchStatus"`
	OpenAlerts  int    `json:"openAlerts"`
}

// patchStatusRisky reports whether an asset patch status signals missing or
// pending critical patches.
func patchStatusRisky(status string) bool {
	s := strings.ToLower(strings.TrimSpace(status))
	if s == "" {
		return false
	}
	for _, bad := range []string{"missing", "pending", "critical", "failed", "outdated", "available", "vulnerable", "non-compliant", "noncompliant"} {
		if strings.Contains(s, bad) {
			return true
		}
	}
	return false
}

// computeAtRiskAssets returns assets with a risky patch status that also carry
// at least one unresolved alert (asset->alert link). The exact asset->ticket
// link is not on the SuperOps list payloads, so an active alert is the synced
// proxy for "currently causing pain".
func computeAtRiskAssets(assets, alerts []rec, clientFilter string) []AtRiskAssetRow {
	openAlertsByAsset := map[string]int{}
	for _, a := range alerts {
		if strings.TrimSpace(recStr(a, "resolvedTime")) != "" {
			continue
		}
		if strings.EqualFold(recStr(a, "status"), "resolved") || strings.EqualFold(recStr(a, "status"), "closed") {
			continue
		}
		aid := nestedField(a, "asset", "assetId")
		if aid != "" {
			openAlertsByAsset[aid]++
		}
	}
	var rows []AtRiskAssetRow
	for _, as := range assets {
		ps := recStr(as, "patchStatus")
		if !patchStatusRisky(ps) {
			continue
		}
		aid := recStr(as, "assetId")
		open := openAlertsByAsset[aid]
		if open == 0 {
			continue
		}
		client := nestedField(as, "client", "name")
		if clientFilter != "" && !strings.EqualFold(client, clientFilter) {
			continue
		}
		rows = append(rows, AtRiskAssetRow{
			AssetID:     aid,
			Name:        recStr(as, "name"),
			HostName:    recStr(as, "hostName"),
			Client:      client,
			PatchStatus: ps,
			OpenAlerts:  open,
		})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].OpenAlerts > rows[j].OpenAlerts })
	return rows
}

// --- alert-coverage -------------------------------------------------------

// AlertCoverageRow groups alerts by client into resolved vs unresolved counts.
type AlertCoverageRow struct {
	Client   string `json:"client"`
	Open     int    `json:"openAlerts"`
	Resolved int    `json:"resolvedAlerts"`
	Total    int    `json:"totalAlerts"`
}

// computeAlertCoverage partitions alerts into resolved ("covered") vs
// unresolved ("uncovered / needs action") and groups by the alert's asset
// client. The exact alert->ticket linkage is not exposed on the SuperOps list
// payloads; resolved-vs-open status is the synced proxy for coverage.
func computeAlertCoverage(alerts []rec, assets []rec, clientFilter string) []AlertCoverageRow {
	assetClient := map[string]string{}
	for _, as := range assets {
		assetClient[recStr(as, "assetId")] = nestedField(as, "client", "name")
	}
	type agg struct{ open, resolved int }
	byClient := map[string]*agg{}
	for _, a := range alerts {
		resolved := strings.TrimSpace(recStr(a, "resolvedTime")) != "" ||
			strings.EqualFold(recStr(a, "status"), "resolved") ||
			strings.EqualFold(recStr(a, "status"), "closed")
		client := assetClient[nestedField(a, "asset", "assetId")]
		if client == "" {
			client = nestedField(a, "asset", "name")
		}
		if client == "" {
			client = "(unknown client)"
		}
		if clientFilter != "" && !strings.EqualFold(client, clientFilter) {
			continue
		}
		ag := byClient[client]
		if ag == nil {
			ag = &agg{}
			byClient[client] = ag
		}
		if resolved {
			ag.resolved++
		} else {
			ag.open++
		}
	}
	var rows []AlertCoverageRow
	for c, ag := range byClient {
		rows = append(rows, AlertCoverageRow{
			Client:   c,
			Open:     ag.open,
			Resolved: ag.resolved,
			Total:    ag.open + ag.resolved,
		})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Open > rows[j].Open })
	return rows
}

// --- stale-tickets --------------------------------------------------------

// StaleTicketRow is an open ticket with no recent activity.
type StaleTicketRow struct {
	TicketID     string `json:"ticketId"`
	DisplayID    string `json:"displayId"`
	Subject      string `json:"subject"`
	Client       string `json:"client"`
	Technician   string `json:"technician"`
	LastActivity string `json:"lastActivity"`
	IdleDays     int    `json:"idleDays"`
}

// computeStaleTickets returns open tickets whose most recent activity (the
// later of ticket.updatedTime and any worklog.startTime referencing the
// ticket) is older than days.
func computeStaleTickets(tickets, worklogs []rec, days int, now time.Time) []StaleTicketRow {
	// Latest worklog activity per ticket id.
	lastWorklog := map[string]time.Time{}
	for _, w := range worklogs {
		tid := nestedField(w, "ticket", "ticketId")
		if tid == "" {
			tid = nestedField(w, "ticket", "displayId")
		}
		if tid == "" {
			continue
		}
		for _, k := range []string{"endTime", "startTime"} {
			if ts, ok := parseSuperopsTime(recStr(w, k)); ok {
				if ts.After(lastWorklog[tid]) {
					lastWorklog[tid] = ts
				}
				break
			}
		}
	}
	cutoff := now.AddDate(0, 0, -days)
	var rows []StaleTicketRow
	for _, t := range tickets {
		if !ticketIsOpen(t) {
			continue
		}
		tid := recStr(t, "ticketId")
		last := time.Time{}
		if ts, ok := parseSuperopsTime(recStr(t, "updatedTime")); ok {
			last = ts
		} else if ts, ok := parseSuperopsTime(recStr(t, "createdTime")); ok {
			last = ts
		}
		if wl, ok := lastWorklog[tid]; ok && wl.After(last) {
			last = wl
		}
		if last.IsZero() || !last.Before(cutoff) {
			continue
		}
		rows = append(rows, StaleTicketRow{
			TicketID:     tid,
			DisplayID:    recStr(t, "displayId"),
			Subject:      recStr(t, "subject"),
			Client:       nestedField(t, "client", "name"),
			Technician:   nestedField(t, "technician", "name"),
			LastActivity: last.UTC().Format(time.RFC3339),
			IdleDays:     int(now.Sub(last).Hours() / 24),
		})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].IdleDays > rows[j].IdleDays })
	return rows
}

// --- client-360 -----------------------------------------------------------

// Client360 is a one-shot bundle of everything tied to a client.
type Client360 struct {
	Client       rec   `json:"client"`
	Sites        []rec `json:"sites"`
	Users        []rec `json:"users"`
	Contracts    []rec `json:"contracts"`
	OpenTickets  []rec `json:"openTickets"`
	Assets       []rec `json:"assets"`
	OpenInvoices []rec `json:"openInvoices"`
}

// matchesClient reports whether a record's client relation (or its own
// accountId/name) matches the given query (id or name, case-insensitive).
func matchesClient(r rec, accountID, name string) bool {
	cid := nestedField(r, "client", "accountId")
	cname := nestedField(r, "client", "name")
	if accountID != "" && cid == accountID {
		return true
	}
	if name != "" && strings.EqualFold(cname, name) {
		return true
	}
	return false
}

// invoiceIsOpen reports whether an invoice is unpaid (no paymentDate).
func invoiceIsOpen(inv rec) bool {
	return strings.TrimSpace(recStr(inv, "paymentDate")) == ""
}

// buildClient360 assembles the bundle for the client matching query (account id
// or name) out of the already-loaded resource slices.
func buildClient360(query string, clients, sites, users, contracts, tickets, assets, invoices []rec) (Client360, bool) {
	var c rec
	for _, cl := range clients {
		if recStr(cl, "accountId") == query || strings.EqualFold(recStr(cl, "name"), query) {
			c = cl
			break
		}
	}
	if c == nil {
		return Client360{}, false
	}
	accountID := recStr(c, "accountId")
	name := recStr(c, "name")
	out := Client360{Client: c, Sites: []rec{}, Users: []rec{}, Contracts: []rec{}, OpenTickets: []rec{}, Assets: []rec{}, OpenInvoices: []rec{}}
	for _, s := range sites {
		if matchesClient(s, accountID, name) {
			out.Sites = append(out.Sites, s)
		}
	}
	for _, u := range users {
		if matchesClient(u, accountID, name) {
			out.Users = append(out.Users, u)
		}
	}
	for _, ct := range contracts {
		if matchesClient(ct, accountID, name) {
			out.Contracts = append(out.Contracts, ct)
		}
	}
	for _, t := range tickets {
		if matchesClient(t, accountID, name) && ticketIsOpen(t) {
			out.OpenTickets = append(out.OpenTickets, t)
		}
	}
	for _, a := range assets {
		if matchesClient(a, accountID, name) {
			out.Assets = append(out.Assets, a)
		}
	}
	for _, inv := range invoices {
		if matchesClient(inv, accountID, name) && invoiceIsOpen(inv) {
			out.OpenInvoices = append(out.OpenInvoices, inv)
		}
	}
	return out, true
}

// --- context-ticket -------------------------------------------------------

// TicketContext is an agent-shaped bundle for one ticket.
type TicketContext struct {
	Ticket   rec    `json:"ticket"`
	Client   rec    `json:"client,omitempty"`
	SLA      rec    `json:"sla,omitempty"`
	Worklogs []rec  `json:"worklogs"`
	Note     string `json:"note,omitempty"`
}

// buildTicketContext assembles a single ticket plus its synced worklogs and the
// client/sla sub-objects embedded on the ticket. Conversation and note threads
// are not synced (no list resource), so they are omitted with a note.
func buildTicketContext(ticketID string, tickets, worklogs []rec) (TicketContext, bool) {
	var t rec
	for _, cand := range tickets {
		if recStr(cand, "ticketId") == ticketID || recStr(cand, "displayId") == ticketID {
			t = cand
			break
		}
	}
	if t == nil {
		return TicketContext{}, false
	}
	out := TicketContext{Ticket: t, Worklogs: []rec{}}
	if m, ok := t["client"].(map[string]any); ok {
		out.Client = rec(m)
	}
	if m, ok := t["sla"].(map[string]any); ok {
		out.SLA = rec(m)
	}
	tid := recStr(t, "ticketId")
	disp := recStr(t, "displayId")
	for _, w := range worklogs {
		wtid := nestedField(w, "ticket", "ticketId")
		wdisp := nestedField(w, "ticket", "displayId")
		if (tid != "" && wtid == tid) || (disp != "" && wdisp == disp) {
			out.Worklogs = append(out.Worklogs, w)
		}
	}
	out.Note = "conversation and note threads are not synced locally; fetch live with 'superops-cli tickets get " + ticketID + "'"
	return out, true
}
