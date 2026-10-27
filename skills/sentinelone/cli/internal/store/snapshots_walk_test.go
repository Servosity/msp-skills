// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func openWalkTestStore(t *testing.T, path string) *Store {
	t.Helper()
	s, err := OpenWithContext(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// outsiderConn pins a connection opened BEFORE a walk is armed: it carries no
// recorder, exactly like a connection in another process (a concurrent sync,
// an MCP tool call caching a read).
func outsiderConn(t *testing.T, s *Store) *sql.Conn {
	t.Helper()
	conn, err := s.DB().Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.PingContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func execConn(t *testing.T, conn *sql.Conn, q string, args ...any) {
	t.Helper()
	if _, err := conn.ExecContext(context.Background(), q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

// walkCompletes writes the sync_state sequence a complete generated walk
// writes: an incomplete-attempt marker when it starts requesting, then the
// completed checkpoint.
func walkCompletes(t *testing.T, s *Store, entity string) {
	t.Helper()
	if err := s.SaveSyncProgress(entity, "", 0); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveSyncState(entity, "", 1); err != nil {
		t.Fatal(err)
	}
}

func captureIDs(t *testing.T, s *Store, entity string, at time.Time) []string {
	t.Helper()
	rows, err := s.FleetSnapshotRowsByID(entity, at)
	if err != nil {
		t.Fatal(err)
	}
	ids := keysSorted(rows)
	return ids
}

func keysSorted[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestFleetWalkHoldsExactlyTheRowsItsInvocationWrote: every row carries the
// same synced_at second, so no timestamp can separate them. The capture holds
// B (written by the walk) and neither A (written before the walk) nor C
// (written during the walk by a connection without the recorder), and B's
// data is what the walk wrote, not the outsider's later overwrite.
func TestFleetWalkHoldsExactlyTheRowsItsInvocationWrote(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.db")
	pre := openWalkTestStore(t, path)
	if err := pre.Upsert("agents", "A", []byte(`{"id":"A","v":"before"}`)); err != nil {
		t.Fatal(err)
	}
	out := outsiderConn(t, pre)

	walk, err := pre.BeginFleetWalk()
	if err != nil {
		t.Fatal(err)
	}
	walker := openWalkTestStore(t, path) // opened after arming: its connections record
	if err := walker.Upsert("agents", "B", []byte(`{"id":"B","v":"walk"}`)); err != nil {
		t.Fatal(err)
	}
	walkCompletes(t, walker, "agents")
	execConn(t, out, `INSERT INTO resources (id, resource_type, data) VALUES ('C', 'agents', '{"id":"C"}')`)
	execConn(t, out, `UPDATE resources SET data = '{"id":"B","v":"outsider"}' WHERE id = 'B'`)
	execConn(t, out, `UPDATE resources SET synced_at = '2026-10-24T12:00:00Z' WHERE resource_type = 'agents'`)
	if err := walker.Close(); err != nil {
		t.Fatal(err)
	}

	counts, err := pre.CaptureFleetWalk(walk, time.Now(), []string{"agents"})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if counts["agents"] != 1 {
		t.Fatalf("counts = %v, want agents: 1", counts)
	}
	times, _ := pre.FleetSnapshotTimes("agents")
	if len(times) != 1 {
		t.Fatalf("captures = %v, want 1", times)
	}
	if ids := captureIDs(t, pre, "agents", times[0]); strings.Join(ids, ",") != "B" {
		t.Fatalf("capture ids = %v, want exactly [B] (A predates the walk, C is another writer's)", ids)
	}
	rows, _ := pre.FleetSnapshotRowsByID("agents", times[0])
	if !strings.Contains(string(rows["B"]), `"walk"`) {
		t.Fatalf("capture data for B = %s, want the data the walk wrote", rows["B"])
	}
	var walkID sql.NullInt64
	if err := pre.DB().QueryRow(`SELECT walk_id FROM fleet_capture_log WHERE entity = 'agents'`).Scan(&walkID); err != nil || !walkID.Valid || walkID.Int64 != walk.ID() {
		t.Fatalf("capture log walk_id = %v (err %v), want %d", walkID, err, walk.ID())
	}
	for _, table := range []string{"fleet_walks", "fleet_walk_rows", "fleet_walk_state"} {
		var n int
		if err := pre.DB().QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil || n != 0 {
			t.Fatalf("%s holds %d rows after the capture (err %v), want 0", table, n, err)
		}
	}
}

// TestFleetWalkCapturesOnlyWhatItStartedAndFinished: a walk that never marked
// the attempt started (a pre-walk checkpoint write only) or never finished it
// is not fleet state, and records nothing without an error.
func TestFleetWalkCapturesOnlyWhatItStartedAndFinished(t *testing.T) {
	for _, tc := range []struct {
		name  string
		state func(t *testing.T, s *Store)
	}{
		{"never_finished", func(t *testing.T, s *Store) {
			if err := s.SaveSyncProgress("agents", "cursor", 1); err != nil {
				t.Fatal(err)
			}
		}},
		{"never_started", func(t *testing.T, s *Store) {
			// A --full checkpoint clear writes complete=1 before any request.
			if err := s.SaveSyncState("agents", "", 0); err != nil {
				t.Fatal(err)
			}
		}},
		{"finished_then_restarted", func(t *testing.T, s *Store) {
			walkCompletes(t, s, "agents")
			if err := s.SaveSyncProgress("agents", "", 0); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "s.db")
			pre := openWalkTestStore(t, path)
			walk, err := pre.BeginFleetWalk()
			if err != nil {
				t.Fatal(err)
			}
			walker := openWalkTestStore(t, path)
			if err := walker.Upsert("agents", "B", []byte(`{"id":"B"}`)); err != nil {
				t.Fatal(err)
			}
			tc.state(t, walker)
			_ = walker.Close()
			counts, err := pre.CaptureFleetWalk(walk, time.Now(), []string{"agents"})
			if err != nil {
				t.Fatalf("capture: %v", err)
			}
			if times, _ := pre.FleetSnapshotTimes("agents"); len(times) != 0 || len(counts) != 0 {
				t.Fatalf("captures = %v counts = %v, want none for an unfinished walk", times, counts)
			}
		})
	}
}

// TestFleetWalkFailsClosedWithoutProof: when the walk cannot prove its record
// is complete, nothing is recorded and the error says so.
func TestFleetWalkFailsClosedWithoutProof(t *testing.T) {
	for _, tc := range []struct {
		name string
		walk func(t *testing.T, path string, pre *Store, out *sql.Conn, walk *FleetWalk)
	}{
		{"no_connection_recorded", func(t *testing.T, path string, pre *Store, out *sql.Conn, walk *FleetWalk) {
			// The walk's writes go through a connection that predates the walk.
			execConn(t, out, `INSERT INTO resources (id, resource_type, data) VALUES ('B', 'agents', '{"id":"B"}')`)
			execConn(t, out, `INSERT INTO sync_state (resource_type, last_attempt_complete) VALUES ('agents', 0)`)
			execConn(t, out, `UPDATE sync_state SET last_attempt_complete = 1 WHERE resource_type = 'agents'`)
		}},
		{"schema_changed_mid_walk", func(t *testing.T, path string, pre *Store, out *sql.Conn, walk *FleetWalk) {
			walker := openWalkTestStore(t, path)
			if err := walker.Upsert("agents", "B", []byte(`{"id":"B"}`)); err != nil {
				t.Fatal(err)
			}
			walkCompletes(t, walker, "agents")
			_ = walker.Close()
			execConn(t, out, `CREATE TABLE unrelated_rebuild (x INTEGER)`)
		}},
		{"walk_no_longer_registered", func(t *testing.T, path string, pre *Store, out *sql.Conn, walk *FleetWalk) {
			walker := openWalkTestStore(t, path)
			if err := walker.Upsert("agents", "B", []byte(`{"id":"B"}`)); err != nil {
				t.Fatal(err)
			}
			walkCompletes(t, walker, "agents")
			_ = walker.Close()
			execConn(t, out, `DELETE FROM fleet_walks`)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "s.db")
			pre := openWalkTestStore(t, path)
			out := outsiderConn(t, pre)
			walk, err := pre.BeginFleetWalk()
			if err != nil {
				t.Fatal(err)
			}
			tc.walk(t, path, pre, out, walk)
			_, err = pre.CaptureFleetWalk(walk, time.Now(), []string{"agents"})
			if !errors.Is(err, ErrFleetWalkUnproven) {
				t.Fatalf("capture err = %v, want ErrFleetWalkUnproven", err)
			}
			if times, _ := pre.FleetSnapshotTimes("agents"); len(times) != 0 {
				t.Fatalf("an unproven walk recorded captures %v", times)
			}
		})
	}
}

// TestFleetWalkIsOnePerStorePerProcess: a second walk of the same store in
// the same process cannot share the recorder, so it is refused, not merged.
func TestFleetWalkIsOnePerStorePerProcess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.db")
	s := openWalkTestStore(t, path)
	first, err := s.BeginFleetWalk()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.BeginFleetWalk(); err == nil {
		t.Fatal("a second concurrent walk of one store was armed")
	}
	var n int
	if err := s.DB().QueryRow(`SELECT COUNT(*) FROM fleet_walks`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("fleet_walks = %d (err %v), want only the first walk registered", n, err)
	}
	if err := s.AbandonFleetWalk(first); err != nil {
		t.Fatal(err)
	}
	second, err := s.BeginFleetWalk()
	if err != nil {
		t.Fatalf("a walk after the first was abandoned: %v", err)
	}
	if second.ID() <= first.ID() {
		t.Fatalf("walk ids %d then %d, want strictly increasing", first.ID(), second.ID())
	}
	second.Stop()
}

// TestWalkCapturesNeverShareOrReorderAStamp: two walk captures in one second
// are two captures, and a capture taken after the clock stepped back still
// sorts after the earlier one.
func TestWalkCapturesNeverShareOrReorderAStamp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.db")
	s := openWalkTestStore(t, path)
	at := time.Date(2026, 10, 24, 12, 0, 0, 100_000_000, time.UTC)
	for i, capturedAt := range []time.Time{at, at.Add(200 * time.Millisecond), at.Add(-time.Hour)} {
		walk, err := s.BeginFleetWalk()
		if err != nil {
			t.Fatal(err)
		}
		walker := openWalkTestStore(t, path)
		if err := walker.Upsert("agents", "A", []byte(`{"id":"A"}`)); err != nil {
			t.Fatal(err)
		}
		walkCompletes(t, walker, "agents")
		_ = walker.Close()
		if _, err := s.CaptureFleetWalk(walk, capturedAt, []string{"agents"}); err != nil {
			t.Fatalf("capture %d: %v", i, err)
		}
	}
	times, _ := s.FleetSnapshotTimes("agents")
	want := []time.Time{at.Truncate(time.Second), at.Truncate(time.Second).Add(time.Second), at.Truncate(time.Second).Add(2 * time.Second)}
	if len(times) != len(want) {
		t.Fatalf("captures = %v, want %v", times, want)
	}
	for i := range want {
		if !times[i].Equal(want[i]) {
			t.Fatalf("captures = %v, want %v", times, want)
		}
	}
}

// TestStoreFromBeforeWalkTrackingStillOpens: a store written by the previous
// release (fleet_capture_log without walk_id, no walk tables) opens, its
// captures read exactly as before with a NULL walk id, and a new walk capture
// lands after them.
func TestStoreFromBeforeWalkTrackingStillOpens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.db")
	s, err := OpenWithContext(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	legacy := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	for _, q := range []string{
		`DROP TABLE fleet_walk_rows`, `DROP TABLE fleet_walk_state`, `DROP TABLE fleet_walks`,
		`DROP TABLE fleet_capture_log`,
		`CREATE TABLE fleet_capture_log (
			entity      TEXT NOT NULL,
			captured_at TEXT NOT NULL,
			row_count   INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (entity, captured_at)
		)`,
	} {
		if _, err := s.DB().Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if _, err := s.DB().Exec(`INSERT INTO fleet_capture_log (entity, captured_at, row_count) VALUES ('agents', ?, 1)`, legacy.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().Exec(`INSERT INTO fleet_snapshots (captured_at, entity, entity_id, data) VALUES (?, 'agents', 'A', '{"id":"A"}')`, legacy.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()

	re := openWalkTestStore(t, path)
	times, err := re.FleetSnapshotTimes("agents")
	if err != nil || len(times) != 1 || !times[0].Equal(legacy) {
		t.Fatalf("legacy captures = %v (err %v), want [%v]", times, err, legacy)
	}
	if ids := captureIDs(t, re, "agents", legacy); strings.Join(ids, ",") != "A" {
		t.Fatalf("legacy capture ids = %v, want [A]", ids)
	}
	var walkID sql.NullInt64
	if err := re.DB().QueryRow(`SELECT walk_id FROM fleet_capture_log WHERE captured_at = ?`, legacy.Format(time.RFC3339)).Scan(&walkID); err != nil || walkID.Valid {
		t.Fatalf("legacy capture walk_id = %v (err %v), want NULL", walkID, err)
	}

	walk, err := re.BeginFleetWalk()
	if err != nil {
		t.Fatal(err)
	}
	walker := openWalkTestStore(t, path)
	if err := walker.Upsert("agents", "B", []byte(`{"id":"B"}`)); err != nil {
		t.Fatal(err)
	}
	walkCompletes(t, walker, "agents")
	_ = walker.Close()
	if _, err := re.CaptureFleetWalk(walk, time.Now(), []string{"agents"}); err != nil {
		t.Fatal(err)
	}
	times, _ = re.FleetSnapshotTimes("agents")
	if len(times) != 2 || !times[1].After(times[0]) {
		t.Fatalf("captures = %v, want the legacy one then the walk's", times)
	}
	if ids := captureIDs(t, re, "agents", times[1]); strings.Join(ids, ",") != "B" {
		t.Fatalf("walk capture ids = %v, want [B]", ids)
	}
}
