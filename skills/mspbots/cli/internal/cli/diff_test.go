// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"testing"
)

func TestDiffSnapshots(t *testing.T) {
	row := func(s string) json.RawMessage { return json.RawMessage(s) }
	tests := []struct {
		name                    string
		fromKeys, toKeys        []string
		fromRows, toRows        []json.RawMessage
		added, removed, changed int
	}{
		{
			name:     "id rows: one added one removed one changed",
			fromKeys: []string{"id:1", "id:2", "id:3"},
			fromRows: []json.RawMessage{row(`{"id":1,"v":"a"}`), row(`{"id":2,"v":"b"}`), row(`{"id":3,"v":"c"}`)},
			toKeys:   []string{"id:1", "id:3", "id:4"},
			toRows:   []json.RawMessage{row(`{"id":1,"v":"a"}`), row(`{"id":3,"v":"CHANGED"}`), row(`{"id":4,"v":"d"}`)},
			added:    1, removed: 1, changed: 1,
		},
		{
			name:     "hash rows: changed content shows as add+remove",
			fromKeys: []string{"hash:aa", "hash:bb"},
			fromRows: []json.RawMessage{row(`{"v":"a"}`), row(`{"v":"b"}`)},
			toKeys:   []string{"hash:aa", "hash:cc"},
			toRows:   []json.RawMessage{row(`{"v":"a"}`), row(`{"v":"c"}`)},
			added:    1, removed: 1, changed: 0,
		},
		{
			name:     "identical snapshots",
			fromKeys: []string{"id:1"},
			fromRows: []json.RawMessage{row(`{"id":1,"v":"a"}`)},
			toKeys:   []string{"id:1"},
			toRows:   []json.RawMessage{row(`{"id":1,"v":"a"}`)},
			added:    0, removed: 0, changed: 0,
		},
		{
			name:     "empty to populated",
			fromKeys: nil, fromRows: nil,
			toKeys: []string{"id:1"},
			toRows: []json.RawMessage{row(`{"id":1}`)},
			added:  1, removed: 0, changed: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			added, removed, changed := diffSnapshots(tt.fromKeys, tt.fromRows, tt.toKeys, tt.toRows)
			if len(added) != tt.added || len(removed) != tt.removed || len(changed) != tt.changed {
				t.Fatalf("got added=%d removed=%d changed=%d, want %d/%d/%d",
					len(added), len(removed), len(changed), tt.added, tt.removed, tt.changed)
			}
		})
	}
}

func TestDiffSnapshotsFieldOrderInsensitive(t *testing.T) {
	// Same id, same content, different field order: not changed.
	added, removed, changed := diffSnapshots(
		[]string{"id:1"}, []json.RawMessage{json.RawMessage(`{"id":1,"v":"a","w":2}`)},
		[]string{"id:1"}, []json.RawMessage{json.RawMessage(`{"w":2,"v":"a","id":1}`)},
	)
	if len(added)+len(removed)+len(changed) != 0 {
		t.Fatalf("field order alone must not register as a change: %d/%d/%d", len(added), len(removed), len(changed))
	}
}
