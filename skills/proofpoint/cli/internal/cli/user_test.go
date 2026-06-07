// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written tests for the user novel feature.

package cli

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"proofpoint-pp-cli/internal/store"
)

func TestDecodeUserEventClick(t *testing.T) {
	data := []byte(`{
		"id": "c1",
		"clickTime": "2026-06-06T10:00:00Z",
		"threatID": "threat-9",
		"classification": "phish",
		"threatStatus": "active",
		"url": "https://evil.example.test/x",
		"sender": "attacker@example.test",
		"recipient": "jane.doe@example.test"
	}`)
	ev := decodeUserEvent("siem-clicks-permitted", data)
	if ev.Kind != "click_permitted" {
		t.Fatalf("kind = %q, want click_permitted", ev.Kind)
	}
	if ev.Time != "2026-06-06T10:00:00Z" || ev.ThreatID != "threat-9" || ev.URL == "" {
		t.Fatalf("click fields not projected: %+v", ev)
	}
}

func TestDecodeUserEventMessageFallsBackToThreatsInfoMap(t *testing.T) {
	data := []byte(`{
		"GUID": "m1",
		"messageTime": "2026-06-06T09:00:00Z",
		"subject": "Invoice attached",
		"sender": "attacker@example.test",
		"recipient": ["jane.doe@example.test"],
		"threatsInfoMap": [{"threatId": "threat-7", "classification": "malware", "threatStatus": "active"}]
	}`)
	ev := decodeUserEvent("siem-messages-delivered", data)
	if ev.Kind != "message_delivered" {
		t.Fatalf("kind = %q, want message_delivered", ev.Kind)
	}
	if ev.Time != "2026-06-06T09:00:00Z" {
		t.Fatalf("messageTime not used as fallback time: %+v", ev)
	}
	if ev.ThreatID != "threat-7" || ev.Classification != "malware" {
		t.Fatalf("threatsInfoMap fallback not applied: %+v", ev)
	}
	if ev.Subject != "Invoice attached" {
		t.Fatalf("subject not projected: %+v", ev)
	}
}

// TestUserViewStatusReflectsFetchFailures verifies the top-level status
// distinguishes a clean empty result (ok) from a degraded one where live
// People lookups failed (partial), so an agent can't read "all sources
// failed" as success.
func TestUserViewStatusReflectsFetchFailures(t *testing.T) {
	if got := userViewStatus(userView{EventCount: 0}); got != "ok" {
		t.Fatalf("clean empty result: status = %q, want ok", got)
	}
	if got := userViewStatus(userView{FetchFailures: []string{"vap: 401"}}); got != "partial" {
		t.Fatalf("degraded result: status = %q, want partial", got)
	}
}

func TestMatchIdentityEmail(t *testing.T) {
	id := peopleIdentity{Emails: []string{"Jane.Doe@Example.Test", "jdoe@example.test"}}
	if !matchIdentityEmail(id, "jane.doe@example.test") {
		t.Fatal("case-insensitive match failed")
	}
	if matchIdentityEmail(id, "other@example.test") {
		t.Fatal("non-member email matched")
	}
}

// TestQueryUserEventsAcrossShapes verifies the SQL matches scalar click
// recipients AND array message recipients, and excludes other people.
func TestQueryUserEventsAcrossShapes(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}
	defer db.Close()

	click := json.RawMessage(`{"id":"c1","clickTime":"2026-06-06T10:00:00Z","recipient":"jane.doe@example.test","threatID":"t1"}`)
	otherClick := json.RawMessage(`{"id":"c2","clickTime":"2026-06-06T11:00:00Z","recipient":"other@example.test","threatID":"t1"}`)
	msg := json.RawMessage(`{"GUID":"m1","messageTime":"2026-06-06T08:00:00Z","recipient":["JANE.DOE@example.test"],"subject":"hi","threatsInfoMap":[{"threatId":"t2"}]}`)
	ccMsg := json.RawMessage(`{"GUID":"m2","messageTime":"2026-06-06T07:00:00Z","recipient":["other@example.test"],"ccAddresses":["jane.doe@example.test"],"subject":"cc"}`)

	if err := db.Upsert("siem-clicks-permitted", "c1", click); err != nil {
		t.Fatal(err)
	}
	if err := db.Upsert("siem-clicks-permitted", "c2", otherClick); err != nil {
		t.Fatal(err)
	}
	if err := db.Upsert("siem-messages-delivered", "m1", msg); err != nil {
		t.Fatal(err)
	}
	if err := db.Upsert("siem-messages-delivered", "m2", ccMsg); err != nil {
		t.Fatal(err)
	}

	events, err := queryUserEvents(context.Background(), db.DB(), "jane.doe@example.test", 50)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("got %d events, want 3 (scalar recipient, array recipient, cc)", len(events))
	}
	if events[0].Time < events[1].Time {
		t.Fatalf("events must sort newest first: %+v", events)
	}
	for _, ev := range events {
		if ev.Kind == "click_permitted" && ev.ThreatID != "t1" {
			t.Fatalf("click threat id lost: %+v", ev)
		}
	}

	threatEvents, err := queryThreatEvents(context.Background(), db.DB(), "t2", 50)
	if err != nil {
		t.Fatalf("threat query: %v", err)
	}
	if len(threatEvents) != 1 || threatEvents[0].Kind != "message_delivered" {
		t.Fatalf("threatsInfoMap threat match failed: %+v", threatEvents)
	}
}
