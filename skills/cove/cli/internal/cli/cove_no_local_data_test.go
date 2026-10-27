// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// snapshot copies API data into the local store, so COVE_NO_LOCAL_DATA=1
// refuses it before any request is sent (Codex review round 1).
func TestSnapshotRefusedWhenLocalDataIsOff(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	fleetTestEnv(t, srv)
	t.Setenv("COVE_NO_LOCAL_DATA", "1")
	out, err := runCoveCmd(t, "snapshot", "--db", filepath.Join(t.TempDir(), "s.db"), "--json")
	if err == nil || !strings.Contains(err.Error(), "COVE_NO_LOCAL_DATA=1") {
		t.Fatalf("snapshot ran with local data off: err=%v\n%s", err, out)
	}
	if hits.Load() != 0 {
		t.Fatalf("snapshot sent %d request(s) with local data off", hits.Load())
	}
}
