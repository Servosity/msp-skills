package insights

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"knowbe4-pp-cli/internal/store"
)

// seed opens a migrated store in a temp dir and inserts a known fixture:
//   - alice: high risk, clicked 2 PSTs, no passed training            (repeat clicker, untrained, leaderboard top)
//   - bob:   clicked 1 PST, has a Passed enrollment                   (trained clicker; excluded from untrained)
//   - carol: no enrollment, no phishing result                        (coverage gap, training+phishing)
//   - group Finance with two risk snapshots (worse over time)
func seed(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.OpenWithContext(context.Background(), filepath.Join(dir, "kb4.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	db := st.DB()
	if err := EnsureSchema(context.Background(), db); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}
	exec := func(q string, args ...any) {
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("seed exec %q: %v", q, err)
		}
	}

	// users (email is a typed column)
	exec(`INSERT INTO "users"(id,data,email,first_name,last_name,current_risk_score,phish_prone_percentage,joined_on,status)
		VALUES ('1','{}','alice@acme.com','Alice','A',88.5,40,'2026-05-20T00:00:00.000Z','active')`)
	exec(`INSERT INTO "users"(id,data,email,first_name,last_name,current_risk_score,phish_prone_percentage,joined_on,status)
		VALUES ('2','{}','bob@acme.com','Bob','B',30,10,'2020-01-01T00:00:00.000Z','active')`)
	exec(`INSERT INTO "users"(id,data,email,first_name,last_name,current_risk_score,phish_prone_percentage,joined_on,status)
		VALUES ('3','{}','carol@acme.com','Carol','C',55,25,'2026-05-25T00:00:00.000Z','active')`)

	// account
	exec(`INSERT INTO "account"(id,data,name,current_risk_score) VALUES ('acct','{}','Acme',45)`)

	// groups
	exec(`INSERT INTO "groups"(id,data,name,member_count,current_risk_score,status) VALUES ('10','{}','Finance',50,60,'active')`)

	// phishing_tests: two PSTs for Finance, phish-prone improving 30 -> 10
	exec(`INSERT INTO "phishing_tests"(id,data,pst_id,name,started_at,phish_prone_percentage)
		VALUES ('p1','{"groups":[{"group_id":10,"name":"Finance"}]}',101,'Q1 Test','2026-01-01T00:00:00.000Z',30)`)
	exec(`INSERT INTO "phishing_tests"(id,data,pst_id,name,started_at,phish_prone_percentage)
		VALUES ('p2','{"groups":[{"group_id":10,"name":"Finance"}]}',102,'Q2 Test','2026-04-01T00:00:00.000Z',10)`)

	// training_enrollments: bob passed; alice/carol absent
	exec(`INSERT INTO "training_enrollments"(id,data,enrollment_id,status) VALUES ('e1','{"user":{"id":2,"email":"bob@acme.com"}}',9001,'Passed')`)
	exec(`INSERT INTO "training_enrollments"(id,data,enrollment_id,status) VALUES ('e2','{"user":{"id":1,"email":"alice@acme.com"}}',9002,'Past Due')`)

	// pst_recipients: alice clicked p1 & p2; bob clicked p1 only; bob reported p2
	exec(`INSERT INTO "pst_recipients"(pst_id,recipient_id,user_id,user_email,clicked_at,reported_at,delivered_at)
		VALUES (101,1,1,'alice@acme.com','2026-01-02T00:00:00.000Z','','2026-01-01T00:00:00.000Z')`)
	exec(`INSERT INTO "pst_recipients"(pst_id,recipient_id,user_id,user_email,clicked_at,reported_at,delivered_at)
		VALUES (102,2,1,'alice@acme.com','2026-04-02T00:00:00.000Z','','2026-04-01T00:00:00.000Z')`)
	exec(`INSERT INTO "pst_recipients"(pst_id,recipient_id,user_id,user_email,clicked_at,reported_at,delivered_at)
		VALUES (101,3,2,'bob@acme.com','2026-01-02T00:00:00.000Z','','2026-01-01T00:00:00.000Z')`)
	exec(`INSERT INTO "pst_recipients"(pst_id,recipient_id,user_id,user_email,clicked_at,reported_at,delivered_at)
		VALUES (102,4,2,'bob@acme.com','','2026-04-02T00:00:00.000Z','2026-04-01T00:00:00.000Z')`)

	// risk_snapshots: Finance group worsened 40 -> 60 over 90 days; account 30 -> 45
	old := time.Now().UTC().Add(-120 * 24 * time.Hour).Format("2006-01-02T15:04:05.000Z")
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	exec(`INSERT INTO "risk_snapshots"(entity_type,entity_id,entity_name,risk_score,member_count,snapshot_at) VALUES ('group','10','Finance',40,50,?)`, old)
	exec(`INSERT INTO "risk_snapshots"(entity_type,entity_id,entity_name,risk_score,member_count,snapshot_at) VALUES ('group','10','Finance',60,50,?)`, now)
	exec(`INSERT INTO "risk_snapshots"(entity_type,entity_id,entity_name,risk_score,snapshot_at) VALUES ('account','acct','Acme',30,?)`, old)
	exec(`INSERT INTO "risk_snapshots"(entity_type,entity_id,entity_name,risk_score,snapshot_at) VALUES ('account','acct','Acme',45,?)`, now)

	return db, func() { st.Close() }
}

func TestRepeatClickers(t *testing.T) {
	db, done := seed(t)
	defer done()
	got, err := RepeatClickers(context.Background(), db, 2, 10, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 repeat clicker (alice), got %d: %+v", len(got), got)
	}
	if got[0].Email != "alice@acme.com" || got[0].ClickedPSTs != 2 {
		t.Fatalf("want alice with 2 clicked PSTs, got %+v", got[0])
	}
}

func TestUntrainedClickers(t *testing.T) {
	db, done := seed(t)
	defer done()
	got, err := UntrainedClickers(context.Background(), db, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	// alice clicked and has no Passed training; bob clicked but Passed -> excluded.
	if len(got) != 1 || got[0].Email != "alice@acme.com" {
		t.Fatalf("want only alice as untrained clicker, got %+v", got)
	}
}

func TestCoverageGapsTraining(t *testing.T) {
	db, done := seed(t)
	defer done()
	got, err := CoverageGaps(context.Background(), db, "training", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	// carol has no enrollment; alice & bob do.
	found := false
	for _, r := range got {
		if r.Email == "carol@acme.com" {
			found = true
		}
		if r.Email == "bob@acme.com" {
			t.Fatalf("bob has a passed enrollment, should not be a training gap")
		}
	}
	if !found {
		t.Fatalf("expected carol in training coverage gaps, got %+v", got)
	}
}

func TestRiskLeaderboard(t *testing.T) {
	db, done := seed(t)
	defer done()
	got, err := RiskLeaderboard(context.Background(), db, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Email != "alice@acme.com" {
		t.Fatalf("want alice top of leaderboard, got %+v", got)
	}
	if got[0].ClickedPSTs != 2 {
		t.Fatalf("want alice clicked 2 PSTs, got %d", got[0].ClickedPSTs)
	}
	if got[0].OpenTrainings != 1 {
		t.Fatalf("want alice 1 open (Past Due) training, got %d", got[0].OpenTrainings)
	}
}

func TestRiskDrift(t *testing.T) {
	db, done := seed(t)
	defer done()
	got, err := RiskDrift(context.Background(), db, "90d", "group", true, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].EntityID != "10" {
		t.Fatalf("want Finance group drift, got %+v", got)
	}
	if got[0].Delta != 20 {
		t.Fatalf("want delta 20 (40->60), got %v", got[0].Delta)
	}
}

func TestPhishProneTrend(t *testing.T) {
	db, done := seed(t)
	defer done()
	got, err := PhishProneTrendQuery(context.Background(), db, "Finance", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Points) != 2 {
		t.Fatalf("want 2 Finance PSTs, got %d", len(got.Points))
	}
	if got.FirstPct != 30 || got.LastPct != 10 || !got.Improving {
		t.Fatalf("want improving 30->10, got %+v", got)
	}
}

func TestGroupRiskContribution(t *testing.T) {
	db, done := seed(t)
	defer done()
	got, err := GroupRiskContribution(context.Background(), db, "90d", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].GroupID != "10" {
		t.Fatalf("want Finance contribution, got %+v", got)
	}
	if got[0].Contribution != 1000 { // delta 20 * 50 members
		t.Fatalf("want weighted contribution 1000, got %v", got[0].Contribution)
	}
}

func TestQBR(t *testing.T) {
	db, done := seed(t)
	defer done()
	// 1y window captures both of alice's clicks (Jan + Apr); a 90d window
	// would legitimately see only one and report 0 repeat clickers.
	rep, err := QBR(context.Background(), db, "1y")
	if err != nil {
		t.Fatal(err)
	}
	if rep.TrainingCompletion <= 0 {
		t.Fatalf("want >0 training completion (bob passed of 2), got %v", rep.TrainingCompletion)
	}
	if rep.RepeatClickerCount != 1 {
		t.Fatalf("want 1 repeat clicker in QBR, got %d", rep.RepeatClickerCount)
	}
	if len(rep.TopRiskUsers) == 0 || rep.TopRiskUsers[0].Email != "alice@acme.com" {
		t.Fatalf("want alice as top risk user, got %+v", rep.TopRiskUsers)
	}
}

func TestEmptyStoreReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	st, err := store.OpenWithContext(context.Background(), filepath.Join(dir, "empty.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	db := st.DB()
	if rc, err := RepeatClickers(context.Background(), db, 2, 10, ""); err != nil || len(rc) != 0 {
		t.Fatalf("empty store should yield no repeat clickers, err=%v n=%d", err, len(rc))
	}
	if lb, err := RiskLeaderboard(context.Background(), db, 10); err != nil || len(lb) != 0 {
		t.Fatalf("empty store should yield empty leaderboard, err=%v n=%d", err, len(lb))
	}
	if _, err := QBR(context.Background(), db, "90d"); err != nil {
		t.Fatalf("QBR on empty store should not error: %v", err)
	}
}

func TestParseWindow(t *testing.T) {
	cases := map[string]time.Duration{
		"90d": 90 * 24 * time.Hour,
		"12w": 12 * 7 * 24 * time.Hour,
		"6mo": 6 * 30 * 24 * time.Hour,
		"1y":  365 * 24 * time.Hour,
		"24h": 24 * time.Hour,
		"":    0,
	}
	for in, want := range cases {
		got, err := ParseWindow(in)
		if err != nil {
			t.Fatalf("ParseWindow(%q) error: %v", in, err)
		}
		if got != want {
			t.Fatalf("ParseWindow(%q) = %v, want %v", in, got, want)
		}
	}
	if _, err := ParseWindow("nonsense"); err == nil {
		t.Fatalf("ParseWindow(nonsense) should error")
	}
}
