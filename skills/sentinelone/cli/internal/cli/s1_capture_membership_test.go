// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"sentinelone-pp-cli/internal/cliutil/testenv"
	"sentinelone-pp-cli/internal/store"
)

// s1DecommissionAPI serves agents 301 and 302 until 301 is decommissioned
// (spec.yaml /agents/actions/decommission); from then on GET /agents returns
// only 302, after calling onDecommissionedRead (when set) inside that walk.
// Filters are ignored: every walk sees the whole fleet.
func s1DecommissionAPI(t *testing.T, decommissioned *atomic.Bool, onDecommissionedRead *atomic.Pointer[func()]) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/agents"):
			agents := `{"id":"302","computerName":"WS-KEEP","agentVersion":"24.1.2.6","networkStatus":"connected"}`
			if !decommissioned.Load() {
				agents = `{"id":"301","computerName":"WS-GONE","agentVersion":"24.1.2.6","networkStatus":"connected"},` + agents
			} else if hook := onDecommissionedRead.Load(); hook != nil {
				(*hook)()
			}
			_, _ = w.Write([]byte(`{"data":[` + agents + `],"pagination":{"nextCursor":null}}`))
		default:
			_, _ = w.Write([]byte(`{"data":[],"pagination":{"nextCursor":null,"totalItems":0}}`))
		}
	}))
}

func runMembershipSync(t *testing.T, db string) {
	t.Helper()
	root := RootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"sync", "--resources", "agents", "--db", db, "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("sync: %v\n%s", err, out.String())
	}
	if strings.Contains(out.String(), "history snapshot not recorded") {
		t.Fatalf("sync: a complete, attributable walk recorded no capture:\n%s", out.String())
	}
}

func agentSyncedAt(t *testing.T, db, id string) string {
	t.Helper()
	s, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var at string
	if err := s.DB().QueryRow(`SELECT CAST(synced_at AS TEXT) FROM resources WHERE resource_type = 'agents' AND id = ?`, id).Scan(&at); err != nil {
		t.Fatalf("synced_at of agent %s: %v", id, err)
	}
	return at
}

// assertDecommissionedAgentReadsAsRemoved: the first walk returned 301 and
// 302, the second only 302. History must hold two captures, the second
// without 301, and whatchanged must report 301 removed.
func assertDecommissionedAgentReadsAsRemoved(t *testing.T, db string) {
	t.Helper()
	s, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	times, err := s.FleetSnapshotTimes("agents")
	if err != nil {
		t.Fatal(err)
	}
	if len(times) != 2 {
		s.Close()
		t.Fatalf("agents captures = %v, want 2 (one per complete walk, even within one second)", times)
	}
	base, _ := s.FleetSnapshotRowsByID("agents", times[0])
	cur, _ := s.FleetSnapshotRowsByID("agents", times[1])
	s.Close()
	if _, ok := base["301"]; !ok || len(base) != 2 {
		t.Fatalf("first capture = %v, want agents 301 and 302", keysOf(base))
	}
	if _, ok := cur["301"]; ok {
		t.Fatalf("second capture holds agent 301, which the second walk did not return: %v", keysOf(cur))
	}
	if _, ok := cur["302"]; !ok || len(cur) != 1 {
		t.Fatalf("second capture = %v, want exactly agent 302", keysOf(cur))
	}
	var removed []string
	for id := range base {
		if _, ok := cur[id]; !ok {
			removed = append(removed, id)
		}
	}
	if len(removed) != 1 || removed[0] != "301" {
		t.Fatalf("capture diff removed = %v, want [301]", removed)
	}

	out := runJSON(t, newNovelWhatchangedCmd(jsonFlags()), "--db", db, "--since", "1h")
	ra, _ := out["removed_agents"].([]any)
	if len(ra) != 1 {
		t.Fatalf("whatchanged removed_agents = %v, want agent 301: %v", ra, out)
	}
	if m, _ := ra[0].(map[string]any); m["id"] != "301" {
		t.Fatalf("whatchanged removed_agents = %v, want agent 301", ra)
	}
}

func keysOf[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestCaptureMembershipIsPerWalkNotPerSecond: a capture must hold exactly the
// rows its own walk wrote. Agent 301 is written by the first walk and is no
// longer returned by the second (decommissioned). When the second walk starts
// within the same wall-clock second as 301's write, or after the clock
// stepped backwards, a timestamp test ("synced_at at or after the walk
// start", or "within the walk's seconds") reads 301 as written by the second
// walk, and history never reports the removal an MSP acts on.
func TestCaptureMembershipIsPerWalkNotPerSecond(t *testing.T) {
	testenv.Isolate(t)
	var decommissioned atomic.Bool
	var onDecommissionedRead atomic.Pointer[func()]
	srv := s1DecommissionAPI(t, &decommissioned, &onDecommissionedRead)
	defer srv.Close()
	t.Setenv("SENTINELONE_BASE_URL", srv.URL)
	t.Setenv("SENTINELONE_API_TOKEN", "test-token-1234")

	t.Run("same_second", func(t *testing.T) {
		// Two walks are not timed to fit one wall-clock second (a loaded
		// runner never fits two syncs in one). Instead, while the second walk
		// is reading agents, the first walk's agent rows and capture are
		// restamped with the wall-clock second of that read: 301 then carries
		// a stamp inside the second walk's own span, on any runner. The
		// restamp goes through a connection opened between the walks, which
		// carries no walk recorder, so the second walk never records it.
		db := filepath.Join(t.TempDir(), "s1.db")
		decommissioned.Store(false)
		runMembershipSync(t, db)

		s, err := store.Open(db)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		conn, err := s.DB().Conn(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		var stamped string
		var stampErr error
		restamp := func() {
			if stamped != "" {
				return
			}
			stamped = time.Now().UTC().Format(time.RFC3339)
			for _, q := range []string{
				`UPDATE resources SET synced_at = ? WHERE resource_type = 'agents'`,
				`UPDATE fleet_snapshots SET captured_at = ? WHERE entity = 'agents'`,
				`UPDATE fleet_capture_log SET captured_at = ? WHERE entity = 'agents'`,
			} {
				if _, err := conn.ExecContext(context.Background(), q, stamped); err != nil && stampErr == nil {
					stampErr = err
				}
			}
		}
		onDecommissionedRead.Store(&restamp)
		defer onDecommissionedRead.Store(nil)

		decommissioned.Store(true)
		runMembershipSync(t, db)
		if stampErr != nil {
			t.Fatalf("restamping the first walk during the second: %v", stampErr)
		}
		if stamped == "" {
			t.Fatal("precondition: the second walk never read agents, so nothing was restamped")
		}
		// 301 carries the second of the second walk's read; 302, written by
		// that walk after the read, carries the same second or a later one.
		if gone, kept := agentSyncedAt(t, db, "301"), agentSyncedAt(t, db, "302"); gone != stamped || kept < gone {
			t.Fatalf("precondition: agent 301 synced_at = %s, want %s (inside the second walk, at or before 302's %s)", gone, stamped, kept)
		}
		assertDecommissionedAgentReadsAsRemoved(t, db)
	})

	t.Run("clock_stepped_back", func(t *testing.T) {
		db := filepath.Join(t.TempDir(), "s1.db")
		decommissioned.Store(false)
		runMembershipSync(t, db)
		// The clock steps backwards between the walks (NTP correction): every
		// row the first walk wrote carries a stamp later than the second
		// walk's start.
		s, err := store.Open(db)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.DB().Exec(`UPDATE resources SET synced_at = '2999-12-31T23:59:59Z' WHERE resource_type = 'agents'`); err != nil {
			t.Fatal(err)
		}
		s.Close()
		time.Sleep(1100 * time.Millisecond) // distinct capture seconds: only membership is under test here
		decommissioned.Store(true)
		runMembershipSync(t, db)
		assertDecommissionedAgentReadsAsRemoved(t, db)
	})
}
