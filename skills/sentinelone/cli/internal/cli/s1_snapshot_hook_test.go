// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sentinelone-pp-cli/internal/cliutil/testenv"
	"sentinelone-pp-cli/internal/store"
)

func s1MockAPI(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/agents"):
			_, _ = w.Write([]byte(`{"data":[{"id":"101","computerName":"ws-01","agentVersion":"23.4.1"}],"pagination":{"nextCursor":null,"totalItems":1}}`))
		case strings.HasSuffix(r.URL.Path, "/threats"):
			_, _ = w.Write([]byte(`{"data":[{"id":"201","threatInfo":{"threatName":"x"}}],"pagination":{"nextCursor":null,"totalItems":1}}`))
		default:
			_, _ = w.Write([]byte(`{"data":[],"pagination":{"nextCursor":null,"totalItems":0}}`))
		}
	}))
}

func runSync(t *testing.T, db string, resources string) {
	t.Helper()
	root := RootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"sync", "--resources", resources, "--db", db, "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("sync %s: %v\n%s", resources, err, out.String())
	}
	if strings.Contains(out.String(), "history snapshot not recorded") {
		t.Fatalf("sync %s: snapshot warning:\n%s", resources, out.String())
	}
}

// TestSyncRecordsFleetHistory pins the snapshot wiring: every sync that
// completes agents/threats appends one capture of each, and a sync that did
// not refresh an entity records nothing for it. Before the hook, sync never
// called CaptureFleetSnapshot and every history novel stayed empty forever.
func TestSyncRecordsFleetHistory(t *testing.T) {
	testenv.Isolate(t)
	srv := s1MockAPI(t)
	defer srv.Close()
	t.Setenv("SENTINELONE_BASE_URL", srv.URL)
	t.Setenv("SENTINELONE_API_TOKEN", "test-token")
	db := filepath.Join(t.TempDir(), "s1.db")

	runSync(t, db, "agents,threats")
	time.Sleep(1100 * time.Millisecond) // captures are second-stamped
	runSync(t, db, "agents")

	s, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	agents, _ := s.FleetSnapshotTimes("agents")
	threats, _ := s.FleetSnapshotTimes("threats")
	if len(agents) != 2 {
		t.Fatalf("agents captures = %d, want 2 (one per sync that refreshed agents)", len(agents))
	}
	if len(threats) != 1 {
		t.Fatalf("threats captures = %d, want 1 (the agents-only sync must not restamp threats)", len(threats))
	}
	rows, _ := s.FleetSnapshotRowsByID("agents", agents[1])
	if _, ok := rows["101"]; !ok || len(rows) != 1 {
		t.Fatalf("latest agents capture = %v, want exactly agent 101", rows)
	}
}
