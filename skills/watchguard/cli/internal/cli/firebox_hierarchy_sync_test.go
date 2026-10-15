// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"watchguard-pp-cli/internal/store"
)

type paramRecordingGetter struct {
	params []map[string]string
	body   string
}

func (g *paramRecordingGetter) Get(_ context.Context, _ string, params map[string]string) (json.RawMessage, error) {
	cp := map[string]string{}
	for k, v := range params {
		cp[k] = v
	}
	g.params = append(g.params, cp)
	return json.RawMessage(g.body), nil
}
func (g *paramRecordingGetter) RateLimit() float64 { return 0 }

// hand-fix (handfixes.json firebox-devices-sync-includes-subscribers): for a
// Service Provider, the spec default include_hierarchy=false returns only the
// provider's own devices, so a bare `sync --full` pruned every Subscriber
// Firebox from the mirror. Sync now asks for the hierarchy by default as a
// base param: it is not a user filter, so --full still prunes devices that
// are really gone, and an explicit --resource-param still wins.
func TestFireboxDevicesSyncIncludesSubscribers(t *testing.T) {
	prev := humanFriendly
	humanFriendly = false
	defer func() { humanFriendly = prev }()
	db, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, _, err := db.UpsertBatch("firebox_devices", []json.RawMessage{json.RawMessage(`{"id":"FB-GONE","name":"retired"}`)}); err != nil {
		t.Fatal(err)
	}
	g := &paramRecordingGetter{body: `{"count":2,"data":[{"id":"FB-SP","name":"sp"},{"id":"FB-SUB","name":"subscriber"}]}`}
	if res := syncResource(context.Background(), g, db, "firebox_devices", "", true, 0, false, true, nil, nil); res.Err != nil {
		t.Fatal(res.Err)
	}
	if got := g.params[0]["include_hierarchy"]; got != "true" {
		t.Fatalf("bare sync sent include_hierarchy=%q, want true", got)
	}
	if row, _ := db.Get("firebox_devices", "FB-GONE"); row != nil {
		t.Fatal("--full no longer prunes a device the API stopped returning (base param read as a filter)")
	}
	if row, _ := db.Get("firebox_devices", "FB-SUB"); row == nil {
		t.Fatal("subscriber device not stored")
	}
	g.params = nil
	override := &syncUserParams{perResource: map[string]map[string]string{"firebox_devices": {"include_hierarchy": "false"}}}
	if res := syncResource(context.Background(), g, db, "firebox_devices", "", false, 0, false, false, override, nil); res.Err != nil {
		t.Fatal(res.Err)
	}
	if got := g.params[0]["include_hierarchy"]; got != "false" {
		t.Fatalf("explicit --resource-param include_hierarchy=false lost: sent %q", got)
	}
}
