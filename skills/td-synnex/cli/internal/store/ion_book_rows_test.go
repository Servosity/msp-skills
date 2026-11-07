// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (handfixes.json: ion-book-one-account).

package store

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// Every writer of a book row, a sync walk or a live read's write-back, commits
// it only for the account the list is recorded for and only while a walk's
// scope still holds, in the same transaction; a refused write leaves nothing.
func TestIONBookRowCommitsOnlyForTheListsAccount(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "book.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	prev := IONBookAccount
	defer func() { IONBookAccount = prev }()
	row := func(id string) []json.RawMessage {
		return []json.RawMessage{json.RawMessage(`{"id":"` + id + `","customerId":"108536"}`)}
	}
	tag := func(id string) string {
		var a string
		_ = s.DB().QueryRow(`SELECT account FROM ion_book_row WHERE resource_type = 'subscriptions' AND id = ?`, id).Scan(&a)
		return a
	}
	stored := func(id string) bool {
		var n int
		_ = s.DB().QueryRow(`SELECT COUNT(*) FROM resources WHERE resource_type = 'subscriptions' AND id = ?`, id).Scan(&n)
		return n == 1
	}

	// No sync has claimed the list: the write commits untagged.
	IONBookAccount = func() string { return "10505" }
	if _, _, err := s.UpsertBatch("subscriptions", row("a")); err != nil || !stored("a") || tag("a") != "" {
		t.Fatalf("unclaimed list: want row a stored untagged; err=%v stored=%v tag=%q", err, stored("a"), tag("a"))
	}

	for _, q := range []string{
		`CREATE TABLE ion_store_account (resource_type TEXT PRIMARY KEY, account TEXT NOT NULL)`,
		`INSERT INTO ion_store_account VALUES ('subscriptions', '20000')`,
		`INSERT INTO sync_state (resource_type, last_cursor, total_count, last_attempt_complete, scope_key) VALUES ('subscriptions', '', 0, 0, 'scope-20000')`,
	} {
		if _, err := s.DB().Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	cases := []struct {
		name, id, account, scope string
		declared, want           bool
	}{
		{"write-back for another account", "b", "10505", "", false, false},
		{"write-back for no account", "c", "", "", false, false},
		{"write-back for the list's account", "d", "20000", "", false, true},
		{"walk for another account", "e", "10505", "scope-20000", true, false},
		{"walk whose scope was rebound", "f", "20000", "scope-old", true, false},
		{"walk holding claim and scope", "g", "20000", "scope-20000", true, true},
	}
	for _, c := range cases {
		IONBookAccount = func() string { return c.account }
		if c.declared {
			s.DeclareIONBookWalk("subscriptions", c.account, c.scope)
		}
		_, _, err := s.UpsertBatch("subscriptions", row(c.id))
		s.EndIONBookWalk("subscriptions")
		if c.want && (err != nil || !stored(c.id) || tag(c.id) != c.account) {
			t.Errorf("%s: want row %s stored and tagged %s; err=%v stored=%v tag=%q", c.name, c.id, c.account, err, stored(c.id), tag(c.id))
		}
		if !c.want && (err == nil || stored(c.id) || tag(c.id) != "") {
			t.Errorf("%s: want the write refused and rolled back; err=%v stored=%v tag=%q", c.name, err, stored(c.id), tag(c.id))
		}
	}
	// A refused overwrite keeps the owner's row and tag.
	IONBookAccount = func() string { return "10505" }
	if _, _, err := s.UpsertBatch("subscriptions", row("d")); err == nil || tag("d") != "20000" {
		t.Fatalf("another account's overwrite of row d: want refused, tag kept 20000; err=%v tag=%q", err, tag("d"))
	}
}
