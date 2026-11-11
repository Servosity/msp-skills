// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

// Hand-fix (handfixes.json sync-forbidden-not-partial): tools/maintainer/
// replay_handfixes.py writes this file from
// handfix_replay/sync_coverage_mcp_test.go.tmpl after a reprint.

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"zoom-phone-pp-cli/internal/store"
)

func TestStoreStatusSeparatesForbiddenFromBroken(t *testing.T) {
	db, err := store.OpenWithContext(context.Background(), filepath.Join(t.TempDir(), "sync.db"))
	if err != nil {
		t.Fatalf("opening the local store: %v", err)
	}
	defer db.Close()
	status := func() mcpStoreStatusKind {
		t.Helper()
		got, err := mcpStoreStatus(db)
		if err != nil {
			t.Fatalf("mcpStoreStatus: %v", err)
		}
		return got
	}
	save := func(fn func() error) {
		t.Helper()
		if err := fn(); err != nil {
			t.Fatal(err)
		}
	}
	save(func() error { return db.SaveSyncState("widgets", "", 3) })
	save(func() error { return db.SaveSyncProgress("tags", "", 0) })
	if got := status(); got != mcpStoreStatusPartial {
		t.Fatalf("an unfinished resource with no refusal: status = %q, want partial", got)
	}

	save(func() error { return db.MarkSyncForbidden("tags") })
	if got := status(); got != mcpStoreStatusCompleteWithSkips {
		t.Fatalf("the only unfinished resource was refused: status = %q, want %q", got, mcpStoreStatusCompleteWithSkips)
	}
	out := mcpWithSyncCoverage(db, status(), map[string]any{})
	if !reflect.DeepEqual(out["skipped_forbidden"], []string{"tags"}) || out["incomplete_resources"] != nil {
		t.Fatalf("complete-with-skips envelope = %v", out)
	}
	if w, _ := out["warning"].(string); !strings.Contains(w, "tags") {
		t.Fatalf("the warning does not name the skip: %v", out["warning"])
	}

	save(func() error { return db.SaveSyncProgress("tickets", "", 0) })
	if got := status(); got != mcpStoreStatusPartial {
		t.Fatalf("a readable resource failed beside a refused one: status = %q, want partial", got)
	}
	out = mcpWithSyncCoverage(db, status(), map[string]any{})
	if !reflect.DeepEqual(out["incomplete_resources"], []string{"tickets"}) {
		t.Fatalf("partial envelope does not name the failed resource: %v", out)
	}

	save(func() error { return db.SaveSyncState("tickets", "", 1) })
	save(func() error { return db.SaveSyncProgress("widgets", "", 0) })
	save(func() error { return db.MarkSyncForbidden("widgets") })
	save(func() error { return db.SaveSyncProgress("tickets", "", 0) })
	save(func() error { return db.MarkSyncForbidden("tickets") })
	if got := status(); got != mcpStoreStatusPartial {
		t.Fatalf("every resource refused, none stored: status = %q, want partial", got)
	}
}
