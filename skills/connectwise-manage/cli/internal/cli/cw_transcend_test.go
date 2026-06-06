// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"connectwise-manage-pp-cli/internal/store"
)

// num mimics the store's UseNumber() decoder so tests exercise the json.Number
// path the real data takes (a float64 path would mask the int-extraction trap).
func num(s string) json.Number { return json.Number(s) }

func ref(id string, name string) map[string]any {
	return map[string]any{"id": num(id), "name": name}
}
func refIdent(id, identifier, name string) map[string]any {
	return map[string]any{"id": num(id), "identifier": identifier, "name": name}
}

func TestCwIntHandlesJSONNumber(t *testing.T) {
	if got, ok := cwInt(num("42")); !ok || got != 42 {
		t.Fatalf("cwInt(json.Number) = %d,%v; want 42,true", got, ok)
	}
	if got, ok := cwInt(float64(7)); !ok || got != 7 {
		t.Fatalf("cwInt(float64) = %d,%v; want 7,true", got, ok)
	}
	m := map[string]any{"id": num("99")}
	if cwTopID(m) != 99 {
		t.Fatalf("cwTopID with json.Number id = %d; want 99", cwTopID(m))
	}
	if id, ok := cwRefID(map[string]any{"board": ref("3", "HD")}, "board"); !ok || id != 3 {
		t.Fatalf("cwRefID = %d,%v; want 3,true", id, ok)
	}
}

func TestBuildConditions(t *testing.T) {
	cases := []struct {
		name    string
		clauses []condClause
		join    string
		want    string
		wantErr bool
	}{
		{"single-string", []condClause{{"board/name", "=", "Help Desk"}}, "and", `board/name = "Help Desk"`, false},
		{"bool-bare", []condClause{{"closedFlag", "=", "false"}}, "and", `closedFlag = false`, false},
		{"number-bare", []condClause{{"id", ">", "100"}}, "and", `id > 100`, false},
		{"in-list", []condClause{{"board/id", "in", "2,3,4"}}, "and", `board/id in (2,3,4)`, false},
		{"date-bracket", []condClause{{"dateEntered", ">", "2024-01-01T00:00:00Z"}}, "and", `dateEntered > [2024-01-01T00:00:00Z]`, false},
		{"two-and", []condClause{{"a", "=", "x"}, {"b", "=", "y"}}, "and", `a = "x" AND b = "y"`, false},
		{"two-or-wrapped", []condClause{{"a", "=", "x"}, {"b", "=", "y"}}, "or", `(a = "x" OR b = "y")`, false},
		{"bad-op", []condClause{{"a", "~=", "x"}}, "and", "", true},
		{"empty-field", []condClause{{"", "=", "x"}}, "and", "", true},
		{"no-clauses", nil, "and", "", true},
		{"bad-join", []condClause{{"a", "=", "x"}, {"b", "=", "y"}}, "xor", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := buildConditions(c.clauses, c.join)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.want {
				t.Fatalf("buildConditions = %q; want %q", got, c.want)
			}
		})
	}
}

func TestExplainConditions(t *testing.T) {
	clauses, join := explainConditions(`board/name="Help Desk" AND closedFlag=false`)
	if join != "AND" || len(clauses) != 2 {
		t.Fatalf("explain AND = %v join=%q; want 2 clauses, AND", clauses, join)
	}
	if clauses[0] != `board/name="Help Desk"` || clauses[1] != `closedFlag=false` {
		t.Fatalf("explain AND clauses = %v", clauses)
	}
	// OR group unwrapped, and AND inside quotes/parens must not split.
	c2, j2 := explainConditions(`(status/id in (1,2) OR summary like "%a AND b%")`)
	if j2 != "OR" || len(c2) != 2 {
		t.Fatalf("explain OR = %v join=%q; want 2 clauses, OR", c2, j2)
	}
	if c2[1] != `summary like "%a AND b%"` {
		t.Fatalf("explain must not split AND inside a quoted value: %q", c2[1])
	}
}

func TestComputeUnbilled(t *testing.T) {
	tickets := []map[string]any{
		{"id": num("1"), "summary": "A", "owner": refIdent("11", "jdoe", "John"), "closedFlag": false},
		{"id": num("2"), "summary": "B", "owner": refIdent("12", "asmith", "Amy"), "closedFlag": false},
	}
	times := []map[string]any{
		{"chargeToType": "ServiceTicket", "chargeToId": num("1"), "hoursBilled": num("2.0")},
		{"chargeToType": "Activity", "chargeToId": num("2"), "hoursBilled": num("3.0")}, // not a ticket: ignored
	}
	got := computeUnbilled(tickets, times, time.Time{}, "", 0, time.Now())
	if len(got) != 1 || got[0].ID != 2 {
		t.Fatalf("unbilled = %+v; want only ticket 2 (zero ServiceTicket hours)", got)
	}
	// member filter: jdoe owns ticket 1 which has 2h > threshold → empty.
	if g := computeUnbilled(tickets, times, time.Time{}, "jdoe", 0, time.Now()); len(g) != 0 {
		t.Fatalf("unbilled member=jdoe = %+v; want empty", g)
	}
}

func TestComputeStale(t *testing.T) {
	now := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
	tickets := []map[string]any{
		{"id": num("1"), "summary": "old-open", "closedFlag": false, "dateEntered": "2024-01-10T00:00:00Z"},
		{"id": num("2"), "summary": "recent-open", "closedFlag": false, "dateEntered": "2024-01-31T00:00:00Z"},
		{"id": num("3"), "summary": "old-closed", "closedFlag": true, "dateEntered": "2024-01-05T00:00:00Z"},
	}
	got := computeStale(tickets, now.AddDate(0, 0, -5), "", now)
	if len(got) != 1 || got[0].ID != 1 {
		t.Fatalf("stale = %+v; want only ticket 1 (open + >5d old)", got)
	}
}

func TestComputeWorkload(t *testing.T) {
	tickets := []map[string]any{
		{"id": num("1"), "owner": refIdent("11", "jdoe", "John"), "closedFlag": false},
		{"id": num("2"), "owner": refIdent("11", "jdoe", "John"), "closedFlag": false},
		{"id": num("3"), "owner": refIdent("12", "asmith", "Amy"), "closedFlag": false},
		{"id": num("4"), "owner": refIdent("11", "jdoe", "John"), "closedFlag": true}, // closed: ignored
	}
	got := computeWorkload(tickets, "", time.Now())
	if len(got) != 2 || got[0].Owner != "jdoe" || got[0].OpenTickets != 2 {
		t.Fatalf("workload = %+v; want jdoe=2 first", got)
	}
}

func TestComputeBoard(t *testing.T) {
	now := time.Now()
	tickets := []map[string]any{
		{"id": num("1"), "summary": "open-b2", "board": ref("2", "Help Desk"), "closedFlag": false},
		{"id": num("2"), "summary": "closed-b2", "board": ref("2", "Help Desk"), "closedFlag": true},
		{"id": num("3"), "summary": "open-b3", "board": ref("3", "Onboarding"), "closedFlag": false},
	}
	got := computeBoard(tickets, "2", false, now)
	if len(got) != 1 || got[0].ID != 1 {
		t.Fatalf("board 2 = %+v; want only open ticket 1", got)
	}
	if g := computeBoard(tickets, "Help Desk", false, now); len(g) != 1 {
		t.Fatalf("board by name = %+v; want 1", g)
	}
}

func TestComputeAgreementBurn(t *testing.T) {
	agreements := []map[string]any{
		{"id": num("10"), "name": "Gold", "company": refIdent("7", "Acme", "Acme Corp"),
			"applicationUnits": "Hours", "applicationLimit": num("40")},
	}
	times := []map[string]any{
		{"agreement": ref("10", "Gold"), "hoursBilled": num("30")},
		{"agreement": ref("10", "Gold"), "hoursBilled": num("20")},
	}
	got := computeAgreementBurn(agreements, times, "", time.Time{})
	if len(got) != 1 {
		t.Fatalf("burn rows = %d; want 1", len(got))
	}
	r := got[0]
	if r.UsedHours != 50 || r.Limit != 40 || r.Pct != 125 || !r.Over {
		t.Fatalf("burn = %+v; want used=50 limit=40 pct=125 over=true", r)
	}
}

// TestCwLoadIntegration seeds a real store via UpsertBatch and reads it back
// through cwLoad → compute, exercising the full data pipeline including the
// store's UseNumber() decoder (where ids arrive as json.Number, not float64).
func TestCwLoadIntegration(t *testing.T) {
	ctx := context.Background()
	db, err := store.OpenWithContext(ctx, filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	ticket1 := json.RawMessage(`{"id":101,"summary":"VPN down","closedFlag":false,"board":{"id":2,"name":"Help Desk"},"owner":{"id":11,"identifier":"jdoe","name":"John"},"company":{"id":7,"identifier":"Acme","name":"Acme Corp"},"_info":{"lastUpdated":"2024-01-10T00:00:00Z"}}`)
	ticket2 := json.RawMessage(`{"id":102,"summary":"printer","closedFlag":false,"board":{"id":2,"name":"Help Desk"},"owner":{"id":12,"identifier":"asmith","name":"Amy"},"company":{"id":7,"identifier":"Acme","name":"Acme Corp"},"_info":{"lastUpdated":"2024-01-11T00:00:00Z"}}`)
	te := json.RawMessage(`{"id":555,"chargeToType":"ServiceTicket","chargeToId":101,"hoursBilled":1.5,"member":{"id":11,"identifier":"jdoe"}}`)

	if stored, _, err := db.UpsertBatch(cwTickets, []json.RawMessage{ticket1, ticket2}); err != nil || stored != 2 {
		t.Fatalf("upsert tickets: stored=%d err=%v", stored, err)
	}
	if stored, _, err := db.UpsertBatch(cwTimeEntries, []json.RawMessage{te}); err != nil || stored != 1 {
		t.Fatalf("upsert time: stored=%d err=%v", stored, err)
	}

	tickets, err := cwLoad(ctx, db, cwTickets)
	if err != nil || len(tickets) != 2 {
		t.Fatalf("cwLoad tickets = %d,%v; want 2", len(tickets), err)
	}
	// The decode path must yield real ids, not 0 (the UseNumber trap).
	if cwTopID(tickets[0]) == 0 && cwTopID(tickets[1]) == 0 {
		t.Fatalf("cwTopID returned 0 for both tickets — json.Number not decoded")
	}
	times, _ := cwLoad(ctx, db, cwTimeEntries)

	unb := computeUnbilled(tickets, times, time.Time{}, "", 0, time.Now())
	if len(unb) != 1 || unb[0].ID != 102 {
		t.Fatalf("integration unbilled = %+v; want only ticket 102 (101 has 1.5h logged)", unb)
	}
}

func TestComputeAccount(t *testing.T) {
	companies := []map[string]any{
		{"id": num("7"), "identifier": "Acme", "name": "Acme Corp", "status": ref("1", "Active")},
		{"id": num("8"), "identifier": "Other", "name": "Other Inc"},
	}
	withCompany := func(id string) map[string]any { return map[string]any{"company": ref(id, "")} }
	contacts := []map[string]any{
		{"company": ref("7", ""), "firstName": "Jane", "lastName": "Doe"},
		{"company": ref("7", ""), "firstName": "Bob", "lastName": "Roe"},
		{"company": ref("8", ""), "firstName": "X", "lastName": "Y"},
	}
	agreements := []map[string]any{
		{"name": "Gold", "company": ref("7", ""), "cancelledFlag": false},
		{"name": "Cancelled", "company": ref("7", ""), "cancelledFlag": true}, // excluded
	}
	configs := []map[string]any{withCompany("7"), withCompany("7"), withCompany("7"), withCompany("8")}
	tickets := []map[string]any{
		{"id": num("1"), "company": ref("7", ""), "closedFlag": false, "dateEntered": "2024-02-01T00:00:00Z"},
		{"id": num("2"), "company": ref("7", ""), "closedFlag": true, "dateEntered": "2024-01-01T00:00:00Z"},
	}

	card := computeAccount(companies, contacts, agreements, configs, tickets, "Acme")
	if !card.Found || card.ID != 7 {
		t.Fatalf("account not found / wrong id: %+v", card)
	}
	if card.Contacts != 2 || card.Configurations != 3 || card.OpenTickets != 1 || len(card.Agreements) != 1 {
		t.Fatalf("account card = %+v; want contacts=2 configs=3 open=1 agreements=1", card)
	}
	if card.Status != "Active" || card.LastActivity == "" {
		t.Fatalf("account card status/activity = %q/%q", card.Status, card.LastActivity)
	}
	// numeric id lookup
	if c2 := computeAccount(companies, contacts, agreements, configs, tickets, "8"); !c2.Found || c2.ID != 8 {
		t.Fatalf("account by numeric id = %+v; want id 8", c2)
	}
	// no match
	if c3 := computeAccount(companies, contacts, agreements, configs, tickets, "Nope"); c3.Found {
		t.Fatalf("account Nope = %+v; want not found", c3)
	}
}

// --- Phase 4.95 review regression tests (reprint 20260605-220229) ---

func TestFormatScalarAdversarial(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"lone-quote", `"`, `"\""`},
		{"lone-bracket", `[`, `"["`},
		{"nan-token", "NaN", `"NaN"`},
		{"inf-token", "Inf", `"Inf"`},
		{"neg-inf", "-Inf", `"-Inf"`},
		{"infinity", "Infinity", `"Infinity"`},
		{"hex-float", "0x1p4", `"0x1p4"`},
		{"plain-int", "42", "42"},
		{"plain-float", "4.5", "4.5"},
		{"exponent", "1e3", "1e3"},
		{"null-literal", "null", "null"},
		{"embedded-quote", `a"b`, `"a\"b"`},
		{"trailing-backslash", `foo\`, `"foo\\"`},
		{"backslash-quote", `a\"b`, `"a\\\"b"`},
		{"prequoted-passthrough", `"already"`, `"already"`},
		{"prebracketed-passthrough", `[2024-01-01T00:00:00Z]`, `[2024-01-01T00:00:00Z]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatScalar(tc.in); got != tc.want {
				t.Fatalf("formatScalar(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestBuildConditionsNotIn(t *testing.T) {
	got, err := buildConditions([]condClause{{"status/id", "not in", "1,2"}}, "and")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := `status/id not in (1,2)`; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestExplainConditionsMixedOrThenAnd(t *testing.T) {
	clauses, join := explainConditions(`a = "x" OR b = "y" AND c = "z"`)
	if join != "MIXED" {
		t.Fatalf("OR-then-AND join = %q, want MIXED", join)
	}
	if len(clauses) != 3 {
		t.Fatalf("clause count = %d, want 3", len(clauses))
	}
}

func TestAgreementHoursSinceWindow(t *testing.T) {
	since := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	entries := []map[string]any{
		{"agreement": ref("10", "Block"), "hoursBilled": num("2"), "dateEntered": "2026-05-20T00:00:00Z"}, // before window — excluded
		{"agreement": ref("10", "Block"), "hoursBilled": num("3"), "dateEntered": "2026-06-03T00:00:00Z"}, // in window — counted
		{"agreement": ref("10", "Block"), "hoursBilled": num("5")},                                        // no dateEntered — kept (inclusion bias by design)
	}
	idx := agreementHours(entries, since)
	if got := idx[10]; got != 8 {
		t.Fatalf("windowed hours = %v, want 8 (3 in-window + 5 undated)", got)
	}
	all := agreementHours(entries, time.Time{})
	if got := all[10]; got != 10 {
		t.Fatalf("unwindowed hours = %v, want 10", got)
	}
}

func TestCwLoggedHoursZeroBilledFallback(t *testing.T) {
	te := map[string]any{"hoursBilled": num("0"), "actualHours": num("1.5")}
	if got := cwLoggedHours(te); got != 1.5 {
		t.Fatalf("zero-billed fallback = %v, want 1.5", got)
	}
	te2 := map[string]any{"hoursBilled": num("2"), "actualHours": num("9")}
	if got := cwLoggedHours(te2); got != 2 {
		t.Fatalf("billed-wins = %v, want 2", got)
	}
}
