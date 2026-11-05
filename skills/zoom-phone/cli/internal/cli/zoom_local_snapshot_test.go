// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Codex round-9: each local command reads one snapshot, and a repeated
// pagination cursor is a fetch failure.

package cli

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"zoom-phone-pp-cli/internal/store"
)

// localInterleaveWriter arranges for a concurrent writer to delete u-bob
// right after the first scan of afterResource. It reports whether the write
// committed while the command was still reading (it must not: a read
// snapshot holds it off), and waits for it to land afterwards.
func localInterleaveWriter(t *testing.T, path, afterResource string) (committedDuringRead func() bool, waitLanded func()) {
	t.Helper()
	writer, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = writer.Close() })
	landed := make(chan error, 1)
	var early bool
	var once sync.Once
	localSnapshotHook = func(rt string) {
		if rt != afterResource {
			return
		}
		once.Do(func() {
			done := make(chan struct{})
			go func() {
				_, err := writer.DB().Exec(`DELETE FROM resources WHERE resource_type = 'users' AND id = 'u-bob'`)
				landed <- err
				close(done)
			}()
			select {
			case <-done:
				early = true
			case <-time.After(700 * time.Millisecond):
			}
		})
	}
	t.Cleanup(func() { localSnapshotHook = nil })
	return func() bool { return early }, func() {
		localSnapshotHook = nil
		select {
		case err := <-landed:
			if err != nil {
				t.Fatalf("concurrent writer failed: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("concurrent writer never landed")
		}
	}
}

func localInventoryInactive(t *testing.T, path string) []string {
	t.Helper()
	stdout, _, err := localRun(t, newNovelNumbersInventoryCmd, "--db", path)
	if err != nil {
		t.Fatalf("inventory: %v", err)
	}
	var res localInventoryResult
	localDecode(t, stdout, &res)
	ids := []string{}
	for _, n := range res.InactiveAssignee {
		ids = append(ids, n.ID+":"+n.Reason)
	}
	return ids
}

func TestLocalInventoryReadsOneSnapshot(t *testing.T) {
	localTestEnv(t)
	path := localTestStore(t, map[string][]string{"numbers": localTestNumbers, "users": localTestUsers})
	committedDuringRead, waitLanded := localInterleaveWriter(t, path, "numbers")

	got := strings.Join(localInventoryInactive(t, path), ",")
	if committedDuringRead() {
		t.Fatalf("a concurrent write committed between the inventory's queries")
	}
	// All-old: Bob is still a phone user in the snapshot, so his number is
	// not called orphaned.
	if strings.Contains(got, "pn-bob") {
		t.Fatalf("old numbers joined against new users: inactive=%s", got)
	}
	waitLanded()
	// All-new afterwards: Bob is gone, his number is orphaned.
	if got := strings.Join(localInventoryInactive(t, path), ","); !strings.Contains(got, "pn-bob:not_in_phone_users") {
		t.Fatalf("after the write landed: inactive=%s", got)
	}
}

func TestLocalDevicesHealthReadsOneSnapshot(t *testing.T) {
	localTestEnv(t)
	path := localTestStore(t, map[string][]string{
		"assigned": localTestAssignedDevices, "unassigned": localTestUnassignedDevices, "users": localTestUsers, "common_areas": {},
	})
	committedDuringRead, waitLanded := localInterleaveWriter(t, path, "assigned")
	stdout, _, err := localRun(t, newNovelDevicesHealthCmd, "--db", path)
	if err != nil {
		t.Fatalf("devices health: %v", err)
	}
	var res localDevicesHealthResult
	localDecode(t, stdout, &res)
	if committedDuringRead() {
		t.Fatalf("a concurrent write committed between the health check's queries")
	}
	for _, sh := range res.BySite {
		for _, f := range sh.Findings {
			if f.DeviceID == "dev-bob" {
				t.Fatalf("old devices joined against new users: dev-bob flagged %v", f.Flags)
			}
		}
	}
	waitLanded()
}

// ---- item 2: repeated pagination cursors ----

type localFakePager struct {
	next  func(token string) string
	calls int
}

func (f *localFakePager) Get(_ context.Context, _ string, params map[string]string) (json.RawMessage, error) {
	f.calls++
	return json.RawMessage(`{"items":[{"id":"x"}],"next_page_token":"` + f.next(params["next_page_token"]) + `"}`), nil
}

func TestLocalFetchPagedRejectsRepeatedCursor(t *testing.T) {
	cases := []struct {
		name      string
		next      func(string) string
		wantErr   bool
		wantCalls int
	}{
		{"same token returned again", func(string) string { return "same" }, true, 2},
		{"two-token cycle", func(tok string) string {
			return map[string]string{"": "a", "a": "b", "b": "a"}[tok]
		}, true, 3},
		{"natural end", func(tok string) string { return map[string]string{"": "a", "a": ""}[tok] }, false, 2},
	}
	for _, tc := range cases {
		f := &localFakePager{next: tc.next}
		items, capped, err := localFetchPaged(context.Background(), f, "/phone/x", nil, "items", 100)
		if (err != nil) != tc.wantErr || capped || f.calls != tc.wantCalls {
			t.Errorf("%s: err=%v capped=%v calls=%d items=%d, want err=%v calls=%d", tc.name, err, capped, f.calls, len(items), tc.wantErr, tc.wantCalls)
		}
		if tc.wantErr && (err == nil || !strings.Contains(err.Error(), "again")) {
			t.Errorf("%s: error %v does not name the repeated cursor", tc.name, err)
		}
	}
}

func TestLocalOffboardRefusesRepeatedMemberCursor(t *testing.T) {
	localTestEnv(t)
	ts := localNewTestServer(t)
	// Every page of Billing's member list points back at the same cursor.
	ts.getBody["/phone/call_queues/cq-billing/members"] = `{"call_queue_members":[{"id":"u-bob","level":"user","extension_id":"ext-bob"}],"next_page_token":"tok-1"}`
	db := localTestStore(t, localTestFootprintRows())
	stdout, _, err := localRun(t, newNovelOffboardCmd, "jane@example.com", "--db", db, "--apply")
	if code := localExitCode(err); code != 5 {
		t.Fatalf("exit = %d (%v), want 5", code, err)
	}
	var res localOffboardResult
	localDecode(t, stdout, &res)
	found := false
	for _, f := range res.FetchFailures {
		if f.GroupID == "cq-billing" && strings.Contains(f.Error, "again") {
			found = true
		}
	}
	if !found || !res.Refused {
		t.Fatalf("repeated cursor not recorded as a fetch failure: refused=%v failures=%+v", res.Refused, res.FetchFailures)
	}
	if _, deletes := ts.snapshot(); len(deletes) != 0 {
		t.Fatalf("DELETEs sent despite an unwalkable member list: %v", deletes)
	}
}
