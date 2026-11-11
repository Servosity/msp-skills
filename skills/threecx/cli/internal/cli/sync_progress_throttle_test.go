// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Hand-fix (handfixes.json sync-progress-throttled): tools/maintainer/
// replay_handfixes.py writes this file from
// handfix_replay/sync_progress_throttle_test.go.tmpl after a reprint.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"threecx-pp-cli/internal/store"
)

func TestSyncProgressDueOncePerInterval(t *testing.T) {
	saved := syncProgressEvery
	syncProgressEvery = time.Hour
	defer func() { syncProgressEvery = saved }()
	var last time.Time
	if !syncProgressDue(&last) {
		t.Fatal("the first page of a resource wrote no progress line")
	}
	if syncProgressDue(&last) {
		t.Fatal("a second page inside the interval wrote a progress line")
	}
	last = last.Add(-2 * time.Hour)
	if !syncProgressDue(&last) {
		t.Fatal("a page after the interval wrote no progress line")
	}
}

// syncThrottleFake serves three full pages of fresh rows, then an empty one. A
// page is as long as the largest number the request carries (its page size),
// so a page or offset walk sees each page as full and asks for the next.
type syncThrottleFake struct{ calls int }

func (f *syncThrottleFake) reply(params map[string]string) json.RawMessage {
	f.calls++
	size := 2
	for _, v := range params {
		if n, err := strconv.Atoi(v); err == nil && n > size && n <= 500 {
			size = n
		}
	}
	if f.calls > 3 {
		return json.RawMessage(`[]`)
	}
	rows := make([]string, size)
	for i := range rows {
		id := (f.calls-1)*size + i + 1
		rows[i] = fmt.Sprintf(`{"id":"%d","name":"Acme Corp %d"}`, id, id)
	}
	return json.RawMessage("[" + strings.Join(rows, ",") + "]")
}

func (f *syncThrottleFake) Get(_ context.Context, _ string, params map[string]string) (json.RawMessage, error) {
	return f.reply(params), nil
}

func (f *syncThrottleFake) PostWithParams(_ context.Context, _ string, params map[string]string, _ any) (json.RawMessage, int, error) {
	return f.reply(params), 200, nil
}

func (f *syncThrottleFake) PostQueryWithParams(_ context.Context, _ string, params map[string]string, _ any) (json.RawMessage, int, error) {
	return f.reply(params), 200, nil
}

func (f *syncThrottleFake) RateLimit() float64 { return 0 }

// syncProgressLines walks one resource in a fresh store and counts the
// per-page sync_progress lines it wrote and the total sync_complete reported.
func syncProgressLines(t *testing.T, resource string) (lines int, total int64, stored int) {
	t.Helper()
	db, err := store.OpenWithContext(context.Background(), filepath.Join(t.TempDir(), "sync.db"))
	if err != nil {
		t.Fatalf("opening the local store: %v", err)
	}
	defer db.Close()
	var events bytes.Buffer
	res := syncResource(context.Background(), &syncThrottleFake{}, db, resource, "", false, 0, false, false, &syncUserParams{}, &events)
	total = -1
	for _, line := range strings.Split(events.String(), "\n") {
		var event struct {
			Event   string `json:"event"`
			Fetched *int64 `json:"fetched"`
			Total   int64  `json:"total"`
		}
		if json.Unmarshal([]byte(line), &event) != nil {
			continue
		}
		switch {
		case event.Event == "sync_progress" && event.Fetched != nil:
			lines++
		case event.Event == "sync_complete":
			total = event.Total
		}
	}
	return lines, total, res.Count
}

// TestSyncWritesProgressAtMostOncePerInterval finds a default resource that
// walks several pages from the fake, then walks it again inside one interval:
// only its first page may write a progress line, and sync_complete still
// carries the final count.
func TestSyncWritesProgressAtMostOncePerInterval(t *testing.T) {
	savedHuman, savedEvery := humanFriendly, syncProgressEvery
	humanFriendly = false
	defer func() { humanFriendly, syncProgressEvery = savedHuman, savedEvery }()
	for _, resource := range defaultSyncResources() {
		syncProgressEvery = 0
		perPage, _, _ := syncProgressLines(t, resource)
		if perPage < 2 {
			continue // one page from the fake: nothing to throttle
		}
		syncProgressEvery = time.Hour
		lines, total, stored := syncProgressLines(t, resource)
		if lines != 1 {
			t.Fatalf("%s: %d pages inside one interval wrote %d progress lines, want 1 (the first page)", resource, perPage, lines)
		}
		if total != int64(stored) {
			t.Fatalf("%s: sync_complete total = %d, want the %d rows stored", resource, total, stored)
		}
		return
	}
	t.Skip("no default resource walked a second page from the fake, so there was nothing to throttle")
}
