// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"syncro-pp-cli/internal/store"
)

// newNovelTestStore opens a fresh temp SQLite store for a test.
func newNovelTestStore(t *testing.T) (*store.Store, string) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "data.db")
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db, dbPath
}

// mustJSON marshals v to a JSON byte slice or fails the test.
func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// execNovel runs the given command with args, capturing stdout, and returns the
// parsed JSON object plus the raw bytes.
func execNovel(t *testing.T, cmd *cobra.Command, args []string) (map[string]any, []byte) {
	t.Helper()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute %v: %v", args, err)
	}
	out := buf.Bytes()
	var parsed map[string]any
	if len(bytes.TrimSpace(out)) > 0 {
		if err := json.Unmarshal(out, &parsed); err != nil {
			t.Fatalf("unmarshal output %q: %v", string(out), err)
		}
	}
	return parsed, out
}

// seedResource inserts a generic resources row.
func seedResource(t *testing.T, db *store.Store, resourceType, id string, data any) {
	t.Helper()
	_, err := db.DB().Exec(
		`INSERT INTO resources(id, resource_type, data) VALUES(?, ?, ?)`,
		id, resourceType, string(mustJSON(t, data)),
	)
	if err != nil {
		t.Fatalf("seed resource %s/%s: %v", resourceType, id, err)
	}
}

// seedTimerEntry inserts a timer_entry row for a ticket.
func seedTimerEntry(t *testing.T, db *store.Store, id, ticketID string, data any) {
	t.Helper()
	_, err := db.DB().Exec(
		`INSERT INTO timer_entry(id, tickets_id, data) VALUES(?, ?, ?)`,
		id, ticketID, string(mustJSON(t, data)),
	)
	if err != nil {
		t.Fatalf("seed timer_entry %s: %v", id, err)
	}
}

// seedComment inserts a comments row for a ticket.
func seedComment(t *testing.T, db *store.Store, id, ticketID string, data any) {
	t.Helper()
	_, err := db.DB().Exec(
		`INSERT INTO comments(id, tickets_id, data) VALUES(?, ?, ?)`,
		id, ticketID, string(mustJSON(t, data)),
	)
	if err != nil {
		t.Fatalf("seed comment %s: %v", id, err)
	}
}

// seedCustomerAsset inserts a customer_assets row.
func seedCustomerAsset(t *testing.T, db *store.Store, id string, data any) {
	t.Helper()
	_, err := db.DB().Exec(
		`INSERT INTO customer_assets(id, data) VALUES(?, ?)`,
		id, string(mustJSON(t, data)),
	)
	if err != nil {
		t.Fatalf("seed customer_asset %s: %v", id, err)
	}
}

// seedPatch inserts a patches row for an asset.
func seedPatch(t *testing.T, db *store.Store, id, assetID string, data any) {
	t.Helper()
	_, err := db.DB().Exec(
		`INSERT INTO patches(id, customer_assets_id, data) VALUES(?, ?, ?)`,
		id, assetID, string(mustJSON(t, data)),
	)
	if err != nil {
		t.Fatalf("seed patch %s: %v", id, err)
	}
}

// jsonFloat reads a float field from a parsed JSON object.
func jsonFloat(t *testing.T, m map[string]any, key string) float64 {
	t.Helper()
	v, ok := m[key]
	if !ok {
		t.Fatalf("missing key %q in %v", key, m)
	}
	f, ok := v.(float64)
	if !ok {
		t.Fatalf("key %q is not a number: %T", key, v)
	}
	return f
}

// jsonItems reads the "items" array from a parsed JSON object.
func jsonItems(t *testing.T, m map[string]any) []map[string]any {
	t.Helper()
	raw, ok := m["items"]
	if !ok {
		t.Fatalf("missing items in %v", m)
	}
	arr, ok := raw.([]any)
	if !ok {
		t.Fatalf("items is not an array: %T", raw)
	}
	out := make([]map[string]any, 0, len(arr))
	for _, e := range arr {
		obj, ok := e.(map[string]any)
		if !ok {
			t.Fatalf("item is not an object: %T", e)
		}
		out = append(out, obj)
	}
	return out
}
