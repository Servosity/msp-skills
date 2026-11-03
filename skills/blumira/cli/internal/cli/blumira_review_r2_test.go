// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Codex round-2 review reproductions (blumira handfix novel-analytics-review-fixes), one per class.

package cli

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"blumira-pp-cli/internal/cliutil/testenv"
	"blumira-pp-cli/internal/store"
)

// class: snapshot history - a sync that returns no findings is not a history point.
func TestR2EmptySyncIsAHistoryPoint(t *testing.T) {
	testenv.Isolate(t)
	api := newFakeBlumira(t)
	api.set("/org/findings", []map[string]any{})
	runRoot(t, "sync", "--resources", "org-findings")
	api.set("/org/findings", []map[string]any{findingRecord("f1", "org-1", 10, "Open")})
	runRoot(t, "sync", "--resources", "org-findings")
	if rows := jsonRows(t, runRoot(t, "drift", "--json")); len(rows) != 1 || rows[0]["change"] != "new" {
		t.Fatalf("two syncs (empty, then one open finding): drift = %+v, want f1 new", rows)
	}
}

// class: incomplete read marked complete - a truncated direct-org device walk is reported complete.
func TestR2TruncatedDirectDeviceWalkNotComplete(t *testing.T) {
	testenv.Isolate(t)
	api := newFakeBlumira(t)
	fresh := time.Now().UTC().Add(-time.Hour).Format("2006-01-02T15:04:05.000")
	page1 := make([]map[string]any, 0, 50)
	for i := 0; i < 50; i++ {
		page1 = append(page1, map[string]any{"device_id": "d" + string(rune('A'+i%26)) + string(rune('a'+i/26)), "hostname": "ws", "org_id": "org-1", "alive": fresh})
	}
	api.set("/org/agents/devices", page1, []map[string]any{{"device_id": "dc", "hostname": "dc01", "org_id": "org-1", "is_domain_controller": true, "alive": "2026-01-01T00:00:00.000"}})
	api.set("/org/findings", []map[string]any{findingRecord("f1", "org-1", 10, "Open")})
	runRoot(t, "sync", "--resources", "org-findings")
	runRoot(t, "sync", "--full", "--resources", "org-agents-devices", "--max-pages", "1")
	for _, r := range jsonRows(t, runRoot(t, "overview", "--json")) {
		if r["account"] == "org-1" && r["device_coverage"] == "complete" {
			t.Fatalf("a device walk cut off at --max-pages 1 is reported complete: %+v", r)
		}
	}
}

// class: ownership read - two documented owners with the same name and different ids collapse.
func TestR2SameNameOwnersStayDistinct(t *testing.T) {
	owners, known := decodeOwnership(map[string]any{"owners": map[string]any{"analysts": []any{
		map[string]any{"id": "00000000-0000-4000-8000-000000000001", "first_name": "Alex", "last_name": "Smith"},
		map[string]any{"id": "00000000-0000-4000-8000-000000000002", "first_name": "Alex", "last_name": "Smith"},
	}}})
	if !known || len(owners) != 2 {
		t.Fatalf("two distinct analysts (different ids) decoded as %v (known=%v)", owners, known)
	}
}

// class: limit before de-duplication - the evidence/raw merge drops a matching
// finding. Which finding the raw scan reaches first depends on store order, so
// the evidence match is tried on each finding in turn.
func TestR2EvidenceAndRawMergeKeepsLimit(t *testing.T) {
	for _, withEvidence := range []string{"f1", "f2"} {
		t.Run(withEvidence, func(t *testing.T) {
			testenv.Isolate(t)
			api := newFakeBlumira(t)
			f1 := findingRecord("f1", "org-1", 10, "Open")
			f1["analysis"] = "traffic from 10.0.4.21"
			f2 := findingRecord("f2", "org-1", 10, "Open")
			f2["analysis"] = "traffic from 10.0.4.21"
			api.set("/org/findings", []map[string]any{f1, f2})
			for _, id := range []string{"f1", "f2"} {
				if id == withEvidence {
					api.set("/org/findings/"+id+"/evidence", []map[string]any{{"user": "a"}}, []map[string]any{{"src_ip": "10.0.4.21"}})
				} else {
					api.set("/org/findings/"+id+"/evidence", []map[string]any{{"user": "b"}})
				}
			}
			runRoot(t, "sync", "--resources", "org-findings")
			out := runRoot(t, "evidence-search", "10.0.4.21", "--fetch", "--max-fetch", "2", "--limit", "2", "--json")
			var view struct {
				Matches []map[string]any `json:"matches"`
			}
			if err := json.Unmarshal(out, &view); err != nil {
				t.Fatal(err)
			}
			if len(view.Matches) != 2 {
				t.Fatalf("two findings match (%s in evidence and payload, the other in payload) with --limit 2; got %s", withEvidence, out)
			}
		})
	}
}

// class: incomplete read marked complete - sync reads only the first page.
// Blumira pages by page_size (documented default 50) and answers links.next
// and meta.total_items; a 50-row first page of 51 devices is not the end.
func TestR2SyncFollowsDocumentedPaging(t *testing.T) {
	for _, tc := range []struct {
		args         []string
		rows         int
		wantComplete int
	}{
		{[]string{"sync", "--full", "--resources", "org-agents-devices"}, 51, 1},
		{[]string{"sync", "--full", "--resources", "org-agents-devices", "--max-pages", "1"}, 50, 0},
	} {
		testenv.Isolate(t)
		api := newFakeBlumira(t)
		page1 := make([]map[string]any, 0, 50)
		for i := 0; i < 50; i++ {
			page1 = append(page1, map[string]any{"device_id": fmt.Sprintf("d%02d", i), "hostname": "ws", "org_id": "org-1"})
		}
		api.set("/org/agents/devices", page1, []map[string]any{{"device_id": "dc", "hostname": "dc01", "org_id": "org-1"}})
		runRoot(t, tc.args...)
		s, err := openAnalyticsStore(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		var complete int
		if err := s.DB().QueryRow(`SELECT last_attempt_complete FROM sync_state WHERE resource_type = 'org-agents-devices'`).Scan(&complete); err != nil {
			t.Fatal(err)
		}
		rows, err := s.List("org-agents-devices", 1000)
		_ = s.Close()
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != tc.rows || complete != tc.wantComplete {
			t.Fatalf("%v over 2 documented pages (51 devices): stored %d rows, last_attempt_complete=%d; want %d rows, %d", tc.args, len(rows), complete, tc.rows, tc.wantComplete)
		}
	}
}

var _ = filepath.Join
var _ = store.Open
