// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"watchguard-pp-cli/internal/store"
)

type wgDevicesGetter struct{ body string }

func (g wgDevicesGetter) Get(context.Context, string, map[string]string) (json.RawMessage, error) {
	return json.RawMessage(g.body), nil
}
func (wgDevicesGetter) RateLimit() float64 { return 0 }

// hand-fix (handfixes.json sync-full-refresh-guidance, ported from
// connectsecure): no WatchGuard resource is incremental, so a bare `sync`
// re-walks every page but never prunes, and a decommissioned Firebox stays in
// the local store (and in firmware-lag --data-source local) for good. Only
// `sync --full` removes it, so every refresh hint must name --full.
func TestRefreshGuidanceNamesFullSync(t *testing.T) {
	prev := humanFriendly
	humanFriendly = false
	defer func() { humanFriendly = prev }()
	db, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	both := wgDevicesGetter{`{"data":[{"id":"d1","name":"FB-1"},{"id":"d2","name":"FB-RETIRED"}]}`}
	one := wgDevicesGetter{`{"data":[{"id":"d1","name":"FB-1"}]}`}
	if res := syncResource(context.Background(), both, db, "firebox_devices", "", true, 0, false, true, nil, nil); res.Err != nil {
		t.Fatal(res.Err)
	}
	// What a bare `sync` runs: full=false, so prune=false.
	if res := syncResource(context.Background(), one, db, "firebox_devices", "", false, 0, false, false, nil, nil); res.Err != nil {
		t.Fatal(res.Err)
	}
	if row, _ := db.Get("firebox_devices", "d2"); row == nil {
		t.Fatal("premise changed: a bare sync now removes deleted rows; revisit this hand-fix")
	}
	if res := syncResource(context.Background(), one, db, "firebox_devices", "", true, 0, false, true, nil, nil); res.Err != nil {
		t.Fatal(res.Err)
	}
	if row, _ := db.Get("firebox_devices", "d2"); row != nil {
		t.Fatal("sync --full kept a Firebox the API no longer returns")
	}

	if err := db.SaveSyncStateAt("firebox_devices", "", 1, time.Now().Add(-48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	var hint bytes.Buffer
	emitSyncHints(&hint, db, "firebox_devices", time.Hour)
	if !strings.Contains(hint.String(), "sync --full") {
		t.Fatalf("stale-store hint does not name sync --full: %q", hint.String())
	}
}

// doctor's stale-cache hint names --full too.
func TestDoctorStaleHintNamesFullSync(t *testing.T) {
	isolateAuthEnv(t)
	t.Setenv("WATCHGUARD_HOME", t.TempDir())
	path := defaultDBPath("watchguard-cli")
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SaveSyncStateAt("firebox_devices", "", 1, time.Now().Add(-48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	db.Close()
	rep := collectCacheReport(context.Background(), "1h")
	if rep["status"] != "stale" || !strings.Contains(rep["hint"].(string), "sync --full") {
		t.Fatalf("doctor cache report status=%v hint=%v, want stale naming sync --full", rep["status"], rep["hint"])
	}
}
