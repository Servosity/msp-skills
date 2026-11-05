// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"testing"
	"time"
)

func TestServerHealthIssues(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	server := func(js string) mdmServer {
		var s mdmServer
		if err := json.Unmarshal([]byte(js), &s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	for _, c := range []struct {
		name, js string
		want     []string
		days     int // -1: unknown
	}{
		{"active, fresh", `{"id":"S1","attributes":{"serverName":"Jamf","status":"ACTIVE","deviceCount":3,"lastConnectedDateTime":"2026-10-26T12:00:00Z"}}`, nil, 1},
		{"active, silent, owns devices", `{"id":"S2","attributes":{"status":"ACTIVE","deviceCount":3,"lastConnectedDateTime":"2026-09-27T12:00:00Z"}}`, []string{issueStale}, 30},
		{"active, silent, owns nothing", `{"id":"S3","attributes":{"status":"ACTIVE","deviceCount":0,"lastConnectedDateTime":"2026-09-27T12:00:00Z"}}`, nil, 30},
		{"active, never connected", `{"id":"S4","attributes":{"status":"ACTIVE","deviceCount":2}}`, nil, -1},
		{"inactive, owns devices", `{"id":"S5","attributes":{"status":"INACTIVE","deviceCount":1,"lastConnectedDateTime":"2026-10-20T12:00:00Z"}}`, []string{issueDeadOwnsDevs}, 7},
		{"deleted, empty", `{"id":"S6","attributes":{"status":"DELETED","deviceCount":0}}`, nil, -1},
	} {
		r := serverHealth("", server(c.js), now, 14*24*time.Hour)
		if len(r.Issues) != len(c.want) || (len(c.want) == 1 && r.Issues[0] != c.want[0]) {
			t.Errorf("%s: issues %v, want %v", c.name, r.Issues, c.want)
		}
		if daysOr(r.DaysSince) != c.days {
			t.Errorf("%s: days since %d, want %d", c.name, daysOr(r.DaysSince), c.days)
		}
		if r.DefaultFamilies == nil || r.Issues == nil {
			t.Errorf("%s: JSON arrays must be [], not null", c.name)
		}
	}
	// Name falls back to the id so a row is never anonymous.
	if r := serverHealth("", server(`{"id":"S9","attributes":{"status":"ACTIVE"}}`), now, time.Hour); r.Name != "S9" {
		t.Errorf("name = %q, want the id", r.Name)
	}
}

func TestActorRows(t *testing.T) {
	rows := actorRows([]digestEvent{
		{Time: "2026-10-20T10:00:00Z", Type: "DEVICE_ASSIGNED_TO_SERVER", Category: "DEVICE_INVENTORY", Outcome: "SUCCESS", ActorType: "USER", Actor: "pat@example.com"},
		{Time: "2026-10-22T10:00:00Z", Type: "DEVICE_ASSIGNED_TO_SERVER", Category: "DEVICE_INVENTORY", Outcome: "FAILURE", ActorType: "USER", Actor: "pat@example.com"},
		{Time: "2026-10-21T10:00:00Z", Type: "API_ACCOUNT_KEY_GENERATED", Category: "ACCOUNTS", Outcome: "SUCCESS", ActorType: "USER", Actor: "pat@example.com"},
		{Time: "2026-10-23T10:00:00Z", Type: "DEVICE_UNASSIGNED_FROM_SERVER", Category: "DEVICE_INVENTORY", Outcome: "SUCCESS", ActorType: "API_ACCOUNT", Actor: "BUSINESSAPI.x"},
		{Time: "2026-10-24T10:00:00Z", Type: "X", Outcome: "SUCCESS"},
		// The same actor id without its optional name (digest shows the id).
		{Time: "2026-10-25T10:00:00Z", Type: "X", Category: "ACCOUNTS", Outcome: "SUCCESS", ActorType: "USER", ActorID: "u1", Actor: "u1"},
		{Time: "2026-10-26T10:00:00Z", Type: "X", Category: "ACCOUNTS", Outcome: "SUCCESS", ActorType: "USER", ActorID: "u1", Actor: "admin@example.com"},
	})
	if len(rows) != 4 {
		t.Fatalf("rows = %d, want 4: %+v", len(rows), rows)
	}
	for _, r := range rows {
		if r.Actor == "u1" {
			t.Errorf("actor u1 kept its id although an event named it: %+v", rows)
		}
		if r.Actor == "admin@example.com" && r.Events != 2 {
			t.Errorf("one actor id split into rows: %+v", rows)
		}
	}
	pat := rows[0]
	if pat.Actor != "pat@example.com" || pat.Events != 3 || pat.NotSuccess != 1 {
		t.Errorf("pat row = %+v", pat)
	}
	if pat.First != "2026-10-20T10:00:00Z" || pat.Last != "2026-10-22T10:00:00Z" {
		t.Errorf("pat first/last = %s/%s", pat.First, pat.Last)
	}
	if pat.Categories[0] != (countByKey{"DEVICE_INVENTORY", 2}) || pat.Types[0] != (countByKey{"DEVICE_ASSIGNED_TO_SERVER", 2}) {
		t.Errorf("pat counts = %+v %+v", pat.Categories, pat.Types)
	}
	if rows[3].Actor != "(unknown)" && rows[2].Actor != "(unknown)" && rows[1].Actor != "(unknown)" {
		t.Errorf("an event with no actor must group under (unknown): %+v", rows)
	}
}
