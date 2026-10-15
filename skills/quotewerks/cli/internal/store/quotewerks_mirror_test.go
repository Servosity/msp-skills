// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestApplyMirrorReplacesDeletesAndStamps(t *testing.T) {
	ctx := context.Background()
	s, err := OpenWithContext(ctx, filepath.Join(t.TempDir(), "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	row := func(js string) json.RawMessage { return json.RawMessage(js) }
	if err := s.ApplyMirror(ctx, []MirrorWrite{{Resource: "documents", Rows: []json.RawMessage{row(`{"id":"a","DocStatus":"Open","ConvertedRef":"X"}`), row(`{"id":"b"}`)}, Stamp: true, Count: 2}}); err != nil {
		t.Fatal(err)
	}
	// Replacement, not keep-richer merge: a cleared field must not survive.
	if err := s.ApplyMirror(ctx, []MirrorWrite{{Resource: "documents", Rows: []json.RawMessage{row(`{"id":"a","DocStatus":"Won","ConvertedRef":null}`)}, Delete: []string{"b"}}}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get("documents", "a")
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal(got, &m)
	if m["DocStatus"] != "Won" || m["ConvertedRef"] != nil {
		t.Fatalf("row not replaced: %v", m)
	}
	if b, _ := s.Get("documents", "b"); b != nil {
		t.Fatalf("b not deleted: %s", b)
	}
	if _, _, count, err := s.GetSyncState("documents"); err != nil || count != 2 {
		t.Fatalf("sync state count = %d, %v", count, err)
	}
	if err := s.ApplyMirror(ctx, []MirrorWrite{{Resource: "documents", Rows: []json.RawMessage{row(`{"nope":1}`)}}}); err == nil {
		t.Fatal("row without id must fail the whole commit")
	}
}
