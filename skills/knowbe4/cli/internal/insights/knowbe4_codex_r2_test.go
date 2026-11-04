// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package insights

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// Codex review round 2 (knowbe4 4.34.2 reprint), class
// missing-input-read-as-zero: a collection is complete only by its own sync
// completion record (CollectionSynced), never by a row in its table. The
// failing inputs: enrollment rows without a finished enrollments sync (one
// cached detail row), a recipient walk older than the phishing tests' last
// finished sync, and a recipient walk with a failed test.
func TestR2CollectionSyncedNeedsItsOwnFinishedWalk(t *testing.T) {
	ctx := context.Background()
	db, done := seed(t)
	defer done()

	// Rows in training_enrollments, but their sync never finished.
	if _, err := db.Exec(`UPDATE sync_state SET last_attempt_complete = 0 WHERE resource_type = 'training-enrollments'`); err != nil {
		t.Fatal(err)
	}
	if got, err := UntrainedClickers(ctx, db, 0, ""); !errors.Is(err, ErrInputNotSynced) {
		t.Errorf("untrained-clickers over enrollment rows no sync finished: %+v, err %v; want ErrInputNotSynced", got, err)
	}
	if got, err := CoverageGaps(ctx, db, "training", "", 0); !errors.Is(err, ErrInputNotSynced) {
		t.Errorf("coverage-gaps --type training over unsynced enrollments: %+v, err %v; want ErrInputNotSynced", got, err)
	}
	rep, err := QBR(ctx, db, "1y")
	if err != nil {
		t.Fatal(err)
	}
	if rep.TrainingCompletion != nil || rep.UntrainedClickers != nil {
		t.Errorf("qbr over unsynced enrollments: training_completion_pct %v, untrained_clicker_count %v; want null", rep.TrainingCompletion, rep.UntrainedClickers)
	}
	fr, err := Freshness(ctx, db, 0, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range fr.Commands {
		if c.Command == "untrained-clickers" && c.Ready {
			t.Errorf("freshness: untrained-clickers ready over unsynced enrollments: %+v", c)
		}
	}
	markSynced(t, db, "training-enrollments")

	// The phishing tests finished a sync after the recipient walk: a test the
	// walk never saw may exist, so who never clicked is unknown.
	later := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	if _, err := db.Exec(`UPDATE sync_state SET last_synced_at = ? WHERE resource_type = 'phishing-tests'`, later); err != nil {
		t.Fatal(err)
	}
	if got, err := RepeatClickers(ctx, db, 2, 0, ""); !errors.Is(err, ErrInputNotSynced) {
		t.Errorf("repeat-clickers over a recipient walk older than the tests: %+v, err %v; want ErrInputNotSynced", got, err)
	}
	if got, err := RiskLeaderboard(ctx, db, 0); err != nil || len(got) == 0 || got[0].ClickedPSTs != nil {
		t.Errorf("risk-leaderboard over a stale recipient walk: err %v, want clicked_psts null: %+v", err, got)
	}
}

// pstGetter answers each phishing test's recipients; tests in fail answer an
// error.
type pstGetter struct{ fail map[string]bool }

func (g pstGetter) Get(_ context.Context, path string, params map[string]string) (json.RawMessage, error) {
	if g.fail[path] {
		return nil, fmt.Errorf("500 internal error")
	}
	if params["page"] != "1" {
		return json.RawMessage(`[]`), nil
	}
	if strings.Contains(path, "/101/") {
		return json.RawMessage(`[{"recipient_id":1,"pst_id":101,"user":{"id":1,"email":"alice@acme.com"},"delivered_at":"2026-01-01T00:00:00.000Z","clicked_at":"2026-01-02T00:00:00.000Z"}]`), nil
	}
	return json.RawMessage(`[{"recipient_id":2,"pst_id":102,"user":{"id":1,"email":"alice@acme.com"},"delivered_at":"2026-04-01T00:00:00.000Z","clicked_at":"2026-04-02T00:00:00.000Z"}]`), nil
}

// The recipient walk records itself complete only when every test was walked
// and the tests were a completely synced set.
func TestR2RecipientWalkRecordsOnlyAFinishedWalk(t *testing.T) {
	ctx := context.Background()
	db, done := seed(t)
	defer done()

	walk, err := SyncRecipients(ctx, pstGetter{fail: map[string]bool{"/phishing/security_tests/102/recipients": true}}, db, 100)
	if err != nil || walk.Failed != 1 {
		t.Fatalf("partial walk: %+v, %v", walk, err)
	}
	if walk.Complete || recipientsHeld(ctx, db) {
		t.Errorf("a walk with a failed test is recorded complete")
	}
	if _, err := RepeatClickers(ctx, db, 2, 0, ""); !errors.Is(err, ErrInputNotSynced) {
		t.Errorf("repeat-clickers after a partial walk: err %v, want ErrInputNotSynced", err)
	}

	if walk, err = SyncRecipients(ctx, pstGetter{}, db, 100); err != nil || !walk.Complete || !recipientsHeld(ctx, db) {
		t.Errorf("a full walk over synced tests: %+v, %v, held %v; want complete", walk, err, recipientsHeld(ctx, db))
	}

	if _, err := db.Exec(`UPDATE sync_state SET last_attempt_complete = 0 WHERE resource_type = 'phishing-tests'`); err != nil {
		t.Fatal(err)
	}
	if walk, err = SyncRecipients(ctx, pstGetter{}, db, 100); err != nil || walk.Complete {
		t.Errorf("a walk over phishing tests whose sync never finished: %+v, %v; want not complete", walk, err)
	}
	markSynced(t, db, "phishing-tests")
	if recipientsHeld(ctx, db) {
		t.Errorf("a walk that ran over unsynced tests reads as held once the tests sync")
	}
}

// Class stale-source-preferred, the group weight: a group's member count is
// held by the groups table and by each of its risk snapshots. The failing
// input: a group whose row is no longer in the groups table (its snapshots
// still carry member_count 50) was weighted as 0 members (member_count 0,
// contribution = delta x 1); with no member count anywhere the weight is
// unknown (null), never 0.
func TestR2GroupWeightReadsTheNewerMemberCount(t *testing.T) {
	ctx := context.Background()
	db, done := seed(t)
	defer done()
	row := func() map[string]any {
		t.Helper()
		got, err := GroupRiskContribution(ctx, db, "90d", 10)
		if err != nil || len(got) != 1 {
			t.Fatalf("group-risk-contribution: %+v, %v", got, err)
		}
		b, _ := json.Marshal(got[0])
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		return m
	}
	if _, err := db.Exec(`DELETE FROM "groups" WHERE id = '10'`); err != nil {
		t.Fatal(err)
	}
	if m := row(); m["member_count"] != 50.0 || m["weighted_contribution"] != 1000.0 {
		t.Errorf("group out of the groups table, snapshots carry 50 members: member_count %v, weighted_contribution %v; want 50, 1000", m["member_count"], m["weighted_contribution"])
	}
	if _, err := db.Exec(`UPDATE "risk_snapshots" SET member_count = NULL WHERE entity_type = 'group'`); err != nil {
		t.Fatal(err)
	}
	if m := row(); m["member_count"] != nil || m["weighted_contribution"] != nil {
		t.Errorf("no member count anywhere: member_count %v, weighted_contribution %v; want null, null", m["member_count"], m["weighted_contribution"])
	}
	// A later read of the group (groups get) is the newer member count.
	later := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO "groups"(id,data,name,member_count,current_risk_score,status,synced_at) VALUES ('10','{}','Finance',80,60,'active',?)`, later); err != nil {
		t.Fatal(err)
	}
	if m := row(); m["member_count"] != 80.0 || m["weighted_contribution"] != 1600.0 {
		t.Errorf("groups get read 80 members after the snapshots: member_count %v, weighted_contribution %v; want 80, 1600", m["member_count"], m["weighted_contribution"])
	}
}

// Class stale-source-preferred, the account score: the account table (an
// `account info` read) and the account risk snapshots (a sync's GET /account
// read) both hold it; the newer reading wins either way.
func TestR2AccountScoreReadsTheNewerSource(t *testing.T) {
	ctx := context.Background()
	db, done := seed(t)
	defer done()
	later := time.Now().UTC().Add(time.Hour)
	if _, err := db.Exec(`UPDATE "account" SET current_risk_score = 90, synced_at = ? WHERE id = 'acct'`, later.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if rep, _ := QBR(ctx, db, "1y"); rep.AccountRiskScore == nil || *rep.AccountRiskScore != 90 {
		t.Errorf("account table read 90 after the newest snapshot (45): account_risk_score %v, want 90", derefF(rep.AccountRiskScore))
	}
	if _, err := db.Exec(`INSERT INTO "risk_snapshots"(entity_type,entity_id,entity_name,risk_score,snapshot_at) VALUES ('account','account','Acme',70,?)`,
		later.Add(time.Hour).Format("2006-01-02T15:04:05.000Z")); err != nil {
		t.Fatal(err)
	}
	if rep, _ := QBR(ctx, db, "1y"); rep.AccountRiskScore == nil || *rep.AccountRiskScore != 70 {
		t.Errorf("a sync read 70 after the account table's 90: account_risk_score %v, want 70", derefF(rep.AccountRiskScore))
	}
}

// Class stale-source-preferred, the drift input after snapshots carry each
// row's read time: one sync reads the users at one time and the account at
// another, two snapshot times in the store but no entity read twice. Drift
// then has no answer (ErrInputNotSynced), never "nothing moved" ([]), and
// freshness does not call risk-drift ready.
func TestR2DriftNeedsOneEntityReadTwice(t *testing.T) {
	ctx := context.Background()
	db, done := seed(t)
	defer done()
	if _, err := db.Exec(`DELETE FROM "risk_snapshots"`); err != nil {
		t.Fatal(err)
	}
	t0 := time.Now().UTC().Add(-time.Hour)
	for _, s := range []struct {
		kind, id string
		at       time.Time
	}{{"user", "1", t0}, {"user", "2", t0}, {"group", "10", t0}, {"account", "account", t0.Add(700 * time.Millisecond)}} {
		if _, err := db.Exec(`INSERT INTO "risk_snapshots"(entity_type,entity_id,entity_name,risk_score,snapshot_at) VALUES (?,?,'x',50,?)`,
			s.kind, s.id, s.at.Format("2006-01-02T15:04:05.000Z")); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := RiskDrift(ctx, db, "90d", "", false, 0); !errors.Is(err, ErrInputNotSynced) {
		t.Errorf("risk-drift with no entity read twice: %+v, err %v; want ErrInputNotSynced", got, err)
	}
	fr, err := Freshness(ctx, db, 0, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range fr.Commands {
		if c.Command == "risk-drift" && c.Ready {
			t.Errorf("freshness: risk-drift ready with no entity read twice: %+v", c)
		}
	}
	// Control: the users read again later are a drift.
	if _, err := db.Exec(`INSERT INTO "risk_snapshots"(entity_type,entity_id,entity_name,risk_score,snapshot_at) VALUES ('user','1','x',70,?)`,
		time.Now().UTC().Format("2006-01-02T15:04:05.000Z")); err != nil {
		t.Fatal(err)
	}
	if got, err := RiskDrift(ctx, db, "90d", "", false, 0); err != nil || len(got) != 1 || got[0].Delta != 20 {
		t.Errorf("risk-drift with user 1 read twice: %+v, err %v; want one row, delta 20", got, err)
	}
}
