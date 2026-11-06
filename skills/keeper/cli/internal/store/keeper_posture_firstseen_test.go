// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"context"
	"path/filepath"
	"testing"
)

// Codex r1 class wrong-invite-age: an email deleted and re-invited under a
// new enterprise user id starts a new invite; FirstSeen is the start of the
// current unbroken streak of good runs with the same user id and status.
func TestFirstSeenIsTheCurrentIncarnationAndStatus(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	put := func(ok bool, facts map[string]string) int64 {
		t.Helper()
		id, err := db.BeginPostureRun(ctx, 90)
		if err != nil {
			t.Fatal(err)
		}
		var users []UserPosture
		for email, f := range facts {
			users = append(users, UserPosture{CompanyID: "4421", Email: email, Facts: []byte(f)})
		}
		if err := db.PutCompanyPosture(ctx, CompanyPosture{RunID: id, CompanyID: "4421", OK: ok, Metrics: []byte(`{}`)}, users); err != nil {
			t.Fatal(err)
		}
		return id
	}
	put(true, map[string]string{"u@example.com": `{"user_id":"7","status":"Active"}`, "inv@example.com": `{"user_id":"3","status":"Invited"}`})
	r2 := put(true, map[string]string{"u@example.com": `{"user_id":"8","status":"Invited"}`, "inv@example.com": `{"user_id":"3","status":"Invited"}`})
	put(false, nil) // a failed run reads nothing and does not break a streak
	put(true, map[string]string{"u@example.com": `{"user_id":"8","status":"Invited"}`, "inv@example.com": `{"user_id":"3","status":"Invited"}`})
	runs, err := db.PostureRuns(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	started := map[int64]string{}
	for _, r := range runs {
		started[r.ID] = r.StartedAt
	}
	first, err := db.FirstSeen(ctx, "4421")
	if err != nil {
		t.Fatal(err)
	}
	if first["u@example.com"] != started[r2] {
		t.Errorf("re-invited user: first seen %q, want run %d's %q", first["u@example.com"], r2, started[r2])
	}
	if first["inv@example.com"] != started[1] {
		t.Errorf("long-standing invite: first seen %q, want run 1's %q", first["inv@example.com"], started[1])
	}
}
