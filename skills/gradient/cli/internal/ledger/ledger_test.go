// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package ledger

import (
	"testing"
	"time"
)

func TestPushLedgerRoundTrip(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC().Truncate(time.Second)
	in := []PushRecord{
		{RunID: "r1", At: now, ServiceID: "svc1", AccountID: "a1", UnitCount: 10, NoBuild: true, Status: "sent"},
		{RunID: "r1", At: now, ServiceID: "svc1", AccountID: "a2", UnitCount: 5, Status: "failed", Error: "boom"},
	}
	if err := AppendPushes(dir, in); err != nil {
		t.Fatalf("AppendPushes: %v", err)
	}
	got, err := ReadPushes(dir)
	if err != nil {
		t.Fatalf("ReadPushes: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d records, want 2", len(got))
	}
	if got[0].AccountID != "a1" || got[0].UnitCount != 10 || !got[0].NoBuild {
		t.Errorf("record 0 mismatch: %+v", got[0])
	}
	if got[1].Status != "failed" || got[1].Error != "boom" {
		t.Errorf("record 1 mismatch: %+v", got[1])
	}
}

func TestReadPushesMissingFileIsEmpty(t *testing.T) {
	got, err := ReadPushes(t.TempDir())
	if err != nil {
		t.Fatalf("ReadPushes on empty dir: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want empty, got %d", len(got))
	}
}

func TestAlertLedgerUpdate(t *testing.T) {
	dir := t.TempDir()
	rec := AlertRecord{At: time.Now().UTC(), AccountID: "a1", AlertID: "al-1", MessageID: "m-1", Title: "Backup failure", TicketStatus: "pending"}
	if err := AppendAlert(dir, rec); err != nil {
		t.Fatalf("AppendAlert: %v", err)
	}
	rec2 := rec
	rec2.MessageID = "m-2"
	if err := AppendAlert(dir, rec2); err != nil {
		t.Fatalf("AppendAlert 2: %v", err)
	}

	rec.TicketID = "T-99"
	rec.TicketStatus = "created"
	ok, err := UpdateAlert(dir, rec)
	if err != nil || !ok {
		t.Fatalf("UpdateAlert: ok=%v err=%v", ok, err)
	}
	alerts, err := ReadAlerts(dir)
	if err != nil {
		t.Fatalf("ReadAlerts: %v", err)
	}
	if len(alerts) != 2 {
		t.Fatalf("got %d alerts, want 2", len(alerts))
	}
	if alerts[0].TicketID != "T-99" || alerts[0].TicketStatus != "created" {
		t.Errorf("alert 0 not updated: %+v", alerts[0])
	}
	if alerts[1].TicketStatus != "pending" {
		t.Errorf("alert 1 should be untouched: %+v", alerts[1])
	}

	missing := AlertRecord{MessageID: "nope"}
	ok, err = UpdateAlert(dir, missing)
	if err != nil {
		t.Fatalf("UpdateAlert missing: %v", err)
	}
	if ok {
		t.Error("UpdateAlert on unknown messageId should return false")
	}
}

func TestComputeDrift(t *testing.T) {
	mk := func(run, svc, acct string, count float64, status string) PushRecord {
		return PushRecord{RunID: run, ServiceID: svc, AccountID: acct, UnitCount: count, Status: status}
	}
	cases := []struct {
		name          string
		pushes        []PushRecord
		wantChanges   int
		wantAdded     int
		wantRemoved   int
		wantUnchanged int
		wantNote      bool
	}{
		{name: "empty ledger", pushes: nil, wantNote: true},
		{name: "single run", pushes: []PushRecord{mk("r1", "s", "a", 1, "sent")}, wantNote: true},
		{
			name: "changed added removed unchanged",
			pushes: []PushRecord{
				mk("r1", "s1", "a1", 10, "sent"),
				mk("r1", "s1", "a2", 5, "sent"),
				mk("r1", "s1", "a3", 7, "sent"),
				mk("r2", "s1", "a1", 12, "sent"), // changed
				mk("r2", "s1", "a2", 5, "sent"),  // unchanged
				mk("r2", "s1", "a4", 3, "sent"),  // added (a3 removed)
			},
			wantChanges: 1, wantAdded: 1, wantRemoved: 1, wantUnchanged: 1,
		},
		{
			name: "failed rows excluded",
			pushes: []PushRecord{
				mk("r1", "s1", "a1", 10, "sent"),
				mk("r2", "s1", "a1", 99, "failed"), // failed: a1 looks removed, not changed
			},
			wantChanges: 0, wantAdded: 0, wantRemoved: 1,
		},
		{
			name: "three runs compares last two",
			pushes: []PushRecord{
				mk("r1", "s1", "a1", 1, "sent"),
				mk("r2", "s1", "a1", 2, "sent"),
				mk("r3", "s1", "a1", 3, "sent"),
			},
			wantChanges: 1, // r2 -> r3
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rep := ComputeDrift(tc.pushes)
			if len(rep.Changes) != tc.wantChanges {
				t.Errorf("changes = %d, want %d", len(rep.Changes), tc.wantChanges)
			}
			if len(rep.Added) != tc.wantAdded {
				t.Errorf("added = %d, want %d", len(rep.Added), tc.wantAdded)
			}
			if len(rep.Removed) != tc.wantRemoved {
				t.Errorf("removed = %d, want %d", len(rep.Removed), tc.wantRemoved)
			}
			if rep.Unchanged != tc.wantUnchanged {
				t.Errorf("unchanged = %d, want %d", rep.Unchanged, tc.wantUnchanged)
			}
			if tc.wantNote && rep.Note == "" {
				t.Error("expected explanatory note")
			}
		})
	}
}

func TestComputeDriftDelta(t *testing.T) {
	rep := ComputeDrift([]PushRecord{
		{RunID: "r1", ServiceID: "s", AccountID: "a", UnitCount: 10, Status: "sent"},
		{RunID: "r2", ServiceID: "s", AccountID: "a", UnitCount: 7, Status: "sent"},
	})
	if len(rep.Changes) != 1 {
		t.Fatalf("changes = %d, want 1", len(rep.Changes))
	}
	c := rep.Changes[0]
	if c.Old != 10 || c.New != 7 || c.Delta != -3 {
		t.Errorf("delta mismatch: %+v", c)
	}
}
