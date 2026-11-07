// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"testing"

	"eset-protect-pp-cli/internal/cliutil/testenv"
	"eset-protect-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

// localReadSeed writes n rows of resourceType into the default mirror, the
// way sync stores them, and returns how many the mirror holds.
func localReadSeed(t *testing.T, db *store.Store, resourceType string, n int, extra map[string]any) int {
	t.Helper()
	idField := resourceIDFieldOverrides[resourceType]
	if idField == "" {
		idField = "uuid"
	}
	rows := make([]json.RawMessage, 0, n)
	for i := 0; i < n; i++ {
		row := map[string]any{idField: fmt.Sprintf("00000000-0000-4000-8000-%012d", i+1), "displayName": fmt.Sprintf("%s row %d", resourceType, i)}
		for k, v := range extra {
			row[k] = v
		}
		b, _ := json.Marshal(row)
		rows = append(rows, b)
	}
	if _, _, err := db.UpsertBatch(resourceType, rows); err != nil {
		t.Fatalf("seeding %s: %v", resourceType, err)
	}
	got, err := db.Count(resourceType)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// TestLocalListReadsEverySyncedResource (adversarial round 3, P1): for
// EVERY resource sync writes (knownSyncResourceNames, flat and dependent), a
// local list read through the same path the generated command uses returns
// every row the mirror holds. The generated reader used to read the scheme
// and host of an absolute ESET Connect URL as a parent filter, so every
// service but device-management read back "0 results (cached ...)". The
// generated list commands are also run end to end with --data-source local.
func TestLocalListReadsEverySyncedResource(t *testing.T) {
	home := testenv.Isolate(t)
	for _, v := range []string{"ESET_PROTECT_TOKEN", "ESET_PROTECT_USERNAME", "ESET_PROTECT_PASSWORD", "ESET_PROTECT_BASE_URL"} {
		t.Setenv(v, "")
	}
	// The mirror path is scoped by credential; settle the scope the commands
	// below will resolve (an earlier test may have left another one set).
	configureDefaultDBScope("")
	// Pin --home for the seed and for every command below (an earlier test's
	// --home can still be the process-wide override).
	pin := RootCmd()
	pin.SetOut(io.Discard)
	pin.SetErr(io.Discard)
	pin.SetArgs([]string{"version", "--home", home})
	if err := pin.Execute(); err != nil {
		t.Fatal(err)
	}
	db, err := store.OpenWithContext(context.Background(), defaultDBPath(esetCLIName))
	if err != nil {
		t.Fatal(err)
	}
	type target struct {
		name, path string
		want       int
	}
	var targets []target
	deps := map[string]dependentResourceDef{}
	for _, d := range dependentResourceDefs() {
		deps[d.Name] = d
	}
	for _, name := range knownSyncResourceNames() {
		if dep, ok := deps[name]; ok {
			// Two rows under the parent read back, one under another parent
			// stays out: the parent filter still works.
			parentField := localResourceSnake(dep.ParentTable) + "_id"
			localReadSeed(t, db, name, 1, map[string]any{parentField: "parent-other"})
			path := strings.Replace(dep.PathTemplate, "{"+dep.ParentIDParam+"}", "parent-one", 1)
			before, _ := db.Count(name)
			rows := make([]json.RawMessage, 0, 2)
			for i := 0; i < 2; i++ {
				b, _ := json.Marshal(map[string]any{"uuid": fmt.Sprintf("00000000-0000-4000-9000-%012d", i+1), parentField: "parent-one"})
				rows = append(rows, b)
			}
			if _, _, err := db.UpsertBatch(name, rows); err != nil {
				t.Fatal(err)
			}
			after, _ := db.Count(name)
			targets = append(targets, target{name, path, after - before})
			continue
		}
		path, err := syncResourcePath(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if strings.Contains(strings.Replace(path, "{region}", "", 1), "{") {
			// A path with a parent placeholder (ip-sets under a policy).
			path = strings.Replace(path, "{policyUuid}", "policy-one", 1)
			targets = append(targets, target{name, path, localReadSeed(t, db, name, 3, map[string]any{"policies_id": "policy-one"})})
			continue
		}
		targets = append(targets, target{name, path, localReadSeed(t, db, name, 3, nil)})
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	for _, tg := range targets {
		data, _, err := resolveLocal(context.Background(), nil, io.Discard, tg.name, true, tg.path, nil, "test")
		if err != nil {
			t.Errorf("%s: local read failed: %v", tg.name, err)
			continue
		}
		var items []json.RawMessage
		if err := json.Unmarshal(data, &items); err != nil {
			t.Errorf("%s: %v (%s)", tg.name, err, data)
			continue
		}
		if len(items) != tg.want {
			t.Errorf("%s (%s): local read returned %d rows, the mirror holds %d", tg.name, tg.path, len(items), tg.want)
		}
	}

	// End to end: every generated list command whose path is a synced
	// resource's path, run with --data-source local.
	byPath := map[string]target{}
	for _, tg := range targets {
		if !strings.Contains(tg.path, "parent-") && !strings.Contains(tg.path, "policy-one") {
			byPath[tg.path] = tg
		}
	}
	ran := map[string]bool{}
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			walk(sub)
		}
		tg, ok := byPath[c.Annotations["pp:path"]]
		if !ok || c.Annotations["pp:method"] != "GET" || c.Name() != "list" {
			return
		}
		root := RootCmd()
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(io.Discard)
		args := append(strings.Fields(strings.TrimPrefix(c.CommandPath(), c.Root().Name()+" ")), "--data-source", "local", "--json", "--home", home)
		root.SetArgs(args)
		if err := root.Execute(); err != nil {
			t.Errorf("%v: %v\n%s", args, err, out.String())
			return
		}
		var env struct {
			Results []json.RawMessage `json:"results"`
		}
		if err := json.Unmarshal(out.Bytes(), &env); err != nil {
			t.Errorf("%v: %v\n%s", args, err, out.String())
			return
		}
		if len(env.Results) != tg.want {
			t.Errorf("%v: %d results, the mirror holds %d %s rows", args, len(env.Results), tg.want, tg.name)
		}
		ran[tg.name] = true
	}
	walk(RootCmd())
	var missing []string
	for _, must := range []string{"devices", "detections", "detections_v2", "incidents", "policies", "executables", "quarantined-objects", "device-tasks"} {
		if !ran[must] {
			missing = append(missing, must)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("no generated list command was run for %v (ran %v)", missing, ran)
	}
}
