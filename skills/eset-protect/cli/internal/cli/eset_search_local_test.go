// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"eset-protect-pp-cli/internal/cliutil/testenv"
)

// TestSearchIsLocalOnly (handfixes.json: search-is-local-only): the generated
// default search posted the query as `q` to detection-groups:search, which
// accepts only `filter`, and ignored --type, so `search "ransomware" --type
// devices` could return unrelated detection groups. Default search must stay
// on the local mirror (no request) and live mode must be refused.
func TestSearchIsLocalOnly(t *testing.T) {
	testenv.Isolate(t)
	respondClearHarness(t)
	t.Cleanup(func() { setDefaultDBScopeIdentity("", "", "") })
	var calls int32
	respondFakeESET(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		_, _ = w.Write([]byte(`{"detectionGroups":[{"uuid":"unrelated"}]}`))
	})
	out, errOut, _ := captureExecuteAll(t, "search", "ransomware", "--type", "devices", "--json")
	if atomic.LoadInt32(&calls) != 0 || strings.Contains(string(out), "unrelated") {
		t.Fatalf("default search reached the API: calls=%d out=%s err=%s", atomic.LoadInt32(&calls), out, errOut)
	}
	_, errOut, err := captureExecuteAll(t, "search", "ransomware", "--data-source", "live")
	if err == nil || !strings.Contains(string(errOut)+err.Error(), "local mirror") {
		t.Fatalf("live search must be refused: err=%v stderr=%s", err, errOut)
	}
	if atomic.LoadInt32(&calls) != 0 {
		t.Fatalf("a refused live search still called the API (%d)", atomic.LoadInt32(&calls))
	}
}
