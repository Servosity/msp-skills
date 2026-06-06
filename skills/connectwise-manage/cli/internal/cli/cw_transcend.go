// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored transcendence support: shared store-loading, ConnectWise JSON
// field extraction, and the pure-logic functions behind the novel commands
// (unbilled, account, board, stale, workload, agreement-burn, condition).
// Kept separate from the generated command files so the compute is unit-tested
// independently of Cobra wiring.

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"connectwise-manage-pp-cli/internal/store"
	"github.com/spf13/cobra"
)

// ConnectWise resource_type keys exactly as `sync` writes them into the generic
// `resources` table (see internal/cli/sync.go syncResourcePath). These are the
// uniform query surface for every transcendence command.
const (
	cwTickets     = "service-tickets"   // /service/tickets
	cwTimeEntries = "time-entries"      // /time/entries
	cwCompanies   = "company-companies" // /company/companies
	cwContacts    = "company"           // /company/contacts (bare "company" resource)
	cwConfigs     = "company-configurations"
	cwAgreements  = "finance-agreements"
	cwMembers     = "system-members"
)

// errNoStore signals the local SQLite store has not been created yet. Commands
// translate it into an empty result + a sync hint and exit 0, so a fresh CLI
// (or a verify --json probe before any sync) never fails hard.
var errNoStore = errors.New("no local store yet")

func cwDBPath() string { return defaultDBPath("connectwise-manage-cli") }

// cwOpenStore opens the local store read-only. Returns errNoStore (wrapped)
// when the DB file does not exist so callers can degrade gracefully.
func cwOpenStore(ctx context.Context) (*store.Store, error) {
	p := cwDBPath()
	if _, err := os.Stat(p); err != nil {
		return nil, errNoStore
	}
	return store.OpenWithContext(ctx, p)
}

// cwLoad returns every synced record for a resource_type as a decoded JSON
// object. Uses the store's UseNumber decoder, so numeric fields arrive as
// json.Number — the cwFloat/cwInt helpers below handle that explicitly.
func cwLoad(ctx context.Context, db *store.Store, resourceType string) ([]map[string]any, error) {
	rows, err := db.DB().QueryContext(ctx, `SELECT data FROM resources WHERE resource_type = ?`, resourceType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		obj, derr := store.DecodeJSONObject(json.RawMessage(data))
		if derr != nil {
			continue
		}
		out = append(out, obj)
	}
	return out, rows.Err()
}

// --- field extraction (json.Number-safe) ---

func cwAsString(v any) string {
	switch n := v.(type) {
	case nil:
		return ""
	case string:
		return n
	case json.Number:
		return n.String()
	case float64:
		return strconv.FormatFloat(n, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(n)
	default:
		return fmt.Sprintf("%v", n)
	}
}

// cwFloat coerces a json.Number / float64 / int / numeric-string into a float.
func cwFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case string:
		f, err := strconv.ParseFloat(n, 64)
		return f, err == nil
	}
	return 0, false
}

func cwInt(v any) (int, bool) {
	if f, ok := cwFloat(v); ok {
		return int(f), true
	}
	return 0, false
}

func cwBool(m map[string]any, key string) bool {
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

// cwTopID returns the record's own integer id (CW entities all carry `id`).
func cwTopID(m map[string]any) int {
	if v, ok := m["id"]; ok {
		if n, ok := cwInt(v); ok {
			return n
		}
	}
	return 0
}

// cwObj returns a nested object field (e.g. ticket["board"]).
func cwObj(m map[string]any, key string) map[string]any {
	if v, ok := m[key]; ok {
		if o, ok := v.(map[string]any); ok {
			return o
		}
	}
	return nil
}

// cwRefName returns a reference object's display name, preferring name then
// identifier (CW ReferenceItem = {id, name, identifier}).
func cwRefName(m map[string]any, key string) string {
	o := cwObj(m, key)
	if o == nil {
		return ""
	}
	if s := cwAsString(o["name"]); s != "" {
		return s
	}
	return cwAsString(o["identifier"])
}

// cwRefID returns a reference object's id.
func cwRefID(m map[string]any, key string) (int, bool) {
	o := cwObj(m, key)
	if o == nil {
		return 0, false
	}
	if v, ok := o["id"]; ok {
		return cwInt(v)
	}
	return 0, false
}

// cwRefIdentifier returns a reference object's identifier (login-style key),
// falling back to name.
func cwRefIdentifier(m map[string]any, key string) string {
	o := cwObj(m, key)
	if o == nil {
		return ""
	}
	if s := cwAsString(o["identifier"]); s != "" {
		return s
	}
	return cwAsString(o["name"])
}

var cwTimeLayouts = []string{
	time.RFC3339Nano, time.RFC3339,
	"2006-01-02T15:04:05Z",
	"2006-01-02T15:04:05",
	"2006-01-02",
}

func cwParseTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, l := range cwTimeLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// cwUpdated returns the record's last-updated timestamp, preferring
// _info.lastUpdated, then top-level lastUpdated, then dateEntered.
func cwUpdated(m map[string]any) (time.Time, bool) {
	if info := cwObj(m, "_info"); info != nil {
		if t, ok := cwParseTime(cwAsString(info["lastUpdated"])); ok {
			return t, true
		}
		if t, ok := cwParseTime(cwAsString(info["dateEntered"])); ok {
			return t, true
		}
	}
	if t, ok := cwParseTime(cwAsString(m["lastUpdated"])); ok {
		return t, true
	}
	if t, ok := cwParseTime(cwAsString(m["dateEntered"])); ok {
		return t, true
	}
	return time.Time{}, false
}

func cwAgeDays(from, now time.Time) float64 {
	if from.IsZero() {
		return -1
	}
	return now.Sub(from).Hours() / 24.0
}

// cwEmit renders a result as JSON for agents/pipes (or when --json/--compact/
// --csv set) and as a human table otherwise.
func cwEmit(cmd *cobra.Command, flags *rootFlags, jsonVal any, headers []string, rows [][]string) error {
	if flags.asJSON || flags.compact || flags.csv || flags.quiet || (!isTerminal(cmd.OutOrStdout()) && !humanFriendly) {
		return flags.printJSON(cmd, jsonVal)
	}
	if len(rows) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "(no matching records)")
		return nil
	}
	return flags.printTable(cmd, headers, rows)
}

// cwNoStoreHint prints a sync hint to stderr and emits the empty result so the
// command exits 0 (honest empty, never fabricated data).
func cwNoStoreHint(cmd *cobra.Command, flags *rootFlags, empty any, headers []string, resources string) error {
	fmt.Fprintf(cmd.ErrOrStderr(), "no synced data yet — run `connectwise-manage-cli sync %s` first\n", resources)
	return cwEmit(cmd, flags, empty, headers, nil)
}

// =====================================================================
// Pure compute (unit-tested in the *_test.go files)
// =====================================================================

type ticketRow struct {
	ID          int       `json:"id"`
	Summary     string    `json:"summary"`
	Company     string    `json:"company,omitempty"`
	Board       string    `json:"board,omitempty"`
	Status      string    `json:"status,omitempty"`
	Priority    string    `json:"priority,omitempty"`
	Owner       string    `json:"owner,omitempty"`
	Closed      bool      `json:"closed"`
	Updated     time.Time `json:"updated,omitempty"`
	AgeDays     float64   `json:"age_days,omitempty"`
	HoursLogged float64   `json:"hours_logged"`
}

// cwTicketRow projects a raw ticket object into a ticketRow (without hours/age,
// which the callers fill in).
func cwTicketRow(t map[string]any, now time.Time) ticketRow {
	r := ticketRow{
		ID:       cwTopID(t),
		Summary:  cwAsString(t["summary"]),
		Company:  firstNonEmpty(cwRefIdentifier(t, "company"), cwRefName(t, "company")),
		Board:    cwRefName(t, "board"),
		Status:   cwRefName(t, "status"),
		Priority: cwRefName(t, "priority"),
		Owner:    firstNonEmpty(cwRefIdentifier(t, "owner"), cwRefName(t, "owner")),
		Closed:   cwBool(t, "closedFlag"),
	}
	if up, ok := cwUpdated(t); ok {
		r.Updated = up
		r.AgeDays = round1(cwAgeDays(up, now))
	}
	return r
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func round1(f float64) float64 {
	return float64(int(f*10+0.5)) / 10
}

func cwItoa(i int) string { return strconv.Itoa(i) }

func cwFtoa(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

func cwTrunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

// cwLoggedHours returns the effective logged hours for a time entry:
// hoursBilled when present and non-zero, else actualHours. The zero-billed
// fallback is intentional — these views measure effort, not invoicing, so an
// entry deliberately billed at 0 still counts its actual hours.
func cwLoggedHours(te map[string]any) float64 {
	h, ok := cwFloat(te["hoursBilled"])
	if !ok || h == 0 {
		h, _ = cwFloat(te["actualHours"])
	}
	return h
}

// ticketTimeIndex sums logged hours per service-ticket id from time entries.
// Only chargeToType == "ServiceTicket" rows count; hoursBilled wins, falling
// back to actualHours (see cwLoggedHours).
func ticketTimeIndex(timeEntries []map[string]any) map[int]float64 {
	idx := map[int]float64{}
	for _, te := range timeEntries {
		if !strings.EqualFold(cwAsString(te["chargeToType"]), "ServiceTicket") {
			continue
		}
		tid, ok := cwInt(te["chargeToId"])
		if !ok || tid == 0 {
			continue
		}
		idx[tid] += cwLoggedHours(te)
	}
	return idx
}

// computeUnbilled returns tickets updated since `since` (zero = no window)
// whose logged hours are at or below `threshold`, optionally scoped to a member
// (matched against owner identifier/name). Sorted least-hours-then-oldest.
func computeUnbilled(tickets, timeEntries []map[string]any, since time.Time, member string, threshold float64, now time.Time) []ticketRow {
	idx := ticketTimeIndex(timeEntries)
	out := make([]ticketRow, 0)
	for _, t := range tickets {
		row := cwTicketRow(t, now)
		if row.ID == 0 {
			continue
		}
		if !since.IsZero() && (row.Updated.IsZero() || row.Updated.Before(since)) {
			continue
		}
		if member != "" && !strings.EqualFold(row.Owner, member) {
			continue
		}
		row.HoursLogged = round1(idx[row.ID])
		if row.HoursLogged > threshold {
			continue
		}
		out = append(out, row)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].HoursLogged != out[j].HoursLogged {
			return out[i].HoursLogged < out[j].HoursLogged
		}
		return out[i].Updated.Before(out[j].Updated)
	})
	return out
}

// computeStale returns open tickets not updated since `olderThan`, optionally
// scoped to a board (matched against board name or numeric id), oldest first.
func computeStale(tickets []map[string]any, olderThan time.Time, board string, now time.Time) []ticketRow {
	out := make([]ticketRow, 0)
	for _, t := range tickets {
		if cwBool(t, "closedFlag") {
			continue
		}
		row := cwTicketRow(t, now)
		if row.ID == 0 || row.Updated.IsZero() {
			continue
		}
		if !row.Updated.Before(olderThan) {
			continue
		}
		if board != "" && !matchesBoard(t, board) {
			continue
		}
		out = append(out, row)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Updated.Before(out[j].Updated) })
	return out
}

func matchesBoard(t map[string]any, board string) bool {
	if strings.EqualFold(cwRefName(t, "board"), board) {
		return true
	}
	if id, ok := cwRefID(t, "board"); ok {
		if strconv.Itoa(id) == strings.TrimSpace(board) {
			return true
		}
	}
	return false
}

// computeBoard returns open tickets on a board (matched by name or id),
// optionally only unassigned, oldest-first (most urgent for triage).
func computeBoard(tickets []map[string]any, board string, unassignedOnly bool, now time.Time) []ticketRow {
	out := make([]ticketRow, 0)
	for _, t := range tickets {
		if cwBool(t, "closedFlag") {
			continue
		}
		if board != "" && !matchesBoard(t, board) {
			continue
		}
		row := cwTicketRow(t, now)
		if row.ID == 0 {
			continue
		}
		if unassignedOnly && strings.TrimSpace(row.Owner) != "" {
			continue
		}
		out = append(out, row)
	}
	sort.SliceStable(out, func(i, j int) bool {
		// Oldest (longest since update) first.
		return out[i].Updated.Before(out[j].Updated)
	})
	return out
}

type workloadRow struct {
	Owner       string  `json:"owner"`
	OpenTickets int     `json:"open_tickets"`
	OldestDays  float64 `json:"oldest_age_days"`
}

// computeWorkload aggregates open tickets per owner, with the oldest open age.
func computeWorkload(tickets []map[string]any, board string, now time.Time) []workloadRow {
	type agg struct {
		count  int
		oldest float64
	}
	m := map[string]*agg{}
	for _, t := range tickets {
		if cwBool(t, "closedFlag") {
			continue
		}
		if board != "" && !matchesBoard(t, board) {
			continue
		}
		owner := firstNonEmpty(cwRefIdentifier(t, "owner"), cwRefName(t, "owner"), "(unassigned)")
		a := m[owner]
		if a == nil {
			a = &agg{}
			m[owner] = a
		}
		a.count++
		if up, ok := cwUpdated(t); ok {
			if age := round1(cwAgeDays(up, now)); age > a.oldest {
				a.oldest = age
			}
		}
	}
	out := make([]workloadRow, 0, len(m))
	for owner, a := range m {
		out = append(out, workloadRow{Owner: owner, OpenTickets: a.count, OldestDays: a.oldest})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].OpenTickets > out[j].OpenTickets })
	return out
}

type burnRow struct {
	AgreementID int     `json:"agreement_id"`
	Name        string  `json:"name"`
	Company     string  `json:"company,omitempty"`
	Units       string  `json:"units,omitempty"`
	Limit       float64 `json:"limit"`
	UsedHours   float64 `json:"used_hours"`
	Pct         float64 `json:"utilization_pct"`
	Over        bool    `json:"over_limit"`
}

// agreementHours sums logged hours per agreement id from time entries within
// the window.
func agreementHours(timeEntries []map[string]any, since time.Time) map[int]float64 {
	idx := map[int]float64{}
	for _, te := range timeEntries {
		aid, ok := cwRefID(te, "agreement")
		if !ok || aid == 0 {
			continue
		}
		if !since.IsZero() {
			if up, ok := cwParseTime(cwAsString(te["dateEntered"])); ok && up.Before(since) {
				continue
			}
		}
		idx[aid] += cwLoggedHours(te)
	}
	return idx
}

// computeAgreementBurn joins agreements to logged hours and computes
// utilization vs the agreement's hour allotment (when applicationUnits=Hours).
func computeAgreementBurn(agreements, timeEntries []map[string]any, companyFilter string, since time.Time) []burnRow {
	hrs := agreementHours(timeEntries, since)
	out := make([]burnRow, 0)
	for _, a := range agreements {
		id := cwTopID(a)
		if id == 0 {
			continue
		}
		company := firstNonEmpty(cwRefIdentifier(a, "company"), cwRefName(a, "company"))
		if companyFilter != "" && !strings.EqualFold(company, companyFilter) {
			continue
		}
		row := burnRow{
			AgreementID: id,
			Name:        cwAsString(a["name"]),
			Company:     company,
			Units:       cwAsString(a["applicationUnits"]),
			UsedHours:   round1(hrs[id]),
		}
		if lim, ok := cwFloat(a["applicationLimit"]); ok && lim > 0 {
			row.Limit = lim
			if strings.EqualFold(row.Units, "Hours") {
				row.Pct = round1(row.UsedHours / lim * 100)
				row.Over = row.UsedHours > lim
			}
		}
		out = append(out, row)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Pct > out[j].Pct })
	return out
}

type accountCard struct {
	Found          bool     `json:"found"`
	ID             int      `json:"id,omitempty"`
	Identifier     string   `json:"identifier,omitempty"`
	Name           string   `json:"name,omitempty"`
	Status         string   `json:"status,omitempty"`
	Contacts       int      `json:"contacts"`
	ContactNames   []string `json:"contact_names,omitempty"`
	Agreements     []string `json:"agreements,omitempty"`
	Configurations int      `json:"configurations"`
	OpenTickets    int      `json:"open_tickets"`
	LastActivity   string   `json:"last_activity,omitempty"`
}

// matchCompany reports whether company c matches the query (numeric id, or
// identifier/name case-insensitively).
func matchCompany(c map[string]any, query string) bool {
	q := strings.TrimSpace(query)
	if q == "" {
		return false
	}
	if id, err := strconv.Atoi(q); err == nil && cwTopID(c) == id {
		return true
	}
	if strings.EqualFold(cwAsString(c["identifier"]), q) {
		return true
	}
	return strings.EqualFold(cwAsString(c["name"]), q)
}

// computeAccount assembles the 360 card for the company matching `query` from
// the synced tables. Returns {Found:false} when no company matches.
func computeAccount(companies, contacts, agreements, configs, tickets []map[string]any, query string) accountCard {
	var card accountCard
	var company map[string]any
	for _, c := range companies {
		if matchCompany(c, query) {
			company = c
			break
		}
	}
	if company == nil {
		return card
	}
	card.Found = true
	card.ID = cwTopID(company)
	card.Identifier = cwAsString(company["identifier"])
	card.Name = cwAsString(company["name"])
	card.Status = cwRefName(company, "status")

	for _, ct := range contacts {
		if id, ok := cwRefID(ct, "company"); ok && id == card.ID {
			card.Contacts++
			name := strings.TrimSpace(cwAsString(ct["firstName"]) + " " + cwAsString(ct["lastName"]))
			if name == "" {
				name = cwAsString(ct["name"])
			}
			if name != "" && len(card.ContactNames) < 10 {
				card.ContactNames = append(card.ContactNames, name)
			}
		}
	}
	for _, a := range agreements {
		if id, ok := cwRefID(a, "company"); ok && id == card.ID && !cwBool(a, "cancelledFlag") {
			if n := cwAsString(a["name"]); n != "" {
				card.Agreements = append(card.Agreements, n)
			}
		}
	}
	for _, cf := range configs {
		if id, ok := cwRefID(cf, "company"); ok && id == card.ID {
			card.Configurations++
		}
	}
	var latest time.Time
	for _, t := range tickets {
		if id, ok := cwRefID(t, "company"); ok && id == card.ID {
			if !cwBool(t, "closedFlag") {
				card.OpenTickets++
			}
			if up, ok := cwUpdated(t); ok && up.After(latest) {
				latest = up
			}
		}
	}
	if !latest.IsZero() {
		card.LastActivity = latest.UTC().Format(time.RFC3339)
	}
	return card
}

// --- conditions DSL builder (pure) ---

type condClause struct {
	Field string
	Op    string
	Value string
}

var cwCondOps = map[string]bool{
	"=": true, "!=": true, "<": true, "<=": true, ">": true, ">=": true,
	"contains": true, "like": true, "in": true, "not in": true,
}

// buildConditions assembles a validated ConnectWise `conditions` expression
// from clauses. Strings are double-quoted, ISO-8601 dates are bracketed,
// numbers/booleans are bare, and `in`/`not in` values are comma lists wrapped
// in parens. join is "and" (default) or "or"; an OR set is wrapped in parens
// per ConnectWise's AND-default grammar.
func buildConditions(clauses []condClause, join string) (string, error) {
	if len(clauses) == 0 {
		return "", errors.New("no clauses: provide at least one --field/--op/--value")
	}
	join = strings.ToLower(strings.TrimSpace(join))
	if join == "" {
		join = "and"
	}
	if join != "and" && join != "or" {
		return "", fmt.Errorf("invalid --join %q (use and|or)", join)
	}
	parts := make([]string, 0, len(clauses))
	for i, c := range clauses {
		field := strings.TrimSpace(c.Field)
		if field == "" {
			return "", fmt.Errorf("clause %d: empty --field", i+1)
		}
		op := strings.ToLower(strings.TrimSpace(c.Op))
		if !cwCondOps[op] {
			return "", fmt.Errorf("clause %d: invalid --op %q (valid: = != < <= > >= contains like in \"not in\")", i+1, c.Op)
		}
		val, err := formatCondValue(op, c.Value)
		if err != nil {
			return "", fmt.Errorf("clause %d (%s): %w", i+1, field, err)
		}
		parts = append(parts, field+" "+op+" "+val)
	}
	if len(parts) == 1 {
		return parts[0], nil
	}
	if join == "or" {
		return "(" + strings.Join(parts, " OR ") + ")", nil
	}
	return strings.Join(parts, " AND "), nil
}

var cwISODate = []string{"2006-01-02T15:04:05Z", "2006-01-02T15:04:05", "2006-01-02", time.RFC3339}

func looksLikeISODate(s string) bool {
	for _, l := range cwISODate {
		if _, err := time.Parse(l, s); err == nil {
			return true
		}
	}
	return false
}

// formatCondValue renders a single condition value per the ConnectWise grammar.
func formatCondValue(op, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if op == "in" || op == "not in" {
		if raw == "" {
			return "", errors.New("in/not in requires a comma-separated value list")
		}
		elems := strings.Split(raw, ",")
		rendered := make([]string, 0, len(elems))
		for _, e := range elems {
			rendered = append(rendered, formatScalar(strings.TrimSpace(e)))
		}
		return "(" + strings.Join(rendered, ",") + ")", nil
	}
	if raw == "" {
		return `""`, nil
	}
	return formatScalar(raw), nil
}

// explainConditions splits a conditions expression into its top-level clauses,
// respecting parentheses and double-quoted strings, and reports the detected
// join ("AND", "OR", or "" for a single clause). A leading/trailing paren pair
// wrapping the whole expression (an OR group) is unwrapped for display.
func explainConditions(s string) (clauses []string, join string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, ""
	}
	// Unwrap a single fully-enclosing paren group.
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") && enclosingParens(s) {
		s = strings.TrimSpace(s[1 : len(s)-1])
	}
	depth := 0
	inQuote := false
	var cur strings.Builder
	flush := func() {
		c := strings.TrimSpace(cur.String())
		if c != "" {
			clauses = append(clauses, c)
		}
		cur.Reset()
	}
	runes := []rune(s)
	i := 0
	for i < len(runes) {
		r := runes[i]
		switch {
		case r == '"':
			inQuote = !inQuote
			cur.WriteRune(r)
			i++
		case inQuote:
			cur.WriteRune(r)
			i++
		case r == '(' || r == '[':
			depth++
			cur.WriteRune(r)
			i++
		case r == ')' || r == ']':
			depth--
			cur.WriteRune(r)
			i++
		case depth == 0 && matchKeyword(runes, i, "AND"):
			flush()
			if join == "" || join == "AND" {
				join = "AND"
			} else {
				join = "MIXED"
			}
			i += 5 // " AND " consumed including surrounding spaces handled below
		case depth == 0 && matchKeyword(runes, i, "OR"):
			flush()
			if join == "" {
				join = "OR"
			} else if join != "OR" {
				join = "MIXED"
			}
			i += 4
		default:
			cur.WriteRune(r)
			i++
		}
	}
	flush()
	return clauses, join
}

// matchKeyword reports whether a space-delimited keyword (AND/OR) begins at
// index i (expects a leading space at i and a trailing space after the word).
func matchKeyword(runes []rune, i int, kw string) bool {
	// require " KW " — leading space at i, keyword, trailing space.
	seq := " " + kw + " "
	sr := []rune(seq)
	if i+len(sr) > len(runes) {
		return false
	}
	for j, c := range sr {
		rc := runes[i+j]
		if j == 0 || j == len(sr)-1 {
			if rc != ' ' {
				return false
			}
			continue
		}
		// case-insensitive compare for the keyword letters
		if toUpperRune(rc) != c {
			return false
		}
	}
	return true
}

func toUpperRune(r rune) rune {
	if r >= 'a' && r <= 'z' {
		return r - 32
	}
	return r
}

// enclosingParens reports whether the leading '(' matches the trailing ')'
// (i.e. the whole string is one parenthesized group, not "(a) AND (b)").
func enclosingParens(s string) bool {
	depth := 0
	for i, r := range s {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 && i != len(s)-1 {
				return false
			}
		}
	}
	return depth == 0
}

// formatScalar quotes strings, brackets ISO dates, and leaves
// numbers/booleans/null bare. A value already quoted/bracketed is passed
// through unchanged.
func formatScalar(s string) string {
	if s == "" {
		return `""`
	}
	// Pass through values the user already quoted/bracketed. Require length
	// >= 2 so a lone `"` or `[` is not misread as a balanced pair.
	if len(s) >= 2 && ((s[0] == '"' && s[len(s)-1] == '"') ||
		(s[0] == '[' && s[len(s)-1] == ']')) {
		return s
	}
	switch strings.ToLower(s) {
	case "true", "false", "null":
		return strings.ToLower(s)
	}
	// Bare-number check: plain decimal literals only. ParseFloat alone also
	// accepts NaN/Inf/Infinity and hex floats, which ConnectWise treats as
	// strings, so gate on the decimal shape first.
	if cwPlainNumber.MatchString(s) {
		if _, err := strconv.ParseFloat(s, 64); err == nil {
			return s
		}
	}
	if looksLikeISODate(s) {
		return "[" + s + "]"
	}
	// Escape backslashes before quotes so a value containing `\` or a
	// trailing backslash cannot break out of the double-quoted context.
	escaped := strings.ReplaceAll(s, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}

// cwPlainNumber matches plain decimal numeric literals (optionally signed,
// optional fraction, optional exponent) — the only shapes ConnectWise
// conditions accept bare.
var cwPlainNumber = regexp.MustCompile(`^[+-]?(\d+\.?\d*|\.\d+)([eE][+-]?\d+)?$`)
