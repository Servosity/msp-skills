// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored. Kept in its own file so `generate --force` preserves it.
//
// Regression cover for the zero-watermark sentinel being read as a timestamp.
//
// A resource that has never completed a full sync pass stores the zero time
// rather than NULL: SaveSyncProgress writes a parseable zero so GetSyncState
// can scan without a NULL conversion error, and the watermark only advances
// when sync can prove it saw the whole result set. Filtering on
// `last_synced_at IS NOT NULL` left that sentinel in play, ORDER BY picked it
// as the oldest row every time, and time.Since(zero) saturates time.Duration
// at its 292-year maximum. The visible symptom was every offline command
// printing "local store data is 2562047h47m0s old" against data written
// seconds earlier, and doctor reporting status "stale" on a fresh mirror.

package cli

import (
	"path/filepath"
	"testing"
	"time"

	"immybot-pp-cli/internal/store"
)

func seedSyncState(t *testing.T, rows []struct {
	resource string
	at       string
	count    int
},
) *store.Store {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "immy.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	for _, r := range rows {
		if _, err := db.DB().Exec(
			`INSERT INTO sync_state (resource_type, last_cursor, last_synced_at, total_count) VALUES (?, '', ?, ?)`,
			r.resource, r.at, r.count,
		); err != nil {
			t.Fatalf("seed %s: %v", r.resource, err)
		}
	}
	return db
}

type syncRow = struct {
	resource string
	at       string
	count    int
}

const zeroWatermark = "0001-01-01T00:00:00Z"

// A resource stuck on the sentinel must never be reported as very old data.
func TestSyncHintTreatsZeroWatermarkAsNeverCompleted(t *testing.T) {
	db := seedSyncState(t, []syncRow{{"audits", zeroWatermark, 20581}})

	state, err := readSyncHintState(db, "audits")
	if err != nil {
		t.Fatalf("readSyncHintState: %v", err)
	}
	if state.hasState {
		t.Error("the zero sentinel must not count as a real watermark")
	}
	if !state.lastSynced.IsZero() {
		t.Errorf("no watermark should be returned, got %v", state.lastSynced)
	}
	// Rows landed, so this is a truncated run rather than an empty store.
	if !state.partial {
		t.Error("rows were stored without a completed pass, so partial should be set")
	}
	// The bug in one assertion: the age this would have produced.
	if age := time.Since(state.lastSynced); state.hasState && age > 100*365*24*time.Hour {
		t.Errorf("sentinel leaked into age arithmetic: %v", age)
	}
}

// With a real watermark present, the sentinel must not win the ORDER BY.
func TestSyncHintPrefersRealWatermarkOverSentinel(t *testing.T) {
	real := time.Now().Add(-90 * time.Minute).UTC().Format(time.RFC3339)
	db := seedSyncState(t, []syncRow{
		{"audits", zeroWatermark, 20581},
		{"computers", real, 57},
	})

	state, err := readSyncHintState(db, "")
	if err != nil {
		t.Fatalf("readSyncHintState: %v", err)
	}
	if !state.hasState {
		t.Fatal("a real watermark exists, so fleet-wide state should be reported")
	}
	if age := time.Since(state.lastSynced); age > 24*time.Hour {
		t.Errorf("sentinel won the ordering; age %v should be about 90m", age)
	}
}

// An empty store is a different condition to a truncated one.
func TestSyncHintEmptyStoreIsNotPartial(t *testing.T) {
	db := seedSyncState(t, nil)

	state, err := readSyncHintState(db, "audits")
	if err != nil {
		t.Fatalf("readSyncHintState: %v", err)
	}
	if state.hasState {
		t.Error("an empty store has no watermark")
	}
	if state.partial {
		t.Error("nothing was stored, so this is not partial data")
	}
}
