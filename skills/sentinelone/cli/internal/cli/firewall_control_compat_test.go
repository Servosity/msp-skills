// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"sentinelone-pp-cli/internal/cliutil"
	"sentinelone-pp-cli/internal/cliutil/testenv"
)

// TestFirewallControlOldSpellingSendsNothing pins the firewall-control rename
// guard by behavior. Up to v0.1.x `copy-rules`, `move-rules` and `set-location`
// were groups whose only child was `by-category`; from the 4.32.5 reprint on
// they are leaf POST endpoints and the by-category operations live under
// `item-<name> by-category`. A script still typing the old spelling must be
// refused before any request is built, with the new path named, and the new
// path must still resolve. Press 4.34.0 generates the refusal itself (a hidden
// `by-category` child); this test is what notices if a reprint loses it.
func TestFirewallControlOldSpellingSendsNothing(t *testing.T) {
	for _, name := range []string{"copy-rules", "move-rules", "set-location"} {
		var hits int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&hits, 1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{}}`))
		}))
		t.Setenv("SENTINELONE_BASE_URL", srv.URL)
		t.Setenv("SENTINELONE_API_TOKEN", "test-token")
		testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)

		root := RootCmd()
		root.SetArgs([]string{"firewall-control", name, "by-category", "--no-cache"})
		err := root.Execute()
		srv.Close()
		if err == nil || !strings.Contains(err.Error(), "item-"+name+" by-category") {
			t.Fatalf("%s: old spelling was not refused with the new path: %v", name, err)
		}
		if n := atomic.LoadInt32(&hits); n != 0 {
			t.Fatalf("%s: old spelling sent %d request(s)", name, n)
		}
		item, _, err := RootCmd().Find([]string{"firewall-control", "item-" + name, "by-category"})
		if err != nil || item.Name() != "by-category" || item.Parent().Name() != "item-"+name {
			t.Fatalf("%s: new path does not resolve: %v %v", name, item, err)
		}
	}
}
