// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): the failing inputs from Codex round 1 on the
// zammad 4.34.0 reprint (2026-10-24), driven through the real command tree,
// plus the other direction of each fix (a healthy input still answers).

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"zammad-pp-cli/internal/store"
)

// runR1 executes the command tree with args and returns stdout and the error.
func runR1(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := RootCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

func r1Store(t *testing.T) (string, *store.Store) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "data.db")
	db, err := store.OpenWithContext(context.Background(), path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	return path, db
}

func r1Exec(t *testing.T, db *store.Store, statements ...string) {
	t.Helper()
	for _, s := range statements {
		if _, err := db.DB().Exec(s); err != nil {
			t.Fatalf("exec %q: %v", s, err)
		}
	}
}

func r1UpsertTicket(t *testing.T, db *store.Store, obj map[string]any) {
	t.Helper()
	b, _ := json.Marshal(obj)
	if err := db.UpsertTickets(b); err != nil {
		t.Fatalf("upsert ticket: %v", err)
	}
}

func r1Complete(t *testing.T, db *store.Store, resources ...string) {
	t.Helper()
	for _, r := range resources {
		if err := db.SaveSyncState(r, "", 1); err != nil {
			t.Fatalf("save sync state %s: %v", r, err)
		}
	}
}

func r1Ago(d time.Duration) string {
	return time.Now().Add(-d).UTC().Format("2006-01-02T15:04:05.000Z")
}

// Codex r1 [P1] overdue.go: an unfinished sync read as "nothing overdue".
func TestR1LocalAnalyticsRefuseAnUnfinishedSync(t *testing.T) {
	commands := [][]string{
		{"overdue"}, {"agent-load"}, {"agent-trend"}, {"customer-health"},
		{"escalate"}, {"churn-risk"}, {"feedback-scan"}, {"stale"}, {"orphans"}, {"load"},
	}
	cases := map[string]func(*testing.T, *store.Store){
		"incomplete, empty": func(t *testing.T, db *store.Store) {
			r1Exec(t, db, `INSERT INTO sync_state (resource_type, last_synced_at, last_attempt_complete) VALUES ('tickets', '2026-10-24T16:00:00Z', 0)`)
		},
		"incomplete, partial data": func(t *testing.T, db *store.Store) {
			r1UpsertTicket(t, db, map[string]any{"id": 1, "state_id": 2, "owner_id": 3, "created_at": r1Ago(30 * 24 * time.Hour), "updated_at": r1Ago(30 * 24 * time.Hour)})
			if err := db.SaveSyncProgress("tickets", "2", 1); err != nil {
				t.Fatal(err)
			}
		},
		"never synced": func(t *testing.T, db *store.Store) {},
		"rows with no checkpoint": func(t *testing.T, db *store.Store) {
			r1UpsertTicket(t, db, map[string]any{"id": 42, "state_id": 2, "owner_id": 3, "created_at": r1Ago(30 * 24 * time.Hour), "updated_at": r1Ago(24 * time.Hour)})
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			path, db := r1Store(t)
			setup(t, db)
			db.Close()
			for _, c := range commands {
				out, _, err := runR1(t, append(c, "--db", path, "--json")...)
				if err == nil {
					t.Errorf("%s answered from an unfinished sync without an error: %s", c[0], out)
				}
			}
		})
	}
}

func TestR1LocalAnalyticsAnswerFromACompleteSync(t *testing.T) {
	path, db := r1Store(t)
	r1UpsertTicket(t, db, map[string]any{"id": 1, "title": "Printer", "state_id": 2, "owner_id": 3, "priority_id": 2, "group_id": 1, "organization_id": 7, "created_at": r1Ago(30 * 24 * time.Hour), "updated_at": r1Ago(30 * 24 * time.Hour)})
	art, _ := json.Marshal(map[string]any{"id": 11, "ticket_id": 1, "sender_id": 2, "internal": false, "body": "this is unacceptable", "created_at": r1Ago(29 * 24 * time.Hour)})
	if err := db.UpsertArticles(art); err != nil {
		t.Fatal(err)
	}
	r1Complete(t, db, "tickets", "articles")
	db.Close()
	for _, c := range []string{"overdue", "agent-load", "agent-trend", "customer-health", "escalate", "churn-risk", "feedback-scan", "stale", "orphans", "load"} {
		if out, _, err := runR1(t, c, "--db", path, "--json"); err != nil {
			t.Errorf("%s refused a complete sync: %v %s", c, err, out)
		}
	}
	// An unfinished ARTICLES sync is refused by the article-based commands.
	db2, err := store.OpenWithContext(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db2.SaveSyncProgress("articles", "", 1); err != nil {
		t.Fatal(err)
	}
	db2.Close()
	for _, c := range []string{"escalate", "churn-risk", "feedback-scan"} {
		if out, _, err := runR1(t, c, "--db", path, "--json"); err == nil {
			t.Errorf("%s answered from an unfinished articles sync: %s", c, out)
		}
	}
}

// Codex r1 [P1] churn_risk.go: a ticket 7.5 days old was not overdue at --days 7.
func TestR1ChurnRiskOverdueUsesTheRealAge(t *testing.T) {
	_, db := r1Store(t)
	defer db.Close()
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	for i := 1; i <= 4; i++ {
		r1UpsertTicket(t, db, map[string]any{"id": i, "state_id": 2, "organization_id": 7, "created_at": "2026-10-17T00:00:00Z"})
	}
	art, _ := json.Marshal(map[string]any{"id": 99, "ticket_id": 1, "sender_id": 2, "internal": false, "body": "thanks", "created_at": "2026-10-17T00:00:00Z"})
	if err := db.UpsertArticles(art); err != nil {
		t.Fatal(err)
	}
	rows, err := buildChurnRiskRows(db, 7, "support_tier", false, false, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Overdue != 4 || rows[0].Score != 12 || rows[0].Risk != "high" {
		t.Fatalf("churn-risk = %+v, want overdue=4 score=12 risk=high", rows)
	}
}

// Codex r1 [P1] pm_stale.go: text timestamps compared across UTC offsets.
func TestR1StaleComparesInstantsNotText(t *testing.T) {
	loc := time.FixedZone("UTC-12", -12*3600)
	saved := time.Local
	time.Local = loc
	defer func() { time.Local = saved }()
	path, db := r1Store(t)
	r1UpsertTicket(t, db, map[string]any{"id": 42, "title": "Quiet ticket", "state_id": 2, "owner_id": 3, "created_at": r1Ago(9 * 24 * time.Hour), "updated_at": r1Ago(7*24*time.Hour + 3*time.Hour)})
	r1UpsertTicket(t, db, map[string]any{"id": 43, "title": "Fresh ticket", "state_id": 2, "owner_id": 3, "created_at": r1Ago(9 * 24 * time.Hour), "updated_at": r1Ago(6 * 24 * time.Hour)})
	r1Complete(t, db, "tickets")
	db.Close()
	out, _, err := runR1(t, "stale", "--days", "7", "--db", path, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var res struct {
		TotalCount int `json:"total_count"`
		Items      []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if res.TotalCount != 1 || len(res.Items) != 1 || res.Items[0].ID != "42" {
		t.Fatalf("stale = %s, want only ticket 42", out)
	}
}

// Codex r1 [P1] pm_load.go / pm_orphans.go: generic fields over every resource.
func TestR1LoadAndOrphansReadTheTicketModel(t *testing.T) {
	path, db := r1Store(t)
	user, _ := json.Marshal(map[string]any{"id": 9, "firstname": "Pat", "lastname": "Doe", "login": "pat"})
	if err := db.UpsertUsers(user); err != nil {
		t.Fatal(err)
	}
	r1UpsertTicket(t, db, map[string]any{"id": 42, "title": "Assigned", "state_id": 2, "owner_id": 9, "priority_id": 2, "group_id": 1, "created_at": r1Ago(time.Hour), "updated_at": r1Ago(time.Hour)})
	r1Complete(t, db, "tickets", "users")
	db.Close()

	out, _, err := runR1(t, "load", "--db", path, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var load struct {
		TotalCount   int `json:"total_count"`
		Distribution []struct {
			Assignee string `json:"assignee"`
			Count    int    `json:"count"`
		} `json:"distribution"`
	}
	if err := json.Unmarshal([]byte(out), &load); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if load.TotalCount != 1 || len(load.Distribution) != 1 || load.Distribution[0].Assignee != "Pat Doe" || load.Distribution[0].Count != 1 {
		t.Fatalf("load = %s, want Pat Doe owning the one ticket", out)
	}

	out, _, err = runR1(t, "orphans", "--db", path, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var orphans struct {
		TotalCount int `json:"total_count"`
	}
	if err := json.Unmarshal([]byte(out), &orphans); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if orphans.TotalCount != 0 {
		t.Fatalf("orphans = %s, want none for a fully assigned ticket", out)
	}
}

func TestR1OrphansStillFindsAnUnownedTicket(t *testing.T) {
	path, db := r1Store(t)
	r1UpsertTicket(t, db, map[string]any{"id": 50, "title": "Nobody", "state_id": 1, "owner_id": 1, "priority_id": 2, "group_id": 1, "created_at": r1Ago(time.Hour), "updated_at": r1Ago(time.Hour)})
	r1UpsertTicket(t, db, map[string]any{"id": 51, "title": "Closed unowned", "state_id": 4, "owner_id": 1, "priority_id": 2, "group_id": 1, "created_at": r1Ago(time.Hour), "updated_at": r1Ago(time.Hour)})
	r1Complete(t, db, "tickets")
	db.Close()
	out, _, err := runR1(t, "orphans", "--db", path, "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"50"`) || strings.Contains(out, `"51"`) {
		t.Fatalf("orphans = %s, want the active unowned ticket 50 only", out)
	}
}

// Codex r1 [P1] articles_sync.go: an article deleted upstream lingered.
func TestR1ArticlesSyncDropsArticlesTheServerNoLongerHas(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/ticket_articles/by_ticket/42":
			_, _ = w.Write([]byte(`[]`))
		case "/ticket_articles/by_ticket/43":
			_, _ = w.Write([]byte(`[{"id":31,"ticket_id":43,"sender_id":2,"internal":false,"body":"still here"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	t.Setenv("ZAMMAD_BASE_URL", srv.URL)
	path, db := r1Store(t)
	r1UpsertTicket(t, db, map[string]any{"id": 42, "state_id": 1, "priority_id": 2, "created_at": r1Ago(time.Hour)})
	r1UpsertTicket(t, db, map[string]any{"id": 43, "state_id": 1, "priority_id": 2, "created_at": r1Ago(time.Hour)})
	for _, a := range []map[string]any{
		{"id": 21, "ticket_id": 42, "sender_id": 2, "internal": false, "body": "angry"},
		{"id": 30, "ticket_id": 43, "sender_id": 2, "internal": false, "body": "gone too"},
	} {
		b, _ := json.Marshal(a)
		if err := db.UpsertArticles(b); err != nil {
			t.Fatal(err)
		}
	}
	r1Complete(t, db, "tickets")
	db.Close()
	if out, _, err := runR1(t, "articles", "sync", "--db", path, "--json", "--no-cache"); err != nil {
		t.Fatalf("articles sync: %v %s", err, out)
	}
	db, err := store.OpenWithContext(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var ids []string
	rows, err := db.DB().Query(`SELECT id FROM articles ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id string
		_ = rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	var generic int
	_ = db.DB().QueryRow(`SELECT COUNT(*) FROM resources WHERE resource_type = 'articles'`).Scan(&generic)
	if len(ids) != 1 || ids[0] != "31" || generic != 1 {
		t.Fatalf("articles after sync = %v (generic rows %d), want only 31", ids, generic)
	}
}

// Codex r1 [P2] articles_sync.go: --dry-run created and migrated the store.
func TestR1ArticlesSyncDryRunLeavesTheStoreAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preview.db")
	_, _, _ = runR1(t, "articles", "sync", "--db", path, "--dry-run", "--no-learn")
	if _, err := os.Stat(path); err == nil {
		t.Fatal("articles sync --dry-run created the store")
	}
}

// A partial articles sync exits non-zero and leaves the checkpoint unfinished,
// so the article commands refuse the store until a clean run.
func TestR1PartialArticlesSyncIsAFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ticket_articles/by_ticket/42" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"id":21,"ticket_id":42,"sender_id":2,"internal":false,"body":"fine"}]`))
			return
		}
		http.Error(w, `{"error":"boom"}`, http.StatusInternalServerError)
	}))
	defer srv.Close()
	t.Setenv("ZAMMAD_BASE_URL", srv.URL)
	path, db := r1Store(t)
	r1UpsertTicket(t, db, map[string]any{"id": 42, "state_id": 1, "created_at": r1Ago(time.Hour)})
	r1UpsertTicket(t, db, map[string]any{"id": 43, "state_id": 1, "created_at": r1Ago(time.Hour)})
	r1Complete(t, db, "tickets")
	db.Close()
	out, _, err := runR1(t, "articles", "sync", "--db", path, "--json", "--no-cache")
	if err == nil || !strings.Contains(out, `"fetch_failures"`) {
		t.Fatalf("a partial articles sync exited cleanly: %v %s", err, out)
	}
	if out, _, err := runR1(t, "escalate", "--db", path, "--json"); err == nil {
		t.Fatalf("escalate answered after a partial articles sync: %s", out)
	}
}

func TestR1FallbackStateCatalogMatchesZammadSeeds(t *testing.T) {
	catalog := fallbackZammadStateCatalog()
	if catalog["6"].Kind != zammadStateClosed || catalog["7"].Kind != zammadStateResolved {
		t.Fatalf("fallback catalog: 6=%+v 7=%+v, want removed (closed) and pending close", catalog["6"], catalog["7"])
	}
	if classifyZammadStateName("removed") != zammadStateClosed {
		t.Fatal("a state named removed was classified active")
	}
}

// Codex r2 [P1] articles_sync.go: --limit covered part of the tickets but the
// checkpoint said complete, so escalate missed the rest silently.
func TestR3LimitedArticlesSyncIsNotComplete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/ticket_articles/by_ticket/41":
			_, _ = w.Write([]byte(`[{"id":411,"ticket_id":41,"sender_id":2,"internal":false,"body":"angry"}]`))
		case "/ticket_articles/by_ticket/42":
			_, _ = w.Write([]byte(`[{"id":421,"ticket_id":42,"sender_id":2,"internal":false,"body":"hello"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	t.Setenv("ZAMMAD_BASE_URL", srv.URL)
	path, db := r1Store(t)
	r1UpsertTicket(t, db, map[string]any{"id": 41, "state_id": 1, "priority_id": 2, "created_at": r1Ago(time.Hour)})
	r1UpsertTicket(t, db, map[string]any{"id": 42, "state_id": 1, "priority_id": 2, "created_at": r1Ago(time.Hour)})
	r1Complete(t, db, "tickets")
	db.Close()
	if out, _, err := runR1(t, "articles", "sync", "--limit", "1", "--db", path, "--json", "--no-cache"); err != nil {
		t.Fatalf("articles sync --limit 1: %v %s", err, out)
	}
	if out, _, err := runR1(t, "escalate", "--db", path, "--json"); err == nil {
		t.Fatalf("escalate answered after an articles sync that covered 1 of 2 tickets: %s", out)
	}
	if out, _, err := runR1(t, "articles", "sync", "--db", path, "--json", "--no-cache"); err != nil {
		t.Fatalf("full articles sync: %v %s", err, out)
	}
	if out, _, err := runR1(t, "escalate", "--db", path, "--json"); err != nil {
		t.Fatalf("escalate refused after a full articles sync: %v %s", err, out)
	}
}

// Codex r2 (P2 under the ratified scope; one-line fix): a bare null is not a
// complete article list, so nothing is removed for that ticket.
func TestR3NullArticleListRemovesNothing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`null`))
	}))
	defer srv.Close()
	t.Setenv("ZAMMAD_BASE_URL", srv.URL)
	path, db := r1Store(t)
	r1UpsertTicket(t, db, map[string]any{"id": 42, "state_id": 1, "created_at": r1Ago(time.Hour)})
	b, _ := json.Marshal(map[string]any{"id": 21, "ticket_id": 42, "sender_id": 2, "internal": false, "body": "angry"})
	if err := db.UpsertArticles(b); err != nil {
		t.Fatal(err)
	}
	r1Complete(t, db, "tickets")
	db.Close()
	_, _, _ = runR1(t, "articles", "sync", "--db", path, "--json", "--no-cache")
	db, err := store.OpenWithContext(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	_ = db.DB().QueryRow(`SELECT COUNT(*) FROM articles`).Scan(&n)
	if n != 1 {
		t.Fatalf("a null article list removed stored articles: %d left", n)
	}
}
