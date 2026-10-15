// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"watchguard-pp-cli/internal/store"
)

// wgOneActivationGetter answers every page with one activation, the subset a
// --param-narrowed walk would return.
type wgOneActivationGetter struct{}

func (wgOneActivationGetter) Get(context.Context, string, map[string]string) (json.RawMessage, error) {
	return json.RawMessage(`{"results":[{"id":"a1","serial":"FB-1"}],"pagination":{"totalItems":1}}`), nil
}
func (wgOneActivationGetter) RateLimit() float64 { return 0 }

// wgOneDeviceGetter answers every page with one Firebox, the subset a
// --param-narrowed walk would return. firebox_devices reconciles on --full.
type wgOneDeviceGetter struct{}

func (wgOneDeviceGetter) Get(context.Context, string, map[string]string) (json.RawMessage, error) {
	return json.RawMessage(`{"data":[{"id":"d1","name":"FB-1"}]}`), nil
}
func (wgOneDeviceGetter) RateLimit() float64 { return 0 }

// hand-fix (handfixes.json filtered-sync-no-prune, ported from connectsecure):
// a sync narrowed by --param / --global-param / --resource-param walks a
// subset, so a --full run must neither prune the rows it did not see nor
// stamp the resource complete.
func TestFilteredFullSyncKeepsOtherRows(t *testing.T) {
	prev := humanFriendly
	humanFriendly = false
	defer func() { humanFriendly = prev }()
	for _, tc := range []struct {
		name     string
		params   *syncUserParams
		wantKept bool
	}{
		{"param", &syncUserParams{flatGlobal: map[string]string{"name": "FB-1"}}, true},
		{"resource-param", &syncUserParams{perResource: map[string]map[string]string{"firebox_devices": {"name": "FB-1"}}}, true},
		{"global-param", &syncUserParams{trueGlobal: map[string]string{"name": "FB-1"}}, true},
		{"unfiltered", nil, false},
	} {
		db, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
		if err != nil {
			t.Fatal(err)
		}
		if err := db.Upsert("firebox_devices", "d2", json.RawMessage(`{"id":"d2","name":"FB-OTHER-CLIENT"}`)); err != nil {
			t.Fatal(err)
		}
		var events bytes.Buffer
		if res := syncResource(context.Background(), wgOneDeviceGetter{}, db, "firebox_devices", "", true, 0, false, true, tc.params, &events); res.Err != nil {
			t.Fatalf("%s: %v", tc.name, res.Err)
		}
		row, _ := db.Get("firebox_devices", "d2")
		if kept := row != nil; kept != tc.wantKept {
			t.Errorf("%s: unseen row kept=%v, want %v (events %s)", tc.name, kept, tc.wantKept, events.String())
		}
		_, last, _, _ := db.GetSyncState("firebox_devices")
		if advanced := !last.IsZero(); advanced == tc.wantKept {
			t.Errorf("%s: completion watermark advanced=%v", tc.name, advanced)
		}
		db.Close()
	}
}

// hand-fix (handfixes.json filtered-sync-cursor-isolated, ported from
// connectsecure): a --param walk's offsets index a subset, so it never moves
// the resource-wide resume point a later unfiltered sync continues from.
func TestFilteredSyncKeepsTheResumeCursor(t *testing.T) {
	prev := humanFriendly
	humanFriendly = false
	defer func() { humanFriendly = prev }()
	for _, tc := range []struct {
		name   string
		params *syncUserParams
		want   bool // cursor still "OLD-OFFSET"
	}{
		{"filtered", &syncUserParams{flatGlobal: map[string]string{"serial": "FB-1"}}, true},
		{"unfiltered", nil, false},
	} {
		db, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
		if err != nil {
			t.Fatal(err)
		}
		_ = db.Upsert("activations", "a2", json.RawMessage(`{"id":"a2","serial":"FB-OTHER"}`))
		if err := db.SaveSyncProgress("activations", "OLD-OFFSET", 1); err != nil {
			t.Fatal(err)
		}
		var events bytes.Buffer
		if res := syncResource(context.Background(), wgOneActivationGetter{}, db, "activations", "", false, 0, false, false, tc.params, &events); res.Err != nil {
			t.Fatalf("%s: %v", tc.name, res.Err)
		}
		cur, _, _, _ := db.GetSyncState("activations")
		if kept := cur == "OLD-OFFSET"; kept != tc.want {
			t.Errorf("%s: resume cursor = %q", tc.name, cur)
		}
		db.Close()
	}
}

// wgOperatorsGetter answers every per-account page with one operator.
type wgOperatorsGetter struct{}

func (wgOperatorsGetter) Get(context.Context, string, map[string]string) (json.RawMessage, error) {
	return json.RawMessage(`{"result":[{"id":"op1","name":"admin"}]}`), nil
}
func (wgOperatorsGetter) RateLimit() float64 { return 0 }

// A dependent walk narrowed by --global-param or --resource-param covers a
// subset of each parent's children, so it never stamps the dependent complete.
func TestFilteredDependentSyncNotStampedComplete(t *testing.T) {
	prev := humanFriendly
	humanFriendly = false
	defer func() { humanFriendly = prev }()
	var dep dependentResourceDef
	for _, d := range dependentResourceDefs() {
		if d.Name == "operators" {
			dep = d
		}
	}
	if dep.Name == "" {
		t.Fatal("operators dependent not defined")
	}
	for _, tc := range []struct {
		name         string
		params       *syncUserParams
		wantComplete bool
	}{
		{"global-param", &syncUserParams{trueGlobal: map[string]string{"role": "admin"}}, false},
		{"resource-param", &syncUserParams{perResource: map[string]map[string]string{"operators": {"role": "admin"}}}, false},
		{"param (flat only)", &syncUserParams{flatGlobal: map[string]string{"role": "admin"}}, true},
		{"unfiltered", nil, true},
	} {
		db, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := db.UpsertBatch("accounts", []json.RawMessage{json.RawMessage(`{"id":"ACC-2","account_id":"ACC-2","name":"client"}`)}); err != nil {
			t.Fatal(err)
		}
		var events bytes.Buffer
		if res := syncDependentResource(context.Background(), wgOperatorsGetter{}, db, dep, "", false, 0, false, false, tc.params, &events, 1); res.Err != nil {
			t.Fatalf("%s: %v", tc.name, res.Err)
		}
		_, last, _, _ := db.GetSyncState("operators")
		if complete := !last.IsZero(); complete != tc.wantComplete {
			t.Errorf("%s: operators stamped complete=%v, want %v (events %s)", tc.name, complete, tc.wantComplete, events.String())
		}
		db.Close()
	}
}

// hand-fix (handfixes.json dependent-zero-parents-completes, ported from
// connectsecure): an account with no child accounts leaves the five
// accounts-parented dependents with zero parents. Their attempt rows used to
// stay open, so doctor reported the cache stale straight after `sync --full`.
// A parent that never completed a sync proves nothing, so that case stays open.
func TestDependentWithNoParentsCompletes(t *testing.T) {
	prev := humanFriendly
	humanFriendly = false
	defer func() { humanFriendly = prev }()
	for _, tc := range []struct {
		name         string
		parentSynced bool
	}{{"parent synced and empty", true}, {"parent never synced", false}} {
		db, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
		if err != nil {
			t.Fatal(err)
		}
		if tc.parentSynced {
			if err := db.SaveSyncState("accounts", "", 0); err != nil {
				t.Fatal(err)
			}
		}
		for _, dep := range dependentResourceDefs() {
			if res := syncDependentResource(context.Background(), wgOperatorsGetter{}, db, dep, "", true, 0, false, false, nil, nil, 1); res.Err != nil {
				t.Fatalf("%s: %s: %v", tc.name, dep.Name, res.Err)
			}
			_, last, _, err := db.GetSyncState(dep.Name)
			if err != nil {
				t.Fatal(err)
			}
			if complete := !last.IsZero(); complete != tc.parentSynced {
				t.Errorf("%s: %s stamped complete=%v, want %v", tc.name, dep.Name, complete, tc.parentSynced)
			}
		}
		db.Close()
	}
}
