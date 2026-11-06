// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package insights

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// fullPages answers every recipients page with a full page: a phishing test
// with more recipients than the walk's page bound.
type fullPages struct{ calls int }

func (f *fullPages) Get(_ context.Context, _ string, params map[string]string) (json.RawMessage, error) {
	f.calls++
	return json.RawMessage(fmt.Sprintf(`[{"recipient_id":%d,"pst_id":16142,"user":{"id":%d,"email":"u%d@kb4-demo.com"},"clicked_at":"2026-08-02T16:00:00.000Z"}]`,
		f.calls, f.calls, f.calls)), nil
}

// Codex review round 1 (knowbe4 4.34.2 reprint), class
// incomplete-paging-read-as-complete: the recipient walk stopped after its
// page bound and reported the phishing test as walked in full, so recipients
// past the bound (and their documented clicked_at) never reached
// repeat-clickers. A walk that stops at its bound while the last page was full
// is a failed walk for that test.
func TestRecipientWalkStoppedAtItsBoundIsAFailure(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "walk.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `CREATE TABLE "phishing_tests" (pst_id INTEGER, campaign_id INTEGER)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO "phishing_tests" VALUES (16142, 3423)`); err != nil {
		t.Fatal(err)
	}
	if err := EnsureSchema(ctx, db); err != nil {
		t.Fatal(err)
	}
	g := &fullPages{}
	walk, err := SyncRecipients(ctx, g, db, 1)
	if err == nil || walk.Failed != 1 {
		t.Fatalf("a walk stopped at its bound after %d full pages: err %v, failed %d; want the test counted as failed", g.calls, err, walk.Failed)
	}
}
