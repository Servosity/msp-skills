// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"

	"cisco-duo-pp-cli/internal/store"
)

// fakeGroupSyncClient answers the sync paginator like the Duo Admin API: two
// groups that share a display name (so a name-keyed store would collapse
// them), and one user who is a member of both (so a membership keyed by the
// bare user_id would collapse to one row). Every other collection is empty.
type fakeGroupSyncClient struct {
	mu    sync.Mutex
	paths []string
}

func (f *fakeGroupSyncClient) RateLimit() float64 { return 0 }

func (f *fakeGroupSyncClient) Get(_ context.Context, path string, params map[string]string) (json.RawMessage, error) {
	f.mu.Lock()
	f.paths = append(f.paths, path)
	f.mu.Unlock()
	empty := json.RawMessage(`{"stat":"OK","response":[]}`)
	if off := params["offset"]; off != "" && off != "0" {
		return empty, nil
	}
	switch path {
	case "/admin/v1/groups":
		return json.RawMessage(`{"stat":"OK","response":[
			{"group_id":"DGAAAAAAAAAAAAAAAAAA","name":"Staff","desc":"","status":"Active"},
			{"group_id":"DGBBBBBBBBBBBBBBBBBB","name":"Staff","desc":"","status":"Bypass"}
		]}`), nil
	case "/admin/v2/groups/DGAAAAAAAAAAAAAAAAAA/users", "/admin/v2/groups/DGBBBBBBBBBBBBBBBBBB/users":
		return json.RawMessage(`{"stat":"OK","response":[
			{"user_id":"DUAAAAAAAAAAAAAAAAAA","username":"alice"}
		]}`), nil
	}
	return empty, nil
}

func (f *fakeGroupSyncClient) fetched(path string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Contains(f.paths, path)
}

// Codex merge review (P1): the group-membership dependent was registered under
// the flat resource's own name, "groups", so flatSyncResources dropped the
// top-level groups fetch, the dependent skipped on an empty parent table, and
// the flat groups resource inherited the membership's user_id key. A fresh
// sync must fetch groups, key them by group_id, and store memberships under a
// name of their own.
func TestFreshSyncPopulatesGroupsAndMembers(t *testing.T) {
	resources := flatSyncResources(defaultSyncResources())
	if !slices.Contains(resources, "groups") {
		t.Fatalf("flatSyncResources(defaultSyncResources()) dropped the top-level groups resource: %v", resources)
	}

	db, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	fake := &fakeGroupSyncClient{}
	for _, resource := range resources {
		if res := syncResource(ctx, fake, db, resource, "", true, 0, false, false, nil, io.Discard); res.Err != nil {
			t.Fatalf("sync %s: %v", resource, res.Err)
		}
	}
	for _, res := range syncDependentResources(ctx, fake, db, "", true, 0, false, false, nil, nil, io.Discard, 1) {
		if res.Err != nil {
			t.Fatalf("dependent sync %s: %v", res.Resource, res.Err)
		}
	}

	if !fake.fetched("/admin/v1/groups") {
		t.Fatalf("a fresh sync never requested /admin/v1/groups; requests: %v", fake.paths)
	}

	ids, err := db.ListIDs("groups")
	if err != nil {
		t.Fatalf("list group ids: %v", err)
	}
	for i := range ids {
		ids[i] = store.BareResourceID(ids[i])
	}
	sort.Strings(ids)
	want := []string{"DGAAAAAAAAAAAAAAAAAA", "DGBBBBBBBBBBBBBBBBBB"}
	if !slices.Equal(ids, want) {
		t.Fatalf("groups keyed by %v, want both groups keyed by group_id %v", ids, want)
	}

	membersDep := ""
	for _, dep := range dependentResourceDefs() {
		if dep.PathTemplate == "/admin/v2/groups/{group_id}/users" {
			membersDep = dep.Name
		}
	}
	if membersDep == "" || membersDep == "groups" {
		t.Fatalf("group membership dependent is named %q; it must not reuse the flat groups resource name", membersDep)
	}
	for _, g := range want {
		if !fake.fetched("/admin/v2/groups/" + g + "/users") {
			t.Fatalf("membership sync never requested group %s's members; requests: %v", g, fake.paths)
		}
	}
	rows, err := db.DB().Query(`SELECT data FROM resources WHERE resource_type = ?`, membersDep)
	if err != nil {
		t.Fatalf("query %s rows: %v", membersDep, err)
	}
	defer rows.Close()
	var members []string
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			t.Fatalf("scan %s row: %v", membersDep, err)
		}
		if !strings.Contains(data, "DUAAAAAAAAAAAAAAAAAA") {
			t.Fatalf("%s row = %s, want the member's user_id", membersDep, data)
		}
		members = append(members, data)
	}
	if len(members) != 2 {
		t.Fatalf("one user in two groups stored %d %s rows, want 2 (one per group): %v", len(members), membersDep, members)
	}
}

// The class behind the P1: a dependent resource that shares a flat resource's
// name is silently removed from the flat sync list and shares its key and
// partition maps. No dependent may carry a flat resource's name.
func TestNoDependentShadowsAFlatSyncResource(t *testing.T) {
	for _, dep := range dependentResourceDefs() {
		if _, err := syncResourcePath(dep.Name); err == nil {
			t.Errorf("dependent %q (%s) shares its name with a flat sync resource", dep.Name, dep.PathTemplate)
		}
	}
}
