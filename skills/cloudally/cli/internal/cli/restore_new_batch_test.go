// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

// claimRow is the guard's record of one claim, for before/after checks.
type claimRow struct {
	ID         int64
	State      string
	BatchID    string
	Generation int
	Superseded int
}

func claimRowsFor(t *testing.T, entity string) []claimRow {
	t.Helper()
	g, err := openRestoreGuard(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	rows, err := g.db.Query(`SELECT id, state, batch_id, generation, superseded FROM restore_claims WHERE entity = ? ORDER BY id`, strings.ToLower(entity))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []claimRow
	for rows.Next() {
		var c claimRow
		if err := rows.Scan(&c.ID, &c.State, &c.BatchID, &c.Generation, &c.Superseded); err != nil {
			t.Fatal(err)
		}
		out = append(out, c)
	}
	return out
}

// wantOriginalBack checks that the first claim is live again exactly as it
// was and that every later one is the refused --new-batch attempt.
func wantOriginalBack(t *testing.T, before claimRow, after []claimRow) {
	t.Helper()
	if len(after) != 2 {
		t.Fatalf("claims for a = %+v, want the original and the refused replacement", after)
	}
	if after[0] != before {
		t.Fatalf("original claim = %+v, want it back exactly as %+v", after[0], before)
	}
	if after[1].State != claimRejected {
		t.Fatalf("replacement claim = %+v, want REJECTED", after[1])
	}
}

// A --new-batch resend refused before it ran (401) puts the original
// claim back, so the combined A+B run sends no restore for A; B, never
// sent, goes out through the command the refusal prints.
func TestRestoreRefusedNewBatchKeepsOriginalProtection(t *testing.T) {
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, 0)
	if _, stderr, err := runAuthCmd(t, "", submitArgs("--date", "2026-10-01", "--entities", "a@contoso.com")...); err != nil {
		t.Fatalf("restore A: %v (%s)", err, stderr)
	}
	original := claimRowsFor(t, "a@contoso.com")[0]

	api.setStatus("a@contoso.com", http.StatusUnauthorized)
	if _, _, err := runAuthCmd(t, "", submitArgs("--date", "2026-10-01", "--entities", "a@contoso.com", "--new-batch")...); err == nil {
		t.Fatal("the --new-batch resend should report its 401")
	}

	api.setStatus("a@contoso.com", http.StatusOK)
	_, _, err := runAuthCmd(t, "", submitArgs("--date", "2026-10-01", "--entities", "a@contoso.com,b@contoso.com")...)
	if got := api.entityPosts("a@contoso.com"); got != 2 {
		t.Fatalf("A posts = %d, want 2 (restored once, then the refused 401): A was restored twice", got)
	}
	wantOriginalBack(t, original, claimRowsFor(t, "a@contoso.com"))
	wantGuardRefusal(t, "A and B after a refused --new-batch", err)
	if _, stderr, err := runAuthCmd(t, "", printedCommand(t, err.Error(), "with none of these restores sent: ")...); err != nil {
		t.Fatalf("printed command for B: %v (%s)", err, stderr)
	}
	if api.entityPosts("b@contoso.com") != 1 || api.entityPosts("a@contoso.com") != 2 {
		t.Fatalf("after the printed command: %v", api.byEntity)
	}
}

// The never-sent path: a --new-batch resend whose POST never connects
// (nothing left this process) is REJECTED and puts the original back too.
func TestRestoreNeverSentNewBatchKeepsOriginalProtection(t *testing.T) {
	isolateAuthState(t)
	api := newFakeRestoreAPI(t, 0)
	if _, stderr, err := runAuthCmd(t, "", submitArgs("--date", "2026-10-01", "--entities", "a@contoso.com")...); err != nil {
		t.Fatalf("restore A: %v (%s)", err, stderr)
	}
	original := claimRowsFor(t, "a@contoso.com")[0]

	planPath := filepath.Join(t.TempDir(), "plan.json")
	plan := restorePlan{Scope: scopeAccount, TaskID: "64b0e9", Source: "MS365", Dates: []string{"2026-10-01"},
		Rows: []planRow{{Entity: "a@contoso.com", Requested: "2026-10-01", ResolvedRaw: fakeBackupRaw, Status: planOK}}}
	if err := writePlanFile(planPath, plan); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	dead := "http://" + ln.Addr().String()
	_ = ln.Close()
	t.Setenv("CLOUDALLY_BASE_URL", dead)
	if _, _, err := runAuthCmd(t, "", "restore", "submit", "--plan-file", planPath, "--new-batch", "--rate", "1000", "--yes", "--json"); err == nil {
		t.Fatal("a resend that never connected should not succeed")
	}

	t.Setenv("CLOUDALLY_BASE_URL", api.srv.URL)
	_, _, err = runAuthCmd(t, "", submitArgs("--date", "2026-10-01", "--entities", "a@contoso.com,b@contoso.com")...)
	if got := api.entityPosts("a@contoso.com"); got != 1 {
		t.Fatalf("A posts = %d, want 1: A was restored twice", got)
	}
	wantOriginalBack(t, original, claimRowsFor(t, "a@contoso.com"))
	wantGuardRefusal(t, "A and B after a never-sent --new-batch", err)
}

// At the guard: a REJECTED replacement puts the superseded claim back
// exactly; an UNKNOWN or ACCEPTED replacement holds the restore itself and
// puts nothing back.
func TestRestoreGuardSettleNewBatchOutcomes(t *testing.T) {
	isolateAuthState(t)
	ctx := context.Background()
	g, err := openRestoreGuard(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	for _, tc := range []struct {
		answer       string
		originalBack bool
		replacement  string
	}{
		{jobFailed, true, claimRejected},
		{jobUnknown, false, claimUnknown},
		{jobSubmitted, false, claimAccepted},
	} {
		entity := "x-" + strings.ToLower(tc.answer) + "@contoso.com"
		id := newRestoreIdentity("64b0e9", entity, fakeBackupRaw)
		first, _, err := g.claim(ctx, id, claimOwner{batchID: "b1"}, false, false)
		if err != nil {
			t.Fatal(err)
		}
		if err := g.settle(ctx, first, jobSubmitted, nil); err != nil {
			t.Fatal(err)
		}
		second, superseded, err := g.claim(ctx, id, claimOwner{batchID: "b2"}, false, true)
		if err != nil || len(superseded) != 1 || superseded[0] != first {
			t.Fatalf("%s: --new-batch claim superseded %v (%v), want [%d]", tc.answer, superseded, err, first)
		}
		if err := g.settle(ctx, second, tc.answer, superseded); err != nil {
			t.Fatal(err)
		}
		rows := claimRowsFor(t, entity)
		if back := rows[0].Superseded == 0; back != tc.originalBack || rows[0].State != claimAccepted || rows[0].BatchID != "b1" {
			t.Fatalf("%s: original = %+v, want superseded=%v with state and batch unchanged", tc.answer, rows[0], !tc.originalBack)
		}
		if rows[1].State != tc.replacement {
			t.Fatalf("%s: replacement = %+v, want %s", tc.answer, rows[1], tc.replacement)
		}
		live, err := g.conflicts(ctx, []restoreIdentity{id}, claimOwner{}, false, false)
		if err != nil || len(live) != 1 {
			t.Fatalf("%s: the restore should stay held, conflicts %v (%v)", tc.answer, live, err)
		}
	}
}
