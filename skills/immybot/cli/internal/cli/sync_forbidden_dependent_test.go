// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Hand-fix (handfixes.json sync-forbidden-not-partial): tools/maintainer/
// replay_handfixes.py writes this file from
// handfix_replay/sync_forbidden_dependent_test.go.tmpl after a reprint.

import (
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"immybot-pp-cli/internal/client"
	"immybot-pp-cli/internal/store"
)

// syncParentStatusFake answers each parent's request with that parent's HTTP status.
type syncParentStatusFake struct {
	status map[string]int
	seen   map[string]bool
}

func (f *syncParentStatusFake) reply(path string, params map[string]string) error {
	for parent, status := range f.status {
		hit := strings.Contains(path, parent)
		for _, v := range params {
			hit = hit || v == parent
		}
		if hit {
			f.seen[parent] = true
			return &client.APIError{StatusCode: status, Body: `{"message":"missing required parameter filter"}`}
		}
	}
	return &client.APIError{StatusCode: 500, Body: `{"message":"no parent in the request"}`}
}

func (f *syncParentStatusFake) Get(_ context.Context, path string, params map[string]string) (json.RawMessage, error) {
	return nil, f.reply(path, params)
}

func (f *syncParentStatusFake) PostWithParams(_ context.Context, path string, params map[string]string, _ any) (json.RawMessage, int, error) {
	return nil, 0, f.reply(path, params)
}

func (f *syncParentStatusFake) PostQueryWithParams(_ context.Context, path string, params map[string]string, _ any) (json.RawMessage, int, error) {
	return nil, 0, f.reply(path, params)
}

func (f *syncParentStatusFake) RateLimit() float64 { return 0 }

// TestSyncDependentSkipNeedsEveryParentRefused: a dependent is a forbidden
// skip only when every parent answered an outright 403. One parent's 400
// keeps it partial whichever parent reports first (Codex syncfix r1 P1).
func TestSyncDependentSkipNeedsEveryParentRefused(t *testing.T) {
	prev := humanFriendly
	humanFriendly = false
	defer func() { humanFriendly = prev }()
	const p1, p2 = "910001", "910002"
	for _, dep := range dependentResourceDefs() {
		tested := false
		for _, tc := range []struct {
			s1, s2   int
			wantSkip bool
		}{{403, 400, false}, {400, 403, false}, {403, 403, true}} {
			db, err := store.OpenWithContext(context.Background(), filepath.Join(t.TempDir(), "sync.db"))
			if err != nil {
				t.Fatalf("opening the local store: %v", err)
			}
			var parents []json.RawMessage
			for _, id := range []string{p1, p2} {
				row := map[string]string{"id": id}
				if dep.KeyField != "" {
					row[dep.KeyField] = id
				}
				for _, pp := range dep.PathParams {
					if pp.Field != "" {
						row[pp.Field] = id
					}
				}
				encoded, _ := json.Marshal(row)
				parents = append(parents, encoded)
			}
			if _, _, err := db.UpsertBatch(dep.ParentTable, parents); err != nil {
				db.Close()
				t.Fatalf("%s: seeding parents: %v", dep.Name, err)
			}
			fake := &syncParentStatusFake{status: map[string]int{p1: tc.s1, p2: tc.s2}, seen: map[string]bool{}}
			res := syncDependentResource(context.Background(), fake, db, dep, "", false, 0, false, false, &syncUserParams{}, io.Discard, 0)
			db.Close()
			if !fake.seen[p1] || !fake.seen[p2] {
				break // this dependent's parents come from elsewhere: try the next one
			}
			tested = true
			if gotSkip := res.ForbiddenKey != ""; gotSkip != tc.wantSkip {
				t.Fatalf("%s: parents answered %d then %d: ForbiddenKey=%q, want a skip=%v", dep.Name, tc.s1, tc.s2, res.ForbiddenKey, tc.wantSkip)
			}
			if res.Warn == nil {
				t.Fatalf("%s: parents answered %d then %d: no warning, want the insufficient-access warning", dep.Name, tc.s1, tc.s2)
			}
		}
		if tested {
			return
		}
	}
	t.Skip("no dependent resource fans out over a seeded parent table")
}
