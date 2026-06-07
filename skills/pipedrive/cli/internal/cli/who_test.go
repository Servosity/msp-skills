// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// Novel feature tests: who. Hand-authored against the local store.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"pipedrive-pp-cli/internal/store"
)

func seedPerson(t *testing.T, db *store.Store, id, name, orgID, data string) {
	t.Helper()
	_, err := db.DB().Exec(
		`INSERT INTO persons (id, data, name, org_id) VALUES (?, ?, ?, ?)`,
		id, data, name, orgID)
	if err != nil {
		t.Fatalf("seed person %s: %v", id, err)
	}
}

func TestNovelWho_QueryWho(t *testing.T) {
	_, db := openTestStore(t)

	// Person with primary email/phone in the Pipedrive {value,primary} shape.
	personData := `{"email":[{"value":"jane@acme.test","primary":true}],"phone":[{"value":"+15551234","primary":true}]}`
	seedPerson(t, db, "7", "Jane Doe", "3", personData)
	// A second "Jane" so other_matches is exercised.
	seedPerson(t, db, "8", "Jane Doe Jr", "3", `{}`)

	if _, err := db.DB().Exec(
		`INSERT INTO organizations (id, data, name) VALUES ('3', '{}', 'Acme Corp')`); err != nil {
		t.Fatalf("seed org: %v", err)
	}

	// Open deals for person 7.
	if _, err := db.DB().Exec(`
		INSERT INTO deals (id, data, title, value, currency, status, person_id, stage_id)
		VALUES ('100', '{}', 'Renewal', 4000, 'USD', 'open', '7', 2),
		       ('101', '{}', 'Upsell', 1000, 'USD', 'open', '7', 2),
		       ('102', '{}', 'Closed One', 9000, 'USD', 'won', '7', 5)`); err != nil {
		t.Fatalf("seed deals: %v", err)
	}

	// Activities: one done (past), one open (future).
	if _, err := db.DB().Exec(`
		INSERT INTO activities (id, data, subject, type, due_date, done, person_id)
		VALUES ('200', '{}', 'Kickoff call', 'call', '2026-04-01', 1, 7),
		       ('201', '{}', 'Follow-up', 'task', '2030-01-01', 0, 7)`); err != nil {
		t.Fatalf("seed activities: %v", err)
	}

	// Notes (newest first by add_time).
	if _, err := db.DB().Exec(`
		INSERT INTO notes (id, data, content, person_id, add_time)
		VALUES ('300', '{}', 'Old note', 7, '2026-01-01 00:00:00'),
		       ('301', '{}', 'Newest note\nwith newline', 7, '2026-06-01 00:00:00')`); err != nil {
		t.Fatalf("seed notes: %v", err)
	}

	res, found, err := queryWho(context.Background(), db.DB(), "Jane Doe", 3)
	if err != nil {
		t.Fatalf("queryWho: %v", err)
	}
	if !found {
		t.Fatal("expected to find a person")
	}
	if res.Person.Name == "" {
		t.Fatal("person name empty")
	}
	if res.Person.OrgName != "Acme Corp" {
		t.Fatalf("org name = %q, want Acme Corp", res.Person.OrgName)
	}
	if res.Person.Email != "jane@acme.test" || res.Person.Phone != "+15551234" {
		t.Fatalf("email/phone extraction wrong: %+v", res.Person)
	}
	if res.OpenDealCount != 2 || res.OpenDealValue != 5000 {
		t.Fatalf("open deals = %d/%v, want 2/5000", res.OpenDealCount, res.OpenDealValue)
	}
	if res.LastActivity == nil || res.LastActivity.ID != "200" {
		t.Fatalf("last activity = %+v, want id 200", res.LastActivity)
	}
	if res.NextActivity == nil || res.NextActivity.ID != "201" {
		t.Fatalf("next activity = %+v, want id 201", res.NextActivity)
	}
	if len(res.Notes) != 2 || res.Notes[0].AddTime != "2026-06-01 00:00:00" {
		t.Fatalf("notes wrong: %+v", res.Notes)
	}
	if bytes.ContainsAny([]byte(res.Notes[0].Excerpt), "\n") {
		t.Fatalf("note excerpt has newline: %q", res.Notes[0].Excerpt)
	}
	if len(res.OtherMatches) != 1 {
		t.Fatalf("other matches = %d, want 1", len(res.OtherMatches))
	}
}

func TestNovelWho_NotFound(t *testing.T) {
	_, db := openTestStore(t)
	_, found, err := queryWho(context.Background(), db.DB(), "Nobody Here", 3)
	if err != nil {
		t.Fatalf("queryWho: %v", err)
	}
	if found {
		t.Fatal("expected not found on empty store")
	}
}

func TestNovelWho_JSONEnvelope(t *testing.T) {
	path, db := openTestStore(t)
	seedPerson(t, db, "7", "Solo Person", "", `{}`)

	cmd := newNovelWhoCmd(&rootFlags{asJSON: true})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"Solo Person", "--db", path})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	var res whoResult
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("invalid json: %v (raw=%s)", err, out.String())
	}
	if res.Person.Name != "Solo Person" {
		t.Fatalf("person name = %q, want Solo Person", res.Person.Name)
	}
	// Empty slices marshal as [], and null activities marshal as null.
	if !bytes.Contains(out.Bytes(), []byte(`"open_deals": []`)) {
		t.Fatalf("expected open_deals:[] in output, got %s", out.String())
	}
	if !bytes.Contains(out.Bytes(), []byte(`"last_activity": null`)) {
		t.Fatalf("expected last_activity:null in output, got %s", out.String())
	}
}

func seedActivityForWho(t *testing.T, db *store.Store, id string, person int, subject, due string, done int) {
	t.Helper()
	var dueVal any
	if due != "" {
		dueVal = due
	}
	_, err := db.DB().Exec(
		`INSERT INTO activities (id, data, subject, type, due_date, done, person_id) VALUES (?, '{}', ?, 'call', ?, ?, ?)`,
		id, subject, dueVal, done, person)
	if err != nil {
		t.Fatalf("seed activity %s: %v", id, err)
	}
}

// Regression: a future-dated done activity must not outrank a recent past one
// as "last activity"; the done branch is capped at now.
func TestNovelWho_LastActivityIgnoresFutureDatedDone(t *testing.T) {
	_, db := openTestStore(t)
	seedPerson(t, db, "7", "Jane Doe", "", "{}")
	seedActivityForWho(t, db, "a1", 7, "Recent call", "2026-05-01 10:00:00", 1)
	seedActivityForWho(t, db, "a2", 7, "Future done", "2099-01-01 10:00:00", 1)

	res, found, err := queryWho(context.Background(), db.DB(), "Jane Doe", 0)
	if err != nil || !found {
		t.Fatalf("queryWho: found=%v err=%v", found, err)
	}
	if res.LastActivity == nil || res.LastActivity.ID != "a1" {
		t.Fatalf("last activity = %+v, want a1 (recent past done)", res.LastActivity)
	}
}

// Regression: a done activity with a NULL due_date stays eligible as last
// activity (COALESCE(”) sorts before any timestamp but is not filtered out).
func TestNovelWho_LastActivityNullDueDoneEligible(t *testing.T) {
	_, db := openTestStore(t)
	seedPerson(t, db, "8", "Null Due", "", "{}")
	seedActivityForWho(t, db, "b1", 8, "Logged call", "", 1)

	res, found, err := queryWho(context.Background(), db.DB(), "Null Due", 0)
	if err != nil || !found {
		t.Fatalf("queryWho: found=%v err=%v", found, err)
	}
	if res.LastActivity == nil || res.LastActivity.ID != "b1" {
		t.Fatalf("last activity = %+v, want b1 (NULL-due done)", res.LastActivity)
	}
}
