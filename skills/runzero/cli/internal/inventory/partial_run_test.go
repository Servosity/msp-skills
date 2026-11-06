// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package inventory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
)

// failingGetter answers like fakeGetter but refuses the listed export paths,
// as runZero does with its documented 403 NotAllowedForLicenseError.
type failingGetter struct {
	fakeGetter
	refuse map[string]bool
}

func (f *failingGetter) Get(ctx context.Context, path string, params map[string]string) (json.RawMessage, error) {
	if f.refuse[path] {
		return nil, errors.New("HTTP 403: not allowed for license")
	}
	return f.fakeGetter.Get(ctx, path, params)
}

// Class incomplete-run-read-as-removal (runzero handfix diff-compares-each-
// entity's-own-runs): `diff` compared the global last two sync runs, so after a
// run that did not refresh services (`inventory sync --only assets`, or a
// services export refused with the spec's documented 403), every service read
// as removed. Each entity is compared across the runs that synced it, and the
// result names the entities the latest run did not refresh.
func TestDiffComparesEachEntitysOwnRuns(t *testing.T) {
	ctx := context.Background()
	for name, second := range map[string]func(*testing.T, context.Context, *sql.DB){
		"only assets": func(t *testing.T, ctx context.Context, db *sql.DB) {
			if _, err := SyncAll(ctx, &fakeGetter{payloads: seedRun1()}, db, []string{"assets"}, ""); err != nil {
				t.Fatal(err)
			}
		},
		"services refused": func(t *testing.T, ctx context.Context, db *sql.DB) {
			g := &failingGetter{fakeGetter: fakeGetter{payloads: seedRun1()}, refuse: map[string]bool{"/export/org/services.json": true}}
			if _, err := SyncAll(ctx, g, db, nil, ""); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			db := openTestDB(t)
			if _, err := SyncAll(ctx, &fakeGetter{payloads: seedRun1()}, db, nil, ""); err != nil {
				t.Fatal(err)
			}
			second(t, ctx, db)
			d, err := Diff(ctx, db, 0)
			if err != nil {
				t.Fatal(err)
			}
			if len(d.Services.Removed) != 0 {
				t.Errorf("services the latest run did not refresh read as removed: %v", d.Services.Removed)
			}
			if !contains(d.StaleEntities, "services") {
				t.Errorf("diff does not say services were not refreshed: %+v", d.StaleEntities)
			}
		})
	}
}
