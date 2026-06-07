// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored shared harness for the novel-feature behavioral tests.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"appdirect-pp-cli/internal/store"
)

// newNovelTestDB opens a temp store and returns it plus its on-disk path.
func newNovelTestDB(t *testing.T) (*store.Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "data.db")
	db, err := store.OpenWithContext(context.Background(), path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db, path
}

// seedNovelResource inserts one synced row into the generic resources table.
func seedNovelResource(t *testing.T, db *store.Store, resourceType, id string, obj map[string]any) {
	t.Helper()
	data, err := json.Marshal(obj)
	if err != nil {
		t.Fatalf("marshal %s/%s: %v", resourceType, id, err)
	}
	if _, err := db.DB().Exec(
		`INSERT INTO resources (id, resource_type, data) VALUES (?, ?, ?)`,
		id, resourceType, string(data),
	); err != nil {
		t.Fatalf("seed %s/%s: %v", resourceType, id, err)
	}
}

// runNovelCmd executes a novel command constructor with --json output and
// returns stdout plus the execution error.
func runNovelCmd(t *testing.T, build func(*rootFlags) *cobra.Command, args ...string) (string, error) {
	t.Helper()
	flags := &rootFlags{asJSON: true}
	cmd := build(flags)
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

// decodeNovelJSON parses command output into a generic map.
func decodeNovelJSON(t *testing.T, raw string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, raw)
	}
	return m
}

// epochMS converts a time to the float-friendly epoch-milliseconds shape the
// AppDirect API uses.
func epochMS(t time.Time) float64 {
	return float64(t.UnixMilli())
}

// novelList extracts a []any field from decoded output.
func novelList(t *testing.T, m map[string]any, key string) []any {
	t.Helper()
	v, ok := m[key].([]any)
	if !ok {
		t.Fatalf("field %q missing or not a list in %v", key, m)
	}
	return v
}
