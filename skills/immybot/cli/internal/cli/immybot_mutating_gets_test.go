// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// A GET declared state-changing (cliutil.MutatingGETPaths) is a write in the
// CLI too (round-1 next-batch review): --data-source local refuses it rather
// than answer from the mirror, and its answer is never filed in the local
// store or the response cache, so a second run is sent again.
func TestAMutatingGETIsNeverAnsweredLocally(t *testing.T) {
	const marker = "mutatingGetMarker9Q"
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":4421,"note":"` + marker + `"}`))
	}))
	t.Cleanup(srv.Close)
	home := credTierHome(t, srv.URL)
	cancel := []string{"maintenance-sessions", "get-cancel-for-schedule-by-schedule-id", "4421", "--json"}
	if _, _, err := credTierRun(t, append(append([]string{}, cancel...), "--data-source", "local")...); err == nil || hits.Load() != 0 {
		t.Fatalf("--data-source local answered a state-changing GET (err %v, %d requests)", err, hits.Load())
	}
	for i := 1; i <= 2; i++ {
		if _, stderr, err := credTierRun(t, cancel...); err != nil || hits.Load() != int32(i) {
			t.Fatalf("run %d was not sent (err %v, %d requests): %s", i, err, hits.Load(), stderr)
		}
	}
	if found := credTierFilesHolding(t, home, marker); len(found) > 0 {
		t.Errorf("a state-changing GET's answer was kept on disk: %v", found)
	}
}
